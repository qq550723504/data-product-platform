package metadatahttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	me "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	rightshttp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/transport/http"
)

type forbiddenSchemaObserver struct{ me.SourceSchemaObserver }

func (forbiddenSchemaObserver) Plan(context.Context) (d.SchemaPlan, error) {
	panic("a historical read must not inspect scanner composition")
}

func TestSchemaQueriesReadOnlyExactHistory(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := infra.NewPostgresRepository(pool)
	ws, foreign, actor, resource, authorization := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	p := d.SchemaPlan{Enabled: true, WorkspaceID: ws, ResourceID: resource, ActorID: actor, AuthorizationID: authorization,
		ConsumerRef: "TEST", Purpose: "TEST", ExternalFQN: "test." + resource.String(), RuleRef: "test/schema", RuleVersion: "1",
		RuleHash: fmt.Sprintf("%064d", 1), ConnectorHash: fmt.Sprintf("%064d", 2), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'schema query fixture','TABLE_LIKE');`, resource, ws, resource.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, authorization, ws, authorization.String()); err != nil {
		t.Fatal(err)
	}
	binding, err := repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: resource, Provider: "TEST_METADATA", EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: p.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	var oldestRequest, oldestObservation uuid.UUID
	wanted := map[uuid.UUID]bool{}
	for i := 0; i < 106; i++ {
		plan := p
		if i == 105 {
			plan.RuleVersion = "2"
			plan.RuleHash = fmt.Sprintf("%064d", 3)
		}
		req, _, err := repo.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: plan, PlanHash: d.Hash(plan), BindingID: binding.ID, ExternalID: binding.ExternalID, Key: fmt.Sprintf("query-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		attempt, observation := uuid.New(), uuid.New()
		if err = repo.BeginSchemaAttempt(ctx, tx, req, attempt, json.RawMessage(`{"fixture":"query-only"}`)); err != nil {
			t.Fatal(err)
		}
		status := "DRIFT"
		columns := append(append([]d.SchemaColumn{}, p.Columns...), d.SchemaColumn{Name: "extra", Type: "TEXT", Constraint: "NULL"})
		if i == 105 {
			status = "MATCH"
			columns = p.Columns
		}
		now := time.Now().UTC()
		if err = repo.AppendSchemaObservation(ctx, tx, d.SchemaObservation{ID: observation, RequestID: req.ID, AttemptID: &attempt, Status: status, Reason: "QUERY_FIXTURE", Columns: columns, SchemaHash: d.Hash(columns), StartedAt: &now, CompletedAt: &now}); err != nil {
			t.Fatal(err)
		}
		if status == "DRIFT" {
			wanted[observation] = true
		}
		if i == 0 {
			oldestRequest, oldestObservation = req.ID, observation
			for j := 0; j < 3; j++ {
				if _, err = repo.HandleSchema(ctx, tx, resource, ws, actor, observation, fmt.Sprintf("handle-%d", j), "investigating"); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// A mutable current binding and newer MATCH/rule must not replace old evidence.
	if _, err = tx.Exec(ctx, `UPDATE resource_binding SET external_fqn=$2 WHERE id=$1`, binding.ID, "new.current."+resource.String()); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readPool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer readPool.Close()
	readRepo := infra.NewPostgresRepository(readPool)
	service := app.NewService("TEST_METADATA", "", nil, readRepo, nil, nil)
	service.Schema = &app.SchemaService{Observer: forbiddenSchemaObserver{}}
	h := NewHandler(service, readRepo)
	authorizer, err := rightshttp.NewStaticAuthorizer("query-secret", actor.String(), []string{ws.String(), foreign.String()})
	if err != nil {
		t.Fatal(err)
	}
	h.Authorize = authorizer.Authorize
	mux := http.NewServeMux()
	h.Register(mux)
	get := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Actor-ID", actor.String()) // Spoofable headers grant nothing.
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	listPath := "/api/v1/workspaces/" + ws.String() + "/source-schema-observations/attention"
	detailPath := "/api/v1/data-resources/" + resource.String() + "/schema-observations/" + oldestObservation.String() + "?workspaceId=" + ws.String()
	fingerprint := func() map[string]string {
		out := map[string]string{}
		for _, table := range []string{"source_schema_request", "source_schema_attempt", "source_schema_observation", "source_schema_handling"} {
			var hash string
			query := `SELECT md5(COALESCE(jsonb_agg(to_jsonb(f) ORDER BY f.id)::text,'[]')) FROM ` + table + ` f WHERE `
			switch table {
			case "source_schema_request", "source_schema_attempt":
				query += `f.workspace_id=$1`
			case "source_schema_observation":
				query += `f.request_id IN (SELECT id FROM source_schema_request WHERE workspace_id=$1)`
			default:
				query += `f.observation_id IN (SELECT o.id FROM source_schema_observation o JOIN source_schema_request r ON r.id=o.request_id WHERE r.workspace_id=$1)`
			}
			if err := pool.QueryRow(ctx, query, ws).Scan(&hash); err != nil {
				t.Fatal(err)
			}
			out[table] = hash
		}
		return out
	}
	before := fingerprint()
	seen := map[uuid.UUID]bool{}
	for offset := 0; offset < 125; offset += 25 {
		w := get(fmt.Sprintf("%s?view=ALL&limit=25&offset=%d", listPath, offset), "query-secret")
		var page infra.SchemaAttentionPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		if page.Page.Total != 105 || page.Page.Limit != 25 || page.Page.Offset != offset || page.Summary.HandledCount != 1 || page.Summary.UnhandledCount != 104 || page.ReadAt.IsZero() {
			t.Fatalf("wrong counts: %+v", page)
		}
		for _, row := range page.Items {
			if row.WorkspaceID != ws || !wanted[row.ObservationID] || seen[row.ObservationID] || row.Status != "DRIFT" {
				t.Fatal("duplicate/foreign/non-drift observation", row)
			}
			seen[row.ObservationID] = true
		}
	}
	if len(seen) != 105 {
		t.Fatal("truncated history", len(seen))
	}
	w := get(listPath, "query-secret")
	var unhandled infra.SchemaAttentionPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &unhandled) != nil || unhandled.Page.Total != 104 {
		t.Fatal("wrong default view", w.Body.String())
	}
	for _, row := range unhandled.Items {
		if row.HandlingCount != 0 {
			t.Fatal("handled observation in unhandled view")
		}
	}
	// The existing capped history cannot return this old request; direct lookup must.
	history, err := readRepo.SchemaHistory(ctx, resource, ws)
	if err != nil || len(history) != 50 {
		t.Fatal("history fixture", err, len(history))
	}
	for _, row := range history {
		if row["id"] == oldestRequest {
			t.Fatal("old request unexpectedly inside cap")
		}
	}
	w = get(detailPath, "query-secret")
	var detail infra.SchemaObservationDetail
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil {
		t.Fatalf("detail: %d %s", w.Code, w.Body.String())
	}
	if detail.ObservationID != oldestObservation || detail.RequestID != oldestRequest || detail.ResourceID != resource || detail.WorkspaceID != ws || detail.RuleVersion != "1" || detail.ExternalFQN != p.ExternalFQN || detail.BindingID != binding.ID || detail.ExternalID != binding.ExternalID || detail.ResponsibleActorID != actor || detail.HandlingCount != 3 || len(detail.Handling) != 3 || len(detail.Columns) != 2 || !reflect.DeepEqual(detail.Baseline, p.Columns) || detail.ConnectorHash != p.ConnectorHash {
		t.Fatalf("history substitution: %+v", detail)
	}
	for _, record := range detail.Handling {
		if record.ObservationID != oldestObservation || record.ActorID != actor {
			t.Fatal("handling mismatch")
		}
	}
	// Disabled scanner uses the identical read path and returns identical evidence.
	h.service = nil
	if other := get(detailPath, "query-secret"); other.Code != 200 || other.Body.String() != w.Body.String() {
		t.Fatal("disabled scanner changed history")
	}
	if !reflect.DeepEqual(before, fingerprint()) {
		t.Fatal("GET changed immutable facts")
	}

	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{listPath, "", 403}, {listPath, "wrong", 403}, {detailPath, "wrong", 403},
		{"/api/v1/workspaces/" + uuid.NewString() + "/source-schema-observations/attention", "query-secret", 403},
		{listPath + "?view=CLOSED", "query-secret", 400}, {listPath + "?limit=0", "query-secret", 400}, {listPath + "?limit=101", "query-secret", 400}, {listPath + "?offset=-1", "query-secret", 400}, {listPath + "?offset=2147483648", "query-secret", 400},
		{"/api/v1/data-resources/" + resource.String() + "/schema-observations/" + oldestObservation.String() + "?workspaceId=" + foreign.String(), "query-secret", 404},
		{"/api/v1/data-resources/" + resource.String() + "/schema-observations/" + uuid.NewString() + "?workspaceId=" + ws.String(), "query-secret", 404},
		{"/api/v1/data-resources/" + uuid.NewString() + "/schema-observations/" + oldestObservation.String() + "?workspaceId=" + ws.String(), "query-secret", 404},
	} {
		if response := get(tc.path, tc.token); response.Code != tc.status {
			t.Fatalf("%s: got %d want %d: %s", tc.path, response.Code, tc.status, response.Body.String())
		}
	}
	emptyPath := "/api/v1/workspaces/" + foreign.String() + "/source-schema-observations/attention"
	if response := get(emptyPath, "query-secret"); response.Code != 200 {
		t.Fatal("allowed empty workspace", response.Body.String())
	} else {
		var page infra.SchemaAttentionPage
		if json.Unmarshal(response.Body.Bytes(), &page) != nil || page.Page.Total != 0 || len(page.Items) != 0 {
			t.Fatal("foreign workspace leaked observations")
		}
	}
	h.Authorize = nil
	if get(listPath, "query-secret").Code != 401 || get(detailPath, "query-secret").Code != 401 {
		t.Fatal("unconfigured authorizer allowed history")
	}
}
