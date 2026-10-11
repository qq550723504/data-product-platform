package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

const (
	CapabilityRestrictUse      = "LIFECYCLE_RESTRICT_USE"
	CapabilityPlaceHold        = "LIFECYCLE_PLACE_HOLD"
	CapabilityReleaseHold      = "LIFECYCLE_RELEASE_HOLD"
	CapabilityAdmitReader      = "LIFECYCLE_ADMIT_READER"
	CapabilityTerminateReader  = "LIFECYCLE_TERMINATE_READER"
	CapabilityInspectOccupancy = "LIFECYCLE_INSPECT_OCCUPANCY"
	ContentRestrictedEvent     = "LifecycleContentUseRestricted"
	HoldPlacedEvent            = "LifecycleContentHoldPlaced"
	HoldReleasedEvent          = "LifecycleContentHoldReleased"
	ReaderAdmittedEvent        = "LifecycleReaderAdmitted"
	ReaderTerminatedEvent      = "LifecycleReaderTerminated"
)

// All three ports operate against current server-owned facts in the short
// transaction. Implementations must not perform external HTTP/storage I/O while
// row locks are held. No client field can mint authority or stopped-I/O proof.
type LifecycleAuthority interface {
	AuthorizeLifecycle(context.Context, pgx.Tx, uuid.UUID, string, time.Time) (principal.RequestPrincipal, error)
}
type UseGateRequest struct {
	Copy               domain.Copy
	ReaderID           uuid.UUID
	Caller             principal.RequestPrincipal
	AsOf               time.Time
	DependencyRevision int64
}
type CurrentUseGate interface {
	EvaluateFixtureUse(context.Context, pgx.Tx, UseGateRequest) (domain.UseDecision, error)
}
type ReaderTerminationVerifier interface {
	VerifyTermination(context.Context, pgx.Tx, domain.ReaderPin, string, time.Time) (domain.ReaderTermination, error)
}

type CommandHeader struct {
	WorkspaceID    uuid.UUID
	IdempotencyKey string
}
type RestrictContentUseCommand struct {
	CommandHeader
	DatasetVersionID uuid.UUID
	BasisRef         string
}
type PlaceContentHoldCommand struct {
	CommandHeader
	CopyID   uuid.UUID
	BasisRef string
}
type ReleaseContentHoldCommand struct {
	CommandHeader
	HoldID   uuid.UUID
	BasisRef string
}
type AdmitFixtureReaderCommand struct {
	CommandHeader
	CopyID   uuid.UUID
	ReaderID uuid.UUID
}
type TerminateFixtureReaderCommand struct {
	CommandHeader
	PinID      uuid.UUID
	Generation int64
	Outcome    string
}
type FactResult struct {
	ID       uuid.UUID
	Replayed bool
}
type ReaderAdmission struct {
	Pin      domain.ReaderPin
	Admitted bool
	Replayed bool
}
type PlanOccupancy struct {
	PlanID             uuid.UUID
	DependencyRevision int64
	Copies             []domain.Occupancy
}

type OccupancyService struct {
	tx          *transaction.Manager
	repo        *infrastructure.PostgresRepository
	authority   LifecycleAuthority
	useGate     CurrentUseGate
	termination ReaderTerminationVerifier
	now         func() time.Time
}

func NewOccupancyService(tx *transaction.Manager, repo *infrastructure.PostgresRepository, authority LifecycleAuthority, gate CurrentUseGate, termination ReaderTerminationVerifier) *OccupancyService {
	return &OccupancyService{tx: tx, repo: repo, authority: authority, useGate: gate, termination: termination, now: func() time.Time { return time.Now().UTC() }}
}

func validText(s string, max int) bool {
	return s != "" && len(s) <= max && utf8.ValidString(s) && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\x00\r\n")
}
func (s *OccupancyService) clock() time.Time { return s.now().UTC().Truncate(time.Microsecond) }
func (s *OccupancyService) configured() bool {
	return s != nil && s.tx != nil && s.repo != nil && s.authority != nil && s.now != nil
}

func (s *OccupancyService) authorize(ctx context.Context, tx pgx.Tx, w uuid.UUID, capability string, expected *principal.RequestPrincipal) (principal.RequestPrincipal, time.Time, error) {
	at := s.clock()
	p, err := s.authority.AuthorizeLifecycle(ctx, tx, w, capability, at)
	if err != nil {
		return p, at, err
	}
	if p.WorkspaceID != w || p.ActorID == uuid.Nil || !validText(p.Subject, 255) || !p.HasCapability(capability) ||
		(expected != nil && (p.ActorID != expected.ActorID || p.Subject != expected.Subject)) {
		return p, at, principal.ErrCapabilityDenied
	}
	return p, at, nil
}

// Fresh authorization occurs before replay lookup and again after acquiring all
// content locks for a new mutation. Replays do not start another reader or I/O.
func (s *OccupancyService) command(ctx context.Context, h CommandHeader, kind, capability string, body any,
	fn func(context.Context, pgx.Tx, principal.RequestPrincipal, int64) (uuid.UUID, error)) (FactResult, error) {
	var result FactResult
	if !s.configured() {
		return result, errors.New("lifecycle occupancy service not configured")
	}
	if h.WorkspaceID == uuid.Nil || !validText(h.IdempotencyKey, 255) {
		return result, domain.ErrInvalidPlan
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		revision, err := deliveryfence.Lock(ctx, tx, h.WorkspaceID)
		if err != nil {
			return err
		}
		caller, _, err := s.authorize(ctx, tx, h.WorkspaceID, capability, nil)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(struct {
			Workspace uuid.UUID
			Command   string
			Actor     uuid.UUID
			Principal string
			Body      any
		}{h.WorkspaceID, kind, caller.ActorID, caller.Subject, body})
		if err != nil {
			return err
		}
		hash := sha256.Sum256(encoded)
		fingerprint := hex.EncodeToString(hash[:])
		id, prior, found, err := s.repo.FindCommandReplay(ctx, tx, h.WorkspaceID, kind, h.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if prior != fingerprint {
				return ErrIdempotencyConflict
			}
			result = FactResult{ID: id, Replayed: true}
			return nil
		}
		id, err = fn(ctx, tx, caller, revision)
		if err != nil {
			return err
		}
		if err := s.repo.RecordCommandIdempotency(ctx, tx, h.WorkspaceID, kind, h.IdempotencyKey, fingerprint, id); err != nil {
			return err
		}
		result.ID = id
		return nil
	})
	if err != nil {
		return FactResult{}, err
	}
	return result, nil
}

func (s *OccupancyService) RestrictContentUse(ctx context.Context, cmd RestrictContentUseCommand) (FactResult, error) {
	if cmd.DatasetVersionID == uuid.Nil || !validText(cmd.BasisRef, 255) {
		return FactResult{}, domain.ErrInvalidPlan
	}
	return s.command(ctx, cmd.CommandHeader, "LIFECYCLE.RESTRICT_USE", CapabilityRestrictUse, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		ids, err := s.repo.FixtureCopyIDsForVersion(ctx, tx, cmd.WorkspaceID, cmd.DatasetVersionID)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := s.repo.LockFixtureCopies(ctx, tx, cmd.WorkspaceID, ids); err != nil {
			return uuid.Nil, err
		}
		caller, at, err := s.authorize(ctx, tx, cmd.WorkspaceID, CapabilityRestrictUse, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		id := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_content_use_restriction(id,workspace_id,dataset_version_id,basis_ref,actor_id,principal_ref,dependency_revision,occurred_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, cmd.WorkspaceID, cmd.DatasetVersionID, cmd.BasisRef, caller.ActorID, caller.Subject, revision, at); err != nil {
			return uuid.Nil, err
		}
		return id, appendOccupancyFacts(ctx, tx, ContentRestrictedEvent, "LIFECYCLE_RESTRICTION", id, caller, revision, at, map[string]any{"dataset_version_id": cmd.DatasetVersionID})
	})
}

func (s *OccupancyService) PlaceContentHold(ctx context.Context, cmd PlaceContentHoldCommand) (FactResult, error) {
	if cmd.CopyID == uuid.Nil || !validText(cmd.BasisRef, 255) {
		return FactResult{}, domain.ErrInvalidPlan
	}
	return s.command(ctx, cmd.CommandHeader, "LIFECYCLE.PLACE_HOLD", CapabilityPlaceHold, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		if _, err := s.repo.LockFixtureCopies(ctx, tx, cmd.WorkspaceID, []uuid.UUID{cmd.CopyID}); err != nil {
			return uuid.Nil, err
		}
		caller, at, err := s.authorize(ctx, tx, cmd.WorkspaceID, CapabilityPlaceHold, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		id := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_content_hold(id,workspace_id,copy_id,basis_ref,actor_id,principal_ref,dependency_revision,occurred_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, cmd.WorkspaceID, cmd.CopyID, cmd.BasisRef, caller.ActorID, caller.Subject, revision, at); err != nil {
			return uuid.Nil, err
		}
		return id, appendOccupancyFacts(ctx, tx, HoldPlacedEvent, "LIFECYCLE_HOLD", id, caller, revision, at, map[string]any{"copy_id": cmd.CopyID})
	})
}

func (s *OccupancyService) ReleaseContentHold(ctx context.Context, cmd ReleaseContentHoldCommand) (FactResult, error) {
	if cmd.HoldID == uuid.Nil || !validText(cmd.BasisRef, 255) {
		return FactResult{}, domain.ErrInvalidPlan
	}
	return s.command(ctx, cmd.CommandHeader, "LIFECYCLE.RELEASE_HOLD", CapabilityReleaseHold, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		copyID, placedAt, err := s.repo.HoldCopy(ctx, tx, cmd.WorkspaceID, cmd.HoldID)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := s.repo.LockFixtureCopies(ctx, tx, cmd.WorkspaceID, []uuid.UUID{copyID}); err != nil {
			return uuid.Nil, err
		}
		caller, at, err := s.authorize(ctx, tx, cmd.WorkspaceID, CapabilityReleaseHold, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		if at.Before(placedAt) {
			return uuid.Nil, domain.ErrInvalidPlan
		}
		released, err := s.repo.HoldReleased(ctx, tx, cmd.WorkspaceID, cmd.HoldID)
		if err != nil {
			return uuid.Nil, err
		}
		if released {
			return uuid.Nil, infrastructure.ErrAlreadyReleased
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		id := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_content_hold_release(id,workspace_id,hold_id,basis_ref,actor_id,principal_ref,dependency_revision,occurred_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, cmd.WorkspaceID, cmd.HoldID, cmd.BasisRef, caller.ActorID, caller.Subject, revision, at); err != nil {
			return uuid.Nil, err
		}
		return id, appendOccupancyFacts(ctx, tx, HoldReleasedEvent, "LIFECYCLE_HOLD_RELEASE", id, caller, revision, at, map[string]any{"hold_id": cmd.HoldID, "copy_id": copyID})
	})
}

// An admitted pin is returned only after COMMIT. This method never reads bytes.
// A same-key replay is historical/non-payload; taking data again needs a new key
// and a fresh gate. The existing production processing/delivery paths are not
// enabled for these fixtures by this service.
func (s *OccupancyService) AdmitFixtureReader(ctx context.Context, cmd AdmitFixtureReaderCommand) (ReaderAdmission, error) {
	var pin domain.ReaderPin
	if s == nil || s.useGate == nil {
		return ReaderAdmission{}, errors.New("current fixture use gate not configured")
	}
	if cmd.CopyID == uuid.Nil || cmd.ReaderID == uuid.Nil {
		return ReaderAdmission{}, domain.ErrInvalidPlan
	}
	result, err := s.command(ctx, cmd.CommandHeader, "LIFECYCLE.ADMIT_READER", CapabilityAdmitReader, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, gateRevision int64) (uuid.UUID, error) {
		copies, err := s.repo.LockFixtureCopies(ctx, tx, cmd.WorkspaceID, []uuid.UUID{cmd.CopyID})
		if err != nil {
			return uuid.Nil, err
		}
		copy := copies[0]
		caller, at, err := s.authorize(ctx, tx, cmd.WorkspaceID, CapabilityAdmitReader, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		restricted, err := s.repo.IsUseRestricted(ctx, tx, cmd.WorkspaceID, copy.DatasetVersionID)
		if err != nil {
			return uuid.Nil, err
		}
		if restricted {
			return uuid.Nil, domain.ErrUseBlocked
		}
		decision, err := s.useGate.EvaluateFixtureUse(ctx, tx, UseGateRequest{Copy: copy, ReaderID: cmd.ReaderID, Caller: caller, AsOf: at, DependencyRevision: gateRevision})
		if err != nil {
			return uuid.Nil, err
		}
		if err := decision.ValidateFor(cmd.WorkspaceID, cmd.CopyID, cmd.ReaderID, caller.ActorID, at); err != nil {
			return uuid.Nil, err
		}
		generation, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		pin = domain.ReaderPin{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, CopyID: cmd.CopyID, ReaderID: cmd.ReaderID, Kind: domain.FixtureReader,
			ActorID: caller.ActorID, PrincipalRef: caller.Subject, Generation: generation, GateRevision: gateRevision, StartedAt: at, WindowMode: decision.WindowMode, UseUntil: decision.UseUntil}
		if err := s.repo.InsertReaderPin(ctx, tx, pin); err != nil {
			return uuid.Nil, err
		}
		return pin.ID, appendOccupancyFacts(ctx, tx, ReaderAdmittedEvent, "LIFECYCLE_READER", pin.ID, caller, generation, at, map[string]any{"copy_id": pin.CopyID, "reader_id": pin.ReaderID, "generation": pin.Generation})
	})
	if err != nil {
		return ReaderAdmission{}, err
	}
	if result.Replayed {
		err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			pin, err = s.repo.GetReaderPin(ctx, tx, cmd.WorkspaceID, result.ID)
			return err
		})
		if err != nil {
			return ReaderAdmission{}, err
		}
	}
	return ReaderAdmission{Pin: pin, Admitted: !result.Replayed, Replayed: result.Replayed}, nil
}

func (s *OccupancyService) TerminateFixtureReader(ctx context.Context, cmd TerminateFixtureReaderCommand) (FactResult, error) {
	if s == nil || s.termination == nil {
		return FactResult{}, errors.New("reader termination verifier not configured")
	}
	if cmd.PinID == uuid.Nil || cmd.Generation <= 0 || (cmd.Outcome != domain.ReaderCompleted && cmd.Outcome != domain.ReaderCancelled) {
		return FactResult{}, domain.ErrTerminationUnknown
	}
	return s.command(ctx, cmd.CommandHeader, "LIFECYCLE.TERMINATE_READER", CapabilityTerminateReader, cmd, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		pin, err := s.repo.GetReaderPin(ctx, tx, cmd.WorkspaceID, cmd.PinID)
		if err != nil {
			return uuid.Nil, err
		}
		if pin.Generation != cmd.Generation {
			return uuid.Nil, domain.ErrTerminationUnknown
		}
		if _, err := s.repo.LockFixtureCopies(ctx, tx, cmd.WorkspaceID, []uuid.UUID{pin.CopyID}); err != nil {
			return uuid.Nil, err
		}
		caller, at, err := s.authorize(ctx, tx, cmd.WorkspaceID, CapabilityTerminateReader, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		terminated, err := s.repo.ReaderTerminated(ctx, tx, cmd.WorkspaceID, pin.ID)
		if err != nil {
			return uuid.Nil, err
		}
		if terminated {
			return uuid.Nil, infrastructure.ErrAlreadyReleased
		}
		proof, err := s.termination.VerifyTermination(ctx, tx, pin, cmd.Outcome, at)
		if err != nil {
			return uuid.Nil, err
		}
		if err := proof.ValidateFor(pin, cmd.Outcome, at); err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO lifecycle_reader_termination(id,workspace_id,pin_id,reader_id,generation,outcome,observed_at,actor_id,principal_ref,dependency_revision)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, proof.ID, proof.WorkspaceID, proof.PinID, proof.ReaderID, proof.Generation, proof.Outcome, proof.ObservedAt, caller.ActorID, caller.Subject, revision); err != nil {
			return uuid.Nil, err
		}
		return proof.ID, appendOccupancyFacts(ctx, tx, ReaderTerminatedEvent, "LIFECYCLE_READER_TERMINATION", proof.ID, caller, revision, at, map[string]any{"pin_id": pin.ID, "generation": pin.Generation, "outcome": proof.Outcome})
	})
}

func (s *OccupancyService) InspectPlanOccupancy(ctx context.Context, w, planID uuid.UUID) (PlanOccupancy, error) {
	var result PlanOccupancy
	if !s.configured() || w == uuid.Nil || planID == uuid.Nil {
		return result, domain.ErrInvalidPlan
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		revision, err := deliveryfence.Lock(ctx, tx, w)
		if err != nil {
			return err
		}
		caller, _, err := s.authorize(ctx, tx, w, CapabilityInspectOccupancy, nil)
		if err != nil {
			return err
		}
		p, err := s.repo.GetPlan(ctx, tx, w, planID, false)
		if err != nil {
			return err
		}
		if err := p.VerifyFrozenScope(); err != nil {
			return err
		}
		var ids []uuid.UUID
		for _, c := range p.Targets {
			ids = append(ids, c.ID)
		}
		copies, err := s.repo.LockFixtureCopies(ctx, tx, w, ids)
		if err != nil {
			return err
		}
		if _, _, err := s.authorize(ctx, tx, w, CapabilityInspectOccupancy, &caller); err != nil {
			return err
		}
		if _, err := s.repo.GetPlan(ctx, tx, w, planID, true); err != nil {
			return err
		}
		result.PlanID, result.DependencyRevision = planID, revision
		for _, c := range copies {
			o, err := s.repo.CopyOccupancy(ctx, tx, c)
			if err != nil {
				return err
			}
			result.Copies = append(result.Copies, o)
		}
		return nil
	})
	if err != nil {
		return PlanOccupancy{}, err
	}
	return result, nil
}

func appendOccupancyFacts(ctx context.Context, tx pgx.Tx, eventType, subject string, id uuid.UUID, caller principal.RequestPrincipal, revision int64, at time.Time, fields map[string]any) error {
	fields["fact_id"], fields["workspace_id"], fields["dependency_revision"] = id, caller.WorkspaceID, revision
	event, err := outbox.NewEvent(subject, id, eventType, fields)
	if err != nil {
		return err
	}
	event.AvailableAt = at
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{WorkspaceID: &caller.WorkspaceID, ActorType: "USER", ActorID: &caller.ActorID, Action: eventType, ObjectType: subject, ObjectID: id, Metadata: fields, OccurredAt: at}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: caller.WorkspaceID, EvidenceType: "LIFECYCLE_OCCUPANCY", Title: eventType,
		SourceType: subject, SourceID: &id, Metadata: fields, CreatedAt: at, CreatedBy: &caller.ActorID}, evidence.Relation{ObjectType: subject, ObjectID: id, RelationType: "CURRENT_CONTROL_FACT"})
	return err
}
