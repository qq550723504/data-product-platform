package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

const (
	createExecutionCommandType = "WORKFLOW.CREATE_EXECUTION"
	retryExecutionCommandType  = "WORKFLOW.RETRY_EXECUTION"
)

type CreateExecutionCommand struct {
	WorkspaceID       uuid.UUID
	WorkflowVersionID uuid.UUID
	OutputDatasetID   uuid.UUID
	TargetPeriod      string
	Inputs            []domain.InputBinding
	IdempotencyKey    string
	ActorID           *uuid.UUID
	TraceID           string
}

type ExecutionService struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewExecutionService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *ExecutionService {
	return &ExecutionService{tx: tx, repo: repo}
}

func (s *ExecutionService) Create(ctx context.Context, cmd CreateExecutionCommand) (domain.Execution, error) {
	key, err := domain.NormalizeIdempotencyKey(cmd.IdempotencyKey)
	if err != nil {
		return domain.Execution{}, err
	}
	execution, err := domain.NewExecution(cmd.WorkspaceID, cmd.WorkflowVersionID, cmd.OutputDatasetID, cmd.TargetPeriod, cmd.Inputs, cmd.ActorID)
	if err != nil {
		return domain.Execution{}, err
	}
	fingerprint, err := createExecutionFingerprint(execution)
	if err != nil {
		return domain.Execution{}, err
	}
	var result domain.Execution
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		inserted, err := s.repo.TryInsertIdempotency(ctx, tx, execution.WorkspaceID, createExecutionCommandType, key, execution.ID, &execution.ID, fingerprint)
		if err != nil {
			return err
		}
		if !inserted {
			record, found, err := s.repo.FindIdempotencyTx(ctx, tx, execution.WorkspaceID, createExecutionCommandType, key)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("execution idempotency record disappeared after conflict")
			}
			if record.RequestFingerprint != fingerprint {
				return domain.ErrIdempotencyConflict
			}
			stored, err := s.repo.GetExecutionTx(ctx, tx, record.ObjectID, true)
			if err != nil {
				return fmt.Errorf("load idempotent execution: %w", err)
			}
			if stored.WorkspaceID != execution.WorkspaceID {
				return domain.ErrWorkspaceMismatch
			}
			if err := s.repo.ValidateExecutionAccess(ctx, tx, stored); err != nil {
				return err
			}
			result = stored
			return nil
		}
		if err := s.repo.ValidateExecutionReferences(ctx, tx, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID, execution.Inputs); err != nil {
			return err
		}
		if err := s.repo.InsertExecution(ctx, tx, execution); err != nil {
			return err
		}
		if err := appendExecutionEvent(ctx, tx, execution, "ExecutionQueued"); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "EXECUTION_QUEUED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			AfterState:  executionAuditState(execution),
			TraceID:     cmd.TraceID,
		}); err != nil {
			return err
		}
		result = execution
		return nil
	})
	if err != nil {
		return domain.Execution{}, err
	}
	return result, nil
}

func (s *ExecutionService) Start(ctx context.Context, executionID uuid.UUID, engineExecutionID, traceID string) (domain.Execution, error) {
	return s.start(ctx, executionID, engineExecutionID, traceID, false)
}

// StartWithReferenceCheck is the native processing claim. It validates the Execution's
// references and transitions QUEUED -> RUNNING in one transaction, holding shared locks
// on the referenced DatasetVersions, so a concurrent invalidation either commits first
// (this call returns domain.ErrExecutionReferenceUnusable) or waits for the claim.
func (s *ExecutionService) StartWithReferenceCheck(ctx context.Context, executionID uuid.UUID, engineExecutionID, traceID string) (domain.Execution, error) {
	return s.start(ctx, executionID, engineExecutionID, traceID, true)
}

func (s *ExecutionService) start(ctx context.Context, executionID uuid.UUID, engineExecutionID, traceID string, validateReferences bool) (domain.Execution, error) {
	execution, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return domain.Execution{}, err
	}
	before := executionAuditState(execution)
	expected := execution.Status
	if err := execution.Start(engineExecutionID); err != nil {
		return domain.Execution{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if validateReferences {
			// Same transaction as the claim: take shared locks on the referenced
			// DatasetVersions, then validate. A concurrent invalidation cannot slip
			// between validation and the QUEUED -> RUNNING compare-and-set.
			if err := s.repo.LockExecutionInputVersions(ctx, tx, execution.Inputs); err != nil {
				return err
			}
			if err := s.repo.ValidateExecutionReferences(ctx, tx, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID, execution.Inputs); err != nil {
				return err
			}
		}
		if err := s.repo.SaveExecutionState(ctx, tx, execution, expected); err != nil {
			return err
		}
		if validateReferences {
			executionIDCopy := execution.ID
			if err := cost.Append(ctx, tx, cost.Event{
				WorkspaceID: execution.WorkspaceID,
				ExecutionID: &executionIDCopy,
				ActivityID:  nativeInitialInvocationActivityID(execution.ID),
				CostType:    cost.NativeEngineInvocation,
				Quantity:    1,
				Unit:        "invocation",
				PricingMode: "POC_ESTIMATE",
				Metadata: map[string]any{
					"engineType": "NATIVE",
					"recovery":   false,
				},
			}); err != nil {
				return err
			}
		}
		if err := appendExecutionEvent(ctx, tx, execution, "ExecutionStarted"); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_STARTED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			BeforeState: before,
			AfterState:  executionAuditState(execution),
			TraceID:     traceID,
		})
	})
	return execution, err
}

func (s *ExecutionService) RecordNativeRecovery(ctx context.Context, executionID uuid.UUID, action, traceID string) error {
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		execution, err := s.repo.GetExecutionTx(ctx, tx, executionID, true)
		if err != nil {
			return err
		}
		if execution.Status != domain.ExecutionRunning || execution.EngineType != "NATIVE" {
			return domain.ErrInvalidTransition
		}
		activityID := uuid.New()
		payload := map[string]any{
			"executionId":       execution.ID,
			"workflowVersionId": execution.WorkflowVersionID,
			"outputDatasetId":   execution.OutputDatasetID,
			"action":            action,
			"attempt":           execution.Attempt,
			"activityId":        activityID,
		}
		record, err := evidence.Append(ctx, tx, evidence.Record{
			ID:           activityID,
			WorkspaceID:  execution.WorkspaceID,
			EvidenceType: "EXECUTION_RECOVERY",
			Title:        "Native execution recovery started",
			SourceType:   "EXECUTION",
			SourceID:     &execution.ID,
			Metadata:     payload,
		}, evidence.Relation{ObjectType: "EXECUTION", ObjectID: execution.ID, RelationType: "RECOVERY"})
		if err != nil {
			return err
		}
		if action == "REEXECUTE" {
			executionIDCopy := execution.ID
			if err := cost.Append(ctx, tx, cost.Event{
				WorkspaceID: execution.WorkspaceID,
				ExecutionID: &executionIDCopy,
				ActivityID:  activityID,
				CostType:    cost.NativeEngineInvocation,
				Quantity:    1,
				Unit:        "invocation",
				PricingMode: "POC_ESTIMATE",
				Metadata: map[string]any{
					"engineType": "NATIVE",
					"recovery":   true,
					"action":     action,
				},
			}); err != nil {
				return err
			}
		}
		event, err := outbox.NewEvent("EXECUTION", execution.ID, "ExecutionRecoveryStarted", payload)
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_RECOVERY_STARTED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			AfterState: map[string]any{
				"status":     execution.Status,
				"action":     action,
				"activityId": activityID,
				"evidenceId": record.ID,
			},
			TraceID: traceID,
		})
	})
}

func (s *ExecutionService) Succeed(ctx context.Context, executionID, outputDatasetVersionID uuid.UUID, metrics map[string]any, traceID string) (domain.Execution, error) {
	execution, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return domain.Execution{}, err
	}
	before := executionAuditState(execution)
	expected := execution.Status
	if err := execution.Succeed(outputDatasetVersionID, metrics); err != nil {
		return domain.Execution{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.ValidateOutputVersion(ctx, tx, execution.OutputDatasetID, outputDatasetVersionID); err != nil {
			return err
		}
		// This compare-and-set must happen before Cost/Evidence/Audit/Outbox facts.
		// Only one reconciler is allowed to win RUNNING -> SUCCEEDED.
		if err := s.repo.SaveExecutionState(ctx, tx, execution, expected); err != nil {
			return err
		}
		executionIDCopy := execution.ID
		if err := cost.Append(ctx, tx, cost.Event{
			WorkspaceID: execution.WorkspaceID,
			ExecutionID: &executionIDCopy,
			CostType:    "PROCESSING_EXECUTION",
			Quantity:    1,
			Unit:        "execution",
			PricingMode: "POC_ESTIMATE",
			Metadata:    map[string]any{"engineType": execution.EngineType, "attempt": execution.Attempt},
		}); err != nil {
			return err
		}
		record, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  execution.WorkspaceID,
			EvidenceType: "PROCESSING_EXECUTION",
			Title:        "Workflow execution succeeded",
			SourceType:   "EXECUTION",
			SourceID:     &execution.ID,
			Metadata: map[string]any{
				"workflowVersionId":      execution.WorkflowVersionID,
				"outputDatasetVersionId": outputDatasetVersionID,
				"targetPeriod":           execution.TargetPeriod,
				"attempt":                execution.Attempt,
				"metrics":                metrics,
			},
		},
			evidence.Relation{ObjectType: "EXECUTION", ObjectID: execution.ID, RelationType: "SUPPORTS"},
			evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: outputDatasetVersionID, RelationType: "SUPPORTS"},
		)
		if err != nil {
			return err
		}
		if err := appendExecutionEvent(ctx, tx, execution, "ExecutionSucceeded"); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_SUCCEEDED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			BeforeState: before,
			AfterState: map[string]any{
				"status":                 execution.Status,
				"outputDatasetVersionId": outputDatasetVersionID,
				"evidenceId":             record.ID,
			},
			TraceID: traceID,
		})
	})
	return execution, err
}

func (s *ExecutionService) Fail(ctx context.Context, executionID uuid.UUID, code, message string, metrics map[string]any, traceID string) (domain.Execution, error) {
	execution, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return domain.Execution{}, err
	}
	before := executionAuditState(execution)
	expected := execution.Status
	if err := execution.Fail(code, message, metrics); err != nil {
		return domain.Execution{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.SaveExecutionState(ctx, tx, execution, expected); err != nil {
			return err
		}
		if err := appendExecutionEvent(ctx, tx, execution, "ExecutionFailed"); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_FAILED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			BeforeState: before,
			AfterState:  executionAuditState(execution),
			Reason:      execution.ErrorMessage,
			TraceID:     traceID,
		})
	})
	return execution, err
}

type RetryExecutionCommand struct {
	ExecutionID    uuid.UUID
	IdempotencyKey string
	ActorID        *uuid.UUID
	TraceID        string
}

func (s *ExecutionService) Retry(ctx context.Context, cmd RetryExecutionCommand) (domain.Execution, error) {
	key, err := domain.NormalizeIdempotencyKey(cmd.IdempotencyKey)
	if err != nil {
		return domain.Execution{}, err
	}
	var result domain.Execution
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		previous, err := s.repo.GetExecutionTx(ctx, tx, cmd.ExecutionID, true)
		if err != nil {
			return err
		}
		fingerprint, err := retryExecutionFingerprint(previous, cmd.ActorID)
		if err != nil {
			return err
		}
		// The original Execution row is locked before checking/inserting the
		// command record. This makes concurrent retries deterministic and lets a
		// replay return its original child even if the source has since changed.
		inserted, err := s.repo.TryInsertIdempotency(ctx, tx, previous.WorkspaceID, retryExecutionCommandType, key, uuid.New(), nil, fingerprint)
		if err != nil {
			return err
		}
		if !inserted {
			record, found, err := s.repo.FindIdempotencyTx(ctx, tx, previous.WorkspaceID, retryExecutionCommandType, key)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("retry idempotency record disappeared after conflict")
			}
			if record.RequestFingerprint != fingerprint {
				return domain.ErrIdempotencyConflict
			}
			stored, err := s.repo.GetExecutionTx(ctx, tx, record.ObjectID, true)
			if err != nil {
				return fmt.Errorf("load idempotent retry execution: %w", err)
			}
			if stored.WorkspaceID != previous.WorkspaceID {
				return domain.ErrWorkspaceMismatch
			}
			if err := s.repo.ValidateExecutionAccess(ctx, tx, stored); err != nil {
				return err
			}
			result = stored
			return nil
		}

		// Generate the child only after the idempotency slot is won. The random
		// child ID never enters request comparison and cannot create a second
		// business fact for a concurrent retry with the same key.
		retry, err := previous.Retry(cmd.ActorID)
		if err != nil {
			return err
		}
		if err := s.repo.ValidateExecutionReferences(ctx, tx, retry.WorkspaceID, retry.WorkflowVersionID, retry.OutputDatasetID, retry.Inputs); err != nil {
			return err
		}
		if err := s.repo.InsertExecution(ctx, tx, retry); err != nil {
			return err
		}
		if err := s.repo.UpdateIdempotencyObject(ctx, tx, previous.WorkspaceID, retryExecutionCommandType, key, retry.ID); err != nil {
			return err
		}
		if err := appendExecutionEvent(ctx, tx, retry, "ExecutionRetried"); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &retry.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "EXECUTION_RETRIED",
			ObjectType:  "EXECUTION",
			ObjectID:    retry.ID,
			AfterState:  executionAuditState(retry),
			Reason:      fmt.Sprintf("retry of %s", previous.ID),
			TraceID:     cmd.TraceID,
		}); err != nil {
			return err
		}
		result = retry
		return nil
	})
	if err != nil {
		return domain.Execution{}, err
	}
	return result, nil
}

func nativeInitialInvocationActivityID(executionID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("native-engine-invocation:initial:"+executionID.String()))
}

func appendExecutionEvent(ctx context.Context, tx pgx.Tx, execution domain.Execution, eventType string) error {
	event, err := outbox.NewEvent("EXECUTION", execution.ID, eventType, map[string]any{
		"executionId":            execution.ID,
		"workflowVersionId":      execution.WorkflowVersionID,
		"status":                 execution.Status,
		"attempt":                execution.Attempt,
		"retryOfExecutionId":     execution.RetryOfExecutionID,
		"outputDatasetVersionId": execution.OutputDatasetVersionID,
	})
	if err != nil {
		return err
	}
	return outbox.Append(ctx, tx, event)
}

func executionAuditState(execution domain.Execution) map[string]any {
	return map[string]any{
		"status":                 execution.Status,
		"attempt":                execution.Attempt,
		"workflowVersionId":      execution.WorkflowVersionID,
		"targetPeriod":           execution.TargetPeriod,
		"engineType":             execution.EngineType,
		"engineExecutionId":      execution.EngineExecutionID,
		"outputDatasetVersionId": execution.OutputDatasetVersionID,
		"errorCode":              execution.ErrorCode,
	}
}

type executionRequestInput struct {
	Name             string    `json:"name"`
	DatasetVersionID uuid.UUID `json:"datasetVersionId"`
}

type createExecutionRequestFingerprint struct {
	WorkspaceID       uuid.UUID               `json:"workspaceId"`
	WorkflowVersionID uuid.UUID               `json:"workflowVersionId"`
	OutputDatasetID   uuid.UUID               `json:"outputDatasetId"`
	TargetPeriod      string                  `json:"targetPeriod"`
	Inputs            []executionRequestInput `json:"inputs"`
	ActorID           *uuid.UUID              `json:"actorId,omitempty"`
}

type retryExecutionRequestFingerprint struct {
	WorkspaceID       uuid.UUID               `json:"workspaceId"`
	OriginalExecution uuid.UUID               `json:"originalExecutionId"`
	WorkflowVersionID uuid.UUID               `json:"workflowVersionId"`
	OutputDatasetID   uuid.UUID               `json:"outputDatasetId"`
	TargetPeriod      string                  `json:"targetPeriod"`
	Inputs            []executionRequestInput `json:"inputs"`
	ActorID           *uuid.UUID              `json:"actorId,omitempty"`
}

func createExecutionFingerprint(execution domain.Execution) (string, error) {
	payload := createExecutionRequestFingerprint{
		WorkspaceID:       execution.WorkspaceID,
		WorkflowVersionID: execution.WorkflowVersionID,
		OutputDatasetID:   execution.OutputDatasetID,
		TargetPeriod:      execution.TargetPeriod,
		Inputs:            fingerprintInputs(execution.Inputs),
		ActorID:           execution.CreatedBy,
	}
	return hashExecutionRequest(payload)
}

func retryExecutionFingerprint(execution domain.Execution, actorID *uuid.UUID) (string, error) {
	payload := retryExecutionRequestFingerprint{
		WorkspaceID:       execution.WorkspaceID,
		OriginalExecution: execution.ID,
		WorkflowVersionID: execution.WorkflowVersionID,
		OutputDatasetID:   execution.OutputDatasetID,
		TargetPeriod:      execution.TargetPeriod,
		Inputs:            fingerprintInputs(execution.Inputs),
		ActorID:           actorID,
	}
	return hashExecutionRequest(payload)
}

func fingerprintInputs(inputs []domain.InputBinding) []executionRequestInput {
	result := make([]executionRequestInput, 0, len(inputs))
	for _, input := range inputs {
		result = append(result, executionRequestInput{Name: input.Name, DatasetVersionID: input.DatasetVersionID})
	}
	return result
}

func hashExecutionRequest(payload any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal execution idempotency fingerprint: %w", err)
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}
