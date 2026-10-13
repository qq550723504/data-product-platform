package application

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"testing"
	"time"
)

func referenceCheck(t *testing.T, pool *pgxpool.Pool, c domain.Copy) domain.Check {
	t.Helper()
	tx, e := pool.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(context.Background())
	result, refs, e := InspectFixtureReferences(context.Background(), tx, c)
	t.Logf("reference check %s: %+v", result, refs)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(context.Background()); e != nil {
		t.Fatal(e)
	}
	return result
}
func TestFixtureReferenceOwnerReleasePreservesHistoryAndBlocksActiveWork(t *testing.T) {
	pool := planDatabase(t)
	w, cs, o, a, _, _, plan := occupancyFixture(t, pool)
	ctx := context.Background()
	ex, _ := subjectExecution(t, pool, w, cs[0], o)
	ref := contentreference.Reference{Kind: "EXECUTION_INPUT", SubjectID: ex.ID, InputName: "fixture"}
	a.caller.Capabilities[ReferenceOwnerCapability("workflow")] = struct{}{}
	svc := NewFixtureReferenceService(o, workflowapp.FixtureReferenceOwner{})
	decl := DeclareFixtureReferenceCommand{CommandHeader: CommandHeader{w, "required-input"}, CopyID: cs[0].ID, Reference: ref, BasisRef: "controlled-source-replay"}
	required, e := svc.DeclareFixtureReference(ctx, decl)
	if e != nil {
		t.Fatal(e)
	}
	replay, e := svc.DeclareFixtureReference(ctx, decl)
	if e != nil || !replay.Replayed || replay.ID != required.ID {
		t.Fatal(replay, e)
	}
	decl.BasisRef = "conflict"
	if _, e = svc.DeclareFixtureReference(ctx, decl); !errors.Is(e, ErrIdempotencyConflict) {
		t.Fatal(e)
	}
	if referenceCheck(t, pool, cs[0]) != domain.Denied {
		t.Fatal("material demand was ignored")
	}
	pin, e := o.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "live-owner"}, CopyID: cs[0].ID, ReaderID: uuid.New()})
	if e != nil {
		t.Fatal(e)
	}
	loss, _ := contentreference.Lookup(ref.Kind)
	release := ReleaseFixtureReferenceCommand{CommandHeader: CommandHeader{w, "release-input"}, RequirementID: required.ID, CopyID: cs[0].ID, Reference: ref, BasisRef: "owner-confirmed-stop-and-loss", AcknowledgedLoss: loss.Loss, OwnerVerifiedQuiescent: true}
	if _, e = svc.ReleaseFixtureReference(ctx, release); e == nil {
		t.Fatal("release without restriction")
	}
	if _, e = o.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "stop-new-use"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "controlled-owner-release"}); e != nil {
		t.Fatal(e)
	}
	if _, e = svc.ReleaseFixtureReference(ctx, release); e == nil {
		t.Fatal("active pin released by owner assertion")
	}
	if occupancyFor(t, o, w, plan.ID, cs[0].ID).ActiveReaders != 1 {
		t.Fatal("release drained active pin")
	}
	if _, e = o.TerminateFixtureReader(ctx, TerminateFixtureReaderCommand{CommandHeader: CommandHeader{w, "verified-stop"}, PinID: pin.Pin.ID, Generation: pin.Pin.Generation, Outcome: domain.ReaderCompleted}); e != nil {
		t.Fatal(e)
	}
	released, e := svc.ReleaseFixtureReference(ctx, release)
	if e != nil {
		t.Fatal(e)
	}
	if referenceCheck(t, pool, cs[0]) != domain.Satisfied {
		t.Fatal("released history still occupies exact copy")
	}
	if referenceCheck(t, pool, cs[1]) != domain.Denied {
		t.Fatal("release expanded to another copy")
	}
	if count(t, pool, `SELECT count(*) FROM execution_input WHERE execution_id=$1 AND dataset_version_id=$2`, ex.ID, cs[0].DatasetVersionID) != 1 {
		t.Fatal("business FK history rewritten")
	}
	for _, table := range []string{"lifecycle_fixture_reference_requirement", "lifecycle_fixture_reference_release"} {
		tx, _ := pool.Begin(ctx)
		if _, e = tx.Exec(ctx, "DELETE FROM "+table); e == nil {
			t.Fatal("historical fact deleted")
		}
		tx.Rollback(ctx)
	}
	for _, event := range []string{ReferenceDeclaredEvent, ReferenceReleasedEvent} {
		if count(t, pool, `SELECT count(*) FROM outbox_event WHERE event_type=$1`, event) != 1 || count(t, pool, `SELECT count(*) FROM audit_event WHERE action=$1`, event) != 1 || count(t, pool, `SELECT count(*) FROM evidence WHERE title=$1`, event) != 1 {
			t.Fatal("atomic minimal facts missing", event)
		}
	}
	replay, e = svc.ReleaseFixtureReference(ctx, release)
	if e != nil || !replay.Replayed || replay.ID != released.ID {
		t.Fatal(replay, e)
	}
	a.denied.Store(true)
	if _, e = svc.ReleaseFixtureReference(ctx, release); e == nil {
		t.Fatal("revoked owner replay")
	}
}
func TestFixtureReferenceRejectsWrongOwnerTenantLossAndNewSchema(t *testing.T) {
	pool := planDatabase(t)
	w, cs, o, a, _, _, _ := occupancyFixture(t, pool)
	ex, _ := subjectExecution(t, pool, w, cs[0], o)
	ref := contentreference.Reference{Kind: "EXECUTION_INPUT", SubjectID: ex.ID, InputName: "fixture"}
	svc := NewFixtureReferenceService(o, workflowapp.FixtureReferenceOwner{})
	cmd := DeclareFixtureReferenceCommand{CommandHeader: CommandHeader{w, "authority"}, CopyID: cs[0].ID, Reference: ref, BasisRef: "fixture"}
	if _, e := svc.DeclareFixtureReference(context.Background(), cmd); e == nil {
		t.Fatal("owner capability absent")
	}
	a.caller.Capabilities[ReferenceOwnerCapability("workflow")] = struct{}{}
	cmd.Reference.InputName = "wrong"
	if _, e := svc.DeclareFixtureReference(context.Background(), cmd); e == nil {
		t.Fatal("wrong source identity")
	}
	cmd.Reference = ref
	cmd.WorkspaceID = uuid.New()
	if _, e := svc.DeclareFixtureReference(context.Background(), cmd); e == nil {
		t.Fatal("cross tenant")
	}
	if _, e := pool.Exec(context.Background(), `CREATE TABLE unmanaged_reference(id uuid PRIMARY KEY,version_id uuid REFERENCES dataset_version(id))`); e != nil {
		t.Fatal(e)
	}
	if referenceCheck(t, pool, cs[0]) != domain.Unknown {
		t.Fatal("new unfenced FK table accepted")
	}
}
func TestFixtureReferenceInsertionAndDecisionSerializeBothOrders(t *testing.T) {
	for _, referenceFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "reference-first", false: "decision-first"}[referenceFirst], func(t *testing.T) {
			pool := planDatabase(t)
			w, cs, o, _, _, _, _ := occupancyFixture(t, pool)
			ex, _ := subjectExecution(t, pool, w, cs[0], o)
			ctx := context.Background()
			tx, e := pool.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			// A transaction that has already observed the source cannot commit a late
			// reference through a later restriction; the trigger locks before admission.
			if _, e = tx.Exec(ctx, `SELECT id FROM dataset_version WHERE id=$1`, cs[0].DatasetVersionID); e != nil {
				t.Fatal(e)
			}
			if referenceFirst {
				if _, e = tx.Exec(ctx, `INSERT INTO execution_input(execution_id,input_name,dataset_version_id) VALUES($1,'late',$2)`, ex.ID, cs[0].DatasetVersionID); e != nil {
					t.Fatal(e)
				}
				conn, e := pool.Acquire(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer conn.Release()
				pid := conn.Conn().PgConn().PID()
				done := make(chan domain.Check, 1)
				errch := make(chan error, 1)
				go func() {
					next, e := conn.Begin(ctx)
					if e != nil {
						errch <- e
						return
					}
					check, _, e := InspectFixtureReferences(ctx, next, cs[0])
					if e == nil {
						e = next.Commit(ctx)
					} else {
						next.Rollback(ctx)
					}
					errch <- e
					done <- check
				}()
				waitForPlanLock(t, pool, int32(pid))
				if e = tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				if e = <-errch; e != nil {
					t.Fatal(e)
				}
				if <-done != domain.Denied {
					t.Fatal("late reference escaped decision")
				}
			} else {
				if _, e = o.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "decision-first"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "controlled"}); e != nil {
					t.Fatal(e)
				}
				if _, e = tx.Exec(ctx, `INSERT INTO execution_input(execution_id,input_name,dataset_version_id) VALUES($1,'late',$2)`, ex.ID, cs[0].DatasetVersionID); e == nil {
					t.Fatal("old transaction crossed restriction")
				}
			}
		})
	}
}

func TestPendingCopyAndDispatchSerializeBothOrders(t *testing.T) {
	for _, copyFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "pending-copy-first", false: "dispatch-first"}[copyFirst], func(t *testing.T) {
			pool := planDatabase(t)
			f := newDisposalFixture(t, pool)
			ctx := context.Background()
			cp := f.copies[0]
			var ns uuid.UUID
			if e := pool.QueryRow(ctx, `SELECT namespace_id FROM lifecycle_fixture_copy WHERE id=$1`, cp.ID).Scan(&ns); e != nil {
				t.Fatal(e)
			}
			policy := uuid.New()
			if _, e := pool.Exec(ctx, `INSERT INTO lifecycle_copy_policy(id,workspace_id,dataset_version_id,basis_ref,use_until,restore_until,actor_id,principal_ref,created_at) VALUES($1,$2,$3,'CONTROLLED_SYNTHETIC_CUSTODY_AND_USE',$4,$4,$5,$6,$7)`, policy, f.w, cp.DatasetVersionID, f.now.Add(time.Hour), f.authority.caller.ActorID, f.authority.caller.Subject, f.now); e != nil {
				t.Fatal(e)
			}
			conn, e := pool.Acquire(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Release()
			tx, e := conn.Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(ctx)
			insert := func() error {
				_, e := tx.Exec(ctx, `INSERT INTO lifecycle_copy_operation(id,workspace_id,source_copy_id,target_copy_id,target_namespace_id,target_key,action,policy_id,basis_ref,actor_id,principal_ref,dependency_revision,created_at,expires_at) VALUES($1,$2,$3,$4,$5,'closed-pending-copy','ARCHIVE',$6,'SQL-only',$7,$8,1,$9,$10)`, uuid.New(), f.w, cp.ID, uuid.New(), ns, policy, f.authority.caller.ActorID, f.authority.caller.Subject, f.now, f.now.Add(time.Minute))
				return e
			}
			if copyFirst {
				if e = insert(); e != nil {
					t.Fatal(e)
				}
				result := make(chan DispatchResult, 1)
				errch := make(chan error, 1)
				go func() {
					r, e := f.s.DispatchContentDisposition(ctx, f.dispatch("copy-before-dispatch", 0))
					result <- r
					errch <- e
				}()
				waitForOccupancyFence(t, pool)
				if e = tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				if e = <-errch; e != nil {
					t.Fatal(e)
				}
				if r := <-result; r.Admitted {
					t.Fatal("pending copy crossed decision")
				}
			} else {
				// Old snapshot is not an admission: dispatch commits before its insert.
				if _, e = tx.Exec(ctx, `SELECT id FROM lifecycle_fixture_copy WHERE id=$1`, cp.ID); e != nil {
					t.Fatal(e)
				}
				requireDispatch(t, f, "dispatch-before-copy", 0)
				if e = insert(); e == nil {
					t.Fatal("late pending copy escaped dispatch")
				}
				tx.Rollback(ctx)
				if _, e = pool.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,'late-target','not-real-provider',repeat('a',64),12)`, uuid.New(), f.w, ns, cp.DatasetVersionID); e == nil {
					t.Fatal("late registered copy escaped dispatch")
				}
			}
		})
	}
}
