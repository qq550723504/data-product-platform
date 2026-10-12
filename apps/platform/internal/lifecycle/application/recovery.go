package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

const CapabilityReconcileDisposition = "LIFECYCLE_RECONCILE_DISPOSITION"
const ReconciliationRequestedEvent = "LifecycleDispositionRecoveryRequested"

// Matches the existing Asynq policy (initial execution plus five retries).
const MaxRecoveryRuns = 6

var ErrRecoveryExhausted = errors.New("lifecycle recovery retry budget exhausted; explicit new request required")

// An early lookup can race the original physical send. Presence or a missing
// target cannot finish that work; Asynq retries and the durable pending scan
// must remain able to observe its later outcome within the existing budget.
// UNKNOWN_COPY_LEDGER finishes a specific restore admission evaluation, which
// requires a new explicit request after publishing a fresh independent cut.
func recoveryOutcomeTerminal(outcome string) bool {
	return outcome != "UNKNOWN_PROVIDER" && outcome != "UNKNOWN_PRESENT" && outcome != "UNKNOWN_COPY_TARGET"
}

type RecoveryLocker interface {
	WithAdvisoryLock(context.Context, string, func(context.Context) error) error
}
type RecoveryEnqueuer interface {
	EnqueueRecovery(context.Context, uuid.UUID, uuid.UUID) error
}
type RequestRecoveryCommand struct {
	CommandHeader
	AttemptID uuid.UUID
	BasisRef  string
}

// The only external capability is StorageInspector: this service cannot delete,
// copy, restore, release holds or resend an original provider attempt.
type RecoveryService struct {
	execution    *ExactExecution
	inspector    StorageInspector
	locker       RecoveryLocker
	enqueuer     RecoveryEnqueuer
	workspaces   []uuid.UUID
	copyRecovery *CopyRecoveryService
}

func NewRecoveryService(e *ExactExecution, i StorageInspector, l RecoveryLocker, q RecoveryEnqueuer, workspaces []uuid.UUID) *RecoveryService {
	return &RecoveryService{execution: e, inspector: i, locker: l, enqueuer: q, workspaces: append([]uuid.UUID(nil), workspaces...)}
}

// Shares the durable request scanner, outbox obligation, queue and lock owner.
func (s *RecoveryService) WithCopyRecovery(c *CopyRecoveryService) *RecoveryService {
	s.copyRecovery = c
	return s
}
func (s *RecoveryService) RequestCopyRecovery(ctx context.Context, c RequestCopyRecoveryCommand) (FactResult, error) {
	if !s.configured() || !s.allowed(c.WorkspaceID) || s.copyRecovery == nil {
		return FactResult{}, ErrStorageUnsupported
	}
	return s.copyRecovery.RequestRecovery(ctx, c)
}
func (s *RecoveryService) configured() bool {
	return s != nil && s.execution != nil && s.execution.disposition != nil && s.execution.disposition.configured() && s.inspector != nil && s.locker != nil
}
func (s *RecoveryService) allowed(w uuid.UUID) bool {
	for _, v := range s.workspaces {
		if v == w && w != uuid.Nil {
			return true
		}
	}
	return false
}

// Called inside dispatch's existing transaction, before any native side effect.
func appendRecoveryWork(ctx context.Context, tx pgx.Tx, a domain.ProviderAttempt, basis string, caller principal.RequestPrincipal, rev int64, at time.Time) (uuid.UUID, error) {
	id := uuid.New()
	_, err := tx.Exec(ctx, `INSERT INTO lifecycle_reconciliation_work(id,workspace_id,attempt_id,basis_ref,requested_at,actor_id,principal_ref,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, a.WorkspaceID, a.ID, basis, at, caller.ActorID, caller.Subject, rev)
	if err != nil {
		return uuid.Nil, err
	}
	return id, appendLifecycleFacts(ctx, tx, "LIFECYCLE_RECONCILIATION", ReconciliationRequestedEvent, "LIFECYCLE_RECONCILIATION_WORK", id, caller, rev, at, map[string]any{"attempt_id": a.ID, "basis_ref": basis})
}
func (s *RecoveryService) RequestRecovery(ctx context.Context, c RequestRecoveryCommand) (FactResult, error) {
	if !s.configured() || !s.allowed(c.WorkspaceID) || c.AttemptID == uuid.Nil || !validText(c.BasisRef, 255) {
		return FactResult{}, ErrStorageScope
	}
	d := s.execution.disposition
	o := d.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.REQUEST_RECOVERY", CapabilityReconcileDisposition, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, rev int64) (uuid.UUID, error) {
		a, err := o.repo.GetAttempt(ctx, tx, c.WorkspaceID, c.AttemptID, false)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err = d.lockedPlan(ctx, tx, c.WorkspaceID, a.PlanID); err != nil {
			return uuid.Nil, err
		}
		caller, at, err := o.authorize(ctx, tx, c.WorkspaceID, CapabilityReconcileDisposition, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		return appendRecoveryWork(ctx, tx, a, c.BasisRef, caller, rev, at)
	})
}

// Redis loss after a confirmed enqueue is repaired from durable Core requests.
// Asynq retains ownership of retries, backoff and orphan leases. This scan never
// invokes a provider, grants authority or converts elapsed time into certainty.
func (s *RecoveryService) RunOnce(ctx context.Context) error {
	if !s.configured() || s.enqueuer == nil {
		return ErrStorageUnsupported
	}
	o := s.execution.disposition.occupancy
	for _, w := range s.workspaces {
		var ids []uuid.UUID
		err := o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT w.id FROM lifecycle_reconciliation_work w WHERE w.workspace_id=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_reconciliation_result r WHERE r.work_id=w.id AND r.terminal) AND (SELECT count(*) FROM lifecycle_reconciliation_run r WHERE r.work_id=w.id)<$2 ORDER BY w.requested_at,w.id LIMIT 64`, w, MaxRecoveryRuns)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var id uuid.UUID
				if err = rows.Scan(&id); err != nil {
					return err
				}
				ids = append(ids, id)
			}
			return rows.Err()
		})
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err = s.enqueuer.EnqueueRecovery(ctx, w, id); err != nil {
				return err
			}
		}
	}
	return nil
}

type recoveryRun struct {
	id, work, workspace                  uuid.UUID
	attempt                              domain.ProviderAttempt
	copy                                 domain.Copy
	plan                                 domain.Plan
	caller                               principal.RequestPrincipal
	revision                             int64
	outcome                              string
	inventory, stat, receipt, completion uuid.UUID
}

func (s *RecoveryService) RunRecovery(ctx context.Context, w, id uuid.UUID) error {
	if !s.configured() || !s.allowed(w) || id == uuid.Nil {
		return ErrStorageUnsupported
	}
	o := s.execution.disposition.occupancy
	var attempt, copyCall *uuid.UUID
	if err := o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT attempt_id,copy_call_id FROM lifecycle_reconciliation_work WHERE workspace_id=$1 AND id=$2`, w, id).Scan(&attempt, &copyCall)
	}); err != nil {
		return err
	}
	if copyCall != nil {
		if s.copyRecovery == nil {
			return ErrStorageUnsupported
		}
		return s.locker.WithAdvisoryLock(ctx, "lifecycle-copy-recovery:"+w.String()+":"+copyCall.String(), func(ctx context.Context) error { return s.copyRecovery.run(ctx, w, id, *copyCall) })
	}
	if attempt == nil {
		return ErrStorageScope
	}
	return s.locker.WithAdvisoryLock(ctx, "lifecycle-recovery:"+w.String()+":"+attempt.String(), func(ctx context.Context) error { return s.run(ctx, w, id) })
}
func (s *RecoveryService) run(ctx context.Context, w, id uuid.UUID) error {
	d := s.execution.disposition
	o := d.occupancy
	r := recoveryRun{work: id, workspace: w}
	var finished bool
	err := o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, w)
		if err != nil {
			return err
		}
		r.revision = rev
		var attempt uuid.UUID
		if err = tx.QueryRow(ctx, `SELECT attempt_id FROM lifecycle_reconciliation_work WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w, id).Scan(&attempt); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_reconciliation_result WHERE work_id=$1 AND terminal)`, id).Scan(&finished); err != nil || finished {
			return err
		}
		var n int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reconciliation_run WHERE work_id=$1`, id).Scan(&n); err != nil {
			return err
		}
		if n >= MaxRecoveryRuns {
			return ErrRecoveryExhausted
		}
		r.attempt, err = o.repo.GetAttempt(ctx, tx, w, attempt, false)
		if err != nil {
			return err
		}
		r.plan, err = d.lockedPlan(ctx, tx, w, r.attempt.PlanID)
		if err != nil {
			return err
		}
		for _, c := range r.plan.Targets {
			if c.ID == r.attempt.CopyID {
				r.copy = c
			}
		}
		if r.copy.ID == uuid.Nil {
			return ErrStorageScope
		}
		caller, at, authErr := o.authorize(ctx, tx, w, CapabilityReconcileDisposition, nil)
		if authErr != nil {
			// The persisted initiator is used only to journal a denial, never to grant I/O.
			if !errors.Is(authErr, principal.ErrCapabilityDenied) && !errors.Is(authErr, principal.ErrNotConfigured) && !errors.Is(authErr, principal.ErrUnauthenticated) && !errors.Is(authErr, principal.ErrWorkspaceDenied) {
				return authErr
			}
			if err = tx.QueryRow(ctx, `SELECT actor_id,principal_ref FROM lifecycle_reconciliation_work WHERE id=$1`, id).Scan(&caller.ActorID, &caller.Subject); err != nil {
				return err
			}
			caller.WorkspaceID = w
			r.outcome = "BLOCKED_AUTHORITY"
		}
		r.caller = caller
		r.id = uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_reconciliation_run(id,workspace_id,work_id,started_at,actor_id,principal_ref,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, r.id, w, id, at, caller.ActorID, caller.Subject, rev)
		if err != nil {
			return err
		}
		if err = appendLifecycleFacts(ctx, tx, "LIFECYCLE_RECONCILIATION", "LifecycleDispositionRecoveryStarted", "LIFECYCLE_RECONCILIATION_RUN", r.id, caller, rev, at, map[string]any{"work_id": id, "attempt_id": r.attempt.ID}); err != nil {
			return err
		}
		if r.outcome != "" {
			return nil
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM lifecycle_copy_absence_receipt WHERE dispatch_id=$1`, r.attempt.DispatchID).Scan(&r.receipt); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err = tx.QueryRow(ctx, `SELECT id FROM lifecycle_plan_completion WHERE plan_id=$1`, r.plan.ID).Scan(&r.completion); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var released bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$1)`, r.attempt.DispatchID).Scan(&released); err != nil {
			return err
		}
		if r.receipt != uuid.Nil || released {
			r.outcome = "ALREADY_RESOLVED"
		}
		return nil
	})
	if err != nil || finished {
		return err
	}
	if r.outcome != "" {
		if r.receipt != uuid.Nil && r.completion == uuid.Nil && r.outcome != "BLOCKED_AUTHORITY" {
			r.outcome = "ABSENT_PARTIAL"
			if err := s.finishPlan(ctx, &r); err != nil {
				return s.observe(ctx, &r, err)
			}
		}
		return s.observe(ctx, &r, nil)
	}
	// Each metadata call also performs fresh authority through the existing
	// instrumented storage journal, and commits a distinct physical call/cost.
	inv, err := s.inspector.InspectNamespace(ctx, r.copy)
	if err != nil {
		return s.observe(ctx, &r, err)
	}
	if inv.WorkspaceID != w || inv.ProviderInstanceID != r.copy.ProviderInstanceID || inv.IncarnationID != r.copy.IncarnationID || inv.Namespace != r.copy.Namespace || inv.CallID == uuid.Nil {
		return s.observe(ctx, &r, ErrStorageContract)
	}
	if inv.ObservedAt.Before(r.attempt.StartedAt) || inv.ObservedAt.After(o.clock()) || o.clock().Sub(inv.ObservedAt) > time.Minute {
		return s.observe(ctx, &r, ErrStorageContract)
	}
	r.inventory = inv.CallID
	seen := map[string]bool{}
	present := false
	for _, v := range inv.Versions {
		k := v.Key + "\x00" + v.Version
		if v.Key == "" || v.Version == "" || v.Version == "null" || seen[k] {
			r.inventory = uuid.Nil
			return s.observe(ctx, &r, ErrStorageContract)
		}
		seen[k] = true
		if v.Key == r.copy.Key && v.Version == r.copy.ProviderVersion {
			if v.DeleteMarker || v.ByteSize != r.copy.ByteSize {
				return s.observe(ctx, &r, ErrStorageContract)
			}
			present = true
		}
	}
	if present {
		if err = s.fresh(ctx, &r); err != nil {
			return s.observe(ctx, &r, err)
		}
		head, e := s.inspector.InspectExactCopy(ctx, r.copy)
		if e != nil {
			return s.observe(ctx, &r, e)
		}
		if head.Copy != r.copy || head.CallID == uuid.Nil {
			return s.observe(ctx, &r, ErrStorageContract)
		}
		if head.ObservedAt.Before(inv.ObservedAt) || head.ObservedAt.After(o.clock()) || o.clock().Sub(head.ObservedAt) > time.Minute {
			return s.observe(ctx, &r, ErrStorageContract)
		}
		r.stat = head.CallID
		r.outcome = "UNKNOWN_PRESENT"
		// Presence never proves an uncertain DELETE cannot still complete. No release.
	} else {
		if err = s.fresh(ctx, &r); err != nil {
			return s.observe(ctx, &r, err)
		}
		receipt, e := s.execution.verifyAbsentInScope(ctx, CommandHeader{w, "recovery-absence:" + r.id.String()}, r.attempt.ID, inv, true)
		if e != nil {
			return s.observe(ctx, &r, e)
		}
		r.receipt = receipt.ID
		r.outcome = "ABSENT_PARTIAL"
		if err := s.finishPlan(ctx, &r); err != nil {
			return s.observe(ctx, &r, err)
		}
	}
	return s.observe(ctx, &r, nil)
}
func (s *RecoveryService) finishPlan(ctx context.Context, r *recoveryRun) error {
	var complete bool
	err := s.execution.disposition.occupancy.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM lifecycle_copy_absence_receipt WHERE plan_id=$1)=(SELECT count(*) FROM lifecycle_disposition_plan_target WHERE plan_id=$1)`, r.plan.ID).Scan(&complete)
	})
	if err != nil || !complete {
		return err
	}
	if err = s.fresh(ctx, r); err != nil {
		return err
	}
	result, err := s.execution.finalizeContentDisposition(ctx, CommandHeader{r.workspace, "recovery-finalize:" + r.plan.ID.String()}, r.plan.ID, true)
	if err != nil {
		return err
	}
	r.completion = result.ID
	r.outcome = "ABSENT_VERIFIED"
	return nil
}
func (s *RecoveryService) fresh(ctx context.Context, r *recoveryRun) error {
	o := s.execution.disposition.occupancy
	return o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := deliveryfence.Lock(ctx, tx, r.workspace); err != nil {
			return err
		}
		_, _, err := o.authorize(ctx, tx, r.workspace, CapabilityReconcileDisposition, &r.caller)
		return err
	})
}
func (s *RecoveryService) observe(ctx context.Context, r *recoveryRun, cause error) error {
	o := s.execution.disposition.occupancy
	// Journal already incurred observations after revocation, with no new I/O.
	err := o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, r.workspace)
		if err != nil {
			return err
		}
		at := o.clock()
		if r.outcome != "BLOCKED_AUTHORITY" {
			_, _, authErr := o.authorize(ctx, tx, r.workspace, CapabilityReconcileDisposition, &r.caller)
			if authErr != nil {
				cause = authErr
				r.outcome = "BLOCKED_AUTHORITY"
			}
		}
		if cause != nil && r.outcome != "BLOCKED_AUTHORITY" {
			r.outcome = "UNKNOWN_PROVIDER"
		}
		if r.outcome == "" {
			r.outcome = "UNKNOWN_PROVIDER"
		}
		id := uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_reconciliation_result(id,workspace_id,work_id,run_id,outcome,terminal,inventory_call_id,stat_call_id,absence_receipt_id,completion_id,observed_at,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, r.workspace, r.work, r.id, r.outcome, recoveryOutcomeTerminal(r.outcome), nullableID(r.inventory), nullableID(r.stat), nullableID(r.receipt), nullableID(r.completion), at, rev)
		if err != nil {
			return err
		}
		return appendLifecycleFacts(ctx, tx, "LIFECYCLE_RECONCILIATION", "LifecycleDispositionRecoveryObserved", "LIFECYCLE_RECONCILIATION_RESULT", id, r.caller, rev, at, map[string]any{"work_id": r.work, "run_id": r.id, "attempt_id": r.attempt.ID, "outcome": r.outcome, "original_attempt_unchanged": true})
	})
	if err != nil {
		return err
	}
	if !recoveryOutcomeTerminal(r.outcome) {
		if cause == nil {
			cause = ErrStorageUnknown
		}
		return fmt.Errorf("lifecycle metadata reconciliation uncertain: %w", cause)
	}
	return nil
}
func nullableID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
