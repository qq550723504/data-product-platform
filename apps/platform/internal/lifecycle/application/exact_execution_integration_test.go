package application

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func TestExactSendClaimSerializedAndLateRevocationKeepsCost(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	dispatch := requireDispatch(t, f, "send", 0)
	var a domain.ProviderAttempt
	err := f.o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		a, e = f.o.repo.GetAttempt(ctx, tx, f.w, dispatch.AttemptID, false)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	execution := NewExactExecution(f.s)
	var admitted atomic.Int64
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if execution.ClaimExactSend(ctx, a, f.copies[0]) == nil {
				admitted.Add(1)
			}
		}()
	}
	group.Wait()
	if admitted.Load() != 1 {
		t.Fatal("physical send admitted twice")
	}
	f.authority.denied.Store(true)
	f.now = f.now.Add(time.Second)
	invoked := f.now.Add(-time.Millisecond)
	proof := domain.ProviderObservation{ID: uuid.New(), WorkspaceID: f.w, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID, Certainty: domain.InvocationConfirmed, Outcome: domain.ProviderUnknown, ProofSHA256: strings.Repeat("a", 64), ObservedAt: f.now, InvokedAt: &invoked}
	if err = execution.ObserveExactSend(ctx, proof); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event WHERE activity_id=$1`, a.ID) != 1 {
		t.Fatal("revoked initiator lost incurred cost")
	}
	if err = execution.ObserveExactSend(ctx, proof); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event WHERE activity_id=$1`, a.ID) != 1 {
		t.Fatal("duplicate cost")
	}
}

func TestFixtureReferenceClosureExcludesOwnControlButBlocksForeignConsumer(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	inspect := func() domain.Check {
		var check domain.Check
		err := f.o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			check, _, err = InspectFixtureReferences(ctx, tx, f.copies[0])
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return check
	}
	if inspect() != domain.Satisfied {
		t.Fatal("own restriction fact became a material consumer")
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE controlled_new_consumer(id uuid PRIMARY KEY,dataset_version_id uuid NOT NULL REFERENCES dataset_version(id))`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO controlled_new_consumer VALUES($1,$2)`, uuid.New(), f.copies[0].DatasetVersionID); err != nil {
		t.Fatal(err)
	}
	if inspect() != domain.Unknown {
		t.Fatal("new unknown consumer silently ignored")
	}
}
func TestExactAbsenceRequiresPostDispatchJournalAndCompleteMembership(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	execution := NewExactExecution(f.s)
	f.authority.caller.Capabilities[CapabilityVerifyAbsence] = struct{}{}
	f.authority.caller.Capabilities[CapabilityInspectStorage] = struct{}{}
	dispatch := requireDispatch(t, f, "send", 0)
	f.now = f.now.Add(time.Second)
	c := f.copies[0]
	inventory := NamespaceInventory{WorkspaceID: f.w, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: c.Namespace, CallID: uuid.New(), ObservedAt: f.now, ResponseSHA256: strings.Repeat("b", 64)}
	// A naked 'absent' assertion has no native-call identity and cannot commit.
	if _, err := execution.VerifyAbsentInScope(ctx, f.header("naked"), dispatch.AttemptID, inventory); err == nil {
		t.Fatal("unbacked absence accepted")
	}
	journal := NewPostgresStorageCallJournal(f.o)
	if err := journal.StartStorageCall(ctx, StorageCallStart{ID: inventory.CallID, Copy: c, Operation: StorageListVersions, StartedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	at := f.now
	if err := journal.ObserveStorageCall(ctx, StorageCallObservation{CallID: inventory.CallID, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: inventory.ResponseSHA256}); err != nil {
		t.Fatal(err)
	}
	inventory.Versions = []ListedVersion{{Key: c.Key, Version: c.ProviderVersion, ByteSize: c.ByteSize, DeleteMarker: true}}
	if _, err := execution.VerifyAbsentInScope(ctx, f.header("marker"), dispatch.AttemptID, inventory); err == nil {
		t.Fatal("exact version/marker accepted as absent")
	}
	inventory.Versions = nil
	receipt, err := execution.VerifyAbsentInScope(ctx, f.header("verified"), dispatch.AttemptID, inventory)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := execution.VerifyAbsentInScope(ctx, f.header("verified"), dispatch.AttemptID, inventory)
	if err != nil || !replay.Replayed || receipt.ID != replay.ID {
		t.Fatal("receipt replay failed")
	}
	if _, err = execution.FinalizeContentDisposition(ctx, f.header("partial"), f.plan.ID); err == nil {
		t.Fatal("partial scope finalized")
	}
	if _, err = pool.Exec(ctx, `UPDATE lifecycle_copy_absence_receipt SET response_sha256=repeat('c',64)`); err == nil {
		t.Fatal("receipt history changed")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_plan_completion`) != 0 {
		t.Fatal("partial transaction committed")
	}
}
