package deliveryfence

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Lock serializes a delivery gate evaluation with dependency mutations for a
// workspace. Callers must acquire this fence before reading gate dependencies.
func Lock(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID) (int64, error) {
	if workspaceID == uuid.Nil {
		return 0, fmt.Errorf("delivery fence requires workspace")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO delivery_authorization_fence(workspace_id) VALUES ($1)
		ON CONFLICT (workspace_id) DO NOTHING
	`, workspaceID); err != nil {
		return 0, fmt.Errorf("ensure delivery authorization fence: %w", err)
	}
	var revision int64
	if err := tx.QueryRow(ctx, `
		SELECT revision FROM delivery_authorization_fence
		WHERE workspace_id=$1 FOR UPDATE
	`, workspaceID).Scan(&revision); err != nil {
		return 0, fmt.Errorf("lock delivery authorization fence: %w", err)
	}
	return revision, nil
}

// Advance serializes a dependency mutation with delivery gate evaluation and
// increments the revision before the mutation is committed.
func Advance(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID) (int64, error) {
	if workspaceID == uuid.Nil {
		return 0, fmt.Errorf("delivery fence requires workspace")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO delivery_authorization_fence(workspace_id) VALUES ($1)
		ON CONFLICT (workspace_id) DO NOTHING
	`, workspaceID); err != nil {
		return 0, fmt.Errorf("ensure delivery authorization fence: %w", err)
	}
	var revision int64
	if err := tx.QueryRow(ctx, `
		UPDATE delivery_authorization_fence
		SET revision=revision+1, updated_at=now()
		WHERE workspace_id=$1
		RETURNING revision
	`, workspaceID).Scan(&revision); err != nil {
		return 0, fmt.Errorf("advance delivery authorization fence: %w", err)
	}
	return revision, nil
}
