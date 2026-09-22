package application_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type integrationStore struct{}

func (integrationStore) Put(_ context.Context, objectName string, reader io.Reader, size int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if int64(len(content)) != size {
		return "", fmt.Errorf("size mismatch: got %d want %d", len(content), size)
	}
	return "s3://workflow-test/" + objectName, nil
}

func TestExecutionPersistsFrozenInputsRetryAndTraceability(t *testing.T) {
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
		t.Fatalf("create routing table: %v", err)
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	createDataset := datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := datasetapp.NewUploadVersionService(txManager, datasetRepo, integrationStore{})

	workspaceID := uuid.New()
	inputDataset, err := createDataset.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "WF-INPUT-" + uuid.NewString(),
		Name:        "Workflow input",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "workflow-test",
	})
	if err != nil {
		t.Fatalf("create input dataset: %v", err)
	}
	inputV1, err := uploadDataset.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   inputDataset.ID,
		Filename:    "input.csv",
		ContentType: "text/csv",
		Content:     []byte("canonical_company_id,value\n1,10\n"),
		TraceID:     "workflow-test",
	})
	if err != nil {
		t.Fatalf("upload input v1: %v", err)
	}

	outputDataset, err := createDataset.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "WF-OUTPUT-" + uuid.NewString(),
		Name:        "Workflow output",
		DatasetType: datasetdomain.DatasetTypeCurated,
		TraceID:     "workflow-test",
	})
	if err != nil {
		t.Fatalf("create output dataset: %v", err)
	}

	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	versionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	workflowVersion, err := versionService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           "enterprise-activity-" + uuid.NewString(),
		Name:           "Enterprise Activity Test",
		Version:        "1.0.0",
		DefinitionRef:  "examples/enterprise-activity/workflow/workflow-v1.yaml",
		DefinitionYAML: []byte("apiVersion: dataprod.platform/v1alpha1\nkind: WorkflowDefinition\nmetadata:\n  name: test\n  version: 1.0.0\n"),
		TraceID:        "workflow-test",
	})
	if err != nil {
		t.Fatalf("create workflow version: %v", err)
	}

	executionService := workflowapp.NewExecutionService(txManager, workflowRepo)
	execution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersion.ID,
		OutputDatasetID:   outputDataset.ID,
		TargetPeriod:      "2025-03",
		Inputs: []domain.InputBinding{{
			Name:             "enterprise_standardized",
			DatasetVersionID: inputV1.ID,
		}},
		IdempotencyKey: "workflow-create-" + workspaceID.String(),
		TraceID:        "workflow-test",
	})
	if err != nil {
		t.Fatalf("create execution: %v", err)
	}
	replay, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersion.ID,
		OutputDatasetID:   outputDataset.ID,
		TargetPeriod:      "2025-03",
		Inputs:            []domain.InputBinding{{Name: " enterprise_standardized ", DatasetVersionID: inputV1.ID}},
		IdempotencyKey:    "workflow-create-" + workspaceID.String(),
		TraceID:           "workflow-test-network-retry",
	})
	if err != nil || replay.ID != execution.ID {
		t.Fatalf("same-key create replay = %s/%v, want original %s", replay.ID, err, execution.ID)
	}
	if _, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersion.ID,
		OutputDatasetID:   outputDataset.ID,
		TargetPeriod:      "2025-04",
		Inputs:            []domain.InputBinding{{Name: "enterprise_standardized", DatasetVersionID: inputV1.ID}},
		IdempotencyKey:    "workflow-create-" + workspaceID.String(),
	}); !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("same-key changed create request error = %v, want idempotency conflict", err)
	}
	createKey := "workflow-create-" + workspaceID.String()
	var beforeFingerprint string
	var beforeObjectID, beforeResultRef uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT request_fingerprint, object_id, result_ref
		FROM command_idempotency
		WHERE workspace_id=$1 AND command_type='WORKFLOW.CREATE_EXECUTION' AND idempotency_key=$2
	`, workspaceID, createKey).Scan(&beforeFingerprint, &beforeObjectID, &beforeResultRef); err != nil {
		t.Fatalf("read execution fingerprint before migration down: %v", err)
	}
	if beforeFingerprint == "" || beforeObjectID != execution.ID || beforeResultRef != execution.ID {
		t.Fatalf("unexpected idempotency record before migration down: fingerprint=%q object=%s result=%s", beforeFingerprint, beforeObjectID, beforeResultRef)
	}
	beforeInputCount := countExecutionFact(t, ctx, pool, `SELECT count(*) FROM execution_input WHERE execution_id=$1`, execution.ID)
	beforeEventCount := countExecutionFact(t, ctx, pool, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ExecutionQueued'`, execution.ID)
	beforeAuditCount := countExecutionFact(t, ctx, pool, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='EXECUTION_QUEUED'`, execution.ID)
	downSQL, err := os.ReadFile(executionFingerprintMigrationPath(t))
	if err != nil {
		t.Fatalf("read execution fingerprint down migration: %v", err)
	}
	downTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin execution fingerprint down migration: %v", err)
	}
	_, downErr := downTx.Exec(ctx, string(downSQL))
	if downErr == nil {
		_ = downTx.Rollback(ctx)
		t.Fatal("000017 down succeeded while execution fingerprints exist")
	}
	_ = downTx.Rollback(ctx)

	var afterFingerprint string
	var afterObjectID, afterResultRef uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT request_fingerprint, object_id, result_ref
		FROM command_idempotency
		WHERE workspace_id=$1 AND command_type='WORKFLOW.CREATE_EXECUTION' AND idempotency_key=$2
	`, workspaceID, createKey).Scan(&afterFingerprint, &afterObjectID, &afterResultRef); err != nil {
		t.Fatalf("read execution fingerprint after rejected migration down: %v", err)
	}
	if afterFingerprint != beforeFingerprint || afterObjectID != beforeObjectID || afterResultRef != beforeResultRef {
		t.Fatalf("idempotency record changed after rejected migration down: before=(%q %s %s) after=(%q %s %s)", beforeFingerprint, beforeObjectID, beforeResultRef, afterFingerprint, afterObjectID, afterResultRef)
	}
	if got := countExecutionFact(t, ctx, pool, `SELECT count(*) FROM execution_input WHERE execution_id=$1`, execution.ID); got != beforeInputCount {
		t.Fatalf("input bindings after rejected migration down = %d, want %d", got, beforeInputCount)
	}
	if got := countExecutionFact(t, ctx, pool, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ExecutionQueued'`, execution.ID); got != beforeEventCount {
		t.Fatalf("queue events after rejected migration down = %d, want %d", got, beforeEventCount)
	}
	if got := countExecutionFact(t, ctx, pool, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='EXECUTION_QUEUED'`, execution.ID); got != beforeAuditCount {
		t.Fatalf("audit events after rejected migration down = %d, want %d", got, beforeAuditCount)
	}
	postDownReplay, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersion.ID,
		OutputDatasetID:   outputDataset.ID,
		TargetPeriod:      "2025-03",
		Inputs:            []domain.InputBinding{{Name: "enterprise_standardized", DatasetVersionID: inputV1.ID}},
		IdempotencyKey:    createKey,
		TraceID:           "workflow-after-rejected-down",
	})
	if err != nil || postDownReplay.ID != execution.ID {
		t.Fatalf("same-key replay after rejected migration down = %s/%v, want original %s", postDownReplay.ID, err, execution.ID)
	}

	concurrentKey := "workflow-concurrent-" + workspaceID.String()
	results := make(chan domain.Execution, 2)
	errorsCh := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			created, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
				WorkspaceID:       workspaceID,
				WorkflowVersionID: workflowVersion.ID,
				OutputDatasetID:   outputDataset.ID,
				TargetPeriod:      "2025-05",
				Inputs:            []domain.InputBinding{{Name: "enterprise_standardized", DatasetVersionID: inputV1.ID}},
				IdempotencyKey:    concurrentKey,
			})
			if err != nil {
				errorsCh <- err
				return
			}
			results <- created
		}()
	}
	group.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent same-key create: %v", err)
	}
	var concurrentIDs []uuid.UUID
	for created := range results {
		concurrentIDs = append(concurrentIDs, created.ID)
	}
	if len(concurrentIDs) != 2 || concurrentIDs[0] != concurrentIDs[1] {
		t.Fatalf("concurrent create IDs = %v, want two identical IDs", concurrentIDs)
	}
	var concurrentFacts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution WHERE id=$1`, concurrentIDs[0]).Scan(&concurrentFacts); err != nil {
		t.Fatalf("count concurrent execution: %v", err)
	}
	if concurrentFacts != 1 {
		t.Fatalf("concurrent execution facts = %d, want 1", concurrentFacts)
	}
	var concurrentEvents, concurrentAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ExecutionQueued'`, concurrentIDs[0]).Scan(&concurrentEvents); err != nil {
		t.Fatalf("count concurrent outbox events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='EXECUTION_QUEUED'`, concurrentIDs[0]).Scan(&concurrentAudits); err != nil {
		t.Fatalf("count concurrent audits: %v", err)
	}
	if concurrentEvents != 1 || concurrentAudits != 1 {
		t.Fatalf("concurrent facts = outbox %d audit %d, want 1/1", concurrentEvents, concurrentAudits)
	}
	if _, err := executionService.Start(ctx, execution.ID, "native-attempt-1", "workflow-test"); err != nil {
		t.Fatalf("start execution: %v", err)
	}
	failed, err := executionService.Fail(ctx, execution.ID, "POC_FAILURE", "intentional test failure", map[string]any{"phase": "test"}, "workflow-test")
	if err != nil {
		t.Fatalf("fail execution: %v", err)
	}
	if failed.Status != domain.ExecutionFailed {
		t.Fatalf("failed status = %s", failed.Status)
	}

	// A newer source version supersedes V1. Retry must still bind to the exact immutable V1.
	if _, err := uploadDataset.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   inputDataset.ID,
		Filename:    "input.csv",
		ContentType: "text/csv",
		Content:     []byte("canonical_company_id,value\n1,11\n"),
		TraceID:     "workflow-test",
	}); err != nil {
		t.Fatalf("upload input v2: %v", err)
	}
	storedInputV1, err := datasetRepo.GetVersion(ctx, inputV1.ID)
	if err != nil {
		t.Fatalf("read input v1: %v", err)
	}
	if storedInputV1.Status != datasetdomain.VersionSuperseded {
		t.Fatalf("input v1 status = %s, want SUPERSEDED", storedInputV1.Status)
	}

	retry, err := executionService.Retry(ctx, workflowapp.RetryExecutionCommand{ExecutionID: execution.ID, IdempotencyKey: "workflow-retry-" + execution.ID.String(), TraceID: "workflow-test"})
	if err != nil {
		t.Fatalf("retry execution using superseded frozen input: %v", err)
	}
	if retry.Attempt != 2 || retry.RetryOfExecutionID == nil || *retry.RetryOfExecutionID != execution.ID {
		t.Fatalf("unexpected retry lineage: %#v", retry)
	}
	retryReplay, err := executionService.Retry(ctx, workflowapp.RetryExecutionCommand{ExecutionID: execution.ID, IdempotencyKey: "workflow-retry-" + execution.ID.String(), TraceID: "workflow-network-retry"})
	if err != nil || retryReplay.ID != retry.ID {
		t.Fatalf("same-key retry replay = %s/%v, want original retry %s", retryReplay.ID, err, retry.ID)
	}
	var retryFacts, retryEvents, retryAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution WHERE id=$1`, retry.ID).Scan(&retryFacts); err != nil {
		t.Fatalf("count retry execution: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ExecutionRetried'`, retry.ID).Scan(&retryEvents); err != nil {
		t.Fatalf("count retry outbox events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='EXECUTION_RETRIED'`, retry.ID).Scan(&retryAudits); err != nil {
		t.Fatalf("count retry audits: %v", err)
	}
	if retryFacts != 1 || retryEvents != 1 || retryAudits != 1 {
		t.Fatalf("retry facts = execution %d event %d audit %d, want 1/1/1", retryFacts, retryEvents, retryAudits)
	}
	if len(retry.Inputs) != 1 || retry.Inputs[0].DatasetVersionID != inputV1.ID {
		t.Fatalf("retry inputs = %#v, want frozen v1 %s", retry.Inputs, inputV1.ID)
	}
	if _, err := executionService.Start(ctx, retry.ID, "native-attempt-2", "workflow-test"); err != nil {
		t.Fatalf("start retry: %v", err)
	}
	outputVersion, err := uploadDataset.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   outputDataset.ID,
		Filename:    "curated.csv",
		ContentType: "text/csv",
		Content:     []byte("canonical_company_id,activity_score\n1,96.01\n"),
		TraceID:     "workflow-test",
	})
	if err != nil {
		t.Fatalf("upload output: %v", err)
	}
	succeeded, err := executionService.Succeed(ctx, retry.ID, outputVersion.ID, map[string]any{"rows": 1}, "workflow-test")
	if err != nil {
		t.Fatalf("succeed retry: %v", err)
	}
	if succeeded.Status != domain.ExecutionSucceeded || succeeded.OutputDatasetVersionID == nil || *succeeded.OutputDatasetVersionID != outputVersion.ID {
		t.Fatalf("unexpected success state: %#v", succeeded)
	}

	var costCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_event WHERE execution_id=$1 AND cost_type='PROCESSING_EXECUTION'`, retry.ID).Scan(&costCount); err != nil {
		t.Fatalf("count cost events: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("processing cost events = %d, want 1", costCount)
	}
	var evidenceCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM evidence_relation
		WHERE object_type='EXECUTION' AND object_id=$1 AND relation_type='SUPPORTS'
	`, retry.ID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count execution evidence: %v", err)
	}
	if evidenceCount != 1 {
		t.Fatalf("execution evidence relations = %d, want 1", evidenceCount)
	}

	if _, err := pool.Exec(ctx, `UPDATE workflow_version SET definition_sha256='tampered' WHERE id=$1`, workflowVersion.ID); err == nil {
		t.Fatal("expected workflow_version mutation to be rejected")
	}
}

func TestExecutionFingerprintDownSerializesWithUncommittedBusinessWrite(t *testing.T) {
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

	workspaceID, objectID := uuid.New(), uuid.New()
	commandType := "WORKFLOW.CREATE_EXECUTION"
	key := "migration-lock-" + uuid.NewString()
	fingerprint := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	businessTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin business transaction: %v", err)
	}
	defer func() { _ = businessTx.Rollback(context.Background()) }()
	if _, err := businessTx.Exec(ctx, `
		INSERT INTO command_idempotency (
			workspace_id, command_type, idempotency_key, object_id, result_ref, request_fingerprint
		) VALUES ($1, $2, $3, $4, $4, $5)
	`, workspaceID, commandType, key, objectID, fingerprint); err != nil {
		t.Fatalf("insert uncommitted execution fingerprint: %v", err)
	}

	downTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin migration down transaction: %v", err)
	}
	defer func() { _ = downTx.Rollback(context.Background()) }()
	var downPID int
	if err := downTx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&downPID); err != nil {
		t.Fatalf("read down transaction backend pid: %v", err)
	}
	downSQL, err := os.ReadFile(executionFingerprintMigrationPath(t))
	if err != nil {
		t.Fatalf("read execution fingerprint down migration: %v", err)
	}
	downResult := make(chan error, 1)
	go func() {
		_, execErr := downTx.Exec(ctx, string(downSQL))
		downResult <- execErr
	}()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	lockObserved := false
	for !lockObserved {
		if err := pool.QueryRow(waitCtx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_locks
				WHERE pid=$1
				  AND relation='command_idempotency'::regclass
				  AND mode='AccessExclusiveLock'
				  AND NOT granted
			)
		`, downPID).Scan(&lockObserved); err != nil {
			t.Fatalf("observe migration table lock: %v", err)
		}
		if lockObserved {
			break
		}
		select {
		case <-waitCtx.Done():
			t.Fatal("migration down did not wait for the business write's table lock")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if err := businessTx.Commit(ctx); err != nil {
		t.Fatalf("commit business fingerprint after down started waiting: %v", err)
	}
	if err := <-downResult; err == nil {
		t.Fatal("000017 down succeeded after the concurrent fingerprint committed")
	}
	if err := downTx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback rejected migration down: %v", err)
	}

	var storedFingerprint string
	if err := pool.QueryRow(ctx, `
		SELECT request_fingerprint
		FROM command_idempotency
		WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3
	`, workspaceID, commandType, key).Scan(&storedFingerprint); err != nil {
		t.Fatalf("read fingerprint after rejected concurrent down: %v", err)
	}
	if storedFingerprint != fingerprint {
		t.Fatalf("fingerprint after rejected concurrent down = %q, want %q", storedFingerprint, fingerprint)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM command_idempotency WHERE workspace_id=$1 AND idempotency_key=$2`, workspaceID, key); err != nil {
		t.Fatalf("cleanup concurrent fingerprint: %v", err)
	}
}

func countExecutionFact(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, executionID uuid.UUID) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, query, executionID).Scan(&count); err != nil {
		t.Fatalf("count execution fact: %v", err)
	}
	return count
}

func executionFingerprintMigrationPath(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate execution integration test source")
	}
	return filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "..", "migrations", "000017_execution_command_fingerprint.down.sql")
}
