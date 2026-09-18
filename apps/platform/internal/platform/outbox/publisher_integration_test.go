package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

// Integration proofs for C1-a/C1-b:
//  1. a claim holder whose lease expired cannot overwrite the result of the
//     next claimer (claim tokens guard every terminal write);
//  2. poison events dead-letter instead of retrying forever;
//  3. events written before this hardening (flat payloads, version 1) stay
//     consumable, and unknown versions fail diagnosably;
//  4. redelivery after a lost acknowledgement is at-least-once with exactly
//     one per-consumer dispatch confirmation;
//  5. handler failures never stop the run loop; fatal database conditions do.

const testConsumer = "outbox-test-consumer"

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig(t *testing.T, mutate func(*Config)) Config {
	t.Helper()
	cfg := Config{
		PollInterval: time.Millisecond,
		ClaimTTL:     30 * time.Second,
		FailureBase:  time.Millisecond,
		FailureCap:   10 * time.Millisecond,
		MaxAttempts:  12,
		ConsumerName: testConsumer,
		Logger:       testLogger(),
	}
	if mutate != nil {
		mutate(&cfg)
	}
	return cfg
}

// insertEvent backdates created_at far into the past so the event always wins
// the claim scan, even if tests from other packages left claimable rows in the
// shared database (the claim query orders by created_at, id).
func insertEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventType string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO outbox_event (
			id, aggregate_type, aggregate_id, event_type, payload,
			status, attempts, available_at, created_at
		) VALUES ($1, 'TEST', $1, $2, '{"test":true}', 'PENDING', 0, now(), now() - interval '100 years')
	`, id, eventType); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event_consumption WHERE event_id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event WHERE id = $1`, id)
	})
	return id
}

type eventRow struct {
	status         string
	attempts       int
	claimToken     *uuid.UUID
	deadLetteredAt *time.Time
	publishedAt    *time.Time
	lastError      *string
}

func loadEventRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) eventRow {
	t.Helper()
	var row eventRow
	err := pool.QueryRow(ctx, `
		SELECT status, attempts, claim_token, dead_lettered_at, published_at, last_error
		FROM outbox_event WHERE id = $1
	`, id).Scan(&row.status, &row.attempts, &row.claimToken, &row.deadLetteredAt, &row.publishedAt, &row.lastError)
	if err != nil {
		t.Fatalf("load outbox event %s: %v", id, err)
	}
	return row
}

func expireLease(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(ctx,
		`UPDATE outbox_event SET available_at = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
}

func consumptionCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID uuid.UUID) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_event_consumption WHERE consumer_name = $1 AND event_id = $2`,
		testConsumer, eventID).Scan(&count); err != nil {
		t.Fatalf("count consumption: %v", err)
	}
	return count
}

// Proof 1: the previous lease holder can neither publish nor fail the event
// after its lease was taken over by a new claim.
func TestLostLeaseHolderCannotOverwriteCurrentClaim(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	publisher := NewPublisher(pool, testConfig(t, nil))

	eventID := insertEvent(t, ctx, pool, "LeaseTakeover")

	first, err := publisher.claimOne(ctx)
	if err != nil || first == nil {
		t.Fatalf("first claim: event=%v err=%v", first, err)
	}
	if first.Event.ID != eventID || first.Event.Attempts != 1 {
		t.Fatalf("first claim event = %+v, want id %s attempts 1", first.Event, eventID)
	}

	expireLease(t, ctx, pool, eventID)
	second, err := publisher.claimOne(ctx)
	if err != nil || second == nil {
		t.Fatalf("takeover claim: event=%v err=%v", second, err)
	}
	if second.token == first.token {
		t.Fatal("takeover claim must mint a new claim token")
	}
	if second.Event.Attempts != 2 {
		t.Fatalf("takeover attempts = %d, want 2", second.Event.Attempts)
	}

	ok, err := publisher.completeClaim(ctx, first)
	if err != nil {
		t.Fatalf("stale completeClaim: %v", err)
	}
	if ok {
		t.Fatal("stale claim holder must not be able to mark the event published")
	}
	if got := consumptionCount(t, ctx, pool, eventID); got != 0 {
		t.Fatalf("stale completion must roll back, consumption rows = %d, want 0", got)
	}
	outcome, err := first.Fail(ctx, errors.New("stale holder"))
	if err != nil || outcome != lostOutcome {
		t.Fatalf("stale Fail outcome = %q err = %v, want lost", outcome, err)
	}

	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusProcessing {
		t.Fatalf("status after stale writes = %s, want PROCESSING", row.status)
	}
	if row.claimToken == nil || *row.claimToken != second.token {
		t.Fatalf("claim token = %v, want the current holder's token", row.claimToken)
	}

	ok, err = publisher.completeClaim(ctx, second)
	if err != nil || !ok {
		t.Fatalf("current holder complete = %v err = %v, want true", ok, err)
	}
	row = loadEventRow(t, ctx, pool, eventID)
	if row.status != statusPublished {
		t.Fatalf("status = %s, want PUBLISHED", row.status)
	}
	if got := consumptionCount(t, ctx, pool, eventID); got != 1 {
		t.Fatalf("consumption rows = %d, want 1", got)
	}
}

// Proof 2: poison events dead-letter after MaxAttempts and are never claimed again.
func TestPoisonEventDeadLettersAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	cfg := testConfig(t, func(c *Config) { c.MaxAttempts = 2 })
	publisher := NewPublisher(pool, cfg)

	eventID := insertEvent(t, ctx, pool, "Poison")
	handler := func(context.Context, PublishedEvent) error { return errors.New("poison handler") }

	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusFailed {
		t.Fatalf("status after first failure = %s, want FAILED", row.status)
	}
	if row.attempts != 1 || row.lastError == nil || !strings.Contains(*row.lastError, "poison handler") {
		t.Fatalf("row after first failure = %+v", row)
	}

	expireLease(t, ctx, pool, eventID)
	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
	row = loadEventRow(t, ctx, pool, eventID)
	if row.status != statusDeadLetter {
		t.Fatalf("status after second failure = %s, want DEAD_LETTER", row.status)
	}
	if row.attempts != 2 || row.deadLetteredAt == nil {
		t.Fatalf("dead-lettered row = %+v", row)
	}
	if row.claimToken != nil {
		t.Fatalf("dead-lettered event must release its claim token, got %v", *row.claimToken)
	}

	expireLease(t, ctx, pool, eventID)
	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("dispatch after dead letter: %v", err)
	}
	row = loadEventRow(t, ctx, pool, eventID)
	if row.status != statusDeadLetter || row.attempts != 2 {
		t.Fatalf("dead-lettered event must not be retried, got status=%s attempts=%d", row.status, row.attempts)
	}
}

// Proof 3a: legacy flat payloads stay consumable without rewriting history.
func TestLegacyFlatPayloadEventStaysConsumable(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	publisher := NewPublisher(pool, testConfig(t, nil))

	eventID := insertEvent(t, ctx, pool, "LegacyEvent")

	var received []PublishedEvent
	handler := func(_ context.Context, event PublishedEvent) error {
		received = append(received, event)
		return nil
	}
	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("dispatch legacy event: %v", err)
	}
	if len(received) != 1 {
		t.Fatalf("handler calls = %d, want 1", len(received))
	}
	if received[0].ID != eventID || received[0].EventVersion != 1 {
		t.Fatalf("received event = %+v, want id %s version 1", received[0], eventID)
	}
	var payload map[string]any
	if err := json.Unmarshal(received[0].Payload, &payload); err != nil {
		t.Fatalf("payload is not the legacy flat json: %v", err)
	}
	if payload["test"] != true {
		t.Fatalf("legacy payload contents lost: %s", received[0].Payload)
	}
	if row := loadEventRow(t, ctx, pool, eventID); row.status != statusPublished {
		t.Fatalf("status = %s, want PUBLISHED", row.status)
	}
}

// Proof 3b: unknown event versions fail diagnosably instead of being skipped
// or misparsed, and dead-letter instead of looping forever.
func TestUnknownEventVersionFailsDiagnosably(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	cfg := testConfig(t, func(c *Config) { c.MaxAttempts = 1 })
	publisher := NewPublisher(pool, cfg)

	eventID := insertEvent(t, ctx, pool, "FutureEvent")
	if _, err := pool.Exec(ctx, `UPDATE outbox_event SET event_version = 99 WHERE id = $1`, eventID); err != nil {
		t.Fatalf("set future event version: %v", err)
	}

	handlerCalled := false
	handler := func(context.Context, PublishedEvent) error {
		handlerCalled = true
		return nil
	}
	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("dispatch future event: %v", err)
	}
	if handlerCalled {
		t.Fatal("handler must not receive an event version it cannot parse")
	}
	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusDeadLetter || row.attempts != 1 {
		t.Fatalf("row = %+v, want DEAD_LETTER after the only allowed attempt", row)
	}
	if row.lastError == nil || !strings.Contains(*row.lastError, "unsupported outbox event version 99") {
		t.Fatalf("last_error = %v, want a diagnosable version message", row.lastError)
	}
}

// Proof 4: redelivery after a lost acknowledgement dispatches again
// (at-least-once) but records exactly one consumption confirmation.
func TestRedeliveryAfterLostAcknowledgementRecordsSingleConsumption(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	publisher := NewPublisher(pool, testConfig(t, nil))

	eventID := insertEvent(t, ctx, pool, "LostAck")
	calls := 0
	handler := func(context.Context, PublishedEvent) error {
		calls++
		return nil
	}

	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if calls != 1 || consumptionCount(t, ctx, pool, eventID) != 1 {
		t.Fatalf("after first delivery: calls=%d consumption=%d", calls, consumptionCount(t, ctx, pool, eventID))
	}

	// Simulate the dispatcher dying between handler success and the
	// PUBLISHED write: the event becomes claimable again.
	if _, err := pool.Exec(ctx, `
		UPDATE outbox_event
		SET status = 'PENDING', published_at = NULL, claim_token = NULL, available_at = now()
		WHERE id = $1
	`, eventID); err != nil {
		t.Fatalf("revert publish acknowledgement: %v", err)
	}

	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	if calls != 2 {
		t.Fatalf("handler calls after redelivery = %d, want 2 (at-least-once)", calls)
	}
	if got := consumptionCount(t, ctx, pool, eventID); got != 1 {
		t.Fatalf("consumption rows after redelivery = %d, want 1", got)
	}
	row := loadEventRow(t, ctx, pool, eventID)
	if row.status != statusPublished || row.attempts != 2 {
		t.Fatalf("row after redelivery = %+v", row)
	}
}

// Dispatch order is stable: same created_at breaks ties by id, regardless of
// insertion order.
func TestDispatchOrderIsStableByCreatedAtThenID(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)
	publisher := NewPublisher(pool, testConfig(t, nil))

	idA := uuid.MustParse("00000000-0000-4000-8000-00000000000a")
	idB := uuid.MustParse("00000000-0000-4000-8000-00000000000b")
	sameTime := time.Now().UTC().AddDate(-100, 0, 0)

	insertFixed := func(t *testing.T, id uuid.UUID) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO outbox_event (
				id, aggregate_type, aggregate_id, event_type, payload,
				status, attempts, available_at, created_at
			) VALUES ($1, 'TEST', $1, 'OrderTest', '{"test":true}', 'PENDING', 0, now(), $2)
		`, id, sameTime); err != nil {
			t.Fatalf("insert ordered event: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event_consumption WHERE event_id = $1`, id)
			_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_event WHERE id = $1`, id)
		})
	}
	insertFixed(t, idB)
	insertFixed(t, idA)

	var order []uuid.UUID
	handler := func(_ context.Context, event PublishedEvent) error {
		if event.EventType == "OrderTest" {
			order = append(order, event.ID)
		}
		return nil
	}
	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if err := publisher.publishOnce(ctx, handler); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
	if len(order) != 2 || order[0] != idA || order[1] != idB {
		t.Fatalf("dispatch order = %v, want [%s %s]", order, idA, idB)
	}
}

func waitForStatus(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, want string, timeout time.Duration) eventRow {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var row eventRow
	for {
		row = loadEventRow(t, context.Background(), pool, id)
		if row.status == want {
			return row
		}
		if time.Now().After(deadline) {
			t.Fatalf("event %s status = %s, want %s within %s", id, row.status, want, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Proof 5a: Run keeps dispatching after handler failures (the poison event
// dead-letters) so one bad event cannot take the worker down.
func TestRunSurvivesHandlerFailureAndDispatchesFollowingEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := newTestPool(t)
	cfg := testConfig(t, func(c *Config) { c.MaxAttempts = 2 })
	publisher := NewPublisher(pool, cfg)

	poison := insertEvent(t, ctx, pool, "RunPoison")
	good := insertEvent(t, ctx, pool, "RunGood")

	done := make(chan struct{})
	var once sync.Once
	handler := func(_ context.Context, event PublishedEvent) error {
		if event.ID == good {
			once.Do(func() { close(done) })
			return nil
		}
		return errors.New("run poison")
	}

	runErr := make(chan error, 1)
	go func() { runErr <- publisher.Run(ctx, handler) }()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("publisher never dispatched the healthy event")
	}
	// Let the run loop finish the terminal write before stopping it; cancelling
	// mid-completion legitimately leaves the event PROCESSING for lease takeover.
	goodRow := waitForStatus(t, pool, good, statusPublished, 5*time.Second)
	cancel()
	select {
	case err := <-runErr:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want clean cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop after context cancellation")
	}

	poisonRow := loadEventRow(t, context.Background(), pool, poison)
	if poisonRow.status != statusDeadLetter {
		t.Fatalf("poison status = %s, want DEAD_LETTER", poisonRow.status)
	}
	if goodRow.status != statusPublished {
		t.Fatalf("good status = %s, want PUBLISHED", goodRow.status)
	}
}

// Proof 5b: a fatal database condition (missing table) must stop the publisher
// instead of looping forever. Uses a scratch database so the shared test
// database schema is never touched.
func TestRunStopsOnFatalSchemaError(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	adminPool := newTestPool(t)

	dbName := "outbox_fatal_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q TEMPLATE template0`, dbName)); err != nil {
		t.Fatalf("create scratch database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
	})

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	poolCfg.ConnConfig.Database = dbName
	emptyPool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("open scratch database pool: %v", err)
	}
	t.Cleanup(emptyPool.Close)

	publisher := NewPublisher(emptyPool, testConfig(t, nil))
	runErr := make(chan error, 1)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { runErr <- publisher.Run(runCtx, func(context.Context, PublishedEvent) error { return nil }) }()

	select {
	case err := <-runErr:
		if err == nil || !strings.Contains(err.Error(), "fatal database condition") {
			t.Fatalf("Run error = %v, want a fatal database condition failure", err)
		}
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) {
			t.Fatalf("Run error chain = %v, want a wrapped pgconn.PgError", err)
		}
		if pgErr.Code != "42P01" {
			t.Fatalf("fatal error code = %s, want 42P01 (undefined table)", pgErr.Code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run kept looping on a missing table instead of failing fast")
	}
}
