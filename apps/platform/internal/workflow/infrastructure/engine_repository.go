package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func (r *PostgresRepository) ListStaleNativeExecutionIDs(ctx context.Context, startedBefore time.Time, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM execution
		WHERE status = 'RUNNING'
		  AND engine_type = 'NATIVE'
		  AND COALESCE(started_at, created_at) <= $1
		  AND ($2::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR id > $2::uuid)
		ORDER BY id
		LIMIT $3
	`, startedBefore.UTC(), after, limit)
	if err != nil {
		return nil, fmt.Errorf("list stale native executions: %w", err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stale native execution id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale native executions: %w", err)
	}
	return ids, nil
}

// ListManagedExecutionIDsByEngine uses UUID keyset pagination so long-running
// rows at the front of the queue cannot starve newer remote executions.
func (r *PostgresRepository) ListManagedExecutionIDsByEngine(ctx context.Context, engineType string, after uuid.UUID, limit int) ([]uuid.UUID, error) {
	engineType = strings.ToUpper(strings.TrimSpace(engineType))
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM execution
		WHERE status IN ('SUBMITTING','RUNNING')
		  AND engine_type = $1
		  AND ($2::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR id > $2::uuid)
		ORDER BY id
		LIMIT $3
	`, engineType, after, limit)
	if err != nil {
		return nil, fmt.Errorf("list managed executions for engine %s: %w", engineType, err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan managed execution id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed executions: %w", err)
	}
	return ids, nil
}
