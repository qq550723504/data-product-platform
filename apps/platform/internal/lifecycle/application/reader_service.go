package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

const StorageReadPinned = "READ_PINNED"

type fixtureReaderCallerKey struct{}

// Called by the trusted transport after authentication, never from body fields.
func WithFixtureReaderCaller(ctx context.Context, p principal.RequestPrincipal) context.Context {
	return context.WithValue(ctx, fixtureReaderCallerKey{}, p)
}

// Only a server-owned adapter supplies this result. StoppedAt means its actual
// native response/read owner stopped, not a caller assertion or lease expiry.
type PinnedReadResult struct {
	Copy          domain.Copy
	PinID, CallID uuid.UUID
	Bytes         []byte
	StoppedAt     time.Time
}
type PinnedReaderPort interface {
	ReadExactPinned(context.Context, domain.Copy, domain.ReaderPin) (PinnedReadResult, error)
}

// A transport with buffering must synchronously finish its actual output
// (including flush) before returning. Context caps the owned write deadline.
type PinnedOutputWriter interface {
	WriteContext(context.Context, []byte) (int, error)
}

type FixtureReadResult struct {
	Admission     ReaderAdmission
	CallID        uuid.UUID
	BytesWritten  int64
	TerminationID uuid.UUID
	StopConfirmed bool
}

// Pin was issued by a server-owned Delivery Command in its ISSUED transaction.
// This method never admits or replays a second reader. The single-send journal
// rejects reuse of the same pin, including an unknown physical attempt.
func (s *FixtureReaderService) ReadPinnedTo(ctx context.Context, w, pinID uuid.UUID, dst io.Writer) (FixtureReadResult, error) {
	var pin domain.ReaderPin
	if s == nil || dst == nil {
		return FixtureReadResult{}, ErrStorageUnsupported
	}
	p, ok := ctx.Value(fixtureReaderCallerKey{}).(principal.RequestPrincipal)
	if !ok || p.WorkspaceID != w {
		return FixtureReadResult{}, principal.ErrUnauthenticated
	}
	e := s.occupancy.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		pin, e = s.occupancy.repo.GetReaderPin(ctx, tx, w, pinID)
		if e != nil {
			return e
		}
		if pin.Kind != domain.FixtureDelivery || pin.ActorID != p.ActorID || pin.PrincipalRef != p.Subject {
			return ErrStorageDenied
		}
		ended, e := s.occupancy.repo.ReaderTerminated(ctx, tx, w, pin.ID)
		if e != nil {
			return e
		}
		if ended {
			return ErrStorageDenied
		}
		return nil
	})
	if e != nil {
		return FixtureReadResult{}, e
	}
	return s.readAdmitted(ctx, AdmitFixtureReaderCommand{CommandHeader: CommandHeader{w, "owned-transfer-" + pin.ID.String()}, CopyID: pin.CopyID, ReaderID: pin.ReaderID}, ReaderAdmission{Pin: pin, Admitted: true}, dst)
}

// FixtureReaderService owns native reads and the synchronous output writer.
// It is a closed synthetic composition, not a production delivery command.
// A crash, hung writer or unconfirmed native stop retains the durable pin.
type FixtureReaderService struct {
	occupancy *OccupancyService
	port      PinnedReaderPort
	mu        sync.Mutex
	stops     map[uuid.UUID]domain.ReaderTermination
	recovery  ReaderTerminationVerifier
}

func validatePinnedRead(copy domain.Copy, pin domain.ReaderPin, r PinnedReadResult) error {
	if r.PinID != pin.ID || r.Copy != copy || r.StoppedAt.IsZero() || r.StoppedAt.Before(pin.StartedAt) || r.StoppedAt.After(time.Now().UTC()) {
		return domain.ErrTerminationUnknown
	}
	sum := sha256.Sum256(r.Bytes)
	if r.CallID == uuid.Nil || int64(len(r.Bytes)) != copy.ByteSize || hex.EncodeToString(sum[:]) != copy.ContentSHA256 {
		return ErrStorageContract
	}
	return nil
}

// Compose once before serving. Do not replace another reader owner's verifier.
func NewFixtureReaderService(o *OccupancyService, p PinnedReaderPort) *FixtureReaderService {
	return NewFixtureReaderServiceWithRecovery(o, p, nil)
}

// Recovery trust is installed by the closed server composition, never supplied
// by a request. Normal synchronous stop proofs keep the same verifier owner.
func NewFixtureReaderServiceWithRecovery(o *OccupancyService, p PinnedReaderPort, recovery ReaderTerminationVerifier) *FixtureReaderService {
	if o == nil || !o.configured() || o.useGate == nil || o.termination != nil || p == nil {
		return nil
	}
	s := &FixtureReaderService{occupancy: o, port: p, stops: map[uuid.UUID]domain.ReaderTermination{}, recovery: recovery}
	o.termination = s
	return s
}

func (s *FixtureReaderService) VerifyTermination(ctx context.Context, tx pgx.Tx, pin domain.ReaderPin, outcome string, at time.Time) (domain.ReaderTermination, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.stops[pin.ID]
	if !ok && s.recovery != nil {
		return s.recovery.VerifyTermination(ctx, tx, pin, outcome, at)
	}
	if !ok || p.Generation != pin.Generation || p.ReaderID != pin.ReaderID || p.Outcome != outcome {
		return domain.ReaderTermination{}, domain.ErrTerminationUnknown
	}
	return p, nil
}

func (s *FixtureReaderService) ReadTo(ctx context.Context, cmd AdmitFixtureReaderCommand, dst io.Writer) (FixtureReadResult, error) {
	var result FixtureReadResult
	if s == nil || dst == nil {
		return result, ErrStorageUnsupported
	}
	p, ok := ctx.Value(fixtureReaderCallerKey{}).(principal.RequestPrincipal)
	if !ok || p.WorkspaceID != cmd.WorkspaceID || p.ActorID == uuid.Nil || !validText(p.Subject, 255) || !p.HasCapability(CapabilityAdmitReader) {
		return result, principal.ErrUnauthenticated
	}
	a, e := s.occupancy.AdmitFixtureReader(ctx, cmd)
	if e != nil {
		return result, e
	}
	result.Admission = a
	if !a.Admitted {
		return result, nil
	} // historical replay never opens bytes
	return s.readAdmitted(ctx, cmd, a, dst)
}

func (s *FixtureReaderService) readAdmitted(ctx context.Context, cmd AdmitFixtureReaderCommand, a ReaderAdmission, dst io.Writer) (FixtureReadResult, error) {
	result := FixtureReadResult{Admission: a}
	var copy domain.Copy
	e := s.occupancy.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		cs, e := s.occupancy.repo.LoadFixtureCopies(ctx, tx, cmd.WorkspaceID, []uuid.UUID{cmd.CopyID})
		if e != nil {
			return e
		}
		copy = cs[0]
		return nil
	})
	if e != nil {
		return result, e
	} // no proof owner established: retain pin
	var readCtx context.Context
	var cancel context.CancelFunc
	if a.Pin.UseUntil != nil {
		readCtx, cancel = context.WithDeadline(ctx, *a.Pin.UseUntil)
	} else {
		readCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()
	read, readErr := s.port.ReadExactPinned(readCtx, copy, a.Pin)
	result.CallID = read.CallID
	if read.PinID != a.Pin.ID || read.Copy != copy || read.StoppedAt.Before(a.Pin.StartedAt) || read.StoppedAt.After(time.Now().UTC()) || read.StoppedAt.IsZero() {
		return result, errors.Join(readErr, domain.ErrTerminationUnknown)
	}
	if readErr == nil {
		sum := sha256.Sum256(read.Bytes)
		if int64(len(read.Bytes)) != copy.ByteSize || hex.EncodeToString(sum[:]) != copy.ContentSHA256 || read.CallID == uuid.Nil {
			readErr = ErrStorageContract
		}
	}
	if readErr == nil {
		readErr = readCtx.Err()
	}
	if readErr == nil {
		var n int
		var writeErr error
		if output, ok := dst.(PinnedOutputWriter); ok {
			n, writeErr = output.WriteContext(readCtx, read.Bytes)
		} else {
			n, writeErr = dst.Write(read.Bytes)
		}
		result.BytesWritten = int64(n)
		readErr = writeErr
		if writeErr == nil && n != len(read.Bytes) {
			readErr = io.ErrShortWrite
		}
	}
	if readErr == nil {
		readErr = readCtx.Err()
	}
	// Native owner returned with a positive stop observation; the sole writer
	// also returned. Cancellation alone never reaches this point while I/O runs.
	outcome := domain.ReaderCompleted
	if readErr != nil || readCtx.Err() != nil {
		outcome = domain.ReaderCancelled
	}
	proof := domain.ReaderTermination{ID: uuid.New(), WorkspaceID: a.Pin.WorkspaceID, PinID: a.Pin.ID, ReaderID: a.Pin.ReaderID, Generation: a.Pin.Generation, Outcome: outcome, ObservedAt: time.Now().UTC()}
	if a.Pin.Kind == domain.FixtureDelivery {
		proof.BytesWritten = &result.BytesWritten
	}
	s.mu.Lock()
	s.stops[a.Pin.ID] = proof
	s.mu.Unlock()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	termination, e := s.occupancy.TerminateFixtureReader(finishCtx, TerminateFixtureReaderCommand{CommandHeader: CommandHeader{cmd.WorkspaceID, "reader-stop-" + a.Pin.ID.String()}, PinID: a.Pin.ID, Generation: a.Pin.Generation, Outcome: outcome})
	if e != nil {
		return result, errors.Join(readErr, e)
	}
	result.TerminationID = termination.ID
	result.StopConfirmed = true
	return result, readErr
}

// The native transport invokes this immediately before I/O. The durable pin,
// owner/generation and all current facts are checked under the existing fence.
type PinnedReaderCallJournal struct {
	Service      *OccupancyService
	Observations StorageCallJournal
	SubjectGate  PinnedSubjectGate
}
type PinnedSubjectGate interface {
	ValidatePinnedSubject(context.Context, pgx.Tx, domain.ReaderPin) error
}

func (j PinnedReaderCallJournal) ObserveStorageCall(ctx context.Context, c StorageCallObservation) error {
	return j.Observations.ObserveStorageCall(ctx, c)
}
func (j PinnedReaderCallJournal) StartStorageCall(ctx context.Context, c StorageCallStart) error {
	if j.Service == nil || !j.Service.configured() || j.Observations == nil || c.Operation != StorageReadPinned || c.ReaderPin == nil || c.ReaderPin.Validate() != nil || c.ID == uuid.Nil || c.Copy.Validate() != nil || c.CopyOperationID != uuid.Nil || c.CopyIntent != nil || c.StartedAt.IsZero() {
		return ErrStorageScope
	}
	s := j.Service
	c.StartedAt = c.StartedAt.UTC().Truncate(time.Microsecond)
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, e := deliveryfence.Lock(ctx, tx, c.Copy.WorkspaceID)
		if e != nil {
			return e
		}
		cs, e := s.repo.LockFixtureCopies(ctx, tx, c.Copy.WorkspaceID, []uuid.UUID{c.Copy.ID})
		if e != nil {
			return e
		}
		if cs[0] != c.Copy {
			return ErrStorageScope
		}
		pin, e := s.repo.GetReaderPin(ctx, tx, c.Copy.WorkspaceID, c.ReaderPin.ID)
		if e != nil {
			return e
		}
		if !sameReaderPin(pin, *c.ReaderPin) || pin.CopyID != c.Copy.ID {
			return ErrStorageScope
		}
		terminated, e := s.repo.ReaderTerminated(ctx, tx, pin.WorkspaceID, pin.ID)
		if e != nil {
			return e
		}
		if terminated {
			return ErrStorageDenied
		}
		if e = validateCurrentReaderSubject(ctx, tx, pin); e != nil {
			return e
		}
		if j.SubjectGate != nil {
			if e = j.SubjectGate.ValidatePinnedSubject(ctx, tx, pin); e != nil {
				return e
			}
		}
		caller, at, e := s.authorize(ctx, tx, pin.WorkspaceID, CapabilityAdmitReader, nil)
		if e != nil {
			return e
		}
		if caller.ActorID != pin.ActorID || caller.Subject != pin.PrincipalRef || c.StartedAt.Before(pin.StartedAt) || c.StartedAt.After(at) {
			return ErrStorageDenied
		}
		decision, e := s.currentReaderDecision(ctx, tx, c.Copy, pin.ReaderID, caller, rev, at)
		if e != nil {
			return e
		}
		if pin.ExecutionID != nil {
			rows, e := tx.Query(ctx, `SELECT p.copy_id FROM lifecycle_reader_pin p WHERE p.execution_id=$1 ORDER BY p.copy_id`, *pin.ExecutionID)
			if e != nil {
				return e
			}
			var ids []uuid.UUID
			for rows.Next() {
				var id uuid.UUID
				if e = rows.Scan(&id); e != nil {
					rows.Close()
					return e
				}
				ids = append(ids, id)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			all, e := s.repo.LockFixtureCopies(ctx, tx, pin.WorkspaceID, ids)
			if e != nil {
				return e
			}
			for _, source := range all {
				if _, e = s.currentReaderDecision(ctx, tx, source, pin.ReaderID, caller, rev, at); e != nil {
					return e
				}
			}
		}
		_, at, e = s.authorize(ctx, tx, pin.WorkspaceID, CapabilityAdmitReader, &caller)
		if e != nil {
			return e
		}
		if domain.ValidateWindowAt(pin.WindowMode, pin.UseUntil, at) != nil || domain.ValidateWindowAt(decision.WindowMode, decision.UseUntil, at) != nil {
			return ErrStorageDenied
		}
		tag, e := tx.Exec(ctx, `INSERT INTO lifecycle_storage_call(id,workspace_id,copy_id,operation,started_at,actor_id,principal_ref,dependency_revision,reader_pin_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, c.ID, pin.WorkspaceID, c.Copy.ID, c.Operation, c.StartedAt, caller.ActorID, caller.Subject, rev, pin.ID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return ErrIdempotencyConflict
		}
		return appendOccupancyFacts(ctx, tx, StorageCallStartedEvent, "LIFECYCLE_STORAGE_CALL", c.ID, caller, rev, at, map[string]any{"copy_id": c.Copy.ID, "reader_pin_id": pin.ID, "generation": pin.Generation, "operation": c.Operation})
	})
}
func sameReaderPin(a, b domain.ReaderPin) bool {
	if !samePinUUID(a.ExecutionID, b.ExecutionID) || !samePinUUID(a.DeliveryOperationID, b.DeliveryOperationID) {
		return false
	}
	if a.ID != b.ID || a.WorkspaceID != b.WorkspaceID || a.CopyID != b.CopyID || a.ReaderID != b.ReaderID || a.Kind != b.Kind || a.ActorID != b.ActorID || a.PrincipalRef != b.PrincipalRef || a.Generation != b.Generation || a.GateRevision != b.GateRevision || !a.StartedAt.Equal(b.StartedAt) || a.WindowMode != b.WindowMode {
		return false
	}
	if a.UseUntil == nil || b.UseUntil == nil {
		return a.UseUntil == nil && b.UseUntil == nil
	}
	return a.UseUntil.Equal(*b.UseUntil)
}

func samePinUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
