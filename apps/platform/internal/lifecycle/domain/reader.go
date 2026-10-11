package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	FixtureReader   = "FIXTURE_READER"
	UseUnbounded    = "EXPLICIT_UNBOUNDED"
	UseDeadline     = "DEADLINE"
	ReaderCompleted = "COMPLETED"
	ReaderCancelled = "CANCELLATION_CONFIRMED"
)

var (
	ErrUseBlocked         = errors.New("current lifecycle use is blocked or unknown")
	ErrTerminationUnknown = errors.New("reader termination is not confirmed")
)

// UseDecision is returned by the server-owned current gate, not a request body.
// The gate owns current identity, rights, applicability, availability and policy;
// the application additionally reads persisted lifecycle restrictions itself.
type UseDecision struct {
	WorkspaceID uuid.UUID
	CopyID      uuid.UUID
	ReaderID    uuid.UUID
	ActorID     uuid.UUID
	CheckedAt   time.Time
	Decision    Check
	WindowMode  string
	UseUntil    *time.Time
}

func (d UseDecision) ValidateFor(workspace, copyID, readerID, actorID uuid.UUID, at time.Time) error {
	if at.IsZero() || workspace == uuid.Nil || copyID == uuid.Nil || readerID == uuid.Nil || actorID == uuid.Nil ||
		d.WorkspaceID != workspace || d.CopyID != copyID || d.ReaderID != readerID || d.ActorID != actorID ||
		d.Decision != Satisfied || !d.CheckedAt.Equal(at) {
		return ErrUseBlocked
	}
	return ValidateWindowAt(d.WindowMode, d.UseUntil, at)
}

// Re-check the declared window near admission after the current-facts query.
// A held revision cannot stop wall-clock expiry during a slow DB evaluation.
func ValidateWindowAt(mode string, until *time.Time, at time.Time) error {
	if at.IsZero() {
		return ErrUseBlocked
	}
	switch mode {
	case UseUnbounded:
		if until != nil {
			return ErrUseBlocked
		}
	case UseDeadline:
		if until == nil || until.IsZero() || !at.Before(*until) {
			return ErrUseBlocked
		}
	default:
		return ErrUseBlocked
	}
	return nil
}

type ReaderPin struct {
	ID           uuid.UUID
	WorkspaceID  uuid.UUID
	CopyID       uuid.UUID
	ReaderID     uuid.UUID
	Kind         string
	ActorID      uuid.UUID
	PrincipalRef string
	Generation   int64
	GateRevision int64
	StartedAt    time.Time
	WindowMode   string
	UseUntil     *time.Time
}

func (p ReaderPin) Validate() error {
	if p.ID == uuid.Nil || p.WorkspaceID == uuid.Nil || p.CopyID == uuid.Nil || p.ReaderID == uuid.Nil || p.ActorID == uuid.Nil ||
		p.Kind != FixtureReader || !boundedText(p.PrincipalRef, 255) || p.Generation <= 0 || p.GateRevision < 0 ||
		p.GateRevision >= p.Generation || p.StartedAt.IsZero() {
		return ErrUseBlocked
	}
	if p.WindowMode == UseUnbounded && p.UseUntil == nil {
		return nil
	}
	if p.WindowMode == UseDeadline && p.UseUntil != nil && p.UseUntil.After(p.StartedAt) {
		return nil
	}
	return ErrUseBlocked
}

// There is deliberately no lease expiry that can release this pin. Completion
// requires the reader owner's authoritative observation of stopped I/O.
type ReaderTermination struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	PinID       uuid.UUID
	ReaderID    uuid.UUID
	Generation  int64
	Outcome     string
	ObservedAt  time.Time
}

func (p ReaderTermination) ValidateFor(pin ReaderPin, requestedOutcome string, at time.Time) error {
	if pin.Validate() != nil || at.IsZero() || p.ID == uuid.Nil || p.WorkspaceID != pin.WorkspaceID || p.PinID != pin.ID || p.ReaderID != pin.ReaderID ||
		p.Generation != pin.Generation || p.Outcome != requestedOutcome || p.ObservedAt.IsZero() ||
		p.ObservedAt.Before(pin.StartedAt) || p.ObservedAt.After(at) ||
		(p.Outcome != ReaderCompleted && p.Outcome != ReaderCancelled) {
		return ErrTerminationUnknown
	}
	return nil
}

type Occupancy struct {
	CopyID               uuid.UUID
	UseRestricted        Check
	HoldFree             Check
	ReadersDrained       Check
	ActiveHolds          int64
	ActiveReaders        int64
	UnresolvedDispatches int64
}
