package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCopyPolicyRenewalRetainsOldFactAndSerializesExpectedHead(t *testing.T) {
	pool, w, cs, s, _, _ := newCopyFixture(t)
	ctx := context.Background()
	var old uuid.UUID
	var deadline time.Time
	if e := pool.QueryRow(ctx, `SELECT id,use_until FROM lifecycle_copy_policy WHERE workspace_id=$1`, w).Scan(&old, &deadline); e != nil {
		t.Fatal(e)
	}
	// Advance only this deterministic SQL test clock beyond the original window.
	now := deadline.Add(time.Minute)
	s.occupancy.now = func() time.Time { return now }
	commands := []RenewFixtureCopyPolicyCommand{
		{CommandHeader: CommandHeader{w, "renew-a"}, DatasetVersionID: cs[0].DatasetVersionID, PreviousPolicyID: old, UseUntil: now.Add(10 * time.Minute), RestoreUntil: now.Add(10 * time.Minute)},
		{CommandHeader: CommandHeader{w, "renew-b"}, DatasetVersionID: cs[0].DatasetVersionID, PreviousPolicyID: old, UseUntil: now.Add(10 * time.Minute), RestoreUntil: now.Add(10 * time.Minute)},
	}
	var wg sync.WaitGroup
	results := make([]FactResult, 2)
	errs := make([]error, 2)
	for i := range commands {
		wg.Add(1)
		go func(i int) { defer wg.Done(); results[i], errs[i] = s.RenewFixtureCopyPolicy(ctx, commands[i]) }(i)
	}
	wg.Wait()
	winner := -1
	for i, e := range errs {
		if e == nil {
			if winner >= 0 {
				t.Fatal("two successor heads committed")
			}
			winner = i
		}
	}
	if winner < 0 {
		t.Fatal(errs)
	}
	replay, e := s.RenewFixtureCopyPolicy(ctx, commands[winner])
	if e != nil || !replay.Replayed || replay.ID != results[winner].ID {
		t.Fatal(replay, e)
	}
	var retained time.Time
	var head uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT use_until FROM lifecycle_copy_policy WHERE id=$1`, old).Scan(&retained); e != nil || !retained.Equal(deadline) {
		t.Fatal("old policy changed", e)
	}
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_current_copy_policy WHERE workspace_id=$1`, w).Scan(&head); e != nil || head != results[winner].ID {
		t.Fatal("head ambiguous", e)
	}
	if n := count(t, pool, `SELECT count(*) FROM lifecycle_copy_policy`); n != 2 {
		t.Fatal(n)
	}
	if _, e = pool.Exec(ctx, `UPDATE lifecycle_copy_policy SET use_until=use_until+interval '1 hour' WHERE id=$1`, old); e == nil {
		t.Fatal("old history mutable")
	}
	if _, e = pool.Exec(ctx, `INSERT INTO lifecycle_copy_policy(id,workspace_id,dataset_version_id,basis_ref,use_until,restore_until,actor_id,principal_ref,created_at,supersedes_policy_id) SELECT $1,workspace_id,dataset_version_id,basis_ref,use_until,restore_until,actor_id,principal_ref,created_at+interval '1 second',supersedes_policy_id FROM lifecycle_copy_policy WHERE id=$2`, uuid.New(), head); e == nil {
		t.Fatal("raw branch accepted")
	}
	if n := count(t, pool, `SELECT count(*) FROM outbox_event WHERE event_type='LifecycleCopyPolicyRenewed'`); n != 1 {
		t.Fatal("renew event count", n)
	}
}

func TestCopyPolicyRenewalRefusesExpiredAuthorityWideWindowAndLiveOccupancy(t *testing.T) {
	for _, kind := range []string{"authority", "wide", "restriction", "hold", "reader", "dispatch"} {
		t.Run(kind, func(t *testing.T) {
			pool, w, cs, s, _, authority := newCopyFixture(t)
			ctx := context.Background()
			var old uuid.UUID
			if e := pool.QueryRow(ctx, `SELECT id FROM lifecycle_copy_policy WHERE workspace_id=$1`, w).Scan(&old); e != nil {
				t.Fatal(e)
			}
			cmd := RenewFixtureCopyPolicyCommand{CommandHeader: CommandHeader{w, "renew-denied"}, DatasetVersionID: cs[0].DatasetVersionID, PreviousPolicyID: old, UseUntil: time.Now().Add(10 * time.Minute), RestoreUntil: time.Now().Add(10 * time.Minute)}
			var e error
			switch kind {
			case "authority":
				authority.denied.Store(true)
			case "wide":
				cmd.UseUntil = time.Now().Add(2 * time.Hour)
			case "restriction":
				_, e = s.occupancy.RestrictContentUse(ctx, RestrictContentUseCommand{CommandHeader: CommandHeader{w, "restriction"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "test"})
			case "hold":
				_, e = s.occupancy.PlaceContentHold(ctx, PlaceContentHoldCommand{CommandHeader: CommandHeader{w, "hold"}, CopyID: cs[0].ID, BasisRef: "test"})
			case "reader":
				_, e = s.occupancy.AdmitFixtureReader(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "reader"}, CopyID: cs[0].ID, ReaderID: uuid.New()})
			case "dispatch":
				_, e = s.PrepareCopy(ctx, PrepareCopyCommand{CommandHeader: CommandHeader{w, "pending"}, SourceCopyID: cs[0].ID, TargetSlot: "archive", BasisRef: "synthetic", ExpiresAt: time.Now().Add(time.Minute)})
			}
			if e != nil {
				t.Fatal(e)
			}
			if result, e := s.RenewFixtureCopyPolicy(ctx, cmd); e == nil {
				t.Fatal("renew admitted", kind, result)
			}
			if n := count(t, pool, `SELECT count(*) FROM lifecycle_copy_policy`); n != 1 {
				t.Fatal("denied renewal left fact", n)
			}
		})
	}
}
