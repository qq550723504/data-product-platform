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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// These ports supply controlled server facts for application contract tests.
// They do not prove a live principal, real reader quiescence or provider state.
type occupancyAuthority struct {
	caller principal.RequestPrincipal
	denied atomic.Bool
	hook   func(context.Context, pgx.Tx, string) error
}

func (a *occupancyAuthority) AuthorizeLifecycle(ctx context.Context, tx pgx.Tx, _ uuid.UUID, capability string, _ time.Time) (principal.RequestPrincipal, error) {
	if a.denied.Load() {
		return principal.RequestPrincipal{}, principal.ErrUnauthenticated
	}
	if a.hook != nil {
		if err := a.hook(ctx, tx, capability); err != nil {
			return principal.RequestPrincipal{}, err
		}
	}
	return a.caller, nil
}

type occupancyUseGate struct {
	calls atomic.Int64
	hook  func(context.Context, pgx.Tx, UseGateRequest, *domain.UseDecision) error
}

func (g *occupancyUseGate) EvaluateFixtureUse(ctx context.Context, tx pgx.Tx, r UseGateRequest) (domain.UseDecision, error) {
	g.calls.Add(1)
	d := domain.UseDecision{WorkspaceID: r.Copy.WorkspaceID, CopyID: r.Copy.ID, ReaderID: r.ReaderID,
		ActorID: r.Caller.ActorID, CheckedAt: r.AsOf, Decision: domain.Satisfied, WindowMode: domain.UseUnbounded}
	if g.hook != nil {
		if err := g.hook(ctx, tx, r, &d); err != nil {
			return domain.UseDecision{}, err
		}
	}
	return d, nil
}

type occupancyTermination struct {
	calls atomic.Int64
	hook  func(*domain.ReaderTermination)
}

func (v *occupancyTermination) VerifyTermination(_ context.Context, _ pgx.Tx, pin domain.ReaderPin, outcome string, at time.Time) (domain.ReaderTermination, error) {
	v.calls.Add(1)
	p := domain.ReaderTermination{ID: uuid.New(), WorkspaceID: pin.WorkspaceID, PinID: pin.ID,
		ReaderID: pin.ReaderID, Generation: pin.Generation, Outcome: outcome, ObservedAt: at}
	if v.hook != nil {
		v.hook(&p)
	}
	return p, nil
}

func occupancyFixture(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, []domain.Copy, *OccupancyService, *occupancyAuthority, *occupancyUseGate, *occupancyTermination, domain.Plan) {
	t.Helper()
	w, copies, planAuthority := seedPlanFixture(t, pool)
	tx, repo := transaction.NewManager(pool), &infrastructure.PostgresRepository{}
	plan, err := NewPlanService(tx, repo, planAuthority).PrepareDeletePlan(context.Background(), PrepareDeletePlanCommand{
		WorkspaceID: w, CopyIDs: []uuid.UUID{copies[0].ID, copies[1].ID}, IdempotencyKey: "occupancy-plan"})
	if err != nil {
		t.Fatal(err)
	}
	p := planAuthority.caller
	p.Capabilities = map[string]struct{}{CapabilityRestrictUse: {}, CapabilityPlaceHold: {}, CapabilityReleaseHold: {},
		CapabilityAdmitReader: {}, CapabilityTerminateReader: {}, CapabilityInspectOccupancy: {}}
	a, g, v := &occupancyAuthority{caller: p}, &occupancyUseGate{}, &occupancyTermination{}
	return w, copies, NewOccupancyService(tx, repo, a, g, v), a, g, v, plan
}

func occupancyFor(t *testing.T, s *OccupancyService, w, planID, copyID uuid.UUID) domain.Occupancy {
	t.Helper()
	p, err := s.InspectPlanOccupancy(context.Background(), w, planID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Copies {
		if c.CopyID == copyID {
			return c
		}
	}
	t.Fatal("copy missing from frozen plan inspection")
	return domain.Occupancy{}
}

func TestOccupancyIndependentHoldsRestrictionPinExpiryAndVerifiedTermination(t *testing.T) {
	pool := planDatabase(t)
	w, copies, s, a, gate, verifier, plan := occupancyFixture(t, pool)
	ctx := context.Background()
	now := time.Date(2026, 10, 11, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	h := func(key string) CommandHeader { return CommandHeader{WorkspaceID: w, IdempotencyKey: key} }
	var holds []FactResult
	for i := 0; i < 2; i++ {
		r, err := s.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: h(fmt.Sprintf("hold-%d", i)), CopyID: copies[0].ID, BasisRef: "synthetic-hold-basis"})
		if err != nil {
			t.Fatal(err)
		}
		holds = append(holds, r)
	}
	gate.hook = func(_ context.Context, _ pgx.Tx, r UseGateRequest, d *domain.UseDecision) error {
		until := r.AsOf.Add(time.Minute)
		d.WindowMode, d.UseUntil = domain.UseDeadline, &until
		return nil
	}
	admit := AdmitFixtureReaderCommand{CommandHeader: h("reader-1"), CopyID: copies[0].ID, ReaderID: uuid.New()}
	reader, err := s.AdmitFixtureReader(ctx, admit)
	if err != nil || !reader.Admitted || reader.Replayed {
		t.Fatalf("hold should preserve a separately authorized read: %+v %v", reader, err)
	}
	replay, err := s.AdmitFixtureReader(ctx, admit)
	if err != nil || replay.Admitted || !replay.Replayed || replay.Pin.ID != reader.Pin.ID || gate.calls.Load() != 1 {
		t.Fatalf("historical replay started another read: %+v %v", replay, err)
	}
	restrict := RestrictContentUseCommand{CommandHeader: h("restrict"), DatasetVersionID: copies[0].DatasetVersionID, BasisRef: "synthetic-restriction-basis"}
	if _, err := s.RestrictContentUse(ctx, restrict); err != nil {
		t.Fatal(err)
	}
	// Deadline passage and heartbeat absence cannot release an in-flight reader.
	now = now.Add(time.Hour)
	o := occupancyFor(t, s, w, plan.ID, copies[0].ID)
	if o.ActiveHolds != 2 || o.HoldFree != domain.Denied || o.ActiveReaders != 1 || o.ReadersDrained != domain.Denied || o.UseRestricted != domain.Satisfied {
		t.Fatalf("lost independent occupancy: %+v", o)
	}
	for _, c := range copies {
		if _, err := s.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: h("blocked-" + c.ID.String()), CopyID: c.ID, ReaderID: uuid.New()}); !errors.Is(err, domain.ErrUseBlocked) {
			t.Fatalf("version restriction failed on copy %s: %v", c.ID, err)
		}
	}
	replay, err = s.AdmitFixtureReader(ctx, admit)
	if err != nil || replay.Admitted || !replay.Replayed || gate.calls.Load() != 1 {
		t.Fatalf("post-restriction replay reauthorized payload: %+v %v", replay, err)
	}
	changed := admit
	changed.ReaderID = uuid.New()
	if _, err := s.AdmitFixtureReader(ctx, changed); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key accepted changed reader identity: %v", err)
	}
	closeCmd := TerminateFixtureReaderCommand{CommandHeader: h("reader-close"), PinID: reader.Pin.ID, Generation: reader.Pin.Generation, Outcome: domain.ReaderCompleted}
	stale := closeCmd
	stale.Generation++
	if _, err := s.TerminateFixtureReader(ctx, stale); !errors.Is(err, domain.ErrTerminationUnknown) || verifier.calls.Load() != 0 {
		t.Fatalf("stale generation accepted: %v", err)
	}
	verifier.hook = func(p *domain.ReaderTermination) { p.Outcome = "TIMEOUT" }
	if _, err := s.TerminateFixtureReader(ctx, closeCmd); !errors.Is(err, domain.ErrTerminationUnknown) {
		t.Fatalf("timeout treated as stopped I/O: %v", err)
	}
	verifier.hook = func(p *domain.ReaderTermination) { p.Generation++ }
	if _, err := s.TerminateFixtureReader(ctx, closeCmd); !errors.Is(err, domain.ErrTerminationUnknown) {
		t.Fatalf("foreign verifier generation accepted: %v", err)
	}
	if occupancyFor(t, s, w, plan.ID, copies[0].ID).ActiveReaders != 1 {
		t.Fatal("unconfirmed termination released pin")
	}
	verifier.hook = nil
	closed, err := s.TerminateFixtureReader(ctx, closeCmd)
	if err != nil {
		t.Fatal(err)
	}
	closedReplay, err := s.TerminateFixtureReader(ctx, closeCmd)
	if err != nil || !closedReplay.Replayed || closedReplay.ID != closed.ID || verifier.calls.Load() != 3 {
		t.Fatalf("termination replay repeated proof: %+v %v", closedReplay, err)
	}
	for i, hold := range holds {
		cmd := ReleaseContentHoldCommand{CommandHeader: h(fmt.Sprintf("release-%d", i)), HoldID: hold.ID, BasisRef: "synthetic-release-basis"}
		released, err := s.ReleaseContentHold(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		r, err := s.ReleaseContentHold(ctx, cmd)
		if err != nil || !r.Replayed || r.ID != released.ID {
			t.Fatalf("release replay: %+v %v", r, err)
		}
		cmd.BasisRef = "changed-basis"
		if _, err := s.ReleaseContentHold(ctx, cmd); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("modified same-key release accepted: %v", err)
		}
		cmd.CommandHeader = h(fmt.Sprintf("release-again-%d", i))
		if _, err := s.ReleaseContentHold(ctx, cmd); !errors.Is(err, infrastructure.ErrAlreadyReleased) {
			t.Fatalf("duplicate release accepted: %v", err)
		}
		o = occupancyFor(t, s, w, plan.ID, copies[0].ID)
		if o.ActiveHolds != int64(1-i) || o.ActiveReaders != 0 || o.ReadersDrained != domain.Satisfied || o.UseRestricted != domain.Satisfied {
			t.Fatalf("release erased other facts: %+v", o)
		}
	}
	// Backward wall clock movement must not reopen an immediate restriction.
	now = now.Add(-2 * time.Hour)
	if _, err := s.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: h("clock-rollback"), CopyID: copies[0].ID, ReaderID: uuid.New()}); !errors.Is(err, domain.ErrUseBlocked) {
		t.Fatalf("clock rollback reopened content: %v", err)
	}
	a.denied.Store(true)
	if _, err := s.AdmitFixtureReader(ctx, admit); !errors.Is(err, principal.ErrUnauthenticated) {
		t.Fatalf("replay bypassed revoked authority: %v", err)
	}
	for event, expected := range map[string]int{ContentRestrictedEvent: 1, HoldPlacedEvent: 2, HoldReleasedEvent: 2, ReaderAdmittedEvent: 1, ReaderTerminatedEvent: 1} {
		for _, query := range []string{`SELECT count(*) FROM outbox_event WHERE event_type=$1`, `SELECT count(*) FROM audit_event WHERE action=$1`, `SELECT count(*) FROM evidence WHERE title=$1`} {
			if n := count(t, pool, query, event); n != expected {
				t.Fatalf("atomic/idempotent history %s=%d want=%d", event, n, expected)
			}
		}
	}
	if count(t, pool, `SELECT count(*) FROM cost_event`) != 0 {
		t.Fatal("invented provider cost without invocation")
	}
	var payloads string
	if err := pool.QueryRow(ctx, `SELECT string_agg(payload::text,',') FROM outbox_event WHERE event_type <> 'LifecycleDispositionPlanPrepared'`).Scan(&payloads); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"synthetic-hold-basis", "synthetic-release-basis", "synthetic-restriction-basis", "sql-only", "provider_version", "storage_uri"} {
		if strings.Contains(payloads, forbidden) {
			t.Fatalf("sensitive detail leaked into event: %s", forbidden)
		}
	}
}

func TestOccupancyRejectsForeignScopeUnknownGateAndPartialFacts(t *testing.T) {
	pool := planDatabase(t)
	w, copies, s, _, gate, _, _ := occupancyFixture(t, pool)
	foreignW, foreignCopies, _ := seedPlanFixture(t, pool)
	ctx := context.Background()
	h := func(key string) CommandHeader { return CommandHeader{WorkspaceID: w, IdempotencyKey: key} }
	if _, err := s.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: h("foreign-hold"), CopyID: foreignCopies[0].ID, BasisRef: "basis"}); !errors.Is(err, infrastructure.ErrScopeUnavailable) {
		t.Fatalf("foreign hold accepted: %v", err)
	}
	if _, err := s.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: h("foreign-restriction"), DatasetVersionID: foreignCopies[0].DatasetVersionID, BasisRef: "basis"}); !errors.Is(err, infrastructure.ErrScopeUnavailable) {
		t.Fatalf("foreign restriction accepted: %v", err)
	}
	if _, err := s.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: h("foreign-reader"), CopyID: foreignCopies[0].ID, ReaderID: uuid.New()}); !errors.Is(err, infrastructure.ErrScopeUnavailable) || gate.calls.Load() != 0 {
		t.Fatalf("foreign reader reached use gate: %v", err)
	}
	for _, mode := range []string{"unknown", "expired", "wrong-copy"} {
		gate.hook = func(_ context.Context, _ pgx.Tx, r UseGateRequest, d *domain.UseDecision) error {
			switch mode {
			case "unknown":
				d.Decision = domain.Unknown
			case "expired":
				d.WindowMode, d.UseUntil = domain.UseDeadline, &r.AsOf
			case "wrong-copy":
				d.CopyID = uuid.New()
			}
			return nil
		}
		if _, err := s.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: h(mode), CopyID: copies[0].ID, ReaderID: uuid.New()}); !errors.Is(err, domain.ErrUseBlocked) {
			t.Fatalf("%s gate accepted: %v", mode, err)
		}
	}
	gate.hook = nil
	outbox.ConfigureAppendObligation(nil)
	for _, run := range []func() error{
		func() error {
			_, err := s.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: h("rollback-hold"), CopyID: copies[0].ID, BasisRef: "basis"})
			return err
		},
		func() error {
			_, err := s.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: h("rollback-restrict"), DatasetVersionID: copies[0].DatasetVersionID, BasisRef: "basis"})
			return err
		},
		func() error {
			_, err := s.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: h("rollback-reader"), CopyID: copies[0].ID, ReaderID: uuid.New()})
			return err
		},
	} {
		if err := run(); err == nil {
			t.Fatal("missing required routing did not roll back command")
		}
	}
	for _, query := range []string{`SELECT count(*) FROM lifecycle_content_hold`, `SELECT count(*) FROM lifecycle_content_use_restriction`, `SELECT count(*) FROM lifecycle_reader_pin`,
		`SELECT count(*) FROM command_idempotency WHERE command_type <> 'LIFECYCLE.PREPARE_DELETE_PLAN'`,
		`SELECT count(*) FROM evidence WHERE evidence_type='LIFECYCLE_OCCUPANCY'`, `SELECT count(*) FROM outbox_event WHERE event_type <> 'LifecycleDispositionPlanPrepared'`} {
		if count(t, pool, query) != 0 {
			t.Fatalf("partial current control fact: %s", query)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_content_hold(id,workspace_id,copy_id,basis_ref,actor_id,principal_ref,dependency_revision,occurred_at)
		VALUES(gen_random_uuid(),$1,$2,'basis',gen_random_uuid(),'synthetic',1,now())`, foreignW, copies[0].ID); err == nil {
		t.Fatal("cross-workspace hold FK accepted")
	}
}

// Observe a real contender waiting on the shared fence, rather than infer a
// race from elapsed time. The blocker holds it while a test port is paused.
func waitForOccupancyFence(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
			WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%delivery_authorization_fence%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("contender did not reach the shared delivery fence")
		case <-ticker.C:
		}
	}
}

func TestOccupancyReaderAndRestrictionSerializeBothOrders(t *testing.T) {
	for _, readerFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("reader-first=%t", readerFirst), func(t *testing.T) {
			pool := planDatabase(t)
			w, copies, s, authority, gate, _, plan := occupancyFixture(t, pool)
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
			if readerFirst {
				gate.hook = func(_ context.Context, _ pgx.Tx, _ UseGateRequest, _ *domain.UseDecision) error { return pause() }
			} else {
				var calls atomic.Int64
				authority.hook = func(_ context.Context, _ pgx.Tx, capability string) error {
					if capability == CapabilityRestrictUse && calls.Add(1) == 2 {
						return pause()
					}
					return nil
				}
			}
			readDone, restrictDone := make(chan error, 1), make(chan error, 1)
			read := func() {
				_, err := s.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "race-read"}, CopyID: copies[0].ID, ReaderID: uuid.New()})
				readDone <- err
			}
			restrict := func() {
				_, err := s.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "race-restrict"}, DatasetVersionID: copies[0].DatasetVersionID, BasisRef: "basis"})
				restrictDone <- err
			}
			if readerFirst {
				go read()
			} else {
				go restrict()
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("first command did not enter locked test port")
			}
			if readerFirst {
				go restrict()
			} else {
				go read()
			}
			waitForOccupancyFence(t, pool)
			close(release)
			if err := <-restrictDone; err != nil {
				t.Fatal(err)
			}
			readErr := <-readDone
			wantReaders := int64(0)
			if readerFirst {
				wantReaders = 1
				if readErr != nil {
					t.Fatal(readErr)
				}
			} else if !errors.Is(readErr, domain.ErrUseBlocked) || gate.calls.Load() != 0 {
				t.Fatalf("restriction-first allowed a new reader: %v", readErr)
			}
			o := occupancyFor(t, s, w, plan.ID, copies[0].ID)
			if o.ActiveReaders != wantReaders || o.UseRestricted != domain.Satisfied {
				t.Fatalf("race lost durable occupancy: %+v", o)
			}
		})
	}
}

func TestOccupancyInspectionWaitsForNewHoldAndDBFactsAreImmutable(t *testing.T) {
	pool := planDatabase(t)
	w, copies, s, authority, _, _, plan := occupancyFixture(t, pool)
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
	var calls atomic.Int64
	authority.hook = func(_ context.Context, _ pgx.Tx, capability string) error {
		if capability == CapabilityPlaceHold && calls.Add(1) == 2 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	holdDone := make(chan error, 1)
	go func() {
		_, err := s.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: CommandHeader{w, "racing-hold"}, CopyID: copies[0].ID, BasisRef: "basis"})
		holdDone <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("hold did not enter locked port")
	}
	inspectDone := make(chan PlanOccupancy, 1)
	inspectErr := make(chan error, 1)
	go func() { p, err := s.InspectPlanOccupancy(ctx, w, plan.ID); inspectDone <- p; inspectErr <- err }()
	waitForOccupancyFence(t, pool)
	close(release)
	if err := <-holdDone; err != nil {
		t.Fatal(err)
	}
	p := <-inspectDone
	if err := <-inspectErr; err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range p.Copies {
		if c.CopyID == copies[0].ID {
			found = c.ActiveHolds == 1 && c.HoldFree == domain.Denied
		}
	}
	if !found {
		t.Fatal("inspection missed a committed hold")
	}
	admitted, err := s.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "db-reader"}, CopyID: copies[0].ID, ReaderID: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]any{
		{uuid.New(), admitted.Pin.ReaderID, admitted.Pin.Generation, admitted.Pin.StartedAt},
		{w, uuid.New(), admitted.Pin.Generation, admitted.Pin.StartedAt},
		{w, admitted.Pin.ReaderID, admitted.Pin.Generation + 1, admitted.Pin.StartedAt},
		{w, admitted.Pin.ReaderID, admitted.Pin.Generation, admitted.Pin.StartedAt.Add(-time.Second)},
	} {
		if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_reader_termination(id,workspace_id,pin_id,reader_id,generation,outcome,observed_at,actor_id,principal_ref,dependency_revision)
			VALUES(gen_random_uuid(),$1,$5,$2,$3,'COMPLETED',$4,gen_random_uuid(),'synthetic',10)`, append(args, admitted.Pin.ID)...); err == nil {
			t.Fatal("DB accepted termination for the wrong pin context")
		}
	}
	if _, err := s.TerminateFixtureReader(ctx, TerminateFixtureReaderCommand{CommandHeader: CommandHeader{w, "db-close"}, PinID: admitted.Pin.ID, Generation: admitted.Pin.Generation, Outcome: domain.ReaderCompleted}); err != nil {
		t.Fatal(err)
	}
	var holdID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM lifecycle_content_hold`).Scan(&holdID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_content_hold_release(id,workspace_id,hold_id,basis_ref,actor_id,principal_ref,dependency_revision,occurred_at)
		SELECT gen_random_uuid(),workspace_id,id,'basis',actor_id,principal_ref,10,occurred_at-interval '1 second' FROM lifecycle_content_hold WHERE id=$1`, holdID); err == nil {
		t.Fatal("DB accepted release before hold placement")
	}
	if _, err := s.ReleaseContentHold(ctx, ReleaseContentHoldCommand{CommandHeader: CommandHeader{w, "db-release"}, HoldID: holdID, BasisRef: "basis"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "db-restrict"}, DatasetVersionID: copies[0].DatasetVersionID, BasisRef: "basis"}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"lifecycle_content_use_restriction", "lifecycle_content_hold", "lifecycle_content_hold_release", "lifecycle_reader_pin", "lifecycle_reader_termination"} {
		for _, sql := range []string{"UPDATE " + table + " SET principal_ref='rewrite'", "DELETE FROM " + table, "TRUNCATE " + table + " CASCADE"} {
			if _, err := pool.Exec(ctx, sql); err == nil {
				t.Fatalf("history erased: %s", sql)
			}
		}
	}
}
