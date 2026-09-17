package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

func (s *ExecutionService) SelectEngine(ctx context.Context, executionID uuid.UUID, engineType, traceID string) (domain.Execution, error) {
	execution, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return domain.Execution{}, err
	}
	before := executionAuditState(execution)
	if err := execution.SelectEngine(engineType); err != nil {
		return domain.Execution{}, err
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.UpdateExecutionEngine(ctx, tx, execution.ID, execution.EngineType); err != nil {
			return err
		}
		event, err := outbox.NewEvent("EXECUTION", execution.ID, "ExecutionEngineSelected", map[string]any{
			"executionId": execution.ID,
			"engineType":  execution.EngineType,
			"status":      execution.Status,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_ENGINE_SELECTED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			BeforeState: before,
			AfterState: map[string]any{
				"status":     execution.Status,
				"engineType": execution.EngineType,
			},
			TraceID: traceID,
		})
	})
	return execution, err
}

// BeginManagedSubmission is the durable dispatch claim for a remote runtime.
// The state transition is persisted before any network call is made. Concurrent
// queue deliveries race on the expected QUEUED state and only one can win.
func (s *ExecutionService) BeginManagedSubmission(ctx context.Context, executionID uuid.UUID, engineType, traceID string) (domain.Execution, error) {
	execution, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return domain.Execution{}, err
	}
	before := executionAuditState(execution)
	expected := execution.Status
	if err := execution.BeginManagedSubmission(engineType); err != nil {
		return domain.Execution{}, err
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Validate and claim in one transaction. The shared lock on the referenced
		// DatasetVersions makes a concurrent invalidation either win (this returns
		// domain.ErrExecutionReferenceUnusable) or wait for the SUBMITTING claim.
		if err := s.repo.LockExecutionInputVersions(ctx, tx, execution.Inputs); err != nil {
			return err
		}
		if err := s.repo.ValidateExecutionReferences(ctx, tx, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID, execution.Inputs); err != nil {
			return err
		}
		if err := s.repo.SaveExecutionState(ctx, tx, execution, expected); err != nil {
			return err
		}
		event, err := outbox.NewEvent("EXECUTION", execution.ID, "ExecutionSubmitting", map[string]any{
			"executionId": execution.ID,
			"engineType":  execution.EngineType,
			"status":      execution.Status,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_SUBMITTING",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			BeforeState: before,
			AfterState:  executionAuditState(execution),
			TraceID:     traceID,
		})
	})
	return execution, err
}
