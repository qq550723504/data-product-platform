package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func renewalWaiter(t *testing.T, pool *pgxpool.Pool, blocking int) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var pid int
		if e := pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) LIMIT 1`, blocking).Scan(&pid); e == nil {
			return pid
		}
		select {
		case <-ctx.Done():
			t.Fatal("real PG waiter not observed")
			return 0
		case <-tick.C:
		}
	}
}

func TestCopyPolicyRenewalFenceBothOrdersAndFrozenHistory(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	for _, kind := range []string{"restriction", "hold", "pin"} {
		for _, first := range []string{"renew", "other"} {
			t.Run(kind+"-"+first, func(t *testing.T) {
				_, w, cs, s, _, _ := newCopyFixtureOnPool(t, pool)
				s.occupancy.useGate = s
				var old uuid.UUID
				if e := pool.QueryRow(ctx, `SELECT id FROM lifecycle_current_copy_policy WHERE workspace_id=$1`, w).Scan(&old); e != nil {
					t.Fatal(e)
				}
				cmd := RenewFixtureCopyPolicyCommand{CommandHeader: CommandHeader{w, "renew-race"}, DatasetVersionID: cs[0].DatasetVersionID, PreviousPolicyID: old, UseUntil: time.Now().Add(25 * time.Minute), RestoreUntil: time.Now().Add(25 * time.Minute)}
				blocker, e := pool.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer blocker.Rollback(ctx)
				if _, e = blocker.Exec(ctx, `SELECT lifecycle_reference_lock($1,$2)`, w, cs[0].DatasetVersionID); e != nil {
					t.Fatal(e)
				}
				var pid int
				if e = blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); e != nil {
					t.Fatal(e)
				}
				renew := func() error { _, e := s.RenewFixtureCopyPolicy(ctx, cmd); return e }
				var pin ReaderAdmission
				other := func() error {
					var e error
					switch kind {
					case "restriction":
						_, e = s.occupancy.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "other"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "SQL-race"})
					case "hold":
						_, e = s.occupancy.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: CommandHeader{w, "other"}, CopyID: cs[0].ID, BasisRef: "SQL-race"})
					case "pin":
						pin, e = s.occupancy.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "other"}, CopyID: cs[0].ID, ReaderID: uuid.New()})
					}
					return e
				}
				one, two := renew, other
				if first == "other" {
					one, two = other, renew
				}
				a, b := make(chan error, 1), make(chan error, 1)
				go func() { a <- one() }()
				waiter := renewalWaiter(t, pool, pid)
				go func() { b <- two() }()
				second := renewalWaiter(t, pool, waiter)
				t.Logf("real waiters blocker=%d first=%d second=%d", pid, waiter, second)
				if e = blocker.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				if e = <-a; e != nil {
					t.Fatal("first command", e)
				}
				e = <-b
				if (first == "renew" && e != nil) || (first == "other" && e == nil) {
					t.Fatal("wrong serialization result", e)
				}
				if kind == "pin" {
					if !pin.Admitted || pin.Pin.UseUntil == nil {
						t.Fatal("pin not bounded", pin)
					}
					var before string
					if e = pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM lifecycle_reader_pin p WHERE id=$1`, pin.Pin.ID).Scan(&before); e != nil {
						t.Fatal(e)
					}
					if _, e = s.occupancy.TerminateFixtureReader(ctx, TerminateFixtureReaderCommand{CommandHeader: CommandHeader{w, "stop"}, PinID: pin.Pin.ID, Generation: pin.Pin.Generation, Outcome: domain.ReaderCompleted}); e != nil {
						t.Fatal(e)
					}
					if first == "other" {
						cmd.IdempotencyKey = "after-stop"
						if _, e = s.RenewFixtureCopyPolicy(ctx, cmd); e != nil {
							t.Fatal(e)
						}
					}
					var after string
					if e = pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM lifecycle_reader_pin p WHERE id=$1`, pin.Pin.ID).Scan(&after); e != nil || before != after {
						t.Fatal("old pin changed", e)
					}
				}
			})
		}
	}
	// Historical operation and independent checkpoint retain the old policy.
	_, w, cs, s, _, _ := newCopyFixtureOnPool(t, pool)
	op := stagedCopy(t, s, w, cs[0].ID, "archive")
	cut, e := s.ExportRestoreCheckpoint(ctx, ExportRestoreCheckpointCommand{CommandHeader: CommandHeader{w, "old-cut"}, DatasetVersionID: cs[0].DatasetVersionID})
	if e != nil {
		t.Fatal(e)
	}
	var before string
	if e = pool.QueryRow(ctx, `SELECT row_to_json(o)::text FROM lifecycle_copy_operation o WHERE id=$1`, op.ID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	if _, e = s.RenewFixtureCopyPolicy(ctx, RenewFixtureCopyPolicyCommand{CommandHeader: CommandHeader{w, "new-head"}, DatasetVersionID: cs[0].DatasetVersionID, PreviousPolicyID: op.PolicyID, UseUntil: time.Now().Add(25 * time.Minute), RestoreUntil: time.Now().Add(25 * time.Minute)}); e != nil {
		t.Fatal(e)
	}
	var after, hash string
	if e = pool.QueryRow(ctx, `SELECT row_to_json(o)::text FROM lifecycle_copy_operation o WHERE id=$1`, op.ID).Scan(&after); e != nil || before != after {
		t.Fatal("old operation changed", e)
	}
	if e = pool.QueryRow(ctx, `SELECT content_sha256 FROM lifecycle_restore_checkpoint WHERE id=$1`, cut.ID).Scan(&hash); e != nil || hash != cut.ContentSHA256 {
		t.Fatal("old checkpoint changed", e)
	}
}

func TestCopyPolicyRenewalCrossScopeAndPendingOutputFailClosed(t *testing.T) {
	pool := planDatabase(t)
	ctx := context.Background()
	_, w, cs, s, _, _ := newCopyFixtureOnPool(t, pool)
	var old uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT id FROM lifecycle_current_copy_policy WHERE workspace_id=$1`, w).Scan(&old); e != nil {
		t.Fatal(e)
	}
	_, foreign, _, _, _, _ := newCopyFixtureOnPool(t, pool)
	var foreignID uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT id FROM lifecycle_current_copy_policy WHERE workspace_id=$1`, foreign).Scan(&foreignID); e != nil {
		t.Fatal(e)
	}
	for _, previous := range []uuid.UUID{uuid.New(), foreignID} {
		if _, e := s.RenewFixtureCopyPolicy(ctx, RenewFixtureCopyPolicyCommand{CommandHeader: CommandHeader{w, "cross-" + previous.String()}, DatasetVersionID: cs[0].DatasetVersionID, PreviousPolicyID: previous, UseUntil: time.Now().Add(time.Minute), RestoreUntil: time.Now().Add(time.Minute)}); e == nil {
			t.Fatal("cross/stale expected head accepted")
		}
	}
	ex, _ := subjectExecution(t, pool, w, cs[0], s.occupancy)
	output := uuid.New()
	var ns uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT namespace_id FROM lifecycle_fixture_copy WHERE id=$1`, cs[0].ID).Scan(&ns); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,generated_by_execution_id) VALUES($1,$2,2,'CREATED',$3)`, output, ex.OutputDatasetID, ex.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO lifecycle_processing_output_intent(id,workspace_id,source_copy_id,dataset_version_id,namespace_id,object_key,content_sha256,byte_size,execution_id,actor_id,principal_ref,created_at) VALUES($1,$2,$3,$4,$5,'pending.csv',repeat('a',64),12,$6,$7,'SQL-only',now())`, uuid.New(), w, cs[0].ID, output, ns, ex.ID, s.occupancy.authority.(*occupancyAuthority).caller.ActorID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.RenewFixtureCopyPolicy(ctx, RenewFixtureCopyPolicyCommand{CommandHeader: CommandHeader{w, "pending-output"}, DatasetVersionID: cs[0].DatasetVersionID, PreviousPolicyID: old, UseUntil: time.Now().Add(time.Minute), RestoreUntil: time.Now().Add(time.Minute)}); e == nil {
		t.Fatal("unclosed output was ignored")
	}
	if _, e := pool.Exec(ctx, `INSERT INTO lifecycle_copy_policy(id,workspace_id,dataset_version_id,basis_ref,use_until,restore_until,actor_id,principal_ref,created_at,supersedes_policy_id) SELECT $1,workspace_id,dataset_version_id,basis_ref,now()+interval '1 minute',now()+interval '1 minute',actor_id,principal_ref,now(),id FROM lifecycle_copy_policy WHERE id=$2`, uuid.New(), old); e == nil {
		t.Fatal("raw pending-output renewal accepted")
	}
}
