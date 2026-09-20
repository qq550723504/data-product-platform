package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
)

func (r *PostgresRepository) SetQualityStatus(ctx context.Context, tx pgx.Tx, versionID uuid.UUID, status string) error {
	workspaceID, err := r.workspaceForVersion(ctx, tx, versionID)
	if err != nil {
		return err
	}
	if _, err := deliveryfence.Advance(ctx, tx, workspaceID); err != nil {
		return err
	}
	commandTag, err := tx.Exec(ctx, `UPDATE dataset_version SET quality_status=$2 WHERE id=$1`, versionID, status)
	if err != nil {
		return fmt.Errorf("set DatasetVersion quality status: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) SetComplianceStatus(ctx context.Context, tx pgx.Tx, versionID uuid.UUID, status string) error {
	workspaceID, err := r.workspaceForVersion(ctx, tx, versionID)
	if err != nil {
		return err
	}
	if _, err := deliveryfence.Advance(ctx, tx, workspaceID); err != nil {
		return err
	}
	commandTag, err := tx.Exec(ctx, `UPDATE dataset_version SET compliance_status=$2 WHERE id=$1`, versionID, status)
	if err != nil {
		return fmt.Errorf("set DatasetVersion compliance status: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

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
