// One-shot synthetic custody acceptance. No DELETE, cleanup, hold release or HTTP API.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

var namespaces = []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}

type config struct {
	RunID, WorkspaceID, ProviderInstanceID, IncarnationID, ActorID                                      uuid.UUID
	Enabled, ArchiveAcceptance                                                                          bool
	Endpoint, PostgresDSN, RootAccess, RootSecret, InspectAccess, InspectSecret, Token, OutputDirectory string
	Environment                                                                                         json.RawMessage
}
type window struct {
	Enabled                        bool
	RunID, WorkspaceID, Epoch      uuid.UUID
	Access, Secret, TopologySHA256 string
	StartedAt, ExpiresAt           time.Time
}
type object struct {
	CopyID           uuid.UUID  `json:"copy_id"`
	DatasetVersionID uuid.UUID  `json:"dataset_version_id"`
	Namespace        string     `json:"namespace"`
	Key              string     `json:"key"`
	Version          string     `json:"version_id"`
	SHA256           string     `json:"sha256"`
	Bytes            int64      `json:"bytes"`
	LegalHold        bool       `json:"legal_hold"`
	Retention        string     `json:"retention_mode"`
	RetainUntil      *time.Time `json:"retain_until"`
}
type manifest struct {
	Objects []object `json:"objects"`
}

func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 256<<10 {
		return errors.New("fixture input unavailable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if e = d.Decode(v); e != nil {
		return errors.New("fixture input invalid")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing input")
	}
	return nil
}
func write(path string, v any) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = json.NewEncoder(f).Encode(v); e != nil {
		return e
	}
	return f.Sync()
}
func load() (config, window, error) {
	var c config
	var p window
	if read("/run/private/config.json", &c) != nil || read("/run/private/archive-window.json", &p) != nil {
		return c, p, errors.New("closed copy configuration unavailable")
	}
	if !c.ArchiveAcceptance || c.RunID == uuid.Nil || c.RunID.String() == "a6b30840-3e76-49ad-91d3-f0c399fcf170" || p.RunID != c.RunID || p.WorkspaceID != c.WorkspaceID || p.Epoch == uuid.Nil || c.ActorID == uuid.Nil || c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil || c.Endpoint != "127.0.0.1:9000" || !filepath.IsAbs(c.OutputDirectory) || p.Access == "" || p.Secret == "" || len(p.TopologySHA256) != 64 || !p.Enabled || time.Now().Before(p.StartedAt) || !time.Now().Before(p.ExpiresAt) || p.ExpiresAt.Sub(p.StartedAt) > time.Hour {
		return c, p, errors.New("copy authority or isolated scope unavailable")
	}
	return c, p, nil
}

type authority struct{}

func (authority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, _ time.Time) (principal.RequestPrincipal, error) {
	c, _, e := load()
	if e != nil {
		return principal.RequestPrincipal{}, principal.ErrUnauthenticated
	}
	r, e := principal.NewStaticResolver(true, c.Token, "lifecycle-fixture:"+c.RunID.String(), c.ActorID.String(), []string{c.WorkspaceID.String()}, []string{lc.CapabilityPreparePlan, lc.CapabilityCopyCustody, lc.CapabilityRestoreAdmission, lc.CapabilityInspectStorage, lc.CapabilityRestrictUse, lc.CapabilityAdmitReader, lc.CapabilityTerminateReader})
	if e != nil {
		return principal.RequestPrincipal{}, e
	}
	req, _ := http.NewRequest("POST", "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return r.Resolve(req, w, cap)
}

type cache struct {
	proofs map[uuid.UUID]lc.CopyProtection
}

func (p *cache) CurrentCopyProtection(_ context.Context, _ pgx.Tx, c domain.Copy, _ time.Time) (lc.CopyProtection, error) {
	r, ok := p.proofs[c.ID]
	if !ok || r.Copy != c {
		return r, lc.ErrStorageUnknown
	}
	return r, nil
}

type useGate struct{ s *lc.CopyService }

func (g *useGate) EvaluateFixtureUse(ctx context.Context, tx pgx.Tx, r lc.UseGateRequest) (domain.UseDecision, error) {
	if g.s == nil {
		return domain.UseDecision{}, domain.ErrUseBlocked
	}
	return g.s.EvaluateFixtureUse(ctx, tx, r)
}

type termination struct{ closed map[uuid.UUID]time.Time }

func (v *termination) VerifyTermination(_ context.Context, _ pgx.Tx, p domain.ReaderPin, outcome string, _ time.Time) (domain.ReaderTermination, error) {
	at, ok := v.closed[p.ID]
	if !ok || outcome != domain.ReaderCompleted {
		return domain.ReaderTermination{}, domain.ErrTerminationUnknown
	}
	return domain.ReaderTermination{ID: uuid.New(), WorkspaceID: p.WorkspaceID, PinID: p.ID, ReaderID: p.ReaderID, Generation: p.Generation, Outcome: outcome, ObservedAt: at}, nil
}
func copies(c config) ([]domain.Copy, error) {
	var m manifest
	if e := read(filepath.Join(c.OutputDirectory, "manifest.json"), &m); e != nil {
		return nil, e
	}
	if len(m.Objects) != 4 {
		return nil, lc.ErrStorageScope
	}
	cs := []domain.Copy{}
	for _, o := range m.Objects {
		cp := domain.Copy{ID: o.CopyID, WorkspaceID: c.WorkspaceID, DatasetVersionID: o.DatasetVersionID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: o.Namespace, Key: o.Key, ProviderVersion: o.Version, ContentSHA256: o.SHA256, ByteSize: o.Bytes}
		if cp.Validate() != nil {
			return nil, lc.ErrStorageScope
		}
		cs = append(cs, cp)
	}
	if cs[0].Namespace != namespaces[0] || cs[0].Key != "fixture/a.csv" || m.Objects[0].LegalHold || m.Objects[0].Retention != "NONE" {
		return nil, lc.ErrStorageScope
	}
	return cs, nil
}

var keys = []string{"archive/a.csv", "restore/online-a.csv", "restore/old-cut-a.csv", "restore/missing-a.csv", "restore/gap-a.csv"}

func execute(ctx context.Context) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	if _, e = os.Stat(filepath.Join(c.OutputDirectory, "archive-start.json")); !os.IsNotExist(e) {
		return errors.New("one-shot archive run already started; inspect retained facts, no replay")
	}
	if e = write(filepath.Join(c.OutputDirectory, "archive-start.json"), map[string]any{"run_id": c.RunID, "started_at": time.Now().UTC(), "topology_sha256": p.TopologySHA256}); e != nil {
		return e
	}
	cs, e := copies(c)
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return errors.New("private Core unavailable")
	}
	defer pool.Close()
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	repo := &infrastructure.PostgresRepository{}
	manager := transaction.NewManager(pool)
	gate := &useGate{}
	end := &termination{closed: map[uuid.UUID]time.Time{}}
	o := lc.NewOccupancyService(manager, repo, authority{}, gate, end)
	proofs := &cache{proofs: map[uuid.UUID]lc.CopyProtection{}}
	var ns uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND namespace=$2`, c.WorkspaceID, namespaces[1]).Scan(&ns); e != nil {
		return e
	}
	slots := []lc.CopyTargetSlot{}
	for i, key := range keys {
		action := domain.RestoreCopy
		if i == 0 {
			action = domain.ArchiveCopy
		}
		slots = append(slots, lc.CopyTargetSlot{Slot: key, Key: key, Namespace: namespaces[1], NamespaceID: ns, Action: action})
	}
	ledger := &infrastructure.FileRestoreLedger{Path: "/run/witness/current.jsonl", WorkspaceID: c.WorkspaceID, DatasetVersionID: cs[0].DatasetVersionID, Epoch: p.Epoch}
	service := lc.NewCopyService(o, slots, proofs, ledger, p.Epoch)
	gate.s = service
	if service == nil {
		return lc.ErrStorageScope
	}
	journal := lc.CopyCallJournal{Service: service, Metadata: lc.NewPostgresStorageCallJournal(o)}
	inspect, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: p.Access, SecretKey: p.Secret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: journal})
	if e != nil {
		return e
	}
	// The bootstrap inspection identity remains read-only for the four protected/original versions.
	originals, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: journal})
	if e != nil {
		return e
	}
	copier, e := storage.NewLifecycleCopier(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: p.Access, SecretKey: p.Secret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: journal})
	if e != nil {
		return e
	}
	header := func(key string) lc.CommandHeader {
		return lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "archive-acceptance:" + key}
	}
	refresh := func(cp domain.Copy) error {
		if _, e := inspect.InspectExactCopy(ctx, cp); e != nil {
			return e
		}
		pr, e := inspect.InspectRetentionAndHold(ctx, cp)
		if e == nil {
			proofs.proofs[cp.ID] = pr
		}
		return e
	}
	if _, e = service.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: header("policy"), DatasetVersionID: cs[0].DatasetVersionID, UseUntil: p.ExpiresAt, RestoreUntil: p.ExpiresAt}); e != nil {
		return e
	}
	stages := []map[string]any{}
	all := append([]domain.Copy{}, cs...)
	stage := func(source domain.Copy, key string) (domain.CopyOperation, domain.Copy, error) {
		fmt.Println("copy stage:", key)
		if e := refresh(source); e != nil {
			return domain.CopyOperation{}, domain.Copy{}, e
		}
		op, e := service.PrepareCopy(ctx, lc.PrepareCopyCommand{CommandHeader: header("prepare-" + key), SourceCopyID: source.ID, TargetSlot: key, BasisRef: "controlled-new-synthetic-archive-restore", ExpiresAt: p.ExpiresAt})
		if e != nil {
			return op, domain.Copy{}, e
		}
		cp, call, e := copier.CreateCopy(ctx, op)
		if e != nil {
			return op, cp, e
		}
		if _, e = service.BindMaterializedCopy(ctx, lc.BindMaterializedCommand{CommandHeader: header("bind-" + key), OperationID: op.ID, Copy: cp, CopyCallID: call}); e != nil {
			return op, cp, e
		}
		all = append(all, cp)
		if e = refresh(cp); e != nil {
			return op, cp, e
		}
		pr, e := copier.VerifyCopyBytes(ctx, op, cp)
		if e != nil {
			return op, cp, e
		}
		_, e = service.VerifyMaterializedCopy(ctx, lc.VerifyCopyCommand{CommandHeader: header("verify-" + key), OperationID: op.ID, Proof: pr})
		return op, cp, e
	}
	finalize := func(s *lc.CopyService, op domain.CopyOperation, key, want string) error {
		d, e := s.FinalizeCopy(ctx, lc.FinalizeCopyCommand{CommandHeader: header(key), OperationID: op.ID})
		if e != nil {
			return e
		}
		stages = append(stages, map[string]any{"operation_id": op.ID, "target_copy_id": op.TargetCopyID, "decision": d.Decision, "reason": d.Reason, "decision_id": d.ID})
		if d.Decision != want {
			return fmt.Errorf("%s got %s; expected %s", key, d.Decision, want)
		}
		return nil
	}
	cut := func() error {
		cp, e := service.ExportRestoreCheckpoint(ctx, c.WorkspaceID, cs[0].DatasetVersionID)
		if e != nil {
			return e
		}
		return ledger.Append(cp)
	}
	archive, archived, e := stage(cs[0], keys[0])
	if e != nil {
		return e
	}
	if e = finalize(service, archive, "archive-final", "ARCHIVED"); e != nil {
		return e
	}
	online, restored, e := stage(archived, keys[1])
	if e != nil {
		return e
	}
	if e = cut(); e != nil {
		return e
	}
	if e = finalize(service, online, "online-final", "ONLINE"); e != nil {
		return e
	}
	admitted, e := o.AdmitFixtureReader(ctx, lc.AdmitFixtureReaderCommand{CommandHeader: header("fresh-reader"), CopyID: restored.ID, ReaderID: uuid.New()})
	if e != nil || !admitted.Admitted {
		return fmt.Errorf("fresh finite reader admission failed: %w", e)
	}
	// The adapter fully reads and closes the bounded exact response before returning.
	if _, e = copier.VerifyCopyBytes(ctx, online, restored); e != nil {
		return e
	}
	end.closed[admitted.Pin.ID] = time.Now().UTC()
	if _, e = o.TerminateFixtureReader(ctx, lc.TerminateFixtureReaderCommand{CommandHeader: header("reader-completed"), PinID: admitted.Pin.ID, Generation: admitted.Pin.Generation, Outcome: domain.ReaderCompleted}); e != nil {
		return e
	}
	stale, staleCopy, e := stage(archived, keys[2])
	if e != nil {
		return e
	}
	if _, e = o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: header("new-current-restriction"), DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "current-prohibition-after-old-archive-cut"}); e != nil {
		return e
	}
	if e = finalize(service, stale, "old-watermark", "UNKNOWN"); e != nil {
		return e
	}
	if e = cut(); e != nil {
		return e
	}
	if e = finalize(service, stale, "current-prohibition", "BLOCKED"); e != nil {
		return e
	}
	for _, cp := range []domain.Copy{restored, staleCopy, archived} {
		if _, e = o.AdmitFixtureReader(ctx, lc.AdmitFixtureReaderCommand{CommandHeader: header("blocked-read-" + cp.ID.String()), CopyID: cp.ID, ReaderID: uuid.New()}); !errors.Is(e, domain.ErrUseBlocked) {
			return errors.New("ordinary fixture reader bypassed current restriction/quarantine")
		}
	}
	for i := 3; i < 5; i++ {
		op, _, e := stage(archived, keys[i])
		if e != nil {
			return e
		}
		if e = cut(); e != nil {
			return e
		}
		faultPath := "/run/witness/missing.jsonl"
		if i == 4 {
			latest, e := ledger.Latest(ctx)
			if e != nil {
				return e
			}
			faultPath = "/run/witness/gap.jsonl"
			if e = write(faultPath, latest); e != nil {
				return e
			}
		}
		bad := &infrastructure.FileRestoreLedger{Path: faultPath, WorkspaceID: c.WorkspaceID, DatasetVersionID: cs[0].DatasetVersionID, Epoch: p.Epoch}
		s := lc.NewCopyService(o, slots, proofs, bad, p.Epoch)
		if e = finalize(s, op, "fault-"+keys[i], "UNKNOWN"); e != nil {
			return e
		}
	}
	return report(ctx, c, p, pool, originals, cs, all, stages)
}

// Finishes evidence after a read-only inventory interruption. It has no copy,
// byte-read or admission command and requires every completed DB proof below.
func (authority) AuthorizePlan(ctx context.Context, tx pgx.Tx, w uuid.UUID) (principal.RequestPrincipal, error) {
	return (authority{}).AuthorizeLifecycle(ctx, tx, w, lc.CapabilityPreparePlan, time.Now())
}

// Only a proposal. No approval, dispatch, provider call or cleanup is reachable.
func prepareSourceDeletePlan(ctx context.Context) error {
	c, _, e := load()
	if e != nil {
		return e
	}
	cs, e := copies(c)
	if e != nil {
		return e
	}
	var accepted struct {
		NativeDeleteRequests int               `json:"native_delete_requests"`
		Acceptance           map[string]string `json:"acceptance"`
	}
	if e = read(filepath.Join(c.OutputDirectory, "archive-acceptance.json"), &accepted); e != nil || accepted.NativeDeleteRequests != 0 || accepted.Acceptance["archive_native_same_bytes"] != "PASS" || accepted.Acceptance["source_retained"] != "PASS" {
		return errors.New("source plan requires completed retained-source archive evidence")
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
	var verified int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_copy_operation o JOIN lifecycle_copy_verification v ON v.operation_id=o.id JOIN lifecycle_copy_decision d ON d.operation_id=o.id WHERE o.workspace_id=$1 AND o.source_copy_id=$2 AND o.action='ARCHIVE' AND d.decision='ARCHIVED'`, c.WorkspaceID, cs[0].ID).Scan(&verified); e != nil || verified != 1 {
		return errors.New("source plan lacks verified ARCHIVED fact")
	}
	plan, e := lc.NewPlanService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, authority{}).PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: []uuid.UUID{cs[0].ID}, IdempotencyKey: "archive-source-plan:" + c.RunID.String()})
	if e != nil {
		return e
	}
	return write(filepath.Join(c.OutputDirectory, "archive-source-delete-plan.json"), map[string]any{"run_id": c.RunID, "workspace_id": c.WorkspaceID, "plan_id": plan.ID, "scope_hash": plan.ContentHash, "hash_algorithm": domain.PlanHashAlgorithm, "targets": plan.Targets, "approval": "PENDING_USER_CONFIRMATION", "delete": "NOT_RUN", "cleanup": 0, "note": "Fresh dependency/retention/occupancy gates required after confirmation; all other versions and volumes retained"})
}
func readback(ctx context.Context) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	cs, e := copies(c)
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
	repo := &infrastructure.PostgresRepository{}
	manager := transaction.NewManager(pool)
	o := lc.NewOccupancyService(manager, repo, authority{}, nil, nil)
	originals, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.NewPostgresStorageCallJournal(o)})
	if e != nil {
		return e
	}
	all := []domain.Copy{}
	e = manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT id FROM lifecycle_fixture_copy WHERE workspace_id=$1 ORDER BY object_key,id`, c.WorkspaceID)
		if e != nil {
			return e
		}
		ids := []uuid.UUID{}
		for rows.Next() {
			var id uuid.UUID
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		all, e = repo.LoadFixtureCopies(ctx, tx, c.WorkspaceID, ids)
		return e
	})
	if e != nil {
		return e
	}
	var verified int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_copy_verification v JOIN lifecycle_storage_call c ON c.id=v.verify_call_id JOIN lifecycle_storage_observation x ON x.call_id=c.id WHERE v.workspace_id=$1 AND x.http_status=200 AND x.failure='' AND x.response_sha256=v.content_sha256`, c.WorkspaceID).Scan(&verified); e != nil || verified != 5 {
		return errors.New("readback refuses unfinished native byte verification")
	}
	stages := []map[string]any{}
	rows, e := pool.Query(ctx, `SELECT d.id,d.operation_id,o.target_copy_id,d.decision,d.reason FROM lifecycle_copy_decision d JOIN lifecycle_copy_operation o ON o.id=d.operation_id WHERE d.workspace_id=$1 ORDER BY d.occurred_at`, c.WorkspaceID)
	if e != nil {
		return e
	}
	defer rows.Close()
	for rows.Next() {
		var id, op, target uuid.UUID
		var decision, reason string
		if e = rows.Scan(&id, &op, &target, &decision, &reason); e != nil {
			return e
		}
		stages = append(stages, map[string]any{"decision_id": id, "operation_id": op, "target_copy_id": target, "decision": decision, "reason": reason})
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if len(stages) != 6 || stages[0]["decision"] != "ARCHIVED" || stages[1]["decision"] != "ONLINE" || stages[2]["decision"] != "UNKNOWN" || stages[3]["decision"] != "BLOCKED" || stages[4]["decision"] != "UNKNOWN" || stages[5]["decision"] != "UNKNOWN" {
		return errors.New("readback refuses incomplete restore admission evidence")
	}
	return report(ctx, c, p, pool, originals, cs, all, stages)
}
func report(ctx context.Context, c config, p window, pool *pgxpool.Pool, originals *storage.LifecycleInspector, cs, all []domain.Copy, stages []map[string]any) error {
	var e error
	protected := []lc.CopyProtection{}
	for _, cp := range cs {
		if _, e = originals.InspectExactCopy(ctx, cp); e != nil {
			return e
		}
		pr, e := originals.InspectRetentionAndHold(ctx, cp)
		if e != nil {
			return e
		}
		protected = append(protected, pr)
	}
	var initial manifest
	if e = read(filepath.Join(c.OutputDirectory, "manifest.json"), &initial); e != nil {
		return e
	}
	for i, pr := range protected {
		a := initial.Objects[i]
		if pr.LegalHold != a.LegalHold || pr.RetentionMode != a.Retention || (a.RetainUntil != nil && (pr.RetainUntil == nil || !a.RetainUntil.Equal(*pr.RetainUntil))) {
			return errors.New("original native protection changed")
		}
	}
	inventories := []lc.NamespaceInventory{}
	for _, namespace := range namespaces {
		var subject domain.Copy
		expected := map[string]int64{}
		for _, cp := range all {
			if cp.Namespace == namespace {
				subject = cp
				expected[cp.Key+"\x00"+cp.ProviderVersion] = cp.ByteSize
			}
		}
		inv, e := originals.InspectNamespace(ctx, subject)
		if e != nil {
			return e
		}
		if len(inv.Versions) != len(expected) {
			return errors.New("retained exact inventory cardinality mismatch")
		}
		for _, v := range inv.Versions {
			size, ok := expected[v.Key+"\x00"+v.Version]
			if !ok || size != v.ByteSize || v.DeleteMarker {
				return errors.New("unexpected native version or source disappearance")
			}
		}
		inventories = append(inventories, inv)
	}
	counts := map[string]int{}
	for _, table := range []string{"lifecycle_fixture_copy", "lifecycle_copy_operation", "lifecycle_copy_materialization", "lifecycle_copy_verification", "lifecycle_copy_decision", "lifecycle_restore_checkpoint", "lifecycle_storage_call", "lifecycle_storage_observation", "cost_allocation", "lifecycle_reader_pin", "lifecycle_reader_termination", "lifecycle_disposition_approval", "lifecycle_provider_attempt"} {
		var n int
		if e = pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); e != nil {
			return e
		}
		counts[table] = n
	}
	if counts["lifecycle_fixture_copy"] != 9 || counts["lifecycle_copy_operation"] != 5 || counts["lifecycle_copy_materialization"] != 5 || counts["lifecycle_copy_verification"] != 5 || counts["lifecycle_storage_call"] != counts["lifecycle_storage_observation"] || counts["lifecycle_storage_call"] != counts["cost_allocation"] || counts["lifecycle_reader_pin"] != 1 || counts["lifecycle_reader_termination"] != 1 || counts["lifecycle_disposition_approval"] != 0 || counts["lifecycle_provider_attempt"] != 0 {
		return errors.New("acceptance fact/cost/reader counts mismatch")
	}
	var identity int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version WHERE id=$1 AND checksum_value=$2 AND byte_size=$3 AND status='READY' AND storage_uri=$4`, cs[0].DatasetVersionID, cs[0].ContentSHA256, cs[0].ByteSize, "s3://"+cs[0].Namespace+"/"+cs[0].Key).Scan(&identity); e != nil || identity != 1 {
		return errors.New("dataset identity/hash/original URI changed")
	}
	return write(filepath.Join(c.OutputDirectory, "archive-acceptance.json"), map[string]any{"run_id": c.RunID, "workspace_id": c.WorkspaceID, "epoch": p.Epoch, "environment": json.RawMessage(c.Environment), "verified_at": time.Now().UTC(), "objects": all, "original_protection": protected, "inventories": inventories, "decisions": stages, "facts": counts, "native_delete_requests": 0, "cleanup_requests": 0, "acceptance": map[string]string{"archive_native_same_bytes": "PASS", "restore_current_witness": "PASS", "fresh_fixture_reader": "PASS", "old_watermark": "PASS", "current_prohibition": "PASS", "missing_witness": "PASS", "gapped_witness": "PASS", "source_retained": "PASS", "identity_preserved": "PASS", "cost_attempts": "PASS", "full_D1": "NOT_RUN", "production_DR": "NOT_RUN"}, "new_delete": "PENDING_USER_ENVIRONMENT_AND_EXACT_VERSION_CONFIRMATION"})
}

type adminTransport struct{ base http.RoundTripper }
type adminKey struct{}
type adminCall struct{ used bool }

func (t adminTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	call, ok := r.Context().Value(adminKey{}).(*adminCall)
	if !ok || call.used || r.URL.Scheme != "http" || r.URL.Host != "127.0.0.1:9000" || (r.Method != "PUT" && r.Method != "POST") || len(r.URL.Path) < 16 || r.URL.Path[:16] != "/minio/admin/v3/" {
		return nil, lc.ErrStorageScope
	}
	call.used = true
	f, e := os.OpenFile("/run/private/archive-admin-receipts.jsonl", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	id := uuid.New()
	if e = json.NewEncoder(f).Encode(map[string]any{"id": id, "fact": "START", "path": r.URL.Path, "at": time.Now().UTC()}); e != nil {
		return nil, e
	}
	if e = f.Sync(); e != nil {
		return nil, e
	}
	res, e := t.base.RoundTrip(r)
	status := 0
	if res != nil {
		status = res.StatusCode
	}
	je := json.NewEncoder(f).Encode(map[string]any{"id": id, "fact": "OBSERVATION", "http_status": status, "unknown": e != nil, "at": time.Now().UTC()})
	if je == nil {
		je = f.Sync()
	}
	if je != nil {
		if res != nil {
			res.Body.Close()
		}
		return nil, lc.ErrStorageUnknown
	}
	return res, e
}
func provision(ctx context.Context, disable bool) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	cs, e := copies(c)
	if e != nil {
		return e
	}
	admin, e := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if e != nil {
		return e
	}
	admin.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, ResponseHeaderTimeout: 10 * time.Second}})
	call := func(fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, adminKey{}, &adminCall{}))
	}
	if disable || os.Getenv("LIFECYCLE_ARCHIVE_MODE") == "enable" {
		return call(func(ctx context.Context) error {
			status := madmin.AccountDisabled
			if !disable {
				status = madmin.AccountEnabled
			}
			return admin.SetUserStatus(ctx, p.Access, status)
		})
	}
	statements := []any{map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:GetObjectVersion", "s3:GetObjectRetention"}, "Resource": []string{"arn:aws:s3:::" + cs[0].Namespace + "/" + cs[0].Key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": cs[0].ProviderVersion}}}, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectLegalHold"}, "Resource": []string{"arn:aws:s3:::" + cs[0].Namespace + "/" + cs[0].Key}}}
	for _, key := range keys {
		arn := "arn:aws:s3:::" + namespaces[1] + "/" + key
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:PutObject"}, "Resource": []string{arn}}, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:GetObjectVersion", "s3:GetObjectRetention"}, "Resource": []string{arn}, "Condition": map[string]any{"Null": map[string]string{"s3:versionid": "false"}}}, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectLegalHold"}, "Resource": []string{arn}})
	}
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
	name := "lifecycle307-copy-" + c.RunID.String()
	if e = call(func(ctx context.Context) error { return admin.AddCannedPolicy(ctx, name, policy) }); e != nil {
		r := madmin.ToErrorResponse(e)
		message := r.Message
		for _, secret := range []string{c.RootAccess, c.RootSecret, c.Token, c.InspectAccess, c.InspectSecret, p.Access, p.Secret} {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[redacted]")
			}
		}
		return fmt.Errorf("copy policy rejected/unknown (%s): %s; no automatic retry", r.Code, message)
	}
	if e = call(func(ctx context.Context) error { return admin.AddUser(ctx, p.Access, p.Secret) }); e != nil {
		return errors.New("copy identity provisioning unknown; no retry")
	}
	if e = call(func(ctx context.Context) error {
		_, e := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{name}, User: p.Access})
		return e
	}); e != nil {
		return errors.New("copy policy attachment unknown; no retry")
	}
	return write(filepath.Join(c.OutputDirectory, "archive-executor-policy.json"), json.RawMessage(policy))
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	mode := os.Getenv("LIFECYCLE_ARCHIVE_MODE")
	var e error
	switch mode {
	case "provision":
		e = provision(ctx, false)
	case "disable":
		e = provision(ctx, true)
	case "execute":
		e = execute(ctx)
	case "enable":
		e = provision(ctx, false)
	case "readback":
		e = readback(ctx)
	case "plan-source-delete":
		e = prepareSourceDeletePlan(ctx)
	default:
		e = errors.New("explicit archive mode required")
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Println("controlled", mode, "completed; DELETE=0; cleanup=0")
}
