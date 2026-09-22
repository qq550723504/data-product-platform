package migration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestQualityAssessmentDownRefusesHistoricalFacts pins the 000019 downgrade
// guard: once quality history exists, the migration refuses to drop the rule
// snapshot columns instead of silently discarding evidence.
//
// It runs on a scratch database at version 19 so it keeps exercising the guard
// after later migrations land. The previous version of this test skipped itself
// as soon as the shared database moved past 19, which silently removed the
// coverage.
func TestQualityAssessmentDownRefusesHistoricalFacts(t *testing.T) {
	pool := scratchDatabase(t, 19)
	ctx := context.Background()

	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	assessmentID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Quality assessment down guard','CURATED')
	`, datasetID, workspaceID, "QUALITY-DOWN-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://quality-down/fixture.csv',
			'text/csv','SHA256',repeat('a',64),'{}'::jsonb,now())
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'quality/down-guard.yaml','1.0.0',
			'c4b903018effbb6d36545f03ec3a6513aa3d5b35f12f42a50c150dc4f1ea35dc',
			'legacy-quality-fixture','native-quality','1','PASS','{"dimensions":{}}'::jsonb)
	`, assessmentID, workspaceID, versionID); err != nil {
		t.Fatalf("insert quality assessment: %v", err)
	}

	err := tryApplyMigrationFile(t, pool, 19, "down")
	if err == nil || !strings.Contains(err.Error(), "refusing destructive rollback of quality assessment history") {
		t.Fatalf("quality assessment down error = %v, want historical-fact refusal", err)
	}

	// The refusal must be complete: the rule snapshot columns and the assessment
	// row are still there.
	var snapshotColumns int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name='quality_result'
		  AND column_name IN ('rule_set_content_sha256','rule_set_content','evaluator_name','evaluator_version')
	`).Scan(&snapshotColumns); err != nil {
		t.Fatalf("verify quality assessment columns: %v", err)
	}
	if snapshotColumns != 4 {
		t.Fatalf("quality assessment snapshot columns after refused down = %d, want 4", snapshotColumns)
	}
	var assessments int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quality_result WHERE id=$1`, assessmentID).Scan(&assessments); err != nil {
		t.Fatalf("verify quality assessment row: %v", err)
	}
	if assessments != 1 {
		t.Fatalf("quality assessments after refused down = %d, want 1", assessments)
	}
}
