package migration_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
)

func TestQualityAssessmentDownRefusesHistoricalFacts(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	var latest int64
	if err := pool.QueryRow(ctx, `SELECT COALESCE(max(version),0) FROM schema_migration`).Scan(&latest); err != nil {
		t.Fatalf("read latest migration: %v", err)
	}
	if latest != 19 {
		t.Skipf("quality assessment migration is not latest: %d", latest)
	}

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
			'legacy-quality-fixture','native-quality','1','PASS','{}'::jsonb)
	`, assessmentID, workspaceID, versionID); err != nil {
		t.Fatalf("insert quality assessment: %v", err)
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve migration test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../"))
	runner := migration.NewRunner(pool, filepath.Join(root, "migrations"))
	if err := runner.Down(ctx); err == nil || !strings.Contains(err.Error(), "refusing destructive rollback of quality assessment history") {
		t.Fatalf("quality assessment down error = %v, want historical-fact refusal", err)
	}
	if err := pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&latest); err != nil {
		t.Fatalf("verify migration version: %v", err)
	}
	if latest != 19 {
		t.Fatalf("latest migration after refused down = %d, want 19", latest)
	}
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
}
