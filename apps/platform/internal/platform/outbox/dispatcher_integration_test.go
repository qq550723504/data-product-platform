package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Integration proofs for T1/C1-c: one logical dispatcher, per-handler
// confirmations, and a versioned routing contract.
//  1. one event, two required handlers, one fails: the event is not completed
//     early, and the retry only runs the unconfirmed handler;
//  2. two distinct events of the same type and aggregate are both processed;
//  3. a handler that ran but crashed before confirming runs again on
//     redelivery, while the confirmation stays single and the idempotent
//     business fact is not duplicated;
//  4. a holder whose lease was taken over can neither confirm nor publish;
//  5. an event type absent from the routing version fails diagnosably instead
//     of being completed implicitly;
//  6. a declared retention-only event completes without any handler.

func handlerConfirmationCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, handlerName string, eventID uuid.UUID) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_event_consumption
		WHERE consumer_name = $1 AND event_id = $2
	`, handlerName, eventID).Scan(&count); err != nil {
		t.Fatalf("count handler confirmation: %v", err)
	}
	return count
}

// insertEventForAggregate lets several events share one aggregate_id while
// keeping distinct event_ids, modelling two versions of the same object.
func insertEventForAggregate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string, aggregateID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_event (
			id, aggregate_type, aggregate_id, event_type, payload,
			status, attempts, available_at, created_at
		) VALUES ($1, 'TEST', $2, $3, '{"test":true}', 'PENDING', 0, now(), now() - interval '100 years')
	`, id, aggregateID, eventType); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event_consumption WHERE event_id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event WHERE id = $1`, id)
	})
	return id
}

func TestDispatcherPartialFailureRetriesOnlyUnconfirmedHandlers(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	router, err := NewRouter("test-v1", []Route{
		{EventType: "DispatcherTwoHandlers", RequiredHandlers: []string{"first", "second"}},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	firstCalls, secondCalls := 0, 0
	dispatcher, err := NewDispatcher(pool, testConfig(t, nil), router,
		HandlerRegistration{Name: "first", Handle: func(context.Context, PublishedEvent) error {
			firstCalls++
			return nil
		}},
		HandlerRegistration{Name: "second", Handle: func(context.Context, PublishedEvent) error {
			secondCalls++
			if secondCalls == 1 {
				return errors.New("second handler is not ready")
			}
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	eventID := insertEvent(t, ctx, pool, "DispatcherTwoHandlers")

	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusFailed {
		t.Fatalf("status after partial failure = %s, want FAILED", row.status)
	}
	if firstCalls != 1 || secondCalls != 1 {
		t.Fatalf("handler calls = first %d second %d, want 1 and 1", firstCalls, secondCalls)
	}
	if got := handlerConfirmationCount(t, ctx, pool, "first", eventID); got != 1 {
		t.Fatalf("first confirmations = %d, want 1", got)
	}
	if got := handlerConfirmationCount(t, ctx, pool, "second", eventID); got != 0 {
		t.Fatalf("second confirmations = %d, want 0", got)
	}

	expireLease(t, ctx, pool, eventID)
	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("retry dispatch: %v", err)
	}
	if firstCalls != 1 {
		t.Fatalf("confirmed handler ran again on retry: first calls = %d, want 1", firstCalls)
	}
	if secondCalls != 2 {
		t.Fatalf("unconfirmed handler retries = %d, want 2", secondCalls)
	}
	row = loadEventRow(t, ctx, pool, eventID)
	if row.status != statusPublished {
		t.Fatalf("status after retry = %s, want PUBLISHED", row.status)
	}
	if got := handlerConfirmationCount(t, ctx, pool, "second", eventID); got != 1 {
		t.Fatalf("second confirmations = %d, want 1", got)
	}
}

func TestDispatcherProcessesDistinctEventsOfSameTypeAndAggregate(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	router, err := NewRouter("test-v1", []Route{
		{EventType: "DispatcherDistinctEvents", RequiredHandlers: []string{"counter"}},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	seen := make(map[uuid.UUID]int)
	dispatcher, err := NewDispatcher(pool, testConfig(t, nil), router,
		HandlerRegistration{Name: "counter", Handle: func(_ context.Context, event PublishedEvent) error {
			seen[event.ID]++
			return nil
		}},
	)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	aggregateID := uuid.New()
	first := insertEventForAggregate(t, ctx, pool, "DispatcherDistinctEvents", aggregateID)
	second := insertEventForAggregate(t, ctx, pool, "DispatcherDistinctEvents", aggregateID)

	for i := 0; i < 2; i++ {
		if err := dispatcher.DispatchOnce(ctx); err != nil {
			t.Fatalf("dispatch %d: %v", i, err)
		}
	}

	for _, id := range []uuid.UUID{first, second} {
		row := loadEventRow(t, ctx, pool, id)
		if row.status != statusPublished {
			t.Fatalf("event %s status = %s, want PUBLISHED", id, row.status)
		}
		if seen[id] != 1 {
			t.Fatalf("event %s handler calls = %d, want 1", id, seen[id])
		}
		if got := handlerConfirmationCount(t, ctx, pool, "counter", id); got != 1 {
			t.Fatalf("event %s confirmations = %d, want 1", id, got)
		}
	}
}

func TestDispatcherRedeliveryAfterLostConfirmationKeepsSingleFact(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	router, err := NewRouter("test-v1", []Route{
		{EventType: "DispatcherLostConfirmation", RequiredHandlers: []string{"fact-writer"}},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	// A handler whose business write is idempotent, keyed by event id. This is
	// the required consumer contract: the dispatcher only promises
	// at-least-once delivery plus a single confirmation.
	facts := make(map[uuid.UUID]int)
	invocations := 0
	handle := func(_ context.Context, event PublishedEvent) error {
		invocations++
		if _, exists := facts[event.ID]; !exists {
			facts[event.ID] = 1
		}
		return nil
	}

	dispatcher, err := NewDispatcher(pool, testConfig(t, nil), router,
		HandlerRegistration{Name: "fact-writer", Handle: handle},
	)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	eventID := insertEvent(t, ctx, pool, "DispatcherLostConfirmation")

	// Simulate a crash between handler success and the confirmation commit:
	// claim, run the handler, then never confirm.
	publisher := NewPublisher(pool, testConfig(t, nil))
	stale, err := publisher.claimOne(ctx)
	if err != nil || stale == nil || stale.Event.ID != eventID {
		t.Fatalf("manual claim = %+v err=%v, want event %s", stale, err, eventID)
	}
	if err := handle(ctx, stale.Event); err != nil {
		t.Fatalf("manual handler: %v", err)
	}
	if invocations != 1 || len(facts) != 1 {
		t.Fatalf("after manual handling: invocations=%d facts=%d", invocations, len(facts))
	}
	expireLease(t, ctx, pool, eventID)

	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if invocations != 2 {
		t.Fatalf("redelivery invocations = %d, want 2 (at-least-once)", invocations)
	}
	if len(facts) != 1 {
		t.Fatalf("business facts = %d, want 1 (idempotent handler)", len(facts))
	}
	if got := handlerConfirmationCount(t, ctx, pool, "fact-writer", eventID); got != 1 {
		t.Fatalf("confirmations = %d, want 1", got)
	}
	if row := loadEventRow(t, ctx, pool, eventID); row.status != statusPublished {
		t.Fatalf("status = %s, want PUBLISHED", row.status)
	}
}

func TestDispatcherStaleHolderCannotConfirmAfterTakeover(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	publisher := NewPublisher(pool, testConfig(t, nil))

	router, err := NewRouter("test-v1", []Route{{EventType: "DispatcherTakeover", RequiredHandlers: []string{"h"}}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	if _, err := NewDispatcher(pool, testConfig(t, nil), router,
		HandlerRegistration{Name: "h", Handle: func(context.Context, PublishedEvent) error { return nil }},
	); err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	eventID := insertEvent(t, ctx, pool, "DispatcherTakeover")
	first, err := publisher.claimOne(ctx)
	if err != nil || first == nil {
		t.Fatalf("first claim: event=%v err=%v", first, err)
	}
	expireLease(t, ctx, pool, eventID)
	second, err := publisher.claimOne(ctx)
	if err != nil || second == nil {
		t.Fatalf("takeover claim: event=%v err=%v", second, err)
	}

	recorded, err := first.confirmHandler(ctx, "h")
	if err != nil {
		t.Fatalf("stale confirmHandler: %v", err)
	}
	if recorded {
		t.Fatal("stale holder must not record a confirmation")
	}
	published, err := first.publish(ctx, []string{"h"})
	if err != nil {
		t.Fatalf("stale publish: %v", err)
	}
	if published {
		t.Fatal("stale holder must not publish")
	}
	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusProcessing || row.claimToken == nil || *row.claimToken != second.token {
		t.Fatalf("event state after stale writes = %+v, want PROCESSING under the takeover token", row)
	}
	if got := handlerConfirmationCount(t, ctx, pool, "h", eventID); got != 0 {
		t.Fatalf("confirmations after stale writes = %d, want 0", got)
	}

	if recorded, err := second.confirmHandler(ctx, "h"); err != nil || !recorded {
		t.Fatalf("takeover confirmHandler = %v err=%v, want true", recorded, err)
	}
	if published, err := second.publish(ctx, []string{"h"}); err != nil || !published {
		t.Fatalf("takeover publish = %v err=%v, want true", published, err)
	}
	if row := loadEventRow(t, ctx, pool, eventID); row.status != statusPublished {
		t.Fatalf("status = %s, want PUBLISHED", row.status)
	}
	if got := handlerConfirmationCount(t, ctx, pool, "h", eventID); got != 1 {
		t.Fatalf("confirmations = %d, want 1", got)
	}
}

func TestDispatcherRefusesUndeclaredEventType(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	router, err := NewRouter("test-v1", []Route{{EventType: "DispatcherDeclaredOnly"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	dispatcher, err := NewDispatcher(pool, testConfig(t, nil), router)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	eventID := insertEvent(t, ctx, pool, "DispatcherUndeclared")
	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch undeclared event: %v", err)
	}
	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusFailed {
		t.Fatalf("status = %s, want FAILED (never silently completed)", row.status)
	}
	if row.lastError == nil || !strings.Contains(*row.lastError, "not declared in routing version test-v1") {
		t.Fatalf("last_error = %v, want a diagnosable routing error", row.lastError)
	}
}

func TestDispatcherRetentionOnlyEventCompletesWithoutHandler(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	router, err := NewRouter("test-v1", []Route{{EventType: "DispatcherRetentionOnly"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	dispatcher, err := NewDispatcher(pool, testConfig(t, nil), router)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}

	eventID := insertEvent(t, ctx, pool, "DispatcherRetentionOnly")
	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("dispatch retention-only event: %v", err)
	}
	if row := loadEventRow(t, ctx, pool, eventID); row.status != statusPublished {
		t.Fatalf("status = %s, want PUBLISHED for an explicitly declared retention-only event", row.status)
	}
	if got := handlerConfirmationCount(t, ctx, pool, "any-handler", eventID); got != 0 {
		t.Fatalf("retention-only event confirmations = %d, want 0", got)
	}
}

func TestNewDispatcherRejectsUnregisteredRequiredHandler(t *testing.T) {
	router, err := NewRouter("test-v1", []Route{{EventType: "NeedsHandler", RequiredHandlers: []string{"absent"}}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	_, err = NewDispatcher(nil, testConfig(t, nil), router)
	if err == nil || !strings.Contains(err.Error(), "absent") {
		t.Fatalf("NewDispatcher error = %v, want the missing required handler", err)
	}
}
