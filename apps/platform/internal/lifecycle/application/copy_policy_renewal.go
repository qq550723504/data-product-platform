package application

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type RenewFixtureCopyPolicyCommand struct {
	CommandHeader
	DatasetVersionID, PreviousPolicyID uuid.UUID
	UseUntil, RestoreUntil             time.Time
}

// Renewal creates a new policy head. Historical copy operations/checkpoints
// retain their exact old policy identity/deadline. This is synthetic authority,
// not production rights or an override of restrictions, holds or live owners.
func (s *CopyService) RenewFixtureCopyPolicy(ctx context.Context, c RenewFixtureCopyPolicyCommand) (FactResult, error) {
	if s == nil || c.DatasetVersionID == uuid.Nil || c.PreviousPolicyID == uuid.Nil {
		return FactResult{}, ErrStorageScope
	}
	c.UseUntil = c.UseUntil.UTC().Truncate(time.Microsecond)
	c.RestoreUntil = c.RestoreUntil.UTC().Truncate(time.Microsecond)
	o := s.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.RENEW_COPY_POLICY", CapabilityCopyCustody, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		ids, e := o.repo.FixtureCopyIDsForVersion(ctx, tx, c.WorkspaceID, c.DatasetVersionID)
		if e != nil {
			return uuid.Nil, e
		}
		copies, e := o.repo.LockFixtureCopies(ctx, tx, c.WorkspaceID, ids)
		if e != nil {
			return uuid.Nil, e
		}
		if len(copies) == 0 {
			return uuid.Nil, ErrStorageScope
		}
		_, at, e := o.authorize(ctx, tx, c.WorkspaceID, CapabilityCopyCustody, &caller)
		if e != nil {
			return uuid.Nil, e
		}
		if !c.UseUntil.After(at) || !c.RestoreUntil.After(at) || c.UseUntil.After(at.Add(time.Hour)) || c.RestoreUntil.After(at.Add(time.Hour)) {
			return uuid.Nil, ErrStorageDenied
		}
		for _, cp := range copies {
			state, e := o.repo.CopyOccupancy(ctx, tx, cp)
			if e != nil {
				return uuid.Nil, e
			}
			if state.UseRestricted != domain.Denied || state.HoldFree != domain.Satisfied || state.ReadersDrained != domain.Satisfied || state.UnresolvedDispatches != 0 {
				return uuid.Nil, ErrStorageDenied
			}
		}
		var pendingOutput bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_processing_output_intent i WHERE i.workspace_id=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_processing_output_ready r WHERE r.intent_id=i.id) AND (i.dataset_version_id=$2 OR EXISTS(SELECT 1 FROM lifecycle_fixture_copy c WHERE c.id=i.source_copy_id AND c.dataset_version_id=$2) OR EXISTS(SELECT 1 FROM execution_input x WHERE x.execution_id=i.execution_id AND x.dataset_version_id=$2)))`, c.WorkspaceID, c.DatasetVersionID).Scan(&pendingOutput); e != nil {
			return uuid.Nil, e
		}
		if pendingOutput {
			return uuid.Nil, ErrStorageDenied
		}
		var previous uuid.UUID
		if e = tx.QueryRow(ctx, `SELECT id FROM lifecycle_current_copy_policy WHERE workspace_id=$1 AND dataset_version_id=$2 FOR UPDATE`, c.WorkspaceID, c.DatasetVersionID).Scan(&previous); e != nil {
			return uuid.Nil, e
		}
		if previous != c.PreviousPolicyID {
			return uuid.Nil, ErrStorageDenied
		}
		rev, e := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if e != nil {
			return uuid.Nil, e
		}
		id := uuid.New()
		_, e = tx.Exec(ctx, `INSERT INTO lifecycle_copy_policy(id,workspace_id,dataset_version_id,basis_ref,use_until,restore_until,actor_id,principal_ref,created_at,supersedes_policy_id) VALUES($1,$2,$3,'CONTROLLED_SYNTHETIC_CUSTODY_AND_USE',$4,$5,$6,$7,$8,$9)`, id, c.WorkspaceID, c.DatasetVersionID, c.UseUntil, c.RestoreUntil, caller.ActorID, caller.Subject, at, previous)
		if e != nil {
			return uuid.Nil, e
		}
		return id, appendDispositionFacts(ctx, tx, "LifecycleCopyPolicyRenewed", "LIFECYCLE_COPY_POLICY", id, caller, rev, at, map[string]any{"dataset_version_id": c.DatasetVersionID, "previous_policy_id": previous, "use_until": c.UseUntil, "restore_until": c.RestoreUntil})
	})
}
