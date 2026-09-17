package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
)

// ErrNotFound reports a missing DataResource.
var ErrNotFound = errors.New("data resource not found")

type PostgresRepository struct{}

func NewPostgresRepository() *PostgresRepository {
	return &PostgresRepository{}
}

// WorkspaceOf returns the workspace that owns a DataResource. Callers use it to
// enforce tenant boundaries inside their own transaction, because a foreign key
// on data_resource(id) proves the resource exists but not that it belongs to
// the caller's workspace.
func (r *PostgresRepository) WorkspaceOf(ctx context.Context, tx pgx.Tx, resourceID uuid.UUID) (uuid.UUID, error) {
	var workspaceID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT workspace_id FROM data_resource
		WHERE id=$1 AND deleted_at IS NULL
	`, resourceID).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("read data resource workspace: %w", err)
	}
	return workspaceID, nil
}

func (r *PostgresRepository) Insert(ctx context.Context, tx pgx.Tx, resource domain.DataResource) error {
	businessMetadata, err := json.Marshal(resource.BusinessMetadata)
	if err != nil {
		return fmt.Errorf("marshal business metadata: %w", err)
	}
	extension, err := json.Marshal(resource.Extension)
	if err != nil {
		return fmt.Errorf("marshal extension: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO data_resource (
			id, workspace_id, project_id, code, name, description, domain_code,
			resource_type, owner_id, sensitivity_level, rights_status, quality_status,
			lifecycle_status, business_metadata, extension, revision, created_at, created_by,
			updated_at, updated_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18,
			$17, $18
		)
	`,
		resource.ID,
		resource.WorkspaceID,
		resource.ProjectID,
		resource.Code,
		resource.Name,
		resource.Description,
		resource.DomainCode,
		resource.ResourceType,
		resource.OwnerID,
		resource.SensitivityLevel,
		resource.RightsStatus,
		resource.QualityStatus,
		resource.LifecycleStatus,
		businessMetadata,
		extension,
		resource.Revision,
		resource.CreatedAt,
		resource.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("insert data resource: %w", err)
	}
	return nil
}
