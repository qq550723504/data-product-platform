// Bounded synthetic recovery acceptance. There is no deleter, native write,
// object cleanup, hold release or bypass. COPY recovery hashes bounded target
// bytes internally; it never returns or publishes a data payload.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	lifecyclequeue "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/transport/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	platformqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type config struct {
	Mode, Scenario, PostgresDSN, RedisAddr, RecoveryConfig, Manifest, OutputDirectory, Barrier string
	SeedEnabled                                                                                bool
	CopyRecovery                                                                               bool
	CopyCallID                                                                                 uuid.UUID
	WorkspaceID, ActorID, CopyID, WorkID, AttemptID                                            uuid.UUID
	ExpiresAt                                                                                  time.Time
}

func load(path string) (config, error) {
	var c config
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 32<<10 {
		return c, errors.New("acceptance config unreadable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return c, errors.New("acceptance config invalid")
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return c, errors.New("acceptance trailing data")
	}
	if c.WorkspaceID == uuid.Nil || c.ActorID == uuid.Nil || c.PostgresDSN == "" || !filepath.IsAbs(c.OutputDirectory) || !filepath.IsAbs(c.RecoveryConfig) || !time.Now().Before(c.ExpiresAt) {
		return c, errors.New("acceptance config scope invalid")
	}
	return c, nil
}
func write(path string, v any) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	d := json.NewEncoder(f)
	d.SetIndent("", "  ")
	return d.Encode(v)
}

// Only the one-shot SQL input builder has these test capabilities. It cannot
// construct a storage client or send any provider request. Its approval basis
// explicitly represents synthetic fault input, NOT human/native delete approval.
type sqlAuthority struct{ path string }

func (a sqlAuthority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, at time.Time) (principal.RequestPrincipal, error) {
	c, e := load(a.path)
	if e != nil || w != c.WorkspaceID || !at.Before(c.ExpiresAt) {
		return principal.RequestPrincipal{}, principal.ErrCapabilityDenied
	}
	caps := map[string]struct{}{lc.CapabilityPreparePlan: {}, lc.CapabilityRestrictUse: {}, lc.CapabilityApproveDisposition: {}, lc.CapabilityDispatch: {}}
	subject := "synthetic-sql-fault-input-only"
	if c.Mode == "review" && !c.SeedEnabled {
		caps = map[string]struct{}{lc.CapabilityPreparePlan: {}}
		subject = "lifecycle-scope-review-only"
	} else if c.Mode != "seed" || !c.SeedEnabled {
		return principal.RequestPrincipal{}, principal.ErrCapabilityDenied
	}
	if _, ok := caps[cap]; !ok {
		return principal.RequestPrincipal{}, principal.ErrCapabilityDenied
	}
	return principal.RequestPrincipal{WorkspaceID: w, ActorID: c.ActorID, Subject: subject, Capabilities: caps}, nil
}
func (a sqlAuthority) AuthorizePlan(ctx context.Context, tx pgx.Tx, w uuid.UUID) (principal.RequestPrincipal, error) {
	return a.AuthorizeLifecycle(ctx, tx, w, lc.CapabilityPreparePlan, time.Now().UTC())
}
func (a sqlAuthority) CheckApproverAuthority(ctx context.Context, tx pgx.Tx, v domain.DispositionApproval, at time.Time) (domain.Check, error) {
	p, e := a.AuthorizeLifecycle(ctx, tx, v.WorkspaceID, lc.CapabilityApproveDisposition, at)
	if e != nil || p.ActorID != v.ActorID || v.BasisRef != "SYNTHETIC_SQL_FAULT_INPUT_NO_NATIVE_DELETE_APPROVAL" {
		return domain.Denied, e
	}
	return domain.Satisfied, nil
}

type sqlDependencies struct{}

func (sqlDependencies) EvaluateDisposalDependencies(_ context.Context, _ pgx.Tx, r lc.DisposalDependencyRequest) (lc.DisposalDependencies, error) {
	return lc.DisposalDependencies{WorkspaceID: r.Copy.WorkspaceID, PlanID: r.Plan.ID, CopyID: r.Copy.ID, PlanHash: r.Plan.ContentHash, ActorID: r.Caller.ActorID, CheckedAt: r.AsOf, DependencyRevision: r.DependencyRevision,
		Retention: []domain.RetentionBoundary{{PolicyID: uuid.New(), StartEventID: uuid.New(), StartsAt: r.AsOf.Add(-time.Minute), RetainUntil: r.AsOf.Add(-time.Second)}}, DisposalDueAt: r.AsOf.Add(-time.Second), InventoryComplete: domain.Satisfied, ReferencesReleased: domain.Satisfied, ProviderRetentionAllows: domain.Satisfied, LedgerCurrent: domain.Satisfied, WindowMode: domain.UseUnbounded}, nil
}
func seed(ctx context.Context, path string, c config, pool *pgxpool.Pool) error {
	if !c.SeedEnabled || c.CopyID == uuid.Nil {
		return errors.New("synthetic seed disabled")
	}
	var m struct {
		WorkspaceID uuid.UUID `json:"workspace_id"`
		Objects     []struct {
			CopyID         uuid.UUID `json:"copy_id"`
			Namespace, Key string
			Version        string `json:"version_id"`
			Retention      string `json:"retention_mode"`
			LegalHold      bool   `json:"legal_hold"`
		}
	}
	b, e := os.ReadFile(c.Manifest)
	if e != nil || json.Unmarshal(b, &m) != nil || m.WorkspaceID != c.WorkspaceID || len(m.Objects) != 4 {
		return errors.New("synthetic manifest incomplete")
	}
	allowed := false
	for _, v := range m.Objects {
		if v.CopyID == c.CopyID && v.Namespace == "dpp-lifecycle307-online" && v.Key == "fixture/a.csv" && v.Retention == "NONE" && !v.LegalHold {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("SQL fault input must name the new unprotected synthetic online/a")
	}
	var exists bool
	if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_attempt)`).Scan(&exists); e != nil || exists {
		return errors.New("one-shot SQL input already exists")
	}
	a := sqlAuthority{path}
	repo := &infrastructure.PostgresRepository{}
	tx := transaction.NewManager(pool)
	o := lc.NewOccupancyService(tx, repo, a, nil, nil)
	p, e := lc.NewPlanService(tx, repo, a).PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: []uuid.UUID{c.CopyID}, IdempotencyKey: "worker-sql-fault-plan"})
	if e != nil {
		return e
	}
	if _, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "worker-sql-restrict"}, DatasetVersionID: p.Targets[0].DatasetVersionID, BasisRef: "closed-synthetic-worker-test"}); e != nil {
		return e
	}
	d := lc.NewDispositionService(o, a, sqlDependencies{}, nil)
	approval, e := d.ApproveContentDisposition(ctx, lc.ApproveContentDispositionCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "worker-sql-approval"}, PlanID: p.ID, BasisRef: "SYNTHETIC_SQL_FAULT_INPUT_NO_NATIVE_DELETE_APPROVAL", ExpiresAt: c.ExpiresAt})
	if e != nil {
		return e
	}
	result, e := d.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "worker-sql-dispatch"}, PlanID: p.ID, CopyID: c.CopyID, ApprovalID: approval.ID})
	if e != nil || !result.Admitted {
		return fmt.Errorf("SQL fault input failed: %w", e)
	}
	var work uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_reconciliation_work WHERE attempt_id=$1`, result.AttemptID).Scan(&work); e != nil {
		return e
	}
	return write(filepath.Join(c.OutputDirectory, "synthetic-input.json"), map[string]any{"schema": "SQL_FAULT_INPUT_ONLY_NOT_NATIVE_DELETE_EVIDENCE", "workspace_id": c.WorkspaceID, "copy_id": c.CopyID, "plan_id": p.ID, "plan_hash": p.ContentHash, "attempt_id": result.AttemptID, "work_id": work, "native_delete_invocations": 0, "provider_send_claims": 0, "original_certainty": "UNKNOWN_NO_NATIVE_INVOCATION_ASSERTED", "approval": "NO_NATIVE_DELETE_APPROVAL"})
}

type crashInspector struct {
	base              lc.StorageInspector
	scenario, barrier string
}

func (i crashInspector) InspectNamespace(ctx context.Context, c domain.Copy) (lc.NamespaceInventory, error) {
	if i.scenario == "crash-before-lookup" {
		if write(i.barrier, map[string]string{"fault": "PROCESS_EXIT_BEFORE_LOOKUP_AFTER_RUN_START"}) == nil {
			os.Exit(91)
		}
	}
	v, e := i.base.InspectNamespace(ctx, c)
	if e != nil {
		return v, e
	}
	if i.scenario == "crash-after-lookup" {
		if write(i.barrier, map[string]string{"fault": "PROCESS_EXIT_AFTER_DURABLE_NATIVE_LOOKUP", "call_id": v.CallID.String()}) == nil {
			os.Exit(92)
		}
	}
	if i.scenario == "hold-owner" || i.scenario == "revoke-after-lookup" {
		if e = write(i.barrier, map[string]string{"fault": "OWNER_HELD_AFTER_LOOKUP", "call_id": v.CallID.String()}); e != nil {
			return v, e
		}
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, e = os.Stat(i.barrier + ".release"); e == nil {
				break
			}
			select {
			case <-ctx.Done():
				return v, ctx.Err()
			case <-ticker.C:
			}
		}
	}
	return v, nil
}
func (i crashInspector) InspectExactCopy(ctx context.Context, c domain.Copy) (lc.ExactCopyInspection, error) {
	return i.base.InspectExactCopy(ctx, c)
}
func (i crashInspector) InspectRetentionAndHold(ctx context.Context, c domain.Copy) (lc.CopyProtection, error) {
	return i.base.InspectRetentionAndHold(ctx, c)
}

func witness(ctx context.Context, c config, pool *pgxpool.Pool) error {
	_, inspector, scope, e := pilot.BuildRecoveryInspectorAt(c.RecoveryConfig, pool)
	if e != nil {
		return e
	}
	var copies []domain.Copy
	if e = transaction.NewManager(pool).Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id FROM lifecycle_fixture_copy WHERE workspace_id=$1 ORDER BY id`, c.WorkspaceID)
		if e != nil {
			return e
		}
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		copies, e = (&infrastructure.PostgresRepository{}).LoadFixtureCopies(ctx, tx, c.WorkspaceID, ids)
		return e
	}); e != nil {
		return e
	}
	if len(copies) != 4 || c.WorkspaceID != scope.WorkspaceID {
		return errors.New("witness scope mismatch")
	}
	var objects []map[string]any
	var absent []domain.Copy
	inventories := map[string]lc.NamespaceInventory{}
	for _, copy := range copies {
		if c.Mode == "survivors" && copy.ID == c.CopyID {
			if copy.Namespace != "dpp-lifecycle307-quarantine" || copy.Key != "fixture/b.csv" || copy.ProviderVersion != "ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2" {
				return lc.ErrStorageScope
			}
			var verified bool
			if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE workspace_id=$1 AND copy_id=$2)`, c.WorkspaceID, copy.ID).Scan(&verified); e != nil || !verified {
				return lc.ErrStorageUnknown
			}
			absent = append(absent, copy)
			continue
		}
		if _, e = inspector.InspectExactCopy(ctx, copy); e != nil {
			return e
		}
		protection, e := inspector.InspectRetentionAndHold(ctx, copy)
		if e != nil {
			return e
		}
		objects = append(objects, map[string]any{"copy": copy, "protection": protection})
		if _, found := inventories[copy.Namespace]; !found {
			v, e := inspector.InspectNamespace(ctx, copy)
			if e != nil {
				return e
			}
			inventories[copy.Namespace] = v
		}
	}
	if c.Mode == "survivors" {
		if len(absent) != 1 || len(objects) != 3 {
			return lc.ErrStorageScope
		}
		for namespace, inv := range inventories {
			expected := map[string]domain.Copy{}
			for _, copy := range copies {
				if copy.Namespace == namespace && copy.ID != c.CopyID {
					expected[copy.Key+"\x00"+copy.ProviderVersion] = copy
				}
			}
			if len(inv.Versions) != len(expected) {
				return lc.ErrStorageContract
			}
			for _, v := range inv.Versions {
				copy, ok := expected[v.Key+"\x00"+v.Version]
				if !ok || v.DeleteMarker || v.ByteSize != copy.ByteSize {
					return lc.ErrStorageContract
				}
				delete(expected, v.Key+"\x00"+v.Version)
			}
			if len(expected) != 0 {
				return lc.ErrStorageContract
			}
		}
	}
	counts := map[string]int{}
	for _, table := range []string{"lifecycle_reconciliation_work", "lifecycle_reconciliation_run", "lifecycle_reconciliation_result", "lifecycle_provider_attempt", "lifecycle_provider_send_claim", "lifecycle_provider_observation", "lifecycle_dispatch_release", "lifecycle_copy_absence_receipt", "lifecycle_plan_completion", "lifecycle_storage_call", "lifecycle_storage_observation", "cost_event", "cost_allocation", "audit_event", "evidence", "outbox_event"} {
		var n int
		if e = pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); e != nil {
			return e
		}
		counts[table] = n
	}
	return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), map[string]any{"objects": objects, "inventories": inventories, "receipt_backed_absent": absent, "counts": counts, "native_delete_invocations": 0, "cleanup_invocations": 0})
}
func run(ctx context.Context, path string, c config) error {
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return errors.New("database unavailable")
	}
	defer pool.Close()
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	if c.Mode == "seed" {
		return seed(ctx, path, c, pool)
	}
	if c.Mode == "review" {
		if c.SeedEnabled || c.CopyID == uuid.Nil {
			return lc.ErrStorageScope
		}
		var m struct {
			WorkspaceID uuid.UUID `json:"workspace_id"`
			Objects     []struct {
				CopyID         uuid.UUID `json:"copy_id"`
				Namespace, Key string
				Version        string `json:"version_id"`
				Retention      string `json:"retention_mode"`
				LegalHold      bool   `json:"legal_hold"`
			}
		}
		b, e := os.ReadFile(c.Manifest)
		if e != nil || json.Unmarshal(b, &m) != nil || m.WorkspaceID != c.WorkspaceID {
			return lc.ErrStorageScope
		}
		allowed := false
		for _, v := range m.Objects {
			if v.CopyID == c.CopyID && v.Namespace == "dpp-lifecycle307-quarantine" && v.Key == "fixture/b.csv" && v.Retention == "NONE" && !v.LegalHold {
				allowed = true
			}
		}
		if !allowed {
			return lc.ErrStorageScope
		}
		p, e := lc.NewPlanService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, sqlAuthority{path}).PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: []uuid.UUID{c.CopyID}, IdempotencyKey: "worker-real-delete-review-plan"})
		if e != nil {
			return e
		}
		return write(filepath.Join(c.OutputDirectory, "pending-delete-plan.json"), map[string]any{"plan_id": p.ID, "scope_hash": p.ContentHash, "status": p.Status, "targets": p.Targets, "approval": "PENDING_USER_CONFIRMATION", "native_delete_invocations": 0, "storage_invocations": 0, "current_gate": "MUST_RECHECK_AFTER_CONFIRMATION"})
	}
	if c.Mode == "witness" || c.Mode == "survivors" {
		return witness(ctx, c, pool)
	}
	client := platformqueue.NewClient(platformqueue.Config{Addr: c.RedisAddr})
	defer client.Close()
	q := lifecyclequeue.NewClient(client)
	lockPool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return e
	}
	defer lockPool.Close()
	var s *lc.RecoveryService
	if c.CopyRecovery {
		wrap := func(p lc.CopyRecoveryPort) lc.CopyRecoveryPort { return copyCrashPort{p, c.Scenario, c.Barrier} }
		s, e = pilot.BuildCopyRecoveryWithPortAt(c.RecoveryConfig, pool, transaction.NewManager(lockPool), q, wrap)
		if e != nil {
			return e
		}
	} else {
		execution, inspector, scope, err := pilot.BuildRecoveryInspectorAt(c.RecoveryConfig, pool)
		if err != nil {
			return err
		}
		if scope.WorkspaceID != c.WorkspaceID || scope.ActorID != c.ActorID {
			return errors.New("worker scope mismatch")
		}
		s = lc.NewRecoveryService(execution, crashInspector{inspector, c.Scenario, c.Barrier}, transaction.NewManager(lockPool), q, []uuid.UUID{scope.WorkspaceID})
	}
	switch c.Mode {
	case "outbox":
		unsupported := func(context.Context, outbox.PublishedEvent) error { return lc.ErrStorageUnsupported }
		d, e := outbox.NewDispatcher(pool, outbox.Config{ConsumerName: "lifecycle-controlled-worker", FailureBase: 50 * time.Millisecond, FailureCap: 100 * time.Millisecond, Logger: slog.New(slog.DiscardHandler)}, router,
			outbox.HandlerRegistration{Name: routing.HandlerExecutionQueue, Handle: unsupported}, outbox.HandlerRegistration{Name: routing.HandlerExternalPublicationQueue, Handle: unsupported}, outbox.HandlerRegistration{Name: routing.HandlerLifecycleRecoveryQueue, Handle: lifecyclequeue.OutboxHandler(q)})
		if e != nil {
			return e
		}
		for n := 0; n < 100; n++ {
			if e = d.DispatchOnce(ctx); e != nil {
				return e
			}
			var state string
			var confirmations int
			event := lc.ReconciliationRequestedEvent
			if c.CopyRecovery {
				event = lc.CopyReconciliationRequestedEvent
			}
			if e = pool.QueryRow(ctx, `SELECT e.status,(SELECT count(*) FROM outbox_event_consumption c WHERE c.event_id=e.id) FROM outbox_event e WHERE aggregate_id=$1 AND event_type=$2`, c.WorkID, event).Scan(&state, &confirmations); e != nil {
				return e
			}
			if state == "PUBLISHED" || state == "FAILED" {
				return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), map[string]any{"state": state, "handler_confirmations": confirmations, "work_id": c.WorkID})
			}
		}
		return errors.New("outbox request not reached within finite drain")
	case "request":
		if c.CopyRecovery {
			r, e := s.RequestCopyRecovery(ctx, lc.RequestCopyRecoveryCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "copy-worker-request:" + c.Scenario}, CopyCallID: c.CopyCallID, BasisRef: "controlled-copy-recovery:" + c.Scenario})
			if e != nil {
				return e
			}
			return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), r)
		}
		r, e := s.RequestRecovery(ctx, lc.RequestRecoveryCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "worker-request:" + c.Scenario}, AttemptID: c.AttemptID, BasisRef: "controlled-read-only-reconciliation:" + c.Scenario})
		if e != nil {
			return e
		}
		return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), r)
	case "enqueue":
		if e = q.EnqueueRecovery(ctx, c.WorkspaceID, c.WorkID); e != nil {
			return e
		}
		return q.EnqueueRecovery(ctx, c.WorkspaceID, c.WorkID)
	case "scan":
		return s.RunOnce(ctx)
	case "once":
		e = s.RunRecovery(ctx, c.WorkspaceID, c.WorkID)
		if errors.Is(e, transaction.ErrAdvisoryLockBusy) {
			return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), map[string]any{"result": "OWNER_BUSY", "provider_requests": 0})
		}
		if e != nil {
			return e
		}
		return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), map[string]string{"result": "COMPLETED_READ_ONLY_RECOVERY"})
	case "worker", "worker-result":
		var priorResults int
		if e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1`, c.WorkID).Scan(&priorResults); e != nil {
			return e
		}
		server := platformqueue.NewServer(platformqueue.Config{Addr: c.RedisAddr})
		mux := asynq.NewServeMux()
		mux.HandleFunc(lifecyclequeue.TaskRecover, lifecyclequeue.Handle(s))
		if e = server.Start(mux); e != nil {
			return e
		}
		defer server.Shutdown()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				var done bool
				if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_reconciliation_result WHERE work_id=$1 AND terminal)`, c.WorkID).Scan(&done); e != nil {
					return e
				}
				if done {
					return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), map[string]string{"result": "DURABLE_TERMINAL_RECOVERY"})
				}
				if c.Mode == "worker-result" {
					var n int
					if e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1`, c.WorkID).Scan(&n); e != nil {
						return e
					}
					if n > priorResults {
						return write(filepath.Join(c.OutputDirectory, c.Scenario+".json"), map[string]any{"result": "DURABLE_RESULT_RECOVERY", "terminal": false, "new_results": n - priorResults})
					}
				}
			}
		}
	}
	return errors.New("unsupported finite acceptance mode")
}
func main() {
	if path := os.Getenv("LIFECYCLE_P1_POST_V22_CONFIG"); path != "" {
		if len(os.Args) != 2 || os.Args[1] != "p1-post-v22-readonly" {
			fmt.Fprintln(os.Stderr, "fixed P1 read-only command required")
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if e := runP1PostV22(ctx, path); e != nil {
			fmt.Fprintln(os.Stderr, "P1 post-v22 read-only reconciliation failed:", e)
			os.Exit(1)
		}
		fmt.Println("P1 post-v22 read-only reconciliation completed; DELETE=0")
		return
	}
	path := os.Getenv("LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG")
	c, e := load(path)
	if e == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		e = run(ctx, path, c)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "controlled recovery acceptance failed:", e)
		os.Exit(1)
	}
	fmt.Println("Controlled read-only recovery step completed; DELETE=0 cleanup=0")
}
