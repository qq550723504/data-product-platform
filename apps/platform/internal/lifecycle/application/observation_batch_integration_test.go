package application

import (
	"context"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"testing"
)

func TestProviderObservationBatchRollsBackBothAndReplaysStableProof(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	a := requireDispatch(t, f, "batch-a", 0)
	b := requireDispatch(t, f, "batch-b", 1)
	cmd := RecordProviderObservationsCommand{CommandHeader: f.header("atomic-batch"), Observations: []ProviderObservationInput{{AttemptID: a.AttemptID, ObservationRef: "not-sent"}, {AttemptID: b.AttemptID, ObservationRef: "bad-scope"}}}
	if _, e := f.s.RecordProviderObservations(ctx, cmd); e == nil {
		t.Fatal("bad second observation committed")
	}
	for _, table := range []string{"lifecycle_provider_observation", "lifecycle_dispatch_release"} {
		if count(t, pool, "SELECT count(*) FROM "+table) != 0 {
			t.Fatal("partial batch persisted", table)
		}
	}
	cmd.Observations[1].ObservationRef = "not-sent"
	first, e := f.s.RecordProviderObservations(ctx, cmd)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := f.s.RecordProviderObservations(ctx, cmd)
	if e != nil || !replay.Replayed || replay.ID != first.ID {
		t.Fatal("stable proof replay failed", e)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation WHERE certainty=$1`, domain.InvocationNotSent) != 2 || count(t, pool, `SELECT count(*) FROM lifecycle_dispatch_release`) != 2 || count(t, pool, `SELECT count(*) FROM cost_event`) != 0 {
		t.Fatal("batch duplicate/lost facts or fictitious invocation cost")
	}
	if count(t, pool, `SELECT count(*) FROM outbox_event WHERE event_type=$1`, ProviderObservedEvent) != 2 || count(t, pool, `SELECT count(*) FROM outbox_event WHERE event_type=$1`, ReservationReleasedEvent) != 2 {
		t.Fatal("batch events not atomic/idempotent")
	}
}
