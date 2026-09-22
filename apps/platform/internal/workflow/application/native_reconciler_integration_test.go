package application

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

func TestNativeReconcilerFinalizesReadyOutputAfterCrash(t *testing.T) {
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

	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("build routing table: %v", err)
	}
	outbox.ConfigureAppendObligation(router)

	workspaceID := uuid.New()
	workflowID := uuid.New()
	workflowVersionID := uuid.New()
	datasetID := uuid.New()
	executionID := uuid.New()
	outputVersionID := uuid.New()
	startedAt := time.Now().UTC().Add(-time.Hour)

	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow (id, workspace_id, code, name)
		VALUES ($1,$2,$3,$4)
	`, workflowID, workspaceID, "native-recovery-"+uuid.NewString(), "Native recovery workflow"); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_version (id, workflow_id, version, definition_ref, definition_sha256, definition)
		VALUES ($1,$2,'1.0.0','test/native-recovery','sha256-test','{}'::jsonb)
	`, workflowVersionID, workflowID); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,$4,'CURATED')
	`, datasetID, workspaceID, "native-recovery-output-"+uuid.NewString(), "Native recovery output"); err != nil {
		t.Fatalf("insert output dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO execution (
			id, workspace_id, workflow_version_id, output_dataset_id, target_period,
			status, attempt, engine_type, engine_execution_id, metrics, created_at, started_at
		) VALUES ($1,$2,$3,$4,'2026-09','RUNNING',1,'NATIVE',$5,'{}'::jsonb,$6,$6)
	`, executionID, workspaceID, workflowVersionID, datasetID, "native:"+executionID.String(), startedAt); err != nil {
		t.Fatalf("insert running execution: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri, content_type,
			row_count, byte_size, checksum_algorithm, checksum_value,
			generated_by_execution_id, metadata, created_at, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE',$3,'text/csv',1,4,'SHA256','deadbeef',$4,'{}'::jsonb,$5,$5)
	`, outputVersionID, datasetID, "s3://test/native-recovery.csv", executionID, startedAt.Add(time.Minute)); err != nil {
		t.Fatalf("insert ready output: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE dataset SET current_version_id=$2 WHERE id=$1`, datasetID, outputVersionID); err != nil {
		t.Fatalf("set current output version: %v", err)
	}

	txManager := transaction.NewManager(pool)
	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	service := NewExecutionService(txManager, workflowRepo)
	engine := &fakeNativeEngine{err: errEngineMustNotRun{}}
	reconciler := NewNativeReconciler(txManager, service, workflowRepo, datasetRepo, engine)
	reconciler.leaseTTL = time.Second

	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf("first reconciliation: %v", err)
	}
	if engine.calls != 0 {
		t.Fatalf("existing READY output must be adopted without re-execution, calls=%d", engine.calls)
	}
	stored, err := workflowRepo.GetExecution(ctx, executionID)
	if err != nil {
		t.Fatalf("read recovered execution: %v", err)
	}
	if stored.Status != domain.ExecutionSucceeded || stored.OutputDatasetVersionID == nil || *stored.OutputDatasetVersionID != outputVersionID {
		t.Fatalf("recovered execution = status %s output %v, want SUCCEEDED/%s", stored.Status, stored.OutputDatasetVersionID, outputVersionID)
	}

	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf("second reconciliation: %v", err)
	}
	var recoveryEvents, recoveryEvidence int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_event
		WHERE aggregate_id=$1 AND event_type='ExecutionRecoveryStarted'
	`, executionID).Scan(&recoveryEvents); err != nil {
		t.Fatalf("count recovery events: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM evidence
		WHERE source_type='EXECUTION' AND source_id=$1 AND evidence_type='EXECUTION_RECOVERY'
	`, executionID).Scan(&recoveryEvidence); err != nil {
		t.Fatalf("count recovery evidence: %v", err)
	}
	if recoveryEvents != 1 || recoveryEvidence != 1 {
		t.Fatalf("recovery facts duplicated: events=%d evidence=%d", recoveryEvents, recoveryEvidence)
	}
}


func TestNativeReconcilerAccountsFailedRecoveryInvocation(t *testing.T) {
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

	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("build routing table: %v", err)
	}
	outbox.ConfigureAppendObligation(router)

	workspaceID := uuid.New()
	workflowID := uuid.New()
	workflowVersionID := uuid.New()
	datasetID := uuid.New()
	executionID := uuid.New()
	startedAt := time.Now().UTC().Add(-time.Hour)

	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow (id, workspace_id, code, name)
		VALUES ($1,$2,$3,$4)
	`, workflowID, workspaceID, "native-recovery-cost-"+uuid.NewString(), "Native recovery cost workflow"); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_version (id, workflow_id, version, definition_ref, definition_sha256, definition)
		VALUES ($1,$2,'1.0.0','test/native-recovery-cost','sha256-test','{}'::jsonb)
	`, workflowVersionID, workflowID); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,$4,'CURATED')
	`, datasetID, workspaceID, "native-recovery-cost-output-"+uuid.NewString(), "Native recovery cost output"); err != nil {
		t.Fatalf("insert output dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO execution (
			id, workspace_id, workflow_version_id, output_dataset_id, target_period,
			status, attempt, engine_type, engine_execution_id, metrics, created_at, started_at
		) VALUES ($1,$2,$3,$4,'2026-09','RUNNING',1,'NATIVE',$5,'{}'::jsonb,$6,$6)
	`, executionID, workspaceID, workflowVersionID, datasetID, "native:"+executionID.String(), startedAt); err != nil {
		t.Fatalf("insert running execution: %v", err)
	}

	txManager := transaction.NewManager(pool)
	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	service := NewExecutionService(txManager, workflowRepo)
	engine := &fakeNativeEngine{err: errors.New("synthetic recovery compute failure")}
	reconciler := NewNativeReconciler(txManager, service, workflowRepo, datasetRepo, engine)
	reconciler.leaseTTL = time.Second

	if err := reconciler.RunOnce(ctx); err != nil {
		t.Fatalf("reconcile failed native execution: %v", err)
	}
	stored, err := workflowRepo.GetExecution(ctx, executionID)
	if err != nil {
		t.Fatalf("read failed recovered execution: %v", err)
	}
	if stored.Status != domain.ExecutionFailed || stored.ErrorCode != "NATIVE_RECOVERY_FAILED" {
		t.Fatalf("recovered failure = status %s code %q, want FAILED/NATIVE_RECOVERY_FAILED", stored.Status, stored.ErrorCode)
	}

	var invocationCosts int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM cost_event
		WHERE execution_id=$1 AND cost_type='NATIVE_ENGINE_INVOCATION'
	`, executionID).Scan(&invocationCosts); err != nil {
		t.Fatalf("count recovery invocation cost: %v", err)
	}
	if invocationCosts != 1 {
		t.Fatalf("recovery invocation costs = %d, want 1", invocationCosts)
	}
}


type errEngineMustNotRun struct{}

func (errEngineMustNotRun) Error() string { return "native engine must not execute when READY output already exists" }
