package pilot

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// One fresh native cache per dispatch/claim. The eight case plans each have a
// distinct disposal-basis run; lifecycle_fixture_disposal_basis is one-plan/run.
type RemainingDependencies struct {
	Execution   uuid.UUID
	All         []domain.Copy
	ValidUntil  time.Time
	Protection  map[uuid.UUID]lc.CopyProtection
	Inventories map[string]lc.NamespaceInventory
}

func RemainingBasisRun(execution, plan uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(execution, []byte(plan.String()))
}
func (d *RemainingDependencies) EvaluateDisposalDependencies(ctx context.Context, tx pgx.Tx, r lc.DisposalDependencyRequest) (lc.DisposalDependencies, error) {
	v := lc.DisposalDependencies{WorkspaceID: r.Copy.WorkspaceID, PlanID: r.Plan.ID, CopyID: r.Copy.ID, PlanHash: r.Plan.ContentHash, ActorID: r.Caller.ActorID, CheckedAt: r.AsOf, DependencyRevision: r.DependencyRevision, InventoryComplete: domain.Unknown, ReferencesReleased: domain.Unknown, ProviderRetentionAllows: domain.Unknown, LedgerCurrent: domain.Unknown, WindowMode: domain.UseDeadline}
	var policy, event uuid.UUID
	var start, retain, due time.Time
	err := tx.QueryRow(ctx, `SELECT id,start_event_id,starts_at,retain_until,dispose_due_at FROM lifecycle_fixture_disposal_basis WHERE workspace_id=$1 AND run_id=$2 AND approved_plan_id=$3 AND approved_hash=$4`, r.Copy.WorkspaceID, RemainingBasisRun(d.Execution, r.Plan.ID), r.Plan.ID, r.Plan.ContentHash).Scan(&policy, &event, &start, &retain, &due)
	if err != nil {
		return v, err
	}
	v.Retention = []domain.RetentionBoundary{{PolicyID: policy, StartEventID: event, StartsAt: start, RetainUntil: retain}}
	v.DisposalDueAt = due
	inv, ok := d.Inventories[r.Copy.Namespace]
	p, pok := d.Protection[r.Copy.ID]
	until := d.ValidUntil
	// Protection has independent authority. A current native retention denial
	// must survive an unknown inventory, e.g. M1-C before any lookup of UNKNOWN B.
	if pok && p.Copy == r.Copy {
		if !p.LegalHold && (p.RetentionMode == "NONE" || (p.RetainUntil != nil && !r.AsOf.Before(*p.RetainUntil))) {
			v.ProviderRetentionAllows = domain.Satisfied
		} else {
			v.ProviderRetentionAllows = domain.Denied
		}
		if p.ObservedAt.After(r.AsOf) {
			return v, lc.ErrStorageContract
		}
		if cap := p.ObservedAt.Add(time.Minute); cap.Before(until) {
			until = cap
		}
	}
	if ok && pok {
		expected := map[string]domain.Copy{}
		for _, cp := range d.All {
			if cp.Namespace != r.Copy.Namespace {
				continue
			}
			var absent bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE workspace_id=$1 AND copy_id=$2)`, cp.WorkspaceID, cp.ID).Scan(&absent); err != nil {
				return v, err
			}
			if !absent {
				expected[cp.Key+"\x00"+cp.ProviderVersion] = cp
			}
		}
		complete := len(inv.Versions) == len(expected)
		for _, x := range inv.Versions {
			cp, found := expected[x.Key+"\x00"+x.Version]
			if !found || x.DeleteMarker || x.ByteSize != cp.ByteSize {
				complete = false
			}
			delete(expected, x.Key+"\x00"+x.Version)
		}
		if complete && len(expected) == 0 && inv.WorkspaceID == r.Copy.WorkspaceID && inv.ProviderInstanceID == r.Copy.ProviderInstanceID && inv.IncarnationID == r.Copy.IncarnationID {
			v.InventoryComplete = domain.Satisfied
		}
		if p.Copy == r.Copy && !p.LegalHold && (p.RetentionMode == "NONE" || (p.RetainUntil != nil && !r.AsOf.Before(*p.RetainUntil))) {
			v.ProviderRetentionAllows = domain.Satisfied
		} else {
			v.ProviderRetentionAllows = domain.Denied
		}
		for _, at := range []time.Time{p.ObservedAt, inv.ObservedAt} {
			if at.After(r.AsOf) {
				return v, lc.ErrStorageContract
			}
			if cap := at.Add(time.Minute); cap.Before(until) {
				until = cap
			}
		}
		ids := append([]uuid.UUID{inv.CallID}, p.CallIDs...)
		var n int
		if err = tx.QueryRow(ctx, `SELECT count(DISTINCT c.id) FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.workspace_id=$1 AND c.id=ANY($2) AND o.invoked_at IS NOT NULL AND o.failure='' AND o.http_status IN(200,400)`, r.Copy.WorkspaceID, ids).Scan(&n); err != nil {
			return v, err
		}
		if n == 3 {
			v.LedgerCurrent = domain.Satisfied
		}
	}
	v.ValidUntil = &until
	var registered int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_fixture_copy WHERE workspace_id=$1`, r.Copy.WorkspaceID).Scan(&registered); err != nil {
		return v, err
	}
	if registered == len(d.All) {
		v.ReferencesReleased, _, err = lc.InspectFixtureReferences(ctx, tx, r.Copy)
	}
	return v, err
}
