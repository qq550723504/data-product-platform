package infrastructure

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
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
	statusCode := 504
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin engine attempt outcome transaction: %v", err)
	}
	if err := NewRepository(pool).AppendEngineAttemptOutcome(ctx, tx, annotationdomain.EngineAttemptOutcome{
		ID:                 outcomeID,
		AttemptID:          attemptID,
		Outcome:            annotationdomain.EngineAttemptUnknown,
		ProviderStatusCode: &statusCode,
		DiagnosticRef:      "transport-timeout",
		OccurredAt:         time.Now().UTC(),
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert engine attempt outcome: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit engine attempt outcome: %v", err)
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

func TestAnnotationSnapshotGateRejectsUnsettledEngineOperations(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	manifest := []byte("{\"operation\":\"submit\"}")
	operationID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_operation(
			id, workspace_id, campaign_id, provider, provider_instance_ref,
			operation_kind, request_id, request_fingerprint,
			payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256
		) VALUES (
			$1,$2,$3,'LABEL_STUDIO','local-ls','SUBMIT_TASKS',
			'snapshot-gate',$4,$5::jsonb,$6,$7
		)
	`, operationID, fx.workspaceID, fx.campaignID, strings.Repeat("6", 64),
		string(manifest), manifest, sha256Hex(manifest)); err != nil {
		t.Fatalf("insert unsettled engine operation: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unsettled gate transaction: %v", err)
	}
	err = NewRepository(pool).LockAndRequireEngineOperationsSettledTx(ctx, tx, fx.campaignID)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrEngineOperationsUnsettled) {
		t.Fatalf("unsettled gate error = %v, want ErrEngineOperationsUnsettled", err)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE annotation_engine_operation
		   SET status='MATCHED', revision=revision+1
		 WHERE id=$1
	`, operationID); err != nil {
		t.Fatalf("settle engine operation: %v", err)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin settled gate transaction: %v", err)
	}
	if err := NewRepository(pool).LockAndRequireEngineOperationsSettledTx(ctx, tx, fx.campaignID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("settled engine operation blocked snapshot gate: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit settled gate transaction: %v", err)
	}
}

func TestAnnotationEngineActorBindingIsImmutableAndUnambiguous(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	repo := NewRepository(pool)
	binding := annotationdomain.EngineActorBinding{
		ID:               uuid.New(),
		WorkspaceID:      fx.workspaceID,
		Provider:         "LABEL_STUDIO",
		ProviderInstance: "local-ls",
		ExternalActorRef: "17",
		CoreActorRef:     "user:annotator-1",
		CreatedAt:        time.Now().UTC(),
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin actor binding transaction: %v", err)
	}
	created, err := repo.InsertEngineActorBinding(ctx, tx, binding)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert actor binding: %v", err)
	}
	if !created {
		_ = tx.Rollback(ctx)
		t.Fatal("actor binding was not created")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit actor binding: %v", err)
	}

	resolved, err := repo.ResolveEngineActorBinding(
		ctx, fx.workspaceID, "LABEL_STUDIO", "local-ls", "17",
	)
	if err != nil {
		t.Fatalf("resolve actor binding: %v", err)
	}
	if resolved.CoreActorRef != "user:annotator-1" {
		t.Fatalf("core actor ref = %q", resolved.CoreActorRef)
	}

	_, err = pool.Exec(ctx, `
		UPDATE annotation_engine_actor_binding
		   SET core_actor_ref='user:other'
		 WHERE id=$1
	`, binding.ID)
	if err == nil || !strings.Contains(err.Error(), "append-only annotation engine history") {
		t.Fatalf("actor binding mutation error = %v", err)
	}

	conflict := binding
	conflict.ID = uuid.New()
	conflict.ExternalActorRef = "18"
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin actor conflict transaction: %v", err)
	}
	_, err = repo.InsertEngineActorBinding(ctx, tx, conflict)
	_ = tx.Rollback(ctx)
	if !errors.Is(err, ErrEngineBindingConflict) {
		t.Fatalf("actor binding conflict error = %v, want ErrEngineBindingConflict", err)
	}
}
