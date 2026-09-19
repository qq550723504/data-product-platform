package outbox_test

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	platformoutbox "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

type switchableExecutionEnqueuer struct {
	mu     sync.RWMutex
	client workflowqueue.ExecutionEnqueuer
}

func (e *switchableExecutionEnqueuer) EnqueueExecution(ctx context.Context, executionID uuid.UUID) error {
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()
	if client == nil {
		return errors.New("execution enqueuer is not configured")
	}
	return client.EnqueueExecution(ctx, executionID)
}

func (e *switchableExecutionEnqueuer) Set(client workflowqueue.ExecutionEnqueuer) {
	e.mu.Lock()
	e.client = client
	e.mu.Unlock()
}

type faultObjectStore struct{}

func (faultObjectStore) Put(_ context.Context, objectName string, reader io.Reader, _ int64, _ string) (string, error) {
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return "", err
	}
	return "s3://workflow-fault-test/" + objectName, nil
}

type faultProcessingEngine struct {
	outputVersionID uuid.UUID
	release         <-chan struct{}
	started         chan struct{}

	mu    sync.Mutex
	calls int
}

func (e *faultProcessingEngine) Execute(ctx context.Context, _ workflowapp.ProcessingRequest) (workflowapp.ProcessingResult, error) {
	e.mu.Lock()
	e.calls++
	first := e.calls == 1
	e.mu.Unlock()
	if first && e.started != nil {
		close(e.started)
	}
	if first && e.release != nil {
		select {
		case <-e.release:
		case <-ctx.Done():
			return workflowapp.ProcessingResult{}, ctx.Err()
		}
	}
	return workflowapp.ProcessingResult{
		OutputDatasetVersionID: e.outputVersionID,
		EngineExecutionID:      "test-native-engine",
		Metrics:                map[string]any{"rows": 1},
	}, nil
}

func (e *faultProcessingEngine) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

type executionFaultFixture struct {
	pool            *pgxpool.Pool
	redisAddr       string
	execution       workflowdomain.Execution
	eventID         uuid.UUID
	outputVersionID uuid.UUID
	service         *workflowapp.ExecutionService
	repo            *workflowinfra.PostgresRepository
}

func TestExecutionQueueRedisRecoveryAfterOutboxFailure(t *testing.T) {
	ctx := context.Background()
	fixture := newExecutionFaultFixture(t)
	engine := &faultProcessingEngine{outputVersionID: fixture.outputVersionID}

	badAddr := unusedRedisAddress(t)
	badClient := workflowqueue.NewClient(asynq.NewClient(asynq.RedisClientOpt{Addr: badAddr}))
	defer badClient.Close()
	goodClient := workflowqueue.NewClient(asynq.NewClient(asynq.RedisClientOpt{Addr: fixture.redisAddr}))
	defer goodClient.Close()

	switchable := &switchableExecutionEnqueuer{client: badClient}
	handler := workflowqueue.NewHandler(fixture.service, fixture.repo, engine)
	stopWorker := startExecutionQueueWorker(t, fixture.redisAddr, handler, nil)
	defer stopWorker()
	dispatcher := newExecutionDispatcher(t, fixture.pool, switchable, "t2-redis-recovery-"+uuid.NewString())

	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch while Redis is unavailable: %v", err)
	}
	assertExecutionOutboxState(t, ctx, fixture.pool, fixture.eventID, "FAILED", 0)

	switchable.Set(goodClient)
	makeExecutionEventAvailable(t, ctx, fixture.pool, fixture.eventID)
	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch after Redis recovery: %v", err)
	}
	waitForExecutionStatus(t, fixture.repo, fixture.execution.ID, workflowdomain.ExecutionSucceeded)
	assertExecutionOutboxState(t, ctx, fixture.pool, fixture.eventID, "PUBLISHED", 1)
	assertExecutionBusinessFacts(t, ctx, fixture.pool, fixture.execution.ID, fixture.outputVersionID)
	if got := engine.callCount(); got != 1 {
		t.Fatalf("recovered execution engine calls = %d, want 1", got)
	}
}

func TestExecutionQueueRedeliveryAfterLostConfirmationUsesProductionHandler(t *testing.T) {
	ctx := context.Background()
	fixture := newExecutionFaultFixture(t)
	releaseEngine := make(chan struct{})
	engine := &faultProcessingEngine{
		outputVersionID: fixture.outputVersionID,
		release:         releaseEngine,
		started:         make(chan struct{}),
	}
	handler := workflowqueue.NewHandler(fixture.service, fixture.repo, engine)
	var queueDeliveries atomic.Int32
	stopWorker := startExecutionQueueWorker(t, fixture.redisAddr, handler, &queueDeliveries)
	defer stopWorker()

	rawClient := asynq.NewClient(asynq.RedisClientOpt{Addr: fixture.redisAddr})
	client := workflowqueue.NewClient(rawClient)
	defer client.Close()
	firstEnqueued := make(chan struct{})
	releaseFirst := make(chan struct{})
	var firstCalls atomic.Int32
	firstHandler := func(ctx context.Context, event platformoutbox.PublishedEvent) error {
		if err := client.EnqueueExecution(ctx, event.AggregateID); err != nil {
			return err
		}
		if firstCalls.Add(1) == 1 {
			close(firstEnqueued)
			<-releaseFirst
		}
		return nil
	}
	secondHandler := func(ctx context.Context, event platformoutbox.PublishedEvent) error {
		return client.EnqueueExecution(ctx, event.AggregateID)
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("create routing table: %v", err)
	}
	firstDispatcher, err := platformoutbox.NewDispatcher(fixture.pool, executionDispatcherConfig("t2-lost-confirmation-first-"+uuid.NewString()), router,
		platformoutbox.HandlerRegistration{Name: routing.HandlerExecutionQueue, Handle: firstHandler})
	if err != nil {
		t.Fatalf("create first dispatcher: %v", err)
	}
	secondDispatcher, err := platformoutbox.NewDispatcher(fixture.pool, executionDispatcherConfig("t2-lost-confirmation-second-"+uuid.NewString()), router,
		platformoutbox.HandlerRegistration{Name: routing.HandlerExecutionQueue, Handle: secondHandler})
	if err != nil {
		t.Fatalf("create second dispatcher: %v", err)
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- firstDispatcher.DispatchOnce(ctx) }()
	select {
	case <-firstEnqueued:
	case <-time.After(10 * time.Second):
		t.Fatal("first dispatcher did not enqueue before confirmation barrier")
	}
	select {
	case <-engine.started:
	case <-time.After(10 * time.Second):
		t.Fatal("production workflow handler did not claim the Execution")
	}

	makeExecutionEventAvailable(t, ctx, fixture.pool, fixture.eventID)
	if err := secondDispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("takeover dispatch after lost confirmation: %v", err)
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("stale first dispatcher: %v", err)
	}
	close(releaseEngine)

	waitForExecutionStatus(t, fixture.repo, fixture.execution.ID, workflowdomain.ExecutionSucceeded)
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for queueDeliveries.Load() < 2 {
		select {
		case <-deadline.C:
			t.Fatalf("production queue deliveries = %d, want at least 2", queueDeliveries.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
	assertExecutionOutboxState(t, ctx, fixture.pool, fixture.eventID, "PUBLISHED", 1)
	assertExecutionBusinessFacts(t, ctx, fixture.pool, fixture.execution.ID, fixture.outputVersionID)
	if got := engine.callCount(); got != 1 {
		t.Fatalf("duplicate delivery invoked processing engine %d times, want 1", got)
	}
}

func newExecutionFaultFixture(t *testing.T) *executionFaultFixture {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	redisAddr := os.Getenv("REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("TEST_POSTGRES_DSN and REDIS_ADDR are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	probe := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	if err := probe.Ping(); err != nil {
		probe.Close()
		t.Fatalf("ping Redis: %v", err)
	}
	probe.Close()

	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("create routing table: %v", err)
	}
	platformoutbox.ConfigureAppendObligation(router)
	t.Cleanup(func() { platformoutbox.ConfigureAppendObligation(nil) })

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	createDataset := datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := datasetapp.NewUploadVersionService(txManager, datasetRepo, faultObjectStore{})
	workspaceID := uuid.New()
	inputDataset, err := createDataset.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "T2-FAULT-INPUT-" + uuid.NewString(),
		Name:        "T2 fault input",
		DatasetType: datasetdomain.DatasetTypeStandardized,
		TraceID:     "t2-redis-fault",
	})
	if err != nil {
		t.Fatalf("create input dataset: %v", err)
	}
	inputVersion, err := uploadDataset.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   inputDataset.ID,
		Filename:    "input.csv",
		ContentType: "text/csv",
		Content:     []byte("id,value\n1,10\n"),
		TraceID:     "t2-redis-fault",
	})
	if err != nil {
		t.Fatalf("upload input version: %v", err)
	}
	outputDataset, err := createDataset.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "T2-FAULT-OUTPUT-" + uuid.NewString(),
		Name:        "T2 fault output",
		DatasetType: datasetdomain.DatasetTypeCurated,
		TraceID:     "t2-redis-fault",
	})
	if err != nil {
		t.Fatalf("create output dataset: %v", err)
	}
	outputVersion, err := uploadDataset.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   outputDataset.ID,
		Filename:    "output.csv",
		ContentType: "text/csv",
		Content:     []byte("id,result\n1,ready\n"),
		TraceID:     "t2-redis-fault",
	})
	if err != nil {
		t.Fatalf("upload output version: %v", err)
	}
	versionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	workflowVersion, err := versionService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           "t2-redis-fault-" + uuid.NewString(),
		Name:           "T2 Redis fault workflow",
		Version:        "1.0.0",
		DefinitionRef:  "tests/t2-redis-fault.yaml",
		DefinitionYAML: []byte("apiVersion: dataprod.platform/v1alpha1\nkind: WorkflowDefinition\nmetadata:\n  name: t2-redis-fault\n  version: 1.0.0\n"),
		TraceID:        "t2-redis-fault",
	})
	if err != nil {
		t.Fatalf("create workflow version: %v", err)
	}
	service := workflowapp.NewExecutionService(txManager, workflowRepo)
	execution, err := service.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersion.ID,
		OutputDatasetID:   outputDataset.ID,
		TargetPeriod:      "2026-09",
		Inputs:            []workflowdomain.InputBinding{{Name: "input", DatasetVersionID: inputVersion.ID}},
		IdempotencyKey:    "t2-redis-fault-create-" + uuid.NewString(),
		TraceID:           "t2-redis-fault",
	})
	if err != nil {
		t.Fatalf("create real execution: %v", err)
	}
	var eventID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT id FROM outbox_event
		WHERE aggregate_id=$1 AND event_type='ExecutionQueued'
		ORDER BY created_at DESC, id DESC LIMIT 1
	`, execution.ID).Scan(&eventID); err != nil {
		t.Fatalf("find real ExecutionQueued event: %v", err)
	}
	// Make this fixture's event deterministic for DispatchOnce without changing
	// its business payload, obligation, or the Execution created by the service.
	if _, err := pool.Exec(ctx, `
		UPDATE outbox_event
		SET created_at=now() - interval '100 years', available_at=now() - interval '1 second'
		WHERE id=$1
	`, eventID); err != nil {
		t.Fatalf("prioritize real ExecutionQueued event: %v", err)
	}
	return &executionFaultFixture{
		pool:            pool,
		redisAddr:       redisAddr,
		execution:       execution,
		eventID:         eventID,
		outputVersionID: outputVersion.ID,
		service:         service,
		repo:            workflowRepo,
	}
}

func newExecutionDispatcher(t *testing.T, pool *pgxpool.Pool, enqueuer workflowqueue.ExecutionEnqueuer, consumer string) *platformoutbox.Dispatcher {
	t.Helper()
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("create routing table: %v", err)
	}
	dispatcher, err := platformoutbox.NewDispatcher(pool, executionDispatcherConfig(consumer), router,
		platformoutbox.HandlerRegistration{
			Name: routing.HandlerExecutionQueue,
			Handle: func(ctx context.Context, event platformoutbox.PublishedEvent) error {
				return enqueuer.EnqueueExecution(ctx, event.AggregateID)
			},
		})
	if err != nil {
		t.Fatalf("create execution dispatcher: %v", err)
	}
	return dispatcher
}

func executionDispatcherConfig(consumer string) platformoutbox.Config {
	return platformoutbox.Config{
		ClaimTTL:     100 * time.Millisecond,
		FailureBase:  time.Millisecond,
		FailureCap:   time.Millisecond,
		MaxAttempts:  5,
		ConsumerName: consumer,
	}
}

func startExecutionQueueWorker(t *testing.T, redisAddr string, handler *workflowqueue.Handler, deliveries *atomic.Int32) func() {
	t.Helper()
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: redisAddr}, asynq.Config{
		Concurrency: 2,
		Queues:      map[string]int{"default": 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(workflowqueue.TaskExecute, func(ctx context.Context, task *asynq.Task) error {
		if deliveries != nil {
			deliveries.Add(1)
		}
		return handler.Handle(ctx, task)
	})
	go func() {
		if err := server.Run(mux); err != nil {
			t.Errorf("real asynq worker stopped: %v", err)
		}
	}()
	return func() { server.Shutdown() }
}

func waitForExecutionStatus(t *testing.T, repo *workflowinfra.PostgresRepository, executionID uuid.UUID, want workflowdomain.ExecutionStatus) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		execution, err := repo.GetExecution(context.Background(), executionID)
		if err == nil && execution.Status == want {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("execution %s status = %s/%v, want %s", executionID, execution.Status, err, want)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func makeExecutionEventAvailable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE outbox_event SET available_at=now() - interval '1 second' WHERE id=$1`, eventID); err != nil {
		t.Fatalf("make execution queue event available: %v", err)
	}
}

func assertExecutionOutboxState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID, status string, confirmations int) {
	t.Helper()
	var gotStatus string
	var gotConfirmations int
	if err := pool.QueryRow(ctx, `SELECT status FROM outbox_event WHERE id=$1`, eventID).Scan(&gotStatus); err != nil {
		t.Fatalf("read execution outbox status: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event_consumption WHERE event_id=$1 AND consumer_name=$2`, eventID, routing.HandlerExecutionQueue).Scan(&gotConfirmations); err != nil {
		t.Fatalf("read execution outbox confirmations: %v", err)
	}
	if gotStatus != status || gotConfirmations != confirmations {
		t.Fatalf("execution outbox state = status %s confirmations %d, want %s/%d", gotStatus, gotConfirmations, status, confirmations)
	}
}

func assertExecutionBusinessFacts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, executionID, outputVersionID uuid.UUID) {
	t.Helper()
	var executionCount, successAudits, costEvents, evidenceRelations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution WHERE id=$1`, executionID).Scan(&executionCount); err != nil {
		t.Fatalf("count Execution rows: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='EXECUTION_SUCCEEDED'`, executionID).Scan(&successAudits); err != nil {
		t.Fatalf("count execution success audits: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_event WHERE execution_id=$1 AND cost_type='PROCESSING_EXECUTION'`, executionID).Scan(&costEvents); err != nil {
		t.Fatalf("count execution cost events: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM evidence_relation
		WHERE object_type='EXECUTION' AND object_id=$1 AND relation_type='SUPPORTS'
	`, executionID).Scan(&evidenceRelations); err != nil {
		t.Fatalf("count execution evidence relations: %v", err)
	}
	var storedOutput uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT output_dataset_version_id FROM execution WHERE id=$1`, executionID).Scan(&storedOutput); err != nil {
		t.Fatalf("read execution output version: %v", err)
	}
	if executionCount != 1 || successAudits != 1 || costEvents != 1 || evidenceRelations != 1 || storedOutput != outputVersionID {
		t.Fatalf("execution business facts = execution %d success_audit %d cost %d evidence %d output %s, want 1/1/1/1/%s", executionCount, successAudits, costEvents, evidenceRelations, storedOutput, outputVersionID)
	}
}

func unusedRedisAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Redis test address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release Redis test address: %v", err)
	}
	return addr
}
