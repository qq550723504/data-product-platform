package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// New human-confirmed scope. Consumed profiles above remain unchanged.
func selectRaceDelete() {
	raceDelete = true
	run = "9812f0fc-c05a-4bcb-87ea-a4cef9089f3f"
	workspace = uuid.MustParse("b2412364-b7a4-4df6-ab45-db6ded8e4104")
	providerID = "bca92ac3-a695-4a12-bfeb-08b5b0edef22"
	incarnationID = "1976426c-4c82-404c-8def-261b73e490b6"
	planID = uuid.MustParse("756a4139-bfae-4b5e-9368-e65d28d4c877")
	planHash = "fa2b19e1d7fc12132b3f4e4b4acee09e4f5bc0f59eae87820b83fb2fff220c86"
	inventoryPlanID = uuid.MustParse("648fcdc9-621d-4568-8609-546812a49be6")
	inventoryHash = "8748bb6c397f3961e2637e8bb31e014152be4f75013a4e3c99d71e3a9ae12580"
}

func raceCopies(c config) ([]domain.Copy, error) {
	var m manifest
	if e := read(filepath.Join(c.OutputDirectory, "manifest.json"), &m); e != nil {
		return nil, e
	}
	expected := map[string]string{
		"dpp-lifecycle307-online/fixture/a.csv":     "0f6aa755-4b86-496b-9ee1-43d7c59a8bb3",
		"dpp-lifecycle307-online/fixture/b.csv":     "61bed66a-9fe1-47bd-8a97-a376ad7bed89",
		"dpp-lifecycle307-quarantine/fixture/a.csv": "a0a94326-1914-44e0-aaf8-577b6d824084",
		"dpp-lifecycle307-quarantine/fixture/b.csv": "e1611b0a-efc0-4939-abc2-30394ec2c8d2",
	}
	if len(m.Objects) != 4 {
		return nil, lc.ErrStorageScope
	}
	var all []domain.Copy
	for _, o := range m.Objects {
		key := o.Namespace + "/" + o.Key
		if expected[key] != o.Version {
			return nil, lc.ErrStorageScope
		}
		delete(expected, key)
		cp := domain.Copy{ID: o.CopyID, WorkspaceID: workspace, DatasetVersionID: o.DatasetVersionID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: o.Namespace, Key: o.Key, ProviderVersion: o.Version, ContentSHA256: o.SHA256, ByteSize: o.Bytes}
		if cp.Validate() != nil {
			return nil, lc.ErrStorageScope
		}
		all = append(all, cp)
	}
	if len(expected) != 0 {
		return nil, lc.ErrStorageScope
	}
	return all, nil
}

func provisionRaceReader(ctx context.Context, c config, admin *madmin.AdminClient, call func(func(context.Context) error) error) error {
	if c.ReaderAccess == "" || c.ReaderSecret == "" {
		return lc.ErrStorageScope
	}
	statements := []any{}
	for _, target := range []struct{ bucket, key, version string }{{"dpp-lifecycle307-online", "fixture/a.csv", "0f6aa755-4b86-496b-9ee1-43d7c59a8bb3"}, {"dpp-lifecycle307-quarantine", "fixture/b.csv", "e1611b0a-efc0-4939-abc2-30394ec2c8d2"}} {
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:GetObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + target.bucket + "/" + target.key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": target.version}}})
	}
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
	name := "lifecycle307-race-reader-" + run
	if e := call(func(x context.Context) error { return admin.AddCannedPolicy(x, name, policy) }); e != nil {
		return errors.New("reader policy outcome unknown; no retry")
	}
	if e := call(func(x context.Context) error {
		return admin.SetUser(x, c.ReaderAccess, c.ReaderSecret, madmin.AccountEnabled)
	}); e != nil {
		return errors.New("reader creation outcome unknown; no retry")
	}
	if e := call(func(x context.Context) error {
		_, e := admin.AttachPolicy(x, madmin.PolicyAssociationReq{Policies: []string{name}, User: c.ReaderAccess})
		return e
	}); e != nil {
		return errors.New("reader policy attachment outcome unknown; no retry")
	}
	return write(filepath.Join(c.OutputDirectory, "race-reader-policy.json"), json.RawMessage(policy))
}

type raceGate struct{ policy *lc.CopyService }

type raceNoCustody struct{}

func (raceNoCustody) CurrentCopyProtection(context.Context, pgx.Tx, domain.Copy, time.Time) (lc.CopyProtection, error) {
	return lc.CopyProtection{}, lc.ErrStorageUnsupported
}

func (g *raceGate) EvaluateFixtureUse(ctx context.Context, tx pgx.Tx, r lc.UseGateRequest) (domain.UseDecision, error) {
	return g.policy.EvaluateFixtureUse(ctx, tx, r)
}

type raceWriter struct{ entered, release chan struct{} }

func (w raceWriter) Write(b []byte) (int, error) { return 0, lc.ErrStorageUnsupported }
func (w raceWriter) WriteContext(ctx context.Context, b []byte) (int, error) {
	close(w.entered)
	select {
	case <-w.release:
		return len(b), nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Real native GET + durable ordinary fixture-reader pin, deliberately not
// claimed as an Execution or certified DirectData business-race acceptance.
func executeRace(ctx context.Context) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	stage := os.Getenv("LIFECYCLE_RACE_STAGE")
	if (stage != "a" && stage != "b") || c.RootAccess != "" || c.RootSecret != "" || c.ReaderAccess == "" || c.ReaderSecret == "" {
		return lc.ErrStorageScope
	}
	all, e := raceCopies(c)
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return errors.New("database unavailable")
	}
	defer pool.Close()
	var schema int
	if e = pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&schema); e != nil || schema != 66 {
		return lc.ErrStorageScope
	}
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	tx := transaction.NewManager(pool)
	repo := &infrastructure.PostgresRepository{}
	auth := authority{}
	g := &raceGate{}
	o := lc.NewOccupancyService(tx, repo, auth, g, nil)
	var quarantine uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND namespace='dpp-lifecycle307-quarantine'`, workspace).Scan(&quarantine); e != nil {
		return e
	}
	g.policy = lc.NewCopyService(o, []lc.CopyTargetSlot{{Slot: "read-policy-only", Action: domain.ArchiveCopy, NamespaceID: quarantine, Namespace: "dpp-lifecycle307-quarantine", Key: "archive/a.csv"}}, raceNoCustody{}, nil, c.RunID)
	if g.policy == nil {
		return lc.ErrStorageScope
	}
	dep := &dependencies{all: all, protection: map[uuid.UUID]lc.CopyProtection{}, inventory: map[string]lc.NamespaceInventory{}}
	d := lc.NewDispositionService(o, auth, dep, nil)
	execution := lc.NewExactExecution(d)
	h := func(key string) lc.CommandHeader {
		return lc.CommandHeader{WorkspaceID: workspace, IdempotencyKey: run + ":race:" + key}
	}
	var approved, inventory domain.Plan
	e = tx.Do(ctx, func(ctx context.Context, t pgx.Tx) error {
		var e error
		approved, e = repo.GetPlan(ctx, t, workspace, planID, false)
		if e != nil {
			return e
		}
		inventory, e = repo.GetPlan(ctx, t, workspace, inventoryPlanID, false)
		if e != nil {
			return e
		}
		if approved.VerifyFrozenScope() != nil || inventory.VerifyFrozenScope() != nil || approved.ContentHash != planHash || inventory.ContentHash != inventoryHash || len(approved.Targets) != 2 || len(inventory.Targets) != 4 {
			return lc.ErrStorageScope
		}
		return nil
	})
	if e != nil {
		return e
	}
	var target domain.Copy
	for _, cp := range approved.Targets {
		if (stage == "a" && cp.Namespace == "dpp-lifecycle307-online" && cp.Key == "fixture/a.csv") || (stage == "b" && cp.Namespace == "dpp-lifecycle307-quarantine" && cp.Key == "fixture/b.csv") {
			target = cp
		}
	}
	if target.ID == uuid.Nil || !containsCopy(all, target.ID) {
		return lc.ErrStorageScope
	}
	var previous bool
	if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_provider_attempt WHERE copy_id=$1)`, target.ID).Scan(&previous); e != nil || previous {
		return errors.New("attempt exists; recovery only, never resend")
	}
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: workspace, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.NewPostgresStorageCallJournal(o)})
	if e != nil {
		return e
	}
	refresh := func() error {
		protection, e := inspector.InspectRetentionAndHold(ctx, target)
		if e != nil {
			return e
		}
		dep.protection[target.ID] = protection
		inv, e := inspector.InspectNamespace(ctx, target)
		if e != nil {
			return e
		}
		dep.inventory[target.Namespace] = inv
		return nil
	}
	basis, e := execution.RegisterFixtureDisposalBasis(ctx, lc.FixtureDisposalBasisCommand{CommandHeader: h("basis"), RunID: c.RunID, InventoryPlanID: inventoryPlanID, ApprovedPlanID: planID, InventoryHash: inventoryHash, ApprovedHash: planHash, BasisRef: p.BasisRef})
	if e != nil {
		return e
	}
	// A current disposal approval already stops new fixture use. Exit earlier
	// test approvals by Command, establish the actual reader, then approve again.
	rows, e := pool.Query(ctx, `SELECT a.id FROM lifecycle_disposition_approval a WHERE a.plan_id=$1 AND NOT EXISTS(SELECT 1 FROM lifecycle_approval_revocation r WHERE r.approval_id=a.id) ORDER BY a.id`, planID)
	if e != nil {
		return e
	}
	var priorApprovals []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		priorApprovals = append(priorApprovals, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range priorApprovals {
		if _, e = d.RevokeDispositionApproval(ctx, lc.RevokeDispositionApprovalCommand{CommandHeader: h("reader-first-revoke-" + stage + "-" + id.String()), ApprovalID: id, BasisRef: "Controlled reader-first order; old approval retained"}); e != nil {
			return e
		}
	}
	if _, e = g.policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: h("policy-" + stage), DatasetVersionID: target.DatasetVersionID, UseUntil: p.ExpiresAt, RestoreUntil: p.ExpiresAt}); e != nil {
		return e
	}
	native, e := storage.NewLifecycleReader(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: c.ReaderAccess, SecretKey: c.ReaderSecret, WorkspaceID: workspace, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.PinnedReaderCallJournal{Service: o, Observations: lc.NewPostgresStorageCallJournal(o)}}, all)
	if e != nil {
		return e
	}
	reader := lc.NewFixtureReaderService(o, native)
	if reader == nil {
		return lc.ErrStorageScope
	}
	caller, e := auth.AuthorizeLifecycle(ctx, nil, workspace, lc.CapabilityAdmitReader, time.Now().UTC())
	if e != nil {
		return e
	}
	rctx := lc.WithFixtureReaderCaller(ctx, caller)
	writer := raceWriter{make(chan struct{}), make(chan struct{})}
	type readResult struct {
		result lc.FixtureReadResult
		err    error
	}
	done := make(chan readResult, 1)
	go func() {
		r, e := reader.ReadTo(rctx, lc.AdmitFixtureReaderCommand{CommandHeader: h("reader-first-" + stage), CopyID: target.ID, ReaderID: uuid.New()}, writer)
		done <- readResult{r, e}
	}()
	select {
	case <-writer.entered:
	case early := <-done:
		return fmt.Errorf("native reader failed before barrier: %w", early.err)
	case <-ctx.Done():
		return ctx.Err()
	}
	released := false
	defer func() {
		if !released {
			close(writer.release)
			stopped := <-done
			_ = write(filepath.Join(c.OutputDirectory, "race-error-stop-"+uuid.NewString()+".json"), stopped.result)
		}
	}()
	approval, e := d.ApproveContentDisposition(ctx, lc.ApproveContentDispositionCommand{CommandHeader: h("approve-after-reader-" + stage), PlanID: planID, BasisRef: p.BasisRef, ExpiresAt: p.ExpiresAt})
	if e != nil {
		return e
	}
	if _, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: h("restrict-" + stage), DatasetVersionID: target.DatasetVersionID, BasisRef: p.BasisRef}); e != nil {
		return e
	}
	if e = refresh(); e != nil {
		return e
	}
	blocked, e := d.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: h("while-reader-" + stage), PlanID: planID, ApprovalID: approval.ID, CopyID: target.ID})
	if e != nil {
		return e
	}
	if blocked.Admitted || len(blocked.Blockers) == 0 || blocked.AttemptID != uuid.Nil {
		return errors.New("live reader did not block dispatch")
	}
	close(writer.release)
	released = true
	read := <-done
	if read.err != nil || !read.result.StopConfirmed || read.result.BytesWritten != target.ByteSize {
		return errors.New("native reader stop not verified")
	}
	if e = refresh(); e != nil {
		return e
	}
	dispatch, e := d.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: h("after-reader-" + stage), PlanID: planID, ApprovalID: approval.ID, CopyID: target.ID})
	if e != nil {
		return e
	}
	if !dispatch.Admitted || len(dispatch.Blockers) != 0 {
		return fmt.Errorf("post-drain blocked: %v", dispatch.Blockers)
	}
	later, laterErr := reader.ReadTo(rctx, lc.AdmitFixtureReaderCommand{CommandHeader: h("disposal-first-" + stage), CopyID: target.ID, ReaderID: uuid.New()}, writer)
	if later.Admission.Admitted || later.BytesWritten != 0 || later.CallID != uuid.Nil {
		return errors.New("reserved copy admitted new native read")
	}
	if e = write(filepath.Join(c.OutputDirectory, "race-reader-orders-"+stage+".json"), map[string]any{"copy": target, "reader_first_block": blocked, "verified_stop": read.result, "disposal_first_result": later, "disposal_first_denied": laterErr != nil || !later.Admission.Admitted, "reader_kind": "ordinary fixture reader; not Execution/DirectData", "full_LC11_12": "NOT_RUN"}); e != nil {
		return e
	}
	var attempt domain.ProviderAttempt
	if e = tx.Do(ctx, func(ctx context.Context, t pgx.Tx) error {
		var e error
		attempt, e = repo.GetAttempt(ctx, t, workspace, dispatch.AttemptID, false)
		return e
	}); e != nil {
		return e
	}
	if e = refresh(); e != nil {
		return e
	}
	deleter, e := storage.NewLifecycleDeleter(storage.LifecycleDeleteConfig{Endpoint: c.Endpoint, AccessKey: p.DeleteAccess, SecretKey: p.DeleteSecret, Targets: approved.Targets, Journal: execution})
	if e != nil {
		return e
	}
	sendErr := deleter.DeleteExactCopy(ctx, attempt, target)
	var workID uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_reconciliation_work WHERE attempt_id=$1`, attempt.ID).Scan(&workID); e != nil {
		return e
	}
	if e = write(filepath.Join(c.OutputDirectory, "race-delete-send-"+stage+".json"), map[string]any{"copy": target, "basis": basis, "approval": approval, "dispatch": dispatch, "attempt": attempt, "work_id": workID, "response_unknown": sendErr != nil, "absence": "UNVERIFIED", "next": "READ_ONLY_WORKER_RECOVERY", "resend": "FORBIDDEN"}); e != nil {
		return e
	}
	// Actual process loss after durable provider observation/cost, before absence.
	os.Exit(93)
	return nil
}

func inspectRace(ctx context.Context) error {
	c, _, e := load()
	if e != nil {
		return e
	}
	if c.RootAccess != "" || c.RootSecret != "" {
		return lc.ErrStorageScope
	}
	all, e := raceCopies(c)
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return e
	}
	defer pool.Close()
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, authority{}, nil, nil)
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: workspace, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.NewPostgresStorageCallJournal(o)})
	if e != nil {
		return e
	}
	var original manifest
	if e = read(filepath.Join(c.OutputDirectory, "manifest.json"), &original); e != nil {
		return e
	}
	var protected []any
	inventories := map[string]lc.NamespaceInventory{}
	absent := []domain.Copy{}
	for _, cp := range all {
		var receipt bool
		if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE copy_id=$1)`, cp.ID).Scan(&receipt); e != nil {
			return e
		}
		if receipt {
			absent = append(absent, cp)
			continue
		}
		if cp.ProviderVersion != "61bed66a-9fe1-47bd-8a97-a376ad7bed89" && cp.ProviderVersion != "a0a94326-1914-44e0-aaf8-577b6d824084" {
			return lc.ErrStorageScope
		}
		presence, e := inspector.InspectExactCopy(ctx, cp)
		if e != nil {
			return e
		}
		protection, e := inspector.InspectRetentionAndHold(ctx, cp)
		if e != nil {
			return e
		}
		matched := false
		for _, before := range original.Objects {
			if before.CopyID == cp.ID {
				matched = before.LegalHold == protection.LegalHold && before.Retention == protection.RetentionMode && (before.RetainUntil == nil) == (protection.RetainUntil == nil)
				if matched && before.RetainUntil != nil {
					matched = before.RetainUntil.Equal(*protection.RetainUntil)
				}
			}
		}
		if !matched {
			return errors.New("protected version protection changed")
		}
		protected = append(protected, map[string]any{"copy": cp, "presence": presence, "protection": protection})
		inventory, e := inspector.InspectNamespace(ctx, cp)
		if e != nil {
			return e
		}
		inventories[cp.Namespace] = inventory
	}
	if len(absent) != 2 || len(protected) != 2 {
		return lc.ErrStorageUnknown
	}
	for _, inv := range inventories {
		if !inventoryMatches(inv, all, absent) {
			return lc.ErrStorageContract
		}
	}
	var completion, attempts, claims, pins, stops int
	if e = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM lifecycle_plan_completion WHERE plan_id=$1),(SELECT count(*) FROM lifecycle_provider_attempt),(SELECT count(*) FROM lifecycle_provider_send_claim),(SELECT count(*) FROM lifecycle_reader_pin),(SELECT count(*) FROM lifecycle_reader_termination)`, planID).Scan(&completion, &attempts, &claims, &pins, &stops); e != nil {
		return e
	}
	if completion != 1 || attempts != 2 || claims != 2 || pins != 2 || stops != 2 {
		return lc.ErrStorageContract
	}
	return write(filepath.Join(c.OutputDirectory, "confirmed-race-survivors.json"), map[string]any{"run_id": run, "absent": absent, "inventories": inventories, "protected": protected, "completion": completion, "attempts": attempts, "send_claims": claims, "pins": pins, "verified_stops": stops, "backup_payloads": "retained; excluded from source deletion scope", "full_D1": "NOT_RUN", "full_LC11_12": "NOT_RUN", "cleanup": 0})
}
