package application

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	deliverydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
)

// Selection is owned by the closed composition. DatasetVersion IDs resolve to
// exactly one registered versioned copy; neither caller URI nor namespace is used.
type FixtureSubjectPins struct {
	reader    *FixtureReaderService
	selection map[uuid.UUID]uuid.UUID
}

func NewFixtureSubjectPins(reader *FixtureReaderService, selection map[uuid.UUID]uuid.UUID) *FixtureSubjectPins {
	if reader == nil || len(selection) == 0 || len(selection) > 16 {
		return nil
	}
	owned := map[uuid.UUID]uuid.UUID{}
	for v, c := range selection {
		if v == uuid.Nil || c == uuid.Nil {
			return nil
		}
		owned[v] = c
	}
	return &FixtureSubjectPins{reader, owned}
}

func (s *FixtureSubjectPins) admit(ctx context.Context, tx pgx.Tx, w, version, subject uuid.UUID, kind string, capUntil *time.Time) (domain.ReaderPin, domain.Copy, error) {
	o := s.reader.occupancy
	id, ok := s.selection[version]
	if !ok {
		return domain.ReaderPin{}, domain.Copy{}, ErrStorageScope
	}
	rev, e := deliveryfence.Lock(ctx, tx, w)
	if e != nil {
		return domain.ReaderPin{}, domain.Copy{}, e
	}
	cs, e := o.repo.LockFixtureCopies(ctx, tx, w, []uuid.UUID{id})
	if e != nil {
		return domain.ReaderPin{}, domain.Copy{}, e
	}
	copy := cs[0]
	if copy.DatasetVersionID != version {
		return domain.ReaderPin{}, copy, ErrStorageScope
	}
	caller, at, e := o.authorize(ctx, tx, w, CapabilityAdmitReader, nil)
	if e != nil {
		return domain.ReaderPin{}, copy, e
	}
	decision, e := o.currentReaderDecision(ctx, tx, copy, subject, caller, rev, at)
	if e != nil {
		return domain.ReaderPin{}, copy, e
	}
	if capUntil != nil && (decision.UseUntil == nil || capUntil.Before(*decision.UseUntil)) {
		decision.WindowMode = domain.UseDeadline
		decision.UseUntil = capUntil
	}
	if decision.UseUntil != nil {
		until := decision.UseUntil.UTC().Truncate(time.Microsecond)
		decision.UseUntil = &until
	}
	_, at, e = o.authorize(ctx, tx, w, CapabilityAdmitReader, &caller)
	if e != nil {
		return domain.ReaderPin{}, copy, e
	}
	if e = domain.ValidateWindowAt(decision.WindowMode, decision.UseUntil, at); e != nil {
		return domain.ReaderPin{}, copy, e
	}
	generation, e := deliveryfence.Advance(ctx, tx, w)
	if e != nil {
		return domain.ReaderPin{}, copy, e
	}
	pin := domain.ReaderPin{ID: uuid.New(), WorkspaceID: w, CopyID: id, ReaderID: subject, Kind: kind, ActorID: caller.ActorID, PrincipalRef: caller.Subject, Generation: generation, GateRevision: rev, StartedAt: at, WindowMode: decision.WindowMode, UseUntil: decision.UseUntil}
	switch kind {
	case domain.FixtureProcessing:
		pin.ExecutionID = &subject
	case domain.FixtureDelivery:
		pin.DeliveryOperationID = &subject
	default:
		return pin, copy, ErrStorageScope
	}
	if e = o.repo.InsertReaderPin(ctx, tx, pin); e != nil {
		return pin, copy, e
	}
	e = appendOccupancyFacts(ctx, tx, ReaderAdmittedEvent, "LIFECYCLE_READER", pin.ID, caller, generation, at, map[string]any{"copy_id": id, "reader_id": subject, "kind": kind, "generation": generation})
	return pin, copy, e
}

// Called only by DirectDataService after its ISSUED projection and fresh gate,
// while that same transaction still owns the delivery authorization fence.
func (s *FixtureSubjectPins) AdmitDirectDataTransfer(ctx context.Context, tx pgx.Tx, op deliverydomain.Operation) (uuid.UUID, error) {
	if s == nil || tx == nil || op.Status != deliverydomain.StatusIssued || op.FreshCapExpiresAt == nil {
		return uuid.Nil, ErrStorageScope
	}
	caller, _, e := s.reader.occupancy.authorize(ctx, tx, op.WorkspaceID, CapabilityAdmitReader, nil)
	if e != nil {
		return uuid.Nil, e
	}
	if caller.Subject != op.PrincipalRef {
		return uuid.Nil, ErrStorageDenied
	}
	pin, _, e := s.admit(ctx, tx, op.WorkspaceID, op.DatasetVersionID, op.ID, domain.FixtureDelivery, op.FreshCapExpiresAt)
	return pin.ID, e
}

type processingScopeKey struct{}
type ownedInput struct {
	pin     domain.ReaderPin
	copy    domain.Copy
	read    bool
	stopped bool
	bytes   []byte
	open    int
	err     error
}
type processingScope struct {
	owner         *FixtureSubjectPins
	mu            sync.Mutex
	inputs        map[uuid.UUID]*ownedInput
	outputUnknown bool
}

// Existing ProcessingEngine remains the producer. This wrapper owns all input
// leases until the synchronous engine AND all read handles actually stop. A
// missing native-stop proof or crash keeps every durable pin; no lease reset.
func (s *FixtureSubjectPins) ExecuteProcessing(ctx context.Context, w, execution uuid.UUID, execute func(context.Context) error) error {
	if s == nil || execute == nil {
		return ErrStorageUnsupported
	}
	scope := &processingScope{owner: s, inputs: map[uuid.UUID]*ownedInput{}}
	o := s.reader.occupancy
	e := o.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, e := deliveryfence.Lock(ctx, tx, w); e != nil {
			return e
		}
		var status string
		var actor uuid.UUID
		if e := tx.QueryRow(ctx, `SELECT status,created_by FROM execution WHERE id=$1 AND workspace_id=$2 FOR KEY SHARE`, execution, w).Scan(&status, &actor); e != nil {
			return e
		}
		if status != "RUNNING" {
			return ErrStorageDenied
		}
		caller, _, e := o.authorize(ctx, tx, w, CapabilityAdmitReader, nil)
		if e != nil {
			return e
		}
		if actor != caller.ActorID {
			return ErrStorageDenied
		}
		var prior bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_reader_pin WHERE execution_id=$1)`, execution).Scan(&prior); e != nil {
			return e
		}
		if prior {
			return domain.ErrTerminationUnknown
		}
		rows, e := tx.Query(ctx, `SELECT DISTINCT dataset_version_id FROM execution_input WHERE execution_id=$1 ORDER BY dataset_version_id`, execution)
		if e != nil {
			return e
		}
		var versions []uuid.UUID
		for rows.Next() {
			var v uuid.UUID
			if e = rows.Scan(&v); e != nil {
				rows.Close()
				return e
			}
			versions = append(versions, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(versions) == 0 || len(versions) > 16 {
			return ErrStorageScope
		}
		ids := []uuid.UUID{}
		for _, v := range versions {
			id, ok := s.selection[v]
			if !ok {
				return ErrStorageScope
			}
			ids = append(ids, id)
		}
		if _, e = o.repo.LockFixtureCopies(ctx, tx, w, ids); e != nil {
			return e
		}
		for _, v := range versions {
			pin, copy, e := s.admit(ctx, tx, w, v, execution, domain.FixtureProcessing, nil)
			if e != nil {
				return e
			}
			scope.inputs[v] = &ownedInput{pin: pin, copy: copy, stopped: true}
		}
		return nil
	})
	if e != nil {
		return e
	}
	runCtx := context.WithValue(ctx, processingScopeKey{}, scope)
	var until *time.Time
	for _, input := range scope.inputs {
		if input.pin.UseUntil != nil && (until == nil || input.pin.UseUntil.Before(*until)) {
			until = input.pin.UseUntil
		}
	}
	var cancel context.CancelFunc
	if until != nil {
		runCtx, cancel = context.WithDeadline(runCtx, *until)
	} else {
		runCtx, cancel = context.WithCancel(runCtx)
	}
	defer cancel()
	runErr := errors.Join(execute(runCtx), runCtx.Err())
	scope.mu.Lock()
	known := !scope.outputUnknown
	for _, input := range scope.inputs {
		known = known && input.stopped && input.open == 0
	}
	scope.mu.Unlock()
	if !known {
		return errors.Join(runErr, domain.ErrTerminationUnknown)
	}
	outcome := domain.ReaderCompleted
	if runErr != nil || runCtx.Err() != nil {
		outcome = domain.ReaderCancelled
	}
	for _, input := range scope.inputs {
		proof := domain.ReaderTermination{ID: uuid.New(), WorkspaceID: w, PinID: input.pin.ID, ReaderID: execution, Generation: input.pin.Generation, Outcome: outcome, ObservedAt: time.Now().UTC()}
		s.reader.mu.Lock()
		s.reader.stops[input.pin.ID] = proof
		s.reader.mu.Unlock()
		stopCtx, stopCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, e = o.TerminateFixtureReader(stopCtx, TerminateFixtureReaderCommand{CommandHeader: CommandHeader{w, "processing-stop-" + input.pin.ID.String()}, PinID: input.pin.ID, Generation: input.pin.Generation, Outcome: outcome})
		stopCancel()
		if e != nil {
			return errors.Join(runErr, e)
		}
	}
	return runErr
}

type inputHandle struct {
	*bytes.Reader
	ctx   context.Context
	scope *processingScope
	input *ownedInput
	once  sync.Once
}

func (h *inputHandle) Read(b []byte) (int, error) {
	if e := h.ctx.Err(); e != nil {
		return 0, e
	}
	return h.Reader.Read(b)
}

func (h *inputHandle) Close() error {
	h.once.Do(func() { h.scope.mu.Lock(); h.input.open--; h.scope.mu.Unlock() })
	return nil
}

func (s *FixtureSubjectPins) ReadVersion(ctx context.Context, v uuid.UUID) (io.ReadCloser, error) {
	scope, ok := ctx.Value(processingScopeKey{}).(*processingScope)
	if !ok || scope.owner != s {
		return nil, ErrStorageDenied
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	input, ok := scope.inputs[v]
	if !ok {
		return nil, ErrStorageScope
	}
	if input.err != nil {
		return nil, input.err
	}
	if !input.read {
		input.read = true
		input.stopped = false
		r, e := s.reader.port.ReadExactPinned(ctx, input.copy, input.pin)
		if r.PinID != input.pin.ID || r.Copy != input.copy || r.StoppedAt.IsZero() {
			input.err = errors.Join(e, domain.ErrTerminationUnknown)
			return nil, input.err
		}
		input.stopped = true
		if e != nil {
			input.err = e
			return nil, e
		}
		if e = validatePinnedRead(input.copy, input.pin, r); e != nil {
			input.err = e
			return nil, e
		}
		input.bytes = r.Bytes
	}
	input.open++
	return &inputHandle{Reader: bytes.NewReader(input.bytes), ctx: ctx, scope: scope, input: input}, nil
}

// A copy fetched only once may still not be reopened by a cancelled owner.
func (s *FixtureSubjectPins) MarkProcessingOutputUnknown(ctx context.Context) {
	if scope, ok := ctx.Value(processingScopeKey{}).(*processingScope); ok && scope.owner == s {
		scope.mu.Lock()
		scope.outputUnknown = true
		scope.mu.Unlock()
	}
}

func validateCurrentReaderSubject(ctx context.Context, tx pgx.Tx, p domain.ReaderPin) error {
	var valid bool
	if p.ExecutionID != nil {
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM execution e WHERE e.id=$1 AND e.workspace_id=$2 AND e.status='RUNNING' AND e.created_by=$3)`, *p.ExecutionID, p.WorkspaceID, p.ActorID).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return ErrStorageDenied
		}
	}
	if p.DeliveryOperationID != nil {
		if e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM delivery_operation d WHERE d.id=$1 AND d.workspace_id=$2 AND d.status='ISSUED' AND d.principal_ref=$3 AND d.fresh_cap_expires_at>$4)`, *p.DeliveryOperationID, p.WorkspaceID, p.PrincipalRef, time.Now().UTC()).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return ErrStorageDenied
		}
	}
	return nil
}
