package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const evaluateCertificationCommandType = "CERTIFICATION.EVALUATE_DATASET"

var ErrCertificationIdempotencyConflict = errors.New("certification command idempotency conflict")

// EvidenceResolver is the application boundary for reading the immutable
// Quality/Rights/Compliance/Contract/Traceability/Evidence facts used by a
// certification. HTTP request fields must not be used as evidence directly.
type EvidenceResolver interface {
	Resolve(ctx context.Context, cmd EvaluateDatasetCertificationCommand, profile domain.ProfileSnapshot) (domain.EvaluationInput, error)
}

type CertificationService struct {
	tx                *transaction.Manager
	profileRepo       *certificationinfra.ProfileRepository
	certificationRepo *certificationinfra.CertificationRepository
	resolver          EvidenceResolver
}

func NewCertificationService(
	tx *transaction.Manager,
	profileRepo *certificationinfra.ProfileRepository,
	certificationRepo *certificationinfra.CertificationRepository,
	resolver EvidenceResolver,
) *CertificationService {
	return &CertificationService{
		tx: tx, profileRepo: profileRepo, certificationRepo: certificationRepo, resolver: resolver,
	}
}

type EvaluateDatasetCertificationCommand struct {
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	ProfileID        uuid.UUID
	IdempotencyKey   string
	ActorID          *uuid.UUID
	TraceID          string
	Now              time.Time
	CostActivity     *cost.CertificationActivity
}

type ChangeCertificationDispositionCommand struct {
	WorkspaceID                 uuid.UUID
	CertificationID             uuid.UUID
	Disposition                 domain.Disposition
	SupersededByCertificationID *uuid.UUID
	EvidenceSnapshotID          *uuid.UUID
	Reason                      string
	EffectiveAt                 time.Time
	IdempotencyKey              string
	ActorID                     *uuid.UUID
	TraceID                     string
	CostActivity                *cost.CertificationActivity
}

func (s *CertificationService) Evaluate(ctx context.Context, cmd EvaluateDatasetCertificationCommand) (domain.DatasetCertification, error) {
	if cmd.WorkspaceID == uuid.Nil || cmd.DatasetVersionID == uuid.Nil || cmd.ProfileID == uuid.Nil {
		return domain.DatasetCertification{}, fmt.Errorf("workspace, DatasetVersion, and profile are required")
	}
	if strings.TrimSpace(cmd.IdempotencyKey) == "" {
		return domain.DatasetCertification{}, fmt.Errorf("idempotency key is required")
	}
	if s.tx == nil || s.profileRepo == nil || s.certificationRepo == nil || s.resolver == nil {
		return domain.DatasetCertification{}, fmt.Errorf("certification service dependencies are incomplete")
	}

	profile, err := s.profileRepo.GetProfile(ctx, cmd.ProfileID)
	if err != nil {
		return domain.DatasetCertification{}, err
	}
	if profile.WorkspaceID != cmd.WorkspaceID {
		return domain.DatasetCertification{}, fmt.Errorf("certification profile does not belong to the command workspace")
	}

	input, err := s.resolver.Resolve(ctx, cmd, profile)
	if err != nil {
		return domain.DatasetCertification{}, fmt.Errorf("resolve certification evidence: %w", err)
	}
	if input.WorkspaceID != cmd.WorkspaceID || input.DatasetVersionID != cmd.DatasetVersionID {
		return domain.DatasetCertification{}, fmt.Errorf("resolved certification evidence crosses command boundary")
	}
	if input.ActorID != nil && (cmd.ActorID == nil || *input.ActorID != *cmd.ActorID) {
		return domain.DatasetCertification{}, fmt.Errorf("resolved certification actor conflicts with command actor")
	}
	input.ActorID = cmd.ActorID
	if !cmd.Now.IsZero() {
		input.IssuedAt = cmd.Now
	}
	if input.IssuedAt.IsZero() {
		input.IssuedAt = time.Now().UTC()
	}

	fingerprint, err := certificationFingerprint(cmd)
	if err != nil {
		return domain.DatasetCertification{}, err
	}
	var result domain.DatasetCertification
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.certificationRepo.BindTrustedEvaluationFactsTx(ctx, tx, profile, &input); err != nil {
			return err
		}
		candidate, err := domain.Evaluate(profile, input)
		if err != nil {
			return err
		}
		inserted, err := s.certificationRepo.TryInsertIdempotency(ctx, tx, cmd.WorkspaceID, evaluateCertificationCommandType, strings.TrimSpace(cmd.IdempotencyKey), candidate.ID, fingerprint)
		if err != nil {
			return err
		}
		if !inserted {
			record, found, err := s.certificationRepo.FindIdempotencyTx(ctx, tx, cmd.WorkspaceID, evaluateCertificationCommandType, strings.TrimSpace(cmd.IdempotencyKey))
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("certification idempotency record disappeared")
			}
			if record.RequestFingerprint != fingerprint {
				return fmt.Errorf("%w: key already used for a different certification request", ErrCertificationIdempotencyConflict)
			}
			result, err = s.certificationRepo.GetCertificationTx(ctx, tx, record.ObjectID, profile)
			return err
		}

		if err := s.certificationRepo.InsertCertification(ctx, tx, candidate); err != nil {
			return err
		}
		decisionEvidenceID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("dataset-certification-evidence:"+candidate.ID.String()))
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			ID:           decisionEvidenceID,
			WorkspaceID:  candidate.WorkspaceID,
			EvidenceType: "DATASET_CERTIFICATION_DECISION",
			Title:        "Dataset certification decision",
			SourceType:   "DATASET_CERTIFICATION",
			SourceID:     &candidate.ID,
			Metadata: map[string]any{
				"certificationId":  candidate.ID,
				"datasetVersionId": candidate.DatasetVersionID,
				"profileId":        candidate.Profile.ID,
				"decision":         candidate.Decision,
				"blockerCodes":     blockerCodes(candidate.Blockers),
			},
			CreatedAt: candidate.IssuedAt,
			CreatedBy: candidate.ActorID,
		}, evidence.Relation{
			ObjectType:   "DATASET_CERTIFICATION",
			ObjectID:     candidate.ID,
			RelationType: "DECISION_EVIDENCE",
		}); err != nil {
			return err
		}
		if cmd.CostActivity != nil {
			activity := *cmd.CostActivity
			activity.WorkspaceID = candidate.WorkspaceID
			activity.CertificationID = &candidate.ID
			activity.DispositionID = nil
			if err := cost.AppendCertificationActivity(ctx, tx, activity); err != nil {
				return err
			}
		}
		eventType := "DatasetCertificationRejected"
		action := "DATASET_CERTIFICATION_REJECTED"
		if candidate.Decision == domain.DecisionCertified {
			eventType = "DatasetCertified"
			action = "DATASET_CERTIFIED"
		}
		blockerCodes := blockerCodes(candidate.Blockers)
		event, err := outbox.NewEvent("DATASET_CERTIFICATION", candidate.ID, eventType, map[string]any{
			"certificationId":  candidate.ID,
			"workspaceId":      candidate.WorkspaceID,
			"datasetVersionId": candidate.DatasetVersionID,
			"profileId":        candidate.Profile.ID,
			"profileRef":       candidate.Profile.ProfileRef,
			"profileVersion":   candidate.Profile.Version,
			"decision":         candidate.Decision,
			"blockerCodes":     blockerCodes,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &candidate.WorkspaceID,
			ActorType:   actorType(candidate.ActorID),
			ActorID:     candidate.ActorID,
			Action:      action,
			ObjectType:  "DATASET_CERTIFICATION",
			ObjectID:    candidate.ID,
			AfterState: map[string]any{
				"decision":     candidate.Decision,
				"blockerCodes": blockerCodes,
				"profileId":    candidate.Profile.ID,
			},
			Reason:  candidate.Reason,
			TraceID: cmd.TraceID,
		}); err != nil {
			return err
		}
		result = candidate
		return nil
	})
	return result, err
}

func certificationFingerprint(cmd EvaluateDatasetCertificationCommand) (string, error) {
	payload, err := json.Marshal(struct {
		WorkspaceID      uuid.UUID `json:"workspaceId"`
		DatasetVersionID uuid.UUID `json:"datasetVersionId"`
		ProfileID        uuid.UUID `json:"profileId"`
	}{cmd.WorkspaceID, cmd.DatasetVersionID, cmd.ProfileID})
	if err != nil {
		return "", fmt.Errorf("marshal certification idempotency fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func (s *CertificationService) ChangeDisposition(ctx context.Context, cmd ChangeCertificationDispositionCommand) (domain.CertificationDisposition, error) {
	if cmd.WorkspaceID == uuid.Nil || cmd.CertificationID == uuid.Nil {
		return domain.CertificationDisposition{}, fmt.Errorf("workspace and certification are required")
	}
	if strings.TrimSpace(cmd.IdempotencyKey) == "" {
		return domain.CertificationDisposition{}, fmt.Errorf("idempotency key is required")
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		return domain.CertificationDisposition{}, fmt.Errorf("disposition reason is required")
	}
	if s.tx == nil || s.profileRepo == nil || s.certificationRepo == nil {
		return domain.CertificationDisposition{}, fmt.Errorf("certification service dependencies are incomplete")
	}
	profileID, err := s.certificationRepo.GetCertificationProfileID(ctx, cmd.CertificationID)
	if err != nil {
		return domain.CertificationDisposition{}, err
	}
	profile, err := s.profileRepo.GetProfile(ctx, profileID)
	if err != nil {
		return domain.CertificationDisposition{}, err
	}
	fingerprint, err := dispositionFingerprint(cmd)
	if err != nil {
		return domain.CertificationDisposition{}, err
	}
	var result domain.CertificationDisposition
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		certification, err := s.certificationRepo.GetCertificationTx(ctx, tx, cmd.CertificationID, profile)
		if err != nil {
			return err
		}
		if certification.WorkspaceID != cmd.WorkspaceID {
			return fmt.Errorf("certification does not belong to the command workspace")
		}
		if certification.Decision != domain.DecisionCertified {
			return fmt.Errorf("only CERTIFIED facts can receive a certification disposition")
		}
		if !cmd.EffectiveAt.IsZero() && cmd.EffectiveAt.Before(certification.IssuedAt) {
			return fmt.Errorf("certification disposition cannot precede certification issuance")
		}
		if cmd.Disposition == domain.DispositionSuperseded && cmd.SupersededByCertificationID != nil {
			replacement, err := s.certificationRepo.GetCertificationTargetTx(ctx, tx, *cmd.SupersededByCertificationID)
			if err != nil {
				return err
			}
			if replacement.WorkspaceID != certification.WorkspaceID ||
				replacement.DatasetVersionID != certification.DatasetVersionID ||
				replacement.ProfileID != profile.ID ||
				(replacement.Decision != domain.DecisionCertified && replacement.Decision != domain.DecisionRejected) {
				return fmt.Errorf("superseding certification must be a same-target profile replacement with CERTIFIED or REJECTED decision")
			}
		}
		candidate, err := domain.NewDisposition(cmd.WorkspaceID, cmd.CertificationID, cmd.Disposition, cmd.EffectiveAt, cmd.Reason, cmd.SupersededByCertificationID, cmd.EvidenceSnapshotID, cmd.ActorID)
		if err != nil {
			return err
		}
		inserted, err := s.certificationRepo.TryInsertIdempotency(ctx, tx, cmd.WorkspaceID, "CERTIFICATION.DISPOSITION", strings.TrimSpace(cmd.IdempotencyKey), candidate.ID, fingerprint)
		if err != nil {
			return err
		}
		if !inserted {
			record, found, err := s.certificationRepo.FindIdempotencyTx(ctx, tx, cmd.WorkspaceID, "CERTIFICATION.DISPOSITION", strings.TrimSpace(cmd.IdempotencyKey))
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("certification disposition idempotency record disappeared")
			}
			if record.RequestFingerprint != fingerprint {
				return fmt.Errorf("%w: key already used for a different disposition request", ErrCertificationIdempotencyConflict)
			}
			result, err = s.certificationRepo.GetDispositionTx(ctx, tx, record.ObjectID)
			return err
		}

		if err := s.certificationRepo.InsertDisposition(ctx, tx, candidate); err != nil {
			return err
		}
		decisionEvidenceID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("certification-disposition-evidence:"+candidate.ID.String()))
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			ID:           decisionEvidenceID,
			WorkspaceID:  candidate.WorkspaceID,
			EvidenceType: "CERTIFICATION_DISPOSITION",
			Title:        "Certification disposition",
			SourceType:   "CERTIFICATION_DISPOSITION",
			SourceID:     &candidate.ID,
			Metadata: map[string]any{
				"dispositionId":   candidate.ID,
				"certificationId": candidate.CertificationID,
				"disposition":     candidate.Disposition,
				"effectiveAt":     candidate.EffectiveAt,
			},
			CreatedBy: candidate.ActorID,
		}, evidence.Relation{
			ObjectType:   "CERTIFICATION_DISPOSITION",
			ObjectID:     candidate.ID,
			RelationType: "DISPOSITION_EVIDENCE",
		}); err != nil {
			return err
		}
		if cmd.CostActivity != nil {
			activity := *cmd.CostActivity
			activity.WorkspaceID = candidate.WorkspaceID
			activity.CertificationID = nil
			activity.DispositionID = &candidate.ID
			if err := cost.AppendCertificationActivity(ctx, tx, activity); err != nil {
				return err
			}
		}
		eventType := "DatasetCertificationRevoked"
		action := "DATASET_CERTIFICATION_REVOKED"
		if candidate.Disposition == domain.DispositionSuperseded {
			eventType = "DatasetCertificationSuperseded"
			action = "DATASET_CERTIFICATION_SUPERSEDED"
		}
		event, err := outbox.NewEvent("DATASET_CERTIFICATION", candidate.CertificationID, eventType, map[string]any{
			"dispositionId":   candidate.ID,
			"certificationId": candidate.CertificationID,
			"workspaceId":     candidate.WorkspaceID,
			"effectiveAt":     candidate.EffectiveAt,
			"replacementId":   candidate.SupersededByCertificationID,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &candidate.WorkspaceID,
			ActorType:   actorType(candidate.ActorID),
			ActorID:     candidate.ActorID,
			Action:      action,
			ObjectType:  "CERTIFICATION_DISPOSITION",
			ObjectID:    candidate.ID,
			AfterState: map[string]any{
				"certificationId": candidate.CertificationID,
				"disposition":     candidate.Disposition,
				"effectiveAt":     candidate.EffectiveAt,
			},
			Reason:  candidate.Reason,
			TraceID: cmd.TraceID,
		}); err != nil {
			return err
		}
		result = candidate
		return nil
	})
	return result, err
}

func blockerCodes(blockers []domain.Blocker) []string {
	codes := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		codes = append(codes, blocker.Code)
	}
	return codes
}

func dispositionFingerprint(cmd ChangeCertificationDispositionCommand) (string, error) {
	payload, err := json.Marshal(struct {
		WorkspaceID                 uuid.UUID          `json:"workspaceId"`
		CertificationID             uuid.UUID          `json:"certificationId"`
		Disposition                 domain.Disposition `json:"disposition"`
		SupersededByCertificationID *uuid.UUID         `json:"supersededByCertificationId,omitempty"`
		EvidenceSnapshotID          *uuid.UUID         `json:"evidenceSnapshotId,omitempty"`
		Reason                      string             `json:"reason"`
		EffectiveAt                 time.Time          `json:"effectiveAt"`
	}{cmd.WorkspaceID, cmd.CertificationID, cmd.Disposition, cmd.SupersededByCertificationID, cmd.EvidenceSnapshotID, strings.TrimSpace(cmd.Reason), cmd.EffectiveAt.UTC()})
	if err != nil {
		return "", fmt.Errorf("marshal certification disposition fingerprint: %w", err)
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
