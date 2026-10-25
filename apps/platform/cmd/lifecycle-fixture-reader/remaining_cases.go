package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

func waitRemainingWaiter(ctx context.Context, pool *pgxpool.Pool, blocking int) (int, error) {
	until := time.NewTimer(5 * time.Second)
	defer until.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		var pid int
		err := pool.QueryRow(ctx, `SELECT pid FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)) LIMIT 1`, blocking).Scan(&pid)
		if err == nil {
			return pid, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return 0, err
		}
		select {
		case <-tick.C:
		case <-until.C:
			return 0, errors.New("real PG waiter not observed")
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}
func remainingCases(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, cs []domain.Copy, mode string) error {
	c := r.Base
	o, _, _, _, err := remainingServices(r, pool, cs)
	if err != nil {
		return err
	}
	d, err := newRemainingDisposition(ctx, r, pool, cs, o)
	if err != nil {
		return err
	}
	header := func(key string) lc.CommandHeader {
		return lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: r.Batch.String() + ":" + key}
	}
	hold := func(cp domain.Copy, key string) (lc.FactResult, error) {
		return o.PlaceContentHold(ctx, lc.PlaceContentHoldCommand{CommandHeader: header(key), CopyID: cp.ID, BasisRef: "remaining-late-hold:" + r.Batch.String()})
	}
	if mode == "remaining-late" {
		if r.Group != "late-hold" {
			return lc.ErrStorageScope
		}
		// H0: hold owns the authorization fence while it waits for a test-owned
		// copy-row lock. Observe a real dispatch waiter behind that hold transaction.
		if err = d.restrict(ctx, cs[0]); err != nil {
			return err
		}
		if err = d.approve(ctx, "H0"); err != nil {
			return err
		}
		if err = d.refresh(ctx, cs[0]); err != nil {
			return err
		}
		blocker, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer blocker.Rollback(context.WithoutCancel(ctx))
		var blockerPID int
		if err = blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
			return err
		}
		if _, err = blocker.Exec(ctx, `SELECT id FROM lifecycle_fixture_copy WHERE id=$1 FOR UPDATE`, cs[0].ID); err != nil {
			return err
		}
		held := make(chan error, 1)
		go func() { _, e := hold(cs[0], "H0-place"); held <- e }()
		holdPID, err := waitRemainingWaiter(ctx, pool, blockerPID)
		if err != nil {
			return err
		}
		type dispatchResult struct {
			result lc.DispatchResult
			err    error
		}
		dispatched := make(chan dispatchResult, 1)
		go func() {
			result, e := d.commandDispatch(ctx, "H0", "H0-fence-waiter", cs[0])
			dispatched <- dispatchResult{result, e}
		}()
		dispatchPID, err := waitRemainingWaiter(ctx, pool, holdPID)
		if err != nil {
			return err
		}
		if err = blocker.Rollback(ctx); err != nil {
			return err
		}
		if err = <-held; err != nil {
			return err
		}
		h0 := <-dispatched
		if h0.err != nil || h0.result.Admitted || h0.result.AttemptID != uuid.Nil {
			return errors.New("H0 hold-first admitted an attempt")
		}
		if err = write(c, "remaining-H0.json", map[string]any{"blocking_pid": blockerPID, "hold_pid": holdPID, "dispatch_waiter_pid": dispatchPID, "dispatch": h0.result, "real_pg_waiter": true, "native_delete": 0}); err != nil {
			return err
		}
		// H1: durable dispatch/start is not a wire send. Revoke the whole plan first,
		// then append holds for every copy, rejecting claim and the remaining target.
		if err = d.restrict(ctx, cs[1]); err != nil {
			return err
		}
		if err = d.approve(ctx, "H1"); err != nil {
			return err
		}
		started, err := d.dispatch(ctx, "H1", "H1-start", cs[1])
		if err != nil || !started.Admitted {
			return errors.New("H1 start not admitted")
		}
		if _, err = d.service.RevokeDispositionApproval(ctx, lc.RevokeDispositionApprovalCommand{CommandHeader: header("H1-revoke"), ApprovalID: d.approvals["H1"], BasisRef: "hold-before-claim"}); err != nil {
			return err
		}
		var holds []lc.FactResult
		for _, cp := range cs[1:3] {
			h, e := hold(cp, "H1-hold:"+cp.ID.String())
			if e != nil {
				return e
			}
			holds = append(holds, h)
		}
		var attempt domain.ProviderAttempt
		if err = transaction.NewManager(pool).Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var e error
			attempt, e = (&infrastructure.PostgresRepository{}).GetAttempt(ctx, tx, c.WorkspaceID, started.AttemptID, false)
			return e
		}); err != nil {
			return err
		}
		claimErr := lc.NewExactExecution(d.service).ClaimExactSend(ctx, attempt, cs[1])
		if claimErr == nil {
			return errors.New("H1 claim survived revocation/holds")
		}
		remaining, err := d.dispatch(ctx, "H1", "H1-rest-blocked", cs[2])
		if err != nil || remaining.Admitted || !holds[0].PreservationUnconfirmed {
			return errors.New("H1 conflict/rest boundary differs")
		}
		if err = write(c, "remaining-H1.json", map[string]any{"attempt": attempt, "claim_denied": true, "holds": holds, "remaining_target": remaining, "NOT_SENT": "PENDING_TRUSTED_CLOSED_OWNER_ATTESTATION", "native_delete": 0}); err != nil {
			return err
		}
		if err = d.restrict(ctx, cs[3]); err != nil {
			return err
		}
		if err = d.approve(ctx, "H2"); err != nil {
			return err
		}
		ready, err := d.dispatch(ctx, "H2", "H2-A-ready", cs[3])
		if err != nil || !ready.Admitted {
			return fmt.Errorf("H2 admission failed: %+v %v", ready, err)
		}
		return write(c, "remaining-H2-ready.json", map[string]any{"dispatch": ready, "B_never_dispatched": cs[4], "send_claims": 0, "native_delete": 0, "wire_stage": "SEPARATE_FRESH_EXACT_AUTHORITY_REQUIRED"})
	}
	if r.Group != "mixed" {
		return lc.ErrStorageScope
	}
	if mode == "remaining-mixed" {
		if err = d.restrict(ctx, cs[0]); err != nil {
			return err
		}
		if err = d.approve(ctx, "M1"); err != nil {
			return err
		}
		ready, err := d.dispatch(ctx, "M1", "M1-A-ready", cs[0])
		if err != nil || !ready.Admitted {
			return errors.New("M1-A admission failed")
		}
		return write(c, "remaining-M1-A-ready.json", map[string]any{"dispatch": ready, "copies": cs[:3], "native_delete": 0})
	}
	var approval uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT id FROM lifecycle_disposition_approval WHERE plan_id=$1 AND basis_ref=$2 ORDER BY valid_from DESC LIMIT 1`, d.plans["M1"].ID, "REMAINING_NON_DELETE_REHEARSAL:"+r.Batch.String()).Scan(&approval); err != nil {
		return err
	}
	d.approvals["M1"] = approval
	if mode == "remaining-mixed-protected" {
		if !r.Rehearsal {
			return lc.ErrStorageScope
		}
		blocked, e := d.dispatch(ctx, "M1", "M1-C-nondelete-protection", cs[2])
		observation := observeProtectedDispatch(blocked, e)
		if err = write(c, "remaining-M1-C-dispatch-observation.json", observation); err != nil {
			return err
		}
		if observation.Reason != "REJECTED_WITHOUT_ATTEMPT" {
			return fmt.Errorf("protected C dispatch: %s", observation.Reason)
		}
		p := d.deps.Protection[cs[2].ID]
		return write(c, "remaining-M1-C-native-nondelete.json", map[string]any{"dispatch": blocked, "native_protection": p, "mixed_outcomes": "NOT_RUN", "native_delete": 0})
	}
	if mode == "remaining-mixed-b" {
		var absent bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$1)`, cs[0].ID).Scan(&absent); err != nil || !absent {
			return errors.New("M1-A exact absence must commit first")
		}
		ready, err := d.dispatch(ctx, "M1", "M1-B-ready", cs[1])
		if err != nil || !ready.Admitted {
			return errors.New("M1-B admission failed")
		}
		return write(c, "remaining-M1-B-ready.json", ready)
	}
	// No C namespace inventory may inadvertently look up B before the sender's
	// immutable mixed checkpoint. The host supplies the exact checkpoint file.
	var checkpoint map[string]json.RawMessage
	if err = readRemaining("/run/private/M1-mixed-checkpoint.json", &checkpoint); err != nil {
		return err
	}
	var ex string
	var ordered bool
	var at time.Time
	var unknown domain.ProviderAttempt
	if json.Unmarshal(checkpoint["execution"], &ex) != nil || ex != r.Batch.String() || json.Unmarshal(checkpoint["before_any_B_lookup"], &ordered) != nil || !ordered || json.Unmarshal(checkpoint["at"], &at) != nil || at.After(time.Now().UTC()) || json.Unmarshal(checkpoint["B_confirmed_unknown"], &unknown) != nil || unknown.CopyID != cs[1].ID || unknown.PlanID != d.plans["M1"].ID {
		return lc.ErrStorageScope
	}
	var beforeLookup bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_observation o WHERE o.attempt_id=$1 AND o.certainty='CONFIRMED' AND o.outcome='UNKNOWN' AND o.observed_at<=$2 AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_call c JOIN lifecycle_fixture_copy cp ON cp.id=c.copy_id WHERE c.started_at>o.observed_at AND c.started_at<$2 AND (cp.id=$3 OR (c.operation='LIST_NAMESPACE_VERSIONS' AND cp.namespace_id=(SELECT namespace_id FROM lifecycle_fixture_copy WHERE id=$3)))))`, unknown.ID, at, cs[1].ID).Scan(&beforeLookup); err != nil || !beforeLookup {
		return errors.New("M1 checkpoint was preceded by a B lookup")
	}
	blocked, err := d.dispatch(ctx, "M1", "M1-C-retained", cs[2])
	observation := observeProtectedDispatch(blocked, err)
	if e := write(c, "remaining-M1-C-dispatch-observation.json", observation); e != nil {
		return e
	}
	if observation.Reason != "REJECTED_WITHOUT_ATTEMPT" {
		return fmt.Errorf("protected C dispatch: %s", observation.Reason)
	}
	protection := d.deps.Protection[cs[2].ID]
	return write(c, "remaining-M1-C-blocked.json", map[string]any{"dispatch": blocked, "native_protection": protection, "completion": 0, "native_delete_C": 0})
}
