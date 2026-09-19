package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

type ExecutionQueueSource struct {
	ID               uuid.UUID
	EventType        string
	Status           string
	RoutingVersion   string
	RequiredHandlers []string
	CreatedAt        time.Time
	QueueConfirmed   bool
}

func (r *PostgresRepository) ListQueuedExecutionIDs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM execution
		WHERE status='QUEUED'
		ORDER BY created_at, id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list queued executions: %w", err)
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan queued execution: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate queued executions: %w", err)
	}
	return ids, nil
}

func (r *PostgresRepository) LatestExecutionQueueSource(ctx context.Context, executionID uuid.UUID) (ExecutionQueueSource, bool, error) {
	var source ExecutionQueueSource
	err := r.pool.QueryRow(ctx, `
		SELECT id, event_type, status, COALESCE(routing_version,''),
		       COALESCE(required_handlers, ARRAY[]::text[]), created_at,
		       EXISTS (
			       SELECT 1 FROM outbox_event_consumption c
			       WHERE c.event_id=o.id AND c.consumer_name='execution-queue'
		       )
		FROM outbox_event o
		WHERE aggregate_type='EXECUTION'
		  AND aggregate_id=$1
		  AND event_type IN ('ExecutionQueued','ExecutionRetried','ExecutionReconciliationQueued')
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, executionID).Scan(
		&source.ID, &source.EventType, &source.Status, &source.RoutingVersion,
		&source.RequiredHandlers, &source.CreatedAt, &source.QueueConfirmed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExecutionQueueSource{}, false, nil
	}
	if err != nil {
		return ExecutionQueueSource{}, false, fmt.Errorf("find execution queue source: %w", err)
	}
	return source, true, nil
}

// HasActiveExecutionQueueDispatch distinguishes the new T2 queue obligation
// from T1 retention-only events. A PUBLISHED event counts only when its
// execution-queue confirmation exists; its status alone is never evidence.
func (r *PostgresRepository) HasActiveExecutionQueueDispatch(ctx context.Context, tx pgx.Tx, executionID uuid.UUID) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM outbox_event o
			WHERE o.aggregate_type='EXECUTION'
			  AND o.aggregate_id=$1
			  AND o.event_type IN ('ExecutionQueued','ExecutionRetried','ExecutionReconciliationQueued')
			  AND 'execution-queue' = ANY(COALESCE(o.required_handlers, ARRAY[]::text[]))
			  AND NOT (
				  o.status='PUBLISHED' AND EXISTS (
					  SELECT 1 FROM outbox_event_consumption c
					  WHERE c.event_id=o.id AND c.consumer_name='execution-queue'
				  )
			  )
		)
	`, executionID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check execution queue dispatch: %w", err)
	}
	return exists, nil
}

func (r *PostgresRepository) ValidateQueuedExecution(ctx context.Context, tx pgx.Tx, execution domain.Execution) error {
	if execution.Status != domain.ExecutionQueued {
		return domain.ErrInvalidTransition
	}
	return r.ValidateExecutionReferences(ctx, tx, execution.WorkspaceID, execution.WorkflowVersionID, execution.OutputDatasetID, execution.Inputs)
}
