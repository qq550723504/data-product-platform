package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (r *PostgresRepository) DatasetVersionBelongsToWorkspace(ctx context.Context, workspaceID, datasetVersionID uuid.UUID) (bool, error) {
	var matches bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM dataset_version v
			JOIN dataset d ON d.id=v.dataset_id
			WHERE v.id=$1 AND d.workspace_id=$2 AND d.deleted_at IS NULL
		)
	`, datasetVersionID, workspaceID).Scan(&matches); err != nil {
		return false, fmt.Errorf("check quality DatasetVersion workspace: %w", err)
	}
	return matches, nil
}
