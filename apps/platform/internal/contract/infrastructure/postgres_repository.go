package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
)

var ErrNotFound = errors.New("data contract object not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) EnsureContract(ctx context.Context, tx pgx.Tx, contract domain.DataContract) (domain.DataContract, error) {
	var stored domain.DataContract
	err := tx.QueryRow(ctx, `
		INSERT INTO data_contract (id, workspace_id, code, name, product_code, created_at, created_by)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7)
		ON CONFLICT (workspace_id, code) DO UPDATE SET
			name=EXCLUDED.name,
			product_code=EXCLUDED.product_code
		RETURNING id, workspace_id, code, name, COALESCE(product_code,''), created_at, created_by
	`, contract.ID, contract.WorkspaceID, contract.Code, contract.Name, contract.ProductCode,
		contract.CreatedAt, contract.CreatedBy).Scan(
		&stored.ID, &stored.WorkspaceID, &stored.Code, &stored.Name, &stored.ProductCode,
		&stored.CreatedAt, &stored.CreatedBy,
	)
	if err != nil {
		return domain.DataContract{}, fmt.Errorf("ensure data contract: %w", err)
	}
	return stored, nil
}

func (r *PostgresRepository) GetContract(ctx context.Context, contractID uuid.UUID) (domain.DataContract, error) {
	var contract domain.DataContract
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, code, name, COALESCE(product_code,''), created_at, created_by
		FROM data_contract WHERE id=$1
	`, contractID).Scan(&contract.ID, &contract.WorkspaceID, &contract.Code, &contract.Name,
		&contract.ProductCode, &contract.CreatedAt, &contract.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DataContract{}, ErrNotFound
	}
	if err != nil {
		return domain.DataContract{}, fmt.Errorf("get data contract: %w", err)
	}
	return contract, nil
}

func (r *PostgresRepository) InsertVersion(ctx context.Context, tx pgx.Tx, version domain.ContractVersion) error {
	document, err := json.Marshal(version.Document)
	if err != nil {
		return fmt.Errorf("marshal contract document: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO contract_version (
			id, contract_id, major_version, minor_version, patch_version, status,
			document, source_ref, source_sha256, created_at, created_by, published_at, published_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13)
	`, version.ID, version.ContractID, version.MajorVersion, version.MinorVersion, version.PatchVersion,
		version.Status, document, version.SourceRef, version.SourceSHA256, version.CreatedAt,
		version.CreatedBy, version.PublishedAt, version.PublishedBy)
	if err != nil {
		return fmt.Errorf("insert contract version: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetVersion(ctx context.Context, versionID uuid.UUID) (domain.ContractVersion, error) {
	var version domain.ContractVersion
	var document []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, contract_id, major_version, minor_version, patch_version, status,
		       document, COALESCE(source_ref,''), source_sha256, created_at, created_by,
		       published_at, published_by
		FROM contract_version WHERE id=$1
	`, versionID).Scan(&version.ID, &version.ContractID, &version.MajorVersion, &version.MinorVersion,
		&version.PatchVersion, &version.Status, &document, &version.SourceRef, &version.SourceSHA256,
		&version.CreatedAt, &version.CreatedBy, &version.PublishedAt, &version.PublishedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ContractVersion{}, ErrNotFound
	}
	if err != nil {
		return domain.ContractVersion{}, fmt.Errorf("get contract version: %w", err)
	}
	if err := json.Unmarshal(document, &version.Document); err != nil {
		return domain.ContractVersion{}, fmt.Errorf("decode contract document: %w", err)
	}
	return version, nil
}

func (r *PostgresRepository) SaveVersionState(ctx context.Context, tx pgx.Tx, version domain.ContractVersion) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE contract_version
		SET status=$2, published_at=$3, published_by=$4
		WHERE id=$1
	`, version.ID, version.Status, version.PublishedAt, version.PublishedBy)
	if err != nil {
		return fmt.Errorf("save contract version state: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}
