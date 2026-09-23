package infrastructure

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestExpiredAnnotationEngineSendRecoversOnlyToUnknown(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	operationID := uuid.New()
	manifest := []byte("{\"operation\":\"submit\",\"tasks\":[\"" + fx.taskID.String() + "\"]}")
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_operation(
			id, workspace_id, campaign_id, provider, provider_instance_ref,
			operation_kind, request_id, request_fingerprint,
			payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256
		) VALUES (
			$1,$2,$3,'LABEL_STUDIO','local-ls','SUBMIT_TASKS',
			'crash-after-send',$4,$5::jsonb,$6,$7
		)
	`, operationID, fx.workspaceID, fx.campaignID, strings.Repeat("e", 64),
		string(manifest), manifest, sha256Hex(manifest)); err != nil {
		t.Fatalf("insert engine operation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE annotation_engine_operation
		   SET status='SENDING',
		       claimed_by='dead-worker',
		       claim_expires_at=$2,
		       revision=revision+1
		 WHERE id=$1
	`, operationID, time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("mark operation SENDING: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin recovery transaction: %v", err)
	}
	recovered, err := NewRepository(pool).RecoverExpiredEngineSending(
		ctx,
		tx,
		operationID,
		time.Now().UTC(),
	)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("recover expired send: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit recovery: %v", err)
	}
	if recovered.Status != "UNKNOWN" {
		t.Fatalf("recovered status = %s, want UNKNOWN", recovered.Status)
	}
	if recovered.ClaimedBy != "" || recovered.ClaimExpiresAt != nil {
		t.Fatalf("recovered claim = %q / %v, want released", recovered.ClaimedBy, recovered.ClaimExpiresAt)
	}

	var submitAttemptCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM annotation_engine_attempt
		 WHERE operation_id=$1 AND attempt_kind='SUBMIT'
	`, operationID).Scan(&submitAttemptCount); err != nil {
		t.Fatalf("count submit attempts: %v", err)
	}
	if submitAttemptCount != 0 {
		t.Fatalf("recovery created %d submit attempts, want 0", submitAttemptCount)
	}
}
