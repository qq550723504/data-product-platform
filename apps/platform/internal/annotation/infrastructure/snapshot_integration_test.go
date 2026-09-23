package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

type annotationDBFixture struct {
	workspaceID     uuid.UUID
	campaignID      uuid.UUID
	taskID          uuid.UUID
	resultID        uuid.UUID
	decisionID      uuid.UUID
	attemptID       uuid.UUID
	snapshotID      uuid.UUID
	versionID       uuid.UUID
	certificationID uuid.UUID
	resourceID      uuid.UUID
	manifest        []byte
	rootHash        string
	payloadHash     string
	inputChecksum   string
	builtAt         time.Time
	resultCreatedAt time.Time
}

func TestAnnotationSnapshotFinalizationFreezesHeaderAndMembership(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin snapshot transaction: %v", err)
	}
	insertAnnotationSnapshotAggregate(t, ctx, tx, fx)
	if _, err := tx.Exec(ctx, `
		UPDATE annotation_snapshot
		   SET status='FINALIZED', finalized_at=now()
		 WHERE id=$1
	`, fx.snapshotID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("finalize snapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit finalized snapshot: %v", err)
	}

	var snapshotStatus, campaignStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM annotation_snapshot WHERE id=$1`, fx.snapshotID).Scan(&snapshotStatus); err != nil {
		t.Fatalf("read snapshot status: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM annotation_campaign WHERE id=$1`, fx.campaignID).Scan(&campaignStatus); err != nil {
		t.Fatalf("read campaign status: %v", err)
	}
	if snapshotStatus != "FINALIZED" || campaignStatus != "SEALED" {
		t.Fatalf("snapshot/campaign = %s/%s, want FINALIZED/SEALED", snapshotStatus, campaignStatus)
	}
	integrityValid, err := NewRepository(pool).GetSnapshotIntegrity(ctx, fx.snapshotID)
	if err != nil {
		t.Fatalf("verify snapshot integrity: %v", err)
	}
	if !integrityValid {
		t.Fatal("finalized annotation snapshot integrity is invalid")
	}

	assertAnnotationMutationRejected(t, pool, ctx, `
		UPDATE annotation_snapshot SET expected_output_count=0 WHERE id=$1
	`, "annotation snapshot is immutable", fx.snapshotID)
	assertAnnotationMutationRejected(t, pool, ctx, `
		DELETE FROM annotation_snapshot_task WHERE snapshot_id=$1 AND task_id=$2
	`, "membership is immutable", fx.snapshotID, fx.taskID)
	assertAnnotationMutationRejected(t, pool, ctx, `
		UPDATE annotation_snapshot_result
		   SET canonical_payload_sha256=$3
		 WHERE snapshot_id=$1 AND result_id=$2
	`, "membership is immutable", fx.snapshotID, fx.resultID, strings.Repeat("f", 64))
	assertAnnotationMutationRejected(t, pool, ctx, `
		INSERT INTO annotation_snapshot_output(snapshot_id, task_id, selected_result_id)
		VALUES ($1,$2,$3)
	`, "membership is finalized", fx.snapshotID, fx.taskID, fx.resultID)
	assertAnnotationMutationRejected(t, pool, ctx, `
		UPDATE annotation_task SET revision=revision+1 WHERE id=$1
	`, "immutable outside ACTIVE campaign", fx.taskID)
}

func TestAnnotationWorkspaceIsolationFailsClosed(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	foreignWorkspace := uuid.New()
	specContent := annotationFixtureSpecContent()
	specHash := sha256Hex([]byte(specContent))

	_, err := pool.Exec(ctx, `
		INSERT INTO annotation_campaign(
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot
		) VALUES (
			$1,$2,$3,$4,$5,'gold-pilot','PROCESS',
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7
		)
	`, uuid.New(), foreignWorkspace, fx.versionID, fx.certificationID, fx.resourceID, specHash, specContent)
	if err == nil || !strings.Contains(err.Error(), "does not match workspace") {
		t.Fatalf("cross-workspace campaign error = %v, want workspace rejection", err)
	}

	payload := []byte("{\"label\":\"FOREIGN\"}")
	_, err = pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','foreign-obs-task','foreign-obs-annotation','1','foreign:obs',$5,$6,'fixture-v1')
	`, uuid.New(), foreignWorkspace, fx.campaignID, fx.taskID, payload, sha256Hex(payload))
	if err == nil || !strings.Contains(err.Error(), "does not match active task") {
		t.Fatalf("cross-workspace result error = %v, want active task workspace rejection", err)
	}
}

func TestAnnotationSnapshotRejectsDirectFinalizedInsert(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	_, err := pool.Exec(ctx, `
		INSERT INTO annotation_snapshot(
			id, workspace_id, campaign_id, status, manifest, manifest_hash_payload, root_hash,
			expected_task_count, expected_result_count, expected_decision_count, expected_output_count,
			finalized_at
		) VALUES ($1,$2,$3,'FINALIZED',$4,$5,$6,1,1,1,1,now())
	`, fx.snapshotID, fx.workspaceID, fx.campaignID, fx.manifest, fx.manifest, fx.rootHash)
	if err == nil || !strings.Contains(err.Error(), "must start BUILDING") {
		t.Fatalf("direct FINALIZED insert error = %v, want BUILDING guard", err)
	}
}

func TestAnnotationSnapshotCannotCommitUnfinishedBuilding(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin unfinished snapshot: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_snapshot(
			id, workspace_id, campaign_id, manifest, manifest_hash_payload, root_hash,
			expected_task_count, expected_result_count, expected_decision_count, expected_output_count,
			created_at
		) VALUES ($1,$2,$3,$4,$5,$6,1,1,1,1,$7)
	`, fx.snapshotID, fx.workspaceID, fx.campaignID, fx.manifest, fx.manifest, fx.rootHash, fx.builtAt); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert BUILDING snapshot: %v", err)
	}
	err = tx.Commit(ctx)
	if err == nil || !strings.Contains(err.Error(), "must be FINALIZED before commit") {
		t.Fatalf("unfinished BUILDING commit error = %v, want deferred finalization guard", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM annotation_snapshot WHERE id=$1`, fx.snapshotID).Scan(&count); err != nil {
		t.Fatalf("count rolled-back BUILDING snapshot: %v", err)
	}
	if count != 0 {
		t.Fatalf("unfinished BUILDING snapshot persisted: count=%d", count)
	}
}

func TestAnnotationSnapshotDoubleFinalizerLeavesOneSnapshot(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	winner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin winner finalizer: %v", err)
	}
	insertAnnotationSnapshotAggregate(t, ctx, winner, fx)
	if _, err := winner.Exec(ctx, `
		UPDATE annotation_snapshot SET status='FINALIZED', finalized_at=now() WHERE id=$1
	`, fx.snapshotID); err != nil {
		_ = winner.Rollback(ctx)
		t.Fatalf("finalize winner snapshot: %v", err)
	}

	loserDone := make(chan error, 1)
	loserSnapshotID := uuid.New()
	go func() {
		loserCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, loserErr := pool.Exec(loserCtx, `
			INSERT INTO annotation_snapshot(
				id, workspace_id, campaign_id, manifest, manifest_hash_payload, root_hash,
				expected_task_count, expected_result_count, expected_decision_count, expected_output_count
			) VALUES ($1,$2,$3,$4,$5,$6,1,1,1,1)
		`, loserSnapshotID, fx.workspaceID, fx.campaignID, fx.manifest, fx.rootHash)
		loserDone <- loserErr
	}()

	time.Sleep(150 * time.Millisecond)
	if err := winner.Commit(ctx); err != nil {
		t.Fatalf("commit winner finalizer: %v", err)
	}

	select {
	case loserErr := <-loserDone:
		if loserErr == nil {
			t.Fatal("second finalizer unexpectedly inserted a second snapshot")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("second finalizer did not converge")
	}

	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM annotation_snapshot WHERE campaign_id=$1 AND status='FINALIZED'
	`, fx.campaignID).Scan(&count); err != nil {
		t.Fatalf("count finalized snapshots: %v", err)
	}
	if count != 1 {
		t.Fatalf("finalized snapshot count = %d, want 1", count)
	}
}

func TestAnnotationSnapshotSealRejectsLateResult(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin snapshot transaction: %v", err)
	}
	insertAnnotationSnapshotAggregate(t, ctx, tx, fx)
	if _, err := tx.Exec(ctx, `
		UPDATE annotation_snapshot
		   SET status='FINALIZED', finalized_at=now()
		 WHERE id=$1
	`, fx.snapshotID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("finalize snapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit finalized snapshot: %v", err)
	}

	payload := []byte("{\"label\":\"late\"}")
	_, err = pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','obs-late-task','obs-late-annotation','1','obs:late',$5,$6,'fixture-v1')
	`, uuid.New(), fx.workspaceID, fx.campaignID, fx.taskID, payload, sha256Hex(payload))
	if err == nil || !strings.Contains(err.Error(), "requires ACTIVE campaign") {
		t.Fatalf("late result error = %v, want sealed campaign rejection", err)
	}
}

func TestAnnotationSnapshotRejectsManifestMembershipMismatch(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	var manifest map[string]any
	if err := json.Unmarshal(fx.manifest, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest["outputs"] = []any{}
	tampered, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode tampered manifest: %v", err)
	}
	fx.manifest = tampered
	fx.rootHash = sha256Hex(tampered)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin snapshot transaction: %v", err)
	}
	insertAnnotationSnapshotAggregate(t, ctx, tx, fx)
	_, err = tx.Exec(ctx, `
		UPDATE annotation_snapshot
		   SET status='FINALIZED', finalized_at=now()
		 WHERE id=$1
	`, fx.snapshotID)
	if err == nil || !strings.Contains(err.Error(), "manifest membership does not match") {
		_ = tx.Rollback(ctx)
		t.Fatalf("finalize mismatch error = %v, want manifest membership mismatch", err)
	}
	_ = tx.Rollback(ctx)
}

func TestAnnotationSnapshotLateMembershipWriterFailsClosed(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	finalizer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin finalizer: %v", err)
	}
	insertAnnotationSnapshotAggregate(t, ctx, finalizer, fx)

	writerDone := make(chan error, 1)
	go func() {
		writerCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, writeErr := pool.Exec(writerCtx, `
			INSERT INTO annotation_snapshot_task (
				snapshot_id, task_id, source_item_ref, source_content_sha256, task_text_sha256
			) VALUES ($1,$2,'duplicate',$3,$4)
		`, fx.snapshotID, fx.taskID, strings.Repeat("a", 64), strings.Repeat("b", 64))
		writerDone <- writeErr
	}()

	time.Sleep(150 * time.Millisecond)
	if _, err := finalizer.Exec(ctx, `
		UPDATE annotation_snapshot SET status='FINALIZED', finalized_at=now() WHERE id=$1
	`, fx.snapshotID); err != nil {
		_ = finalizer.Rollback(ctx)
		t.Fatalf("finalize snapshot: %v", err)
	}
	if err := finalizer.Commit(ctx); err != nil {
		t.Fatalf("commit finalizer: %v", err)
	}

	select {
	case writeErr := <-writerDone:
		if writeErr == nil ||
			(!strings.Contains(writeErr.Error(), "parent is not visible") &&
				!strings.Contains(writeErr.Error(), "membership is finalized")) {
			t.Fatalf("late membership writer error = %v, want invisible-parent or finalized rejection", writeErr)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("late membership writer did not converge")
	}
}

func TestAnnotationReviewCostUsesPhysicalAttemptIdentity(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	appendCost := func(attemptID uuid.UUID) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin cost transaction: %v", err)
		}
		if err := cost.AppendAnnotationReviewActivity(ctx, tx, cost.AnnotationReviewActivity{
			WorkspaceID: fx.workspaceID,
			AttemptID:   attemptID,
			Quantity:    1,
			Unit:        "review",
			PricingMode: "ACTUAL",
			Metadata:    map[string]any{"test": "annotation-review"},
		}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("append annotation review cost: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit annotation review cost: %v", err)
		}
	}

	appendCost(fx.attemptID)
	appendCost(fx.attemptID)

	loserAttempt := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_review_attempt(
			id, workspace_id, campaign_id, task_id, reviewer_ref, expected_task_revision,
			review_action, reason, idempotency_key, request_fingerprint
		) VALUES ($1,$2,$3,$4,'reviewer-loser',2,'ACCEPT','stale work',$5,$6)
	`, loserAttempt, fx.workspaceID, fx.campaignID, fx.taskID,
		"cost-loser-"+uuid.NewString(), strings.Repeat("9", 64)); err != nil {
		t.Fatalf("insert losing review attempt: %v", err)
	}
	appendCost(loserAttempt)

	var eventCount, allocationCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM cost_event
		 WHERE workspace_id=$1
		   AND cost_type=$2
		   AND activity_id IN ($3,$4)
	`, fx.workspaceID, cost.AnnotationReviewCostType, fx.attemptID, loserAttempt).Scan(&eventCount); err != nil {
		t.Fatalf("count annotation review cost events: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		  FROM cost_allocation
		 WHERE annotation_review_attempt_id IN ($1,$2)
	`, fx.attemptID, loserAttempt).Scan(&allocationCount); err != nil {
		t.Fatalf("count annotation review cost allocations: %v", err)
	}
	if eventCount != 2 || allocationCount != 2 {
		t.Fatalf("annotation review cost events/allocations = %d/%d, want 2/2", eventCount, allocationCount)
	}
}

func TestAnnotationProviderObservationIdentityDeduplicatesAliasChanges(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	base := createAnnotationDBFixture(t, ctx, pool)
	campaignID, taskID, _ := createAnnotationReviewRaceFixture(t, ctx, pool, base)

	payload := []byte("{\"label\":\"DEDUP\"}")
	payloadHash := sha256Hex(payload)
	firstID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','dedup-provider','dedup-task','dedup-ann','7',
		          'caller-alias-a',$5,$6,'fixture-v1')
	`, firstID, base.workspaceID, campaignID, taskID, payload, payloadHash); err != nil {
		t.Fatalf("insert first provider observation: %v", err)
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','dedup-provider','dedup-task','dedup-ann','7',
		          'caller-alias-b',$5,$6,'fixture-v1')
	`, uuid.New(), base.workspaceID, campaignID, taskID, payload, payloadHash)
	if err == nil || !strings.Contains(err.Error(), "uq_annotation_result_provider_observation") {
		t.Fatalf("duplicate provider observation error = %v, want provider identity unique violation", err)
	}
}

func TestAnnotationDatabaseSchemaValidationMatchesGoTrimSpace(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	cases := []struct {
		name    string
		schema  string
		payload string
		want    bool
	}{
		{
			name:    "schema tab is normalized",
			schema:  "{\"kind\":\"single-label-v1\",\"labels\":[\"\\tA\"]}",
			payload: "{\"label\":\"A\"}",
			want:    true,
		},
		{
			name:    "payload tab is rejected as non-normalized",
			schema:  "{\"kind\":\"single-label-v1\",\"labels\":[\"A\"]}",
			payload: "{\"label\":\"\\tA\"}",
			want:    false,
		},
		{
			name:    "schema nbsp is normalized",
			schema:  "{\"kind\":\"single-label-v1\",\"labels\":[\"\\u00a0A\"]}",
			payload: "{\"label\":\"A\"}",
			want:    true,
		},
		{
			name:    "payload nbsp is rejected as non-normalized",
			schema:  "{\"kind\":\"single-label-v1\",\"labels\":[\"A\"]}",
			payload: "{\"label\":\"\\u00a0A\"}",
			want:    false,
		},
		{
			name:    "schema labels duplicate after trim",
			schema:  "{\"kind\":\"single-label-v1\",\"labels\":[\"A\",\"\\tA\"]}",
			payload: "{\"label\":\"A\"}",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got bool
			if err := pool.QueryRow(ctx, `
				SELECT annotation_payload_matches_frozen_schema($1, convert_to($2, 'UTF8'))
			`, tc.schema, tc.payload).Scan(&got); err != nil {
				t.Fatalf("schema validation query: %v", err)
			}
			if got != tc.want {
				t.Fatalf("schema validation = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnnotationDatabaseRejectsPayloadOutsideFrozenSchema(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	base := createAnnotationDBFixture(t, ctx, pool)
	campaignID, taskID, _ := createAnnotationReviewRaceFixture(t, ctx, pool, base)

	payload := []byte("{\"label\":\"NOT-ALLOWED\"}")
	_, err := pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','schema-test-provider','schema-test-task',
		          'schema-test-ann','1','schema-test-alias',$5,$6,'fixture-v1')
	`, uuid.New(), base.workspaceID, campaignID, taskID, payload, sha256Hex(payload))
	if err == nil || !strings.Contains(err.Error(), "violates frozen schema") {
		t.Fatalf("schema-invalid direct insert error = %v, want frozen schema rejection", err)
	}
}


func TestAnnotationResultAliasReservationIsAtomicAndAppendOnly(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	base := createAnnotationDBFixture(t, ctx, pool)
	campaignID, taskID, firstResultID := createAnnotationReviewRaceFixture(t, ctx, pool, base)

	secondResultID := uuid.New()
	payload := []byte("{\"label\":\"A\"}")
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES (
			$1,$2,$3,$4,'annotator','alias-race-provider','alias-race-task',
			'alias-race-second','1','alias-race-second-canonical',$5,$6,'fixture-v1'
		)
	`, secondResultID, base.workspaceID, campaignID, taskID, payload, sha256Hex(payload)); err != nil {
		t.Fatalf("insert second alias-race result: %v", err)
	}

	repo := NewRepository(pool)
	alias := "shared-replay-alias"
	type reservation struct {
		resultID uuid.UUID
		err      error
	}
	results := make(chan reservation, 2)
	for _, resultID := range []uuid.UUID{firstResultID, secondResultID} {
		resultID := resultID
		go func() {
			results <- reservation{
				resultID: resultID,
				err: repo.ReserveResultAlias(ctx, base.workspaceID, campaignID, alias, resultID),
			}
		}()
	}

	first := <-results
	second := <-results
	var winner, loser reservation
	switch {
	case first.err == nil && second.err == ErrResultAliasConflict:
		winner, loser = first, second
	case second.err == nil && first.err == ErrResultAliasConflict:
		winner, loser = second, first
	default:
		t.Fatalf("alias race results = (%s,%v) / (%s,%v), want one success and one alias conflict",
			first.resultID, first.err, second.resultID, second.err)
	}

	resolved, err := repo.GetResultByObservation(ctx, base.workspaceID, campaignID, alias)
	if err != nil {
		t.Fatalf("resolve reserved alias: %v", err)
	}
	if resolved.ID != winner.resultID {
		t.Fatalf("alias resolved to %s, want winner %s", resolved.ID, winner.resultID)
	}
	if err := repo.ReserveResultAlias(ctx, base.workspaceID, campaignID, alias, winner.resultID); err != nil {
		t.Fatalf("winner alias replay must stay idempotent: %v", err)
	}
	if err := repo.ReserveResultAlias(ctx, base.workspaceID, campaignID, alias, loser.resultID); err != ErrResultAliasConflict {
		t.Fatalf("loser alias replay error = %v, want alias conflict", err)
	}
}

func TestAnnotationResultFirstMakesOldReviewStale(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	base := createAnnotationDBFixture(t, ctx, pool)
	campaignID, taskID, firstResultID := createAnnotationReviewRaceFixture(t, ctx, pool, base)
	attemptID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_review_attempt(
			id, workspace_id, campaign_id, task_id, reviewer_ref, expected_task_revision,
			review_action, reason, idempotency_key, request_fingerprint
		) VALUES ($1,$2,$3,$4,'reviewer',2,'ACCEPT','old revision',$5,$6)
	`, attemptID, base.workspaceID, campaignID, taskID,
		"result-first-"+uuid.NewString(), strings.Repeat("8", 64)); err != nil {
		t.Fatalf("insert review attempt: %v", err)
	}

	resultTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin result transaction: %v", err)
	}
	payload := []byte("{\"label\":\"newer\"}")
	if _, err := resultTx.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','obs-newer-task','obs-newer-annotation','1','obs:newer',$5,$6,'fixture-v1')
	`, uuid.New(), base.workspaceID, campaignID, taskID, payload, sha256Hex(payload)); err != nil {
		_ = resultTx.Rollback(ctx)
		t.Fatalf("insert newer result: %v", err)
	}
	if _, err := resultTx.Exec(ctx, `
		UPDATE annotation_task
		   SET status='REVIEWABLE', revision=revision+1
		 WHERE id=$1 AND revision=2
	`, taskID); err != nil {
		_ = resultTx.Rollback(ctx)
		t.Fatalf("advance task for newer result: %v", err)
	}

	reviewDone := make(chan error, 1)
	go func() {
		reviewCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, reviewErr := pool.Exec(reviewCtx, `
			INSERT INTO annotation_review_decision(
				id, workspace_id, campaign_id, task_id, review_attempt_id,
				reviewed_result_id, selected_result_id, reviewer_ref, outcome, reason,
				expected_task_revision
			) VALUES ($1,$2,$3,$4,$5,$6,$6,'reviewer','ACCEPT','old revision',2)
		`, uuid.New(), base.workspaceID, campaignID, taskID, attemptID, firstResultID)
		reviewDone <- reviewErr
	}()

	time.Sleep(150 * time.Millisecond)
	if err := resultTx.Commit(ctx); err != nil {
		t.Fatalf("commit newer result: %v", err)
	}

	select {
	case reviewErr := <-reviewDone:
		if reviewErr == nil || !strings.Contains(reviewErr.Error(), "expected task/attempt CAS") {
			t.Fatalf("old review error = %v, want stale CAS", reviewErr)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("old review did not converge")
	}

	var revision int64
	var decisionCount int
	if err := pool.QueryRow(ctx, `SELECT revision FROM annotation_task WHERE id=$1`, taskID).Scan(&revision); err != nil {
		t.Fatalf("read task revision: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM annotation_review_decision WHERE task_id=$1`, taskID).Scan(&decisionCount); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	if revision != 3 || decisionCount != 0 {
		t.Fatalf("result-first state revision/decisions = %d/%d, want 3/0", revision, decisionCount)
	}
}

func TestAnnotationReviewFirstRejectsLaterResult(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	fx := createAnnotationDBFixture(t, ctx, pool)
	payload := []byte("{\"label\":\"too-late\"}")
	_, err := pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','obs-after-review-task','obs-after-review-annotation','1','obs:after-review',$5,$6,'fixture-v1')
	`, uuid.New(), fx.workspaceID, fx.campaignID, fx.taskID, payload, sha256Hex(payload))
	if err == nil || !strings.Contains(err.Error(), "does not match active task") {
		t.Fatalf("post-review result error = %v, want terminal task rejection", err)
	}
}

func TestAnnotationReviewDecisionSerializesConcurrentReviewers(t *testing.T) {
	pool, ctx := openAnnotationTestDB(t)
	defer pool.Close()

	base := createAnnotationDBFixture(t, ctx, pool)
	campaignID, taskID, resultID := createAnnotationReviewRaceFixture(t, ctx, pool, base)
	winnerAttempt := uuid.New()
	loserAttempt := uuid.New()
	for _, attempt := range []struct {
		id       uuid.UUID
		reviewer string
		key      string
	}{
		{winnerAttempt, "reviewer-a", "race-a-" + uuid.NewString()},
		{loserAttempt, "reviewer-b", "race-b-" + uuid.NewString()},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO annotation_review_attempt(
				id, workspace_id, campaign_id, task_id, reviewer_ref, expected_task_revision,
				review_action, reason, idempotency_key, request_fingerprint
			) VALUES ($1,$2,$3,$4,$5,2,'ACCEPT','race review',$6,$7)
		`, attempt.id, base.workspaceID, campaignID, taskID, attempt.reviewer, attempt.key, strings.Repeat("f", 64)); err != nil {
			t.Fatalf("insert race review attempt: %v", err)
		}
	}

	winnerTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin winner review: %v", err)
	}
	winnerDecision := uuid.New()
	if _, err := winnerTx.Exec(ctx, `
		INSERT INTO annotation_review_decision(
			id, workspace_id, campaign_id, task_id, review_attempt_id,
			reviewed_result_id, selected_result_id, reviewer_ref, outcome, reason,
			expected_task_revision
		) VALUES ($1,$2,$3,$4,$5,$6,$6,'reviewer-a','ACCEPT','race review',2)
	`, winnerDecision, base.workspaceID, campaignID, taskID, winnerAttempt, resultID); err != nil {
		_ = winnerTx.Rollback(ctx)
		t.Fatalf("insert winner decision: %v", err)
	}
	if _, err := winnerTx.Exec(ctx, `
		INSERT INTO annotation_review_attempt_outcome(id, attempt_id, outcome)
		VALUES ($1,$2,'SUCCEEDED')
	`, uuid.New(), winnerAttempt); err != nil {
		_ = winnerTx.Rollback(ctx)
		t.Fatalf("insert winner outcome: %v", err)
	}

	loserDone := make(chan error, 1)
	go func() {
		loserCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		loserTx, beginErr := pool.Begin(loserCtx)
		if beginErr != nil {
			loserDone <- beginErr
			return
		}
		defer loserTx.Rollback(context.Background())
		_, insertErr := loserTx.Exec(loserCtx, `
			INSERT INTO annotation_review_decision(
				id, workspace_id, campaign_id, task_id, review_attempt_id,
				reviewed_result_id, selected_result_id, reviewer_ref, outcome, reason,
				expected_task_revision
			) VALUES ($1,$2,$3,$4,$5,$6,$6,'reviewer-b','ACCEPT','race review',2)
		`, uuid.New(), base.workspaceID, campaignID, taskID, loserAttempt, resultID)
		if insertErr == nil {
			insertErr = loserTx.Commit(loserCtx)
		}
		loserDone <- insertErr
	}()

	time.Sleep(150 * time.Millisecond)
	if err := winnerTx.Commit(ctx); err != nil {
		t.Fatalf("commit winner review: %v", err)
	}

	select {
	case loserErr := <-loserDone:
		if loserErr == nil || !strings.Contains(loserErr.Error(), "expected task/attempt CAS") {
			t.Fatalf("losing review error = %v, want stale CAS rejection", loserErr)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("losing reviewer did not converge")
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_review_attempt_outcome(id, attempt_id, outcome, error_code)
		VALUES ($1,$2,'STALE_CONFLICT','STALE_REVISION')
	`, uuid.New(), loserAttempt); err != nil {
		t.Fatalf("record losing review outcome: %v", err)
	}

	var decisionCount int
	var taskRevision int64
	var currentDecision uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM annotation_review_decision WHERE task_id=$1
	`, taskID).Scan(&decisionCount); err != nil {
		t.Fatalf("count race decisions: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT revision, current_decision_id FROM annotation_task WHERE id=$1
	`, taskID).Scan(&taskRevision, &currentDecision); err != nil {
		t.Fatalf("read race task projection: %v", err)
	}
	if decisionCount != 1 || taskRevision != 3 || currentDecision != winnerDecision {
		t.Fatalf("race result decisions/revision/current = %d/%d/%s, want 1/3/%s",
			decisionCount, taskRevision, currentDecision, winnerDecision)
	}
}

func createAnnotationReviewRaceFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	base annotationDBFixture,
) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	campaignID := uuid.New()
	taskID := uuid.New()
	resultID := uuid.New()
	specContent := annotationFixtureSpecContent()
	specHash := sha256Hex([]byte(specContent))

	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_campaign(
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot
		) VALUES (
			$1,$2,$3,$4,$5,'gold-pilot','PROCESS',
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7
		)
	`, campaignID, base.workspaceID, base.versionID, base.certificationID, base.resourceID, specHash, specContent); err != nil {
		t.Fatalf("insert race campaign: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES ($1,$2,$3,'row:race',$4,$5,'annotator')
	`, taskID, base.workspaceID, campaignID, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert race task: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=1,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("c", 64), base.inputChecksum); err != nil {
		t.Fatalf("activate race campaign: %v", err)
	}
	payload := []byte("{\"label\":\"RACE\"}")
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','obs-race-task','obs-race-annotation','1','obs:race',$5,$6,'fixture-v1')
	`, resultID, base.workspaceID, campaignID, taskID, payload, sha256Hex(payload)); err != nil {
		t.Fatalf("insert race result: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE annotation_task SET status='REVIEWABLE', revision=2 WHERE id=$1
	`, taskID); err != nil {
		t.Fatalf("make race task reviewable: %v", err)
	}
	return campaignID, taskID, resultID
}

func openAnnotationTestDB(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	return pool, ctx
}

func createAnnotationDBFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) annotationDBFixture {
	t.Helper()

	workspaceID := uuid.New()
	resourceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	certificationID := uuid.New()
	campaignID := uuid.New()
	taskID := uuid.New()
	resultID := uuid.New()
	attemptID := uuid.New()
	decisionID := uuid.New()
	snapshotID := uuid.New()
	codeSuffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	inputChecksum := strings.Repeat("1", 64)
	builtAt := time.Date(2026, 9, 23, 10, 0, 0, 123456000, time.UTC)
	resultCreatedAt := time.Date(2026, 9, 23, 9, 59, 59, 654321000, time.UTC)

	if _, err := pool.Exec(ctx, `
		INSERT INTO data_resource(id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES ($1,$2,$3,'annotation contribution','OTHER','READY')
	`, resourceID, workspaceID, "ANN-"+codeSuffix); err != nil {
		t.Fatalf("insert data resource: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'annotation input','CURATED')
	`, datasetID, workspaceID, "DS-"+codeSuffix); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, checksum_algorithm, checksum_value, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT','test://annotation','application/json',1,'SHA256',$3,now())
	`, versionID, datasetID, inputChecksum); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}

	ruleContent := "annotation-fixture-rules"
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result(
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, rule_set_content_sha256, rule_set_content,
			evaluator_name, evaluator_version
		) VALUES ($1,$2,$3,'fixture-rules','1','PASS',$4,$5,$6,'fixture','1')
	`, qualityID, workspaceID, versionID, []byte("{\"dimensions\":{}}"), sha256Hex([]byte(ruleContent)), ruleContent); err != nil {
		t.Fatalf("insert quality result: %v", err)
	}

	profileContent := "annotation-fixture-profile"
	profileHash := sha256Hex([]byte(profileContent))
	if _, err := pool.Exec(ctx, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state
		) VALUES ($1,$2,'fixture-profile',$3,'fixture profile','1',$4,$5,
		          'ANY','ANY','ANY','ANY',true,false,false,false,false,false,'DRAFT')
	`, profileID, workspaceID, "PROFILE-"+codeSuffix, profileHash, profileContent); err != nil {
		t.Fatalf("insert certification profile: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1
	`, profileID); err != nil {
		t.Fatalf("finalize certification profile: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_certification(
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			decision, blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,'fixture-profile','1',$6,$7,'CERTIFIED','[]'::jsonb,'fixture',now())
	`, certificationID, workspaceID, versionID, qualityID, profileID, profileHash, profileContent); err != nil {
		t.Fatalf("insert dataset certification: %v", err)
	}

	specContent := annotationFixtureSpecContent()
	specHash := sha256Hex([]byte(specContent))
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_campaign(
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot
		) VALUES (
			$1,$2,$3,$4,$5,'gold-pilot','PROCESS',
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7
		)
	`, campaignID, workspaceID, versionID, certificationID, resourceID, specHash, specContent); err != nil {
		t.Fatalf("insert annotation campaign: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES ($1,$2,$3,'row:1',$4,$5,'annotator')
	`, taskID, workspaceID, campaignID, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert annotation task: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=1,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("c", 64), inputChecksum); err != nil {
		t.Fatalf("activate annotation campaign: %v", err)
	}

	payload := []byte("{\"label\":\"A\"}")
	payloadHash := sha256Hex(payload)
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version, created_at
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','obs-1-task','obs-1-annotation','1','obs:1',$5,$6,'fixture-v1',$7)
	`, resultID, workspaceID, campaignID, taskID, payload, payloadHash, resultCreatedAt); err != nil {
		t.Fatalf("insert annotation result: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE annotation_task SET status='REVIEWABLE', revision=2 WHERE id=$1
	`, taskID); err != nil {
		t.Fatalf("make task reviewable: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_review_attempt(
			id, workspace_id, campaign_id, task_id, reviewer_ref, expected_task_revision,
			review_action, reason, idempotency_key, request_fingerprint
		) VALUES ($1,$2,$3,$4,'reviewer',2,'ACCEPT','verified',$5,$6)
	`, attemptID, workspaceID, campaignID, taskID, "review-"+uuid.NewString(), strings.Repeat("e", 64)); err != nil {
		t.Fatalf("insert review attempt: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_review_decision(
			id, workspace_id, campaign_id, task_id, review_attempt_id,
			reviewed_result_id, selected_result_id, reviewer_ref, outcome, reason,
			expected_task_revision
		) VALUES ($1,$2,$3,$4,$5,$6,$6,'reviewer','ACCEPT','verified',2)
	`, decisionID, workspaceID, campaignID, taskID, attemptID, resultID); err != nil {
		t.Fatalf("insert review decision: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO annotation_review_attempt_outcome(id, attempt_id, outcome)
		VALUES ($1,$2,'SUCCEEDED')
	`, uuid.New(), attemptID); err != nil {
		t.Fatalf("insert review outcome: %v", err)
	}

	manifest := annotationFixtureManifest(
		workspaceID, campaignID, versionID, certificationID, resourceID,
		taskID, resultID, attemptID, decisionID, payloadHash, inputChecksum,
		builtAt, resultCreatedAt,
	)
	return annotationDBFixture{
		workspaceID: workspaceID, campaignID: campaignID, taskID: taskID,
		resultID: resultID, decisionID: decisionID, attemptID: attemptID, snapshotID: snapshotID,
		versionID: versionID, certificationID: certificationID, resourceID: resourceID,
		manifest: manifest, rootHash: sha256Hex(manifest), payloadHash: payloadHash,
		inputChecksum: inputChecksum, builtAt: builtAt, resultCreatedAt: resultCreatedAt,
	}
}

func insertAnnotationSnapshotAggregate(t *testing.T, ctx context.Context, tx pgx.Tx, fx annotationDBFixture) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_snapshot(
			id, workspace_id, campaign_id, manifest, manifest_hash_payload, root_hash,
			expected_task_count, expected_result_count, expected_decision_count, expected_output_count,
			created_at
		) VALUES ($1,$2,$3,$4,$5,$6,1,1,1,1,$7)
	`, fx.snapshotID, fx.workspaceID, fx.campaignID, fx.manifest, fx.manifest, fx.rootHash, fx.builtAt); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert annotation snapshot: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_snapshot_task(
			snapshot_id, task_id, source_item_ref, source_content_sha256, task_text_sha256
		) VALUES ($1,$2,'row:1',$3,$4)
	`, fx.snapshotID, fx.taskID, strings.Repeat("a", 64), strings.Repeat("b", 64)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert snapshot task: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_snapshot_result(
			snapshot_id, result_id, task_id, canonical_payload_sha256, author_ref
		) VALUES ($1,$2,$3,$4,'annotator')
	`, fx.snapshotID, fx.resultID, fx.taskID, fx.payloadHash); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert snapshot result: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_snapshot_decision(
			snapshot_id, decision_id, task_id, outcome, reviewed_result_id,
			selected_result_id, reviewer_ref, reason
		) VALUES ($1,$2,$3,'ACCEPT',$4,$4,'reviewer','verified')
	`, fx.snapshotID, fx.decisionID, fx.taskID, fx.resultID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert snapshot decision: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_snapshot_output(snapshot_id, task_id, selected_result_id)
		VALUES ($1,$2,$3)
	`, fx.snapshotID, fx.taskID, fx.resultID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert snapshot output: %v", err)
	}
}

func annotationFixtureSpecContent() string {
	return `{"kind":"single-label-v1","labels":["A","RACE","DEDUP","newer","late","too-late","FOREIGN"]}`
}

func annotationFixtureManifest(
	workspaceID, campaignID, versionID, certificationID, resourceID, taskID, resultID, attemptID, decisionID uuid.UUID,
	payloadHash, inputChecksum string,
	builtAt, resultCreatedAt time.Time,
) []byte {
	specContent := annotationFixtureSpecContent()
	specHash := sha256Hex([]byte(specContent))
	spec := func(ref string) map[string]any {
		return map[string]any{
			"ref": ref, "version": "1", "contentSha256": specHash, "contentSnapshot": specContent,
		}
	}
	manifest := map[string]any{
		"formatVersion":                    "annotation-snapshot-v1",
		"workspaceId":                      workspaceID.String(),
		"campaignId":                       campaignID.String(),
		"inputDatasetVersionId":            versionID.String(),
		"inputChecksumAlgorithm":           "SHA256",
		"inputChecksumSha256":              inputChecksum,
		"inputCertificationId":             certificationID.String(),
		"annotationContributionResourceId": resourceID.String(),
		"purpose":                          "gold-pilot",
		"action":                           "PROCESS",
		"consumerRef":                      "",
		"scopeType":                        "",
		"scopeRef":                         "",
		"taskManifestHash":                 strings.Repeat("c", 64),
		"schema":                           spec("schema"),
		"taxonomy":                         spec("taxonomy"),
		"rubric":                           spec("rubric"),
		"renderer":                         spec("renderer"),
		"reviewPolicy":                     spec("review"),
		"builtAtUnixMicros":                builtAt.UTC().UnixMicro(),
		"builtBy":                          "",
		"tasks": []map[string]any{{
			"id": taskID.String(), "sourceItemRef": "row:1",
			"sourceContentSha256": strings.Repeat("a", 64),
			"taskTextSha256":      strings.Repeat("b", 64),
			"primaryAnnotatorRef": "annotator",
		}},
		"results": []map[string]any{{
			"id": resultID.String(), "taskId": taskID.String(),
			"authorRef": "annotator", "providerBindingRef": "fixture-provider",
			"externalTaskId": "obs-1-task", "externalAnnotationId": "obs-1-annotation", "externalRevision": "1",
			"observationKey": "obs:1", "canonicalPayloadSha256": payloadHash,
			"normalizerVersion": "fixture-v1", "createdAtUnixMicros": resultCreatedAt.UTC().UnixMicro(),
			"createdBy": "",
		}},
		"decisions": []map[string]any{{
			"id": decisionID.String(), "taskId": taskID.String(),
			"reviewAttemptId": attemptID.String(), "outcome": "ACCEPT",
			"reviewedResultId": resultID.String(), "selectedResultId": resultID.String(),
			"reviewerRef": "reviewer", "reason": "verified", "expectedTaskRevision": int64(2),
		}},
		"outputs": []map[string]any{{
			"taskId": taskID.String(), "selectedResultId": resultID.String(),
		}},
	}
	encoded, _ := json.Marshal(manifest)
	return encoded
}

func assertAnnotationMutationRejected(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	query string,
	want string,
	args ...any,
) {
	t.Helper()
	_, err := pool.Exec(ctx, query, args...)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("mutation error = %v, want containing %q", err, want)
	}
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
