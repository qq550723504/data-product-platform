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

// AttachManagedSubmissionReference preserves a remote run identity without
// claiming that the remote start definitely succeeded. The row lock prevents
// competing uncertain responses from replacing an already frozen remote id.
func (s *ExecutionService) AttachManagedSubmissionReference(ctx context.Context, executionID uuid.UUID, engineExecutionID, traceID string) (domain.Execution, error) {
	var result domain.Execution
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		execution, err := s.repo.GetExecutionTx(ctx, tx, executionID, true)
		if err != nil {
			return err
		}
		before := executionAuditState(execution)
		if err := execution.AttachManagedSubmissionReference(engineExecutionID); err != nil {
			return err
		}
		if err := s.repo.SaveExecutionState(ctx, tx, execution, domain.ExecutionSubmitting); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_SUBMISSION_REFERENCE_ATTACHED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			BeforeState: before,
			AfterState:  executionAuditState(execution),
			TraceID:     traceID,
		}); err != nil {
			return err
		}
		result = execution
		return nil
	})
	return result, err
}

const (
	managedSubmissionStartArmedMetric     = "submissionStartArmed"
	managedSubmissionOutcomeUnknownMetric = "submissionOutcomeUnknown"
)

func (s *ExecutionService) MarkManagedSubmissionStartArmed(ctx context.Context, executionID uuid.UUID, traceID string) (domain.Execution, error) {
	return s.markManagedSubmissionMetric(ctx, executionID, managedSubmissionStartArmedMetric, "EXECUTION_SUBMISSION_START_ARMED", traceID)
}

func (s *ExecutionService) MarkManagedSubmissionOutcomeUnknown(ctx context.Context, executionID uuid.UUID, traceID string) (domain.Execution, error) {
	return s.markManagedSubmissionMetric(ctx, executionID, managedSubmissionOutcomeUnknownMetric, "EXECUTION_SUBMISSION_OUTCOME_UNKNOWN", traceID)
}

func (s *ExecutionService) markManagedSubmissionMetric(ctx context.Context, executionID uuid.UUID, key, action, traceID string) (domain.Execution, error) {
	var result domain.Execution
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		execution, err := s.repo.GetExecutionTx(ctx, tx, executionID, true)
		if err != nil {
			return err
		}
		if execution.Status != domain.ExecutionSubmitting || execution.EngineExecutionID == "" {
			return domain.ErrInvalidTransition
		}
		before := executionAuditState(execution)
		metrics := cloneMetrics(execution.Metrics)
		metrics[key] = true
		execution.Metrics = metrics
		if err := s.repo.SaveExecutionState(ctx, tx, execution, domain.ExecutionSubmitting); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      action,
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			BeforeState: before,
			AfterState:  executionAuditState(execution),
			TraceID:     traceID,
		}); err != nil {
			return err
		}
		result = execution
		return nil
	})
	return result, err
}

func ManagedSubmissionOutcomeWasUnknown(execution domain.Execution) bool {
	value, ok := execution.Metrics[managedSubmissionOutcomeUnknownMetric]
	return ok && value == true
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
