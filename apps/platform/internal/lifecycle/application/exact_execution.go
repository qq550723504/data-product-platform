package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

const CapabilityVerifyAbsence = "LIFECYCLE_VERIFY_ABSENCE"

// Server wiring only: observations are supplied by the instrumented Adapter,
// never accepted from an HTTP client. A durable claim permits at most one send.
type ExactDeletionJournal interface {
	ClaimExactSend(context.Context, domain.ProviderAttempt, domain.Copy) error
	ObserveExactSend(context.Context, domain.ProviderObservation) error
}
type ExactCopyDeleter interface {
	DeleteExactCopy(context.Context, domain.ProviderAttempt, domain.Copy) error
}
type ExactExecution struct{ disposition *DispositionService }

func NewExactExecution(s *DispositionService) *ExactExecution { return &ExactExecution{s} }

func (s *ExactExecution) ClaimExactSend(ctx context.Context, a domain.ProviderAttempt, c domain.Copy) error {
	if s == nil || s.disposition == nil || !s.disposition.configured() || a.Validate() != nil || c.Validate() != nil {
		return ErrStorageScope
	}
	d := s.disposition
	o := d.occupancy
	return o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, a.WorkspaceID)
		if err != nil {
			return err
		}
		p, err := d.lockedPlan(ctx, tx, a.WorkspaceID, a.PlanID)
		if err != nil {
			return err
		}
		found := false
		for _, target := range p.Targets {
			if target == c && target.ID == a.CopyID {
				found = true
			}
		}
		if !found {
			return ErrStorageScope
		}
		stored, err := o.repo.GetAttempt(ctx, tx, a.WorkspaceID, a.ID, true)
		if err != nil {
			return err
		}
		if stored != a {
			return ErrStorageScope
		}
		var previouslyObserved bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$1) OR EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$2) OR EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE dispatch_id=$2)`, a.ID, a.DispatchID).Scan(&previouslyObserved); err != nil {
			return err
		}
		if previouslyObserved {
			return ErrIdempotencyConflict
		}
		caller, at, err := o.authorize(ctx, tx, a.WorkspaceID, CapabilityDispatch, nil)
		if err != nil {
			return err
		}
		var approvalID uuid.UUID
		err = tx.QueryRow(ctx, `SELECT approval_id FROM lifecycle_disposition_dispatch WHERE id=$1`, a.DispatchID).Scan(&approvalID)
		if err != nil {
			return err
		}
		approval, err := o.repo.GetApproval(ctx, tx, a.WorkspaceID, approvalID, false)
		if err != nil {
			return err
		}
		current, err := d.approver.CheckApproverAuthority(ctx, tx, approval, at)
		if err != nil {
			return err
		}
		revoked, err := o.repo.ApprovalRevoked(ctx, tx, a.WorkspaceID, approvalID)
		if err != nil {
			return err
		}
		dep, err := d.dependencies.EvaluateDisposalDependencies(ctx, tx, DisposalDependencyRequest{Plan: p, Copy: c, Caller: caller, AsOf: at, DependencyRevision: rev})
		if err != nil {
			return err
		}
		occ, err := o.repo.CopyOccupancy(ctx, tx, c)
		if err != nil {
			return err
		}
		if revoked || current != domain.Satisfied || approval.ValidateFor(p) != nil || at.Before(approval.ValidFrom) || !at.Before(approval.ExpiresAt) ||
			dep.WorkspaceID != a.WorkspaceID || dep.CopyID != c.ID || dep.PlanID != p.ID || dep.PlanHash != p.ContentHash || dep.ActorID != caller.ActorID || !dep.CheckedAt.Equal(at) || dep.DependencyRevision != rev ||
			dep.InventoryComplete != domain.Satisfied || dep.ReferencesReleased != domain.Satisfied || dep.ProviderRetentionAllows != domain.Satisfied || dep.LedgerCurrent != domain.Satisfied ||
			domain.ValidateWindowAt(dep.WindowMode, dep.ValidUntil, at) != nil || occ.UseRestricted != domain.Satisfied || occ.ReadersDrained != domain.Satisfied || occ.HoldFree != domain.Satisfied {
			return ErrStorageDenied
		}
		decision := domain.EvaluateDeletePrerequisites(domain.DeletePrerequisites{AsOf: at, Plan: p, Approval: domain.Approval{WorkspaceID: p.WorkspaceID, PlanHash: approval.PlanHash, Action: approval.Action, ValidFrom: approval.ValidFrom, ExpiresAt: approval.ExpiresAt, Current: current}, Retention: dep.Retention, DisposalDueAt: dep.DisposalDueAt, CallerAuthority: domain.Satisfied, UseRestricted: occ.UseRestricted, InventoryComplete: dep.InventoryComplete, ReferencesReleased: dep.ReferencesReleased, ProviderRetentionAllows: dep.ProviderRetentionAllows, LedgerCurrent: dep.LedgerCurrent, ReadersDrained: occ.ReadersDrained, HoldFree: occ.HoldFree})
		if !decision.Eligible {
			return ErrStorageDenied
		}
		tag, err := tx.Exec(ctx, `INSERT INTO lifecycle_provider_send_claim(attempt_id,workspace_id,claimed_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, a.ID, a.WorkspaceID, at)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrIdempotencyConflict
		}
		return appendDispositionFacts(ctx, tx, "LifecycleProviderSendClaimed", "LIFECYCLE_PROVIDER_ATTEMPT", a.ID, caller, rev, at, map[string]any{"copy_id": c.ID})
	})
}

// Revoking the caller after a send must not discard incurred costs. This private
// journal resolves the persisted initiator and appends, without granting new I/O.
func (s *ExactExecution) ObserveExactSend(ctx context.Context, proof domain.ProviderObservation) error {
	if s == nil || s.disposition == nil || !s.disposition.configured() {
		return ErrStorageScope
	}
	d := s.disposition
	o := d.occupancy
	return o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, proof.WorkspaceID)
		if err != nil {
			return err
		}
		a, err := o.repo.GetAttempt(ctx, tx, proof.WorkspaceID, proof.AttemptID, false)
		if err != nil {
			return err
		}
		if _, err = d.lockedPlan(ctx, tx, a.WorkspaceID, a.PlanID); err != nil {
			return err
		}
		a, err = o.repo.GetAttempt(ctx, tx, a.WorkspaceID, a.ID, true)
		if err != nil {
			return err
		}
		at := o.clock()
		if proof.ValidateFor(a, at) != nil {
			return ErrStorageContract
		}
		caller := principal.RequestPrincipal{WorkspaceID: a.WorkspaceID}
		err = tx.QueryRow(ctx, `SELECT g.actor_id,g.principal_ref FROM lifecycle_disposition_dispatch d JOIN lifecycle_disposition_gate_evaluation g ON g.id=d.evaluation_id JOIN lifecycle_provider_send_claim s ON s.attempt_id=$1 WHERE d.id=$2`, a.ID, a.DispatchID).Scan(&caller.ActorID, &caller.Subject)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO lifecycle_provider_observation(id,workspace_id,attempt_id,certainty,outcome,proof_sha256,observed_at,invoked_at,actor_id,principal_ref,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT(id) DO NOTHING`, proof.ID, a.WorkspaceID, a.ID, proof.Certainty, proof.Outcome, proof.ProofSHA256, proof.ObservedAt, proof.InvokedAt, caller.ActorID, caller.Subject, rev)
		if err != nil {
			return err
		}
		if proof.Certainty == domain.InvocationConfirmed {
			if err = cost.AppendLifecycleProviderActivity(ctx, tx, a.WorkspaceID, a.ID, *proof.InvokedAt); err != nil {
				return err
			}
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return appendDispositionFacts(ctx, tx, ProviderObservedEvent, "LIFECYCLE_PROVIDER_OBSERVATION", proof.ID, caller, rev, at, map[string]any{"attempt_id": a.ID, "outcome": proof.Outcome, "certainty": proof.Certainty})
	})
}

// VerifyAbsentInScope consumes a complete native inventory from server wiring.
// It proves only the exact version in the provider-accessible namespace absent.
// UNKNOWN send history is preserved; verification performs no second deletion.
func (s *ExactExecution) VerifyAbsentInScope(ctx context.Context, h CommandHeader, attemptID uuid.UUID, inventory NamespaceInventory) (FactResult, error) {
	if s == nil || s.disposition == nil || !s.disposition.configured() {
		return FactResult{}, ErrStorageScope
	}
	d := s.disposition
	o := d.occupancy
	return o.command(ctx, h, "LIFECYCLE.VERIFY_ABSENCE", CapabilityVerifyAbsence, struct {
		Attempt uuid.UUID
		Call    uuid.UUID
	}{attemptID, inventory.CallID}, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		a, err := o.repo.GetAttempt(ctx, tx, h.WorkspaceID, attemptID, false)
		if err != nil {
			return uuid.Nil, err
		}
		p, err := d.lockedPlan(ctx, tx, h.WorkspaceID, a.PlanID)
		if err != nil {
			return uuid.Nil, err
		}
		var c domain.Copy
		for _, target := range p.Targets {
			if target.ID == a.CopyID {
				c = target
			}
		}
		caller, at, err := o.authorize(ctx, tx, h.WorkspaceID, CapabilityVerifyAbsence, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		if inventory.WorkspaceID != h.WorkspaceID || inventory.ProviderInstanceID != c.ProviderInstanceID || inventory.IncarnationID != c.IncarnationID || inventory.Namespace != c.Namespace || inventory.CallID == uuid.Nil || inventory.ObservedAt.Before(a.StartedAt) || inventory.ObservedAt.After(at) || at.Sub(inventory.ObservedAt) > time.Minute {
			return uuid.Nil, ErrStorageScope
		}
		seen := map[string]bool{}
		for _, v := range inventory.Versions {
			key := v.Key + "\x00" + v.Version
			if v.Key == "" || v.Version == "" || v.Version == "null" || seen[key] || (v.Key == c.Key && v.Version == c.ProviderVersion) {
				return uuid.Nil, ErrStorageContract
			}
			seen[key] = true
		}
		var exists bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE dispatch_id=$1)`, a.DispatchID).Scan(&exists)
		if err != nil {
			return uuid.Nil, err
		}
		if exists {
			return uuid.Nil, ErrIdempotencyConflict
		}
		rev, err := deliveryfence.Advance(ctx, tx, h.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		id := uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_copy_absence_receipt(id,workspace_id,plan_id,copy_id,dispatch_id,inventory_call_id,plan_hash,response_sha256,observed_at,confirmed_at,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, h.WorkspaceID, p.ID, c.ID, a.DispatchID, inventory.CallID, p.ContentHash, inventory.ResponseSHA256, inventory.ObservedAt, at, rev)
		if err != nil {
			return uuid.Nil, err
		}
		return id, appendDispositionFacts(ctx, tx, "LifecycleExactCopyAbsenceVerified", "LIFECYCLE_COPY_ABSENCE", id, caller, rev, at, map[string]any{"copy_id": c.ID, "plan_id": p.ID, "inventory_call_id": inventory.CallID, "coverage": "EXACT_VERSION_IN_PROVIDER_ACCESSIBLE_NAMESPACE", "excluded": "provider internal media/backups/replicas, external recipients"})
	})
}
func (s *ExactExecution) FinalizeContentDisposition(ctx context.Context, h CommandHeader, planID uuid.UUID) (FactResult, error) {
	if s == nil || s.disposition == nil || !s.disposition.configured() {
		return FactResult{}, ErrStorageScope
	}
	d := s.disposition
	o := d.occupancy
	return o.command(ctx, h, "LIFECYCLE.FINALIZE_DISPOSITION", CapabilityVerifyAbsence, planID, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		p, err := d.lockedPlan(ctx, tx, h.WorkspaceID, planID)
		if err != nil {
			return uuid.Nil, err
		}
		caller, at, err := o.authorize(ctx, tx, h.WorkspaceID, CapabilityVerifyAbsence, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		rev, err := deliveryfence.Advance(ctx, tx, h.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		id := uuid.New()
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_plan_completion(id,workspace_id,plan_id,plan_hash,target_count,confirmed_at,dependency_revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, h.WorkspaceID, p.ID, p.ContentHash, len(p.Targets), at, rev)
		if err != nil {
			return uuid.Nil, err
		}
		return id, appendDispositionFacts(ctx, tx, "LifecycleContentDispositionFinalized", "LIFECYCLE_PLAN_COMPLETION", id, caller, rev, at, map[string]any{"plan_id": p.ID, "plan_hash": p.ContentHash, "verified_exact_versions": len(p.Targets), "coverage": "EXACT_VERSION_IN_PROVIDER_ACCESSIBLE_NAMESPACE"})
	})
}
func ObservationHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// A completed DENIED send plus a newly instrumented exact positive HEAD can
// resolve its reservation. It preserves the original invocation/cost/history;
// it does not grant another DELETE, which requires a new fresh dispatch/start.
func (s *ExactExecution) ReconcileDeniedAttempt(ctx context.Context, h CommandHeader, attemptID uuid.UUID, presence ExactCopyInspection) (FactResult, error) {
	if s == nil || s.disposition == nil || !s.disposition.configured() {
		return FactResult{}, ErrStorageScope
	}
	d := s.disposition
	o := d.occupancy
	return o.command(ctx, h, "LIFECYCLE.RECONCILE_DENIED", CapabilityVerifyAbsence, struct{ Attempt, Call uuid.UUID }{attemptID, presence.CallID}, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		a, err := o.repo.GetAttempt(ctx, tx, h.WorkspaceID, attemptID, false)
		if err != nil {
			return uuid.Nil, err
		}
		p, err := d.lockedPlan(ctx, tx, h.WorkspaceID, a.PlanID)
		if err != nil {
			return uuid.Nil, err
		}
		var c domain.Copy
		for _, target := range p.Targets {
			if target.ID == a.CopyID {
				c = target
			}
		}
		caller, at, err := o.authorize(ctx, tx, h.WorkspaceID, CapabilityVerifyAbsence, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		if presence.Copy != c || presence.CallID == uuid.Nil || presence.ObservedAt.After(at) || at.Sub(presence.ObservedAt) > time.Minute {
			return uuid.Nil, ErrStorageScope
		}
		var observationID uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id FROM lifecycle_provider_observation WHERE attempt_id=$1 AND certainty='CONFIRMED' AND outcome='DENIED' ORDER BY observed_at,id LIMIT 1`, a.ID).Scan(&observationID)
		if err != nil {
			return uuid.Nil, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_dispatch_release(dispatch_id,workspace_id,observation_id,verification_call_id) VALUES($1,$2,$3,$4)`, a.DispatchID, h.WorkspaceID, observationID, presence.CallID)
		if err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, h.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		return a.DispatchID, appendDispositionFacts(ctx, tx, ReservationReleasedEvent, "LIFECYCLE_DISPATCH", a.DispatchID, caller, revision, at, map[string]any{"reason": "CONFIRMED_DENIED_EXACT_VERSION_PRESENT", "observation_id": observationID, "verification_call_id": presence.CallID})
	})
}
