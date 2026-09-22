package migration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type certificationFixture struct {
	workspaceID    uuid.UUID
	datasetID      uuid.UUID
	versionID      uuid.UUID
	qualityID      uuid.UUID
	profileID      uuid.UUID
	profileRef     string
	profileHash    string
	profileContent string
}

func insertCertificationFixture(t *testing.T, pool *pgxpool.Pool, workspaceID uuid.UUID, datasetCode string, decision string) certificationFixture {
	t.Helper()
	ctx := context.Background()
	fixture := certificationFixture{
		workspaceID:    workspaceID,
		datasetID:      uuid.New(),
		versionID:      uuid.New(),
		qualityID:      uuid.New(),
		profileID:      uuid.New(),
		profileRef:     "migration/profile/" + uuid.NewString(),
		profileContent: "{}",
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Certification migration fixture','CURATED')
	`, fixture.datasetID, workspaceID, datasetCode); err != nil {
		t.Fatalf("insert certification dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://certification-fixture',
			'text/csv','SHA256',repeat('b',64),'{}'::jsonb,now())
	`, fixture.versionID, fixture.datasetID); err != nil {
		t.Fatalf("insert certification dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'certification/migration.yaml','1.0.0',
			encode(digest(convert_to('certification-fixture','UTF8'),'sha256'),'hex'),
			'certification-fixture','native-quality','1','PASS','{}'::jsonb)
	`, fixture.qualityID, workspaceID, fixture.versionID); err != nil {
		t.Fatalf("insert certification quality result: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO certification_profile (
			id, workspace_id, profile_ref, code, name, version,
			content_sha256, content_snapshot, purpose_mode, action_mode,
			consumer_mode, delivery_mode, quality_gate_required,
			rights_required, compliance_required, contract_required,
			traceability_required, evidence_required
		) VALUES ($1,$2,$3,'migration-profile','Migration Profile','1',
			encode(digest(convert_to('{}','UTF8'),'sha256'),'hex'),'{}','ANY','ANY',
			'ANY','ANY',false,false,false,false,false,false)
		RETURNING content_sha256
	`, fixture.profileID, workspaceID, fixture.profileRef).Scan(&fixture.profileHash); err != nil {
		t.Fatalf("insert certification profile: %v", err)
	}
	certificationID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_certification (
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot, decision,
			blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,$6,'1',$7,$8,$9,'[]'::jsonb,
			'fixture certification',now())
	`, certificationID, workspaceID, fixture.versionID, fixture.qualityID, fixture.profileID,
		fixture.profileRef, fixture.profileHash, fixture.profileContent, decision); err != nil {
		t.Fatalf("insert dataset certification: %v", err)
	}
	return fixture
}

func TestDatasetCertificationMigrationGuardsHistoricalFacts(t *testing.T) {
	pool := scratchDatabase(t, 27)
	ctx := context.Background()
	workspaceID := uuid.New()
	first := insertCertificationFixture(t, pool, workspaceID, "CERT-GUARD-1-"+uuid.NewString(), "CERTIFIED")
	second := insertCertificationFixture(t, pool, workspaceID, "CERT-GUARD-2-"+uuid.NewString(), "CERTIFIED")

	var certificationID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT id FROM dataset_certification WHERE dataset_version_id=$1
	`, first.versionID).Scan(&certificationID); err != nil {
		t.Fatalf("load certification id: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dataset_certification SET reason='mutated' WHERE id=$1`, certificationID); err == nil || !strings.Contains(err.Error(), "immutable historical fact") {
		t.Fatalf("certification update error = %v, want immutable guard", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM dataset_certification WHERE id=$1`, certificationID); err == nil || !strings.Contains(err.Error(), "immutable historical fact") {
		t.Fatalf("certification delete error = %v, want immutable guard", err)
	}

	dispositionID := uuid.New()
	var replacementCertificationID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM dataset_certification WHERE dataset_version_id=$1`, second.versionID).Scan(&replacementCertificationID); err != nil {
		t.Fatalf("load replacement certification id: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO certification_disposition (
			id, workspace_id, certification_id, disposition, effective_at, reason
		) SELECT $1, workspace_id, id, 'REVOKED', issued_at, 'fixture revoke'
		FROM dataset_certification WHERE id=$2
	`, dispositionID, certificationID); err != nil {
		t.Fatalf("insert certification disposition: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE certification_disposition SET reason='mutated' WHERE id=$1`, dispositionID); err == nil || !strings.Contains(err.Error(), "append-only history") {
		t.Fatalf("disposition update error = %v, want append-only guard", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM certification_disposition WHERE id=$1`, dispositionID); err == nil || !strings.Contains(err.Error(), "append-only history") {
		t.Fatalf("disposition delete error = %v, want append-only guard", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO certification_disposition (
			id, workspace_id, certification_id, disposition, effective_at, reason,
			superseded_by_certification_id
		) SELECT $1, workspace_id, id, 'SUPERSEDED', issued_at, 'wrong target', $2
		FROM dataset_certification WHERE id=$3
	`, uuid.New(), replacementCertificationID, certificationID); err == nil || !strings.Contains(err.Error(), "same-target profile") {
		t.Fatalf("cross-target supersession error = %v, want same-target guard", err)
	}

	// A rejected re-evaluation is still the authoritative replacement fact for
	// an older certified result and must be allowed to supersede it.
	rejectedCertificationID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_certification (
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot, decision,
			blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,$6,'1',$7,$8,'REJECTED','[{"code":"QUALITY_GATE_NOT_PASSED"}]'::jsonb,
			're-evaluation rejected',now())
	`, rejectedCertificationID, workspaceID, first.versionID, first.qualityID, first.profileID,
		first.profileRef, first.profileHash, first.profileContent); err != nil {
		t.Fatalf("insert rejected replacement certification: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO certification_disposition (
			id, workspace_id, certification_id, disposition, effective_at, reason,
			superseded_by_certification_id
		) SELECT $1, workspace_id, id, 'SUPERSEDED', issued_at, 'rejected re-evaluation', $2
		FROM dataset_certification WHERE id=$3
	`, uuid.New(), rejectedCertificationID, certificationID); err != nil {
		t.Fatalf("rejected replacement supersession error: %v", err)
	}

	if err := tryApplyMigrationFile(t, pool, 27, "down"); err == nil || !strings.Contains(err.Error(), "refusing to downgrade DatasetCertification historical facts") {
		t.Fatalf("dataset certification down error = %v, want historical-fact refusal", err)
	}
}

func TestDatasetCertificationAllowsSeededRawEffectiveRightsLeaf(t *testing.T) {
	pool := scratchDatabase(t, 27)
	ctx := context.Background()

	workspaceID := uuid.New()
	resourceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	effectiveID := uuid.New()
	effectiveInputID := uuid.New()
	profileRef := "migration/raw-rights/" + uuid.NewString()
	profileContent := "{}"

	if _, err := pool.Exec(ctx, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type)
		VALUES ($1,$2,$3,'Raw rights resource','FILE_COLLECTION')
	`, resourceID, workspaceID, "RAW-RIGHTS-"+uuid.NewString()); err != nil {
		t.Fatalf("insert data resource: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, source_resource_id)
		VALUES ($1,$2,$3,'Raw certification dataset','RAW',$4)
	`, datasetID, workspaceID, "RAW-CERT-"+uuid.NewString(), resourceID); err != nil {
		t.Fatalf("insert raw dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://raw-certification-fixture',
			'text/csv','SHA256',repeat('d',64),'{}'::jsonb,now())
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert raw dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'certification/raw.yaml','1.0.0',
			encode(digest(convert_to('raw-certification','UTF8'),'sha256'),'hex'),
			'raw-certification','native-quality','1','PASS','{}'::jsonb)
	`, qualityID, workspaceID, versionID); err != nil {
		t.Fatalf("insert quality assessment: %v", err)
	}

	profileTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin certification profile transaction: %v", err)
	}
	defer profileTx.Rollback(ctx)

	var profileHash string
	if err := profileTx.QueryRow(ctx, `
		INSERT INTO certification_profile (
			id, workspace_id, profile_ref, code, name, version,
			content_sha256, content_snapshot, purpose_mode, action_mode,
			consumer_mode, delivery_mode, quality_gate_required,
			rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state,
			rights_purpose_mode, rights_action_mode, rights_consumer_mode, rights_scope_mode
		) VALUES ($1,$2,$3,'raw-rights-profile','Raw Rights Profile','1',
			encode(digest(convert_to($4,'UTF8'),'sha256'),'hex'),$4,'ANY','ANY',
			'ANY','ANY',false,true,false,false,false,false,'DRAFT',
			'EXPLICIT','EXPLICIT','EXPLICIT','EXPLICIT')
		RETURNING content_sha256
	`, profileID, workspaceID, profileRef, profileContent).Scan(&profileHash); err != nil {
		t.Fatalf("insert certification profile: %v", err)
	}
	if _, err := profileTx.Exec(ctx, `
		INSERT INTO certification_profile_rights_purpose (profile_id, purpose_code) VALUES ($1,'INTERNAL_USE');
		INSERT INTO certification_profile_rights_action (profile_id, action) VALUES ($1,'READ');
		INSERT INTO certification_profile_rights_consumer (profile_id, consumer_ref) VALUES ($1,'consumer-a');
		INSERT INTO certification_profile_rights_scope (profile_id, scope_type, scope_ref) VALUES ($1,'ALL_RESOURCE',$2);
	`, profileID, resourceID.String()); err != nil {
		t.Fatalf("insert certification rights memberships: %v", err)
	}
	if _, err := profileTx.Exec(ctx, `
		UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1
	`, profileID); err != nil {
		t.Fatalf("finalize certification profile: %v", err)
	}
	if err := profileTx.Commit(ctx); err != nil {
		t.Fatalf("commit certification profile: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO effective_rights_snapshot (
			id, workspace_id, target_dataset_version_id, calculation_as_of,
			consumer_ref, purpose, calculation_rule_version, calculation_rule_hash,
			required_input_hash, status, root_hash, finalized_at
		) VALUES ($1,$2,$3,now(),'consumer-a','INTERNAL_USE','1',repeat('a',64),
			repeat('b',64),'FINALIZED',repeat('c',64),now())
	`, effectiveID, workspaceID, versionID); err != nil {
		t.Fatalf("insert effective rights snapshot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO effective_rights_input (
			id, snapshot_id, input_dataset_version_id, data_resource_id, input_hash
		) VALUES ($1,$2,$3,$4,repeat('e',64))
	`, effectiveInputID, effectiveID, versionID, resourceID); err != nil {
		t.Fatalf("insert effective rights input: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO effective_rights_action (
			id, snapshot_id, action, decision, reason
		) VALUES ($1,$2,'READ','ALLOWED','raw source is currently entitled')
	`, uuid.New(), effectiveID); err != nil {
		t.Fatalf("insert effective rights action: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_certification (
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			effective_rights_snapshot_id, effective_rights_snapshot_hash,
			frozen_rights_context_hash, decision, blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,$6,'1',$7,$8,$9,repeat('c',64),
			repeat('f',64),'CERTIFIED','[]'::jsonb,'raw source certification',now())
	`, uuid.New(), workspaceID, versionID, qualityID, profileID, profileRef, profileHash,
		profileContent, effectiveID); err != nil {
		t.Fatalf("certify raw DatasetVersion with seeded effective-rights leaf: %v", err)
	}
}
