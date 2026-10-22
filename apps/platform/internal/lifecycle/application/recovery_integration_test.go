package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// These are SQL-only fault fixtures. The synthetic provider below never sends
// DELETE, and its journaled observations are not native acceptance evidence.
type recoveryInspector struct {
	f               *disposalFixture
	journal         StorageCallJournal
	present, marker bool
	failed          atomic.Bool
	calls           atomic.Int64
	hook            func()
	crash           bool
}

func (i *recoveryInspector) call(ctx context.Context, c domain.Copy, op string) (uuid.UUID, error) {
	id := uuid.New()
	at := i.f.now
	if err := i.journal.StartStorageCall(ctx, StorageCallStart{ID: id, Copy: c, Operation: op, StartedAt: at}); err != nil {
		return uuid.Nil, err
	}
	i.calls.Add(1)
	if err := i.journal.ObserveStorageCall(ctx, StorageCallObservation{CallID: id, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: strings.Repeat("c", 64)}); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}
func (i *recoveryInspector) InspectNamespace(ctx context.Context, c domain.Copy) (NamespaceInventory, error) {
	if i.failed.Load() {
		return NamespaceInventory{}, ErrStorageUnknown
	}
	id, err := i.call(ctx, c, StorageListVersions)
	if err != nil {
		return NamespaceInventory{}, err
	}
	if i.hook != nil {
		i.hook()
	}
	if i.crash {
		panic("synthetic process interruption after durable lookup observation")
	}
	inv := NamespaceInventory{WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: c.Namespace, CallID: id, ObservedAt: i.f.now, ResponseSHA256: strings.Repeat("c", 64)}
	if i.present || i.marker {
		inv.Versions = []ListedVersion{{Key: c.Key, Version: c.ProviderVersion, ByteSize: c.ByteSize, DeleteMarker: i.marker}}
	}
	return inv, nil
}
func (i *recoveryInspector) InspectExactCopy(ctx context.Context, c domain.Copy) (ExactCopyInspection, error) {
	id, err := i.call(ctx, c, StorageStatExact)
	return ExactCopyInspection{Copy: c, ObservedAt: i.f.now, CallID: id}, err
}
func (i *recoveryInspector) InspectRetentionAndHold(context.Context, domain.Copy) (CopyProtection, error) {
	return CopyProtection{}, ErrStorageUnsupported
}
func recoveryFixture(t *testing.T, pool *pgxpool.Pool) (*disposalFixture, *RecoveryService, *recoveryInspector) {
	f := newDisposalFixture(t, pool)
	for _, cap := range []string{CapabilityReconcileDisposition, CapabilityVerifyAbsence, CapabilityInspectStorage} {
		f.authority.caller.Capabilities[cap] = struct{}{}
	}
	i := &recoveryInspector{f: f, journal: NewReconciliationStorageCallJournal(f.o), present: true}
	s := NewRecoveryService(NewExactExecution(f.s), i, transaction.NewManager(pool), nil, []uuid.UUID{f.w})
	return f, s, i
}
func recoveryWork(t *testing.T, pool *pgxpool.Pool, a uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `SELECT id FROM lifecycle_reconciliation_work WHERE attempt_id=$1`, a).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestRecoveryUnknownPresenceNeverResendsOrReleases(t *testing.T) {
	pool := planDatabase(t)
	f, s, i := recoveryFixture(t, pool)
	ctx := context.Background()
	a := requireDispatch(t, f, "unknown", 0)
	id := recoveryWork(t, pool, a.AttemptID)
	if count(t, pool, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND required_handlers=ARRAY['lifecycle-recovery-queue']::text[]`, id) != 1 {
		t.Fatal("dispatch committed without durable wake obligation")
	}
	for n := 0; n < 2; n++ {
		if err := s.RunRecovery(ctx, f.w, id); !errors.Is(err, ErrStorageUnknown) {
			t.Fatal(err)
		}
	}
	if i.calls.Load() != 4 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_send_claim`) != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_dispatch_release`) != 0 {
		t.Fatal("replay sent I/O or released uncertain reservation")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE outcome='UNKNOWN_PRESENT' AND NOT terminal`) != 2 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != 4 {
		t.Fatal("lookup result/cost missing")
	}
	for _, table := range []string{"lifecycle_reconciliation_work", "lifecycle_reconciliation_run", "lifecycle_reconciliation_result"} {
		if _, err := pool.Exec(ctx, `TRUNCATE `+table+` CASCADE`); err == nil {
			t.Fatal("history truncated", table)
		}
	}
}
func TestRecoveryCrashPreservesLookupAndSingleOwner(t *testing.T) {
	pool := planDatabase(t)
	f, s, i := recoveryFixture(t, pool)
	ctx := context.Background()
	a := requireDispatch(t, f, "crash", 0)
	id := recoveryWork(t, pool, a.AttemptID)
	i.crash = true
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("crash fault not reached")
			}
		}()
		_ = s.RunRecovery(ctx, f.w, id)
	}()
	i.crash = false
	if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_run`) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result`) != 0 || i.calls.Load() != 1 {
		t.Fatal("crash facts lost")
	}
	started := make(chan struct{})
	release := make(chan struct{})
	i.hook = func() { close(started); <-release }
	var wg sync.WaitGroup
	wg.Add(1)
	var first error
	go func() { defer wg.Done(); first = s.RunRecovery(ctx, f.w, id) }()
	<-started
	if err := s.RunRecovery(ctx, f.w, id); !errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		t.Fatal("second worker admitted", err)
	}
	close(release)
	wg.Wait()
	if !errors.Is(first, ErrStorageUnknown) {
		t.Fatal(first)
	}
	if i.calls.Load() != 3 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_run`) != 2 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result`) != 1 {
		t.Fatal("original lookup overwritten or duplicate worker performed I/O")
	}
}
func TestRecoveryAbsenceIsPartialUntilEveryExactReceipt(t *testing.T) {
	pool := planDatabase(t)
	f, s, i := recoveryFixture(t, pool)
	ctx := context.Background()
	i.present = false
	a := requireDispatch(t, f, "absent-a", 0)
	id := recoveryWork(t, pool, a.AttemptID)
	if err := s.RunRecovery(ctx, f.w, id); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE outcome='ABSENT_PARTIAL'`) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_plan_completion`) != 0 {
		t.Fatal("partial inventory finalized whole plan")
	}
	b := requireDispatch(t, f, "absent-b", 1)
	if err := s.RunRecovery(ctx, f.w, recoveryWork(t, pool, b.AttemptID)); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_plan_completion`) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE outcome='ABSENT_VERIFIED'`) != 1 {
		t.Fatal("complete scope not finalized")
	}
	// Crash after an absence commit but before its recovery result is recovered
	// through existing receipt identity, without another provider lookup.
	work, err := s.RequestRecovery(ctx, RequestRecoveryCommand{CommandHeader: f.header("resolved-new-request"), AttemptID: a.AttemptID, BasisRef: "controlled-resolved-check"})
	if err != nil {
		t.Fatal(err)
	}
	calls := i.calls.Load()
	if err = s.RunRecovery(ctx, f.w, work.ID); err != nil || i.calls.Load() != calls {
		t.Fatal("resolved attempt reissued lookup", err)
	}
}
func TestRecoveryRevocationAndRetryBudgetFailClosed(t *testing.T) {
	pool := planDatabase(t)
	f, s, i := recoveryFixture(t, pool)
	ctx := context.Background()
	a := requireDispatch(t, f, "revoked", 0)
	id := recoveryWork(t, pool, a.AttemptID)
	i.hook = func() { f.authority.denied.Store(true) }
	if err := s.RunRecovery(ctx, f.w, id); err != nil {
		t.Fatal(err)
	}
	if i.calls.Load() != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE outcome='BLOCKED_AUTHORITY'`) != 1 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != 1 {
		t.Fatal("revocation allowed HEAD or lost incurred cost")
	}
	f.authority.denied.Store(false)
	i.hook = nil
	w, err := s.RequestRecovery(ctx, RequestRecoveryCommand{CommandHeader: f.header("retry-request"), AttemptID: a.AttemptID, BasisRef: "bounded-synthetic-retry"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.RequestRecovery(ctx, RequestRecoveryCommand{CommandHeader: f.header("retry-request"), AttemptID: a.AttemptID, BasisRef: "bounded-synthetic-retry"})
	if err != nil || !replay.Replayed || replay.ID != w.ID {
		t.Fatal("request replay changed work")
	}
	i.failed.Store(true)
	for n := 0; n < MaxRecoveryRuns; n++ {
		if s.RunRecovery(ctx, f.w, w.ID) == nil {
			t.Fatal("unknown lookup acknowledged")
		}
	}
	if err = s.RunRecovery(ctx, f.w, w.ID); !errors.Is(err, ErrRecoveryExhausted) {
		t.Fatal("unbounded provider retries", err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_copy_absence_receipt`) != 0 {
		t.Fatal("unknown became absent")
	}
}
func TestRecoveryDeleteMarkerIsNotAbsence(t *testing.T) {
	pool := planDatabase(t)
	f, s, i := recoveryFixture(t, pool)
	ctx := context.Background()
	a := requireDispatch(t, f, "marker", 0)
	i.present = false
	i.marker = true
	if err := s.RunRecovery(ctx, f.w, recoveryWork(t, pool, a.AttemptID)); err == nil {
		t.Fatal("marker accepted")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_copy_absence_receipt`) != 0 {
		t.Fatal("marker proved absence")
	}
}

func TestRecoveryResumesCommittedReceiptsBeforePlanCompletion(t *testing.T) {
	pool := planDatabase(t)
	f, s, i := recoveryFixture(t, pool)
	ctx := context.Background()
	i.present = false
	a := requireDispatch(t, f, "receipt-crash-a", 0)
	b := requireDispatch(t, f, "receipt-crash-b", 1)
	for n, attempt := range []uuid.UUID{a.AttemptID, b.AttemptID} {
		inv, err := i.InspectNamespace(ctx, f.copies[n])
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.execution.VerifyAbsentInScope(ctx, f.header("committed-receipt-"+attempt.String()), attempt, inv); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a crash after both receipt commits but before completion/result.
	calls := i.calls.Load()
	if err := s.RunRecovery(ctx, f.w, recoveryWork(t, pool, b.AttemptID)); err != nil {
		t.Fatal(err)
	}
	if i.calls.Load() != calls || count(t, pool, `SELECT count(*) FROM lifecycle_plan_completion`) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE outcome='ABSENT_VERIFIED'`) != 1 {
		t.Fatal("committed receipt recovery reopened I/O or lost plan completion")
	}
}
