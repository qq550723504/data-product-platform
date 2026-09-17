package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	datasethttp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/transport/http"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	entityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflowhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/http"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

// Only this focused HTTP/DB test uses an instrumented memory store. The separate
// live-browser suite continues to exercise actual MinIO, API and standalone Next.
type boundaryStore struct {
	*memoryStore
	reads  atomic.Int64
	writes atomic.Int64
}

func (s *boundaryStore) Get(ctx context.Context, uri string) (io.ReadCloser, error) {
	s.reads.Add(1)
	return s.memoryStore.Get(ctx, uri)
}

func (s *boundaryStore) Put(ctx context.Context, name string, r io.Reader, size int64, contentType string) (string, error) {
	s.writes.Add(1)
	return s.memoryStore.Put(ctx, name, r, size, contentType)
}

// countingQueue records dispatch so rejected commands can be proven to enqueue nothing.
type countingQueue struct {
	enqueued []uuid.UUID
}

func (q *countingQueue) EnqueueExecution(_ context.Context, executionID uuid.UUID) error {
	q.enqueued = append(q.enqueued, executionID)
	return nil
}

// countingNativeEngine proves a quarantined execution never reaches the engine.
type countingNativeEngine struct {
	calls atomic.Int64
}

func (e *countingNativeEngine) Execute(context.Context, workflowapp.ProcessingRequest) (workflowapp.ProcessingResult, error) {
	e.calls.Add(1)
	return workflowapp.ProcessingResult{}, nil
}

// This checks cross-object ownership, not caller authentication. Configured
// workspace/actor IDs are not a production authorization boundary.
func TestWorkspaceOwnershipHTTPRejectsWithoutSideEffects(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	liveOK(t, err, "open boundary test database")
	defer pool.Close()
	tx := transaction.NewManager(pool)
	resources := resourceinfra.NewPostgresRepository()
	datasets := datasetinfra.NewPostgresRepository(pool)
	entities := entityinfra.NewPostgresRepository(pool)
	store := &boundaryStore{memoryStore: newMemoryStore()}
	create := datasetapp.NewCreateDatasetService(tx, datasets, resources)
	upload := datasetapp.NewUploadVersionService(tx, datasets, store)
	invalidate := datasetapp.NewInvalidateVersionService(tx, datasets)
	match := entityapp.NewMatchService(repoPath(t, "industry-packs"), tx, entities, datasets, upload, store)
	datasetHandler := datasethttp.NewHandler(create, upload, datasetapp.NewInvalidateVersionService(tx, datasets), datasets)
	entityHandler := entityhttp.NewHandler(match, entities)
	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	workflowVersions := workflowapp.NewWorkflowVersionService(tx, workflowRepo)
	queue := &countingQueue{}
	executions := workflowapp.NewExecutionService(tx, workflowRepo, queue)
	workflowHandler := workflowhttp.NewHandler(workflowVersions, executions, workflowRepo)
	server := httptest.NewServer(httpserver.NewMux(datasetHandler.Register, entityHandler.Register, workflowHandler.Register))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Second
	owner, foreign, actor := uuid.New(), uuid.New(), uuid.New()
	workspaces := []uuid.UUID{owner, foreign}
	source := mustCreateResource(t, ctx, resourceapp.NewCreateService(tx, resources), owner, "BOUNDARY", "Synthetic boundary source", &actor, "boundary-setup")
	deletedSource := mustCreateResource(t, ctx, resourceapp.NewCreateService(tx, resources), owner, "BOUNDARY-DELETED", "Synthetic unavailable source", &actor, "boundary-setup")
	liveOK(t, func() error {
		_, err := pool.Exec(ctx, `UPDATE data_resource SET deleted_at=now() WHERE id=$1`, deletedSource.ID)
		return err
	}(), "soft-delete synthetic source fixture")
	raw := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-RAW", "RAW", datasetdomain.DatasetTypeRaw, &source.ID, &actor, "boundary-setup")
	standardized := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-STD", "STD", datasetdomain.DatasetTypeStandardized, nil, &actor, "boundary-setup")
	foreignOutput := mustCreateDataset(t, ctx, create, foreign, "BOUNDARY-FOREIGN", "Foreign STD", datasetdomain.DatasetTypeStandardized, nil, &actor, "boundary-setup")
	curated := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-CURATED", "Curated", datasetdomain.DatasetTypeCurated, nil, &actor, "boundary-setup")
	product := mustCreateDataset(t, ctx, create, owner, "BOUNDARY-PRODUCT", "Product", datasetdomain.DatasetTypeProduct, nil, &actor, "boundary-setup")
	filename := "boundary-" + uuid.NewString() + ".csv"
	version := mustUploadCSV(t, ctx, upload, raw.ID, filename, []byte("source_company_id,company_name\nTEST-1,Synthetic boundary company\n"), nil, nil, &actor, "boundary-setup")

	// A second, fully isolated workspace supplies foreign workflow/input/output references.
	foreignSource := mustCreateResource(t, ctx, resourceapp.NewCreateService(tx, resources), foreign, "BOUNDARY-FOREIGN", "Foreign source", &actor, "boundary-setup")
	foreignRaw := mustCreateDataset(t, ctx, create, foreign, "BOUNDARY-FOREIGN-RAW", "Foreign RAW", datasetdomain.DatasetTypeRaw, &foreignSource.ID, &actor, "boundary-setup")
	foreignVersion := mustUploadCSV(t, ctx, upload, foreignRaw.ID, "foreign-"+uuid.NewString()+".csv", []byte("source_company_id,company_name\nF-1,Foreign boundary company\n"), nil, nil, &actor, "boundary-setup")
	ownerWorkflow := mustCreateBoundaryWorkflowVersion(t, ctx, workflowVersions, owner, &actor)
	foreignWorkflow := mustCreateBoundaryWorkflowVersion(t, ctx, workflowVersions, foreign, &actor)

	post := func(t *testing.T, route string, body map[string]any) (int, []byte) {
		t.Helper()
		encoded, err := json.Marshal(body)
		liveOK(t, err, "encode HTTP boundary command")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+route, bytes.NewReader(encoded))
		liveOK(t, err, "create HTTP request")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Actor-ID", actor.String())
		response, err := client.Do(req)
		liveOK(t, err, "send actual handler request")
		defer response.Body.Close()
		content, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		liveOK(t, err, "read HTTP response")
		return response.StatusCode, content
	}
	// Compare persisted facts, not just a success/error status. The filter isolates
	// this test from other packages sharing TEST_POSTGRES_DSN during go test ./....
	snapshot := func(t *testing.T) string {
		t.Helper()
		var value string
		liveOK(t, pool.QueryRow(ctx, `SELECT jsonb_build_object(
			'datasets', (SELECT jsonb_agg(to_jsonb(d) ORDER BY d.id) FROM dataset d WHERE workspace_id=ANY($1::uuid[])),
			'versions', (SELECT jsonb_agg(to_jsonb(v) ORDER BY v.id) FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=ANY($1::uuid[])),
			'types', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM entity_type e WHERE workspace_id=ANY($1::uuid[])),
			'entities', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM entity e WHERE workspace_id=ANY($1::uuid[])),
			'jobs', (SELECT jsonb_agg(to_jsonb(j) ORDER BY j.id) FROM entity_match_job j WHERE workspace_id=ANY($1::uuid[])),
			'audit', (SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM audit_event a WHERE workspace_id=ANY($1::uuid[])),
			'evidence', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM evidence e WHERE workspace_id=ANY($1::uuid[])),
			'workflows', (SELECT jsonb_agg(to_jsonb(w) ORDER BY w.id) FROM workflow w WHERE w.workspace_id=ANY($1::uuid[])),
			'workflowVersions', (SELECT jsonb_agg(to_jsonb(wv) ORDER BY wv.id) FROM workflow_version wv JOIN workflow w ON w.id=wv.workflow_id WHERE w.workspace_id=ANY($1::uuid[])),
			'executions', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM execution e WHERE e.workspace_id=ANY($1::uuid[])),
			'executionInputs', (SELECT jsonb_agg(to_jsonb(ei) ORDER BY ei.execution_id, ei.input_name) FROM execution_input ei JOIN execution e ON e.id=ei.execution_id WHERE e.workspace_id=ANY($1::uuid[])),
			'cost', (SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM cost_event c WHERE c.workspace_id=ANY($1::uuid[])),
			'outbox', (SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM outbox_event o WHERE o.payload->>'workspaceId'=ANY($2::text[]) OR o.aggregate_id IN (SELECT id FROM execution WHERE workspace_id=ANY($1::uuid[])))
		)::text`, workspaces, []string{owner.String(), foreign.String()}).Scan(&value), "snapshot isolated facts")
		return value
	}
	jobBody := func(workspace, output uuid.UUID) map[string]any {
		return map[string]any{"workspaceId": workspace, "inputDatasetVersionId": version.ID, "outputDatasetId": output, "sourceType": "CSV", "sourceRef": filename, "sourceRole": "ANCHOR", "policyRef": companyPolicyRef}
	}
	executionBody := func(workspace, workflowVersion, output, inputVersion uuid.UUID) map[string]any {
		return map[string]any{
			"workspaceId":       workspace,
			"workflowVersionId": workflowVersion,
			"outputDatasetId":   output,
			"targetPeriod":      "2025-03",
			"inputs":            []map[string]any{{"name": "boundary_input", "datasetVersionId": inputVersion}},
		}
	}
	cases := []struct {
		name, route, code string
		body              map[string]any
	}{
		{"missing resource", "/api/v1/datasets", "SOURCE_RESOURCE_NOT_FOUND", map[string]any{"workspaceId": owner, "code": "BOUNDARY-MISSING", "name": "Rejected", "datasetType": "RAW", "sourceResourceId": uuid.New()}},
		{"soft-deleted resource", "/api/v1/datasets", "SOURCE_RESOURCE_NOT_FOUND", map[string]any{"workspaceId": owner, "code": "BOUNDARY-DELETED-SOURCE", "name": "Rejected", "datasetType": "RAW", "sourceResourceId": deletedSource.ID}},
		{"foreign resource", "/api/v1/datasets", "SOURCE_RESOURCE_WORKSPACE_MISMATCH", map[string]any{"workspaceId": foreign, "code": "BOUNDARY-BAD", "name": "Rejected", "datasetType": "RAW", "sourceResourceId": source.ID}},
		{"foreign input", "/api/v1/entity-match-jobs", "DATASET_WORKSPACE_MISMATCH", jobBody(foreign, foreignOutput.ID)},
		{"foreign output", "/api/v1/entity-match-jobs", "DATASET_WORKSPACE_MISMATCH", jobBody(owner, foreignOutput.ID)},
		{"RAW output", "/api/v1/entity-match-jobs", "OUTPUT_DATASET_TYPE_INVALID", jobBody(owner, raw.ID)},
		{"CURATED output", "/api/v1/entity-match-jobs", "OUTPUT_DATASET_TYPE_INVALID", jobBody(owner, curated.ID)},
		{"PRODUCT output", "/api/v1/entity-match-jobs", "OUTPUT_DATASET_TYPE_INVALID", jobBody(owner, product.ID)},
		{"foreign execution workflow", "/api/v1/executions", "EXECUTION_WORKSPACE_MISMATCH", executionBody(owner, foreignWorkflow.ID, standardized.ID, version.ID)},
		{"foreign execution input", "/api/v1/executions", "EXECUTION_WORKSPACE_MISMATCH", executionBody(owner, ownerWorkflow.ID, standardized.ID, foreignVersion.ID)},
		{"foreign execution output", "/api/v1/executions", "EXECUTION_WORKSPACE_MISMATCH", executionBody(owner, ownerWorkflow.ID, foreignOutput.ID, version.ID)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, reads, writes := snapshot(t), store.reads.Load(), store.writes.Load()
			status, response := post(t, tc.route, tc.body)
			var envelope httpserver.ErrorEnvelope
			liveOK(t, json.Unmarshal(response, &envelope), "decode error envelope")
			if status != http.StatusBadRequest || envelope.Code != tc.code {
				t.Fatalf("status=%d response=%s, want 400/%s", status, response, tc.code)
			}
			if strings.Contains(string(response), owner.String()) || strings.Contains(string(response), foreign.String()) {
				t.Fatalf("error discloses a workspace identity: %s", response)
			}
			if after := snapshot(t); after != before {
				t.Fatal("rejected HTTP command changed persisted facts")
			}
			if store.reads.Load() != reads || store.writes.Load() != writes {
				t.Fatal("rejected command accessed object storage")
			}
		})
	}
	t.Run("same workspace resource accepted", func(t *testing.T) {
		status, response := post(t, "/api/v1/datasets", map[string]any{"workspaceId": owner, "code": "BOUNDARY-GOOD", "name": "Accepted", "datasetType": "RAW", "sourceResourceId": source.ID})
		if status != http.StatusCreated {
			t.Fatalf("positive dataset control: %d %s", status, response)
		}
	})
	t.Run("same workspace STANDARDIZED job accepted", func(t *testing.T) {
		status, response := post(t, "/api/v1/entity-match-jobs", jobBody(owner, standardized.ID))
		if status != http.StatusCreated {
			t.Fatalf("positive job control: %d %s", status, response)
		}
		assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM entity_match_job WHERE workspace_id=$1 AND status='SUCCEEDED' AND output_dataset_version_id IS NOT NULL`, owner)
		if store.reads.Load() == 0 {
			t.Fatal("positive control did not read the actual input object")
		}
	})

	var ownerExecutionID uuid.UUID
	t.Run("same workspace execution accepted and enqueued once", func(t *testing.T) {
		before := len(queue.enqueued)
		status, response := post(t, "/api/v1/executions", executionBody(owner, ownerWorkflow.ID, standardized.ID, version.ID))
		if status != http.StatusAccepted {
			t.Fatalf("positive execution control: %d %s", status, response)
		}
		var decoded map[string]any
		liveOK(t, json.Unmarshal(response, &decoded), "decode execution response")
		parsed, err := uuid.Parse(decoded["id"].(string))
		liveOK(t, err, "parse accepted execution id")
		ownerExecutionID = parsed
		if len(queue.enqueued) != before+1 || queue.enqueued[before] != ownerExecutionID {
			t.Fatalf("accepted execution was not enqueued exactly once: %#v", queue.enqueued)
		}
		assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM execution WHERE id=$1 AND workspace_id=$2 AND status='QUEUED'`, ownerExecutionID, owner)
	})

	t.Run("same workspace retry accepted", func(t *testing.T) {
		liveOK(t, func() error {
			_, err := executions.Start(ctx, ownerExecutionID, "boundary-native", "boundary-setup")
			return err
		}(), "start queued execution")
		liveOK(t, func() error {
			_, err := executions.Fail(ctx, ownerExecutionID, "BOUNDARY_FAILURE", "intentional boundary failure", nil, "boundary-setup")
			return err
		}(), "fail execution for retry")
		before := len(queue.enqueued)
		status, response := post(t, "/api/v1/executions/"+ownerExecutionID.String()+"/retry", map[string]any{})
		if status != http.StatusAccepted {
			t.Fatalf("positive retry control: %d %s", status, response)
		}
		if len(queue.enqueued) != before+1 {
			t.Fatalf("accepted retry was not enqueued exactly once: %#v", queue.enqueued)
		}
	})

	t.Run("retry re-validates a drifted workspace reference", func(t *testing.T) {
		// Simulate an Execution persisted before workspace scoping existed: its own
		// workspace is still owner, but its workflow reference now points at foreign.
		liveOK(t, func() error {
			_, err := pool.Exec(ctx, `UPDATE execution SET workflow_version_id=$2 WHERE id=$1`, ownerExecutionID, foreignWorkflow.ID)
			return err
		}(), "simulate drifted execution reference")
		before := snapshot(t)
		dispatches := len(queue.enqueued)
		status, response := post(t, "/api/v1/executions/"+ownerExecutionID.String()+"/retry", map[string]any{})
		var envelope httpserver.ErrorEnvelope
		liveOK(t, json.Unmarshal(response, &envelope), "decode retry error envelope")
		if status != http.StatusBadRequest || envelope.Code != "EXECUTION_WORKSPACE_MISMATCH" {
			t.Fatalf("status=%d response=%s, want 400/EXECUTION_WORKSPACE_MISMATCH", status, response)
		}
		if strings.Contains(string(response), owner.String()) || strings.Contains(string(response), foreign.String()) {
			t.Fatalf("retry error discloses a workspace identity: %s", response)
		}
		if len(queue.enqueued) != dispatches {
			t.Fatalf("rejected retry enqueued a task: %#v", queue.enqueued)
		}
		// The tamper itself changed the Execution row; compare everything except that row.
		if after := snapshot(t); after != before {
			t.Fatal("rejected retry changed persisted facts")
		}
	})

	t.Run("worker quarantines a queued execution with a foreign reference", func(t *testing.T) {
		// Simulate a row queued before reference scoping existed by inserting it directly,
		// binding a foreign DatasetVersion as input. Create/Retry validation is bypassed, so
		// only the worker's own revalidation can stop foreign data from being processed.
		legacyID := uuid.New()
		liveOK(t, func() error {
			_, err := pool.Exec(ctx, `INSERT INTO execution (
				id, workspace_id, workflow_version_id, output_dataset_id,
				target_period, status, attempt, engine_type, metrics, created_at
			) VALUES ($1,$2,$3,$4,'2025-03','QUEUED',1,'NATIVE','{}'::jsonb,now())`,
				legacyID, owner, ownerWorkflow.ID, standardized.ID)
			return err
		}(), "insert legacy queued execution")
		liveOK(t, func() error {
			_, err := pool.Exec(ctx, `INSERT INTO execution_input (execution_id, input_name, dataset_version_id) VALUES ($1,'legacy_input',$2)`, legacyID, foreignVersion.ID)
			return err
		}(), "insert legacy execution input")

		engine := &countingNativeEngine{}
		handler := workflowqueue.NewHandler(executions, workflowRepo, engine)
		task := asynq.NewTask(workflowqueue.TaskExecute, []byte(`{"executionId":"`+legacyID.String()+`"}`))
		liveOK(t, handler.Handle(ctx, task), "worker handles legacy queued execution")

		if calls := engine.calls.Load(); calls != 0 {
			t.Fatalf("worker executed a quarantined execution %d time(s)", calls)
		}
		var status, code string
		liveOK(t, pool.QueryRow(ctx, `SELECT status, COALESCE(error_code,'') FROM execution WHERE id=$1`, legacyID).Scan(&status, &code), "read quarantined execution")
		if status != "FAILED" || code != "EXECUTION_REFERENCE_WORKSPACE_MISMATCH" {
			t.Fatalf("quarantined execution status=%s code=%s, want FAILED/EXECUTION_REFERENCE_WORKSPACE_MISMATCH", status, code)
		}
	})

	t.Run("worker quarantines a queued execution whose input was invalidated", func(t *testing.T) {
		// An input that was READY when the Execution was queued can be invalidated later.
		// That is permanent, so the worker must quarantine instead of retrying forever.
		liveOK(t, func() error {
			_, err := invalidate.Handle(ctx, datasetapp.InvalidateVersionCommand{VersionID: version.ID, Reason: "boundary-invalidation", TraceID: "boundary-setup"})
			return err
		}(), "invalidate queued execution input")
		legacyID := uuid.New()
		liveOK(t, func() error {
			_, err := pool.Exec(ctx, `INSERT INTO execution (
				id, workspace_id, workflow_version_id, output_dataset_id,
				target_period, status, attempt, engine_type, metrics, created_at
			) VALUES ($1,$2,$3,$4,'2025-03','QUEUED',1,'NATIVE','{}'::jsonb,now())`,
				legacyID, owner, ownerWorkflow.ID, standardized.ID)
			return err
		}(), "insert queued execution with invalidated input")
		liveOK(t, func() error {
			_, err := pool.Exec(ctx, `INSERT INTO execution_input (execution_id, input_name, dataset_version_id) VALUES ($1,'stale_input',$2)`, legacyID, version.ID)
			return err
		}(), "insert invalidated execution input")

		engine := &countingNativeEngine{}
		handler := workflowqueue.NewHandler(executions, workflowRepo, engine)
		task := asynq.NewTask(workflowqueue.TaskExecute, []byte(`{"executionId":"`+legacyID.String()+`"}`))
		liveOK(t, handler.Handle(ctx, task), "worker handles execution with invalidated input")

		if calls := engine.calls.Load(); calls != 0 {
			t.Fatalf("worker executed an execution with an invalidated input %d time(s)", calls)
		}
		var status, code string
		liveOK(t, pool.QueryRow(ctx, `SELECT status, COALESCE(error_code,'') FROM execution WHERE id=$1`, legacyID).Scan(&status, &code), "read unusable-input execution")
		if status != "FAILED" || code != "EXECUTION_REFERENCE_UNUSABLE" {
			t.Fatalf("unusable-input execution status=%s code=%s, want FAILED/EXECUTION_REFERENCE_UNUSABLE", status, code)
		}
	})
}

func mustCreateBoundaryWorkflowVersion(t *testing.T, ctx context.Context, service *workflowapp.WorkflowVersionService, workspaceID uuid.UUID, actorID *uuid.UUID) workflowdomain.WorkflowVersion {
	t.Helper()
	version, err := service.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           "boundary-workflow-" + uuid.NewString(),
		Name:           "Boundary workflow",
		Version:        "1.0.0",
		DefinitionRef:  "examples/enterprise-activity/workflow/workflow-v1.yaml",
		DefinitionYAML: []byte("apiVersion: dataprod.platform/v1alpha1\nkind: WorkflowDefinition\nmetadata:\n  name: boundary\n  version: 1.0.0\n"),
		ActorID:        actorID,
		TraceID:        "boundary-setup",
	})
	if err != nil {
		t.Fatalf("create boundary workflow version: %v", err)
	}
	return version
}
