package application_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	qualitynative "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/native"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/infrastructure"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (s *memoryStore) Put(_ context.Context, name string, r io.Reader, _ int64, _ string) (string, error) {
	b, e := io.ReadAll(r)
	if e != nil {
		return "", e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	uri := "s3://c1-test/" + name
	s.objects[uri] = b
	return uri, nil
}
func (s *memoryStore) Get(_ context.Context, uri string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.objects[uri]
	if !ok {
		return nil, fmt.Errorf("test object unavailable")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

type physicalQuality struct {
	native   qualitynative.Engine
	calls    atomic.Int32
	failNext atomic.Bool
}

func (e *physicalQuality) Descriptor() qualityengine.Descriptor {
	d := e.native.Descriptor()
	d.Name = "c1-controlled-test"
	return d
}
func (e *physicalQuality) Evaluate(ctx context.Context, r qualityengine.Request) (qualityengine.Result, error) {
	e.calls.Add(1)
	if e.failNext.Swap(false) {
		return qualityengine.Result{}, errors.New("controlled test evaluation failed")
	}
	result, err := e.native.Evaluate(ctx, r)
	// Controlled provider uses the existing provider-neutral scalar contract.
	// Native-only rule context/sample details are not external provider fields.
	for i := range result.Findings {
		old := result.Findings[i].Observed
		safe := map[string]any{}
		for _, key := range []string{"affectedCount", "total"} {
			if value, ok := old[key]; ok {
				safe[key] = value
			}
		}
		result.Findings[i].Observed = safe
	}
	return result, err
}

type qualityRunner struct{ service *qualityapp.Service }

func (q qualityRunner) Run(ctx context.Context, cmd qualityapp.RunCommand) (qualitydomain.Assessment, error) {
	cmd.EngineName = "c1-controlled-test"
	return q.service.Run(ctx, cmd)
}

type lostQualityResponse struct {
	inner application.Quality
	drop  atomic.Bool
}

func (q *lostQualityResponse) Run(ctx context.Context, cmd qualityapp.RunCommand) (qualitydomain.Assessment, error) {
	assessment, err := q.inner.Run(ctx, cmd)
	if err == nil && q.drop.Swap(false) {
		return qualitydomain.Assessment{}, errors.New("controlled response loss after Quality commit")
	}
	return assessment, err
}

type processing struct {
	writer *datasetapp.UploadVersionService
	tx     *transaction.Manager
	repo   *datasetinfra.PostgresRepository
	calls  atomic.Int32
}

func (e *processing) Execute(ctx context.Context, r workflowapp.ProcessingRequest) (workflowapp.ProcessingResult, error) {
	if e.calls.Add(1) == 1 {
		return workflowapp.ProcessingResult{}, errors.New("controlled physical processing failure")
	}
	output, err := e.writer.Handle(ctx, datasetapp.UploadVersionCommand{DatasetID: r.OutputDatasetID, Filename: "output.csv", ContentType: "text/csv", Content: []byte("company_id,period,activity_score,activity_level,indicator_coverage\ncompany-1,2025-03,50,MEDIUM,100\n"), GeneratedByExecutionID: &r.ExecutionID, Metadata: map[string]any{"acceptedNegativeEnergyRate": 0, "unresolvedEntityRate": 0}})
	if err != nil {
		return workflowapp.ProcessingResult{}, err
	}
	err = e.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return e.repo.AddLineage(ctx, tx, output.ID, r.Inputs[0].DatasetVersionID, "DERIVED_FROM", &r.ExecutionID)
	})
	return workflowapp.ProcessingResult{OutputDatasetVersionID: output.ID, Metrics: map[string]any{"energyRecords": 2, "energyMapped": 1, "quarantineCount": 1, "outputRows": 1}}, err
}

func TestC1RealPostgresAtomicReplayConcurrencyRetryCostsAndHistory(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Dir(file)
	for {
		if _, e := os.Stat(filepath.Join(root, "AGENTS.md")); e == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("repository root not found")
		}
		root = next
	}
	plan, err := application.LoadPlan(filepath.Join(root, "industry-packs"), "park/remediation/energy-quarantine-v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tx := transaction.NewManager(pool)
	ds := datasetinfra.NewPostgresRepository(pool)
	store := &memoryStore{objects: map[string][]byte{}}
	creator := datasetapp.NewCreateDatasetService(tx, ds, resourceinfra.NewPostgresRepository())
	writer := datasetapp.NewUploadVersionService(tx, ds, store)
	ws, actor := uuid.New(), uuid.New()
	raw, err := creator.Handle(ctx, datasetapp.CreateDatasetCommand{WorkspaceID: ws, Code: uuid.NewString(), Name: "C1 RAW", DatasetType: datasetdomain.DatasetTypeRaw})
	if err != nil {
		t.Fatal(err)
	}
	input, err := writer.Handle(ctx, datasetapp.UploadVersionCommand{DatasetID: raw.ID, Filename: "raw.csv", ContentType: "text/csv", Content: []byte("meter_id,energy_kwh\nM-001,10\nM-005,-999\n")})
	if err != nil {
		t.Fatal(err)
	}
	output, err := creator.Handle(ctx, datasetapp.CreateDatasetCommand{WorkspaceID: ws, Code: uuid.NewString(), Name: "C1 output", DatasetType: datasetdomain.DatasetTypeCurated})
	if err != nil {
		t.Fatal(err)
	}
	wfRepo := workflowinfra.NewPostgresRepository(pool)
	exec := workflowapp.NewExecutionService(tx, wfRepo)
	wfBytes, err := os.ReadFile(filepath.Join(root, plan.Spec.WorkflowDefinitionRef))
	if err != nil {
		t.Fatal(err)
	}
	wf, err := workflowapp.NewWorkflowVersionService(tx, wfRepo).Create(ctx, workflowapp.CreateWorkflowVersionCommand{WorkspaceID: ws, Code: uuid.NewString(), Name: "C1 approved", Version: "1", DefinitionRef: plan.Spec.WorkflowDefinitionRef, DefinitionYAML: wfBytes, ActorID: &actor})
	if err != nil {
		t.Fatal(err)
	}
	qe := &physicalQuality{native: qualitynative.NewEngine()}
	q := qualityapp.NewService(filepath.Join(root, "industry-packs"), tx, ds, qualityinfra.NewPostgresRepository(pool), store)
	if err = q.RegisterEngine(qe); err != nil {
		t.Fatal(err)
	}
	service := &application.Service{Tx: tx, Repo: infrastructure.NewRepository(pool), Quality: qualityRunner{q}, Executions: exec, Workflows: wfRepo, Plan: plan}
	cmd := application.Command{WorkspaceID: ws, ActorID: actor, Key: "detect", Reason: "controlled test"}
	detection, err := service.Detect(ctx, cmd, input.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detection.CaseID == nil {
		t.Fatal("negative reading not detected")
	}
	cmd.CaseID = *detection.CaseID
	again, err := service.Detect(ctx, cmd, input.ID)
	if err != nil || again.AssessmentID != detection.AssessmentID || qe.calls.Load() != 1 {
		t.Fatalf("detection replay: %v calls=%d", err, qe.calls.Load())
	}
	bad := cmd
	bad.ActorID = uuid.New()
	if _, err = service.Detect(ctx, bad, input.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed actor detection: %v", err)
	}
	var before string
	if err = pool.QueryRow(ctx, `SELECT md5(to_jsonb(v)::text) FROM dataset_version v WHERE id=$1`, input.ID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	cmd.Key = "assign"
	if _, err = service.Assign(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	command := application.ExecuteCommand{Command: cmd, WorkflowVersionID: wf.ID, OutputDatasetID: output.ID, TargetPeriod: "2025-03", Inputs: []workflowdomain.InputBinding{{Name: plan.Spec.InputName, DatasetVersionID: input.ID}}}
	command.Key = "execute"
	t.Run("case proof failure rolls back execution queue and association", func(t *testing.T) {
		var routes []outbox.Route
		for _, route := range routing.Routes(false) {
			if route.EventType != "QualityRemediationExecutionAccepted" {
				routes = append(routes, route)
			}
		}
		missing, e := outbox.NewRouter("c1-rollback-test", routes)
		if e != nil {
			t.Fatal(e)
		}
		outbox.ConfigureAppendObligation(missing)
		_, e = service.Execute(ctx, command)
		outbox.ConfigureAppendObligation(router)
		if e == nil {
			t.Fatal("missing case proof accepted")
		}
		var count int
		if e = pool.QueryRow(ctx, `SELECT count(*) FROM execution WHERE workspace_id=$1`, ws).Scan(&count); e != nil || count != 0 {
			t.Fatal("orphan execution/queue acceptance", e, count)
		}
		if e = pool.QueryRow(ctx, `SELECT count(*) FROM quality_remediation_execution WHERE case_id=$1`, cmd.CaseID).Scan(&count); e != nil || count != 0 {
			t.Fatal("orphan case attempt", e, count)
		}
	})
	t.Run("same key concurrency commits one execution", func(t *testing.T) {
		var wg sync.WaitGroup
		ids := make(chan uuid.UUID, 8)
		errs := make(chan error, 8)
		for n := 0; n < 8; n++ {
			wg.Add(1)
			go func() { defer wg.Done(); e, err := service.Execute(ctx, command); ids <- e.ID; errs <- err }()
		}
		wg.Wait()
		close(ids)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var first uuid.UUID
		for id := range ids {
			if first == uuid.Nil {
				first = id
			}
			if id != first {
				t.Fatal("duplicate execution")
			}
		}
	})
	first, err := service.Execute(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	fake := &processing{writer: writer, tx: tx, repo: ds}
	handler := workflowqueue.NewHandler(exec, wfRepo, fake).WithNativeExecutionLocker(tx)
	task := asynq.NewTask(workflowqueue.TaskExecute, []byte(fmt.Sprintf(`{"executionId":%q}`, first.ID.String())))
	if err = handler.Handle(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err = handler.Handle(ctx, task); err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 1 {
		t.Fatal("failed same execution physically called again")
	}
	retry := application.ExecuteCommand{Command: cmd, Retry: true}
	retry.Key = "explicit-retry"
	child, err := service.Execute(ctx, retry)
	if err != nil {
		t.Fatal(err)
	}
	same, err := service.Execute(ctx, retry)
	if err != nil || same.ID != child.ID {
		t.Fatal("retry replay differs", err)
	}
	task = asynq.NewTask(workflowqueue.TaskExecute, []byte(fmt.Sprintf(`{"executionId":%q}`, child.ID.String())))
	if err = handler.Handle(ctx, task); err != nil {
		t.Fatal(err)
	}
	var calls int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type=$2`, ws, cost.NativeEngineInvocation).Scan(&calls)
	if err != nil || calls != 2 || fake.calls.Load() != 2 {
		t.Fatalf("physical retry costs: %v costs=%d calls=%d", err, calls, fake.calls.Load())
	}
	var link uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT id FROM quality_remediation_execution WHERE execution_id=$1`, child.ID).Scan(&link); err != nil {
		t.Fatal(err)
	}
	qe.failNext.Store(true)
	recheck := cmd
	recheck.Key = "recheck-fails"
	if _, err = service.Recheck(ctx, recheck, link); err == nil {
		t.Fatal("controlled failed evaluation passed")
	}
	failedCalls := qe.calls.Load()
	if _, err = service.Recheck(ctx, recheck, link); err == nil || qe.calls.Load() != failedCalls {
		t.Fatal("failed recheck replay invoked again", err)
	}
	recheck.Key = "recheck-new-physical-attempt"
	checkID, err := service.Recheck(ctx, recheck, link)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Recheck(ctx, recheck, link); err != nil {
		t.Fatal(err)
	}
	if qe.calls.Load() != failedCalls+1 {
		t.Fatal("new evaluation cost/call boundary differs")
	}
	t.Run("parent lock serializes late assignment", func(t *testing.T) {
		hold, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer hold.Rollback(context.Background())
		var locked uuid.UUID
		if e = hold.QueryRow(ctx, `SELECT id FROM quality_remediation_case WHERE id=$1 FOR UPDATE`, cmd.CaseID).Scan(&locked); e != nil {
			t.Fatal(e)
		}
		pending := cmd
		pending.Key = "late-assignment"
		done := make(chan error, 1)
		go func() { _, err := service.Assign(ctx, pending); done <- err }()
		waiting := false
		deadline := time.Now().Add(4 * time.Second)
		for time.Now().Before(deadline) {
			if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%FROM quality_remediation_case WHERE id=%')`).Scan(&waiting); e != nil {
				t.Fatal(e)
			}
			if waiting {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !waiting {
			t.Fatal("did not observe real parent lock wait")
		}
		select {
		case e := <-done:
			t.Fatal("assignment crossed held parent lock", e)
		default:
		}
		if e = hold.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
		if e = <-done; e != nil {
			t.Fatal(e)
		}
	})
	var gate string
	err = pool.QueryRow(ctx, `SELECT q.gate_decision FROM quality_remediation_recheck_outcome o JOIN quality_result q ON q.id=o.assessment_id WHERE o.id=$1`, checkID).Scan(&gate)
	if err != nil || gate != "PASS" {
		t.Fatalf("output recheck: %s %v", gate, err)
	}
	var after string
	if err = pool.QueryRow(ctx, `SELECT md5(to_jsonb(v)::text) FROM dataset_version v WHERE id=$1`, input.ID).Scan(&after); err != nil || before != after {
		t.Fatal("old RAW changed", err)
	}
	t.Run("history update delete truncate reject", func(t *testing.T) {
		for _, sql := range []string{`UPDATE quality_remediation_case SET plan_hash=repeat('0',64) WHERE id=$1`, `DELETE FROM quality_remediation_case WHERE id=$1`} {
			if _, err := pool.Exec(ctx, sql, cmd.CaseID); err == nil {
				t.Fatal("history mutable")
			}
		}
		if _, err := pool.Exec(ctx, `TRUNCATE quality_remediation_close`); err == nil {
			t.Fatal("truncate mutable")
		}
	})
	lost := &lostQualityResponse{inner: service.Quality}
	lost.drop.Store(true)
	service.Quality = lost
	lostCommand := recheck
	lostCommand.Key = "response-lost-after-quality-commit"
	lostID, lostErr := service.Recheck(ctx, lostCommand, link)
	if lostErr == nil || lostID == uuid.Nil {
		t.Fatal("response loss was not retained as pending")
	}
	callsAfterLostResponse := qe.calls.Load()
	cmd.Key = "manual-close"
	if _, err = service.Close(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Close(ctx, cmd); err != nil {
		t.Fatal("close replay", err)
	}
	if recovered, e := service.Recheck(ctx, lostCommand, link); e != nil || recovered != lostID || qe.calls.Load() != callsAfterLostResponse {
		t.Fatal("closed case did not recover committed quality without new physical work", e)
	}
	lostCommand.Key = "new-recheck-after-close"
	if _, e := service.Recheck(ctx, lostCommand, link); !errors.Is(e, domain.ErrClosed) {
		t.Fatal("closed case admitted a new evaluation", e)
	}
	if _, err = service.Execute(ctx, application.ExecuteCommand{Command: cmd, Retry: true}); !errors.Is(err, domain.ErrClosed) {
		t.Fatal("closed case started execution", err)
	}
	t.Logf("real PG: 8 same-key requests -> one execution; failed+retry native calls/costs=2; detection+failed+successful quality physical calls=%d; old RAW unchanged", qe.calls.Load())
}
