package main

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

type inspectionConfig struct{ Endpoint, AccessKey, SecretKey string }

func closeV28Denied(ctx context.Context, c config, pool *pgxpool.Pool, o *lc.OccupancyService, s *lc.DispositionService, results *[]map[string]any) error {
	d := pilot.PostV28BusinessDispatches()[0]
	// Exact immutable DENIED, not an UNKNOWN or an assertion derived from a host
	// receipt. This SELECT cannot create observations or authorize another send.
	var valid, released, revoked bool
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_attempt a
 JOIN lifecycle_disposition_dispatch d ON d.id=a.dispatch_id
 JOIN lifecycle_disposition_approval p ON p.id=d.approval_id
 JOIN lifecycle_provider_send_claim cl ON cl.attempt_id=a.id
 JOIN lifecycle_provider_observation ob ON ob.attempt_id=a.id
 WHERE a.id=$1 AND a.dispatch_id=$2 AND a.plan_id=$3 AND a.copy_id=$4 AND a.workspace_id=$5
 AND d.plan_id=$3 AND d.copy_id=$4 AND d.workspace_id=$5 AND p.id=$6 AND p.plan_hash=$7
 AND p.basis_ref=$8 AND p.principal_ref=$9 AND ob.certainty='CONFIRMED' AND ob.outcome='DENIED'
 AND ob.invoked_at IS NOT NULL)
 AND NOT EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$1 AND (certainty<>'CONFIRMED' OR outcome<>'DENIED'))
 AND NOT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$4)
 AND NOT EXISTS(SELECT 1 FROM lifecycle_provider_attempt WHERE copy_id=$4 AND id NOT IN($1,$10,$11))
 AND NOT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch WHERE copy_id=$4 AND id NOT IN($2,$12,$13)),
 EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$2),
 EXISTS(SELECT 1 FROM lifecycle_approval_revocation WHERE approval_id=$6)`, d.Attempt, d.Dispatch, d.Plan, d.Copy, c.WorkspaceID, d.Approval, d.PlanHash, "REMAINING_NON_DELETE_REHEARSAL:"+pilot.PostV28Execution, "remaining-fixture:"+pilot.PostV28Execution, pilot.PostV22BusinessDispatches()[0].Attempt, pilot.PostV27BusinessDispatches()[0].Attempt, pilot.PostV22BusinessDispatches()[0].Dispatch, pilot.PostV27BusinessDispatches()[0].Dispatch).Scan(&valid, &released, &revoked)
	if err != nil {
		return err
	}
	if !valid {
		return lc.ErrStorageDenied
	}
	entry := map[string]any{"case": "P2", "attempt": d.Attempt, "preserved_original": "CONFIRMED DENIED", "old_approval_already_revoked": revoked}
	*results = append(*results, entry)
	if released {
		if !revoked {
			return lc.ErrStorageDenied
		}
		entry["already_released"] = true
		return nil
	}
	if !revoked {
		fact, e := s.RevokeDispositionApproval(ctx, lc.RevokeDispositionApprovalCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: c.AuditID.String() + ":P2:revoke"}, ApprovalID: d.Approval, BasisRef: "post-v28-closed:" + c.AuditID.String()})
		if e != nil {
			return e
		}
		entry["new_approval_revocation"] = fact
	}
	var ic inspectionConfig
	if err = read("/run/private/inspection.json", &ic); err != nil {
		return err
	}
	if ic.Endpoint != "127.0.0.1:9000" || ic.AccessKey == "" || ic.SecretKey == "" {
		return lc.ErrStorageScope
	}
	scope, err := pilot.RemainingScopeFor("business-races")
	if err != nil {
		return err
	}
	var targetFound bool
	for _, copy := range scope.Copies {
		if copy.ID != d.Copy {
			continue
		}
		targetFound = true
		inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: ic.Endpoint, AccessKey: ic.AccessKey, SecretKey: ic.SecretKey, WorkspaceID: scope.Workspace, ProviderInstanceID: scope.Provider, IncarnationID: scope.Incarnation, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.NewPostgresStorageCallJournal(o)})
		if e != nil {
			return e
		}
		presence, e := inspector.InspectExactCopy(ctx, copy)
		if e != nil {
			return e
		}
		// CallID/time come directly from the adapter's durable pre-I/O journal.
		if presence.CallID == uuid.Nil || time.Since(presence.ObservedAt) > time.Minute {
			return lc.ErrStorageContract
		}
		fact, e := lc.NewExactExecution(s).ReconcileDeniedAttempt(ctx, lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: c.AuditID.String() + ":P2:reconcile"}, d.Attempt, presence)
		if e != nil {
			return e
		}
		entry["exact_presence"] = presence
		entry["DENIED_release"] = fact
	}
	if !targetFound {
		return lc.ErrStorageScope
	}
	return nil
}
