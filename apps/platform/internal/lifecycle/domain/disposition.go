package domain

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidDispositionFact = errors.New("invalid lifecycle disposition fact")

const (
	InvocationUncertain = "UNCERTAIN"
	InvocationConfirmed = "CONFIRMED"
	InvocationNotSent   = "NOT_SENT"
	ProviderAccepted    = "ACCEPTED"
	ProviderDenied      = "DENIED"
	ProviderFailed      = "FAILED"
	ProviderUnknown     = "UNKNOWN"
)

type DispositionApproval struct {
	ID                 uuid.UUID
	WorkspaceID        uuid.UUID
	PlanID             uuid.UUID
	PlanHash           string
	Action             string
	BasisRef           string
	ActorID            uuid.UUID
	PrincipalRef       string
	ValidFrom          time.Time
	ExpiresAt          time.Time
	DependencyRevision int64
}

func (a DispositionApproval) ValidateFor(p Plan) error {
	if p.VerifyFrozenScope() != nil || a.ID == uuid.Nil || a.WorkspaceID != p.WorkspaceID || a.PlanID != p.ID ||
		a.PlanHash != p.ContentHash || a.Action != p.Action || a.ActorID == uuid.Nil || !boundedText(a.PrincipalRef, 255) ||
		!boundedText(a.BasisRef, 255) || a.ValidFrom.IsZero() || a.ValidFrom.Before(*p.FinalizedAt) ||
		!a.ExpiresAt.After(a.ValidFrom) || a.DependencyRevision <= 0 {
		return ErrInvalidDispositionFact
	}
	return nil
}

// A physical start proves admission, not that the provider was called. There is
// no automatic retry/expiry that can make this immutable identity NOT_SENT.
type ProviderAttempt struct {
	ID                 uuid.UUID
	WorkspaceID        uuid.UUID
	DispatchID         uuid.UUID
	PlanID             uuid.UUID
	CopyID             uuid.UUID
	Action             string
	ProviderRequestKey string
	StartedAt          time.Time
	DependencyRevision int64
}

func (a ProviderAttempt) Validate() error {
	if a.ID == uuid.Nil || a.WorkspaceID == uuid.Nil || a.DispatchID == uuid.Nil || a.PlanID == uuid.Nil || a.CopyID == uuid.Nil ||
		a.Action != DeleteExact || a.ProviderRequestKey != "lifecycle-delete:"+a.ID.String() || a.StartedAt.IsZero() || a.DependencyRevision <= 0 {
		return ErrInvalidDispositionFact
	}
	return nil
}

// The worker/verifier supplies a secret-free commitment to its server-owned
// observation. Public command input cannot choose certainty or forge stopped
// I/O. UNKNOWN observations survive a later authoritative resolution.
type ProviderObservation struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	AttemptID   uuid.UUID
	DispatchID  uuid.UUID
	CopyID      uuid.UUID
	Certainty   string
	Outcome     string
	ProofSHA256 string
	ObservedAt  time.Time
	InvokedAt   *time.Time
}

func (o ProviderObservation) ValidateFor(a ProviderAttempt, at time.Time) error {
	digest, err := hex.DecodeString(o.ProofSHA256)
	if a.Validate() != nil || at.IsZero() || o.ID == uuid.Nil || o.WorkspaceID != a.WorkspaceID || o.AttemptID != a.ID ||
		o.DispatchID != a.DispatchID || o.CopyID != a.CopyID || err != nil || len(digest) != 32 ||
		strings.ToLower(o.ProofSHA256) != o.ProofSHA256 || o.ObservedAt.Before(a.StartedAt) || o.ObservedAt.After(at) {
		return ErrInvalidDispositionFact
	}
	switch o.Certainty {
	case InvocationUncertain:
		if o.Outcome != ProviderUnknown || o.InvokedAt != nil {
			return ErrInvalidDispositionFact
		}
	case InvocationNotSent:
		if o.Outcome != InvocationNotSent || o.InvokedAt != nil {
			return ErrInvalidDispositionFact
		}
	case InvocationConfirmed:
		if o.InvokedAt == nil || o.InvokedAt.Before(a.StartedAt) || o.InvokedAt.After(o.ObservedAt) ||
			(o.Outcome != ProviderAccepted && o.Outcome != ProviderDenied && o.Outcome != ProviderFailed && o.Outcome != ProviderUnknown) {
			return ErrInvalidDispositionFact
		}
	default:
		return ErrInvalidDispositionFact
	}
	return nil
}
