package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

var ErrNotFound = errors.New("rights object not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertAuthorization(ctx context.Context, tx pgx.Tx, authorization domain.Authorization) error {
	metadata, err := json.Marshal(authorization.Metadata)
	if err != nil {
		return fmt.Errorf("marshal authorization metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO data_authorization (
			id, workspace_id, code, grantor_ref, grantee_ref, purpose, status,
			valid_from, valid_to, metadata, created_at, created_by, updated_at, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, authorization.ID, authorization.WorkspaceID, authorization.Code, authorization.GrantorRef,
		authorization.GranteeRef, authorization.Purpose, authorization.Status, authorization.ValidFrom,
		authorization.ValidTo, metadata, authorization.CreatedAt, authorization.CreatedBy,
		authorization.UpdatedAt, authorization.UpdatedBy)
	if err != nil {
		return fmt.Errorf("insert authorization: %w", err)
	}
	for _, resource := range authorization.Resources {
		// The authorization and every resource it grants must share one workspace so a
		// grant declared in one workspace cannot name another tenant's resource.
		var resourceWorkspace uuid.UUID
		err := tx.QueryRow(ctx, `SELECT workspace_id FROM data_resource WHERE id=$1`, resource.DataResourceID).Scan(&resourceWorkspace)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("authorization resource %s: %w", resource.DataResourceID, ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("read authorization resource workspace: %w", err)
		}
		if resourceWorkspace != authorization.WorkspaceID {
			return fmt.Errorf("authorization resource %s: %w", resource.DataResourceID, domain.ErrResourceWorkspace)
		}
		scope, err := json.Marshal(resource.Scope)
		if err != nil {
			return fmt.Errorf("marshal authorization resource scope: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO authorization_resource (
				id, authorization_id, data_resource_id, actions, scope, raw_export_allowed, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, resource.ID, resource.AuthorizationID, resource.DataResourceID, resource.Actions, scope,
			resource.RawExportAllowed, resource.CreatedAt); err != nil {
			return fmt.Errorf("insert authorization resource: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetAuthorization(ctx context.Context, authorizationID uuid.UUID) (domain.Authorization, error) {
	var authorization domain.Authorization
	var metadata []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, code, grantor_ref, grantee_ref, purpose, status,
		       valid_from, valid_to, metadata, created_at, created_by, updated_at, updated_by
		FROM data_authorization WHERE id=$1
	`, authorizationID).Scan(
		&authorization.ID, &authorization.WorkspaceID, &authorization.Code, &authorization.GrantorRef,
		&authorization.GranteeRef, &authorization.Purpose, &authorization.Status,
		&authorization.ValidFrom, &authorization.ValidTo, &metadata, &authorization.CreatedAt,
		&authorization.CreatedBy, &authorization.UpdatedAt, &authorization.UpdatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Authorization{}, ErrNotFound
	}
	if err != nil {
		return domain.Authorization{}, fmt.Errorf("get authorization: %w", err)
	}
	if err := json.Unmarshal(metadata, &authorization.Metadata); err != nil {
		return domain.Authorization{}, fmt.Errorf("decode authorization metadata: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, authorization_id, data_resource_id, actions, scope, raw_export_allowed, created_at
		FROM authorization_resource WHERE authorization_id=$1 ORDER BY created_at, id
	`, authorizationID)
	if err != nil {
		return domain.Authorization{}, fmt.Errorf("list authorization resources: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var resource domain.ResourceGrant
		var scope []byte
		if err := rows.Scan(&resource.ID, &resource.AuthorizationID, &resource.DataResourceID,
			&resource.Actions, &scope, &resource.RawExportAllowed, &resource.CreatedAt); err != nil {
			return domain.Authorization{}, fmt.Errorf("scan authorization resource: %w", err)
		}
		if err := json.Unmarshal(scope, &resource.Scope); err != nil {
			return domain.Authorization{}, fmt.Errorf("decode authorization resource scope: %w", err)
		}
		authorization.Resources = append(authorization.Resources, resource)
	}
	if err := rows.Err(); err != nil {
		return domain.Authorization{}, fmt.Errorf("iterate authorization resources: %w", err)
	}
	return authorization, nil
}

func (r *PostgresRepository) SaveAuthorizationState(ctx context.Context, tx pgx.Tx, authorization domain.Authorization) error {
	commandTag, err := tx.Exec(ctx, `
		UPDATE data_authorization
		SET status=$2, updated_at=$3, updated_by=$4
		WHERE id=$1
	`, authorization.ID, authorization.Status, authorization.UpdatedAt, authorization.UpdatedBy)
	if err != nil {
		return fmt.Errorf("save authorization state: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) InsertSnapshot(ctx context.Context, tx pgx.Tx, snapshot domain.RightsSnapshot) error {
	manifest, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return fmt.Errorf("marshal rights snapshot manifest: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO rights_snapshot (
			id, workspace_id, product_release_id, purpose, consumer_ref, as_of,
			manifest, root_hash, created_at, created_by
		) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10)
	`, snapshot.ID, snapshot.WorkspaceID, snapshot.ProductReleaseID, snapshot.Purpose,
		snapshot.ConsumerRef, snapshot.AsOf, manifest, snapshot.RootHash, snapshot.CreatedAt, snapshot.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert rights snapshot: %w", err)
	}
	for _, authorization := range snapshot.Manifest.Authorizations {
		if _, err := tx.Exec(ctx, `
			INSERT INTO rights_snapshot_authorization (rights_snapshot_id, authorization_id)
			VALUES ($1,$2)
		`, snapshot.ID, authorization.AuthorizationID); err != nil {
			return fmt.Errorf("bind rights snapshot authorization: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetSnapshot(ctx context.Context, snapshotID uuid.UUID) (domain.RightsSnapshot, error) {
	var snapshot domain.RightsSnapshot
	var manifest []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, product_release_id, purpose, COALESCE(consumer_ref,''), as_of,
		       manifest, root_hash, created_at, created_by
		FROM rights_snapshot WHERE id=$1
	`, snapshotID).Scan(&snapshot.ID, &snapshot.WorkspaceID, &snapshot.ProductReleaseID, &snapshot.Purpose,
		&snapshot.ConsumerRef, &snapshot.AsOf, &manifest, &snapshot.RootHash, &snapshot.CreatedAt, &snapshot.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RightsSnapshot{}, ErrNotFound
	}
	if err != nil {
		return domain.RightsSnapshot{}, fmt.Errorf("get rights snapshot: %w", err)
	}
	if err := json.Unmarshal(manifest, &snapshot.Manifest); err != nil {
		return domain.RightsSnapshot{}, fmt.Errorf("decode rights snapshot manifest: %w", err)
	}
	return snapshot, nil
}
