package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

// Synthetic server facts exercise the DB protocol; no provider, deletion or
// live disposition authority is configured. Confirmed costs below are fixtures.
type disposalApprover struct{ current domain.Check }

func (a *disposalApprover) CheckApproverAuthority(_ context.Context, _ pgx.Tx, _ domain.DispositionApproval, _ time.Time) (domain.Check, error) {
	return a.current, nil
}

type disposalDependencies struct {
	calls atomic.Int64
	hook  func(context.Context, pgx.Tx, DisposalDependencyRequest, *DisposalDependencies) error
}

func (g *disposalDependencies) EvaluateDisposalDependencies(ctx context.Context, tx pgx.Tx, r DisposalDependencyRequest) (DisposalDependencies, error) {
	g.calls.Add(1)
	d := DisposalDependencies{WorkspaceID: r.Plan.WorkspaceID, PlanID: r.Plan.ID, CopyID: r.Copy.ID, PlanHash: r.Plan.ContentHash, ActorID: r.Caller.ActorID,
		CheckedAt: r.AsOf, DependencyRevision: r.DependencyRevision, Retention: []domain.RetentionBoundary{{PolicyID: uuid.New(), StartEventID: uuid.New(), StartsAt: r.AsOf.Add(-3 * time.Hour), RetainUntil: r.AsOf.Add(-2 * time.Hour)}},
		DisposalDueAt: r.AsOf.Add(-time.Hour), InventoryComplete: domain.Satisfied, ReferencesReleased: domain.Satisfied, ProviderRetentionAllows: domain.Satisfied, LedgerCurrent: domain.Satisfied}
	d.WindowMode = domain.UseUnbounded
	if g.hook != nil {
		if err := g.hook(ctx, tx, r, &d); err != nil {
			return DisposalDependencies{}, err
		}
	}
	return d, nil
}

type disposalObservationVerifier struct{ calls atomic.Int64 }

func (v *disposalObservationVerifier) VerifyProviderObservation(_ context.Context, _ pgx.Tx, a domain.ProviderAttempt, ref string, at time.Time) (domain.ProviderObservation, error) {
	v.calls.Add(1)
	p := domain.ProviderObservation{ID: uuid.New(), WorkspaceID: a.WorkspaceID, AttemptID: a.ID, DispatchID: a.DispatchID, CopyID: a.CopyID,
		Certainty: domain.InvocationUncertain, Outcome: domain.ProviderUnknown, ProofSHA256: strings.Repeat("a", 64), ObservedAt: at}
	switch ref {
	case "not-sent":
		p.Certainty, p.Outcome = domain.InvocationNotSent, domain.InvocationNotSent
	case "failed", "accepted", "timeout":
		invokedAt := a.StartedAt.Add(time.Second)
		p.Certainty, p.InvokedAt = domain.InvocationConfirmed, &invokedAt
		if ref == "failed" {
			p.Outcome = domain.ProviderFailed
		}
		if ref == "accepted" {
			p.Outcome = domain.ProviderAccepted
		}
	case "bad-scope":
		p.CopyID = uuid.New()
	}
	return p, nil
}

type disposalFixture struct {
	w         uuid.UUID
	copies    []domain.Copy
	plan      domain.Plan
	o         *OccupancyService
	s         *DispositionService
	authority *occupancyAuthority
	approver  *disposalApprover
	deps      *disposalDependencies
	verifier  *disposalObservationVerifier
	approval  FactResult
	now       time.Time
	reader    domain.ReaderPin
}

func newDisposalFixture(t *testing.T, pool *pgxpool.Pool, withReader ...bool) *disposalFixture {
	t.Helper()
	w, copies, o, authority, _, _, plan := occupancyFixture(t, pool)
	for _, capability := range []string{CapabilityApproveDisposition, CapabilityRevokeApproval, CapabilityDispatch, CapabilityObserveAttempt} {
		authority.caller.Capabilities[capability] = struct{}{}
	}
	f := &disposalFixture{w: w, copies: copies, plan: plan, o: o, authority: authority, now: plan.FinalizedAt.Add(time.Minute), approver: &disposalApprover{current: domain.Satisfied}, deps: &disposalDependencies{}, verifier: &disposalObservationVerifier{}}
	o.now = func() time.Time { return f.now }
	f.s = NewDispositionService(o, f.approver, f.deps, f.verifier)
	var err error
	f.approval, err = f.s.ApproveContentDisposition(context.Background(), ApproveContentDispositionCommand{CommandHeader: f.header("approve"), PlanID: plan.ID, BasisRef: "controlled-approval-basis", ExpiresAt: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(withReader) > 0 && withReader[0] {
		r, err := o.AdmitFixtureReader(context.Background(), AdmitFixtureReaderCommand{CommandHeader: f.header("reader-before-restrict"), CopyID: copies[0].ID, ReaderID: uuid.New()})
		if err != nil {
			t.Fatal(err)
		}
		f.reader = r.Pin
	}
	if _, err := o.RestrictContentUse(context.Background(), RestrictContentUseCommand{CommandHeader: f.header("restrict"), DatasetVersionID: copies[0].DatasetVersionID, BasisRef: "controlled-restriction-basis"}); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *disposalFixture) header(key string) CommandHeader {
	return CommandHeader{WorkspaceID: f.w, IdempotencyKey: key}
}
func (f *disposalFixture) dispatch(key string, index int) DispatchContentDispositionCommand {
	return DispatchContentDispositionCommand{CommandHeader: f.header(key), PlanID: f.plan.ID, ApprovalID: f.approval.ID, CopyID: f.copies[index].ID}
}
func requireDispatch(t *testing.T, f *disposalFixture, key string, index int) DispatchResult {
	t.Helper()
	r, err := f.s.DispatchContentDisposition(context.Background(), f.dispatch(key, index))
	if err != nil || !r.Admitted {
		t.Fatalf("dispatch %+v %v", r, err)
	}
	return r
}
func hasBlocker(r DispatchResult, code string) bool {
	for _, b := range r.Blockers {
		if b == code {
			return true
		}
	}
	return false
}

func TestDispositionStartUnknownObservationCostAndReplayAreSeparateFacts(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	first := requireDispatch(t, f, "dispatch", 0)
	if count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 1 || count(t, pool, `SELECT count(*) FROM cost_event`) != 0 {
		t.Fatal("start treated as a definite invocation")
	}
	replay, err := f.s.DispatchContentDisposition(ctx, f.dispatch("dispatch", 0))
	if err != nil || !replay.Replayed || replay.Admitted || replay.AttemptID != first.AttemptID || f.deps.calls.Load() != 1 {
		t.Fatalf("replay readmitted or repeated gate: %+v %v", replay, err)
	}
	changed := f.dispatch("dispatch", 1)
	if _, err := f.s.DispatchContentDisposition(ctx, changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key scope changed: %v", err)
	}
	f.now = f.now.Add(time.Minute)
	observe := func(key, ref string, id uuid.UUID) (FactResult, error) {
		return f.s.RecordProviderObservation(ctx, RecordProviderObservationCommand{CommandHeader: f.header(key), AttemptID: id, ObservationRef: ref})
	}
	unknown, err := observe("obs-unknown", "uncertain", first.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	unknownReplay, err := observe("obs-unknown", "uncertain", first.AttemptID)
	if err != nil || !unknownReplay.Replayed || unknownReplay.ID != unknown.ID || f.verifier.calls.Load() != 1 {
		t.Fatalf("observation replay: %+v %v", unknownReplay, err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event`) != 0 {
		t.Fatal("uncertainty became confirmed cost")
	}
	blocked, err := f.s.DispatchContentDisposition(ctx, f.dispatch("blind-retry", 0))
	if err != nil || blocked.Admitted || !hasBlocker(blocked, "DISPOSITION_ALREADY_DISPATCHED_OR_UNKNOWN") {
		t.Fatalf("unknown allowed blind retry: %+v %v", blocked, err)
	}
	if _, err := observe("obs-failed", "failed", first.AttemptID); err != nil {
		t.Fatal(err)
	}
	if _, err := observe("obs-resolution", "accepted", first.AttemptID); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event WHERE amount IS NULL AND quantity=1`) != 1 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_provider_attempt_id=$1`, first.AttemptID) != 1 {
		t.Fatal("same physical invocation double-counted or failure cost lost")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation WHERE attempt_id=$1 AND certainty='UNCERTAIN'`, first.AttemptID) != 1 {
		t.Fatal("original unknown observation overwritten")
	}
	if _, err := observe("contradict-not-sent", "not-sent", first.AttemptID); err == nil {
		t.Fatal("confirmed invocation flipped to NOT_SENT")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_dispatch_release`) != 0 {
		t.Fatal("provider acknowledgement treated as deletion/absence proof")
	}
	second := requireDispatch(t, f, "dispatch-second-copy", 1)
	f.now = f.now.Add(time.Minute)
	if _, err := observe("obs-second-timeout", "timeout", second.AttemptID); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event`) != 2 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_provider_attempt_id IS NOT NULL`) != 2 {
		t.Fatal("new physical activity swallowed by plan idempotency")
	}
	if _, err := f.s.RevokeDispositionApproval(ctx, RevokeDispositionApprovalCommand{CommandHeader: f.header("revoke"), ApprovalID: f.approval.ID, BasisRef: "controlled-revocation"}); err != nil {
		t.Fatal(err)
	}
	if _, err := observe("obs-after-revoke", "accepted", second.AttemptID); err != nil {
		t.Fatalf("revocation erased already incurred invocation cost/history: %v", err)
	}
	if count(t, pool, `SELECT count(*) FROM cost_event`) != 2 {
		t.Fatal("resolution after revoke double-counted invocation")
	}
	for _, table := range []string{"lifecycle_disposition_approval", "lifecycle_approval_revocation", "lifecycle_disposition_gate_evaluation", "lifecycle_disposition_dependency_evaluation", "lifecycle_disposition_retention_evaluation", "lifecycle_disposition_dispatch", "lifecycle_provider_attempt", "lifecycle_provider_observation"} {
		for _, sql := range []string{"DELETE FROM " + table, "TRUNCATE " + table + " CASCADE"} {
			if _, err := pool.Exec(ctx, sql); err == nil {
				t.Fatalf("history erased: %s", sql)
			}
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE lifecycle_provider_observation SET outcome='NOT_SENT'`); err == nil {
		t.Fatal("observation rewritten")
	}
	if _, err := pool.Exec(ctx, `UPDATE lifecycle_disposition_gate_evaluation SET blockers=ARRAY['rewrite']`); err == nil {
		t.Fatal("gate result rewritten")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_disposition_retention_evaluation(evaluation_id,workspace_id,ordinal) VALUES($1,$2,1)`, first.EvaluationID, f.w); err == nil {
		t.Fatal("frozen gate accepted late input")
	}
	var payload string
	if err := pool.QueryRow(ctx, `SELECT string_agg(payload::text,',') FROM outbox_event`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"controlled-approval-basis", "controlled-revocation", "sql-only", "lifecycle-delete:", "provider_version"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("secret/detail leaked: %s", forbidden)
		}
	}
}

func TestDispositionFreshApprovalMissingDependenciesAndNotSentRelease(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	f.approver.current = domain.Denied
	r, err := f.s.DispatchContentDisposition(ctx, f.dispatch("inactive-approver", 0))
	if err != nil || r.Admitted || !hasBlocker(r, "APPROVAL_NOT_CURRENT_FOR_SCOPE") {
		t.Fatalf("approval create-time role became permanent: %+v %v", r, err)
	}
	f.approver.current = domain.Satisfied
	f.deps.hook = func(_ context.Context, _ pgx.Tx, _ DisposalDependencyRequest, d *DisposalDependencies) error {
		d.InventoryComplete = domain.Unknown
		d.Retention = nil
		return nil
	}
	r, err = f.s.DispatchContentDisposition(ctx, f.dispatch("unknown", 0))
	if err != nil || r.Admitted || !hasBlocker(r, "INVENTORY_INCOMPLETE") || !hasBlocker(r, "RETENTION_UNKNOWN") {
		t.Fatalf("unknown dependency allowed: %+v %v", r, err)
	}
	f.deps.hook = func(_ context.Context, _ pgx.Tx, _ DisposalDependencyRequest, d *DisposalDependencies) error {
		d.CopyID = uuid.New()
		return nil
	}
	if _, err := f.s.DispatchContentDisposition(ctx, f.dispatch("wrong-gate-scope", 0)); !errors.Is(err, domain.ErrInvalidDispositionFact) {
		t.Fatalf("wrong dependency scope accepted: %v", err)
	}
	f.deps.hook = nil
	first := requireDispatch(t, f, "not-sent-dispatch", 0)
	f.now = f.now.Add(time.Minute)
	if _, err := f.s.RecordProviderObservation(ctx, RecordProviderObservationCommand{CommandHeader: f.header("bad-proof"), AttemptID: first.AttemptID, ObservationRef: "bad-scope"}); !errors.Is(err, domain.ErrInvalidDispositionFact) {
		t.Fatalf("wrong server receipt scope accepted: %v", err)
	}
	cmd := RecordProviderObservationCommand{CommandHeader: f.header("not-sent"), AttemptID: first.AttemptID, ObservationRef: "not-sent"}
	if _, err := f.s.RecordProviderObservation(ctx, cmd); err != nil {
		t.Fatal(err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_dispatch_release`) != 1 || count(t, pool, `SELECT count(*) FROM cost_event`) != 0 {
		t.Fatal("NOT_SENT release/cost incorrect")
	}
	second := requireDispatch(t, f, "explicit-new-dispatch", 0)
	if second.AttemptID == first.AttemptID {
		t.Fatal("new physical attempt reused old identity")
	}
	if _, err := f.s.RecordProviderObservation(ctx, RecordProviderObservationCommand{CommandHeader: f.header("contradiction"), AttemptID: first.AttemptID, ObservationRef: "accepted"}); err == nil {
		t.Fatal("NOT_SENT flipped to invoked")
	}
	if _, err := f.s.RevokeDispositionApproval(ctx, RevokeDispositionApprovalCommand{CommandHeader: f.header("revoke"), ApprovalID: f.approval.ID, BasisRef: "basis"}); err != nil {
		t.Fatal(err)
	}
	r, err = f.s.DispatchContentDisposition(ctx, f.dispatch("revoked-second-copy", 1))
	if err != nil || r.Admitted || !hasBlocker(r, "APPROVAL_NOT_CURRENT_FOR_SCOPE") {
		t.Fatalf("revoked approval allowed: %+v %v", r, err)
	}
	f.now = f.now.Add(2 * time.Hour)
	r, err = f.s.DispatchContentDisposition(ctx, f.dispatch("expired", 1))
	if err != nil || r.Admitted || !hasBlocker(r, "APPROVAL_NOT_CURRENT_FOR_SCOPE") {
		t.Fatalf("expired approval allowed: %+v %v", r, err)
	}
	_, foreignCopies, _ := seedPlanFixture(t, pool)
	changed := f.dispatch("foreign-target", 0)
	changed.CopyID = foreignCopies[0].ID
	if _, err := f.s.DispatchContentDisposition(ctx, changed); !errors.Is(err, infrastructure.ErrScopeUnavailable) {
		t.Fatalf("foreign scope accepted: %v", err)
	}
	f.authority.denied.Store(true)
	if _, err := f.s.DispatchContentDisposition(ctx, f.dispatch("not-sent-dispatch", 0)); err == nil {
		t.Fatal("replay bypassed current caller authority")
	}
}

func TestDispositionFactsCostAndEventRollbackTogether(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	outbox.ConfigureAppendObligation(nil)
	if _, err := f.s.DispatchContentDisposition(ctx, f.dispatch("rollback", 0)); err == nil {
		t.Fatal("dispatch lost its required event")
	}
	for _, table := range []string{"lifecycle_disposition_gate_evaluation", "lifecycle_disposition_dependency_evaluation", "lifecycle_disposition_retention_evaluation", "lifecycle_disposition_dispatch", "lifecycle_provider_attempt"} {
		if count(t, pool, "SELECT count(*) FROM "+table) != 0 {
			t.Fatalf("partial dispatch: %s", table)
		}
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	r := requireDispatch(t, f, "committed-dispatch", 0)
	f.now = f.now.Add(time.Minute)
	outbox.ConfigureAppendObligation(nil)
	if _, err := f.s.RecordProviderObservation(ctx, RecordProviderObservationCommand{CommandHeader: f.header("rollback-cost"), AttemptID: r.AttemptID, ObservationRef: "failed"}); err == nil {
		t.Fatal("observation lost required event")
	}
	for _, query := range []string{`SELECT count(*) FROM lifecycle_provider_observation`, `SELECT count(*) FROM cost_event`, `SELECT count(*) FROM cost_allocation`} {
		if count(t, pool, query) != 0 {
			t.Fatalf("partial observation/cost: %s", query)
		}
	}
}

func TestDispositionHoldAndDispatchSerializeBothOrders(t *testing.T) {
	for _, holdFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("hold-first=%t", holdFirst), func(t *testing.T) {
			pool := planDatabase(t)
			f := newDisposalFixture(t, pool)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			entered, release := make(chan struct{}), make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			pause := func() error {
				close(entered)
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			if holdFirst {
				var calls atomic.Int64
				f.authority.hook = func(_ context.Context, _ pgx.Tx, capability string) error {
					if capability == CapabilityPlaceHold && calls.Add(1) == 2 {
						return pause()
					}
					return nil
				}
			} else {
				f.deps.hook = func(_ context.Context, _ pgx.Tx, _ DisposalDependencyRequest, _ *DisposalDependencies) error {
					return pause()
				}
			}
			dispatchDone, holdDone := make(chan DispatchResult, 1), make(chan FactResult, 1)
			errs := make(chan error, 2)
			dispatch := func() {
				r, err := f.s.DispatchContentDisposition(ctx, f.dispatch("race-dispatch", 0))
				dispatchDone <- r
				errs <- err
			}
			hold := func() {
				r, err := f.o.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: f.header("race-hold"), CopyID: f.copies[0].ID, BasisRef: "controlled-hold"})
				holdDone <- r
				errs <- err
			}
			if holdFirst {
				go hold()
			} else {
				go dispatch()
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("first writer did not enter fenced port")
			}
			if holdFirst {
				go dispatch()
			} else {
				go hold()
			}
			waitForOccupancyFence(t, pool)
			close(release)
			r, h := <-dispatchDone, <-holdDone
			for i := 0; i < 2; i++ {
				if err := <-errs; err != nil {
					t.Fatal(err)
				}
			}
			if holdFirst {
				if r.Admitted || !hasBlocker(r, "HOLD_ACTIVE_OR_UNKNOWN") || h.PreservationUnconfirmed || count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 0 {
					t.Fatalf("hold-first dispatched: %+v %+v", r, h)
				}
			} else {
				if !r.Admitted || !h.PreservationUnconfirmed || count(t, pool, `SELECT count(*) FROM lifecycle_hold_dispatch_conflict`) != 1 {
					t.Fatalf("late hold falsely promised containment: %+v %+v", r, h)
				}
				for _, query := range []string{`SELECT count(*) FROM outbox_event WHERE event_type='LifecycleHoldDispatchConflict'`, `SELECT count(*) FROM audit_event WHERE action='LifecycleHoldDispatchConflict'`, `SELECT count(*) FROM evidence WHERE title='LifecycleHoldDispatchConflict'`} {
					if count(t, pool, query) != 1 {
						t.Fatal("late hold conflict missing atomic proof")
					}
				}
			}
			if count(t, pool, `SELECT count(*) FROM cost_event`) != 0 {
				t.Fatal("SQL admission counted as a provider call")
			}
		})
	}
}

func TestDispositionReaderAndElapsedWindowsBlockNewAdmission(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool, true)
	ctx := context.Background()
	r, err := f.s.DispatchContentDisposition(ctx, f.dispatch("reader-active", 0))
	if err != nil || r.Admitted || !hasBlocker(r, "READERS_ACTIVE_OR_UNKNOWN") {
		t.Fatalf("active reader not protected: %+v %v", r, err)
	}
	if _, err := f.o.TerminateFixtureReader(ctx, TerminateFixtureReaderCommand{CommandHeader: f.header("reader-completed"), PinID: f.reader.ID, Generation: f.reader.Generation, Outcome: domain.ReaderCompleted}); err != nil {
		t.Fatal(err)
	}
	r, err = f.s.DispatchContentDisposition(ctx, f.dispatch("reader-active", 0))
	if err != nil || r.Admitted || !r.Replayed {
		t.Fatalf("old blocked evaluation reused as new authority: %+v %v", r, err)
	}
	f.deps.hook = func(_ context.Context, _ pgx.Tx, r DisposalDependencyRequest, d *DisposalDependencies) error {
		until := r.AsOf.Add(time.Second)
		d.WindowMode, d.ValidUntil = domain.UseDeadline, &until
		f.now = f.now.Add(time.Minute)
		return nil
	}
	r, err = f.s.DispatchContentDisposition(ctx, f.dispatch("dependency-expired-in-query", 0))
	if err != nil || r.Admitted || !hasBlocker(r, "DEPENDENCY_WINDOW_UNKNOWN_OR_EXPIRED") {
		t.Fatalf("elapsed snapshot admitted: %+v %v", r, err)
	}
	f.deps.hook = func(_ context.Context, _ pgx.Tx, _ DisposalDependencyRequest, _ *DisposalDependencies) error {
		f.now = f.now.Add(2 * time.Hour)
		return nil
	}
	r, err = f.s.DispatchContentDisposition(ctx, f.dispatch("approval-expired-in-query", 0))
	if err != nil || r.Admitted || !hasBlocker(r, "APPROVAL_NOT_CURRENT_FOR_SCOPE") {
		t.Fatalf("elapsed approval admitted: %+v %v", r, err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 0 {
		t.Fatal("rejected temporal/reader gate started provider activity")
	}
}

func TestDispositionEvaluationFreezeSerializesLateInputsBothOrders(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	source := requireDispatch(t, f, "source", 0)
	repo := &infrastructure.PostgresRepository{}
	for _, inputFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("input-first=%t", inputFirst), func(t *testing.T) {
			id := uuid.New()
			expected := 1
			if inputFirst {
				expected = 2
			}
			if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_disposition_gate_evaluation(id,workspace_id,plan_id,approval_id,copy_id,actor_id,principal_ref,checked_at,dependency_revision,eligible,blockers,retention_count)
			SELECT $1,workspace_id,plan_id,approval_id,copy_id,actor_id,principal_ref,checked_at,dependency_revision,false,ARRAY['SYNTHETIC'], $3 FROM lifecycle_disposition_gate_evaluation WHERE id=$2`, id, source.EvaluationID, expected); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_disposition_dependency_evaluation SELECT $1,workspace_id,disposal_due_at,inventory_complete,references_released,provider_retention_allows,ledger_current,use_restricted,hold_free,readers_drained,unresolved_dispatches,window_mode,valid_until
			FROM lifecycle_disposition_dependency_evaluation WHERE evaluation_id=$2`, id, source.EvaluationID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_disposition_retention_evaluation SELECT $1,workspace_id,ordinal,policy_id,start_event_id,starts_at,retain_until FROM lifecycle_disposition_retention_evaluation WHERE evaluation_id=$2`, id, source.EvaluationID); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE lifecycle_disposition_gate_evaluation SET status='FINALIZED',content_hash=repeat('b',64) WHERE id=$1`, id); err == nil {
				t.Fatal("forged/incomplete input hash finalized")
			}
			tx1, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx1.Rollback(ctx)
			tx2, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx2.Rollback(ctx)
			var pid int32
			if err := tx2.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			insert := func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_retention_evaluation SELECT $1,workspace_id,1,policy_id,start_event_id,starts_at,retain_until FROM lifecycle_disposition_retention_evaluation WHERE evaluation_id=$2 AND ordinal=0`, id, source.EvaluationID)
				return err
			}
			done := make(chan error, 1)
			if inputFirst {
				if err := insert(tx1); err != nil {
					t.Fatal(err)
				}
				go func() { done <- repo.FinalizeDispatchEvaluation(ctx, tx2, f.w, id) }()
			} else {
				if err := repo.FinalizeDispatchEvaluation(ctx, tx1, f.w, id); err != nil {
					t.Fatal(err)
				}
				go func() { done <- insert(tx2) }()
			}
			waitForPlanLock(t, pool, pid)
			if err := tx1.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-done
			if inputFirst {
				if err != nil {
					t.Fatal(err)
				}
				if err := tx2.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("late input crossed finalization")
				}
				_ = tx2.Rollback(ctx)
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_disposition_retention_evaluation WHERE evaluation_id=$1`, id) != expected {
				t.Fatal("frozen input set incomplete")
			}
			var hashMatches bool
			if err := pool.QueryRow(ctx, `SELECT status='FINALIZED' AND content_hash=lifecycle_gate_inputs_hash(id) FROM lifecycle_disposition_gate_evaluation WHERE id=$1`, id).Scan(&hashMatches); err != nil || !hashMatches {
				t.Fatalf("immutable input hash mismatch: %v", err)
			}
		})
	}
}
