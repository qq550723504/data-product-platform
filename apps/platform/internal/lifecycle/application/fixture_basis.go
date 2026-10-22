package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type FixtureDisposalBasisCommand struct {
	CommandHeader
	RunID, InventoryPlanID, ApprovedPlanID uuid.UUID
	InventoryHash, ApprovedHash, BasisRef  string
}

// Closed controlled synthetic inputs have an explicit test-only zero minimum
// retention policy, starting at this command's event. Do not infer this policy
// for business datasets, statutory retention or a general-purpose delete API.
func (s *ExactExecution) RegisterFixtureDisposalBasis(ctx context.Context, c FixtureDisposalBasisCommand) (FactResult, error) {
	if s == nil || s.disposition == nil || !s.disposition.configured() || c.RunID == uuid.Nil || !validText(c.BasisRef, 255) {
		return FactResult{}, ErrStorageScope
	}
	d := s.disposition
	o := d.occupancy
	return o.command(ctx, c.CommandHeader, "LIFECYCLE.REGISTER_FIXTURE_BASIS", CapabilityApproveDisposition, c, func(ctx context.Context, tx pgx.Tx, caller principal.RequestPrincipal, _ int64) (uuid.UUID, error) {
		// Acquire the full inventory's parents first; never expand locks after taking
		// an operation/child lock. The approved subset is validated while held.
		inventory, err := d.lockedPlan(ctx, tx, c.WorkspaceID, c.InventoryPlanID)
		if err != nil {
			return uuid.Nil, err
		}
		approved, err := o.repo.GetPlan(ctx, tx, c.WorkspaceID, c.ApprovedPlanID, true)
		if err != nil {
			return uuid.Nil, err
		}
		if inventory.VerifyFrozenScope() != nil || approved.VerifyFrozenScope() != nil || inventory.ContentHash != c.InventoryHash || approved.ContentHash != c.ApprovedHash || len(approved.Targets) > len(inventory.Targets) {
			return uuid.Nil, ErrStorageScope
		}
		for _, target := range approved.Targets {
			found := false
			for _, all := range inventory.Targets {
				if target == all {
					found = true
				}
			}
			if !found {
				return uuid.Nil, ErrStorageScope
			}
		}
		var copies, namespaces int
		err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM lifecycle_fixture_copy WHERE workspace_id=$1),(SELECT count(*) FROM lifecycle_fixture_namespace WHERE workspace_id=$1)`, c.WorkspaceID).Scan(&copies, &namespaces)
		if err != nil {
			return uuid.Nil, err
		}
		if copies != len(inventory.Targets) || namespaces != 2 {
			return uuid.Nil, ErrStorageScope
		}
		// Cardinality alone cannot establish closure. Compare every registered
		// immutable identity while holding the workspace fence and inventory parents.
		ids := make([]uuid.UUID, 0, len(inventory.Targets))
		for _, target := range inventory.Targets {
			ids = append(ids, target.ID)
		}
		registered, err := o.repo.LoadFixtureCopies(ctx, tx, c.WorkspaceID, ids)
		if err != nil || len(registered) != copies {
			return uuid.Nil, ErrStorageScope
		}
		for _, target := range registered {
			found := false
			for _, member := range inventory.Targets {
				if target == member {
					found = true
				}
			}
			if !found {
				return uuid.Nil, ErrStorageScope
			}
		}
		caller, at, err := o.authorize(ctx, tx, c.WorkspaceID, CapabilityApproveDisposition, &caller)
		if err != nil {
			return uuid.Nil, err
		}
		revision, err := deliveryfence.Advance(ctx, tx, c.WorkspaceID)
		if err != nil {
			return uuid.Nil, err
		}
		id := uuid.New()
		err = appendDispositionFacts(ctx, tx, "LifecycleFixtureDisposalBasisRegistered", "LIFECYCLE_FIXTURE_BASIS", id, caller, revision, at, map[string]any{"run_id": c.RunID, "inventory_plan_id": inventory.ID, "approved_plan_id": approved.ID, "policy": "CONTROLLED_SYNTHETIC_TEST_ONLY_ZERO_MINIMUM_RETENTION", "basis_ref": c.BasisRef})
		if err != nil {
			return uuid.Nil, err
		}
		var eventID uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id FROM outbox_event WHERE aggregate_id=$1 AND event_type='LifecycleFixtureDisposalBasisRegistered'`, id).Scan(&eventID)
		if err != nil {
			return uuid.Nil, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_fixture_disposal_basis(id,workspace_id,run_id,inventory_plan_id,approved_plan_id,inventory_hash,approved_hash,basis_ref,starts_at,retain_until,dispose_due_at,start_event_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$9,$9,$10)`, id, c.WorkspaceID, c.RunID, inventory.ID, approved.ID, inventory.ContentHash, approved.ContentHash, c.BasisRef, at, eventID)
		return id, err
	})
}
