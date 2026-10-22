package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestReaderCurrentGateRequiresExactFreshContextAndExplicitWindow(t *testing.T) {
	w, c, r, a := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	at := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	base := UseDecision{WorkspaceID: w, CopyID: c, ReaderID: r, ActorID: a, CheckedAt: at, Decision: Satisfied, WindowMode: UseUnbounded}
	if err := base.ValidateFor(w, c, r, a, at); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*UseDecision)
	}{
		{"unknown", func(d *UseDecision) { d.Decision = Unknown }},
		{"denied", func(d *UseDecision) { d.Decision = Denied }},
		{"missing window", func(d *UseDecision) { d.WindowMode = "" }},
		{"implicit ANY", func(d *UseDecision) { d.WindowMode = "ANY" }},
		{"foreign copy", func(d *UseDecision) { d.CopyID = uuid.New() }},
		{"foreign workspace", func(d *UseDecision) { d.WorkspaceID = uuid.New() }},
		{"wrong reader", func(d *UseDecision) { d.ReaderID = uuid.New() }},
		{"wrong actor", func(d *UseDecision) { d.ActorID = uuid.New() }},
		{"stale", func(d *UseDecision) { d.CheckedAt = at.Add(-time.Second) }},
		{"expired", func(d *UseDecision) { d.WindowMode = UseDeadline; d.UseUntil = &at }},
		{"deadline missing", func(d *UseDecision) { d.WindowMode = UseDeadline }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := base
			tc.mutate(&d)
			if d.ValidateFor(w, c, r, a, at) == nil {
				t.Fatal("invalid gate admitted reader")
			}
		})
	}
	until := at.Add(time.Second)
	bounded := base
	bounded.WindowMode = UseDeadline
	bounded.UseUntil = &until
	if err := bounded.ValidateFor(w, c, r, a, at); err != nil {
		t.Fatal(err)
	}
}

func TestReaderTerminationRequiresOwnerProofAndExactGeneration(t *testing.T) {
	at := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	p := ReaderPin{ID: uuid.New(), WorkspaceID: uuid.New(), CopyID: uuid.New(), ReaderID: uuid.New(), Kind: FixtureReader,
		ActorID: uuid.New(), PrincipalRef: "controlled-owner", Generation: 3, GateRevision: 2, StartedAt: at, WindowMode: UseUnbounded}
	proof := ReaderTermination{ID: uuid.New(), WorkspaceID: p.WorkspaceID, PinID: p.ID, ReaderID: p.ReaderID, Generation: p.Generation, Outcome: ReaderCompleted, ObservedAt: at.Add(time.Hour)}
	if err := proof.ValidateFor(p, ReaderCompleted, at.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*ReaderTermination)
	}{
		{"timeout is not quiescence", func(x *ReaderTermination) { x.Outcome = "TIMEOUT" }},
		{"stale generation", func(x *ReaderTermination) { x.Generation-- }},
		{"other pin", func(x *ReaderTermination) { x.PinID = uuid.New() }},
		{"other workspace", func(x *ReaderTermination) { x.WorkspaceID = uuid.New() }},
		{"other reader", func(x *ReaderTermination) { x.ReaderID = uuid.New() }},
		{"before start", func(x *ReaderTermination) { x.ObservedAt = at.Add(-time.Second) }},
		{"future", func(x *ReaderTermination) { x.ObservedAt = at.Add(3 * time.Hour) }},
		{"no observation", func(x *ReaderTermination) { x.ID = uuid.Nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := proof
			tc.mutate(&x)
			if x.ValidateFor(p, ReaderCompleted, at.Add(2*time.Hour)) == nil {
				t.Fatal("invalid termination released pin")
			}
		})
	}
	if proof.ValidateFor(ReaderPin{}, ReaderCompleted, at.Add(2*time.Hour)) == nil {
		t.Fatal("empty source pin accepted")
	}
}
