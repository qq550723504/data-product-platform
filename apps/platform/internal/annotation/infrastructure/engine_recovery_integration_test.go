package infrastructure

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
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
	attemptID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_attempt(
			id, workspace_id, operation_id, attempt_no, attempt_kind
		) VALUES ($1,$2,$3,1,'SUBMIT')
	`, attemptID, fx.workspaceID, operationID); err != nil {
		t.Fatalf("insert durable submit attempt: %v", err)
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
	if submitAttemptCount != 1 {
		t.Fatalf("submit attempt count = %d, want 1", submitAttemptCount)
	}

	var outcome, diagnostic string
	if err := pool.QueryRow(ctx, `
		SELECT outcome, COALESCE(diagnostic_ref,'')
		  FROM annotation_engine_attempt_outcome
		 WHERE attempt_id=$1
	`, attemptID).Scan(&outcome, &diagnostic); err != nil {
		t.Fatalf("read recovered submit attempt outcome: %v", err)
	}
	if outcome != "UNKNOWN" {
		t.Fatalf("recovered submit attempt outcome = %s, want UNKNOWN", outcome)
	}
	if !strings.Contains(diagnostic, "lease expired") {
		t.Fatalf("recovery diagnostic = %q", diagnostic)
	}
}

func TestExpiredSendWithCommittedOutcomeRecoversAndAcceptsLateObservation(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	repo := NewRepository(pool)
	operationID := uuid.New()
	manifest := []byte("{\"operation\":\"submit\",\"tasks\":[\"" + fx.taskID.String() + "\"]}")
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_operation(
			id, workspace_id, campaign_id, provider, provider_instance_ref,
			operation_kind, request_id, request_fingerprint,
			payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256
		) VALUES (
			$1,$2,$3,'LABEL_STUDIO','local-ls','SUBMIT_TASKS',
			'crash-after-outcome',$4,$5::jsonb,$6,$7
		)
	`, operationID, fx.workspaceID, fx.campaignID, strings.Repeat("f", 64),
		string(manifest), manifest, sha256Hex(manifest)); err != nil {
		t.Fatalf("insert engine operation: %v", err)
	}

	attemptID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_engine_attempt(
			id, workspace_id, operation_id, attempt_no, attempt_kind
		) VALUES ($1,$2,$3,1,'SUBMIT')
	`, attemptID, fx.workspaceID, operationID); err != nil {
		t.Fatalf("insert durable submit attempt: %v", err)
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
		t.Fatalf("begin outcome transaction: %v", err)
	}
	if err := repo.AppendEngineAttemptOutcome(ctx, tx, annotationdomain.EngineAttemptOutcome{
		ID:            uuid.New(),
		AttemptID:     attemptID,
		Outcome:       annotationdomain.EngineAttemptSucceeded,
		DiagnosticRef: "provider returned before projection commit",
		OccurredAt:    time.Now().UTC(),
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append committed outcome: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit provider outcome: %v", err)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin recovery transaction: %v", err)
	}
	recovered, err := repo.RecoverExpiredEngineSending(ctx, tx, operationID, time.Now().UTC())
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("recover committed-outcome send: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit recovery: %v", err)
	}
	if recovered.Status != annotationdomain.EngineOperationUnknown {
		t.Fatalf("recovered status = %s, want UNKNOWN", recovered.Status)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin late observation transaction: %v", err)
	}
	if err := repo.AppendEngineAttemptOutcome(ctx, tx, annotationdomain.EngineAttemptOutcome{
		ID:            uuid.New(),
		AttemptID:     attemptID,
		Outcome:       annotationdomain.EngineAttemptConflict,
		DiagnosticRef: "late definitive provider observation",
		OccurredAt:    time.Now().UTC(),
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append late observation: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit late observation: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT observation_no, outcome
		  FROM annotation_engine_attempt_outcome
		 WHERE attempt_id=$1
		 ORDER BY observation_no
	`, attemptID)
	if err != nil {
		t.Fatalf("query attempt observations: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var observationNo int
		var outcome string
		if err := rows.Scan(&observationNo, &outcome); err != nil {
			t.Fatalf("scan attempt observation: %v", err)
		}
		got = append(got, fmt.Sprintf("%d:%s", observationNo, outcome))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate attempt observations: %v", err)
	}
	if len(got) != 2 || got[0] != "1:SUCCEEDED" || got[1] != "2:CONFLICT" {
		t.Fatalf("attempt observations = %v", got)
	}
}
