package application_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
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
