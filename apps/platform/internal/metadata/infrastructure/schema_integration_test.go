package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	repo "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// Uses actual PostgreSQL guards and locks. Native discovery is separately tested
// through the controlled acceptance runner; this test claims no provider work.
func TestSourceSchemaFactsConcurrencyAndCostIsolation(t *testing.T) {
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
	if _, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'schema guard test','TABLE_LIKE')`, p.ResourceID, p.WorkspaceID, p.ResourceID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, p.AuthorizationID, p.WorkspaceID, p.AuthorizationID.String()); err != nil {
		t.Fatal(err)
	}
	r := repo.NewPostgresRepository(pool)
	b, err := r.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: p.ResourceID, Provider: testProvider, EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: p.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	key := uuid.NewString()
	requests := make(chan d.SchemaRequest, 8)
	failures := make(chan error, 8)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tx, e := pool.Begin(ctx)
			if e != nil {
				failures <- e
				return
			}
			defer tx.Rollback(ctx)
			v, _, e := r.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: p, PlanHash: d.Hash(p), BindingID: b.ID, ExternalID: b.ExternalID, Key: key})
			if e == nil {
				e = tx.Commit(ctx)
			}
			if e != nil {
				failures <- e
			} else {
				requests <- v
			}
		}()
	}
	close(start)
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	close(requests)
	var request d.SchemaRequest
	for v := range requests {
		if request.ID != uuid.Nil && v.ID != request.ID {
			t.Fatal("same-key request split")
		}
		request = v
	}
	if request.ID == uuid.Nil {
		t.Fatal("no request persisted")
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changed := request
	changed.ID = uuid.New()
	changed.Plan.RuleVersion = "2"
	changed.PlanHash = d.Hash(changed.Plan)
	if _, _, err = r.PrepareSchema(ctx, tx, changed); !errors.Is(err, repo.ErrSchemaConflict) {
		t.Fatalf("changed same key: %v", err)
	}
	tx.Rollback(ctx)
	attempt, observation := uuid.New(), uuid.New()
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = r.BeginSchemaAttempt(ctx, tx, request, attempt, []byte(`{"fixture":"repository-runtime"}`)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err = r.AppendSchemaObservation(ctx, tx, d.SchemaObservation{ID: observation, RequestID: request.ID, AttemptID: &attempt, Status: "DRIFT", Reason: "TEST", Columns: p.Columns, SchemaHash: d.Hash(p.Columns), StartedAt: &now, CompletedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if _, err = r.HandleSchema(ctx, tx, p.ResourceID, p.WorkspaceID, p.ActorID, observation, "handle", "test drift responsibility"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := r.SchemaHistory(ctx, p.ResourceID, p.WorkspaceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("history %d: %v", len(items), err)
	}
	service := app.NewService(testProvider, "", transaction.NewManager(pool), r, nil, nil)
	schema := app.SchemaService{Service: service}
	failures = make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- schema.ChargeScan(ctx, attempt) }()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation WHERE source_schema_attempt_id=$1`, attempt).Scan(&n); err != nil || n != 1 {
		t.Fatalf("same physical-attempt cost duplicated: %d %v", n, err)
	}
	rejected := func(sql string, args ...any) {
		t.Helper()
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, sql, args...); e == nil {
			t.Fatalf("unsafe fact write accepted: %s", sql)
		}
	}
	for table, id := range map[string]uuid.UUID{"source_schema_request": request.ID, "source_schema_attempt": attempt, "source_schema_observation": observation} {
		rejected("UPDATE "+table+" SET id=id WHERE id=$1", id)
		rejected("DELETE FROM "+table+" WHERE id=$1", id)
	}
	rejected(`INSERT INTO source_schema_attempt(id,request_id,workspace_id) VALUES($1,$2,$3)`, uuid.New(), request.ID, uuid.New())
	rejected(`INSERT INTO source_schema_observation(id,request_id,attempt_id,status,reason,columns,schema_hash) VALUES($1,$2,$3,'UNAVAILABLE','TEST','[]','')`, uuid.New(), request.ID, attempt)
	rejected(`INSERT INTO source_schema_handling(id,observation_id,actor_id,reason,request_key) VALUES($1,$2,$3,'test','foreign')`, uuid.New(), observation, uuid.New())
	rejected(`UPDATE source_schema_attempt_runtime SET contract='{}' WHERE attempt_id=$1`, attempt)
	rejected(`DELETE FROM source_schema_attempt_runtime WHERE attempt_id=$1`, attempt)
	missing, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = missing.Exec(ctx, `INSERT INTO source_schema_attempt(id,request_id,workspace_id) VALUES($1,$2,$3)`, uuid.New(), request.ID, p.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	if err = missing.Commit(ctx); err == nil {
		t.Fatal("unbound attempt committed")
	}
	var allocationID uuid.UUID
	if err = pool.QueryRow(ctx, `SELECT id FROM cost_allocation WHERE source_schema_attempt_id=$1`, attempt).Scan(&allocationID); err != nil {
		t.Fatal(err)
	}
	rejected(`INSERT INTO cost_allocation(id,cost_event_id,source_schema_attempt_id) SELECT $1,cost_event_id,source_schema_attempt_id FROM cost_allocation WHERE id=$2`, uuid.New(), allocationID)
	// A mismatch in the typed request contract must fail before becoming history.
	rejected(`INSERT INTO source_schema_request(id,workspace_id,resource_id,actor_id,authorization_id,consumer_ref,purpose,binding_id,external_id,external_fqn,rule_ref,rule_version,rule_hash,plan_hash,plan,request_key)
 SELECT $1,workspace_id,resource_id,$2,authorization_id,consumer_ref,purpose,binding_id,external_id,external_fqn,rule_ref,rule_version,rule_hash,plan_hash,plan,$3 FROM source_schema_request WHERE id=$4`, uuid.New(), uuid.New(), uuid.NewString(), request.ID)
	// The immutable guard also covers handling rows; never clean up frozen facts.
	rejected(`UPDATE source_schema_handling SET actor_id=$1 WHERE observation_id=$2`, p.ActorID, observation)
	rejected(`DELETE FROM source_schema_handling WHERE observation_id=$1`, observation)
}
