package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

func TestStorageJournalPhysicalCallsCostsAndHistory(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	w, copies, s, a, _, _, _ := occupancyFixture(t, pool)
	a.caller.Capabilities[CapabilityInspectStorage] = struct{}{}
	at := time.Now().UTC().Truncate(time.Microsecond)
	s.now = func() time.Time { return at.Add(time.Minute) }
	j := NewPostgresStorageCallJournal(s)
	start := StorageCallStart{ID: uuid.New(), Copy: copies[0], Operation: StorageStatExact, StartedAt: at}
	if err := j.StartStorageCall(ctx, start); err != nil {
		t.Fatal(err)
	}
	if err := j.StartStorageCall(ctx, start); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatal("same physical ID readmitted", err)
	}
	wrong := start
	wrong.ID = uuid.New()
	wrong.Copy.WorkspaceID = uuid.New()
	if err := j.StartStorageCall(ctx, wrong); err == nil {
		t.Fatal("foreign call admitted")
	}
	wrong = start
	wrong.ID = uuid.New()
	wrong.Copy.ProviderVersion = "forged-version"
	if err := j.StartStorageCall(ctx, wrong); !errors.Is(err, ErrStorageScope) {
		t.Fatal("forged exact copy admitted", err)
	}
	uncertain := StorageCallObservation{CallID: start.ID, ObservedAt: at.Add(time.Second), Failure: "TRANSPORT"}
	if err := j.ObserveStorageCall(ctx, uncertain); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event WHERE workspace_id=$1`, w) != 0 {
		t.Fatal("start/unknown fabricated invocation cost")
	}
	invoked := at.Add(2 * time.Second)
	o := StorageCallObservation{CallID: start.ID, ObservedAt: at.Add(3 * time.Second), InvokedAt: &invoked, HTTPStatus: 403, RequestRef: "native-test-request", Failure: ""}
	a.denied.Store(true)
	if err := j.ObserveStorageCall(ctx, o); err != nil {
		t.Fatal("revocation erased incurred request", err)
	}
	if err := j.ObserveStorageCall(ctx, o); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_storage_observation WHERE call_id=$1`, start.ID) != 2 ||
		count(t, pool, `SELECT count(*) FROM cost_event e JOIN cost_allocation a ON a.cost_event_id=e.id
		 WHERE e.workspace_id=$1 AND e.quantity=1 AND e.amount IS NULL AND a.lifecycle_storage_call_id=$2`, w, start.ID) != 1 {
		t.Fatal("observations/cost replay boundary wrong")
	}
	resolution := o
	resolution.ObservedAt = at.Add(4 * time.Second)
	resolution.RequestRef = "later-server-receipt"
	if err := j.ObserveStorageCall(ctx, resolution); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event WHERE activity_id=$1`, start.ID) != 1 {
		t.Fatal("same call resolution counted twice")
	}
	second := start
	second.ID = uuid.New()
	second.Operation = StorageLegalHold
	if err := j.StartStorageCall(ctx, second); err == nil {
		t.Fatal("revoked new call admitted")
	}
	a.denied.Store(false)
	if err := j.StartStorageCall(ctx, second); err != nil {
		t.Fatal(err)
	}
	o.CallID = second.ID
	if err := j.ObserveStorageCall(ctx, o); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event WHERE workspace_id=$1`, w) != 2 {
		t.Fatal("independent invocation lost")
	}
	if count(t, pool, `SELECT count(*) FROM outbox_event WHERE event_type=$1`, StorageCallStartedEvent) != 2 ||
		count(t, pool, `SELECT count(*) FROM outbox_event WHERE event_type=$1`, StorageCallObservedEvent) != 4 {
		t.Fatal("proof replay emitted duplicate facts")
	}
	bad := o
	later := invoked.Add(time.Microsecond)
	bad.InvokedAt = &later
	if err := j.ObserveStorageCall(ctx, bad); err == nil {
		t.Fatal("one call hid a second invocation")
	}
	bad = o
	bad.ObservedAt = at.Add(-time.Second)
	bad.InvokedAt = nil
	bad.HTTPStatus = 0
	if err := j.ObserveStorageCall(ctx, bad); !errors.Is(err, ErrStorageContract) {
		t.Fatal(err)
	}
	for _, table := range []string{"lifecycle_storage_call", "lifecycle_storage_observation"} {
		for _, query := range []string{fmt.Sprintf(`UPDATE %s SET workspace_id=workspace_id`, table), fmt.Sprintf(`DELETE FROM %s`, table), fmt.Sprintf(`TRUNCATE %s CASCADE`, table)} {
			if _, err := pool.Exec(ctx, query); err == nil {
				t.Fatalf("mutable history: %s", query)
			}
		}
	}
	// The typed allocation cannot turn a made-up cost into a physical receipt.
	event := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO cost_event(id,workspace_id,activity_id,cost_type,quantity,unit,amount,pricing_mode,metadata,occurred_at)
	 VALUES($1,$2,$3,'LIFECYCLE_PROVIDER_INVOCATION',2,'invocation',NULL,'ACTUAL','{}',$4)`, event, w, uuid.New(), invoked); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,lifecycle_storage_call_id) VALUES($1,$2,$3)`, uuid.New(), event, start.ID); err == nil {
		t.Fatal("forged quantity allocated")
	}
}

func TestStorageJournalAtomicStartAndObservationProof(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	w, copies, s, a, _, _, _ := occupancyFixture(t, pool)
	a.caller.Capabilities[CapabilityInspectStorage] = struct{}{}
	at := time.Now().UTC().Truncate(time.Microsecond)
	s.now = func() time.Time { return at.Add(time.Minute) }
	j := NewPostgresStorageCallJournal(s)
	start := StorageCallStart{ID: uuid.New(), Copy: copies[0], Operation: StorageRetention, StartedAt: at}
	routes := routing.Routes(false)
	without := func(event string) {
		var selected []outbox.Route
		for _, r := range routes {
			if r.EventType != event {
				selected = append(selected, r)
			}
		}
		router, err := outbox.NewRouter("test-missing-"+event, selected)
		if err != nil {
			t.Fatal(err)
		}
		outbox.ConfigureAppendObligation(router)
	}
	without(StorageCallStartedEvent)
	if err := j.StartStorageCall(ctx, start); err == nil {
		t.Fatal("start without proof committed")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE workspace_id=$1`, w) != 0 {
		t.Fatal("partial start persisted")
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	if err := j.StartStorageCall(ctx, start); err != nil {
		t.Fatal(err)
	}
	invoked := at.Add(time.Second)
	o := StorageCallObservation{CallID: start.ID, ObservedAt: at.Add(2 * time.Second), InvokedAt: &invoked, HTTPStatus: 200, ResponseSHA256: strings.Repeat("a", 64)}
	without(StorageCallObservedEvent)
	if err := j.ObserveStorageCall(ctx, o); err == nil {
		t.Fatal("observation without proof committed")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_storage_observation WHERE call_id=$1`, start.ID) != 0 ||
		count(t, pool, `SELECT count(*) FROM cost_event WHERE activity_id=$1`, start.ID) != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE id=$1`, start.ID) != 1 {
		t.Fatal("partial observation/cost persisted, or uncertainty erased")
	}
	outbox.ConfigureAppendObligation(router)
	if err := j.ObserveStorageCall(ctx, o); err != nil {
		t.Fatal(err)
	}
}
