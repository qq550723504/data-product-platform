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
//  6. a declared retention-only event completes without any handler;
//  7. an obligation frozen by one deployment profile is honoured by a later
//     process started with a different profile and is never re-interpreted as
//     retention-only.

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

// loadObligation reads the obligation frozen on the event. ok is false when no
// obligation is frozen (both columns NULL).
func loadObligation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID) (version string, handlers []string, ok bool) {
	t.Helper()
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(routing_version, ''), COALESCE(required_handlers, ARRAY[]::text[])
		FROM outbox_event WHERE id = $1
	`, eventID).Scan(&version, &handlers); err != nil {
		t.Fatalf("load outbox obligation: %v", err)
	}
	return version, handlers, version != ""
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

// Proof 7: an obligation frozen by one deployment profile survives a restart
// with a different profile. The old event must never be re-interpreted as
// retention-only just because the new process does not declare the handler.
func TestDispatcherHonoursFrozenObligationAcrossRoutingProfiles(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	governanceRouter, err := NewRouter("c1-v1+governance", []Route{
		{EventType: "ProductReleased", RequiredHandlers: []string{"metadata-projection"}},
	})
	if err != nil {
		t.Fatalf("NewRouter(governance): %v", err)
	}
	dispatcherA, err := NewDispatcher(pool, testConfig(t, nil), governanceRouter,
		HandlerRegistration{Name: "metadata-projection", Handle: func(context.Context, PublishedEvent) error {
			return errors.New("openmetadata is unavailable")
		}},
	)
	if err != nil {
		t.Fatalf("NewDispatcher(governance): %v", err)
	}

	// The event is recorded before its obligation is known, so the first claimer
	// freezes it inside the claim transaction.
	eventID := insertEvent(t, ctx, pool, "ProductReleased")
	if err := dispatcherA.DispatchOnce(ctx); err != nil {
		t.Fatalf("governance dispatch: %v", err)
	}

	version, handlers, frozen := loadObligation(t, ctx, pool, eventID)
	if !frozen {
		t.Fatal("the first claim must freeze the obligation on the event")
	}
	if version != "c1-v1+governance" {
		t.Fatalf("frozen routing version = %q, want c1-v1+governance", version)
	}
	if len(handlers) != 1 || handlers[0] != "metadata-projection" {
		t.Fatalf("frozen handlers = %v, want [metadata-projection]", handlers)
	}
	if row := loadEventRow(t, ctx, pool, eventID); row.status != statusFailed {
		t.Fatalf("status after failed projection = %s, want FAILED", row.status)
	}

	// Deployment B: the process restarts with governance disabled and a routing
	// table that would call this event retention-only. It must not get to.
	disabledRouter, err := NewRouter("c1-v1", []Route{{EventType: "ProductReleased"}})
	if err != nil {
		t.Fatalf("NewRouter(disabled): %v", err)
	}
	dispatcherB, err := NewDispatcher(pool, testConfig(t, nil), disabledRouter)
	if err != nil {
		t.Fatalf("NewDispatcher(disabled): %v", err)
	}

	expireLease(t, ctx, pool, eventID)
	if err := dispatcherB.DispatchOnce(ctx); err != nil {
		t.Fatalf("disabled dispatch: %v", err)
	}

	row := loadEventRow(t, ctx, pool, eventID)
	if row.status == statusPublished {
		t.Fatal("the disabled profile published an event whose frozen obligation was never satisfied")
	}
	if row.lastError == nil || !strings.Contains(*row.lastError, "metadata-projection") {
		t.Fatalf("last_error = %v, want an explicit missing-obligation handler error", row.lastError)
	}
	versionAfter, handlersAfter, stillFrozen := loadObligation(t, ctx, pool, eventID)
	if !stillFrozen || versionAfter != version || len(handlersAfter) != len(handlers) || handlersAfter[0] != handlers[0] {
		t.Fatalf("obligation changed across the profile switch: before (%q %v), after (%q %v)", version, handlers, versionAfter, handlersAfter)
	}
}

// Proof 7b: the obligation is frozen when the event is recorded, before any
// dispatcher runs. A concurrently started instance with a smaller routing table
// cannot claim the event first and complete it with fewer handlers.
func TestAppendFreezesObligationBeforeAnyDispatch(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	governanceRouter, err := NewRouter("c1-v1+governance", []Route{
		{EventType: "ProductReleased", RequiredHandlers: []string{"metadata-projection"}},
	})
	if err != nil {
		t.Fatalf("NewRouter(governance): %v", err)
	}
	ConfigureAppendObligation(governanceRouter)
	defer ConfigureAppendObligation(nil)

	event, err := NewEvent("PRODUCT", uuid.New(), "ProductReleased", map[string]any{"test": true})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := Append(ctx, tx, event); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Append: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event_consumption WHERE event_id = $1`, event.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event WHERE id = $1`, event.ID)
	})

	version, handlers, frozen := loadObligation(t, ctx, pool, event.ID)
	if !frozen || version != "c1-v1+governance" || len(handlers) != 1 || handlers[0] != "metadata-projection" {
		t.Fatalf("append-time obligation = (%q %v frozen=%v), want (c1-v1+governance [metadata-projection] frozen=true)", version, handlers, frozen)
	}

	// A smaller-profile instance must fail the claim rather than complete the
	// event with fewer handlers.
	disabledRouter, err := NewRouter("c1-v1", []Route{{EventType: "ProductReleased"}})
	if err != nil {
		t.Fatalf("NewRouter(disabled): %v", err)
	}
	dispatcher, err := NewDispatcher(pool, testConfig(t, nil), disabledRouter)
	if err != nil {
		t.Fatalf("NewDispatcher(disabled): %v", err)
	}
	if err := dispatcher.DispatchOnce(ctx); err != nil {
		t.Fatalf("disabled dispatch: %v", err)
	}
	if row := loadEventRow(t, ctx, pool, event.ID); row.status == statusPublished {
		t.Fatal("a smaller-profile instance published an event it could not satisfy")
	}
}

// Proof 7c: a retention-only obligation is frozen as a non-null empty handler
// set, which stays distinguishable from "not frozen yet" (NULL).
func TestAppendFreezesRetentionOnlyObligation(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	retentionRouter, err := NewRouter("c1-v1", []Route{{EventType: "ProductReleased"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	ConfigureAppendObligation(retentionRouter)
	defer ConfigureAppendObligation(nil)

	event, err := NewEvent("PRODUCT", uuid.New(), "ProductReleased", map[string]any{"test": true})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := Append(ctx, tx, event); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("Append: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event_consumption WHERE event_id = $1`, event.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event WHERE id = $1`, event.ID)
	})

	version, handlers, frozen := loadObligation(t, ctx, pool, event.ID)
	if !frozen || version != "c1-v1" {
		t.Fatalf("retention obligation = (%q frozen=%v), want (c1-v1 true)", version, frozen)
	}
	if len(handlers) != 0 {
		t.Fatalf("retention-only handlers = %v, want an explicit empty set", handlers)
	}
}

// Proof 7d: recording an event whose type is not declared is an error. Silently
// recording it would make it indistinguishable from an explicit retention-only
// event.
func TestAppendRejectsUndeclaredEventType(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	router, err := NewRouter("test-v1", []Route{{EventType: "DeclaredOnly"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	ConfigureAppendObligation(router)
	defer ConfigureAppendObligation(nil)

	event, err := NewEvent("TEST", uuid.New(), "NotDeclared", map[string]any{"test": true})
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := Append(ctx, tx, event); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("Append error = %v, want an undeclared-type error", err)
	}
}
