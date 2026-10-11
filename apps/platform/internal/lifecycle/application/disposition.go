package application

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

const (
	CapabilityApproveDisposition = "LIFECYCLE_APPROVE_DISPOSITION"
	CapabilityRevokeApproval     = "LIFECYCLE_REVOKE_APPROVAL"
	CapabilityDispatch           = "LIFECYCLE_DISPATCH"
	CapabilityObserveAttempt     = "LIFECYCLE_OBSERVE_ATTEMPT"
	DispositionApprovedEvent     = "LifecycleDispositionApproved"
	ApprovalRevokedEvent         = "LifecycleApprovalRevoked"
	DispositionDispatchedEvent   = "LifecycleDispositionDispatched"
	DispositionBlockedEvent      = "LifecycleDispositionBlocked"
	ProviderObservedEvent        = "LifecycleProviderAttemptObserved"
	ReservationReleasedEvent     = "LifecycleDispositionReservationReleased"
)

// Ports inspect current server facts in the short fenced transaction. They
// must not perform external I/O, accept client ALLOWED flags or substitute old
// checks for current approval authority, inventory, policy or reference facts.
type ApprovalAuthority interface {
	CheckApproverAuthority(context.Context, pgx.Tx, domain.DispositionApproval, time.Time) (domain.Check, error)
}
type DisposalDependencyRequest struct {
	Plan               domain.Plan
	Copy               domain.Copy
	Caller             principal.RequestPrincipal
	AsOf               time.Time
	DependencyRevision int64
}
type DisposalDependencies struct {
	WorkspaceID             uuid.UUID
	PlanID                  uuid.UUID
	CopyID                  uuid.UUID
	PlanHash                string
	ActorID                 uuid.UUID
	CheckedAt               time.Time
	DependencyRevision      int64
	Retention               []domain.RetentionBoundary
	DisposalDueAt           time.Time
	InventoryComplete       domain.Check
	ReferencesReleased      domain.Check
	ProviderRetentionAllows domain.Check
	LedgerCurrent           domain.Check
	WindowMode              string
	ValidUntil              *time.Time
}
type CurrentDisposalDependencies interface {
	EvaluateDisposalDependencies(context.Context, pgx.Tx, DisposalDependencyRequest) (DisposalDependencies, error)
}
type ProviderObservationVerifier interface {
	VerifyProviderObservation(context.Context, pgx.Tx, domain.ProviderAttempt, string, time.Time) (domain.ProviderObservation, error)
}

type ApproveContentDispositionCommand struct {
	CommandHeader
	PlanID    uuid.UUID
	BasisRef  string
	ExpiresAt time.Time
}
type RevokeDispositionApprovalCommand struct {
	CommandHeader
	ApprovalID uuid.UUID
	BasisRef   string
}
type DispatchContentDispositionCommand struct {
	CommandHeader
	PlanID     uuid.UUID
	ApprovalID uuid.UUID
	CopyID     uuid.UUID
}

// ObservationRef is a server-owned receipt handle; it is neither a provider
// error string nor authority to select certainty/outcome from an HTTP body.
type RecordProviderObservationCommand struct {
	CommandHeader
	AttemptID      uuid.UUID
	ObservationRef string
}
type DispatchResult struct {
	EvaluationID uuid.UUID
	DispatchID   uuid.UUID
	AttemptID    uuid.UUID
	Admitted     bool
	Replayed     bool
	Blockers     []string
}

type DispositionService struct {
	occupancy    *OccupancyService
	approver     ApprovalAuthority
	dependencies CurrentDisposalDependencies
	observations ProviderObservationVerifier
}

func NewDispositionService(o *OccupancyService, a ApprovalAuthority, d CurrentDisposalDependencies, v ProviderObservationVerifier) *DispositionService {
	return &DispositionService{occupancy: o, approver: a, dependencies: d, observations: v}
}
func (s *DispositionService) configured() bool { return s != nil && s.occupancy.configured() }

func (s *DispositionService) lockedPlan(ctx context.Context, tx pgx.Tx, w, id uuid.UUID) (domain.Plan, error) {
	o := s.occupancy
	p, err := o.repo.GetPlan(ctx, tx, w, id, false)
	if err != nil {
		return p, err
	}
	if err = p.VerifyFrozenScope(); err != nil {
		return p, err
	}
	var ids []uuid.UUID
	for _, c := range p.Targets {
		ids = append(ids, c.ID)
	}
	if _, err = o.repo.LockFixtureCopies(ctx, tx, w, ids); err != nil {
		return p, err
	}
	p, err = o.repo.GetPlan(ctx, tx, w, id, true)
	if err != nil {
		return p, err
	}
	return p, p.VerifyFrozenScope()
}

func (s *DispositionService) ApproveContentDisposition(ctx context.Context, cmd ApproveContentDispositionCommand) (FactResult, error) {
	if !s.configured() || cmd.PlanID == uuid.Nil || !validText(cmd.BasisRef, 255) || cmd.ExpiresAt.IsZero() {
		return FactResult{}, domain.ErrInvalidDispositionFact
	}
	cmd.ExpiresAt = cmd.ExpiresAt.UTC().Truncate(time.Microsecond)
	o := s.occupancy
	return o.command(ctx, cmd.CommandHeader, "LIFECYCLE.APPROVE_DISPOSITION", CapabilityApproveDisposition, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		p, err := s.lockedPlan(ctx, tx, cmd.WorkspaceID, cmd.PlanID)
		if err != nil {
			return uuid.Nil, err
		}
		caller, at, err := o.authorize(ctx, tx, cmd.WorkspaceID, CapabilityApproveDisposition, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		a := domain.DispositionApproval{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, PlanID: p.ID, PlanHash: p.ContentHash, Action: p.Action,
			BasisRef: cmd.BasisRef, ActorID: caller.ActorID, PrincipalRef: caller.Subject, ValidFrom: at, ExpiresAt: cmd.ExpiresAt, DependencyRevision: revision}
		if err := a.ValidateFor(p); err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_approval(id,workspace_id,plan_id,plan_hash,action,basis_ref,actor_id,principal_ref,valid_from,expires_at,dependency_revision)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, a.ID, a.WorkspaceID, a.PlanID, a.PlanHash, a.Action, a.BasisRef, a.ActorID, a.PrincipalRef, a.ValidFrom, a.ExpiresAt, a.DependencyRevision); err != nil {
			return uuid.Nil, err
		}
		return a.ID, appendDispositionFacts(ctx, tx, DispositionApprovedEvent, "LIFECYCLE_APPROVAL", a.ID, caller, revision, at, map[string]any{"plan_id": p.ID, "plan_hash": p.ContentHash, "action": p.Action})
	})
}

func (s *DispositionService) RevokeDispositionApproval(ctx context.Context, cmd RevokeDispositionApprovalCommand) (FactResult, error) {
	if !s.configured() || cmd.ApprovalID == uuid.Nil || !validText(cmd.BasisRef, 255) {
		return FactResult{}, domain.ErrInvalidDispositionFact
	}
	o := s.occupancy
	return o.command(ctx, cmd.CommandHeader, "LIFECYCLE.REVOKE_APPROVAL", CapabilityRevokeApproval, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		a, err := o.repo.GetApproval(ctx, tx, cmd.WorkspaceID, cmd.ApprovalID, false)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := s.lockedPlan(ctx, tx, cmd.WorkspaceID, a.PlanID); err != nil {
			return uuid.Nil, err
		}
		if _, err := o.repo.GetApproval(ctx, tx, cmd.WorkspaceID, a.ID, true); err != nil {
			return uuid.Nil, err
		}
		caller, at, err := o.authorize(ctx, tx, cmd.WorkspaceID, CapabilityRevokeApproval, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		if at.Before(a.ValidFrom) {
			return uuid.Nil, domain.ErrInvalidDispositionFact
		}
		revoked, err := o.repo.ApprovalRevoked(ctx, tx, cmd.WorkspaceID, a.ID)
		if err != nil {
			return uuid.Nil, err
		}
		if revoked {
			return uuid.Nil, infrastructure.ErrAlreadyReleased
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		id := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_approval_revocation(id,workspace_id,approval_id,basis_ref,actor_id,principal_ref,occurred_at,dependency_revision)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, cmd.WorkspaceID, a.ID, cmd.BasisRef, caller.ActorID, caller.Subject, at, revision); err != nil {
			return uuid.Nil, err
		}
		return id, appendDispositionFacts(ctx, tx, ApprovalRevokedEvent, "LIFECYCLE_APPROVAL_REVOCATION", id, caller, revision, at, map[string]any{"approval_id": a.ID, "plan_id": a.PlanID})
	})
}

// Only a new committed admission returns Admitted=true. It creates an exclusive
// reservation and immutable physical start, but performs no provider call.
// Same-key replay and a start-only/UNKNOWN reservation never authorize resend.
func (s *DispositionService) DispatchContentDisposition(ctx context.Context, cmd DispatchContentDispositionCommand) (DispatchResult, error) {
	if !s.configured() || s.approver == nil || s.dependencies == nil || cmd.PlanID == uuid.Nil || cmd.ApprovalID == uuid.Nil || cmd.CopyID == uuid.Nil {
		return DispatchResult{}, domain.ErrInvalidDispositionFact
	}
	o := s.occupancy
	var result DispatchResult
	fact, err := o.command(ctx, cmd.CommandHeader, "LIFECYCLE.DISPATCH", CapabilityDispatch, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, gateRevision int64) (uuid.UUID, error) {
		p, err := s.lockedPlan(ctx, tx, cmd.WorkspaceID, cmd.PlanID)
		if err != nil {
			return uuid.Nil, err
		}
		var copy domain.Copy
		for _, c := range p.Targets {
			if c.ID == cmd.CopyID {
				copy = c
				break
			}
		}
		if copy.ID == uuid.Nil {
			return uuid.Nil, infrastructure.ErrScopeUnavailable
		}
		a, err := o.repo.GetApproval(ctx, tx, cmd.WorkspaceID, cmd.ApprovalID, true)
		if err != nil {
			return uuid.Nil, err
		}
		if err := a.ValidateFor(p); err != nil {
			return uuid.Nil, err
		}
		caller, at, err := o.authorize(ctx, tx, cmd.WorkspaceID, CapabilityDispatch, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		revoked, err := o.repo.ApprovalRevoked(ctx, tx, cmd.WorkspaceID, a.ID)
		if err != nil {
			return uuid.Nil, err
		}
		request := DisposalDependencyRequest{Plan: p, Copy: copy, Caller: caller, AsOf: at, DependencyRevision: gateRevision}
		dep, err := s.dependencies.EvaluateDisposalDependencies(ctx, tx, request)
		if err != nil {
			return uuid.Nil, err
		}
		if dep.WorkspaceID != p.WorkspaceID || dep.PlanID != p.ID || dep.CopyID != copy.ID || dep.PlanHash != p.ContentHash ||
			dep.ActorID != caller.ActorID || !dep.CheckedAt.Equal(at) || dep.DependencyRevision != gateRevision {
			return uuid.Nil, domain.ErrInvalidDispositionFact
		}
		caller, at, err = o.authorize(ctx, tx, cmd.WorkspaceID, CapabilityDispatch, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		current, err := s.approver.CheckApproverAuthority(ctx, tx, a, at)
		if err != nil {
			return uuid.Nil, err
		}
		if revoked {
			current = domain.Denied
		}
		occ, err := o.repo.CopyOccupancy(ctx, tx, copy)
		if err != nil {
			return uuid.Nil, err
		}
		decision := domain.EvaluateDeletePrerequisites(domain.DeletePrerequisites{AsOf: at, Plan: p,
			Approval:  domain.Approval{WorkspaceID: a.WorkspaceID, PlanHash: a.PlanHash, Action: a.Action, ValidFrom: a.ValidFrom, ExpiresAt: a.ExpiresAt, Current: current},
			Retention: dep.Retention, DisposalDueAt: dep.DisposalDueAt, CallerAuthority: domain.Satisfied, UseRestricted: occ.UseRestricted,
			InventoryComplete: dep.InventoryComplete, ReferencesReleased: dep.ReferencesReleased, ReadersDrained: occ.ReadersDrained,
			HoldFree: occ.HoldFree, ProviderRetentionAllows: dep.ProviderRetentionAllows, LedgerCurrent: dep.LedgerCurrent})
		if occ.UnresolvedDispatches > 0 {
			decision.Eligible = false
			decision.Blockers = append(decision.Blockers, "DISPOSITION_ALREADY_DISPATCHED_OR_UNKNOWN")
		}
		if domain.ValidateWindowAt(dep.WindowMode, dep.ValidUntil, at) != nil {
			decision.Eligible = false
			decision.Blockers = append(decision.Blockers, "DEPENDENCY_WINDOW_UNKNOWN_OR_EXPIRED")
		}
		if decision.Blockers == nil {
			decision.Blockers = []string{}
		}
		evaluationID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_gate_evaluation(id,workspace_id,plan_id,approval_id,copy_id,actor_id,principal_ref,checked_at,dependency_revision,eligible,blockers,retention_count)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, evaluationID, cmd.WorkspaceID, p.ID, a.ID, copy.ID, caller.ActorID, caller.Subject, at, gateRevision, decision.Eligible, decision.Blockers, len(dep.Retention)); err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_dependency_evaluation(evaluation_id,workspace_id,disposal_due_at,inventory_complete,references_released,provider_retention_allows,
			ledger_current,use_restricted,hold_free,readers_drained,unresolved_dispatches,window_mode,valid_until) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, evaluationID, cmd.WorkspaceID, knownTime(dep.DisposalDueAt),
			knownCheck(dep.InventoryComplete), knownCheck(dep.ReferencesReleased), knownCheck(dep.ProviderRetentionAllows), knownCheck(dep.LedgerCurrent), occ.UseRestricted, occ.HoldFree, occ.ReadersDrained, occ.UnresolvedDispatches, knownWindowMode(dep.WindowMode), dep.ValidUntil); err != nil {
			return uuid.Nil, err
		}
		for i, r := range dep.Retention {
			if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_retention_evaluation(evaluation_id,workspace_id,ordinal,policy_id,start_event_id,starts_at,retain_until)
				VALUES($1,$2,$3,$4,$5,$6,$7)`, evaluationID, cmd.WorkspaceID, i, knownID(r.PolicyID), knownID(r.StartEventID), knownTime(r.StartsAt), knownTime(r.RetainUntil)); err != nil {
				return uuid.Nil, err
			}
		}
		if err := o.repo.FinalizeDispatchEvaluation(ctx, tx, cmd.WorkspaceID, evaluationID); err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		result = DispatchResult{EvaluationID: evaluationID, Admitted: decision.Eligible, Blockers: decision.Blockers}
		eventType := DispositionBlockedEvent
		fields := map[string]any{"plan_id": p.ID, "approval_id": a.ID, "copy_id": copy.ID, "blockers": decision.Blockers}
		if decision.Eligible {
			eventType = DispositionDispatchedEvent
			result.DispatchID, result.AttemptID = uuid.New(), uuid.New()
			if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_dispatch(id,workspace_id,plan_id,approval_id,copy_id,evaluation_id,dispatched_at,dependency_revision)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, result.DispatchID, cmd.WorkspaceID, p.ID, a.ID, copy.ID, evaluationID, at, revision); err != nil {
				return uuid.Nil, err
			}
			attempt := domain.ProviderAttempt{ID: result.AttemptID, WorkspaceID: cmd.WorkspaceID, DispatchID: result.DispatchID, PlanID: p.ID, CopyID: copy.ID,
				Action: domain.DeleteExact, ProviderRequestKey: "lifecycle-delete:" + result.AttemptID.String(), StartedAt: at, DependencyRevision: revision}
			if err := attempt.Validate(); err != nil {
				return uuid.Nil, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_provider_attempt(id,workspace_id,dispatch_id,plan_id,copy_id,action,provider_request_key,started_at,dependency_revision)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, attempt.ID, attempt.WorkspaceID, attempt.DispatchID, attempt.PlanID, attempt.CopyID, attempt.Action, attempt.ProviderRequestKey, attempt.StartedAt, attempt.DependencyRevision); err != nil {
				return uuid.Nil, err
			}
			fields["dispatch_id"], fields["attempt_id"] = result.DispatchID, result.AttemptID
			if _, err := appendRecoveryWork(ctx, tx, attempt, "DURABLE_DISPATCH_RECOVERY", caller, revision, at); err != nil {
				return uuid.Nil, err
			}
		}
		return evaluationID, appendDispositionFacts(ctx, tx, eventType, "LIFECYCLE_DISPOSITION_GATE", evaluationID, caller, revision, at, fields)
	})
	if err != nil {
		return DispatchResult{}, err
	}
	if fact.Replayed {
		err = o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			e, err := o.repo.GetDispatchEvaluation(ctx, tx, cmd.WorkspaceID, fact.ID)
			if err != nil {
				return err
			}
			result = DispatchResult{EvaluationID: e.ID, Replayed: true, Blockers: e.Blockers}
			if e.DispatchID != nil {
				result.DispatchID = *e.DispatchID
			}
			if e.AttemptID != nil {
				result.AttemptID = *e.AttemptID
			}
			return nil
		})
	}
	return result, err
}

func (s *DispositionService) RecordProviderObservation(ctx context.Context, cmd RecordProviderObservationCommand) (FactResult, error) {
	if !s.configured() || s.observations == nil || cmd.AttemptID == uuid.Nil || !validText(cmd.ObservationRef, 255) {
		return FactResult{}, domain.ErrInvalidDispositionFact
	}
	o := s.occupancy
	return o.command(ctx, cmd.CommandHeader, "LIFECYCLE.RECORD_PROVIDER_OBSERVATION", CapabilityObserveAttempt, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		a, err := o.repo.GetAttempt(ctx, tx, cmd.WorkspaceID, cmd.AttemptID, false)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := s.lockedPlan(ctx, tx, cmd.WorkspaceID, a.PlanID); err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM lifecycle_disposition_dispatch WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, cmd.WorkspaceID, a.DispatchID); err != nil {
			return uuid.Nil, err
		}
		a, err = o.repo.GetAttempt(ctx, tx, cmd.WorkspaceID, a.ID, true)
		if err != nil {
			return uuid.Nil, err
		}
		caller, at, err := o.authorize(ctx, tx, cmd.WorkspaceID, CapabilityObserveAttempt, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		proof, err := s.observations.VerifyProviderObservation(ctx, tx, a, cmd.ObservationRef, at)
		if err != nil {
			return uuid.Nil, err
		}
		if err := proof.ValidateFor(a, at); err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_provider_observation(id,workspace_id,attempt_id,certainty,outcome,proof_sha256,observed_at,invoked_at,actor_id,principal_ref,dependency_revision)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, proof.ID, proof.WorkspaceID, proof.AttemptID, proof.Certainty, proof.Outcome, proof.ProofSHA256, proof.ObservedAt, proof.InvokedAt, caller.ActorID, caller.Subject, revision); err != nil {
			return uuid.Nil, err
		}
		if proof.Certainty == domain.InvocationConfirmed {
			if err := cost.AppendLifecycleProviderActivity(ctx, tx, cmd.WorkspaceID, a.ID, *proof.InvokedAt); err != nil {
				return uuid.Nil, err
			}
		}
		if proof.Certainty == domain.InvocationNotSent {
			tag, err := tx.Exec(ctx, `INSERT INTO lifecycle_dispatch_release(dispatch_id,workspace_id,observation_id) VALUES($1,$2,$3) ON CONFLICT(dispatch_id) DO NOTHING`, a.DispatchID, cmd.WorkspaceID, proof.ID)
			if err != nil {
				return uuid.Nil, err
			}
			if tag.RowsAffected() > 0 {
				if err := appendDispositionFacts(ctx, tx, ReservationReleasedEvent, "LIFECYCLE_DISPATCH", a.DispatchID, caller, revision, at, map[string]any{"observation_id": proof.ID, "reason": "NOT_SENT"}); err != nil {
					return uuid.Nil, err
				}
			}
		}
		return proof.ID, appendDispositionFacts(ctx, tx, ProviderObservedEvent, "LIFECYCLE_PROVIDER_OBSERVATION", proof.ID, caller, revision, at,
			map[string]any{"attempt_id": a.ID, "dispatch_id": a.DispatchID, "certainty": proof.Certainty, "outcome": proof.Outcome})
	})
}

func appendDispositionFacts(ctx context.Context, tx pgx.Tx, eventType, subject string, id uuid.UUID, caller principal.RequestPrincipal, revision int64, at time.Time, fields map[string]any) error {
	return appendLifecycleFacts(ctx, tx, "LIFECYCLE_DISPOSITION", eventType, subject, id, caller, revision, at, fields)
}

func knownTime(at time.Time) any {
	if at.IsZero() {
		return nil
	}
	return at
}
func knownID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
func knownCheck(c domain.Check) domain.Check {
	if c == domain.Satisfied || c == domain.Denied {
		return c
	}
	return domain.Unknown
}

func knownWindowMode(mode string) string {
	if mode == domain.UseUnbounded || mode == domain.UseDeadline {
		return mode
	}
	return "UNKNOWN"
}
