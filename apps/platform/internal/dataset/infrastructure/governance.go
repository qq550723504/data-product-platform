package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) SetQualityStatus(ctx context.Context, tx pgx.Tx, versionID uuid.UUID, status string) error {
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
	commandTag, err := tx.Exec(ctx, `UPDATE dataset_version SET compliance_status=$2 WHERE id=$1`, versionID, status)
	if err != nil {
		return fmt.Errorf("set DatasetVersion compliance status: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
