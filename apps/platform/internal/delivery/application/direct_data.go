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
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const (
	directDataCommandType   = "DELIVERY.DIRECT_DATA"
	directDataProviderName  = "PLATFORM_DIRECT_DATA"
	directDataChannel       = "DIRECT_DATA"
	directDataMode          = "DIRECT_DATA"
	directDataAuthorization = 5 * time.Minute
)

var (
	ErrDirectDataReplayRequiresNewAttempt = errors.New("direct data replay requires a new delivery attempt")
	ErrDeliveryIdempotencyConflict        = errors.New("delivery idempotency key conflicts with another request")
)

// DirectDataGateRequest is intentionally separate from the provider-credential
// GateRequest. DIRECT_DATA has no external capability to recover or contain,
// but still has to evaluate the same current certification/rights facts while
// the shared delivery authorization fence is held.
type DirectDataGateRequest struct {
	OperationID          uuid.UUID
	WorkspaceID          uuid.UUID
	DatasetVersionID     uuid.UUID
	ProfileID            uuid.UUID
	PrincipalRef         string
	EffectiveConsumerRef string
	Purpose              string
	Action               string
	ScopeType            string
	ScopeRef             string
	DeliveryChannel      string
	DeliveryMode         string
	RequestedExpiresAt   time.Time
}

type DirectDataGateResult struct {
	Evaluation       domain.GateEvaluation
	CertificationRef *uuid.UUID
}

type DirectDataGate interface {
	EvaluateDirectData(context.Context, pgx.Tx, DirectDataGateRequest) (DirectDataGateResult, error)
}

type DirectDataCommand struct {
	WorkspaceID                uuid.UUID
	DatasetVersionID           uuid.UUID
	ProfileID                  uuid.UUID
	PrincipalRef               string
	EffectiveConsumerRef       string
	Purpose                    string
	Action                     string
	ScopeType                  string
	ScopeRef                   string
	RetryOfDeliveryOperationID *uuid.UUID
	IdempotencyKey             string
	TraceID                    string
}

type DirectDataResult struct {
	Operation      domain.Operation
	DatasetVersion datasetdomain.DatasetVersion
	Blockers       []string
	PayloadReady   bool
	ReplayRequired bool
}

type DirectDataService struct {
	tx       *transaction.Manager
	repo     *infrastructure.PostgresRepository
	gate     DirectDataGate
	datasets *datasetinfra.PostgresRepository
	now      func() time.Time
}

func NewDirectDataService(tx *transaction.Manager, repo *infrastructure.PostgresRepository, gate DirectDataGate, datasets *datasetinfra.PostgresRepository) *DirectDataService {
	return &DirectDataService{
		tx: tx, repo: repo, gate: gate, datasets: datasets,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Deliver linearizes authorization and ISSUED/BLOCKED in one PostgreSQL
// transaction. It never reads object bytes. Callers may open the returned
// DatasetVersion.StorageURI only after this method returns PayloadReady=true,
// which means the ISSUED transaction has already committed.
func (s *DirectDataService) Deliver(ctx context.Context, cmd DirectDataCommand) (DirectDataResult, error) {
	if s == nil || s.tx == nil || s.repo == nil || s.gate == nil || s.datasets == nil || s.now == nil {
		return DirectDataResult{}, fmt.Errorf("direct data delivery service is not configured")
	}
	cmd = normalizeDirectDataCommand(cmd)
	if err := validateDirectDataCommand(cmd); err != nil {
		return DirectDataResult{}, err
	}
	fingerprint, err := directDataFingerprint(cmd)
	if err != nil {
		return DirectDataResult{}, err
	}

	var result DirectDataResult
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		candidateID := uuid.New()
		inserted, err := s.repo.TryInsertIdempotencyForCommand(
			ctx, tx, cmd.WorkspaceID, directDataCommandType, cmd.IdempotencyKey, fingerprint, candidateID,
		)
		if err != nil {
			return err
		}
		if !inserted {
			record, found, err := s.repo.FindIdempotencyForCommand(ctx, tx, cmd.WorkspaceID, directDataCommandType, cmd.IdempotencyKey)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("direct data idempotency record disappeared after conflict")
			}
			if record.RequestFingerprint != fingerprint {
				return ErrDeliveryIdempotencyConflict
			}
			operation, err := s.repo.GetOperation(ctx, tx, record.ObjectID, false)
			if err != nil {
				return err
			}
			result.Operation = operation
			switch operation.Status {
			case domain.StatusIssued:
				result.ReplayRequired = true
				return nil
			case domain.StatusBlocked, domain.StatusFailed:
				if operation.TerminalReason != "" {
					result.Blockers = []string{operation.TerminalReason}
				}
				return nil
			default:
				return fmt.Errorf("direct data operation %s is in unexpected non-terminal state %s", operation.ID, operation.Status)
			}
		}

		if cmd.RetryOfDeliveryOperationID != nil {
			prior, err := s.repo.GetOperation(ctx, tx, *cmd.RetryOfDeliveryOperationID, false)
			if err != nil {
				return fmt.Errorf("%w: retry_of delivery operation is not available", domain.ErrInvalidOperation)
			}
			if prior.Status != domain.StatusIssued ||
				prior.WorkspaceID != cmd.WorkspaceID ||
				prior.DatasetVersionID != cmd.DatasetVersionID ||
				prior.PrincipalRef != cmd.PrincipalRef ||
				prior.EffectiveConsumerRef != cmd.EffectiveConsumerRef ||
				prior.Purpose != cmd.Purpose ||
				prior.Action != cmd.Action ||
				prior.ProviderName != directDataProviderName ||
				prior.DeliveryChannel != directDataChannel ||
				prior.DeliveryMode != directDataMode {
				return fmt.Errorf("%w: retry_of must reference the same caller/context and an ISSUED DIRECT_DATA attempt", domain.ErrInvalidOperation)
			}
			priorProfileID, err := s.repo.GetTerminalGateCertificationProfile(ctx, tx, prior.ID)
			if errors.Is(err, infrastructure.ErrNotFound) || (err == nil && priorProfileID != cmd.ProfileID) {
				return fmt.Errorf("%w: retry_of must reference an ISSUED DIRECT_DATA attempt evaluated with the same certification profile", domain.ErrInvalidOperation)
			}
			if err != nil {
				return fmt.Errorf("load retry_of delivery certification profile: %w", err)
			}
		}

		revision, err := s.repo.LockFence(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return err
		}
		requestedExpiresAt := s.now().Add(directDataAuthorization)
		gateResult, err := s.gate.EvaluateDirectData(ctx, tx, DirectDataGateRequest{
			OperationID: candidateID, WorkspaceID: cmd.WorkspaceID, DatasetVersionID: cmd.DatasetVersionID,
			ProfileID: cmd.ProfileID, PrincipalRef: cmd.PrincipalRef,
			EffectiveConsumerRef: cmd.EffectiveConsumerRef,
			Purpose:              cmd.Purpose,
			Action:               cmd.Action,
			ScopeType:            cmd.ScopeType, ScopeRef: cmd.DatasetVersionID.String(),
			DeliveryChannel: directDataChannel, DeliveryMode: directDataMode, RequestedExpiresAt: requestedExpiresAt,
		})
		if err != nil {
			return fmt.Errorf("evaluate direct data delivery gate: %w", err)
		}
		evaluation := gateResult.Evaluation
		evaluation.ID = uuid.New()
		evaluation.EvaluationKey = "direct/terminal/" + candidateID.String()
		evaluation.Stage = domain.GateTerminalFinalize
		evaluation.CertificationProfileID = &cmd.ProfileID
		evaluation.DependencyRevision = revision
		if evaluation.Allowed {
			if evaluation.PrincipalRef != cmd.PrincipalRef ||
				evaluation.EffectiveConsumerRef != cmd.EffectiveConsumerRef ||
				evaluation.DelegationRef != "" {
				return fmt.Errorf("direct data gate returned untrusted caller context")
			}
			if evaluation.FreshCapExpiresAt == nil || evaluation.FreshCapExpiresAt.After(requestedExpiresAt) {
				return fmt.Errorf("direct data gate returned an invalid authorization cap")
			}
		} else {
			if evaluation.PrincipalRef == "" {
				evaluation.PrincipalRef = strings.TrimSpace(cmd.PrincipalRef)
			}
			if evaluation.EffectiveConsumerRef == "" {
				evaluation.EffectiveConsumerRef = strings.TrimSpace(cmd.EffectiveConsumerRef)
			}
		}

		operation, err := domain.NewOperation(
			cmd.WorkspaceID, cmd.DatasetVersionID, cmd.IdempotencyKey, directDataProviderName,
			cmd.PrincipalRef, cmd.EffectiveConsumerRef, "", cmd.Purpose, cmd.Action,
			directDataScopeRef(cmd), directDataChannel, directDataMode, requestedExpiresAt, gateResult.CertificationRef,
		)
		if err != nil {
			return err
		}
		operation.ID = candidateID
		operation.RetryOfDeliveryOperationID = cmd.RetryOfDeliveryOperationID
		operation.DependencyRevision = evaluation.DependencyRevision
		operation.FreshCapExpiresAt = evaluation.FreshCapExpiresAt
		operation.CurrentGateDecision = evaluation.Decision()

		if err := s.repo.InsertOperation(ctx, tx, operation); err != nil {
			return err
		}
		// DIRECT_DATA has no external provider invocation, but it is still one
		// physical delivery attempt. Persist that activity in the same
		// transaction as the DeliveryOperation so blocked/issued attempts are
		// both cost-auditable and same-key replay cannot duplicate cost.
		if err := s.repo.InsertDirectDataCost(ctx, tx, operation); err != nil {
			return err
		}
		if err := s.repo.InsertGateEvaluation(ctx, tx, operation.ID, evaluation, operation.CreatedAt); err != nil {
			return err
		}
		if err := appendGateFacts(ctx, tx, operation, evaluation, nil, cmd.TraceID); err != nil {
			return err
		}

		before := operation.Status
		if evaluation.Allowed {
			if err := operation.Transition(domain.StatusIssued); err != nil {
				return err
			}
			if err := s.repo.InsertTransition(
				ctx, tx, operation.ID, "direct/issued", before, operation.Status,
				&evaluation.ID, nil, "direct data authorization linearized; transfer not confirmed", operation.UpdatedAt,
			); err != nil {
				return err
			}
			if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
				return err
			}
			if err := appendTransitionFacts(
				ctx, tx, operation, before, "DatasetDeliveryIssued",
				"direct data authorization linearized; transfer not confirmed", nil, cmd.TraceID,
			); err != nil {
				return err
			}
			result.PayloadReady = true
		} else {
			if err := operation.Transition(domain.StatusBlocked); err != nil {
				return err
			}
			operation.TerminalReason = firstBlocker(evaluation.Blockers)
			if err := s.repo.InsertTransition(
				ctx, tx, operation.ID, "direct/blocked", before, operation.Status,
				&evaluation.ID, nil, operation.TerminalReason, operation.UpdatedAt,
			); err != nil {
				return err
			}
			if err := s.repo.UpdateProjection(ctx, tx, operation); err != nil {
				return err
			}
			if err := appendTransitionFacts(ctx, tx, operation, before, "DatasetDeliveryBlocked", operation.TerminalReason, nil, cmd.TraceID); err != nil {
				return err
			}
			result.Blockers = append([]string(nil), evaluation.Blockers...)
		}
		result.Operation = operation
		return nil
	})
	if err != nil {
		return DirectDataResult{}, err
	}
	if result.ReplayRequired {
		return result, ErrDirectDataReplayRequiresNewAttempt
	}
	if !result.PayloadReady {
		return result, nil
	}

	version, err := s.datasets.GetVersion(ctx, result.Operation.DatasetVersionID)
	if err != nil {
		return result, fmt.Errorf("load issued direct data DatasetVersion: %w", err)
	}
	workspaceID, _, err := s.datasets.GetWorkspaceAndType(ctx, version.DatasetID)
	if err != nil {
		return result, fmt.Errorf("load issued direct data Dataset workspace: %w", err)
	}
	if workspaceID != result.Operation.WorkspaceID || strings.TrimSpace(version.StorageURI) == "" {
		return result, fmt.Errorf("issued direct data target no longer resolves to its immutable storage object")
	}
	result.DatasetVersion = version
	return result, nil
}

func normalizeDirectDataCommand(cmd DirectDataCommand) DirectDataCommand {
	cmd.PrincipalRef = strings.TrimSpace(cmd.PrincipalRef)
	cmd.EffectiveConsumerRef = strings.TrimSpace(cmd.EffectiveConsumerRef)
	cmd.Purpose = strings.ToUpper(strings.TrimSpace(cmd.Purpose))
	cmd.Action = strings.ToUpper(strings.TrimSpace(cmd.Action))
	cmd.ScopeType = strings.ToUpper(strings.TrimSpace(cmd.ScopeType))
	cmd.ScopeRef = strings.TrimSpace(cmd.ScopeRef)
	cmd.IdempotencyKey = strings.TrimSpace(cmd.IdempotencyKey)
	cmd.TraceID = strings.TrimSpace(cmd.TraceID)
	return cmd
}

func validateDirectDataCommand(cmd DirectDataCommand) error {
	if cmd.WorkspaceID == uuid.Nil || cmd.DatasetVersionID == uuid.Nil || cmd.ProfileID == uuid.Nil ||
		strings.TrimSpace(cmd.PrincipalRef) == "" || strings.TrimSpace(cmd.EffectiveConsumerRef) == "" ||
		strings.TrimSpace(cmd.Purpose) == "" || strings.TrimSpace(cmd.Action) == "" ||
		strings.TrimSpace(cmd.IdempotencyKey) == "" || len(cmd.IdempotencyKey) > 255 {
		return domain.ErrInvalidOperation
	}
	if cmd.RetryOfDeliveryOperationID != nil && *cmd.RetryOfDeliveryOperationID == uuid.Nil {
		return fmt.Errorf("%w: retryOfDeliveryOperationId must be a non-nil UUID", domain.ErrInvalidOperation)
	}
	if cmd.ScopeType != "ALL_RESOURCE" {
		return fmt.Errorf("%w: DIRECT_DATA first slice supports only ALL_RESOURCE scope", domain.ErrInvalidOperation)
	}
	return nil
}

func directDataScopeRef(cmd DirectDataCommand) string {
	// The first DIRECT_DATA slice supports only ALL_RESOURCE. Persist the
	// platform-owned target identity rather than a client-provided scopeRef that
	// the entitlement gate would not use for this scope mode.
	return cmd.DatasetVersionID.String()
}

func directDataFingerprint(cmd DirectDataCommand) (string, error) {
	payload := struct {
		WorkspaceID      uuid.UUID  `json:"workspaceId"`
		DatasetVersionID uuid.UUID  `json:"datasetVersionId"`
		ProfileID        uuid.UUID  `json:"profileId"`
		PrincipalRef     string     `json:"principalRef"`
		ConsumerRef      string     `json:"consumerRef"`
		Purpose          string     `json:"purpose"`
		Action           string     `json:"action"`
		ScopeType        string     `json:"scopeType"`
		ScopeRef         string     `json:"scopeRef,omitempty"`
		RetryOf          *uuid.UUID `json:"retryOfDeliveryOperationId,omitempty"`
	}{
		cmd.WorkspaceID, cmd.DatasetVersionID, cmd.ProfileID,
		cmd.PrincipalRef, cmd.EffectiveConsumerRef,
		cmd.Purpose, cmd.Action,
		cmd.ScopeType, "", cmd.RetryOfDeliveryOperationID,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal direct data delivery fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
