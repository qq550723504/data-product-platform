package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/contentreference"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"strings"
)

const ReferenceDeclaredEvent = "LifecycleFixtureReferenceRequired"
const ReferenceReleasedEvent = "LifecycleFixtureReferenceReleased"

func ReferenceOwnerCapability(module string) string {
	return "FIXTURE_REFERENCE_OWNER_" + strings.ToUpper(module)
}

type DeclareFixtureReferenceCommand struct {
	CommandHeader
	CopyID    uuid.UUID
	Reference contentreference.Reference
	BasisRef  string
}
type ReleaseFixtureReferenceCommand struct {
	CommandHeader
	RequirementID              uuid.UUID
	Reference                  contentreference.Reference
	CopyID                     uuid.UUID
	BasisRef, AcknowledgedLoss string
	// A current trusted owner explicitly verifies all untracked/cached/background
	// work is stopped. This is never inferred from a terminal state or lease.
	OwnerVerifiedQuiescent bool
}
type FixtureReferenceService struct {
	occupancy *OccupancyService
	owners    map[string]contentreference.Owner
}

func NewFixtureReferenceService(o *OccupancyService, owners ...contentreference.Owner) *FixtureReferenceService {
	s := &FixtureReferenceService{occupancy: o, owners: map[string]contentreference.Owner{}}
	for _, owner := range owners {
		if owner == nil || s.owners[owner.Module()] != nil {
			return nil
		}
		s.owners[owner.Module()] = owner
	}
	return s
}
func (s *FixtureReferenceService) owner(ref contentreference.Reference) (contentreference.Owner, contentreference.Spec, error) {
	if s == nil || ref.Validate() != nil {
		return nil, contentreference.Spec{}, contentreference.ErrReference
	}
	spec, ok := contentreference.Lookup(ref.Kind)
	owner := s.owners[spec.Module]
	if !ok || owner == nil {
		return nil, spec, contentreference.ErrReference
	}
	return owner, spec, nil
}
func (s *FixtureReferenceService) DeclareFixtureReference(ctx context.Context, c DeclareFixtureReferenceCommand) (FactResult, error) {
	owner, spec, err := s.owner(c.Reference)
	if err != nil || c.CopyID == uuid.Nil || !validText(c.BasisRef, 255) {
		return FactResult{}, contentreference.ErrReference
	}
	o := s.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.DECLARE_FIXTURE_REFERENCE", ReferenceOwnerCapability(spec.Module), c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		copies, e := o.repo.LockFixtureCopies(ctx, tx, c.WorkspaceID, []uuid.UUID{c.CopyID})
		if e != nil {
			return uuid.Nil, e
		}
		copy := copies[0]
		checked, e := owner.Check(ctx, tx, c.WorkspaceID, copy.DatasetVersionID, c.Reference)
		if e != nil || checked != spec {
			return uuid.Nil, contentreference.ErrReference
		}
		caller, at, e := o.authorize(ctx, tx, c.WorkspaceID, ReferenceOwnerCapability(spec.Module), &caller)
		if e != nil {
			return uuid.Nil, e
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		id := uuid.New()
		columns := "id,workspace_id,copy_id,dataset_version_id,kind," + pgx.Identifier{spec.FactColumn}.Sanitize() + ",basis_ref,actor_id,principal_ref,dependency_revision,occurred_at"
		args := []any{id, c.WorkspaceID, c.CopyID, copy.DatasetVersionID, spec.Kind, c.Reference.SubjectID, c.BasisRef, caller.ActorID, caller.Subject, rev, at}
		values := "$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11"
		if c.Reference.Kind == "EXECUTION_INPUT" {
			columns += ",input_name"
			values += ",$12"
			args = append(args, c.Reference.InputName)
		}
		_, e = tx.Exec(ctx, "INSERT INTO lifecycle_fixture_reference_requirement("+columns+") VALUES("+values+")", args...)
		if e != nil {
			return uuid.Nil, e
		}
		return id, appendOccupancyFacts(ctx, tx, ReferenceDeclaredEvent, "LIFECYCLE_REFERENCE_REQUIREMENT", id, caller, rev, at, map[string]any{"copy_id": c.CopyID, "reference_kind": spec.Kind, "owner_module": spec.Module, "requirement": "MATERIAL"})
	})
}
func (s *FixtureReferenceService) ReleaseFixtureReference(ctx context.Context, c ReleaseFixtureReferenceCommand) (FactResult, error) {
	owner, spec, err := s.owner(c.Reference)
	if err != nil || c.RequirementID == uuid.Nil || c.CopyID == uuid.Nil || !validText(c.BasisRef, 255) || c.AcknowledgedLoss != spec.Loss || !c.OwnerVerifiedQuiescent {
		return FactResult{}, contentreference.ErrReference
	}
	o := s.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.RELEASE_FIXTURE_REFERENCE", ReferenceOwnerCapability(spec.Module), c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		copies, e := o.repo.LockFixtureCopies(ctx, tx, c.WorkspaceID, []uuid.UUID{c.CopyID})
		if e != nil {
			return uuid.Nil, e
		}
		copy := copies[0]
		var matches bool
		query := "SELECT EXISTS(SELECT 1 FROM lifecycle_fixture_reference_requirement r WHERE r.id=$1 AND r.workspace_id=$2 AND r.copy_id=$3 AND r.kind=$4 AND r." + pgx.Identifier{spec.FactColumn}.Sanitize() + "=$5"
		args := []any{c.RequirementID, c.WorkspaceID, c.CopyID, spec.Kind, c.Reference.SubjectID}
		if spec.Kind == "EXECUTION_INPUT" {
			query += " AND r.input_name=$6"
			args = append(args, c.Reference.InputName)
		}
		query += ")"
		if e = tx.QueryRow(ctx, query, args...).Scan(&matches); e != nil {
			return uuid.Nil, e
		}
		if !matches {
			return uuid.Nil, contentreference.ErrReference
		}
		checked, e := owner.Check(ctx, tx, c.WorkspaceID, copy.DatasetVersionID, c.Reference)
		if e != nil || checked != spec {
			return uuid.Nil, contentreference.ErrReference
		}
		caller, at, e := o.authorize(ctx, tx, c.WorkspaceID, ReferenceOwnerCapability(spec.Module), &caller)
		if e != nil {
			return uuid.Nil, e
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		id := uuid.New()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_fixture_reference_release(id,workspace_id,requirement_id,loss,basis_ref,actor_id,principal_ref,dependency_revision,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, c.WorkspaceID, c.RequirementID, spec.Loss, c.BasisRef, caller.ActorID, caller.Subject, rev, at)
		if e != nil {
			return uuid.Nil, e
		}
		return id, appendOccupancyFacts(ctx, tx, ReferenceReleasedEvent, "LIFECYCLE_REFERENCE_RELEASE", id, caller, rev, at, map[string]any{"requirement_id": c.RequirementID, "copy_id": c.CopyID, "owner_module": spec.Module, "lost_capability": spec.Loss, "owner_verified_quiescent": true})
	})
}
