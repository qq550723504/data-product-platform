package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPhysicalObservationRequiresExactBindingAndExplicitInvocationCertainty(t *testing.T) {
	at := time.Date(2026, 10, 11, 4, 0, 0, 0, time.UTC)
	a := ProviderAttempt{ID: uuid.New(), WorkspaceID: uuid.New(), DispatchID: uuid.New(), PlanID: uuid.New(), CopyID: uuid.New(),
		Action: DeleteExact, StartedAt: at, DependencyRevision: 10}
	a.ProviderRequestKey = "lifecycle-delete:" + a.ID.String()
	base := ProviderObservation{ID: uuid.New(), WorkspaceID: a.WorkspaceID, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID,
		Certainty: InvocationUncertain, Outcome: ProviderUnknown, ProofSHA256: strings.Repeat("a", 64), ObservedAt: at}
	if err := base.ValidateFor(a, at); err != nil {
		t.Fatal(err)
	}
	confirmed := base
	confirmed.Certainty, confirmed.Outcome, confirmed.InvokedAt = InvocationConfirmed, ProviderFailed, &at
	if err := confirmed.ValidateFor(a, at); err != nil {
		t.Fatal(err)
	}
	notSent := base
	notSent.Certainty, notSent.Outcome = InvocationNotSent, InvocationNotSent
	if err := notSent.ValidateFor(a, at); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*ProviderObservation){
		"foreign workspace":         func(o *ProviderObservation) { o.WorkspaceID = uuid.New() },
		"wrong attempt":             func(o *ProviderObservation) { o.AttemptID = uuid.New() },
		"wrong dispatch":            func(o *ProviderObservation) { o.DispatchID = uuid.New() },
		"wrong copy":                func(o *ProviderObservation) { o.CopyID = uuid.New() },
		"missing proof":             func(o *ProviderObservation) { o.ProofSHA256 = "" },
		"before start":              func(o *ProviderObservation) { o.ObservedAt = at.Add(-time.Second) },
		"future":                    func(o *ProviderObservation) { o.ObservedAt = at.Add(time.Second) },
		"unspecified certainty":     func(o *ProviderObservation) { o.Certainty = "" },
		"unknown claims invocation": func(o *ProviderObservation) { o.InvokedAt = &at },
		"unknown claims accepted":   func(o *ProviderObservation) { o.Outcome = ProviderAccepted },
		"confirmed no time":         func(o *ProviderObservation) { o.Certainty = InvocationConfirmed },
	} {
		t.Run(name, func(t *testing.T) {
			o := base
			change(&o)
			if o.ValidateFor(a, at) == nil {
				t.Fatal("invalid proof accepted")
			}
		})
	}
}
