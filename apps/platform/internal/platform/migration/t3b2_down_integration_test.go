package migration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

const t3b2DownLockSQL = `
LOCK TABLE
    execution_dependency_binding,
    execution_mapping_usage,
    execution_dependency_preparation,
    entity_resolution_output_decision,
    entity_match_job
IN ACCESS EXCLUSIVE MODE`

func TestT3B2DownGuardSerializesBusinessWrites(t *testing.T) {
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

	var dependencyTable string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('execution_dependency_binding')::text`).Scan(&dependencyTable); err != nil {
		t.Fatalf("check T3/B2 schema: %v", err)
	}
	if dependencyTable == "" {
		t.Skip("T3/B2 migration is not applied")
	}

	workspaceID := uuid.New()
	datasetID := uuid.New()
	workflowID := uuid.New()
	workflowVersionID := uuid.New()
	executionID := uuid.New()
	bindingID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, lifecycle_status, metadata)
		VALUES ($1,$2,$3,'T3/B2 migration lock dataset','CURATED','ACTIVE','{}'::jsonb)
	`, datasetID, workspaceID, "T3B2-MIGRATION-"+uuid.NewString()); err != nil {
		t.Fatalf("insert lock-test dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow (id, workspace_id, code, name)
		VALUES ($1,$2,$3,'T3/B2 migration lock workflow')
	`, workflowID, workspaceID, "T3B2-MIGRATION-"+uuid.NewString()); err != nil {
		t.Fatalf("insert lock-test workflow: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_version (id, workflow_id, version, definition_ref, definition_sha256, definition)
		VALUES ($1,$2,'1.0.0','t3b2-migration-test','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','{}'::jsonb)
	`, workflowVersionID, workflowID); err != nil {
		t.Fatalf("insert lock-test workflow version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO execution (id, workspace_id, workflow_version_id, output_dataset_id, target_period)
		VALUES ($1,$2,$3,$4,'2026-09')
	`, executionID, workspaceID, workflowVersionID, datasetID); err != nil {
		t.Fatalf("insert lock-test execution: %v", err)
	}

	businessTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin business transaction: %v", err)
	}
	if _, err := businessTx.Exec(ctx, `
		INSERT INTO execution_dependency_binding (
			id, execution_id, workspace_id, dependency_name, reference, version, content_sha256, content
		) VALUES ($1,$2,$3,'company_match_policy','t3b2-test','1.0.0',
		          'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855','')
	`, bindingID, executionID, workspaceID); err != nil {
		_ = businessTx.Rollback(ctx)
		t.Fatalf("insert uncommitted dependency fact: %v", err)
	}

	migrationTx, err := pool.Begin(ctx)
	if err != nil {
		_ = businessTx.Rollback(ctx)
		t.Fatalf("begin migration transaction: %v", err)
	}
	var migrationPID int
	if err := migrationTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&migrationPID); err != nil {
		_ = businessTx.Rollback(ctx)
		_ = migrationTx.Rollback(ctx)
		t.Fatalf("read migration backend pid: %v", err)
	}
	lockResult := make(chan error, 1)
	go func() {
		_, lockErr := migrationTx.Exec(ctx, t3b2DownLockSQL)
		lockResult <- lockErr
	}()

	deadline := time.Now().Add(5 * time.Second)
	pollTicker := time.NewTicker(10 * time.Millisecond)
	defer pollTicker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1
				FROM pg_locks l
				JOIN pg_class c ON c.oid=l.relation
				WHERE l.pid=$1 AND NOT l.granted
				  AND c.relname='execution_dependency_binding'
			)
		`, migrationPID).Scan(&waiting); err != nil {
			_ = businessTx.Rollback(ctx)
			_ = migrationTx.Rollback(ctx)
			t.Fatalf("observe migration lock: %v", err)
		}
		if waiting {
			break
		}
		select {
		case lockErr := <-lockResult:
			_ = businessTx.Rollback(ctx)
			_ = migrationTx.Rollback(ctx)
			t.Fatalf("migration lock completed before observing wait: %v", lockErr)
		default:
		}
		if time.Now().After(deadline) {
			_ = businessTx.Rollback(ctx)
			_ = migrationTx.Rollback(ctx)
			t.Fatal("migration lock did not appear in pg_locks")
		}
		<-pollTicker.C
	}

	if err := businessTx.Commit(ctx); err != nil {
		_ = migrationTx.Rollback(ctx)
		t.Fatalf("commit business dependency fact: %v", err)
	}
	if err := <-lockResult; err != nil {
		_ = migrationTx.Rollback(ctx)
		t.Fatalf("migration lock after business commit: %v", err)
	}
	_, err = migrationTx.Exec(ctx, `
		DO $$
		BEGIN
			IF EXISTS (SELECT 1 FROM execution_dependency_binding LIMIT 1) THEN
				RAISE EXCEPTION 'refusing destructive rollback of T3/B2 dependency facts';
			END IF;
		END
		$$
	`)
	if err == nil {
		_ = migrationTx.Rollback(ctx)
		t.Fatal("down guard accepted a committed dependency fact")
	}
	if !strings.Contains(err.Error(), "refusing destructive rollback") {
		_ = migrationTx.Rollback(ctx)
		t.Fatalf("down guard error = %v, want historical-fact refusal", err)
	}
	if err := migrationTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback migration transaction: %v", err)
	}

	var bindingCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_binding WHERE id=$1`, bindingID).Scan(&bindingCount); err != nil {
		t.Fatalf("verify dependency fact survived: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("dependency fact count = %d, want 1", bindingCount)
	}
	for _, table := range []string{"execution_dependency_preparation", "execution_dependency_binding", "execution_mapping_usage", "entity_resolution_output_decision"} {
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1)::text`, table).Scan(&dependencyTable); err != nil {
			t.Fatalf("verify table %s: %v", table, err)
		}
		if dependencyTable == "" {
			t.Fatalf("table %s disappeared after refused down", table)
		}
	}
	var policyColumnCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.columns
		WHERE table_name='entity_match_job' AND column_name IN ('policy_content','policy_content_sha256')
	`).Scan(&policyColumnCount); err != nil {
		t.Fatalf("verify policy snapshot columns: %v", err)
	}
	if policyColumnCount != 2 {
		t.Fatalf("policy snapshot columns = %d, want 2", policyColumnCount)
	}
}
