package migration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDeliveryRetryLinkDownSucceedsWithoutRetryHistory(t *testing.T) {
	pool := scratchDatabase(t, 28)
	ctx := context.Background()

	if err := tryApplyMigrationFile(t, pool, 28, "down"); err != nil {
		t.Fatalf("delivery retry-link down without history: %v", err)
	}

	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name='delivery_operation'
		  AND column_name='retry_of_delivery_operation_id'
	`).Scan(&columns); err != nil {
		t.Fatalf("verify retry_of column removal: %v", err)
	}
	if columns != 0 {
		t.Fatalf("retry_of_delivery_operation_id columns after successful down = %d, want 0", columns)
	}
	var profileColumns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name='delivery_gate_evaluation'
		  AND column_name='certification_profile_id'
	`).Scan(&profileColumns); err != nil {
		t.Fatalf("verify certification_profile_id column removal: %v", err)
	}
	if profileColumns != 0 {
		t.Fatalf("certification_profile_id columns after successful down = %d, want 0", profileColumns)
	}
}

func TestDeliveryRetryLinkDownRefusesImmutableProfileAttribution(t *testing.T) {
	pool := scratchDatabase(t, 28)
	ctx := context.Background()

	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	profileID := uuid.New()
	operationID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Delivery profile down guard','CURATED')
	`, datasetID, workspaceID, "DELIVERY-PROFILE-DOWN-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://delivery-profile-down/fixture.csv',
			'text/csv','SHA256',repeat('a',64),'{}'::jsonb,now())
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	snapshot := "delivery-profile-down-guard"
	if _, err := pool.Exec(ctx, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version,
			content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required,
			contract_required, contract_code, traceability_required, evidence_required
		) VALUES(
			$1,$2,$3,$4,'Delivery profile down guard','1.0.0',
			encode(digest(convert_to($5,'UTF8'),'sha256'),'hex'),$5,
			'ANY','ANY','ANY','ANY',
			false,false,false,false,NULL,false,false
		)
	`, profileID, workspaceID, "delivery/down/"+profileID.String(), "DELIVERY-DOWN-"+profileID.String(), snapshot); err != nil {
		t.Fatalf("insert certification profile: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO delivery_operation (
			id, workspace_id, dataset_version_id, certification_ref,
			idempotency_key, provider_name, provider_request_key,
			status, current_gate_decision, dependency_revision,
			principal_ref, effective_consumer_ref, delegation_ref,
			purpose, action, scope_ref, delivery_channel, delivery_mode,
			requested_expires_at
		) VALUES (
			$1,$2,$3,NULL,$4,'PLATFORM_DIRECT_DATA',$5,
			'BLOCKED','BLOCKED',1,
			'principal-a','consumer-a',NULL,
			'RESEARCH','READ',$3::text,'DIRECT_DATA','DIRECT_DATA',
			now() + interval '5 minutes'
		)
	`, operationID, workspaceID, versionID, "profile-down-"+uuid.NewString(), "direct/"+operationID.String()); err != nil {
		t.Fatalf("insert delivery operation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO delivery_gate_evaluation(
			id, delivery_operation_id, evaluation_key, stage, decision, blockers,
			dependency_revision, principal_ref, effective_consumer_ref, delegation_ref,
			certification_profile_id, fresh_cap_expires_at
		) VALUES(
			$1,$2,$3,'TERMINAL_FINALIZE','BLOCKED',ARRAY['NO_CURRENT_CERTIFIED_RESULT']::text[],
			1,'principal-a','consumer-a',NULL,$4,NULL
		)
	`, uuid.New(), operationID, "profile-down-eval-"+uuid.NewString(), profileID); err != nil {
		t.Fatalf("insert delivery gate evaluation: %v", err)
	}

	err := tryApplyMigrationFile(t, pool, 28, "down")
	if err == nil || !strings.Contains(err.Error(), "cannot rollback delivery profile attribution: immutable gate history exists") {
		t.Fatalf("delivery profile-attribution down error = %v, want immutable-history refusal", err)
	}

	var persistedProfile uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT certification_profile_id
		FROM delivery_gate_evaluation
		WHERE delivery_operation_id=$1
	`, operationID).Scan(&persistedProfile); err != nil {
		t.Fatalf("verify profile attribution after refused down: %v", err)
	}
	if persistedProfile != profileID {
		t.Fatalf("profile attribution after refused down = %s, want %s", persistedProfile, profileID)
	}
}

func TestDeliveryRetryLinkDownRefusesImmutableRetryHistory(t *testing.T) {
	pool := scratchDatabase(t, 28)
	ctx := context.Background()

	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Delivery retry down guard','CURATED')
	`, datasetID, workspaceID, "DELIVERY-RETRY-DOWN-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://delivery-retry-down/fixture.csv',
			'text/csv','SHA256',repeat('a',64),'{}'::jsonb,now())
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}

	firstID := uuid.New()
	retryID := uuid.New()
	insertOperation := func(id uuid.UUID, key string, retryOf *uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO delivery_operation (
				id, workspace_id, dataset_version_id, certification_ref,
				idempotency_key, provider_name, provider_request_key,
				status, current_gate_decision, dependency_revision,
				principal_ref, effective_consumer_ref, delegation_ref,
				purpose, action, scope_ref, delivery_channel, delivery_mode,
				requested_expires_at, retry_of_delivery_operation_id
			) VALUES (
				$1,$2,$3,NULL,$4,'PLATFORM_DIRECT_DATA',$5,
				'ISSUED','ALLOWED',1,
				'principal-a','consumer-a',NULL,
				'RESEARCH','READ',$3::text,'DIRECT_DATA','DIRECT_DATA',
				now() + interval '5 minutes',$6
			)
		`, id, workspaceID, versionID, key, "direct/"+id.String(), retryOf); err != nil {
			t.Fatalf("insert delivery operation %s: %v", id, err)
		}
	}
	insertOperation(firstID, "retry-down-first-"+uuid.NewString(), nil)
	insertOperation(retryID, "retry-down-second-"+uuid.NewString(), &firstID)

	err := tryApplyMigrationFile(t, pool, 28, "down")
	if err == nil || !strings.Contains(err.Error(), "cannot rollback delivery retry link migration: immutable retry history exists") {
		t.Fatalf("delivery retry-link down error = %v, want immutable-history refusal", err)
	}

	var retryOf uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT retry_of_delivery_operation_id
		FROM delivery_operation
		WHERE id=$1
	`, retryID).Scan(&retryOf); err != nil {
		t.Fatalf("verify retry link after refused down: %v", err)
	}
	if retryOf != firstID {
		t.Fatalf("retry link after refused down = %s, want %s", retryOf, firstID)
	}

	var columns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM information_schema.columns
		WHERE table_name='delivery_operation'
		  AND column_name='retry_of_delivery_operation_id'
	`).Scan(&columns); err != nil {
		t.Fatalf("verify retry_of column after refused down: %v", err)
	}
	if columns != 1 {
		t.Fatalf("retry_of_delivery_operation_id columns after refused down = %d, want 1", columns)
	}
}
