package migration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestQualityAssessmentAttemptEngineBackfillPreservesHistoricalIdentity(t *testing.T) {
	pool := scratchDatabase(t, 44)
	ctx := context.Background()

	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Attempt engine migration','CURATED')
	`, datasetID, workspaceID, "ATTEMPT-ENGINE-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://attempt-engine/fixture.csv',
			'text/csv','SHA256',repeat('a',64),'{}'::jsonb,now())
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}

	nativeV1AssessmentID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'quality/native-v1.yaml','1.0.0',
			'c4b903018effbb6d36545f03ec3a6513aa3d5b35f12f42a50c150dc4f1ea35dc',
			'legacy-quality-fixture','native-quality','1','PASS','{"dimensions":{}}'::jsonb)
	`, nativeV1AssessmentID, workspaceID, versionID); err != nil {
		t.Fatalf("insert native v1 assessment: %v", err)
	}

	nativeV1AttemptID := uuid.New()
	nativeFailedAttemptID := uuid.New()
	goldFailedAttemptID := uuid.New()
	for _, fixture := range []struct {
		id         uuid.UUID
		ruleSetRef string
	}{
		{nativeV1AttemptID, "quality/native-v1.yaml"},
		{nativeFailedAttemptID, "quality/native-failed.yaml"},
		{goldFailedAttemptID, "gold/quality/annotation-v1"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO quality_assessment_attempt (
				id, workspace_id, dataset_version_id, rule_set_ref, started_at, lease_expires_at
			) VALUES ($1,$2,$3,$4,now() - interval '1 hour',now() - interval '30 minutes')
		`, fixture.id, workspaceID, versionID, fixture.ruleSetRef); err != nil {
			t.Fatalf("insert attempt %s: %v", fixture.id, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_assessment_attempt_outcome (
			id, attempt_id, assessment_id, outcome, occurred_at
		) VALUES ($1,$2,$3,'SUCCEEDED',now())
	`, uuid.New(), nativeV1AttemptID, nativeV1AssessmentID); err != nil {
		t.Fatalf("insert native v1 outcome: %v", err)
	}
	for _, attemptID := range []uuid.UUID{nativeFailedAttemptID, goldFailedAttemptID} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO quality_assessment_attempt_outcome (
				id, attempt_id, outcome, error_message, occurred_at
			) VALUES ($1,$2,'FAILED','legacy failure',now())
		`, uuid.New(), attemptID); err != nil {
			t.Fatalf("insert failed outcome %s: %v", attemptID, err)
		}
	}

	if _, err := pool.Exec(ctx, `
		UPDATE quality_assessment_attempt SET rule_set_ref='mutated' WHERE id=$1
	`, nativeV1AttemptID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("pre-migration immutable trigger error = %v, want append-only refusal", err)
	}

	applyMigrationFile(t, pool, 45, "up")

	assertEngine := func(attemptID uuid.UUID, wantName, wantVersion string) {
		t.Helper()
		var name, version string
		if err := pool.QueryRow(ctx, `
			SELECT engine_name, engine_version
			FROM quality_assessment_attempt
			WHERE id=$1
		`, attemptID).Scan(&name, &version); err != nil {
			t.Fatalf("read attempt engine %s: %v", attemptID, err)
		}
		if name != wantName || version != wantVersion {
			t.Fatalf("attempt %s engine = %s@%s, want %s@%s", attemptID, name, version, wantName, wantVersion)
		}
	}
	assertEngine(nativeV1AttemptID, "native-quality", "1")
	assertEngine(nativeFailedAttemptID, "native-quality", "legacy-unknown")
	assertEngine(goldFailedAttemptID, "gold-quality", "1")

	if _, err := pool.Exec(ctx, `
		UPDATE quality_assessment_attempt SET engine_version='tampered' WHERE id=$1
	`, nativeV1AttemptID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("post-migration immutable trigger error = %v, want append-only refusal", err)
	}

	if err := tryApplyMigrationFile(t, pool, 45, "down"); err == nil ||
		!strings.Contains(err.Error(), "cannot drop quality assessment engine identity") {
		t.Fatalf("engine identity down migration error = %v, want historical-fact refusal", err)
	}
}
