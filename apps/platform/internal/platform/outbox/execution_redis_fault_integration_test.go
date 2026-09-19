package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	platformoutbox "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
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

type executionQueueWorkerRecorder struct {
	pool        *pgxpool.Pool
	mu          sync.Mutex
	invocations map[uuid.UUID]int
	facts       map[uuid.UUID]int
	processed   chan uuid.UUID
}

func newExecutionQueueWorkerRecorder(t *testing.T, pool *pgxpool.Pool) *executionQueueWorkerRecorder {
	t.Helper()
	recorder := &executionQueueWorkerRecorder{
		pool:        pool,
		invocations: make(map[uuid.UUID]int),
		facts:       make(map[uuid.UUID]int),
		processed:   make(chan uuid.UUID, 8),
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM audit_event WHERE action='T2_TEST_EXECUTION_RESULT'`)
	})
	return recorder
}

func (r *executionQueueWorkerRecorder) Handle(ctx context.Context, task *asynq.Task) error {
	executionID, err := workflowqueue.ExecutionID(task)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.invocations[executionID]++
	// This models the worker's business-result boundary. Duplicate delivery is
	// allowed, but the result is keyed by the stable Core Execution ID.
	if _, exists := r.facts[executionID]; !exists {
		r.facts[executionID] = 1
	}
	r.mu.Unlock()
	resultID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("t2-execution-result/"+executionID.String()))
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO audit_event (id, actor_type, action, object_type, object_id, metadata)
		VALUES ($1, 'SYSTEM', 'T2_TEST_EXECUTION_RESULT', 'EXECUTION', $2, '{}'::jsonb)
		ON CONFLICT (id) DO NOTHING
	`, resultID, executionID); err != nil {
		return fmt.Errorf("persist idempotent worker result: %w", err)
	}
	select {
	case r.processed <- executionID:
	default:
	}
	return nil
}

func (r *executionQueueWorkerRecorder) counts(executionID uuid.UUID) (invocations, facts int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.invocations[executionID], r.facts[executionID]
}

func TestExecutionQueueRedisRecoveryAfterOutboxFailure(t *testing.T) {
	ctx := context.Background()
	pool, redisAddr := newExecutionRedisFaultFixture(t)
	eventID, executionID := insertExecutionQueueEvent(t, ctx, pool)

	badAddr := unusedRedisAddress(t)
	badRawClient := asynq.NewClient(asynq.RedisClientOpt{Addr: badAddr})
	badClient := workflowqueue.NewClient(badRawClient)
	defer badClient.Close()
	goodRawClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	goodClient := workflowqueue.NewClient(goodRawClient)
	defer goodClient.Close()

	switchable := &switchableExecutionEnqueuer{client: badClient}
	recorder := newExecutionQueueWorkerRecorder(t, pool)
	stopWorker := startExecutionQueueWorker(t, redisAddr, recorder)
	defer stopWorker()
	dispatcher := newExecutionDispatcher(t, pool, switchable, "t2-redis-recovery-"+uuid.NewString())

	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch while Redis is unavailable: %v", err)
	}
	assertExecutionOutboxState(t, ctx, pool, eventID, "FAILED", 0)

	switchable.Set(goodClient)
	makeExecutionEventAvailable(t, ctx, pool, eventID)
	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch after Redis recovery: %v", err)
	}
	waitForProcessed(t, recorder, executionID, 1)
	assertExecutionOutboxState(t, ctx, pool, eventID, "PUBLISHED", 1)
	if invocations, facts := recorder.counts(executionID); invocations != 1 || facts != 1 {
		t.Fatalf("recovered worker result = invocations %d facts %d, want 1/1", invocations, facts)
	}
	assertDurableWorkerResult(t, ctx, pool, executionID, 1)
}

func TestExecutionQueueRedeliveryAfterLostConfirmationIsBusinessIdempotent(t *testing.T) {
	ctx := context.Background()
	pool, redisAddr := newExecutionRedisFaultFixture(t)
	eventID, executionID := insertExecutionQueueEvent(t, ctx, pool)
	rawClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})
	client := workflowqueue.NewClient(rawClient)
	defer client.Close()
	recorder := newExecutionQueueWorkerRecorder(t, pool)

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
	firstDispatcher, err := platformoutbox.NewDispatcher(pool, executionDispatcherConfig("t2-lost-confirmation-first-"+uuid.NewString()), router,
		platformoutbox.HandlerRegistration{Name: routing.HandlerExecutionQueue, Handle: firstHandler})
	if err != nil {
		t.Fatalf("create first dispatcher: %v", err)
	}
	secondDispatcher, err := platformoutbox.NewDispatcher(pool, executionDispatcherConfig("t2-lost-confirmation-second-"+uuid.NewString()), router,
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
	makeExecutionEventAvailable(t, ctx, pool, eventID)
	if err := secondDispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("takeover dispatch after lost confirmation: %v", err)
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("stale first dispatcher: %v", err)
	}

	stopWorker := startExecutionQueueWorker(t, redisAddr, recorder)
	defer stopWorker()
	waitForProcessed(t, recorder, executionID, 2)
	assertExecutionOutboxState(t, ctx, pool, eventID, "PUBLISHED", 1)
	if invocations, facts := recorder.counts(executionID); invocations != 2 || facts != 1 {
		t.Fatalf("lost-confirmation worker result = invocations %d facts %d, want 2/1", invocations, facts)
	}
	assertDurableWorkerResult(t, ctx, pool, executionID, 1)
}

func newExecutionRedisFaultFixture(t *testing.T) (*pgxpool.Pool, string) {
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
	return pool, redisAddr
}

func insertExecutionQueueEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (eventID, executionID uuid.UUID) {
	t.Helper()
	eventID, executionID = uuid.New(), uuid.New()
	payload, err := json.Marshal(map[string]any{"executionId": executionID})
	if err != nil {
		t.Fatalf("marshal execution queue payload: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_event (
			id, aggregate_type, aggregate_id, event_type, event_version, payload,
			status, attempts, available_at, created_at, routing_version, required_handlers
		) VALUES ($1, 'EXECUTION', $2, 'ExecutionQueued', 1, $3,
			'PENDING', 0, now(), now() - interval '100 years', $4, $5)
	`, eventID, executionID, payload, routing.Version, []string{routing.HandlerExecutionQueue}); err != nil {
		t.Fatalf("insert execution queue event: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event_consumption WHERE event_id=$1`, eventID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event WHERE id=$1`, eventID)
	})
	return eventID, executionID
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

func startExecutionQueueWorker(t *testing.T, redisAddr string, recorder *executionQueueWorkerRecorder) func() {
	t.Helper()
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: redisAddr}, asynq.Config{
		Concurrency: 1,
		Queues:      map[string]int{"default": 1},
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(workflowqueue.TaskExecute, recorder.Handle)
	go func() {
		if err := server.Run(mux); err != nil {
			t.Errorf("real asynq worker stopped: %v", err)
		}
	}()
	return func() { server.Shutdown() }
}

func waitForProcessed(t *testing.T, recorder *executionQueueWorkerRecorder, executionID uuid.UUID, count int) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		invocations, _ := recorder.counts(executionID)
		if invocations >= count {
			return
		}
		select {
		case <-recorder.processed:
		case <-deadline.C:
			t.Fatalf("worker processed %d messages for execution %s, want at least %d", invocations, executionID, count)
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

func assertDurableWorkerResult(t *testing.T, ctx context.Context, pool *pgxpool.Pool, executionID uuid.UUID, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE action='T2_TEST_EXECUTION_RESULT' AND object_id=$1`, executionID).Scan(&got); err != nil {
		t.Fatalf("read durable worker result: %v", err)
	}
	if got != want {
		t.Fatalf("durable worker results = %d, want %d", got, want)
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
