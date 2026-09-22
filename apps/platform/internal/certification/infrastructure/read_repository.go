package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (r *CertificationRepository) ListProfileIDsForDatasetVersion(ctx context.Context, workspaceID, datasetVersionID uuid.UUID, asOf time.Time) ([]uuid.UUID, error) {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT certification_profile_id
		FROM dataset_certification
		WHERE workspace_id=$1 AND dataset_version_id=$2 AND issued_at <= $3
		ORDER BY certification_profile_id
	`, workspaceID, datasetVersionID, asOf.UTC())
	if err != nil {
		return nil, fmt.Errorf("list certification profiles for DatasetVersion: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan certification profile id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate certification profile ids: %w", err)
	}
	return ids, nil
}
