package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type remainingDisposition struct {
	r         remainingConfig
	pool      *pgxpool.Pool
	o         *lc.OccupancyService
	service   *lc.DispositionService
	deps      *referenceDependencies
	inspector lc.StorageInspector
	plans     map[string]pilot.RemainingPlan
	approvals map[string]uuid.UUID
}

func newRemainingDisposition(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, cs []domain.Copy, o *lc.OccupancyService) (*remainingDisposition, error) {
	c := r.Base
	a := remainingAuthority{r}
	tx := transaction.NewManager(pool)
	ins, err := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.NewPostgresStorageCallJournal(o)})
	if err != nil {
		return nil, err
	}
	deps := &referenceDependencies{c: c, all: cs, protection: map[uuid.UUID]lc.CopyProtection{}, inventories: map[string]lc.NamespaceInventory{}}
	d := &remainingDisposition{r: r, pool: pool, o: o, deps: deps, inspector: ins, plans: map[string]pilot.RemainingPlan{}, approvals: map[string]uuid.UUID{}}
	d.service = lc.NewDispositionService(o, a, deps, nil)
	ids := []uuid.UUID{}
	for _, cp := range cs {
		ids = append(ids, cp.ID)
	}
	inventory, err := lc.NewPlanService(tx, &infrastructure.PostgresRepository{}, a).PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: ids, IdempotencyKey: "remaining-inventory:" + r.Batch.String()})
	if err != nil {
		return nil, err
	}
	scope, _ := pilot.RemainingScopeFor(r.Group)
	for _, p := range scope.Plans {
		d.plans[p.Case] = p
		if _, err = lc.NewExactExecution(d.service).RegisterFixtureDisposalBasis(ctx, lc.FixtureDisposalBasisCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "remaining-basis:" + p.Case}, RunID: c.RunID, InventoryPlanID: inventory.ID, ApprovedPlanID: p.ID, InventoryHash: inventory.ContentHash, ApprovedHash: p.Hash, BasisRef: "REMAINING_NON_DELETE_REHEARSAL:" + r.Batch.String()}); err != nil {
			return nil, err
		}
	}
	return d, nil
}
func (d *remainingDisposition) restrict(ctx context.Context, cp domain.Copy) error {
	_, err := d.o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: d.r.Base.WorkspaceID, IdempotencyKey: "remaining-restrict:" + cp.ID.String()}, DatasetVersionID: cp.DatasetVersionID, BasisRef: "REMAINING_NON_DELETE_REHEARSAL:" + d.r.Batch.String()})
	return err
}
func (d *remainingDisposition) approve(ctx context.Context, name string) error {
	p := d.plans[name]
	a, err := d.service.ApproveContentDisposition(ctx, lc.ApproveContentDispositionCommand{CommandHeader: lc.CommandHeader{WorkspaceID: d.r.Base.WorkspaceID, IdempotencyKey: "remaining-approve:" + name}, PlanID: p.ID, BasisRef: "REMAINING_NON_DELETE_REHEARSAL:" + d.r.Batch.String(), ExpiresAt: d.r.Base.ExpiresAt})
	if err == nil {
		d.approvals[name] = a.ID
	}
	return err
}
func (d *remainingDisposition) dispatch(ctx context.Context, name, key string, cp domain.Copy) (lc.DispatchResult, error) {
	// All native lookups finish before the command takes Core fences/row locks.
	p, err := d.inspector.InspectRetentionAndHold(ctx, cp)
	if err != nil {
		return lc.DispatchResult{}, err
	}
	inv, err := d.inspector.InspectNamespace(ctx, cp)
	if err != nil {
		return lc.DispatchResult{}, err
	}
	d.deps.protection[cp.ID] = p
	d.deps.inventories[cp.Namespace] = inv
	return d.service.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: lc.CommandHeader{WorkspaceID: d.r.Base.WorkspaceID, IdempotencyKey: "remaining-dispatch:" + key}, PlanID: d.plans[name].ID, ApprovalID: d.approvals[name], CopyID: cp.ID})
}
func remainingNoIO(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	return count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation IN('READ_PINNED','PROCESSING_PUT')`)
}
func remainingBusiness(ctx context.Context, r remainingConfig, pool *pgxpool.Pool, original []domain.Copy) error {
	if r.Group != "business-races" {
		return lc.ErrStorageScope
	}
	c := r.Base
	var s remainingState
	if err := readRemaining(filepath.Join(c.OutputDirectory, "remaining-state.json"), &s); err != nil {
		return err
	}
	cs, err := planeCopies(ctx, c, pool)
	if err != nil {
		return err
	}
	o, _, reader, pins, err := remainingServices(r, pool, cs)
	if err != nil {
		return err
	}
	d, err := newRemainingDisposition(ctx, r, pool, cs, o)
	if err != nil {
		return err
	}
	refs := lc.NewFixtureReferenceService(o, datasetapp.FixtureReferenceOwner{}, entityapp.FixtureReferenceOwner{}, workflowapp.FixtureReferenceOwner{}, qualityapp.FixtureReferenceOwner{}, certificationapp.FixtureReferenceOwner{}, deliveryapp.FixtureReferenceOwner{})
	demands := map[uuid.UUID][]referenceDemand{}
	for _, cp := range original[:4] {
		list, e := referenceDemands(ctx, c, pool, refs, cp)
		if e != nil {
			return e
		}
		demands[cp.ID] = list
	}
	h, b, err := remainingEngine(ctx, r, pool, cs, s, true)
	if err != nil {
		return err
	}
	q, err := startRemainingQueue(ctx, r, pool, h, s.P2, s.P1, s.P2)
	if err != nil {
		return err
	}
	defer q.close()
	released := false
	defer func() {
		if !released {
			close(b.release)
		}
	}()
	// Park the actual P2 queue delivery before its handler. Restriction then wins
	// the real first worker admission; a Core QUEUED row alone is insufficient.
	if err = q.client.EnqueueExecution(ctx, s.P2); err != nil {
		return err
	}
	select {
	case <-q.parked:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err = q.client.EnqueueExecution(ctx, s.P1); err != nil {
		return err
	}
	select {
	case <-b.entered:
	case result := <-q.done:
		return fmt.Errorf("P1 owner exited before actual GET barrier: %+v", result)
	case <-ctx.Done():
		return ctx.Err()
	}
	if err = d.restrict(ctx, original[0]); err != nil {
		return err
	}
	if err = d.approve(ctx, "P1"); err != nil {
		return err
	}
	active, err := d.dispatch(ctx, "P1", "P1-live", original[0])
	if err != nil || active.Admitted {
		return fmt.Errorf("P1 live owner admitted: %+v %v", active, err)
	}
	for _, ref := range demands[original[0].ID] {
		if _, e := releaseReference(ctx, c, refs, ref); e == nil {
			return errors.New("P1 live pin released by assertion")
		}
	}
	close(b.release)
	released = true
	if err = q.wait(ctx, s.P1); err != nil {
		return err
	}
	wf := workflowinfra.NewPostgresRepository(pool)
	p1, err := wf.GetExecution(ctx, s.P1)
	if err != nil || p1.Status != workflowdomain.ExecutionFailed || p1.OutputDatasetVersionID != nil {
		return errors.New("P1 restriction failed to prevent READY")
	}
	if err = d.restrict(ctx, original[1]); err != nil {
		return err
	}
	if err = d.approve(ctx, "P2"); err != nil {
		return err
	}
	before, err := remainingNoIO(ctx, pool)
	if err != nil {
		return err
	}
	q.release()
	if err = q.wait(ctx, s.P2); err != nil {
		return err
	}
	p2, err := wf.GetExecution(ctx, s.P2)
	if err != nil || p2.Status != workflowdomain.ExecutionFailed || p2.OutputDatasetVersionID != nil {
		return errors.New("P2 late task not terminally denied")
	}
	// Genuine queue replay and explicit retry use the production command/handler.
	if err = q.client.EnqueueExecution(ctx, s.P2); err != nil {
		return err
	}
	if err = q.wait(ctx, s.P2); err != nil {
		return err
	}
	if _, err = workflowapp.NewExecutionService(transaction.NewManager(pool), wf).Retry(ctx, workflowapp.RetryExecutionCommand{ExecutionID: s.P2, IdempotencyKey: "remaining-P2-retry", ActorID: &c.ActorID}); err == nil {
		return errors.New("P2 restricted explicit retry admitted")
	}
	after, err := remainingNoIO(ctx, pool)
	if err != nil || before != after {
		return errors.New("P2 caused GET/PUT")
	}
	transfers, err := remainingTransfers(ctx, r, pool, original, s, o, reader, pins, d)
	if err != nil {
		return err
	}
	// The only processing owner has joined, every native input handle was closed
	// by PinnedEngine, both task terminals persisted, and HTTP writers have joined.
	// An expired deadline, cancelled context or arbitrary caller boolean is never
	// the source of this module-owner closure attestation.
	n, err := count(ctx, pool, `SELECT count(*) FROM lifecycle_reader_pin p WHERE NOT EXISTS(SELECT 1 FROM lifecycle_reader_termination t WHERE t.pin_id=p.id)`)
	if err != nil || n != 0 {
		return errors.New("native owner closure incomplete")
	}
	if err = write(c, "remaining-owner-closure.json", map[string]any{"P1": p1, "P2": p2, "queue_tasks": q.history, "transfers": transfers, "all_pin_terminations_committed": true, "engine_and_http_owners_joined": true, "at": time.Now().UTC()}); err != nil {
		return err
	}
	var admitted []lc.DispatchResult
	for i, name := range []string{"P1", "P2", "T1", "T2"} {
		cp := original[i]
		// T1's real HTTP command adds a DELIVERY reference after the first
		// snapshot. Re-enumerate the entire current typed set before closure.
		list, e := referenceDemands(ctx, c, pool, refs, cp)
		if e != nil {
			return e
		}
		demands[cp.ID] = list
		for _, ref := range demands[cp.ID] {
			if _, err = releaseReference(ctx, c, refs, ref); err != nil {
				return err
			}
		}
		result, e := d.dispatch(ctx, name, name+"-closed", cp)
		if e != nil || !result.Admitted {
			return fmt.Errorf("%s after proven owner closure blocked: %+v %v", name, result, e)
		}
		admitted = append(admitted, result)
	}
	claims, err := count(ctx, pool, `SELECT count(*) FROM lifecycle_provider_send_claim`)
	if err != nil || claims != 0 {
		return errors.New("non-delete composition acquired a send claim")
	}
	return write(c, "remaining-business-regression.json", map[string]any{"P1_live": active, "P2_native_io_delta": after - before, "transfers": transfers, "admission_after_owner_closure": admitted, "real_asynq": q.history, "send_claims": claims, "real_delete": 0, "full_D1": "NOT_RUN"})
}
