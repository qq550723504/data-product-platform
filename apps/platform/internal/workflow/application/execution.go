package application

import (
	"context"
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

type ExecutionQueue interface {
	EnqueueExecution(ctx context.Context, executionID uuid.UUID) error
}

type CreateExecutionCommand struct {
	WorkspaceID       uuid.UUID
	WorkflowVersionID uuid.UUID
	OutputDatasetID   uuid.UUID
	TargetPeriod      string
	Inputs            []domain.InputBinding
	ActorID           *uuid.UUID
	TraceID           string
}

type ExecutionService struct {
	tx    *transaction.Manager
	repo  *infrastructure.PostgresRepository
	queue ExecutionQueue
}

func NewExecutionService(tx *transaction.Manager, repo *infrastructure.PostgresRepository, queue ExecutionQueue) *ExecutionService {
	return &ExecutionService{tx: tx, repo: repo, queue: queue}
}

func (s *ExecutionService) Create(ctx context.Context, cmd CreateExecutionCommand) (domain.Execution, error) {
	execution, err := domain.NewExecution(cmd.WorkspaceID, cmd.WorkflowVersionID, cmd.OutputDatasetID, cmd.TargetPeriod, cmd.Inputs, cmd.ActorID)
	if err != nil {
		return domain.Execution{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.ValidateExecutionReferences(ctx, tx, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID, execution.Inputs); err != nil {
			return err
		}
		if err := s.repo.InsertExecution(ctx, tx, execution); err != nil {
			return err
		}
		if err := appendExecutionEvent(ctx, tx, execution, "ExecutionQueued"); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "EXECUTION_QUEUED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			AfterState:  executionAuditState(execution),
			TraceID:     cmd.TraceID,
		})
	})
	if err != nil {
		return domain.Execution{}, err
	}
	if s.queue != nil {
		if err := s.queue.EnqueueExecution(ctx, execution.ID); err != nil {
			return execution, fmt.Errorf("execution %s persisted but enqueue failed: %w", execution.ID, err)
		}
	}
	return execution, nil
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

func (s *ExecutionService) Retry(ctx context.Context, executionID uuid.UUID, actorID *uuid.UUID, traceID string) (domain.Execution, error) {
	previous, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return domain.Execution{}, err
	}
	retry, err := previous.Retry(actorID)
	if err != nil {
		return domain.Execution{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.ValidateExecutionReferences(ctx, tx, retry.WorkspaceID, retry.WorkflowVersionID, retry.OutputDatasetID, retry.Inputs); err != nil {
			return err
		}
		if err := s.repo.InsertExecution(ctx, tx, retry); err != nil {
			return err
		}
		if err := appendExecutionEvent(ctx, tx, retry, "ExecutionRetried"); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &retry.WorkspaceID,
			ActorType:   actorType(actorID),
			ActorID:     actorID,
			Action:      "EXECUTION_RETRIED",
			ObjectType:  "EXECUTION",
			ObjectID:    retry.ID,
			AfterState:  executionAuditState(retry),
			Reason:      fmt.Sprintf("retry of %s", previous.ID),
			TraceID:     traceID,
		})
	})
	if err != nil {
		return domain.Execution{}, err
	}
	if s.queue != nil {
		if err := s.queue.EnqueueExecution(ctx, retry.ID); err != nil {
			return retry, fmt.Errorf("retry %s persisted but enqueue failed: %w", retry.ID, err)
		}
	}
	return retry, nil
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
