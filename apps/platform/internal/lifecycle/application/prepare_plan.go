package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

const CapabilityPreparePlan = "LIFECYCLE_PREPARE_PLAN"
const PlanPreparedEvent = "LifecycleDispositionPlanPrepared"

var ErrIdempotencyConflict = errors.New("lifecycle plan idempotency conflict")

// PlanAuthority resolves the current trusted caller while the shared workspace
// fence is held, including on same-key replay. The command cannot supply actor
// identity. Preparing a scope grants neither approval nor execution authority.
type PlanAuthority interface {
	AuthorizePlan(context.Context, pgx.Tx, uuid.UUID) (principal.RequestPrincipal, error)
}

type PrepareDeletePlanCommand struct {
	WorkspaceID    uuid.UUID
	CopyIDs        []uuid.UUID
	IdempotencyKey string
}

type PlanService struct {
	tx        *transaction.Manager
	repo      *infrastructure.PostgresRepository
	authority PlanAuthority
	now       func() time.Time
}

func NewPlanService(tx *transaction.Manager, repo *infrastructure.PostgresRepository, authority PlanAuthority) *PlanService {
	return &PlanService{tx: tx, repo: repo, authority: authority, now: func() time.Time { return time.Now().UTC() }}
}

// PrepareDeletePlan is an atomic, non-destructive command. Inputs are registered
// fixture IDs; object keys, versions and endpoints cannot be supplied here.
// FINALIZED means scope frozen, never approved, dispatched, deleted or verified.
func (s *PlanService) PrepareDeletePlan(ctx context.Context, cmd PrepareDeletePlanCommand) (domain.Plan, error) {
	var result domain.Plan
	if s == nil || s.tx == nil || s.repo == nil || s.authority == nil || s.now == nil {
		return result, errors.New("lifecycle plan service not configured")
	}
	if cmd.WorkspaceID == uuid.Nil || len(cmd.CopyIDs) == 0 || len(cmd.CopyIDs) > domain.MaxPlanTargets ||
		cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > 255 || strings.TrimSpace(cmd.IdempotencyKey) != cmd.IdempotencyKey ||
		!utf8.ValidString(cmd.IdempotencyKey) || strings.ContainsAny(cmd.IdempotencyKey, "\x00\r\n") {
		return result, domain.ErrInvalidPlan
	}
	ids := append([]uuid.UUID(nil), cmd.CopyIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for i, id := range ids {
		if id == uuid.Nil || (i > 0 && ids[i-1] == id) {
			return result, domain.ErrInvalidPlan
		}
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// One existing fence serializes caller changes, idempotency and scope
		// creation. No I/O is performed while this transaction holds row locks.
		if _, err := deliveryfence.Lock(ctx, tx, cmd.WorkspaceID); err != nil {
			return err
		}
		caller, err := s.authority.AuthorizePlan(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return err
		}
		if caller.WorkspaceID != cmd.WorkspaceID || caller.ActorID == uuid.Nil || caller.Subject == "" ||
			len(caller.Subject) > 255 || !utf8.ValidString(caller.Subject) || strings.TrimSpace(caller.Subject) != caller.Subject ||
			strings.ContainsAny(caller.Subject, "\x00\r\n") || !caller.HasCapability(CapabilityPreparePlan) {
			return principal.ErrCapabilityDenied
		}
		fingerprint, err := planRequestFingerprint(cmd.WorkspaceID, ids, caller)
		if err != nil {
			return err
		}
		id, prior, found, err := s.repo.FindReplay(ctx, tx, cmd.WorkspaceID, cmd.IdempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if prior != fingerprint {
				return ErrIdempotencyConflict
			}
			result, err = s.repo.GetPlan(ctx, tx, cmd.WorkspaceID, id, false)
			if err != nil {
				return err
			}
			return result.VerifyFrozenScope()
		}
		copies, err := s.repo.LoadFixtureCopies(ctx, tx, cmd.WorkspaceID, ids)
		if err != nil {
			return err
		}
		if _, err = domain.ScopeHash(cmd.WorkspaceID, domain.DeleteExact, copies); err != nil {
			return err
		}
		revision, err := deliveryfence.Advance(ctx, tx, cmd.WorkspaceID)
		if err != nil {
			return err
		}
		at := s.now().UTC().Truncate(time.Microsecond)
		p := domain.Plan{ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, Action: domain.DeleteExact, Status: domain.Draft,
			PreparedBy: caller.ActorID, PrincipalRef: caller.Subject, DependencyRevision: revision, CreatedAt: at}
		if err := s.repo.InsertDraft(ctx, tx, p); err != nil {
			return err
		}
		for _, c := range copies {
			if err := s.repo.AddTarget(ctx, tx, cmd.WorkspaceID, p.ID, c.ID); err != nil {
				return err
			}
		}
		result, err = s.repo.Finalize(ctx, tx, cmd.WorkspaceID, p.ID, at)
		if err != nil {
			return err
		}
		if err := s.repo.RecordIdempotency(ctx, tx, result, cmd.IdempotencyKey, fingerprint); err != nil {
			return err
		}
		return appendPreparedFacts(ctx, tx, result)
	})
	if err != nil {
		return domain.Plan{}, err
	}
	return result, nil
}

func planRequestFingerprint(workspace uuid.UUID, ids []uuid.UUID, p principal.RequestPrincipal) (string, error) {
	encoded, err := json.Marshal(struct {
		Workspace uuid.UUID
		Action    string
		Copies    []uuid.UUID
		Actor     uuid.UUID
		Principal string
	}{workspace, domain.DeleteExact, ids, p.ActorID, p.Subject})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}

func appendPreparedFacts(ctx context.Context, tx pgx.Tx, p domain.Plan) error {
	// Deliberately allowlisted: no object URI/key, original bytes, provider
	// response, credentials or arbitrary command metadata in historical proof.
	payload := map[string]any{"plan_id": p.ID, "workspace_id": p.WorkspaceID, "action": p.Action,
		"scope_hash": p.ContentHash, "hash_algorithm": domain.PlanHashAlgorithm, "target_count": len(p.Targets),
		"dependency_revision": p.DependencyRevision}
	event, err := outbox.NewEvent("LIFECYCLE_PLAN", p.ID, PlanPreparedEvent, payload)
	if err != nil {
		return err
	}
	event.AvailableAt = p.CreatedAt
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if err := audit.Append(ctx, tx, audit.Event{WorkspaceID: &p.WorkspaceID, ActorType: "USER", ActorID: &p.PreparedBy,
		Action: PlanPreparedEvent, ObjectType: "LIFECYCLE_PLAN", ObjectID: p.ID, Metadata: payload, OccurredAt: p.CreatedAt}); err != nil {
		return err
	}
	_, err = evidence.Append(ctx, tx, evidence.Record{WorkspaceID: p.WorkspaceID, EvidenceType: "LIFECYCLE_PLAN",
		Title: "Synthetic disposition scope prepared", SourceType: "LIFECYCLE_PLAN", SourceID: &p.ID,
		Metadata: payload, CreatedAt: p.CreatedAt, CreatedBy: &p.PreparedBy},
		evidence.Relation{ObjectType: "LIFECYCLE_PLAN", ObjectID: p.ID, RelationType: "PREPARATION"})
	if err != nil {
		return fmt.Errorf("append lifecycle preparation evidence: %w", err)
	}
	return nil
}
