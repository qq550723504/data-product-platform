package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// Preparation cannot reach this path: it first requires a separately captured
// exact-list confirmation, pinned binary/profile/topology and <=40min authority.
func referenceSend(ctx context.Context, stage string) error {
	c, p, e := loadReferenceSend()
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return lc.ErrStorageScope
	}
	defer pool.Close()
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	manager := transaction.NewManager(pool)
	repo := &infrastructure.PostgresRepository{}
	all := pilot.ReferenceCopies()
	targets := pilot.ReferenceTargets()
	var target domain.Copy
	for _, copy := range targets {
		if (stage == "send-a" && copy.Namespace == "dpp-lifecycle307-online") || (stage == "send-b" && copy.Namespace == "dpp-lifecycle307-quarantine") {
			target = copy
		}
	}
	if target.ID == uuid.Nil {
		return lc.ErrStorageScope
	}
	// Old starts are historical facts. Never reuse them or unresolved reservations.
	var oldResolved, newAttempt, firstAbsent bool
	e = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM lifecycle_dispatch_release r JOIN lifecycle_provider_observation x ON x.id=r.observation_id WHERE r.dispatch_id IN('8b9b4d0c-df37-41b5-b3d1-0630deae8675','aa0dc82a-06b4-4b0d-a066-33d19ccc8f45') AND x.certainty='NOT_SENT' AND x.outcome='NOT_SENT')=2, EXISTS(SELECT 1 FROM lifecycle_provider_attempt a WHERE a.copy_id=$1 AND a.id NOT IN('0a86461d-d5c2-4c00-99ae-0d44ac6dde2a','d8a52b7b-9cad-4ab4-a8d5-286d37ceafb9')), EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE plan_id=$2 AND copy_id='8b1ff30c-3695-4847-b4cf-cc92cd6a399c')`, target.ID, planID).Scan(&oldResolved, &newAttempt, &firstAbsent)
	if e != nil {
		return e
	}
	if !oldResolved || newAttempt || (stage == "send-b" && !firstAbsent) || (stage == "send-a" && firstAbsent) {
		return errors.New("attempt/receipt ordering forbids send; read-only recovery required")
	}
	var approved domain.Plan
	e = manager.Do(ctx, func(ctx context.Context, t pgx.Tx) error {
		var e error
		approved, e = repo.GetPlan(ctx, t, workspace, planID, false)
		if e != nil {
			return e
		}
		full, e := repo.GetPlan(ctx, t, workspace, inventoryPlanID, false)
		if e != nil {
			return e
		}
		if approved.VerifyFrozenScope() != nil || full.VerifyFrozenScope() != nil || approved.ContentHash != planHash || full.ContentHash != inventoryHash || len(approved.Targets) != 2 || len(full.Targets) != 6 {
			return lc.ErrStorageScope
		}
		for _, c := range all {
			found := false
			for _, v := range full.Targets {
				found = found || c == v
			}
			if !found {
				return lc.ErrStorageScope
			}
		}
		for _, c := range targets {
			found := false
			for _, v := range approved.Targets {
				found = found || c == v
			}
			if !found {
				return lc.ErrStorageScope
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	auth := authority{}
	occupancy := lc.NewOccupancyService(manager, repo, auth, nil, nil)
	dep := &dependencies{all: all, protection: map[uuid.UUID]lc.CopyProtection{}, inventory: map[string]lc.NamespaceInventory{}}
	disposition := lc.NewDispositionService(occupancy, auth, dep, nil)
	execution := lc.NewExactExecution(disposition)
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: workspace, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.NewPostgresStorageCallJournal(occupancy)})
	if e != nil {
		return e
	}
	refresh := func() error {
		p, e := inspector.InspectRetentionAndHold(ctx, target)
		if e != nil {
			return e
		}
		i, e := inspector.InspectNamespace(ctx, target)
		if e != nil {
			return e
		}
		dep.protection[target.ID] = p
		dep.inventory[target.Namespace] = i
		return nil
	}
	header := func(key string) lc.CommandHeader {
		return lc.CommandHeader{WorkspaceID: workspace, IdempotencyKey: run + ":" + key}
	}
	if _, e = execution.RegisterFixtureDisposalBasis(ctx, lc.FixtureDisposalBasisCommand{CommandHeader: header("basis"), RunID: uuid.MustParse(run), InventoryPlanID: inventoryPlanID, ApprovedPlanID: planID, InventoryHash: inventoryHash, ApprovedHash: planHash, BasisRef: p.BasisRef}); e != nil {
		return e
	}
	approval, e := disposition.ApproveContentDisposition(ctx, lc.ApproveContentDispositionCommand{CommandHeader: header("approval"), PlanID: planID, BasisRef: p.BasisRef, ExpiresAt: p.ExpiresAt})
	if e != nil {
		return e
	}
	if e = refresh(); e != nil {
		return e
	}
	dispatch, e := disposition.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: header(stage + ":dispatch"), PlanID: planID, CopyID: target.ID, ApprovalID: approval.ID})
	if e != nil || dispatch.AttemptID == uuid.Nil {
		if e != nil {
			return e
		}
		return lc.ErrStorageDenied
	}
	var attempt domain.ProviderAttempt
	e = manager.Do(ctx, func(ctx context.Context, t pgx.Tx) error {
		var e error
		attempt, e = repo.GetAttempt(ctx, t, workspace, dispatch.AttemptID, false)
		return e
	})
	if e != nil {
		return e
	}
	// Refresh again; ClaimExactSend performs the final fenced current gate.
	if e = refresh(); e != nil {
		return e
	}
	deleter, e := storage.NewLifecycleDeleter(storage.LifecycleDeleteConfig{Endpoint: c.Endpoint, AccessKey: p.DeleteAccess, SecretKey: p.DeleteSecret, Targets: targets, Journal: execution})
	if e != nil {
		return e
	}
	sendErr := deleter.DeleteExactCopy(ctx, attempt, target)
	var workID uuid.UUID
	var observed bool
	if e = pool.QueryRow(ctx, `SELECT id,EXISTS(SELECT 1 FROM lifecycle_provider_observation WHERE attempt_id=$1 AND certainty='CONFIRMED') FROM lifecycle_reconciliation_work WHERE attempt_id=$1`, attempt.ID).Scan(&workID, &observed); e != nil {
		return e
	}
	if e = write(filepath.Join(c.OutputDirectory, stage+"-returned.json"), map[string]any{"copy": target, "attempt": attempt, "work_id": workID, "confirmed_invocation_observed": observed, "response_unknown": sendErr != nil, "recovery": "READ_ONLY_ONLY; no resend or next stage before exact absence", "completion": "NOT_CLAIMED"}); e != nil {
		return e
	}
	if !observed {
		if sendErr != nil {
			return sendErr
		}
		return lc.ErrStorageUnknown
	}
	// Separately authorized native acceptance loses the original executor only
	// AFTER a physical observation/cost commits, BEFORE any absence/completion.
	os.Exit(93)
	return nil
}
