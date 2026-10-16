package application

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// Composition preparation only: real PostgreSQL, existing synthetic gates/read
// port/observations. No provider, native DELETE, IAM or retained fixture is used.
func remainingDisposition(t *testing.T, w uuid.UUID, copies []domain.Copy, o *OccupancyService, plan domain.Plan) *disposalFixture {
	t.Helper()
	a := o.authority.(*occupancyAuthority)
	for _, capability := range []string{CapabilityApproveDisposition, CapabilityRevokeApproval, CapabilityDispatch, CapabilityObserveAttempt, CapabilityReconcileDisposition, CapabilityVerifyAbsence, CapabilityInspectStorage} {
		a.caller.Capabilities[capability] = struct{}{}
	}
	f := &disposalFixture{w: w, copies: copies, plan: plan, o: o, authority: a, now: time.Now().UTC().Truncate(time.Microsecond), approver: &disposalApprover{current: domain.Satisfied}, deps: &disposalDependencies{}, verifier: &disposalObservationVerifier{}}
	o.now = func() time.Time { return f.now }
	f.s = NewDispositionService(o, f.approver, f.deps, f.verifier)
	var err error
	f.approval, err = f.s.ApproveContentDisposition(context.Background(), ApproveContentDispositionCommand{CommandHeader: f.header("remaining-approve"), PlanID: plan.ID, BasisRef: "SQL-only; not native authority", ExpiresAt: f.now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func remainingAttempt(t *testing.T, f *disposalFixture, id uuid.UUID) domain.ProviderAttempt {
	t.Helper()
	var attempt domain.ProviderAttempt
	err := f.o.tx.Do(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		attempt, err = f.o.repo.GetAttempt(ctx, tx, f.w, id, false)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func remainingRestrict(t *testing.T, f *disposalFixture) {
	t.Helper()
	if _, err := f.o.RestrictContentUse(context.Background(), RestrictContentUseCommand{CommandHeader: f.header("remaining-restrict"), DatasetVersionID: f.copies[0].DatasetVersionID, BasisRef: "SQL-only"}); err != nil {
		t.Fatal(err)
	}
}

func remainingWait(t *testing.T, ctx context.Context, entered <-chan struct{}, done <-chan error) {
	t.Helper()
	select {
	case <-entered:
	case err := <-done:
		t.Fatal("composition owner exited before barrier", err)
	case <-ctx.Done():
		t.Fatal("composition barrier not reached", ctx.Err())
	}
}

func remainingRelease(release chan struct{}) {
	select {
	case <-release:
	default:
		close(release)
	}
}

func TestRemainingProcessingPinBlocksDispositionUntilOwnerStops(t *testing.T) {
	pool, w, c, o, reader, port, plan := newReaderFixture(t)
	f := remainingDisposition(t, w, []domain.Copy{c}, o, plan)
	o.now = func() time.Time { return time.Now().UTC() } // Reader journal uses actual wall time.
	ex, _ := subjectExecution(t, pool, w, c, o)
	later, _ := subjectExecution(t, pool, w, c, o) // SQL RUNNING subject, not an actual queue delivery.
	pins := NewFixtureSubjectPins(reader, map[uuid.UUID]uuid.UUID{c.DatasetVersionID: c.ID})
	ctx, cancel := context.WithTimeout(readerContext(o), 15*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	defer remainingRelease(release)
	done := make(chan error, 1)
	go func() {
		done <- pins.ExecuteProcessing(ctx, w, ex.ID, func(ctx context.Context) error {
			h, err := pins.ReadVersion(ctx, c.DatasetVersionID)
			if err != nil {
				return err
			}
			_, readErr := io.Copy(io.Discard, h)
			if err = errors.Join(readErr, h.Close()); err != nil {
				return err
			}
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	remainingWait(t, ctx, entered, done)
	remainingRestrict(t, f)
	blocked, err := f.s.DispatchContentDisposition(ctx, f.dispatch("while-processing", 0))
	if err != nil || blocked.Admitted || !hasBlocker(blocked, "READERS_ACTIVE_OR_UNKNOWN") || port.calls.Load() != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 0 {
		t.Fatal("active typed processing owner admitted disposal", blocked, err)
	}
	remainingRelease(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_reader_termination`) != 1 {
		t.Fatal("closed engine did not record exactly one termination")
	}
	admitted := requireDispatch(t, f, "after-processing-stopped", 0)
	if err = NewExactExecution(f.s).ClaimExactSend(ctx, remainingAttempt(t, f, admitted.AttemptID), c); err != nil {
		t.Fatal(err)
	}
	if err = pins.ExecuteProcessing(ctx, w, later.ID, func(context.Context) error { t.Error("next engine entered after reservation"); return nil }); !errors.Is(err, domain.ErrUseBlocked) || port.calls.Load() != 1 {
		t.Fatal("disposition-first processing caused another read", err)
	}
	replay, err := f.s.DispatchContentDisposition(ctx, f.dispatch("while-processing", 0))
	if err != nil || replay.Admitted || !replay.Replayed || replay.EvaluationID != blocked.EvaluationID || count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation`) != 0 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_provider_attempt_id IS NOT NULL`) != 0 {
		t.Fatal("old denial readmitted or SQL claim invented native work", replay, err)
	}
}

func TestRemainingDirectDataTransferBlocksDispositionAndReplay(t *testing.T) {
	pool, w, c, o, reader, port, plan := newReaderFixture(t)
	f := remainingDisposition(t, w, []domain.Copy{c}, o, plan)
	o.now = func() time.Time { return time.Now().UTC() } // Reader journal uses actual wall time.
	pins := NewFixtureSubjectPins(reader, map[uuid.UUID]uuid.UUID{c.DatasetVersionID: c.ID})
	ctx, cancel := context.WithTimeout(readerContext(o), 15*time.Second)
	defer cancel()
	profile := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO certification_profile(id,workspace_id,profile_ref,code,name,version,content_sha256,content_snapshot,purpose_mode,action_mode,consumer_mode,delivery_mode,quality_gate_required,rights_required,compliance_required,contract_required,traceability_required,evidence_required) VALUES($1,$2,$3,$3,'SQL-only remaining direct','1',encode(digest(convert_to('sql-only','UTF8'),'sha256'),'hex'),'sql-only','ANY','ANY','ANY','ANY',false,false,false,false,false,false)`, profile, w, profile.String()); err != nil {
		t.Fatal(err)
	}
	svc := deliveryapp.NewDirectDataService(o.tx, deliveryinfra.NewPostgresRepository(pool), sqlDirectGate{}, datasetinfra.NewPostgresRepository(pool)).WithTransferPins(pins)
	cmd := deliveryapp.DirectDataCommand{WorkspaceID: w, DatasetVersionID: c.DatasetVersionID, ProfileID: profile, PrincipalRef: f.authority.caller.Subject, EffectiveConsumerRef: "SQL_ONLY_CONSUMER", Purpose: "FIXTURE", Action: "READ", ScopeType: "ALL_RESOURCE", IdempotencyKey: "remaining-direct"}
	issued, err := svc.Deliver(ctx, cmd)
	if err != nil || !issued.PayloadReady || issued.TransferPinID == uuid.Nil {
		t.Fatal(issued, err)
	}
	writer := &blockingReaderWriter{entered: make(chan struct{}), release: make(chan struct{})}
	defer remainingRelease(writer.release)
	done := make(chan error, 1)
	go func() { _, err := reader.ReadPinnedTo(ctx, w, issued.TransferPinID, writer); done <- err }()
	remainingWait(t, ctx, writer.entered, done)
	remainingRestrict(t, f)
	blocked, err := f.s.DispatchContentDisposition(ctx, f.dispatch("while-transfer", 0))
	if err != nil || blocked.Admitted || !hasBlocker(blocked, "READERS_ACTIVE_OR_UNKNOWN") || count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 0 {
		t.Fatal("live transfer admitted disposal", blocked, err)
	}
	remainingRelease(writer.release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	requireDispatch(t, f, "after-transfer-stopped", 0)
	replay, err := svc.Deliver(ctx, cmd)
	if !errors.Is(err, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) || replay.PayloadReady || replay.TransferPinID != uuid.Nil {
		t.Fatal("same-key ISSUED replay returned payload", replay, err)
	}
	cmd.IdempotencyKey = "remaining-direct-after-reservation"
	next, err := svc.Deliver(ctx, cmd)
	var admission *pgconn.PgError
	if !errors.As(err, &admission) || admission.Code != "P0001" || admission.Message != "new fixture reference after restriction or disposition" || next.PayloadReady || next.TransferPinID != uuid.Nil || port.calls.Load() != 1 || occupancyFor(t, o, w, plan.ID, c.ID).ActiveReaders != 0 {
		t.Fatal("disposition-first direct admission produced payload/pin/read", next, err)
	}
	if count(t, pool, `SELECT count(*) FROM delivery_operation WHERE id=$1 AND status='ISSUED'`, issued.Operation.ID) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation`) != 0 {
		t.Fatal("transfer changed ISSUED history or invented native deletion")
	}
}

func TestRemainingLateHoldRevokesPlanBeforeAndAfterClaim(t *testing.T) {
	for _, claimFirst := range []bool{false, true} {
		name := "before-claim"
		if claimFirst {
			name = "after-claim"
		}
		t.Run(name, func(t *testing.T) {
			pool := planDatabase(t)
			f := newDisposalFixture(t, pool)
			ctx := context.Background()
			dispatch := requireDispatch(t, f, "late-hold-dispatch", 0)
			attempt := remainingAttempt(t, f, dispatch.AttemptID)
			exact := NewExactExecution(f.s)
			if claimFirst {
				if err := exact.ClaimExactSend(ctx, attempt, f.copies[0]); err != nil {
					t.Fatal(err)
				}
			}
			// Copy A's hold is not a plan-wide stop. Revoke the existing approval
			// first; a crash before the remaining per-copy holds stays fail closed.
			if _, err := f.s.RevokeDispositionApproval(ctx, RevokeDispositionApprovalCommand{CommandHeader: f.header("late-hold-stop-plan"), ApprovalID: f.approval.ID, BasisRef: "SQL-only preservation request"}); err != nil {
				t.Fatal(err)
			}
			b, err := f.s.DispatchContentDisposition(ctx, f.dispatch("after-plan-stop-before-b-hold", 1))
			if err != nil || b.Admitted || !hasBlocker(b, "APPROVAL_NOT_CURRENT_FOR_SCOPE") {
				t.Fatal("plan revocation did not stop unheld member B", b, err)
			}
			hold, err := f.o.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: f.header("late-hold-a"), CopyID: f.copies[0].ID, BasisRef: "SQL-only"})
			if err != nil || !hold.PreservationUnconfirmed || count(t, pool, `SELECT count(*) FROM lifecycle_hold_dispatch_conflict`) != 1 {
				t.Fatal("late hold claimed proven preservation", hold, err)
			}
			if err = exact.ClaimExactSend(ctx, attempt, f.copies[0]); !errors.Is(err, ErrStorageDenied) {
				t.Fatal("revoked/held attempt obtained another claim")
			}
			if count(t, pool, `SELECT count(*) FROM cost_event`) != 0 {
				t.Fatal("dispatch/claim was counted as confirmed invocation")
			}
			if claimFirst {
				// Synthetic confirmed invocation after claim; NOT a native send.
				f.now = f.now.Add(2 * time.Second)
				proof := RecordProviderObservationCommand{CommandHeader: f.header("late-hold-unknown"), AttemptID: attempt.ID, ObservationRef: "timeout"}
				first, err := f.s.RecordProviderObservation(ctx, proof)
				if err != nil {
					t.Fatal(err)
				}
				replay, err := f.s.RecordProviderObservation(ctx, proof)
				if err != nil || !replay.Replayed || replay.ID != first.ID || count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation WHERE outcome='UNKNOWN'`) != 1 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_provider_attempt_id=$1`, attempt.ID) != 1 {
					t.Fatal("late hold lost unknown/cost or replay duplicated it", replay, err)
				}
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_dispatch_release`) != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_plan_completion`) != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 1 {
				t.Fatal("late hold released unknown scope or started member B")
			}
		})
	}
}

func TestRemainingThreeCopyMixedOutcomesNeverCompleteRetainedScope(t *testing.T) {
	for _, bPresent := range []bool{true, false} {
		name := "timeout-then-absent"
		if bPresent {
			name = "timeout-then-present"
		}
		t.Run(name, func(t *testing.T) {
			pool := planDatabase(t)
			w, initial, o, a, _, _, _ := occupancyFixture(t, pool)
			ctx := context.Background()
			copies := []domain.Copy{initial[0]}
			var namespace uuid.UUID
			if err := pool.QueryRow(ctx, `SELECT namespace_id FROM lifecycle_fixture_copy WHERE id=$1`, initial[0].ID).Scan(&namespace); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"sql-only/mixed-b", "sql-only/mixed-c"} {
				c := initial[0] // Same DatasetVersion/hash, distinct exact copies.
				c.ID, c.Key, c.ProviderVersion = uuid.New(), key, uuid.NewString()
				if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.ID, w, namespace, c.DatasetVersionID, c.Key, c.ProviderVersion, c.ContentSHA256, c.ByteSize); err != nil {
					t.Fatal(err)
				}
				copies = append(copies, c)
			}
			a.caller.Capabilities[CapabilityPreparePlan] = struct{}{}
			plan, err := NewPlanService(o.tx, o.repo, &testAuthority{caller: a.caller}).PrepareDeletePlan(ctx, PrepareDeletePlanCommand{WorkspaceID: w, CopyIDs: []uuid.UUID{copies[0].ID, copies[1].ID, copies[2].ID}, IdempotencyKey: "mixed-three-copy-plan"})
			if err != nil {
				t.Fatal(err)
			}
			f := remainingDisposition(t, w, copies, o, plan)
			f.deps.hook = func(_ context.Context, _ pgx.Tx, r DisposalDependencyRequest, d *DisposalDependencies) error {
				if r.Copy.ID == copies[2].ID {
					d.ProviderRetentionAllows = domain.Denied
				}
				return nil
			}
			remainingRestrict(t, f)
			first := requireDispatch(t, f, "mixed-a", 0)
			second := requireDispatch(t, f, "mixed-b", 1)
			f.now = f.now.Add(2 * time.Second)
			for _, cmd := range []RecordProviderObservationCommand{{CommandHeader: f.header("mixed-a-accepted"), AttemptID: first.AttemptID, ObservationRef: "accepted"}, {CommandHeader: f.header("mixed-b-timeout"), AttemptID: second.AttemptID, ObservationRef: "timeout"}} {
				if _, err := f.s.RecordProviderObservation(ctx, cmd); err != nil {
					t.Fatal(err)
				}
			}
			blocked, err := f.s.DispatchContentDisposition(ctx, f.dispatch("mixed-c-retention", 2))
			if err != nil || blocked.Admitted || !hasBlocker(blocked, "PROVIDER_RETENTION_BLOCKED_OR_UNKNOWN") {
				t.Fatal("retained copy C admitted", blocked, err)
			}
			inspector := &recoveryInspector{f: f, journal: NewReconciliationStorageCallJournal(o)}
			recovery := NewRecoveryService(NewExactExecution(f.s), inspector, transaction.NewManager(pool), nil, []uuid.UUID{w})
			if err = recovery.RunRecovery(ctx, w, recoveryWork(t, pool, first.AttemptID)); err != nil {
				t.Fatal(err)
			}
			// Capture mixed state BEFORE resolving B; timeout does not imply present.
			if count(t, pool, `SELECT count(*) FROM lifecycle_copy_absence_receipt WHERE plan_id=$1`, plan.ID) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation WHERE attempt_id=$1 AND outcome='UNKNOWN'`, second.AttemptID) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_plan_completion`) != 0 {
				t.Fatal("mixed checkpoint lost partial/unknown facts")
			}
			inspector.present = bPresent
			err = recovery.RunRecovery(ctx, w, recoveryWork(t, pool, second.AttemptID))
			if (bPresent && !errors.Is(err, ErrStorageUnknown)) || (!bPresent && err != nil) {
				t.Fatal("wrong timeout reconciliation result", err)
			}
			_, err = NewExactExecution(f.s).FinalizeContentDisposition(ctx, f.header("mixed-finalize"), plan.ID)
			var incomplete *pgconn.PgError
			if !errors.As(err, &incomplete) || incomplete.Code != "P0001" || incomplete.Message != "completion requires exact frozen receipt membership" || count(t, pool, `SELECT count(*) FROM lifecycle_plan_completion`) != 0 {
				t.Fatal("partial scope completed while C retained", err)
			}
			before := inspector.calls.Load()
			if err = recovery.RunRecovery(ctx, w, recoveryWork(t, pool, first.AttemptID)); err != nil || inspector.calls.Load() != before {
				t.Fatal("terminal A replay performed lookup", err)
			}
			if count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 2 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_send_claim`) != 0 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_provider_attempt_id IS NOT NULL`) != 2 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != int(inspector.calls.Load()) || count(t, pool, `SELECT count(*) FROM lifecycle_provider_observation WHERE attempt_id=$1 AND outcome='UNKNOWN'`, second.AttemptID) != 1 {
				t.Fatal("retry, lost original UNKNOWN, or cost identity mismatch")
			}
		})
	}
}
