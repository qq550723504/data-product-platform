package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *PostgresRepository) AddLineage(ctx context.Context, tx pgx.Tx, outputVersionID, inputVersionID uuid.UUID, relationType string, executionID *uuid.UUID) error {
	if outputVersionID == uuid.Nil || inputVersionID == uuid.Nil || relationType == "" {
		return fmt.Errorf("output/input DatasetVersion and relation type are required")
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO dataset_version_lineage (
			output_version_id, input_version_id, relation_type, execution_id
		) VALUES ($1,$2,$3,$4)
		ON CONFLICT (output_version_id, input_version_id, relation_type) DO NOTHING
	`, outputVersionID, inputVersionID, relationType, executionID)
	if err != nil {
		return fmt.Errorf("insert dataset version lineage: %w", err)
	}
	return nil
}
