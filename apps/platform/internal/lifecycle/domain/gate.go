package domain

import (
	"time"

	"github.com/google/uuid"
)

// Check is explicit: zero, unknown and unrecognized values never mean ANY.
type Check string

const (
	Satisfied Check = "SATISFIED"
	Denied    Check = "DENIED"
	Unknown   Check = "UNKNOWN"
)

type RetentionBoundary struct {
	PolicyID     uuid.UUID
	StartEventID uuid.UUID
	StartsAt     time.Time
	RetainUntil  time.Time
}

type Approval struct {
	WorkspaceID uuid.UUID
	PlanHash    string
	Action      string
	ValidFrom   time.Time
	ExpiresAt   time.Time
	Current     Check
}

type DeletePrerequisites struct {
	AsOf                    time.Time
	Plan                    Plan
	Approval                Approval
	Retention               []RetentionBoundary
	DisposalDueAt           time.Time
	CallerAuthority         Check
	UseRestricted           Check
	InventoryComplete       Check
	ReferencesReleased      Check
	ReadersDrained          Check
	HoldFree                Check
	ProviderRetentionAllows Check
	LedgerCurrent           Check
}

type PrerequisiteDecision struct {
	Eligible bool
	Blockers []string
}

// EvaluateDeletePrerequisites is a pure preflight. Eligibility is not dispatch
// authorization: the command must re-read every current dependency under the
// shared fence and durably record admission/physical attempt before provider I/O.
// Use rights deliberately do not confer disposal authority.
func EvaluateDeletePrerequisites(in DeletePrerequisites) PrerequisiteDecision {
	var blockers []string
	if in.AsOf.IsZero() {
		blockers = append(blockers, "AS_OF_UNKNOWN")
	}
	if in.Plan.VerifyFrozenScope() != nil {
		blockers = append(blockers, "PLAN_NOT_FROZEN")
	}
	a := in.Approval
	if a.Current != Satisfied || a.WorkspaceID != in.Plan.WorkspaceID || a.PlanHash == "" ||
		a.PlanHash != in.Plan.ContentHash || a.Action != DeleteExact || a.ValidFrom.IsZero() ||
		a.ExpiresAt.IsZero() || in.AsOf.Before(a.ValidFrom) || !in.AsOf.Before(a.ExpiresAt) {
		blockers = append(blockers, "APPROVAL_NOT_CURRENT_FOR_SCOPE")
	}
	if len(in.Retention) == 0 {
		blockers = append(blockers, "RETENTION_UNKNOWN")
	}
	for _, r := range in.Retention {
		if r.PolicyID == uuid.Nil || r.StartEventID == uuid.Nil || r.StartsAt.IsZero() ||
			r.RetainUntil.IsZero() || r.RetainUntil.Before(r.StartsAt) || in.AsOf.Before(r.StartsAt) {
			blockers = append(blockers, "RETENTION_UNKNOWN")
			break
		}
		if in.AsOf.Before(r.RetainUntil) {
			blockers = append(blockers, "RETENTION_ACTIVE")
			break
		}
	}
	if in.DisposalDueAt.IsZero() || in.AsOf.Before(in.DisposalDueAt) {
		blockers = append(blockers, "DISPOSAL_NOT_DUE")
	}
	for _, c := range []struct {
		check  Check
		reason string
	}{
		{in.CallerAuthority, "DISPOSAL_AUTHORITY_NOT_CURRENT"},
		{in.UseRestricted, "USE_NOT_RESTRICTED"},
		{in.InventoryComplete, "INVENTORY_INCOMPLETE"},
		{in.ReferencesReleased, "REFERENCES_RETAINED_OR_UNKNOWN"},
		{in.ReadersDrained, "READERS_ACTIVE_OR_UNKNOWN"},
		{in.HoldFree, "HOLD_ACTIVE_OR_UNKNOWN"},
		{in.ProviderRetentionAllows, "PROVIDER_RETENTION_BLOCKED_OR_UNKNOWN"},
		{in.LedgerCurrent, "LEDGER_NOT_CURRENT"},
	} {
		if c.check != Satisfied {
			blockers = append(blockers, c.reason)
		}
	}
	return PrerequisiteDecision{Eligible: len(blockers) == 0, Blockers: blockers}
}
