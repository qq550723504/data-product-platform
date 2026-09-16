package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
)

var ErrNotFound = errors.New("product object not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertProduct(ctx context.Context, tx pgx.Tx, product domain.DataProduct) error {
	metadata, err := json.Marshal(product.Metadata)
	if err != nil {
		return fmt.Errorf("marshal product metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO data_product (
			id, workspace_id, project_id, use_case_id, code, name, description, domain_code,
			owner_id, lifecycle_status, health_status, metadata, created_at, created_by, updated_at, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$13,$14)
	`, product.ID, product.WorkspaceID, product.ProjectID, product.UseCaseID, product.Code, product.Name,
		product.Description, nullableString(product.DomainCode), product.OwnerID, product.LifecycleStatus,
		product.HealthStatus, metadata, product.CreatedAt, product.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert data product: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetProduct(ctx context.Context, productID uuid.UUID) (domain.DataProduct, error) {
	var product domain.DataProduct
	var metadata []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, project_id, use_case_id, code, name, COALESCE(description,''),
		       COALESCE(domain_code,''), owner_id, lifecycle_status, health_status,
		       current_version_id, latest_release_id, metadata, created_at, created_by
		FROM data_product
		WHERE id=$1 AND deleted_at IS NULL
	`, productID).Scan(
		&product.ID, &product.WorkspaceID, &product.ProjectID, &product.UseCaseID, &product.Code,
		&product.Name, &product.Description, &product.DomainCode, &product.OwnerID,
		&product.LifecycleStatus, &product.HealthStatus, &product.CurrentVersionID,
		&product.LatestReleaseID, &metadata, &product.CreatedAt, &product.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DataProduct{}, ErrNotFound
	}
	if err != nil {
		return domain.DataProduct{}, fmt.Errorf("get data product: %w", err)
	}
	if err := json.Unmarshal(metadata, &product.Metadata); err != nil {
		return domain.DataProduct{}, fmt.Errorf("decode product metadata: %w", err)
	}
	return product, nil
}

func (r *PostgresRepository) InsertVersion(ctx context.Context, tx pgx.Tx, version domain.ProductVersion) error {
	definition, err := json.Marshal(version.DefinitionSnapshot)
	if err != nil {
		return fmt.Errorf("marshal product version definition: %w", err)
	}
	var productExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM data_product WHERE id=$1 AND deleted_at IS NULL)`, version.ProductID).Scan(&productExists); err != nil {
		return fmt.Errorf("validate product: %w", err)
	}
	if !productExists {
		return ErrNotFound
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO product_version (
			id, product_id, major_version, minor_version, patch_version, workflow_version_id,
			contract_version_id, entity_policy_ref, indicator_set_ref, definition_snapshot,
			created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, version.ID, version.ProductID, version.MajorVersion, version.MinorVersion, version.PatchVersion,
		version.WorkflowVersionID, version.ContractVersionID, nullableString(version.EntityPolicyRef),
		nullableString(version.IndicatorSetRef), definition, version.CreatedAt, version.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert product version: %w", err)
	}
	for _, asset := range version.Assets {
		if err := r.insertAsset(ctx, tx, asset); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE data_product
		SET current_version_id=$2, updated_at=now(), updated_by=$3
		WHERE id=$1
	`, version.ProductID, version.ID, version.CreatedBy); err != nil {
		return fmt.Errorf("set current product version: %w", err)
	}
	return nil
}

func (r *PostgresRepository) insertAsset(ctx context.Context, tx pgx.Tx, asset domain.ProductAsset) error {
	delivery, err := json.Marshal(asset.DeliveryConfig)
	if err != nil {
		return fmt.Errorf("marshal product asset delivery config: %w", err)
	}
	schema, err := json.Marshal(asset.SchemaSnapshot)
	if err != nil {
		return fmt.Errorf("marshal product asset schema: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO product_asset (
			id, product_version_id, asset_type, name, dataset_id, external_ref,
			delivery_config, schema_snapshot, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, asset.ID, asset.ProductVersionID, asset.AssetType, asset.Name, asset.DatasetID,
		nullableString(asset.ExternalRef), delivery, schema, asset.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert product asset: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetVersion(ctx context.Context, versionID uuid.UUID) (domain.ProductVersion, error) {
	var version domain.ProductVersion
	var definition []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, major_version, minor_version, patch_version, workflow_version_id,
		       contract_version_id, COALESCE(entity_policy_ref,''), COALESCE(indicator_set_ref,''),
		       definition_snapshot, created_at, created_by
		FROM product_version WHERE id=$1
	`, versionID).Scan(
		&version.ID, &version.ProductID, &version.MajorVersion, &version.MinorVersion, &version.PatchVersion,
		&version.WorkflowVersionID, &version.ContractVersionID, &version.EntityPolicyRef,
		&version.IndicatorSetRef, &definition, &version.CreatedAt, &version.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProductVersion{}, ErrNotFound
	}
	if err != nil {
		return domain.ProductVersion{}, fmt.Errorf("get product version: %w", err)
	}
	if err := json.Unmarshal(definition, &version.DefinitionSnapshot); err != nil {
		return domain.ProductVersion{}, fmt.Errorf("decode product version definition: %w", err)
	}
	assets, err := r.listAssets(ctx, version.ID)
	if err != nil {
		return domain.ProductVersion{}, err
	}
	version.Assets = assets
	return version, nil
}

func (r *PostgresRepository) listAssets(ctx context.Context, versionID uuid.UUID) ([]domain.ProductAsset, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, product_version_id, asset_type, name, dataset_id, COALESCE(external_ref,''),
		       delivery_config, schema_snapshot, created_at
		FROM product_asset WHERE product_version_id=$1 ORDER BY created_at, id
	`, versionID)
	if err != nil {
		return nil, fmt.Errorf("list product assets: %w", err)
	}
	defer rows.Close()
	assets := make([]domain.ProductAsset, 0)
	for rows.Next() {
		var asset domain.ProductAsset
		var delivery, schema []byte
		if err := rows.Scan(&asset.ID, &asset.ProductVersionID, &asset.AssetType, &asset.Name,
			&asset.DatasetID, &asset.ExternalRef, &delivery, &schema, &asset.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan product asset: %w", err)
		}
		if err := json.Unmarshal(delivery, &asset.DeliveryConfig); err != nil {
			return nil, fmt.Errorf("decode delivery config: %w", err)
		}
		if err := json.Unmarshal(schema, &asset.SchemaSnapshot); err != nil {
			return nil, fmt.Errorf("decode schema snapshot: %w", err)
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func (r *PostgresRepository) ValidateReleaseReferences(ctx context.Context, tx pgx.Tx, release domain.ProductRelease) error {
	var versionProductID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT product_id FROM product_version WHERE id=$1`, release.ProductVersionID).Scan(&versionProductID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("product version: %w", ErrNotFound)
		}
		return fmt.Errorf("validate product version: %w", err)
	}
	if versionProductID != release.ProductID {
		return fmt.Errorf("product version belongs to product %s, expected %s", versionProductID, release.ProductID)
	}
	for _, binding := range release.Datasets {
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM dataset_version WHERE id=$1`, binding.DatasetVersionID).Scan(&status); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("dataset version %s: %w", binding.DatasetVersionID, ErrNotFound)
			}
			return fmt.Errorf("validate dataset version %s: %w", binding.DatasetVersionID, err)
		}
		if status != "READY" && status != "SUPERSEDED" {
			return fmt.Errorf("dataset version %s must be immutable and usable (READY or SUPERSEDED), got %s", binding.DatasetVersionID, status)
		}
	}
	return nil
}

func (r *PostgresRepository) InsertRelease(ctx context.Context, tx pgx.Tx, release domain.ProductRelease) error {
	metadata, err := json.Marshal(release.Metadata)
	if err != nil {
		return fmt.Errorf("marshal release metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO product_release (
			id, product_id, product_version_id, release_no, status, contract_version_id,
			rights_snapshot_id, quality_result_id, compliance_result_id, evidence_snapshot_id,
			release_notes, metadata, created_at, created_by, released_at, released_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, release.ID, release.ProductID, release.ProductVersionID, release.ReleaseNo, release.Status,
		release.ContractVersionID, release.RightsSnapshotID, release.QualityResultID,
		release.ComplianceResultID, release.EvidenceSnapshotID, release.ReleaseNotes, metadata,
		release.CreatedAt, release.CreatedBy, release.ReleasedAt, release.ReleasedBy)
	if err != nil {
		return fmt.Errorf("insert product release: %w", err)
	}
	for _, binding := range release.Datasets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
			VALUES ($1,$2,$3)
		`, release.ID, binding.DatasetVersionID, binding.Role); err != nil {
			return fmt.Errorf("insert release dataset binding: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetRelease(ctx context.Context, releaseID uuid.UUID) (domain.ProductRelease, error) {
	var release domain.ProductRelease
	var metadata []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, product_id, product_version_id, release_no, status, contract_version_id,
		       rights_snapshot_id, quality_result_id, compliance_result_id, evidence_snapshot_id,
		       COALESCE(release_notes,''), metadata, created_at, created_by, released_at, released_by
		FROM product_release WHERE id=$1
	`, releaseID).Scan(
		&release.ID, &release.ProductID, &release.ProductVersionID, &release.ReleaseNo, &release.Status,
		&release.ContractVersionID, &release.RightsSnapshotID, &release.QualityResultID,
		&release.ComplianceResultID, &release.EvidenceSnapshotID, &release.ReleaseNotes, &metadata,
		&release.CreatedAt, &release.CreatedBy, &release.ReleasedAt, &release.ReleasedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProductRelease{}, ErrNotFound
	}
	if err != nil {
		return domain.ProductRelease{}, fmt.Errorf("get product release: %w", err)
	}
	if err := json.Unmarshal(metadata, &release.Metadata); err != nil {
		return domain.ProductRelease{}, fmt.Errorf("decode release metadata: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT dataset_version_id, role FROM product_release_dataset
		WHERE release_id=$1 ORDER BY role, dataset_version_id
	`, release.ID)
	if err != nil {
		return domain.ProductRelease{}, fmt.Errorf("list release datasets: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var binding domain.ReleaseDataset
		if err := rows.Scan(&binding.DatasetVersionID, &binding.Role); err != nil {
			return domain.ProductRelease{}, fmt.Errorf("scan release dataset: %w", err)
		}
		release.Datasets = append(release.Datasets, binding)
	}
	return release, rows.Err()
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
