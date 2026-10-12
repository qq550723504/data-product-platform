package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"strings"
	"testing"
	"time"
)

func TestDeniedReconciliationRequiresFreshPresenceAndNewPhysicalIdentity(t *testing.T) {
	for _, outcome := range []string{domain.ProviderDenied, domain.ProviderUnknown} {
		t.Run(outcome, func(t *testing.T) {
			pool := planDatabase(t)
			f := newDisposalFixture(t, pool)
			ctx := context.Background()
			execution := NewExactExecution(f.s)
			f.authority.caller.Capabilities[CapabilityVerifyAbsence] = struct{}{}
			f.authority.caller.Capabilities[CapabilityInspectStorage] = struct{}{}
			dispatch := requireDispatch(t, f, "first", 0)
			var a domain.ProviderAttempt
			if err := f.o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
				var err error
				a, err = f.o.repo.GetAttempt(ctx, tx, f.w, dispatch.AttemptID, false)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := execution.ClaimExactSend(ctx, a, f.copies[0]); err != nil {
				t.Fatal(err)
			}
			f.now = f.now.Add(time.Second)
			invoked := f.now.Add(-time.Millisecond)
			proof := domain.ProviderObservation{ID: uuid.New(), WorkspaceID: f.w, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID, Certainty: domain.InvocationConfirmed, Outcome: outcome, ProofSHA256: strings.Repeat("c", 64), ObservedAt: f.now, InvokedAt: &invoked}
			if err := execution.ObserveExactSend(ctx, proof); err != nil {
				t.Fatal(err)
			}
			f.now = f.now.Add(time.Second)
			presence := ExactCopyInspection{Copy: f.copies[0], CallID: uuid.New(), ObservedAt: f.now}
			journal := NewPostgresStorageCallJournal(f.o)
			if err := journal.StartStorageCall(ctx, StorageCallStart{ID: presence.CallID, Copy: presence.Copy, Operation: StorageStatExact, StartedAt: f.now}); err != nil {
				t.Fatal(err)
			}
			at := f.now
			if err := journal.ObserveStorageCall(ctx, StorageCallObservation{CallID: presence.CallID, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200}); err != nil {
				t.Fatal(err)
			}
			_, err := execution.ReconcileDeniedAttempt(ctx, f.header("resolve"), a.ID, presence)
			if outcome == domain.ProviderUnknown {
				if err == nil {
					t.Fatal("UNKNOWN send released")
				}
				if count(t, pool, `SELECT count(*) FROM lifecycle_dispatch_release`) != 0 {
					t.Fatal("UNKNOWN history released")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			next := requireDispatch(t, f, "second", 0)
			if next.AttemptID == a.ID {
				t.Fatal("physical retry swallowed into original")
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation WHERE attempt_id=$1`, a.ID) != 1 || count(t, pool, `SELECT count(*) FROM cost_event WHERE activity_id=$1`, a.ID) != 1 {
				t.Fatal("original denied cost/history rewritten")
			}
		})
	}
}
