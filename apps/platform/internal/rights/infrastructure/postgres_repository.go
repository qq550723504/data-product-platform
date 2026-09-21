package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
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
				id, authorization_id, data_resource_id, actions, scope, scope_type, scope_ref, raw_export_allowed, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, resource.ID, resource.AuthorizationID, resource.DataResourceID, resource.Actions, scope,
			nullableString(resource.ScopeType), nullableString(resource.ScopeRef), resource.RawExportAllowed, resource.CreatedAt); err != nil {
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
		SELECT id, authorization_id, data_resource_id, actions, scope, COALESCE(scope_type,''), COALESCE(scope_ref,''), raw_export_allowed, created_at
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
			&resource.Actions, &scope, &resource.ScopeType, &resource.ScopeRef, &resource.RawExportAllowed, &resource.CreatedAt); err != nil {
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
	if _, err := deliveryfence.Advance(ctx, tx, authorization.WorkspaceID); err != nil {
		return err
	}
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
			manifest, root_hash, created_at, created_by, status
		) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,'BUILDING')
	`, snapshot.ID, snapshot.WorkspaceID, snapshot.ProductReleaseID, snapshot.Purpose,
		snapshot.ConsumerRef, snapshot.AsOf, manifest, snapshot.RootHash, snapshot.CreatedAt, snapshot.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert rights snapshot: %w", err)
	}
	for i := range snapshot.Manifest.Authorizations {
		authorization := &snapshot.Manifest.Authorizations[i]
		if _, err := tx.Exec(ctx, `
			INSERT INTO rights_snapshot_authorization (rights_snapshot_id, authorization_id)
			VALUES ($1,$2)
		`, snapshot.ID, authorization.AuthorizationID); err != nil {
			return fmt.Errorf("bind rights snapshot authorization: %w", err)
		}
		for _, resource := range authorization.Resources {
			if strings.TrimSpace(resource.ScopeType) == "" || strings.TrimSpace(resource.ScopeRef) == "" {
				return domain.ErrAuthorizationInvalid
			}
			rows, err := tx.Query(ctx, `
				SELECT b.id, b.rights_declaration_id
				FROM authorization_provenance_binding b
				JOIN data_authorization a ON a.id=b.authorization_id
				JOIN authorization_resource ar ON ar.authorization_id=a.id AND ar.data_resource_id=b.data_resource_id
				JOIN rights_declaration d ON d.id=b.rights_declaration_id
				JOIN rights_declaration_verification v ON v.declaration_id=d.id AND v.outcome='VERIFIED'
				WHERE b.authorization_id=$1 AND b.data_resource_id=$2 AND b.workspace_id=$3
				  AND a.workspace_id=$3 AND a.status='ACTIVE' AND a.grantee_ref=$5
				  AND ar.scope_type IS NOT NULL AND ar.scope_ref IS NOT NULL
				  AND (ar.scope_type<>'ALL_RESOURCE' OR ar.scope_ref=ar.data_resource_id::text)
				  AND b.created_at <= $4
				  AND (d.effective_from IS NULL OR d.effective_from <= $4)
				  AND (d.effective_to IS NULL OR d.effective_to > $4)
				  AND NOT EXISTS (SELECT 1 FROM rights_declaration_disposition x WHERE x.declaration_id=d.id AND x.effective_at <= $4)
				  AND NOT EXISTS (SELECT 1 FROM authorization_provenance_binding_disposition x WHERE x.binding_id=b.id AND x.effective_at <= $4)
				  AND (b.grantor_authority_mode='DIRECT_DECLARATION_PARTY' OR (
					b.delegation_chain_id IS NOT NULL
					AND EXISTS (SELECT 1 FROM grantor_authority_delegation_chain c WHERE c.id=b.delegation_chain_id AND c.source_declaration_id=b.rights_declaration_id AND c.status='FINALIZED' AND c.chain_hash=b.delegation_chain_hash)
					AND EXISTS (SELECT 1 FROM rights_declaration_party rp WHERE rp.declaration_id=d.id AND rp.party_ref=d.claimant_ref AND rp.role IN ('RIGHTS_HOLDER','PROVIDER','CONTROLLER'))
					AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x WHERE x.chain_id=b.delegation_chain_id AND x.effective_at <= $4)
					AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_disposition x JOIN grantor_authority_delegation_edge e ON e.id=x.edge_id WHERE e.chain_id=b.delegation_chain_id AND x.effective_at <= $4)
					AND NOT EXISTS (SELECT 1 FROM grantor_authority_delegation_edge e WHERE e.chain_id=b.delegation_chain_id AND ((e.valid_from IS NOT NULL AND e.valid_from > $4) OR (e.valid_to IS NOT NULL AND e.valid_to <= $4)))
					AND NOT EXISTS (
						SELECT 1 FROM grantor_authority_delegation_edge e
						WHERE e.chain_id=b.delegation_chain_id
						  AND (e.data_resource_id<>ar.data_resource_id OR NOT (ar.actions <@ e.grantable_actions) OR NOT (a.purpose=ANY(e.grantable_purposes)) OR NOT ((e.scope_type='ALL_RESOURCE' AND e.scope_ref=ar.data_resource_id::text) OR (e.scope_type=ar.scope_type AND e.scope_ref=ar.scope_ref)))
					)
					AND (SELECT e.delegator_ref FROM grantor_authority_delegation_edge e WHERE e.chain_id=b.delegation_chain_id ORDER BY e.ordinal LIMIT 1)=d.claimant_ref
					AND (SELECT e.delegate_ref FROM grantor_authority_delegation_edge e WHERE e.chain_id=b.delegation_chain_id ORDER BY e.ordinal DESC LIMIT 1)=b.grantor_ref
					AND NOT EXISTS (
						SELECT 1
						FROM (
							SELECT e.ordinal,e.delegator_ref,lag(e.delegate_ref) OVER (ORDER BY e.ordinal) previous_delegate,row_number() OVER (ORDER BY e.ordinal)-1 expected_ordinal
							FROM grantor_authority_delegation_edge e
							WHERE e.chain_id=b.delegation_chain_id
						) ordered_edges
						WHERE ordered_edges.ordinal<>ordered_edges.expected_ordinal
						   OR (ordered_edges.previous_delegate IS NOT NULL AND ordered_edges.previous_delegate<>ordered_edges.delegator_ref)
					)
				))
				ORDER BY b.created_at,b.id`, authorization.AuthorizationID, resource.DataResourceID, snapshot.WorkspaceID, snapshot.AsOf, snapshot.ConsumerRef)
			if err != nil {
				return fmt.Errorf("read snapshot provenance bindings: %w", err)
			}
			var bindingIDs, declarationIDs []uuid.UUID
			for rows.Next() {
				var bindingID, declarationID uuid.UUID
				if err := rows.Scan(&bindingID, &declarationID); err != nil {
					rows.Close()
					return fmt.Errorf("scan snapshot provenance binding: %w", err)
				}
				bindingIDs = append(bindingIDs, bindingID)
				declarationIDs = append(declarationIDs, declarationID)
			}
			rows.Close()
			if len(bindingIDs) != 1 {
				return domain.ErrAuthorizationInvalid
			}
			if _, err := tx.Exec(ctx, `INSERT INTO rights_snapshot_provenance_binding(rights_snapshot_id,binding_id) VALUES ($1,$2)`, snapshot.ID, bindingIDs[0]); err != nil {
				return fmt.Errorf("bind snapshot provenance: %w", err)
			}
			if _, err := tx.Exec(ctx, `INSERT INTO rights_snapshot_declaration(rights_snapshot_id,declaration_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, snapshot.ID, declarationIDs[0]); err != nil {
				return fmt.Errorf("bind snapshot declaration: %w", err)
			}
			authorization.BindingIDs = append(authorization.BindingIDs, bindingIDs[0])
			authorization.DeclarationIDs = append(authorization.DeclarationIDs, declarationIDs[0])
		}
	}
	var hashMaterial []byte
	if err := tx.QueryRow(ctx, `SELECT jsonb_build_object('manifest',manifest,'authorizationIds',COALESCE((SELECT jsonb_agg(authorization_id ORDER BY authorization_id) FROM rights_snapshot_authorization WHERE rights_snapshot_id=$1),'[]'::jsonb),'declarationIds',COALESCE((SELECT jsonb_agg(declaration_id ORDER BY declaration_id) FROM rights_snapshot_declaration WHERE rights_snapshot_id=$1),'[]'::jsonb),'bindingIds',COALESCE((SELECT jsonb_agg(binding_id ORDER BY binding_id) FROM rights_snapshot_provenance_binding WHERE rights_snapshot_id=$1),'[]'::jsonb))::text FROM rights_snapshot WHERE id=$1`, snapshot.ID).Scan(&hashMaterial); err != nil {
		return fmt.Errorf("build rights snapshot root hash: %w", err)
	}
	digest := sha256.Sum256(hashMaterial)
	if _, err := tx.Exec(ctx, `UPDATE rights_snapshot SET root_hash=$2 WHERE id=$1`, snapshot.ID, hex.EncodeToString(digest[:])); err != nil {
		return fmt.Errorf("store rights snapshot root hash: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE rights_snapshot SET status='FINALIZED' WHERE id=$1`, snapshot.ID); err != nil {
		return fmt.Errorf("finalize rights snapshot: %w", err)
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
	rows, err := r.pool.Query(ctx, `SELECT declaration_id FROM rights_snapshot_declaration WHERE rights_snapshot_id=$1 ORDER BY declaration_id`, snapshotID)
	if err != nil {
		return domain.RightsSnapshot{}, fmt.Errorf("read snapshot declarations: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return domain.RightsSnapshot{}, err
		}
		snapshot.DeclarationIDs = append(snapshot.DeclarationIDs, id)
	}
	rows.Close()
	rows, err = r.pool.Query(ctx, `SELECT binding_id FROM rights_snapshot_provenance_binding WHERE rights_snapshot_id=$1 ORDER BY binding_id`, snapshotID)
	if err != nil {
		return domain.RightsSnapshot{}, fmt.Errorf("read snapshot bindings: %w", err)
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return domain.RightsSnapshot{}, err
		}
		snapshot.BindingIDs = append(snapshot.BindingIDs, id)
	}
	rows.Close()
	return snapshot, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
