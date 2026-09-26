package outbox

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func insertDeadLetterEvent(t *testing.T, eventType string, attempts int, handlers []string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	pool := newTestPool(t)
	id := uuid.New()
	if handlers == nil {
		handlers = []string{}
	}
	_, err := pool.Exec(ctx, "INSERT INTO outbox_event (id, aggregate_type, aggregate_id, event_type, payload, status, attempts, available_at, created_at, event_version, dead_lettered_at, routing_version, required_handlers, attempt_base) VALUES ($1,'TEST',$1,$2,'{\"test\":true}','DEAD_LETTER',$3,now(),now() - interval '100 years',1,now(),'replay-v1',$4,0)", id, eventType, attempts, handlers)
	if err != nil {
		t.Fatalf("insert dead-letter event: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE outbox_event SET last_error='original failure' WHERE id=$1", id); err != nil {
		t.Fatalf("set dead-letter error: %v", err)
	}
	t.Cleanup(func() {
		// Replay facts are immutable by design. CI uses a disposable database, so
		// facts created by replay tests are intentionally retained until database
		// teardown. Events are driven to a non-claimable terminal state in each
		// successful replay test to avoid interfering with later claim scans.
		_, _ = pool.Exec(context.Background(), "DELETE FROM outbox_event_consumption WHERE event_id=$1 AND NOT EXISTS (SELECT 1 FROM outbox_event_replay WHERE event_id=$1)", id)
		_, _ = pool.Exec(context.Background(), "DELETE FROM outbox_event WHERE id=$1 AND NOT EXISTS (SELECT 1 FROM outbox_event_replay WHERE event_id=$1)", id)
	})
	return id
}

func TestRequeueRejectsNonDeadLetterAndUnsupportedVersion(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	service := NewRequeueService(pool)
	actorID := uuid.New()

	pendingID := insertEvent(t, ctx, pool, "ReplayPending")
	_, err := service.Requeue(ctx, RequeueOutboxEventCommand{
		EventID: pendingID, IdempotencyKey: "pending", ActorID: actorID, Reason: "should fail",
	})
	if !errors.Is(err, ErrReplayNotDeadLetter) {
		t.Fatalf("pending replay error = %v, want ErrReplayNotDeadLetter", err)
	}

	futureID := insertDeadLetterEvent(t, "ReplayFuture", 12, nil)
	if _, err := pool.Exec(ctx, "UPDATE outbox_event SET event_version=99 WHERE id=$1", futureID); err != nil {
		t.Fatalf("set future version: %v", err)
	}
	_, err = service.Requeue(ctx, RequeueOutboxEventCommand{
		EventID: futureID, IdempotencyKey: "future", ActorID: actorID, Reason: "binary not upgraded",
	})
	if !errors.Is(err, ErrReplayUnsupportedVersion) {
		t.Fatalf("future replay error = %v, want ErrReplayUnsupportedVersion", err)
	}
}

func TestRequeueIsIdempotentAuditedAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	service := NewRequeueService(pool)
	actorID := uuid.New()
	eventID := insertDeadLetterEvent(t, "ReplayAudit", 12, nil)
	cmd := RequeueOutboxEventCommand{
		EventID: eventID, IdempotencyKey: "incident-215", ActorID: actorID,
		Reason: "redis recovered", TraceID: "trace-215",
	}

	first, err := service.Requeue(ctx, cmd)
	if err != nil {
		t.Fatalf("first replay: %v", err)
	}
	second, err := service.Requeue(ctx, cmd)
	if err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay id = %s, want original %s", second.ID, first.ID)
	}
	if first.PreviousAttempts != 12 || first.PreviousLastError != "original failure" || first.PreviousDeadLetteredAt.IsZero() {
		t.Fatalf("replay history = %+v", first)
	}

	var status string
	var attempts, attemptBase int
	var lastError string
	var deadLetteredAt time.Time
	if err := pool.QueryRow(ctx, "SELECT status, attempts, attempt_base, COALESCE(last_error,''), dead_lettered_at FROM outbox_event WHERE id=$1", eventID).Scan(&status, &attempts, &attemptBase, &lastError, &deadLetteredAt); err != nil {
		t.Fatalf("read requeued event: %v", err)
	}
	if status != statusPending || attempts != 12 || attemptBase != 12 || lastError != "original failure" || deadLetteredAt.IsZero() {
		t.Fatalf("requeued row status=%s attempts=%d base=%d lastError=%q deadLetteredAt=%v", status, attempts, attemptBase, lastError, deadLetteredAt)
	}

	var replayCount, auditCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox_event_replay WHERE event_id=$1", eventID).Scan(&replayCount); err != nil {
		t.Fatalf("count replay facts: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM audit_event WHERE object_type='OUTBOX_EVENT' AND object_id=$1 AND action='OUTBOX_EVENT_REQUEUED'", eventID).Scan(&auditCount); err != nil {
		t.Fatalf("count replay audit: %v", err)
	}
	if replayCount != 1 || auditCount != 1 {
		t.Fatalf("replay facts=%d audit=%d, want 1/1", replayCount, auditCount)
	}

	conflict := cmd
	conflict.Reason = "different operator intent"
	if _, err := service.Requeue(ctx, conflict); !errors.Is(err, ErrReplayIdempotencyConflict) {
		t.Fatalf("semantic conflict error = %v, want ErrReplayIdempotencyConflict", err)
	}
	if err := NewPublisher(pool, testConfig(t, nil)).publishOnce(ctx, func(context.Context, PublishedEvent) error { return nil }); err != nil {
		t.Fatalf("publish replayed event: %v", err)
	}
}

func TestRequeueRetriesOnlyUnconfirmedHandlers(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	eventID := insertDeadLetterEvent(t, "ReplayPartial", 3, []string{"confirmed", "remaining"})
	if _, err := pool.Exec(ctx, "INSERT INTO outbox_event_consumption (consumer_name,event_id) VALUES ('confirmed',$1)", eventID); err != nil {
		t.Fatalf("seed confirmed handler: %v", err)
	}

	if _, err := NewRequeueService(pool).Requeue(ctx, RequeueOutboxEventCommand{
		EventID: eventID, IdempotencyKey: "partial", ActorID: uuid.New(), Reason: "remaining handler recovered",
	}); err != nil {
		t.Fatalf("requeue partial event: %v", err)
	}

	router, err := NewRouter("replay-v1", []Route{{EventType: "ReplayPartial", RequiredHandlers: []string{"confirmed", "remaining"}}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	var confirmedCalls, remainingCalls atomic.Int32
	dispatcher, err := NewDispatcher(pool, testConfig(t, nil), router,
		HandlerRegistration{Name: "confirmed", Handle: func(context.Context, PublishedEvent) error { confirmedCalls.Add(1); return nil }},
		HandlerRegistration{Name: "remaining", Handle: func(context.Context, PublishedEvent) error { remainingCalls.Add(1); return nil }},
	)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch replay: %v", err)
	}
	if confirmedCalls.Load() != 0 || remainingCalls.Load() != 1 {
		t.Fatalf("handler calls confirmed=%d remaining=%d, want 0/1", confirmedCalls.Load(), remainingCalls.Load())
	}
	if row := loadEventRow(t, ctx, pool, eventID); row.status != statusPublished {
		t.Fatalf("replayed event status=%s, want PUBLISHED", row.status)
	}
}

func TestRequeueGetsFreshRetryBudgetWithoutResettingAttempts(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	eventID := insertDeadLetterEvent(t, "ReplayBudget", 2, nil)
	if _, err := NewRequeueService(pool).Requeue(ctx, RequeueOutboxEventCommand{
		EventID: eventID, IdempotencyKey: "budget", ActorID: uuid.New(), Reason: "retry dependency restored",
	}); err != nil {
		t.Fatalf("requeue: %v", err)
	}

	publisher := NewPublisher(pool, testConfig(t, func(c *Config) { c.MaxAttempts = 2 }))
	failing := func(context.Context, PublishedEvent) error { return errors.New("still failing") }
	if err := publisher.publishOnce(ctx, failing); err != nil {
		t.Fatalf("first replay-generation failure: %v", err)
	}
	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusFailed || row.attempts != 3 {
		t.Fatalf("after first post-replay failure status=%s attempts=%d, want FAILED/3", row.status, row.attempts)
	}
	expireLease(t, ctx, pool, eventID)
	if err := publisher.publishOnce(ctx, failing); err != nil {
		t.Fatalf("second replay-generation failure: %v", err)
	}
	row = loadEventRow(t, ctx, pool, eventID)
	if row.status != statusDeadLetter || row.attempts != 4 {
		t.Fatalf("after second post-replay failure status=%s attempts=%d, want DEAD_LETTER/4", row.status, row.attempts)
	}
}

func TestConcurrentRequeueOnlyOneIntentWins(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	eventID := insertDeadLetterEvent(t, "ReplayConcurrent", 12, nil)
	service := NewRequeueService(pool)

	type result struct {
		fact ReplayFact
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, key := range []string{"operator-a", "operator-b"} {
		key := key
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fact, err := service.Requeue(ctx, RequeueOutboxEventCommand{
				EventID: eventID, IdempotencyKey: key, ActorID: uuid.New(), Reason: "same recovered dependency",
			})
			results <- result{fact: fact, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var successes, rejected int
	for got := range results {
		if got.err == nil {
			successes++
			continue
		}
		if errors.Is(got.err, ErrReplayNotDeadLetter) {
			rejected++
			continue
		}
		t.Fatalf("unexpected concurrent replay error: %v", got.err)
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("concurrent replay successes=%d rejected=%d, want 1/1", successes, rejected)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox_event_replay WHERE event_id=$1", eventID).Scan(&count); err != nil {
		t.Fatalf("count concurrent replay facts: %v", err)
	}
	if count != 1 {
		t.Fatalf("concurrent replay facts=%d, want 1", count)
	}
	if err := NewPublisher(pool, testConfig(t, nil)).publishOnce(ctx, func(context.Context, PublishedEvent) error { return nil }); err != nil {
		t.Fatalf("publish concurrently requeued event: %v", err)
	}
}

func TestConcurrentSameKeyRequeueReturnsSameFact(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	eventID := insertDeadLetterEvent(t, "ReplaySameKeyConcurrent", 12, nil)
	service := NewRequeueService(pool)
	actorID := uuid.New()
	cmd := RequeueOutboxEventCommand{
		EventID: eventID, IdempotencyKey: "same-key", ActorID: actorID, Reason: "dependency recovered",
	}

	type result struct {
		fact ReplayFact
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fact, err := service.Requeue(ctx, cmd)
			results <- result{fact: fact, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var ids []uuid.UUID
	for got := range results {
		if got.err != nil {
			t.Fatalf("same-key concurrent replay: %v", got.err)
		}
		ids = append(ids, got.fact.ID)
	}
	if len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("same-key replay ids=%v, want two identical IDs", ids)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM outbox_event_replay WHERE event_id=$1", eventID).Scan(&count); err != nil {
		t.Fatalf("count same-key replay facts: %v", err)
	}
	if count != 1 {
		t.Fatalf("same-key replay facts=%d, want 1", count)
	}
	if err := NewPublisher(pool, testConfig(t, nil)).publishOnce(ctx, func(context.Context, PublishedEvent) error { return nil }); err != nil {
		t.Fatalf("publish same-key replayed event: %v", err)
	}
}

func TestReplayFactsAreImmutableInPostgres(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	eventID := insertDeadLetterEvent(t, "ReplayImmutable", 12, nil)
	fact, err := NewRequeueService(pool).Requeue(ctx, RequeueOutboxEventCommand{
		EventID: eventID, IdempotencyKey: "immutable", ActorID: uuid.New(), Reason: "dependency recovered",
	})
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}

	if _, err := pool.Exec(ctx, "UPDATE outbox_event_replay SET reason='rewritten' WHERE id=$1", fact.ID); err == nil {
		t.Fatal("UPDATE of immutable outbox replay fact unexpectedly succeeded")
	}
	if _, err := pool.Exec(ctx, "DELETE FROM outbox_event_replay WHERE id=$1", fact.ID); err == nil {
		t.Fatal("DELETE of immutable outbox replay fact unexpectedly succeeded")
	}
	var reason string
	if err := pool.QueryRow(ctx, "SELECT reason FROM outbox_event_replay WHERE id=$1", fact.ID).Scan(&reason); err != nil {
		t.Fatalf("read immutable replay fact: %v", err)
	}
	if reason != "dependency recovered" {
		t.Fatalf("immutable replay reason=%q, want original", reason)
	}
	if err := NewPublisher(pool, testConfig(t, nil)).publishOnce(ctx, func(context.Context, PublishedEvent) error { return nil }); err != nil {
		t.Fatalf("publish immutable-test replayed event: %v", err)
	}
}
