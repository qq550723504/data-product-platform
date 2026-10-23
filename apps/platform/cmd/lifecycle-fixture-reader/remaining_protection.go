package main

// These are fixture acceptance requirements, not disposal-domain rules. Expired
// GOVERNANCE retention correctly permits disposal in the domain; it cannot pass
// a test whose stated purpose is to prove a current native protection block.
import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func remainingProtectionDeadline(r remainingConfig) (time.Time, error) {
	end, err := time.Parse(time.RFC3339, pilot.RemainingNotAfter)
	if err != nil || r.CandidateUseUntil.IsZero() || r.CandidateUseUntil.After(end) {
		return time.Time{}, errors.New("native protection window differs")
	}
	// A rehearsal's short execution lifetime does not shorten the protection
	// required by the eventual fixed window and its two-hour read-only recovery.
	return end.Add(2 * time.Hour), nil
}

type nativeProtectionError struct{ Reason string }

func (e nativeProtectionError) Error() string { return "native protection required: " + e.Reason }

func requireRemainingProtection(p lc.CopyProtection, cp domain.Copy, at, until time.Time) error {
	reason := ""
	switch {
	case p.Copy != cp:
		reason = "EXACT_COPY_DIFFERS"
	case until.IsZero() || !at.Before(until):
		reason = "REQUIRED_HORIZON_EXPIRED"
	case p.ObservedAt.IsZero() || p.ObservedAt.After(at) || at.Sub(p.ObservedAt) > time.Minute:
		reason = "OBSERVATION_NOT_FRESH"
	case p.RetentionMode != "GOVERNANCE" || p.RetainUntil == nil:
		reason = "GOVERNANCE_RETENTION_MISSING"
	case p.LegalHold:
		reason = "LEGAL_HOLD_DIFFERS"
	case !at.Before(*p.RetainUntil):
		reason = "GOVERNANCE_RETENTION_EXPIRED"
	case p.RetainUntil.Before(until):
		reason = "GOVERNANCE_COVERAGE_SHORT"
	}
	if reason != "" {
		return nativeProtectionError{reason}
	}
	return nil
}

func remainingProtectedCopy(cp domain.Copy) bool {
	scope, err := pilot.RemainingScopeFor("mixed")
	if err != nil {
		return false
	}
	for _, expected := range scope.Copies {
		if expected.Key == "fixture/m1-c.csv" && cp == expected {
			return true
		}
	}
	return false
}

// Both the direct rehearsal and the later dispatch use actual inspector facts.
// Keeping this before commandDispatch prevents a stale protection assertion
// from creating an attempt that the test then incorrectly calls a rejection.
func protectedRemainingCommand(ctx context.Context, cp domain.Copy, until time.Time,
	inspect func(context.Context, domain.Copy) (lc.CopyProtection, error),
	command func() (lc.DispatchResult, error), now func() time.Time,
) (lc.DispatchResult, error) {
	p, err := inspect(ctx, cp)
	if err != nil {
		return lc.DispatchResult{}, errors.New("native protection inspection failed")
	}
	if err = requireRemainingProtection(p, cp, now(), until); err != nil {
		return lc.DispatchResult{}, err
	}
	return command()
}

type protectedDispatchObservation struct {
	Dispatch           lc.DispatchResult `json:"dispatch"`
	DispatchError      bool              `json:"dispatch_error"`
	DispatchErrorClass string            `json:"dispatch_error_class"`
	ProtectionReason   string            `json:"protection_reason"`
	Admitted           bool              `json:"admitted"`
	AttemptPresent     bool              `json:"attempt_present"`
	Reason             string            `json:"reason"`
	NativeDelete       int               `json:"native_delete"`
}

func observeProtectedDispatch(result lc.DispatchResult, err error) protectedDispatchObservation {
	o := protectedDispatchObservation{Dispatch: result, DispatchError: err != nil,
		Admitted: result.Admitted, AttemptPresent: result.AttemptID != uuid.Nil, Reason: "REJECTED_WITHOUT_ATTEMPT"}
	if err != nil {
		o.DispatchErrorClass = fmt.Sprintf("%T", err) // never serialize arbitrary error text or credentials
		var protection nativeProtectionError
		if errors.As(err, &protection) {
			o.ProtectionReason = protection.Reason
		}
		o.Reason = "DISPATCH_ERROR"
	} else if result.Admitted {
		o.Reason = "UNEXPECTED_ADMISSION"
	} else if result.AttemptID != uuid.Nil {
		o.Reason = "UNEXPECTED_ATTEMPT"
	}
	return o
}
