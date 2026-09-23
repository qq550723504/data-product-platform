package infrastructure

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAnnotationEngineOperationAndBindingsAreDurableCoreFacts(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	operationID := uuid.New()
	manifest := []byte("{\"operation\":\"submit\",\"tasks\":[\"" + fx.taskID.String() + "\"]}")
	manifestHash := sha256Hex(manifest)

	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_operation(
			id, workspace_id, campaign_id, provider, provider_instance_ref,
			operation_kind, request_id, request_fingerprint,
			payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256
		) VALUES (
			$1,$2,$3,'LABEL_STUDIO','local-ls','SUBMIT_TASKS',
			'submit-1',$4,$5::jsonb,$6,$7
		)
	`, operationID, fx.workspaceID, fx.campaignID, strings.Repeat("a", 64),
		string(manifest), manifest, manifestHash); err != nil {
		t.Fatalf("insert engine operation: %v", err)
	}

	_, err := pool.Exec(ctx, `
		UPDATE annotation_engine_operation
		   SET request_id='changed', revision=revision+1
		 WHERE id=$1
	`, operationID)
	if err == nil || !strings.Contains(err.Error(), "identity and payload are immutable") {
		t.Fatalf("operation identity mutation error = %v", err)
	}

	attemptID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_attempt(
			id, workspace_id, operation_id, attempt_no, attempt_kind
		) VALUES ($1,$2,$3,1,'SUBMIT')
	`, attemptID, fx.workspaceID, operationID); err != nil {
		t.Fatalf("insert engine attempt: %v", err)
	}
	outcomeID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_attempt_outcome(
			id, attempt_id, outcome, provider_status_code, diagnostic_ref
		) VALUES ($1,$2,'UNKNOWN',504,'transport-timeout')
	`, outcomeID, attemptID); err != nil {
		t.Fatalf("insert engine attempt outcome: %v", err)
	}
	_, err = pool.Exec(ctx, `
		UPDATE annotation_engine_attempt_outcome SET outcome='SUCCEEDED' WHERE id=$1
	`, outcomeID)
	if err == nil || !strings.Contains(err.Error(), "append-only annotation engine history") {
		t.Fatalf("attempt outcome mutation error = %v", err)
	}

	bindingID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_campaign_binding(
			id, workspace_id, campaign_id, provider, provider_instance_ref,
			external_project_id, request_id, config_sha256
		) VALUES ($1,$2,$3,'LABEL_STUDIO','local-ls','41','campaign-create-1',$4)
	`, bindingID, fx.workspaceID, fx.campaignID, strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert campaign binding: %v", err)
	}
	taskBindingID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_task_binding(
			id, workspace_id, campaign_binding_id, campaign_id, task_id, external_task_id
		) VALUES ($1,$2,$3,$4,$5,'101')
	`, taskBindingID, fx.workspaceID, bindingID, fx.campaignID, fx.taskID); err != nil {
		t.Fatalf("insert task binding: %v", err)
	}

	_, err = pool.Exec(ctx, `
		UPDATE annotation_engine_task_binding SET external_task_id='999' WHERE id=$1
	`, taskBindingID)
	if err == nil || !strings.Contains(err.Error(), "append-only annotation engine history") {
		t.Fatalf("task binding mutation error = %v", err)
	}
}

func TestAnnotationEngineBoundaryRejectsCrossWorkspaceFacts(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	foreignWorkspace := uuid.New()
	manifest := []byte("{\"operation\":\"submit\"}")

	_, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_operation(
			id, workspace_id, campaign_id, provider, provider_instance_ref,
			operation_kind, request_id, request_fingerprint,
			payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256
		) VALUES (
			$1,$2,$3,'LABEL_STUDIO','local-ls','SUBMIT_TASKS',
			'submit-cross-workspace',$4,$5::jsonb,$6,$7
		)
	`, uuid.New(), foreignWorkspace, fx.campaignID, strings.Repeat("c", 64),
		string(manifest), manifest, sha256Hex(manifest))
	if err == nil || !strings.Contains(err.Error(), "crosses campaign workspace") {
		t.Fatalf("cross-workspace operation error = %v", err)
	}

	operationID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_operation(
			id, workspace_id, campaign_id, provider, provider_instance_ref,
			operation_kind, request_id, request_fingerprint,
			payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256
		) VALUES (
			$1,$2,$3,'LABEL_STUDIO','local-ls','SUBMIT_TASKS',
			'submit-valid',$4,$5::jsonb,$6,$7
		)
	`, operationID, fx.workspaceID, fx.campaignID, strings.Repeat("d", 64),
		string(manifest), manifest, sha256Hex(manifest)); err != nil {
		t.Fatalf("insert valid operation: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO annotation_engine_attempt(
			id, workspace_id, operation_id, attempt_no, attempt_kind
		) VALUES ($1,$2,$3,1,'SUBMIT')
	`, uuid.New(), foreignWorkspace, operationID)
	if err == nil || !strings.Contains(err.Error(), "crosses operation workspace") {
		t.Fatalf("cross-workspace attempt error = %v", err)
	}
}
