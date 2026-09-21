package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func insertRightsProvenanceFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workspaceID, authorizationID, snapshotID uuid.UUID) {
	t.Helper()
	resourceID := uuid.New()
	declarationID := uuid.New()
	permissionID := uuid.New()
	bindingID := uuid.New()
	mustExec(t, ctx, pool, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type)
		VALUES ($1,$2,$3,$3,'TABLE_LIKE')
	`, resourceID, workspaceID, "RIGHTS-FIXTURE-"+uuid.NewString())
	mustExec(t, ctx, pool, `
		INSERT INTO authorization_resource (
			id, authorization_id, data_resource_id, actions, scope, scope_type, scope_ref, raw_export_allowed, created_at
		) VALUES ($1,$2,$3,ARRAY['USE']::text[],'{}'::jsonb,'ALL_RESOURCE',$4,false,now())
	`, uuid.New(), authorizationID, resourceID, resourceID.String())
	mustExec(t, ctx, pool, `
		INSERT INTO rights_declaration (
			id, workspace_id, data_resource_id, claimant_ref, basis_type, basis_ref,
			consumer_scope_type, restrictions, created_at
		) VALUES ($1,$2,$3,'PARK-OPERATOR','LICENSE','application-fixture','ANY','{}'::jsonb,now())
	`, declarationID, workspaceID, resourceID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_declaration_party (id, declaration_id, party_ref, role)
		VALUES ($1,$2,'PARK-OPERATOR','RIGHTS_HOLDER')
	`, uuid.New(), declarationID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_declaration_permission (id, declaration_id, permission_kind, action)
		VALUES ($1,$2,'GRANT','USE')
	`, permissionID, declarationID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_declaration_purpose (id, declaration_id, permission_id, permission_kind, purpose_code)
		VALUES ($1,$2,$3,'GRANT','ENTERPRISE_CREDIT_RISK_SUPPORT')
	`, uuid.New(), declarationID, permissionID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_declaration_scope (id, declaration_id, permission_id, permission_kind, scope_type, scope_ref)
		VALUES ($1,$2,$3,'GRANT','ALL_RESOURCE',$4)
	`, uuid.New(), declarationID, permissionID, resourceID.String())
	mustExec(t, ctx, pool, `
		INSERT INTO rights_declaration_verification (id, declaration_id, outcome, occurred_at)
		VALUES ($1,$2,'VERIFIED',now())
	`, uuid.New(), declarationID)
	mustExec(t, ctx, pool, `
		INSERT INTO authorization_provenance_binding (
			id, workspace_id, authorization_id, data_resource_id, rights_declaration_id,
			grantor_ref, grantor_authority_mode, created_at
		) VALUES ($1,$2,$3,$4,$5,'PARK-OPERATOR','DIRECT_DECLARATION_PARTY',now())
	`, bindingID, workspaceID, authorizationID, resourceID, declarationID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_snapshot_authorization (rights_snapshot_id, authorization_id)
		VALUES ($1,$2)
	`, snapshotID, authorizationID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_snapshot_provenance_binding (rights_snapshot_id, binding_id)
		VALUES ($1,$2)
	`, snapshotID, bindingID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_snapshot_declaration (rights_snapshot_id, declaration_id)
		VALUES ($1,$2)
	`, snapshotID, declarationID)
}
