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
