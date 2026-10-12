package application

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	scans      int
}

func (o *retrySchemaObserver) Plan(context.Context) (d.SchemaPlan, error) { return o.plan, o.err }
func (o *retrySchemaObserver) Stop(context.Context, uuid.UUID) error      { return nil }
func (o *retrySchemaObserver) Scan(_ context.Context, _ uuid.UUID, started func() error) (me.SourceSchema, error) {
	o.scans++
	if err := started(); err != nil {
		return me.SourceSchema{}, err
	}
	now := time.Now().UTC()
	return me.SourceSchema{ExternalID: o.externalID, ExternalFQN: o.plan.ExternalFQN, ConnectorHash: o.plan.ConnectorHash, Columns: o.plan.Columns, StartedAt: now, CompletedAt: now}, nil
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
func TestSourceSchemaProfileFailureRetriesSameRequest(t *testing.T) {
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
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
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
	observer := &retrySchemaObserver{plan: p, externalID: b.ExternalID}
	gate := &schemaTestEntitlements{expected: p}
	s := &SchemaService{Service: NewService("TEST_METADATA", "", transaction.NewManager(pool), repo, nil, nil), Observer: observer, Rights: gate}
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
