package application

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
)

func TestScopedRecoveryOutboxClaimNeverTouchesAnotherObligation(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	router, err := outbox.NewRouter("exact-recovery-test-v1", []outbox.Route{{EventType: "ExactRecovery", RequiredHandlers: []string{"selected"}}})
	if err != nil {
		t.Fatal(err)
	}
	selected, other := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{other, selected} {
		if _, err = pool.Exec(ctx, `INSERT INTO outbox_event(id,aggregate_type,aggregate_id,event_type,payload,status,available_at,created_at,routing_version,required_handlers)
 VALUES($1,'TEST',$1,'ExactRecovery','{}','PENDING',now(),now()-interval '1 day','exact-recovery-test-v1',ARRAY['selected'])`, id); err != nil {
			t.Fatal(err)
		}
	}
	var before string
	if err = pool.QueryRow(ctx, `SELECT to_jsonb(e)::text FROM outbox_event e WHERE id=$1`, other).Scan(&before); err != nil {
		t.Fatal(err)
	}
	calls := 0
	d, err := outbox.NewDispatcher(pool, outbox.Config{ConsumerName: "p1-scoped-test", OnlyEventID: selected}, router,
		outbox.HandlerRegistration{Name: "selected", Handle: func(_ context.Context, e outbox.PublishedEvent) error {
			calls++
			if e.ID != selected {
				t.Fatal("foreign event reached selected handler")
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = d.DispatchOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var after, state string
	if err = pool.QueryRow(ctx, `SELECT to_jsonb(e)::text FROM outbox_event e WHERE id=$1`, other).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM outbox_event WHERE id=$1`, selected).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || before != after || state != "PUBLISHED" || count(t, pool, `SELECT count(*) FROM outbox_event_consumption WHERE event_id=$1`, other) != 0 {
		t.Fatal("scoped claim mutated another obligation or lost selected delivery")
	}
	// The ordinary publisher keeps its existing global-drain contract.
	global, err := outbox.NewDispatcher(pool, outbox.Config{ConsumerName: "global-test"}, router,
		outbox.HandlerRegistration{Name: "selected", Handle: func(_ context.Context, e outbox.PublishedEvent) error {
			if e.ID != other {
				t.Fatal("wrong remaining event")
			}
			return nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	if err = global.DispatchOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM outbox_event WHERE id=$1`, other).Scan(&state); err != nil || state != "PUBLISHED" {
		t.Fatal("default global publisher stopped draining", err)
	}
}
