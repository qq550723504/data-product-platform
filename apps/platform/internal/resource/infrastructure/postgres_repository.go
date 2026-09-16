package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
)

type PostgresRepository struct{}

func NewPostgresRepository() *PostgresRepository {
	return &PostgresRepository{}
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
