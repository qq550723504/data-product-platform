package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

func TestRenewalPrivateHistoricalBindingsAndRealCLI(t *testing.T) {
	pool, c, r, base, old, _, s := renewalFixture(t, false)
	originalRepo := c.Repo
	c.Repo = t.TempDir()
	r.Root = c.Repo
	for ref := range base.Files {
		data, err := os.ReadFile(filepath.Join(originalRepo, filepath.FromSlash(ref)))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(c.Repo, filepath.FromSlash(ref))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(c.Private, 0700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'
$acl=[Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true,$false)
foreach($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User,[Security.Principal.SecurityIdentifier]::new('S-1-5-18'))){$acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow'))}
[IO.Directory]::SetAccessControl($env:DPP_RENEWAL_TEST_DIRECTORY,$acl)`)
		cmd.Env = append(os.Environ(), "DPP_RENEWAL_TEST_DIRECTORY="+c.Private)
		if cmd.Run() != nil {
			t.Fatal("private test ACL unavailable")
		}
	}
	if err := pilot.CheckCustodyPrivatePath(c.Private, true); err != nil {
		t.Fatal(err)
	}
	write := func(path string, value any) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r.ActivationBinding = fileBinding{Path: "original-activation", SHA256: strings.Repeat("b", 64)}
	dir := filepath.Join(c.Private, "business-preparations", "accepted")
	write(filepath.Join(dir, "core-plan.json"), base)
	manifest := preparationManifest{Contract: businessContract, Status: "PREPARED", RunID: r.RunID, Workspace: r.Workspace, Actor: r.Actor, Root: r.Root, Activation: r.ActivationBinding, CorePlanSHA256: old.PlanSHA256, Receipt: old}
	write(filepath.Join(dir, "manifest.json"), manifest)
	r.PreparationBinding = fileBinding{Path: filepath.Join(dir, "manifest.json"), SHA256: hash(encoded(manifest))}
	s.PreparationSHA256 = r.PreparationBinding.SHA256
	loaded, completed, parent, err := loadRenewalBasis(c, r, s)
	if err != nil || !reflect.DeepEqual(loaded, base) || !reflect.DeepEqual(completed, old) {
		t.Fatal("private history admission failed", err)
	}
	p := checkedRenewal(t, pool, c, r, s, loaded, completed, parent)
	bad := r
	bad.PreparationBinding.SHA256 = strings.Repeat("f", 64)
	if _, _, _, err = loadRenewalBasis(c, bad, s); err == nil {
		t.Fatal("altered historical binding admitted")
	}
	if privateDocument(c, filepath.Join(t.TempDir(), "foreign.json"), &renewalSpec{}) == nil {
		t.Fatal("foreign private path admitted")
	}
	// The full CLI requires the real retained database name. This subtest is
	// also exercised by the operator's isolated dpp_trial_core Windows harness;
	// ordinary CI databases still run the SQL and private binding assertions.
	t.Run("native-private-cli", func(t *testing.T) {
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		u, err := url.Parse(dsn)
		if err != nil || u.Path != "/dpp_trial_core" || u.Hostname() != "127.0.0.1" {
			t.Skip("isolated loopback dpp_trial_core CLI database required")
		}
		c.DSN = dsn
		configPath := filepath.Join(c.Private, "config.json")
		specPath := filepath.Join(c.Private, "renewal.json")
		write(configPath, c)
		write(specPath, s)
		write(filepath.Join(c.Repo, ".data/trial/state.json"), r)
		t.Setenv("DPP_TRIAL_CONFIG", configPath)
		before := oldRenewalRows(t, pool, c.Workspace)
		for i := 0; i < 2; i++ {
			read, writePipe, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			previous := os.Stdout
			os.Stdout = writePipe
			type output struct {
				data []byte
				err  error
			}
			done := make(chan output, 1)
			go func() { data, err := io.ReadAll(read); done <- output{data, err} }()
			err = run([]string{"renewal-plan", "-spec", specPath})
			os.Stdout = previous
			_ = writePipe.Close()
			out := <-done
			_ = read.Close()
			var result struct {
				Plan renewalPlan `json:"plan"`
				SHA  string      `json:"planSha256"`
			}
			if err != nil || out.err != nil || json.Unmarshal(out.data, &result) != nil || result.SHA != hash(encoded(p)) || !reflect.DeepEqual(result.Plan, p) {
				t.Fatal("real CLI did not reproduce reviewed plan", err, out.err)
			}
		}
		if !reflect.DeepEqual(before, oldRenewalRows(t, pool, c.Workspace)) {
			t.Fatal("readonly CLI changed Core facts")
		}
		raw := map[string]any{}
		if json.Unmarshal(encoded(s), &raw) != nil {
			t.Fatal("spec encoding")
		}
		raw["consumer"] = "FOREIGN"
		write(specPath, raw)
		if run([]string{"renewal-plan", "-spec", specPath}) == nil {
			t.Fatal("scope override accepted by strict CLI")
		}
	})
}

func renewalFixture(t *testing.T, expire bool) (*pgxpool.Pool, config, registry, plan, receipt, rightsWindow, renewalSpec) {
	t.Helper()
	pool, c, r, s := businessFixture(t)
	s.ValidFrom = time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Microsecond)
	s.ValidTo = time.Now().UTC().Add(5 * time.Second).Truncate(time.Microsecond)
	base, err := makePlan(context.Background(), pool, c, r, s)
	if err != nil {
		t.Fatal(err)
	}
	old, err := prepare(context.Background(), pool, base, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.PreparationBinding = fileBinding{Path: "isolated-manifest-not-loaded-by-domain-test", SHA256: strings.Repeat("c", 64)}
	parent := rightsWindow{PlanSHA256: old.PlanSHA256, Command: preparationCommand, ClaimKey: r.RunID, ValidFrom: s.ValidFrom, ValidTo: s.ValidTo, Objects: old.Objects}
	if expire {
		time.Sleep(time.Until(s.ValidTo) + time.Millisecond)
	}
	spec := renewalSpec{Contract: renewalContract, RenewalID: uuid.New(), RunID: r.RunID, Workspace: c.Workspace, Actor: c.Actor, PreparationSHA256: r.PreparationBinding.SHA256, ValidFrom: s.ValidTo, ValidTo: s.ValidTo.Add(time.Minute), Reason: "Explicit new finite synthetic rights window; same retained source basis."}
	return pool, c, r, base, old, parent, spec
}
func oldRenewalRows(t *testing.T, pool *pgxpool.Pool, ws uuid.UUID) map[string]bool {
	t.Helper()
	result := map[string]bool{}
	queries := map[string]string{
		"authorization": `SELECT to_jsonb(t)::text FROM data_authorization t WHERE workspace_id=$1`,
		"resources":     `SELECT to_jsonb(t)::text FROM authorization_resource t JOIN data_authorization a ON a.id=t.authorization_id WHERE a.workspace_id=$1`,
		"declaration":   `SELECT to_jsonb(t)::text FROM rights_declaration t WHERE workspace_id=$1`,
		"verification":  `SELECT to_jsonb(t)::text FROM rights_declaration_verification t JOIN rights_declaration d ON d.id=t.declaration_id WHERE d.workspace_id=$1`,
		"binding":       `SELECT to_jsonb(t)::text FROM authorization_provenance_binding t WHERE workspace_id=$1`,
		"claim":         `SELECT to_jsonb(t)::text FROM command_idempotency t WHERE workspace_id=$1`,
		"cost":          `SELECT to_jsonb(t)::text FROM cost_event t WHERE workspace_id=$1`,
		"evidence":      `SELECT to_jsonb(t)::text FROM evidence t WHERE workspace_id=$1`,
		"audit":         `SELECT to_jsonb(t)::text FROM audit_event t WHERE workspace_id=$1`,
		"profile":       `SELECT to_jsonb(t)::text FROM certification_profile t WHERE workspace_id=$1`,
		"contract":      `SELECT to_jsonb(t)::text FROM contract_version t JOIN data_contract c ON c.id=t.contract_id WHERE c.workspace_id=$1`,
	}
	for name, q := range queries {
		rows, err := pool.Query(context.Background(), q, ws)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var value string
			if err = rows.Scan(&value); err != nil {
				t.Fatal(err)
			}
			result[name+":"+value] = true
		}
		if rows.Err() != nil {
			t.Fatal(rows.Err())
		}
		rows.Close()
	}
	return result
}
func checkedRenewal(t *testing.T, pool *pgxpool.Pool, c config, r registry, s renewalSpec, base plan, old receipt, parent rightsWindow) renewalPlan {
	t.Helper()
	p, err := makeRenewalPlan(context.Background(), pool, c, r, s, base, old, parent)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestRenewalReadonlyScopeAndFiniteAdmission(t *testing.T) {
	pool, c, r, base, old, parent, s := renewalFixture(t, false)
	before := businessCounts(t, pool, c.Workspace)
	p := checkedRenewal(t, pool, c, r, s, base, old, parent)
	if !reflect.DeepEqual(p, checkedRenewal(t, pool, c, r, s, base, old, parent)) || !reflect.DeepEqual(before, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("renewal plan wrote facts or is unstable")
	}
	if p.Next.Authorization.Code == base.Authorization.Code || len(p.Next.Authorization.Code) > 64 || !reflect.DeepEqual(p.Next.Authorization.Resources, base.Authorization.Resources) || !reflect.DeepEqual(p.Next.Profile, base.Profile) || p.Next.ContractYAML != base.ContractYAML {
		t.Fatal("renewal widened/replaced scope or reusable facts")
	}
	for _, change := range []func(*renewalSpec){func(s *renewalSpec) { s.RenewalID = uuid.Nil }, func(s *renewalSpec) { s.Workspace = uuid.New() }, func(s *renewalSpec) { s.Actor = uuid.New() }, func(s *renewalSpec) { s.PreparationSHA256 = strings.Repeat("f", 64) }, func(s *renewalSpec) { s.ValidFrom = parent.ValidTo.Add(-time.Second) }, func(s *renewalSpec) { s.ValidTo = s.ValidFrom.Add(25 * time.Hour) }, func(s *renewalSpec) { s.ValidFrom = s.ValidFrom.Add(600 * time.Nanosecond) }, func(s *renewalSpec) { s.ValidTo = s.ValidTo.Add(100 * time.Nanosecond) }, func(s *renewalSpec) { s.ValidFrom = s.ValidFrom.In(time.FixedZone("non-UTC", 8*3600)) }} {
		bad := s
		change(&bad)
		if _, err := makeRenewalPlan(context.Background(), pool, c, r, bad, base, old, parent); err == nil {
			t.Fatal("invalid renewal admitted")
		}
	}
	if _, err := renewRights(context.Background(), pool, p, nil, nil); err == nil {
		t.Fatal("future renewal ran before predecessor expiry")
	}
	if !reflect.DeepEqual(before, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("rejected window wrote facts")
	}
	if len(p.Parent.PlanSHA256) != 64 || p.Parent.PlanSHA256 != old.PlanSHA256 {
		t.Fatal("predecessor was recomputed from new input")
	}
}
func TestRenewalCommittedStepRecoveryCostAndCurrentEntitlement(t *testing.T) {
	pool, c, r, base, old, parent, s := renewalFixture(t, true)
	ctx := context.Background()
	p := checkedRenewal(t, pool, c, r, s, base, old, parent)
	frozen := oldRenewalRows(t, pool, c.Workspace)
	recheck := func() error { _, err := makeRenewalPlan(ctx, pool, c, r, s, base, old, parent); return err }
	lost := errors.New("isolated response loss after command commit")
	for i := 0; i < 13; i++ {
		if _, err := renewRights(ctx, pool, p, recheck, func(string) error { return lost }); err != lost {
			t.Fatalf("step %d did not restore same physical activity: %v", i, err)
		}
	}
	result, err := renewRights(ctx, pool, p, recheck, nil)
	if err != nil {
		t.Fatal(err)
	}
	after := businessCounts(t, pool, c.Workspace)
	replayed, err := renewRights(ctx, pool, p, func() error { t.Fatal("completed replay re-entered Commands"); return nil }, nil)
	if err != nil || !reflect.DeepEqual(result, replayed) || !reflect.DeepEqual(after, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("replay changed facts", err)
	}
	retained := oldRenewalRows(t, pool, c.Workspace)
	for row := range frozen {
		if !retained[row] {
			t.Fatal("old preparation/history row changed")
		}
	}
	if after["data_authorization"] != 2 || after["rights_declaration"] != 6 || after["authorization_provenance_binding"] != 6 || after["cost_event"] != 12 || after["certification_profile"] != 1 || after["data_contract"] != 1 || after["execution"] != 1 || after["dataset_certification"] != 0 || after["delivery_operation"] != 0 {
		t.Fatal("renewal duplicated, lost costs or ran downstream work", after)
	}
	rights := rightsapp.NewService(transaction.NewManager(pool), rightsinfra.NewPostgresRepository(pool))
	for _, resource := range base.Resources {
		request := rightsdomain.EntitlementRequest{WorkspaceID: c.Workspace, AuthorizationID: result.Objects["authorization"], DataResourceID: resource, ConsumerRef: base.Spec.Consumer, Purpose: base.Spec.Purpose, Action: "READ", Scope: rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resource.String()}, Path: rightsdomain.EntitlementDownstream}
		decision, err := rights.CheckCurrentEntitlement(ctx, rightsapp.CheckCurrentEntitlementCommand{EntitlementRequest: request})
		if err != nil || decision.Decision != rightsdomain.DecisionAllowed {
			t.Fatal("new finite READ not current", decision, err)
		}
		request.AuthorizationID = old.Objects["authorization"]
		decision, err = rights.CheckCurrentEntitlement(ctx, rightsapp.CheckCurrentEntitlementCommand{EntitlementRequest: request})
		if err == nil && decision.Decision == rightsdomain.DecisionAllowed {
			t.Fatal("expired original authorization became current")
		}
	}
	bad := p
	bad.Spec.RenewalID = uuid.New()
	bad.Next.Authorization.Code = renewalCode(s.RunID, bad.Spec.RenewalID)
	beforeConflict := businessCounts(t, pool, c.Workspace)
	if _, err = renewRights(ctx, pool, bad, nil, nil); err == nil || !strings.Contains(err.Error(), "claimed") {
		t.Fatal("second successor accepted", err)
	}
	if !reflect.DeepEqual(beforeConflict, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("conflicting UUID wrote facts")
	}
	if _, err = rights.Revoke(ctx, rightsapp.TransitionCommand{AuthorizationID: result.Objects["authorization"], ActorID: &c.Actor}); err != nil {
		t.Fatal(err)
	}
	beforeReplay := businessCounts(t, pool, c.Workspace)
	if _, err = renewRights(ctx, pool, p, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeReplay, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("completed replay reactivated rights")
	}
}
func TestRenewalSameRunConcurrencyAndRevokedPredecessor(t *testing.T) {
	pool, c, r, base, old, parent, s := renewalFixture(t, true)
	ctx := context.Background()
	p := checkedRenewal(t, pool, c, r, s, base, old, parent)
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	done := make(chan error, 1)
	go func() {
		_, err := renewRights(ctx, pool, p, nil, func(string) error { once.Do(func() { close(started); <-release }); return nil })
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("renewal never reached first commit")
	}
	if _, err := renewRights(ctx, pool, p, nil, nil); !errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		t.Error("concurrent renewal entered", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	rights := rightsapp.NewService(transaction.NewManager(pool), rightsinfra.NewPostgresRepository(pool))
	if _, err := rights.Revoke(ctx, rightsapp.TransitionCommand{AuthorizationID: old.Objects["authorization"], ActorID: &c.Actor}); err != nil {
		t.Fatal(err)
	}
	if _, err := makeRenewalPlan(ctx, pool, c, r, s, base, old, parent); err == nil {
		t.Fatal("revoked original was treated as natural expiry")
	}
}
func TestRenewalLaterExplicitWindowUsesFixedCompletedParent(t *testing.T) {
	pool, c, r, base, old, parent, s := renewalFixture(t, true)
	ctx := context.Background()
	s.ValidTo = time.Now().UTC().Add(10 * time.Second).Truncate(time.Microsecond)
	p := checkedRenewal(t, pool, c, r, s, base, old, parent)
	first, err := renewRights(ctx, pool, p, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	nextParent := rightsWindow{PlanSHA256: first.PlanSHA256, Command: renewalCommand, ClaimKey: renewalKey(r.RunID, parent.PlanSHA256), ValidFrom: s.ValidFrom, ValidTo: s.ValidTo, Objects: first.Objects}
	nextSpec := s
	nextSpec.RenewalID = uuid.New()
	nextSpec.ValidFrom = s.ValidTo
	nextSpec.ValidTo = s.ValidTo.Add(time.Minute)
	next := checkedRenewal(t, pool, c, r, nextSpec, base, old, nextParent)
	time.Sleep(time.Until(s.ValidTo) + time.Millisecond)
	second, err := renewRights(ctx, pool, next, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.ParentPlanSHA256 != first.PlanSHA256 || second.Objects["authorization"] == first.Objects["authorization"] || second.Objects["contractVersion"] != old.Objects["contractVersion"] || second.Objects["profile"] != old.Objects["profile"] {
		t.Fatal("later window lost predecessor/reuse")
	}
	var originalClaim string
	if err = pool.QueryRow(ctx, `SELECT request_fingerprint FROM command_idempotency WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3`, c.Workspace, preparationCommand, r.RunID).Scan(&originalClaim); err != nil || originalClaim != old.PlanSHA256 {
		t.Fatal("original one-run claim overwritten")
	}
	var roundTrip renewalPlan
	var generic any
	_ = json.Unmarshal(encoded(next), &generic)
	_ = json.Unmarshal(encoded(generic), &roundTrip)
	if hash(encoded(next)) != hash(encoded(roundTrip)) {
		t.Fatal("cross-language renewal plan digest drift")
	}
}
