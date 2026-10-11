package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
)

type PrepareCertificationInputsCommand struct {
	WorkspaceID, DatasetVersionID, ProfileID, QualityAssessmentID uuid.UUID
	ActorID                                                       *uuid.UUID
	IdempotencyKey, TraceID                                       string
}

// PrepareInputs reuses the existing rights calculator, evidence aggregate,
// command idempotency and cross-transaction lock. It never certifies a version.
func (s *CertificationService) PrepareInputs(ctx context.Context, rights *rightsapp.Service, cmd PrepareCertificationInputsCommand) (map[string]string, error) {
	key := strings.TrimSpace(cmd.IdempotencyKey)
	if cmd.ActorID == nil || key == "" || len(key) > 255 {
		return nil, fmt.Errorf("trusted actor and bounded request key required")
	}
	profile, err := s.profileRepo.GetProfile(ctx, cmd.ProfileID)
	if err != nil {
		return nil, err
	}
	if profile.WorkspaceID != cmd.WorkspaceID || profile.Purpose.Mode != "EXPLICIT" || len(profile.Purpose.Values) != 1 || profile.Consumers.Mode != "EXPLICIT" || len(profile.Consumers.Values) != 1 || profile.ContractCode == "" {
		return nil, fmt.Errorf("explicit unambiguous preparation context required")
	}
	payload, _ := json.Marshal([]any{cmd.WorkspaceID, cmd.DatasetVersionID, cmd.ProfileID, cmd.QualityAssessmentID, cmd.ActorID})
	digest := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(digest[:])
	const commandType = "CERTIFICATION.PREPARE_INPUTS"
	var refs map[string]string
	err = s.tx.WithAdvisoryLock(ctx, "certification-inputs:"+cmd.WorkspaceID.String()+":"+key, func(ctx context.Context) error {
		var replay bool
		if err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			record, found, err := s.certificationRepo.FindIdempotencyTx(ctx, tx, cmd.WorkspaceID, commandType, key)
			if err != nil {
				return err
			}
			if !found {
				return nil
			}
			if record.RequestFingerprint != fingerprint {
				return ErrCertificationIdempotencyConflict
			}
			var data []byte
			if err := tx.QueryRow(ctx, `SELECT manifest->'references' FROM evidence_snapshot WHERE id=$1 AND workspace_id=$2 AND status='FINALIZED'`, record.ObjectID, cmd.WorkspaceID).Scan(&data); err != nil {
				return err
			}
			if err := json.Unmarshal(data, &refs); err != nil {
				return err
			}
			refs["traceabilityEvidenceId"] = record.ObjectID.String()
			refs["evidenceSnapshotId"] = record.ObjectID.String()
			replay = true
			return nil
		}); err != nil {
			return err
		}
		if replay {
			return nil
		}
		if err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := s.certificationRepo.PreparationFacts(ctx, tx, cmd.WorkspaceID, cmd.DatasetVersionID, cmd.QualityAssessmentID, profile.ContractCode)
			return err
		}); err != nil {
			return err
		}
		activity := uuid.NewSHA1(uuid.NameSpaceURL, []byte("certification-inputs-rights:"+cmd.WorkspaceID.String()+":"+key))
		effective, err := rights.ComputeEffectiveRights(ctx, rightsapp.ComputeEffectiveRightsCommand{WorkspaceID: cmd.WorkspaceID, TargetDatasetVersionID: cmd.DatasetVersionID, ConsumerRef: profile.Consumers.Values[0], Purpose: profile.Purpose.Values[0], ActorID: cmd.ActorID, TraceID: cmd.TraceID, ActivityID: &activity})
		if err != nil {
			return err
		}
		return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			facts, err := s.certificationRepo.PreparationFacts(ctx, tx, cmd.WorkspaceID, cmd.DatasetVersionID, cmd.QualityAssessmentID, profile.ContractCode)
			if err != nil {
				return err
			}
			refs = map[string]string{"effectiveRightsSnapshotId": effective.ID.String(), "complianceResultId": facts.ComplianceID.String(), "contractVersionId": facts.ContractID.String()}
			snapshot, err := evidence.CreateSnapshot(ctx, tx, cmd.WorkspaceID, "DATASET_VERSION", cmd.DatasetVersionID, map[string]any{"preparationType": "CERTIFICATION_INPUTS", "profileId": cmd.ProfileID, "qualityAssessmentId": cmd.QualityAssessmentID, "executionId": facts.ExecutionID, "references": refs}, facts.Items, cmd.ActorID)
			if err != nil {
				return err
			}
			inserted, err := s.certificationRepo.TryInsertIdempotency(ctx, tx, cmd.WorkspaceID, commandType, key, snapshot.ID, fingerprint)
			if err != nil {
				return err
			}
			if !inserted {
				return ErrCertificationIdempotencyConflict
			}
			refs["traceabilityEvidenceId"] = snapshot.ID.String()
			refs["evidenceSnapshotId"] = snapshot.ID.String()
			event, err := outbox.NewEvent("DATASET_VERSION", cmd.DatasetVersionID, "DatasetCertificationInputsPrepared", map[string]any{"workspaceId": cmd.WorkspaceID, "datasetVersionId": cmd.DatasetVersionID, "profileId": cmd.ProfileID, "evidenceSnapshotId": snapshot.ID, "rootHash": snapshot.RootHash})
			if err != nil {
				return err
			}
			if err := outbox.Append(ctx, tx, event); err != nil {
				return err
			}
			return audit.Append(ctx, tx, audit.Event{WorkspaceID: &cmd.WorkspaceID, ActorType: "USER", ActorID: cmd.ActorID, Action: "CERTIFICATION_INPUTS_PREPARED", ObjectType: "EVIDENCE_SNAPSHOT", ObjectID: snapshot.ID, AfterState: map[string]any{"rootHash": snapshot.RootHash, "datasetVersionId": cmd.DatasetVersionID}, TraceID: cmd.TraceID})
		})
	})
	return refs, err
}

func (s *CertificationService) PreparedInputs(ctx context.Context, ws, version, profile, quality uuid.UUID) (map[string]string, error) {
	refs, err := s.certificationRepo.PreparedInputs(ctx, ws, version, profile, quality)
	return refs, err
}
