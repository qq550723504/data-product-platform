package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// FindSucceededOutputVersionForInput returns the most recent successful entity-resolution
// output produced from the supplied input DatasetVersion/source identity. Workflows that
// consume canonical EntityMappings depend on this historical output even when they still
// read the original RAW file for business fields, so the output must be included in
// downstream DatasetVersion lineage.
func (r *PostgresRepository) FindSucceededOutputVersionForInput(ctx context.Context, inputVersionID uuid.UUID, sourceType, sourceRef string) (uuid.UUID, bool, error) {
	var outputVersionID uuid.UUID
	err := r.pool.QueryRow(ctx, `
		SELECT output_dataset_version_id
		FROM entity_match_job
		WHERE input_dataset_version_id=$1
		  AND source_type=$2
		  AND source_ref=$3
		  AND status='SUCCEEDED'
		  AND output_dataset_version_id IS NOT NULL
		ORDER BY finished_at DESC NULLS LAST, created_at DESC, id DESC
		LIMIT 1
	`, inputVersionID, sourceType, sourceRef).Scan(&outputVersionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("find successful entity-resolution output: %w", err)
	}
	return outputVersionID, true, nil
}
