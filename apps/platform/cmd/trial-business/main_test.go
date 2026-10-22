package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

func testIdentity(t *testing.T) (config, registry, spec) {
	t.Helper()
	runID := strings.ReplaceAll(uuid.NewString(), "-", "")
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
	c := config{DSN: "postgres://postgres:unused@127.0.0.1:5432/dpp_trial_core?sslmode=disable", Repo: repo, Private: filepath.Join(t.TempDir(), "dpp-trial-"+runID), Workspace: uuid.New(), Actor: uuid.New()}
	c.Profile.Subject = "trial-operator-" + runID
	c.Profile.Bucket = "raw"
	r := registry{RunID: runID, Root: repo, Private: c.Private, Workspace: c.Workspace, Actor: c.Actor, Phase: "RUNNING", Ports: map[string]int{"core": 5432}}
	r.Activation.Status = "PASS"
	r.Activation.AttemptID = uuid.NewString()
	s := spec{Contract: businessContract, RunID: runID, Workspace: c.Workspace, Actor: c.Actor, ActivationAttempt: r.Activation.AttemptID, Consumer: "SYNTHETIC_CONSUMER", Purpose: purpose, ValidFrom: time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond), ValidTo: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond), Reason: "Synthetic source fixture authority only; no real-world ownership assertion."}
	for i, kind := range []string{"RAW", "RAW", "RAW", "STANDARDIZED", "CURATED"} {
		s.Versions = append(s.Versions, version{ID: uuid.New(), Kind: kind, SHA256: strings.Repeat("a", 64), URI: "s3://raw/datasets/" + strconv.Itoa(i) + ".csv", Bytes: 20, Rows: 1})
	}
	s.Target = s.Versions[4].ID
	return c, r, s
}

func TestIdentityAndScopeFailClosed(t *testing.T) {
	c, r, s := testIdentity(t)
	if err := validate(c, r, s); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*config, *registry, *spec){
		func(c *config, r *registry, s *spec) { s.Workspace = uuid.New() },
		func(c *config, r *registry, s *spec) { s.Actor = uuid.New() },
		func(c *config, r *registry, s *spec) { s.RunID = uuid.NewString() },
		func(c *config, r *registry, s *spec) { s.ActivationAttempt = "another" },
		func(c *config, r *registry, s *spec) { c.Profile.Subject = "foreign" },
		func(c *config, r *registry, s *spec) { c.DSN = "postgres://postgres:secret@remote:5432/dpp_trial_core" },
		func(c *config, r *registry, s *spec) { r.Phase = "UNKNOWN" },
		func(c *config, r *registry, s *spec) { s.Consumer = "ANY *" },
		func(c *config, r *registry, s *spec) { s.Purpose = "ANY" },
		func(c *config, r *registry, s *spec) { s.ValidTo = s.ValidFrom.Add(25 * time.Hour) },
		func(c *config, r *registry, s *spec) { s.Versions = s.Versions[:4] },
		func(c *config, r *registry, s *spec) { s.Target = s.Versions[0].ID },
	} {
		cc, rr, ss := c, r, s
		ss.Versions = append([]version(nil), s.Versions...)
		change(&cc, &rr, &ss)
		if validate(cc, rr, ss) == nil {
			t.Fatal("accepted changed identity/context")
		}
	}
	for _, args := range [][]string{{"seed"}, {"prepare"}, {"plan", "-spec", "x", "-plan", "y"}, {"prepare", "-spec", "x", "-plan", "y", "-sha256", "bad"}} {
		if run(args) == nil {
			t.Fatal("accepted unbounded CLI arguments")
		}
	}
}

// Only TEST_POSTGRES_DSN is used. CI supplies an isolated, migrated PostgreSQL;
// every fixture has a fresh workspace. No retained trial config is ever read.
func businessFixture(t *testing.T) (*pgxpool.Pool, config, registry, spec) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated migrated TEST_POSTGRES_DSN required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	c, r, s := testIdentity(t)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	c.DSN = "postgres://postgres:unused@127.0.0.1:" + u.Port() + "/dpp_trial_core"
	r.Ports["core"], _ = strconv.Atoi(u.Port())
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	datasets := []uuid.UUID{}
	resources := []uuid.UUID{}
	execution, workflow, wv := uuid.New(), uuid.New(), uuid.New()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO workflow(id,workspace_id,code,name) VALUES($1,$2,$3,'Synthetic existing workflow')`, workflow, c.Workspace, uuid.NewString())
	exec(`INSERT INTO workflow_version(id,workflow_id,version,definition_ref,definition_sha256,definition) VALUES($1,$2,'1.0.0','retained','`+strings.Repeat("a", 64)+`','{}')`, wv, workflow)
	for i, v := range s.Versions {
		dataset := uuid.New()
		datasets = append(datasets, dataset)
		var resource *uuid.UUID
		var producer *uuid.UUID
		if i < 3 {
			id := uuid.New()
			resource = &id
			resources = append(resources, id)
			exec(`INSERT INTO data_resource(id,workspace_id,code,name,resource_type,owner_id) VALUES($1,$2,$3,'Synthetic RAW','TABLE_LIKE',$4)`, id, c.Workspace, uuid.NewString(), c.Actor)
		}
		exec(`INSERT INTO dataset(id,workspace_id,code,name,dataset_type,source_resource_id,owner_id) VALUES($1,$2,$3,'Existing retained fixture',$4,$5,$6)`, dataset, c.Workspace, uuid.NewString(), v.Kind, resource, c.Actor)
		if i == 4 {
			producer = &execution
		}
		exec(`INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size,row_count,generated_by_execution_id) VALUES($1,$2,1,'READY','OBJECT_STORAGE',$3,'SHA256',$4,$5,$6,$7)`, v.ID, dataset, v.URI, v.SHA256, v.Bytes, v.Rows, producer)
	}
	exec(`INSERT INTO execution(id,workspace_id,workflow_version_id,output_dataset_id,output_dataset_version_id,target_period,status,attempt) VALUES($1,$2,$3,$4,$5,'2025-03','SUCCEEDED',1)`, execution, c.Workspace, wv, datasets[4], s.Target)
	for i := 0; i < 4; i++ {
		exec(`INSERT INTO execution_input(execution_id,input_name,dataset_version_id) VALUES($1,$2,$3)`, execution, "input"+strconv.Itoa(i), s.Versions[i].ID)
		exec(`INSERT INTO dataset_version_lineage(output_version_id,input_version_id,relation_type) VALUES($1,$2,'DERIVED_FROM')`, s.Target, s.Versions[i].ID)
	}
	exec(`INSERT INTO dataset_version_lineage(output_version_id,input_version_id,relation_type) VALUES($1,$2,'DERIVED_FROM')`, s.Versions[3].ID, s.Versions[0].ID)
	exec(`INSERT INTO execution_quarantine_record(execution_id,input_name,reason_code,payload) VALUES($1,'energy_raw','NEGATIVE_ENERGY_KWH','{"energy_kwh":"-999"}')`, execution)
	op, attempt, receipt, evidence := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txExec := func(q string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	txExec(`INSERT INTO ingestion_operation(id,workspace_id,actor_id,principal_ref,request_key,batch_key,connection_ref,instance_ref,config_hash,template_hash,schema_hash,request_fingerprint,state,remote_id,manifest,manifest_hash) VALUES($1,$2,$3,$4,'existing','2025-03','test','test',$5,$5,$5,$5,'PREPARED','retained','{}',$5)`, op, c.Workspace, c.Actor, c.Profile.Subject, strings.Repeat("a", 64))
	for i := 0; i < 3; i++ {
		txExec(`INSERT INTO ingestion_request_slot(operation_id,slot_no,code,dataset_id,resource_id,columns,row_count) VALUES($1,$2,$3,$4,$5,ARRAY['id'],1)`, op, i+1, "raw"+strconv.Itoa(i), datasets[i], resources[i])
	}
	txExec(`INSERT INTO ingestion_attempt(id,operation_id) VALUES($1,$2)`, attempt, op)
	txExec(`UPDATE ingestion_operation SET current_attempt_id=$2 WHERE id=$1`, op, attempt)
	for _, state := range []string{"REGISTERING", "REGISTERED", "STARTING", "SUCCEEDED"} {
		txExec(`UPDATE ingestion_operation SET state=$2 WHERE id=$1`, op, state)
	}
	txExec(`INSERT INTO evidence(id,workspace_id,evidence_type,title,source_type,hash_algorithm,hash_value) VALUES($1,$2,'INGESTION','Retained test batch','INGESTION_OPERATION','SHA256-EVIDENCE-V2',$3)`, evidence, c.Workspace, strings.Repeat("a", 64))
	txExec(`INSERT INTO ingestion_receipt(id,operation_id,attempt_id,manifest_hash,evidence_id) VALUES($1,$2,$3,$4,$5)`, receipt, op, attempt, strings.Repeat("a", 64), evidence)
	for i := 0; i < 3; i++ {
		v := s.Versions[i]
		txExec(`INSERT INTO ingestion_receipt_slot(receipt_id,code,dataset_id,version_id,storage_uri,content_hash) VALUES($1,$2,$3,$4,$5,$6)`, receipt, "raw"+strconv.Itoa(i), datasets[i], v.ID, v.URI, v.SHA256)
	}
	txExec(`UPDATE ingestion_operation SET state='ACCEPTED' WHERE id=$1`, op)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return pool, c, r, s
}

func businessCounts(t *testing.T, pool *pgxpool.Pool, ws uuid.UUID) map[string]int {
	t.Helper()
	result := map[string]int{}
	for _, table := range []string{"data_authorization", "rights_declaration", "authorization_provenance_binding", "certification_profile", "data_contract", "audit_event", "cost_event", "evidence", "command_idempotency", "quality_result", "compliance_result", "dataset_certification", "delivery_operation", "execution"} {
		var n int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table+` WHERE workspace_id=$1`, ws).Scan(&n); err != nil {
			t.Fatal(err)
		}
		result[table] = n
	}
	return result
}

func TestBusinessPlanRealReadonlyAndAdmission(t *testing.T) {
	pool, c, r, s := businessFixture(t)
	ctx := context.Background()
	before := businessCounts(t, pool, c.Workspace)
	p, err := makePlan(ctx, pool, c, r, s)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := makePlan(ctx, pool, c, r, s)
	if err != nil || !reflect.DeepEqual(p, p2) {
		t.Fatal("plan unstable", err)
	}
	if !reflect.DeepEqual(before, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("plan wrote Core")
	}
	if len(p.Resources) != 3 || strings.Contains(p.ContractYAML, "LICENSED_BANK") || !strings.Contains(p.ContractYAML, s.Consumer) {
		t.Fatal("contract widened consumer")
	}
	if p.Profile.Rights.Scopes.Mode != "EXPLICIT" || !reflect.DeepEqual(p.Profile.Actions.Values, []string{"READ"}) {
		t.Fatal("profile coverage widened")
	}
	if len(p.Declarations) != 3 || len(p.Authorization.Resources) != 3 || p.BindingAuthorityMode != rightsdomain.AuthorityDirect || p.Authorization.GrantorRef != "SYNTHETIC-TRIAL-"+s.RunID {
		t.Fatal("plan omits reviewed command authority")
	}
	for _, declaration := range p.Declarations {
		if declaration.ConsumerScopeType != "EXPLICIT" || len(declaration.Permissions) != 1 || declaration.Permissions[0].Kind != rightsdomain.PermissionGrant || declaration.Permissions[0].Action != "READ" || declaration.BasisType != "SYNTHETIC_TRIAL" {
			t.Fatal("reviewed declaration permission widened")
		}
	}
	// Round-trip exactly as Python sorted JSON output does: nested JSON keys
	// must not make the reviewed Go fingerprint depend on serializer whitespace.
	var generic any
	_ = json.Unmarshal(encoded(p), &generic)
	var roundTrip plan
	_ = json.Unmarshal(encoded(generic), &roundTrip)
	if hash(encoded(roundTrip)) != hash(encoded(p)) {
		t.Fatal("cross-language plan canonicalization differs")
	}
	for _, change := range []func(*spec){func(s *spec) { s.Versions[0].SHA256 = strings.Repeat("b", 64) }, func(s *spec) { s.Workspace = uuid.New() }, func(s *spec) { s.Versions[0].ID = uuid.New() }} {
		bad := s
		bad.Versions = append([]version(nil), s.Versions...)
		change(&bad)
		if _, err = makePlan(ctx, pool, c, r, bad); err == nil {
			t.Fatal("changed retained admission accepted")
		}
	}
}

func TestBusinessCommandsResumeReplayConflictAndNoReactivation(t *testing.T) {
	pool, c, r, s := businessFixture(t)
	ctx := context.Background()
	p, err := makePlan(ctx, pool, c, r, s)
	if err != nil {
		t.Fatal(err)
	}
	before := businessCounts(t, pool, c.Workspace)
	// Lost result after EACH real DB command commit, including the commands
	// without their own idempotency key. Each restart discovers its audit fact.
	fail := errors.New("simulated response loss after commit")
	for i := 0; i < 16; i++ {
		_, err = prepare(ctx, pool, p, func(string) error { return fail })
		if err != fail {
			t.Fatalf("step %d did not preserve unknown: %v", i, err)
		}
	}
	result, err := prepare(ctx, pool, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	after := businessCounts(t, pool, c.Workspace)
	replayed, err := prepare(ctx, pool, p, nil)
	if err != nil || !reflect.DeepEqual(result, replayed) || !reflect.DeepEqual(after, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("replay duplicated facts", err)
	}
	for _, table := range []string{"quality_result", "compliance_result", "dataset_certification", "delivery_operation", "execution"} {
		if before[table] != after[table] {
			t.Fatalf("preparation executed %s", table)
		}
	}
	if after["data_authorization"] != 1 || after["rights_declaration"] != 3 || after["authorization_provenance_binding"] != 3 || after["certification_profile"] != 1 || after["data_contract"] != 1 || after["cost_event"] != 6 {
		t.Fatal("unexpected prerequisites/costs", after)
	}
	check, err := makePlan(ctx, pool, c, r, s)
	if err != nil || !reflect.DeepEqual(p, check) {
		t.Fatal("original five versions/batch/execution/history changed", err)
	}
	bad := p
	bad.Spec.Consumer = "ANOTHER_CONSUMER"
	if _, err = prepare(ctx, pool, bad, nil); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatal("same key input changed", err)
	}
	if !reflect.DeepEqual(after, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("conflict wrote facts")
	}
	rights := rightsapp.NewService(transaction.NewManager(pool), rightsinfra.NewPostgresRepository(pool))
	// This is an isolated contract test of the downstream path, not another
	// preparation step. GRANT-only declarations must suffice without inventing
	// grantor USE rights, and must not authorize another consumer or action.
	for _, resource := range p.Resources {
		request := rightsdomain.EntitlementRequest{WorkspaceID: s.Workspace, AuthorizationID: result.Objects["authorization"], DataResourceID: resource, ConsumerRef: s.Consumer, Purpose: s.Purpose, Action: "READ", Scope: rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resource.String()}, Path: rightsdomain.EntitlementDownstream}
		decision, err := rights.CheckCurrentEntitlement(ctx, rightsapp.CheckCurrentEntitlementCommand{EntitlementRequest: request})
		if err != nil || decision.Decision != rightsdomain.DecisionAllowed {
			t.Fatal("scoped downstream READ unavailable", decision, err)
		}
		for _, bad := range []rightsdomain.EntitlementRequest{func() rightsdomain.EntitlementRequest { v := request; v.ConsumerRef = "OTHER_CONSUMER"; return v }(), func() rightsdomain.EntitlementRequest { v := request; v.Action = "RAW_EXPORT"; return v }()} {
			decision, err = rights.CheckCurrentEntitlement(ctx, rightsapp.CheckCurrentEntitlementCommand{EntitlementRequest: bad})
			if err == nil && decision.Decision == rightsdomain.DecisionAllowed {
				t.Fatal("broader entitlement accepted")
			}
		}
	}
	effective, err := rights.ComputeEffectiveRights(ctx, rightsapp.ComputeEffectiveRightsCommand{WorkspaceID: s.Workspace, TargetDatasetVersionID: s.Target, ConsumerRef: s.Consumer, Purpose: s.Purpose, ActorID: &s.Actor})
	if err != nil {
		t.Fatal("existing effective-rights Command cannot consume prerequisites", err)
	}
	if len(effective.Inputs) != 3 {
		t.Fatal("required source inputs lost")
	}
	for _, action := range effective.Actions {
		if (action.Action == "READ") != (action.Decision == rightsdomain.DecisionAllowed) {
			t.Fatal("effective-rights action intersection widened", action)
		}
	}
	if _, err = rights.Revoke(ctx, rightsapp.TransitionCommand{AuthorizationID: result.Objects["authorization"], ActorID: &s.Actor}); err != nil {
		t.Fatal(err)
	}
	afterRevoke := businessCounts(t, pool, c.Workspace)
	if _, err = prepare(ctx, pool, p, nil); err != nil {
		t.Fatal(err)
	}
	auth, err := rightsinfra.NewPostgresRepository(pool).GetAuthorization(ctx, result.Objects["authorization"])
	if err != nil || auth.Status != rightsdomain.StatusRevoked || !reflect.DeepEqual(afterRevoke, businessCounts(t, pool, c.Workspace)) {
		t.Fatal("replay reactivated revoked authorization", err)
	}
}

func TestBusinessConcurrentWritersSerialize(t *testing.T) {
	pool, c, r, s := businessFixture(t)
	ctx := context.Background()
	p, err := makePlan(ctx, pool, c, r, s)
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	done := make(chan error, 1)
	go func() {
		_, err := prepare(ctx, pool, p, func(string) error { once.Do(func() { close(started); <-release }); return nil })
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not reach first committed command")
	}
	_, err = prepare(ctx, pool, p, nil)
	if !errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		t.Error("second writer entered active preparation", err)
	}
	close(release)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if businessCounts(t, pool, c.Workspace)["data_authorization"] != 1 {
		t.Fatal("concurrent duplicate authorization")
	}
	// Claim metadata only, no direct domain-table writes are used by executor.
	var fingerprint string
	if err = pool.QueryRow(ctx, `SELECT request_fingerprint FROM command_idempotency WHERE workspace_id=$1 AND command_type='TRIAL.BUSINESS_PREREQUISITES'`, c.Workspace).Scan(&fingerprint); err != nil || fingerprint != hash(encoded(p)) {
		t.Fatal("claim not bound", err)
	}
}
