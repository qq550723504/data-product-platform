// A bounded local composition root for the human-confirmed #307 exact versions.
// No HTTP API, background retry, cleanup or business-policy defaults.
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

const run = "a6b30840-3e76-49ad-91d3-f0c399fcf170"
const planHash = "7c2847699997663ca453db54e0fab1565ad67fc9434c0c2c507c6adaa056a09f"

var planID = uuid.MustParse("83d0ad2c-0df0-4245-8fbf-6d7190bb77cc")
var inventoryPlanID = uuid.MustParse("de994857-c0aa-4f86-beb7-0012f06f7a4a")
var workspace = uuid.MustParse("39dd70e6-691a-4d69-b971-6c75533b72d6")

const inventoryHash = "a68edc4456d3153316d7a585b64818b1275300e090e0258e37f51a71b9118f04"

type config struct {
	RunID, WorkspaceID, ProviderInstanceID, IncarnationID, ActorID                                      uuid.UUID
	Endpoint, PostgresDSN, RootAccess, RootSecret, InspectAccess, InspectSecret, Token, OutputDirectory string
}
type permission struct {
	Enabled                                                        bool
	RunID, WorkspaceID                                             uuid.UUID
	PlanHash, DeleteAccess, DeleteSecret, BasisRef, TopologySHA256 string
	ConfirmedAt, ExpiresAt                                         time.Time
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
	if e != nil || len(b) > 128<<10 {
		return errors.New("private/input file unreadable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if e = d.Decode(v); e != nil {
		return errors.New("private/input file invalid")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing input")
	}
	return nil
}
func load() (config, permission, error) {
	var c config
	var p permission
	if read("/run/private/config.json", &c) != nil || read("/run/private/delete-permission.json", &p) != nil {
		return c, p, errors.New("controlled configuration unreadable")
	}
	if c.RunID.String() != run || p.RunID != c.RunID || c.WorkspaceID != workspace || p.WorkspaceID != workspace || p.PlanHash != planHash || c.Endpoint != "127.0.0.1:9000" || c.ProviderInstanceID.String() != "5e8a5758-96f6-4bf4-a088-1decd62809b0" || c.IncarnationID.String() != "78ebff25-faa6-4f9b-8275-6204d7b786ab" || c.ActorID == uuid.Nil || c.Token == "" || c.RootAccess == "" || c.InspectAccess == "" || p.DeleteAccess == "" || p.DeleteSecret == "" || p.BasisRef == "" || !p.Enabled || time.Now().Before(p.ConfirmedAt) || !time.Now().Before(p.ExpiresAt) || p.ExpiresAt.Sub(p.ConfirmedAt) > time.Hour || len(p.TopologySHA256) != 64 {
		return c, p, errors.New("controlled authority/scope/window unavailable")
	}
	return c, p, nil
}

type authority struct{}

func (authority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, _ time.Time) (principal.RequestPrincipal, error) {
	c, _, e := load()
	if e != nil {
		return principal.RequestPrincipal{}, principal.ErrUnauthenticated
	}
	r, e := principal.NewStaticResolver(true, c.Token, "lifecycle-fixture:"+run, c.ActorID.String(), []string{workspace.String()}, []string{lc.CapabilityApproveDisposition, lc.CapabilityDispatch, lc.CapabilityInspectStorage, lc.CapabilityInspectOccupancy, lc.CapabilityVerifyAbsence})
	if e != nil {
		return principal.RequestPrincipal{}, e
	}
	req, _ := http.NewRequest("POST", "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return r.Resolve(req, w, cap)
}
func (authority) CheckApproverAuthority(_ context.Context, _ pgx.Tx, a domain.DispositionApproval, at time.Time) (domain.Check, error) {
	c, p, e := load()
	if e != nil {
		return domain.Denied, nil
	}
	if a.ActorID != c.ActorID || a.PrincipalRef != "lifecycle-fixture:"+run || a.PlanID != planID || a.PlanHash != planHash || a.BasisRef != p.BasisRef || !at.Before(p.ExpiresAt) {
		return domain.Denied, nil
	}
	return domain.Satisfied, nil
}

type dependencies struct {
	all        []domain.Copy
	protection map[uuid.UUID]lc.CopyProtection
	inventory  map[string]lc.NamespaceInventory
}

func (d *dependencies) EvaluateDisposalDependencies(ctx context.Context, tx pgx.Tx, r lc.DisposalDependencyRequest) (lc.DisposalDependencies, error) {
	_, permission, e := load()
	if e != nil {
		return lc.DisposalDependencies{}, e
	}
	result := lc.DisposalDependencies{WorkspaceID: workspace, PlanID: r.Plan.ID, CopyID: r.Copy.ID, PlanHash: r.Plan.ContentHash, ActorID: r.Caller.ActorID, CheckedAt: r.AsOf, DependencyRevision: r.DependencyRevision, InventoryComplete: domain.Unknown, ReferencesReleased: domain.Unknown, ProviderRetentionAllows: domain.Unknown, LedgerCurrent: domain.Unknown, WindowMode: domain.UseDeadline}
	var policy, startEvent uuid.UUID
	var starts, retain, due time.Time
	e = tx.QueryRow(ctx, `SELECT id,start_event_id,starts_at,retain_until,dispose_due_at FROM lifecycle_fixture_disposal_basis WHERE run_id=$1 AND workspace_id=$2 AND approved_plan_id=$3 AND approved_hash=$4 AND inventory_hash=$5`, uuid.MustParse(run), workspace, planID, planHash, inventoryHash).Scan(&policy, &startEvent, &starts, &retain, &due)
	if e != nil {
		return result, e
	}
	result.Retention = []domain.RetentionBoundary{{PolicyID: policy, StartEventID: startEvent, StartsAt: starts, RetainUntil: retain}}
	result.DisposalDueAt = due
	inventory, ok := d.inventory[r.Copy.Namespace]
	protection, pok := d.protection[r.Copy.ID]
	until := permission.ExpiresAt
	if ok && pok {
		for _, observed := range []time.Time{inventory.ObservedAt, protection.ObservedAt} {
			cap := observed.Add(time.Minute)
			if cap.Before(until) {
				until = cap
			}
			if observed.After(r.AsOf) {
				return result, lc.ErrStorageContract
			}
		}
		if protection.Copy == r.Copy && !protection.LegalHold && (protection.RetentionMode == "NONE" || (protection.RetainUntil != nil && !r.AsOf.Before(*protection.RetainUntil))) {
			result.ProviderRetentionAllows = domain.Satisfied
		} else {
			result.ProviderRetentionAllows = domain.Denied
		}
		// Compare the complete native namespace set with all registered identities.
		// Prior successful exact-version receipts are the only allowed removals.
		expected := map[string]domain.Copy{}
		for _, c := range d.all {
			if c.Namespace != r.Copy.Namespace {
				continue
			}
			var absent bool
			e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_copy_absence_receipt WHERE workspace_id=$1 AND copy_id=$2)`, workspace, c.ID).Scan(&absent)
			if e != nil {
				return result, e
			}
			if !absent {
				expected[c.Key+"\x00"+c.ProviderVersion] = c
			}
		}
		complete := len(inventory.Versions) == len(expected)
		for _, v := range inventory.Versions {
			c, found := expected[v.Key+"\x00"+v.Version]
			if !found || v.DeleteMarker || v.ByteSize != c.ByteSize {
				complete = false
			}
		}
		if complete && inventory.WorkspaceID == workspace && inventory.ProviderInstanceID == r.Copy.ProviderInstanceID && inventory.IncarnationID == r.Copy.IncarnationID {
			result.InventoryComplete = domain.Satisfied
		}
		// Require the persisted, physically invoked inspection receipts, not a caller
		// ALLOWED flag. Native unset-lock 400 has already been strictly parsed.
		ids := append([]uuid.UUID{inventory.CallID}, protection.CallIDs...)
		var successful int
		e = tx.QueryRow(ctx, `SELECT count(DISTINCT c.id) FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.workspace_id=$1 AND c.id=ANY($2) AND o.invoked_at IS NOT NULL AND o.failure='' AND o.http_status IN (200,400)`, workspace, ids).Scan(&successful)
		if e != nil {
			return result, e
		}
		if successful == 3 {
			result.LedgerCurrent = domain.Satisfied
		}
	}
	result.ValidUntil = &until
	// Closed synthetic content must not have escaped to any Core dependent fact.
	// Discover every typed FK to dataset_version.id; a new such dependency blocks
	// without maintaining an incomplete hand-written list of module tables.
	check, _, e := lc.InspectFixtureReferences(ctx, tx, r.Copy)
	if e != nil {
		return result, e
	}
	released := check == domain.Satisfied
	var countCopies int
	e = tx.QueryRow(ctx, `SELECT count(*) FROM lifecycle_fixture_copy WHERE workspace_id=$1`, workspace).Scan(&countCopies)
	if e != nil {
		return result, e
	}
	if released && countCopies == len(d.all) {
		result.ReferencesReleased = domain.Satisfied
	}
	return result, nil
}

func copies(c config) ([]domain.Copy, error) {
	var m manifest
	if e := read(filepath.Join(c.OutputDirectory, "manifest.json"), &m); e != nil {
		return nil, e
	}
	if len(m.Objects) != 4 {
		return nil, lc.ErrStorageScope
	}
	expected := map[string]string{"af54c279-d207-46a3-9458-79cd2b26c8ac": "cc359f61-4921-4487-852f-b6ab6d139278", "629c9e3d-730a-4ce8-bd47-d7bbe2675535": "2ae6479b-2b96-441a-9159-80a07e3ec4e0", "3a53a9e2-c1f4-4ce1-bacb-849596023e9c": "70e379f9-c782-41ec-bc71-12c8de2b1538", "e74b1dcb-e030-4a84-978e-64575e9f7a8a": "0d09ec19-c3c9-45e3-afbe-2d07068954e2"}
	var all []domain.Copy
	for _, o := range m.Objects {
		if expected[o.CopyID.String()] != o.Version {
			return nil, lc.ErrStorageScope
		}
		delete(expected, o.CopyID.String())
		copy := domain.Copy{ID: o.CopyID, WorkspaceID: workspace, DatasetVersionID: o.DatasetVersionID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: o.Namespace, Key: o.Key, ProviderVersion: o.Version, ContentSHA256: o.SHA256, ByteSize: o.Bytes}
		if copy.Validate() != nil {
			return nil, lc.ErrStorageScope
		}
		all = append(all, copy)
	}
	return all, nil
}
func write(path string, v any) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if e = enc.Encode(v); e != nil {
		return e
	}
	return f.Sync()
}

func execute(ctx context.Context) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	path := filepath.Join(c.OutputDirectory, "delete-acceptance.json")
	if _, e = os.Stat(path); !os.IsNotExist(e) {
		return errors.New("acceptance exists: do not resend or recreate")
	}
	all, e := copies(c)
	if e != nil {
		return e
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return errors.New("database configuration failed")
	}
	defer pool.Close()
	manager := transaction.NewManager(pool)
	repo := &infrastructure.PostgresRepository{}
	// Validate the frozen plan and complete registry before any provider request.
	var approved, full domain.Plan
	e = manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e error
		approved, e = repo.GetPlan(ctx, tx, workspace, planID, false)
		if e != nil {
			return e
		}
		full, e = repo.GetPlan(ctx, tx, workspace, inventoryPlanID, false)
		if e != nil {
			return e
		}
		if approved.VerifyFrozenScope() != nil || approved.ContentHash != planHash || full.VerifyFrozenScope() != nil || full.ContentHash != inventoryHash {
			return lc.ErrStorageScope
		}
		for _, c := range all {
			found := false
			for _, v := range full.Targets {
				if c == v {
					found = true
				}
			}
			if !found {
				return lc.ErrStorageScope
			}
		}
		return nil
	})
	if e != nil {
		return errors.New("frozen registry scope mismatch")
	}
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	auth := authority{}
	occupancy := lc.NewOccupancyService(manager, repo, auth, nil, nil)
	dep := &dependencies{all: all, protection: map[uuid.UUID]lc.CopyProtection{}, inventory: map[string]lc.NamespaceInventory{}}
	disposition := lc.NewDispositionService(occupancy, auth, dep, nil)
	execution := lc.NewExactExecution(disposition)
	inspector, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: workspace, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: lc.NewPostgresStorageCallJournal(occupancy)})
	if e != nil {
		return e
	}
	header := func(key string) lc.CommandHeader {
		return lc.CommandHeader{WorkspaceID: workspace, IdempotencyKey: run + ":" + key}
	}
	basis, e := execution.RegisterFixtureDisposalBasis(ctx, lc.FixtureDisposalBasisCommand{CommandHeader: header("basis"), RunID: uuid.MustParse(run), InventoryPlanID: inventoryPlanID, ApprovedPlanID: planID, InventoryHash: inventoryHash, ApprovedHash: planHash, BasisRef: p.BasisRef})
	if e != nil {
		return fmt.Errorf("test-only policy basis: %w", e)
	}
	approval, e := disposition.ApproveContentDisposition(ctx, lc.ApproveContentDispositionCommand{CommandHeader: header("approve"), PlanID: planID, BasisRef: p.BasisRef, ExpiresAt: p.ExpiresAt})
	if e != nil {
		return fmt.Errorf("scope approval: %w", e)
	}
	refresh := func(copy domain.Copy) error {
		protection, e := inspector.InspectRetentionAndHold(ctx, copy)
		if e != nil {
			return e
		}
		dep.protection[copy.ID] = protection
		inventory, e := inspector.InspectNamespace(ctx, copy)
		if e != nil {
			return e
		}
		dep.inventory[copy.Namespace] = inventory
		return nil
	}
	// Read-only platform prerequisite checks for the two protected members. No
	// approval is created for their four-object inventory plan; no dispatch/send.
	var blocked []any
	for _, copy := range all {
		if copy.ID == approved.Targets[0].ID || copy.ID == approved.Targets[1].ID {
			continue
		}
		if e = refresh(copy); e != nil {
			return fmt.Errorf("protected inspection: %w", e)
		}
		protection := dep.protection[copy.ID]
		if !protection.LegalHold && (protection.RetainUntil == nil || !time.Now().Before(*protection.RetainUntil)) {
			return errors.New("expected protected object protection is no longer active")
		}
		decision := domain.EvaluateDeletePrerequisites(domain.DeletePrerequisites{AsOf: time.Now().UTC(), Plan: full, ProviderRetentionAllows: domain.Denied})
		blocked = append(blocked, map[string]any{"copy": copy, "protection": protection, "decision": decision, "native_delete_requests": 0, "approval": "NOT_GRANTED", "check_kind": "DOMAIN_PREREQUISITE_READ_ONLY"})
	}
	if e = write(filepath.Join(c.OutputDirectory, "protected-block-readback-"+uuid.NewString()+".json"), blocked); e != nil {
		return e
	}
	// Only confirmed rejected sends may resolve by a newly journaled positive
	// HEAD. Uncertain/in-flight sends retain their reservation, without resend.
	for _, target := range approved.Targets {
		rows, err := pool.Query(ctx, `SELECT a.id FROM lifecycle_provider_attempt a JOIN lifecycle_provider_observation o ON o.attempt_id=a.id WHERE a.copy_id=$1 AND a.plan_id=$2 AND o.certainty='CONFIRMED' AND o.outcome='DENIED' AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=a.dispatch_id)`, target.ID, planID)
		if err != nil {
			return err
		}
		var attempts []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			attempts = append(attempts, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range attempts {
			presence, err := inspector.InspectExactCopy(ctx, target)
			if err != nil {
				return err
			}
			resolution, err := execution.ReconcileDeniedAttempt(ctx, header("reconcile-denied:"+id.String()), id, presence)
			if err != nil {
				return err
			}
			if err = write(filepath.Join(c.OutputDirectory, "denied-reconciliation-"+id.String()+".json"), map[string]any{"attempt_id": id, "presence": presence, "resolution": resolution, "delete_requests": 0}); err != nil {
				return err
			}
		}
	}
	deleter, e := storage.NewLifecycleDeleter(storage.LifecycleDeleteConfig{Endpoint: c.Endpoint, AccessKey: p.DeleteAccess, SecretKey: p.DeleteSecret, Targets: approved.Targets, Journal: execution})
	if e != nil {
		return e
	}
	var results []any
	for _, copy := range approved.Targets {
		if e = refresh(copy); e != nil {
			return fmt.Errorf("fresh target inspection: %w", e)
		}
		dispatch, e := disposition.DispatchContentDisposition(ctx, lc.DispatchContentDispositionCommand{CommandHeader: header("dispatch:" + copy.ID.String() + ":" + os.Getenv("LIFECYCLE_DELETE_EVALUATION")), PlanID: planID, ApprovalID: approval.ID, CopyID: copy.ID})
		if e != nil {
			return fmt.Errorf("dispatch: %w", e)
		}
		// A replay only performs read-only reconciliation; it never authorizes send.
		if len(dispatch.Blockers) != 0 {
			return fmt.Errorf("target blocked: %v", dispatch.Blockers)
		}
		var attempt domain.ProviderAttempt
		e = manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var e error
			attempt, e = repo.GetAttempt(ctx, tx, workspace, dispatch.AttemptID, false)
			return e
		})
		if e != nil {
			return e
		}
		deleteOutcome := "RECONCILIATION_ONLY"
		if dispatch.Admitted {
			if e = deleter.DeleteExactCopy(ctx, attempt, copy); e != nil {
				deleteOutcome = "UNKNOWN_OR_REJECTED"
			} else {
				deleteOutcome = "PROVIDER_ACCEPTED_ABSENCE_UNVERIFIED"
			}
		}
		inventory, e := inspector.InspectNamespace(ctx, copy)
		if e != nil {
			return fmt.Errorf("post-send inventory: %w", e)
		}
		// Reject extra objects, marker substitution and disappearance of the protected
		// counterpart, in addition to checking the approved exact target absent.
		survivor := 0
		for _, other := range all {
			if other.Namespace != copy.Namespace || other.ID == copy.ID {
				continue
			}
			for _, v := range inventory.Versions {
				if v.Key == other.Key && v.Version == other.ProviderVersion && v.ByteSize == other.ByteSize && !v.DeleteMarker {
					survivor++
				}
			}
		}
		if len(inventory.Versions) != 1 || survivor != 1 {
			return errors.New("post-send namespace exact membership unavailable; preserve UNKNOWN, no resend")
		}
		receipt, e := execution.VerifyAbsentInScope(ctx, header("verify:"+copy.ID.String()), attempt.ID, inventory)
		if e != nil {
			return fmt.Errorf("absence receipt: %w", e)
		}
		results = append(results, map[string]any{"copy": copy, "dispatch": dispatch, "attempt": attempt, "delete_outcome": deleteOutcome, "inventory": inventory, "receipt": receipt})
	}
	completion, e := execution.FinalizeContentDisposition(ctx, header("finalize"), planID)
	if e != nil {
		return fmt.Errorf("scope completion: %w", e)
	}
	// Fresh final read-only protected state plus exact inventories after all sends.
	var protectedFinal []any
	for _, copy := range all {
		if copy.ID == approved.Targets[0].ID || copy.ID == approved.Targets[1].ID {
			continue
		}
		if e = refresh(copy); e != nil {
			return e
		}
		if _, e = inspector.InspectExactCopy(ctx, copy); e != nil {
			return e
		}
		protection := dep.protection[copy.ID]
		if !protection.LegalHold && (protection.RetainUntil == nil || !time.Now().Before(*protection.RetainUntil)) {
			return errors.New("protected final state changed")
		}
		protectedFinal = append(protectedFinal, map[string]any{"copy": copy, "protection": protection, "inventory": dep.inventory[copy.Namespace]})
	}
	return write(path, map[string]any{"as_of": time.Now().UTC(), "run_id": run, "workspace_id": workspace, "plan_id": planID, "scope_hash": planHash, "human_confirmation": p.BasisRef, "fixture_test_policy_basis": basis, "approval": approval, "results": results, "completion": completion, "protected_final": protectedFinal, "acceptance": "PASS_SCOPED_EXACT_VERSION_ABSENCE", "object_cleanup": 0, "volume_cleanup": 0, "protected_delete_requests": 0, "excluded": "provider internal media/backups/replicas, external recipients", "full_D1": "NOT_RUN"})
}

// IAM provisioning is separate infrastructure preparation, using root only for
// its native admin API. This transport cannot send object DELETE or payload GET.
type adminTransport struct{ base http.RoundTripper }
type adminKey struct{}
type adminCall struct{ used bool }

func (t adminTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	call, ok := r.Context().Value(adminKey{}).(*adminCall)
	if !ok || call.used || r.URL.Scheme != "http" || r.URL.Host != "127.0.0.1:9000" || (r.Method != "PUT" && r.Method != "POST") || len(r.URL.Path) < 16 || r.URL.Path[:16] != "/minio/admin/v3/" {
		return nil, lc.ErrStorageScope
	}
	call.used = true
	path := "/run/private/executor-admin-receipts.jsonl"
	f, e := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
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
	response, e := t.base.RoundTrip(r)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	journalErr := json.NewEncoder(f).Encode(map[string]any{"id": id, "fact": "OBSERVATION", "http_status": status, "at": time.Now().UTC(), "unknown": e != nil})
	if journalErr == nil {
		journalErr = f.Sync()
	}
	if journalErr != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, lc.ErrStorageUnknown
	}
	return response, e
}
func provision(ctx context.Context, disable bool) error {
	c, p, e := load()
	if e != nil {
		return e
	}
	admin, e := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if e != nil {
		return e
	}
	admin.SetCustomTransport(adminTransport{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, ResponseHeaderTimeout: 10 * time.Second}})
	call := func(fn func(context.Context) error) error {
		return fn(context.WithValue(ctx, adminKey{}, &adminCall{}))
	}
	if disable || os.Getenv("LIFECYCLE_DELETE_MODE") == "enable" {
		return call(func(ctx context.Context) error {
			status := madmin.AccountDisabled
			if !disable {
				status = madmin.AccountEnabled
			}
			return admin.SetUserStatus(ctx, p.DeleteAccess, status)
		})
	}
	statements := []any{}
	for _, target := range []struct{ bucket, key, version string }{{"dpp-lifecycle307-online", "fixture/a.csv", "cc359f61-4921-4487-852f-b6ab6d139278"}, {"dpp-lifecycle307-quarantine", "fixture/b.csv", "2ae6479b-2b96-441a-9159-80a07e3ec4e0"}} {
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:DeleteObject", "s3:DeleteObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + target.bucket + "/" + target.key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": target.version}}})
	}
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
	name := "lifecycle307-exact-delete-" + run
	if e = call(func(ctx context.Context) error { return admin.AddCannedPolicy(ctx, name, policy) }); e != nil {
		response := madmin.ToErrorResponse(e)
		message := response.Message
		for _, secret := range []string{c.RootAccess, c.RootSecret, c.InspectAccess, c.InspectSecret, c.Token, p.DeleteAccess, p.DeleteSecret} {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
		return fmt.Errorf("native exact-version IAM policy rejected (%s): %.512s; no deletion", response.Code, message)
	}
	if os.Getenv("LIFECYCLE_DELETE_MODE") == "policy" {
		return write(filepath.Join(c.OutputDirectory, "delete-executor-policy-v5.json"), json.RawMessage(policy))
	}
	if e = call(func(ctx context.Context) error {
		return admin.SetUser(ctx, p.DeleteAccess, p.DeleteSecret, madmin.AccountEnabled)
	}); e != nil {
		return errors.New("executor IAM creation unknown; no retry")
	}
	if e = call(func(ctx context.Context) error {
		_, e := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{name}, User: p.DeleteAccess})
		return e
	}); e != nil {
		return errors.New("executor policy attachment unknown; no retry")
	}
	c.OutputDirectory = filepath.Clean(c.OutputDirectory)
	return write(filepath.Join(c.OutputDirectory, "delete-executor-policy.json"), json.RawMessage(policy))
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	mode := os.Getenv("LIFECYCLE_DELETE_MODE")
	var e error
	switch mode {
	case "provision":
		e = provision(ctx, false)
	case "disable":
		e = provision(ctx, true)
	case "enable":
		e = provision(ctx, false)
	case "policy":
		e = provision(ctx, false)
	case "execute":
		e = execute(ctx)
	default:
		e = errors.New("explicit mode required")
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	fmt.Println("controlled", mode, "completed; no cleanup")
}
