package readmodel

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

func (r *Repository) DataProductBelongsToWorkspace(ctx context.Context, productID, workspaceID uuid.UUID) (bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM data_product
			WHERE id=$1 AND workspace_id=$2 AND deleted_at IS NULL
		)
	`, productID, workspaceID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check data product workspace: %w", err)
	}
	return exists, nil
}
