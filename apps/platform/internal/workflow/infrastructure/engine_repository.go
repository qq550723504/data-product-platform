package infrastructure

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) UpdateExecutionEngine(ctx context.Context, tx pgx.Tx, executionID uuid.UUID, engineType string) error {
	engineType = strings.ToUpper(strings.TrimSpace(engineType))
	commandTag, err := tx.Exec(ctx, `
		UPDATE execution
		SET engine_type = $2
		WHERE id = $1 AND status = 'QUEUED'
	`, executionID, engineType)
	if err != nil {
		return fmt.Errorf("update execution engine: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return fmt.Errorf("execution must exist and be QUEUED to select engine")
	}
	return nil
}

func (r *PostgresRepository) ListRunningExecutionIDsByEngine(ctx context.Context, engineType string, limit int) ([]uuid.UUID, error) {
	engineType = strings.ToUpper(strings.TrimSpace(engineType))
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM execution
		WHERE status = 'RUNNING' AND engine_type = $1
		ORDER BY started_at NULLS FIRST, created_at
		LIMIT $2
	`, engineType, limit)
	if err != nil {
		return nil, fmt.Errorf("list running executions for engine %s: %w", engineType, err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan running execution id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate running executions: %w", err)
	}
	return ids, nil
}
