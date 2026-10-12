package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	me "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

type retrySchemaObserver struct {
	plan       d.SchemaPlan
	err        error
	externalID string
	resultHash string
	stopErr    error
	scanErr    error
	onScan     func()
	onStartup  func()
	stops      int
	scans      int
	sourceIO   int
}

func (o *retrySchemaObserver) Plan(context.Context) (d.SchemaPlan, error) { return o.plan, o.err }
func (o *retrySchemaObserver) Prepare(context.Context, uuid.UUID, d.SchemaPlan) (json.RawMessage, error) {
	return json.RawMessage(`{"fixture":"controlled-observer-runtime"}`), nil
}
func (o *retrySchemaObserver) Stop(context.Context, uuid.UUID, json.RawMessage) error {
	o.stops++
	return o.stopErr
}
func (o *retrySchemaObserver) Scan(_ context.Context, _ uuid.UUID, _ json.RawMessage, started, authorizeSourceIO func() error) (me.SourceSchema, error) {
	o.scans++
	if err := started(); err != nil {
		return me.SourceSchema{}, err
	}
	if o.onStartup != nil {
		o.onStartup()
	}
	if o.scanErr != nil {
		return me.SourceSchema{}, o.scanErr
	}
	if err := authorizeSourceIO(); err != nil {
		return me.SourceSchema{}, err
	}
	o.sourceIO++
	if o.onScan != nil {
		o.onScan()
	}
	now := time.Now().UTC()
	return me.SourceSchema{ExternalID: o.externalID, ExternalFQN: o.plan.ExternalFQN, ConnectorHash: o.resultHash, Columns: o.plan.Columns, StartedAt: now, CompletedAt: now}, nil
}

type schemaTestEntitlements struct {
	expected d.SchemaPlan
	checks   int
}

func (c *schemaTestEntitlements) CheckCurrentEntitlementTx(_ context.Context, _ pgx.Tx, r rd.EntitlementRequest) (rd.EntitlementDecision, error) {
	c.checks++
	if r.WorkspaceID != c.expected.WorkspaceID || r.AuthorizationID != c.expected.AuthorizationID || r.DataResourceID != c.expected.ResourceID || r.ConsumerRef != c.expected.ConsumerRef || r.Purpose != c.expected.Purpose || r.Action != "READ" || r.Path != rd.EntitlementDownstream || r.Scope.Type != "ALL_RESOURCE" || r.Scope.Ref != c.expected.ResourceID.String() {
		return rd.EntitlementDecision{}, errors.New("entitlement context changed")
	}
	return rd.EntitlementDecision{Decision: "ALLOWED"}, nil
}

// Real PostgreSQL transaction/replay regression, with controlled Port responses.
// It proves no provider acceptance or real authorization; those have separate evidence.
func schemaRetryFixture(t *testing.T) (context.Context, *pgxpool.Pool, d.SchemaRequest, *retrySchemaObserver, *schemaTestEntitlements, *SchemaService) {
	t.Helper()
	router, err := routing.NewRouter(true)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	t.Cleanup(func() { outbox.ConfigureAppendObligation(nil) })
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	p := d.SchemaPlan{Enabled: true, WorkspaceID: uuid.New(), ResourceID: uuid.New(), ActorID: uuid.New(), AuthorizationID: uuid.New(), ConsumerRef: "TEST", Purpose: "TEST", ExternalFQN: "test." + uuid.NewString(), RuleRef: "test/schema", RuleVersion: "1", RuleHash: strings.Repeat("a", 64), ConnectorHash: strings.Repeat("b", 64), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'profile recovery test','TABLE_LIKE')`, p.ResourceID, p.WorkspaceID, p.ResourceID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, p.AuthorizationID, p.WorkspaceID, p.AuthorizationID.String()); err != nil {
		t.Fatal(err)
	}
	repo := infra.NewPostgresRepository(pool)
	b, err := repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: p.ResourceID, Provider: "TEST_METADATA", EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: p.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	req, _, err := repo.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: p, PlanHash: d.Hash(p), BindingID: b.ID, ExternalID: b.ExternalID, Key: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	observer := &retrySchemaObserver{plan: p, externalID: b.ExternalID, resultHash: p.ConnectorHash}
	gate := &schemaTestEntitlements{expected: p}
	s := &SchemaService{Service: NewService("TEST_METADATA", "", transaction.NewManager(pool), repo, nil, nil), Observer: observer, Rights: gate}
	return ctx, pool, req, observer, gate, s
}

func TestSourceSchemaProfileFailureRetriesSameRequest(t *testing.T) {
	ctx, pool, req, observer, gate, s := schemaRetryFixture(t)
	p := req.Plan
	var err error
	failure := errors.New("profile temporarily unreadable")
	observer.err = failure
	if err = s.Process(ctx, req.ID); !errors.Is(err, failure) || errors.Is(err, ErrSchemaDenied) {
		t.Fatalf("profile failure became denial: %v", err)
	}
	if _, err = s.Request(ctx, p.ResourceID, p.WorkspaceID, p.ActorID, req.Key); !errors.Is(err, failure) {
		t.Fatalf("request load failure: %v", err)
	}
	if err = s.Handle(ctx, p.ResourceID, p.WorkspaceID, p.ActorID, uuid.New(), "handle", "reason"); !errors.Is(err, failure) {
		t.Fatalf("handling load failure: %v", err)
	}
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&n); err != nil || n != 0 || observer.scans != 0 {
		t.Fatalf("unavailability appended terminal or launched work: %d %v", n, err)
	}
	observer.err = nil
	if err = s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status); err != nil || status != "MATCH" {
		t.Fatalf("same-request recovery: %s %v", status, err)
	}
	if err = s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if observer.scans != 1 || gate.checks != 3 {
		t.Fatalf("work/gates duplicated: scans=%d checks=%d", observer.scans, gate.checks)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation a JOIN source_schema_attempt s ON s.id=a.source_schema_attempt_id WHERE s.request_id=$1`, req.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cost count: %d %v", n, err)
	}
	// A valid, loaded disabled plan is still a terminal authority denial.
	other, err := s.Request(ctx, p.ResourceID, p.WorkspaceID, p.ActorID, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	observer.plan.Enabled = false
	if err = s.Process(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM source_schema_observation WHERE request_id=$1`, other.ID).Scan(&status); err != nil || status != "DENIED" || observer.scans != 1 {
		t.Fatalf("disabled plan not denied: %s %v", status, err)
	}
}

func TestSourceSchemaResultUsesFrozenConnectorContract(t *testing.T) {
	for _, hash := range []string{"", strings.Repeat("c", 64)} {
		t.Run("result hash "+hash, func(t *testing.T) {
			ctx, pool, req, observer, gate, s := schemaRetryFixture(t)
			// Both current-plan gates see A, but the scanner returns contract B
			// (or no identity). Model the profile A -> B -> A rotation at the Port.
			observer.resultHash = hash
			if err := s.Process(ctx, req.ID); err == nil {
				t.Fatal("foreign connector result accepted")
			}
			var status, schemaHash string
			if err := pool.QueryRow(ctx, `SELECT status,schema_hash FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status, &schemaHash); err != nil || status != "UNAVAILABLE" || schemaHash != "" {
				t.Fatalf("wrong contract became source evidence: %s %s %v", status, schemaHash, err)
			}
			observer.resultHash = req.Plan.ConnectorHash
			if err := s.Process(ctx, req.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.Process(ctx, req.ID); err != nil {
				t.Fatal(err)
			}
			var costs, observations int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, req.ID).Scan(&costs); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_schema_observation WHERE request_id=$1 AND status='MATCH'`, req.ID).Scan(&observations); err != nil {
				t.Fatal(err)
			}
			if observer.scans != 2 || gate.checks != 6 || costs != 2 || observations != 1 {
				t.Fatalf("contract recovery/replay lost work/cost identity: scans=%d gates=%d costs=%d matches=%d", observer.scans, gate.checks, costs, observations)
			}
		})
	}
}

func TestSourceSchemaCompletedScanDoesNotRequireCleanup(t *testing.T) {
	ctx, pool, req, observer, _, s := schemaRetryFixture(t)
	observer.stopErr = errors.New("profile mount temporarily unavailable for cleanup")
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status); err != nil || status != "MATCH" {
		t.Fatalf("completed scan lost: %s %v", status, err)
	}
	var attempts, costs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_schema_attempt WHERE request_id=$1`, req.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, req.ID).Scan(&costs); err != nil {
		t.Fatal(err)
	}
	if observer.stops != 0 || observer.scans != 1 || attempts != 1 || costs != 1 {
		t.Fatalf("completed scan became cleanup/retry work: stops=%d scans=%d attempts=%d costs=%d", observer.stops, observer.scans, attempts, costs)
	}
}

func TestSourceSchemaStartupFailureCostAndSameAttemptReplay(t *testing.T) {
	ctx, pool, req, observer, _, s := schemaRetryFixture(t)
	observer.scanErr = errors.New("process failed before protocol marker")
	if err := s.Process(ctx, req.ID); err == nil {
		t.Fatal("failed startup accepted")
	}
	var attempt uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM source_schema_attempt WHERE request_id=$1`, req.ID).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.ChargeScan(ctx, attempt); err != nil {
			t.Fatal(err)
		}
	}
	count := func() int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, req.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count() != 1 || observer.sourceIO != 0 {
		t.Fatal("failed startup lost cost, replay duplicated cost, or source I/O ran")
	}
	observer.scanErr = nil
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if count() != 2 || observer.scans != 2 || observer.sourceIO != 1 {
		t.Fatal("real retry cost or successful no-work replay changed")
	}
}

func TestSourceSchemaStartupDoesNotReplaceFreshSourceIOGate(t *testing.T) {
	ctx, pool, req, observer, _, s := schemaRetryFixture(t)
	observer.onStartup = func() {
		if _, err := pool.Exec(ctx, `UPDATE resource_binding SET is_primary=false WHERE id=$1`, req.BindingID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	var costs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, req.ID).Scan(&costs); err != nil {
		t.Fatal(err)
	}
	var status, reason string
	if err := pool.QueryRow(ctx, `SELECT status,reason FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status, &reason); err != nil {
		t.Fatal(err)
	}
	if costs != 1 || observer.sourceIO != 0 || status != "DENIED" || reason != "SOURCE_BINDING_CHANGED" {
		t.Fatalf("startup bypassed source fence: costs=%d io=%d status=%s reason=%s", costs, observer.sourceIO, status, reason)
	}
}

func TestSourceSchemaFailedScanStillRequiresContainment(t *testing.T) {
	ctx, pool, req, observer, _, s := schemaRetryFixture(t)
	observer.scanErr = errors.New("scan completion unknown")
	observer.stopErr = errors.New("containment unconfirmed")
	if err := s.Process(ctx, req.ID); !errors.Is(err, observer.stopErr) {
		t.Fatalf("unknown scan ignored containment: %v", err)
	}
	var observations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&observations); err != nil || observations != 0 || observer.stops != 1 {
		t.Fatalf("uncontained scan accepted: observations=%d stops=%d %v", observations, observer.stops, err)
	}
	observer.stopErr = nil
	observer.scanErr = nil
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	var costs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, req.ID).Scan(&costs); err != nil {
		t.Fatal(err)
	}
	var statuses []string
	if err := pool.QueryRow(ctx, `SELECT array_agg(status ORDER BY recorded_at,id) FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&statuses); err != nil {
		t.Fatal(err)
	}
	if observer.stops != 2 || observer.scans != 2 || costs != 2 || strings.Join(statuses, ",") != "INTERRUPTED,MATCH" {
		t.Fatalf("failed scan recovery changed: stops=%d scans=%d costs=%d statuses=%v", observer.stops, observer.scans, costs, statuses)
	}
}

func TestSourceSchemaFinalBindingChangeHasExactReason(t *testing.T) {
	ctx, pool, req, observer, _, s := schemaRetryFixture(t)
	observer.onScan = func() {
		if _, err := pool.Exec(ctx, `UPDATE resource_binding SET is_primary=false WHERE id=$1`, req.BindingID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	var status, reason string
	if err := pool.QueryRow(ctx, `SELECT status,reason FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status, &reason); err != nil || status != "DENIED" || reason != "SOURCE_BINDING_CHANGED" {
		t.Fatalf("binding mutation misattributed: %s %s %v", status, reason, err)
	}
	var auditReason, evidenceReason string
	if err := pool.QueryRow(ctx, `SELECT after_state->>'reason' FROM audit_event WHERE object_id=$1 AND action='SourceSchemaObserved'`, req.ID).Scan(&auditReason); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT metadata->'fact'->>'reason' FROM evidence WHERE source_id=$1 AND title='SourceSchemaObserved'`, req.ID).Scan(&evidenceReason); err != nil {
		t.Fatal(err)
	}
	if auditReason != reason || evidenceReason != reason {
		t.Fatalf("immutable audit/evidence reason differs: %s %s", auditReason, evidenceReason)
	}
}
