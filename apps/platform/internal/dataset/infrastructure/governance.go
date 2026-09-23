package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) workspaceForVersion(ctx context.Context, tx pgx.Tx, versionID uuid.UUID) (uuid.UUID, error) {
	var workspaceID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT d.workspace_id
		FROM dataset_version v
		JOIN dataset d ON d.id=v.dataset_id
		WHERE v.id=$1
	`, versionID).Scan(&workspaceID); err != nil {
		return uuid.Nil, fmt.Errorf("resolve DatasetVersion workspace: %w", err)
	}
	return workspaceID, nil
}

func (r *PostgresRepository) workspaceForDataset(ctx context.Context, tx pgx.Tx, datasetID uuid.UUID) (uuid.UUID, error) {
	var workspaceID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM dataset WHERE id=$1`, datasetID).Scan(&workspaceID); err != nil {
		return uuid.Nil, fmt.Errorf("resolve dataset workspace: %w", err)
	}
	return workspaceID, nil
}
