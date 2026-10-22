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
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	rightshttp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/transport/http"
)

func TestSchemaExceptionHistoryReadOnly(t *testing.T) {
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
		ConsumerRef: "TEST", Purpose: "TEST", ExternalFQN: "exception." + resource.String(), RuleRef: "test/schema", RuleVersion: "1",
		RuleHash: fmt.Sprintf("%064d", 1), ConnectorHash: fmt.Sprintf("%064d", 2), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'exception history fixture','TABLE_LIKE')`, resource, ws, resource.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, authorization, ws, authorization.String()); err != nil {
		t.Fatal(err)
	}
	binding, err := repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: resource, Provider: "TEST_METADATA", EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: p.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	attempts := map[uuid.UUID]*uuid.UUID{}
	appendObservation := func(req d.SchemaRequest, status string, withoutAttempt bool) uuid.UUID {
		t.Helper()
		o := d.SchemaObservation{ID: uuid.New(), RequestID: req.ID, Status: status, Reason: "HISTORICAL_" + status}
		if !withoutAttempt {
			attempt := uuid.New()
			if err := repo.BeginSchemaAttempt(ctx, tx, req, attempt, json.RawMessage(`{"fixture":"exceptions-only"}`)); err != nil {
				t.Fatal(err)
			}
			o.AttemptID = &attempt
		}
		if status == "MATCH" || status == "DRIFT" {
			now := time.Now().UTC()
			o.Columns, o.SchemaHash, o.StartedAt, o.CompletedAt = p.Columns, d.Hash(p.Columns), &now, &now
		}
		if err := repo.AppendSchemaObservation(ctx, tx, o); err != nil {
			t.Fatal(err)
		}
		attempts[o.ID] = o.AttemptID
		return o.ID
	}
	wanted := map[uuid.UUID]string{}
	var oldReq d.SchemaRequest
	var oldObservation, deniedWithout, deniedWith uuid.UUID
	for i := 0; i < 105; i++ {
		plan := p
		if i == 104 {
			plan.RuleVersion, plan.RuleHash = "2", fmt.Sprintf("%064d", 3)
		}
		req, _, err := repo.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: plan, PlanHash: d.Hash(plan), BindingID: binding.ID, ExternalID: binding.ExternalID, Key: fmt.Sprintf("exception-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		status := []string{"UNAVAILABLE", "DENIED", "INTERRUPTED"}[i%3]
		id := appendObservation(req, status, status == "DENIED" && i%2 == 1)
		wanted[id] = status
		if i == 0 {
			oldReq, oldObservation = req, id
		}
		if i == 1 {
			deniedWithout = id
		}
		if i == 4 {
			deniedWith = id
		}
	}
	// A single request retains two exception attempts followed by success.
	wanted[appendObservation(oldReq, "INTERRUPTED", false)] = "INTERRUPTED"
	appendObservation(oldReq, "MATCH", false)
	// DRIFT and its multiple handling records belong only to the existing queue.
	drift := appendObservation(oldReq, "DRIFT", false)
	for i := 0; i < 3; i++ {
		if _, err = repo.HandleSchema(ctx, tx, resource, ws, actor, drift, fmt.Sprintf("handling-%d", i), "investigating drift"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE resource_binding SET external_fqn=$2 WHERE id=$1`, binding.ID, "current."+resource.String()); err != nil {
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
	auth, err := rightshttp.NewStaticAuthorizer("exception-secret", actor.String(), []string{ws.String(), foreign.String()})
	if err != nil {
		t.Fatal(err)
	}
	h.Authorize = auth.Authorize
	mux := http.NewServeMux()
	h.Register(mux)
	get := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Actor-ID", actor.String())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	list := "/api/v1/workspaces/" + ws.String() + "/source-schema-observations/exceptions"
	detailPath := func(id uuid.UUID) string {
		return "/api/v1/data-resources/" + resource.String() + "/schema-observations/" + id.String() + "?workspaceId=" + ws.String()
	}
	fingerprint := func() []string {
		t.Helper()
		out := []string{}
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
			out = append(out, hash)
		}
		return out
	}
	before := fingerprint()
	seen := map[uuid.UUID]bool{}
	for offset := 0; offset < 150; offset += 25 {
		w := get(fmt.Sprintf("%s?limit=25&offset=%d", list, offset), "exception-secret")
		var page infra.SchemaExceptionPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		if page.Page.Total != 106 || page.Page.Offset != offset || page.Page.Limit != 25 || page.Summary.UnavailableCount != 35 || page.Summary.DeniedCount != 35 || page.Summary.InterruptedCount != 36 || page.ReadAt.IsZero() {
			t.Fatalf("incorrect snapshot/counts: %+v", page)
		}
		for _, o := range page.Items {
			if seen[o.ObservationID] || wanted[o.ObservationID] != o.Status || o.WorkspaceID != ws || o.ResponsibleActorID != actor || o.ExternalFQN != p.ExternalFQN || o.RecordedAt.IsZero() {
				t.Fatalf("duplicate/foreign/substituted row: %+v", o)
			}
			seen[o.ObservationID] = true
			if !reflect.DeepEqual(o.AttemptID, attempts[o.ObservationID]) {
				t.Fatal("historical attempt identity substituted")
			}
			if o.ObservationID == deniedWithout && o.AttemptID != nil || o.ObservationID == deniedWith && o.AttemptID == nil {
				t.Fatal("DENIED attempt identity changed")
			}
		}
	}
	if len(seen) != len(wanted) {
		t.Fatal("exception history was truncated")
	}
	for status, expected := range map[string]int{"UNAVAILABLE": 35, "DENIED": 35, "INTERRUPTED": 36} {
		w := get(list+"?status="+status+"&limit=100", "exception-secret")
		var page infra.SchemaExceptionPage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Page.Total != expected || len(page.Items) != expected || page.Summary.InterruptedCount != 36 {
			t.Fatal("filter changed historical summary", w.Body.String())
		}
		for _, row := range page.Items {
			if row.Status != status {
				t.Fatal("wrong status in filtered page")
			}
		}
	}
	history, err := readRepo.SchemaHistory(ctx, resource, ws)
	if err != nil || len(history) != 50 {
		t.Fatal("capped history fixture", err)
	}
	for _, row := range history {
		if row["id"] == oldReq.ID {
			t.Fatal("old request unexpectedly inside recent history")
		}
	}
	for _, id := range []uuid.UUID{oldObservation, deniedWithout, deniedWith} {
		w := get(detailPath(id), "exception-secret")
		var detail infra.SchemaObservationDetail
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &detail) != nil || detail.ObservationID != id || detail.Status != wanted[id] || detail.ExternalFQN != p.ExternalFQN || detail.BindingID != binding.ID || detail.ExternalID != binding.ExternalID || detail.RuleVersion != "1" || detail.ResponsibleActorID != actor || !reflect.DeepEqual(detail.Baseline, p.Columns) || len(detail.Columns) != 0 || detail.SchemaHash != "" || detail.StartedAt != nil || detail.CompletedAt != nil {
			t.Fatalf("exact history substituted or unavailable: %s", w.Body.String())
		}
		if id == oldObservation && detail.RequestID != oldReq.ID {
			t.Fatal("old request identity changed")
		}
		if !reflect.DeepEqual(detail.AttemptID, attempts[id]) {
			t.Fatal("exact history substituted attempt")
		}
	}
	driftPage, err := readRepo.SchemaAttention(ctx, ws, "ALL", 25, 0)
	if err != nil || driftPage.Page.Total != 1 || driftPage.Summary.HandledCount != 1 || driftPage.Items[0].HandlingCount != 3 {
		t.Fatal("exception read changed DRIFT contract", err)
	}
	rotatedActor := uuid.New()
	rotatedAuth, err := rightshttp.NewStaticAuthorizer("exception-secret", rotatedActor.String(), []string{ws.String(), foreign.String()})
	if err != nil {
		t.Fatal(err)
	}
	h.Authorize = rotatedAuth.Authorize
	var historicalActor infra.SchemaObservationDetail
	rotatedRead := get(detailPath(oldObservation), "exception-secret")
	if rotatedRead.Code != 200 || json.Unmarshal(rotatedRead.Body.Bytes(), &historicalActor) != nil || historicalActor.ResponsibleActorID != actor || historicalActor.ResponsibleActorID == rotatedActor {
		t.Fatal("current configured actor replaced historical responsibility")
	}
	h.Authorize = auth.Authorize
	h.service = nil
	if get(list, "exception-secret").Code != 200 || get(detailPath(oldObservation), "exception-secret").Code != 200 {
		t.Fatal("disabled scanner blocked historical reads")
	}
	if !reflect.DeepEqual(before, fingerprint()) {
		t.Fatal("GET mutated history")
	}
	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{list, "", 403}, {list, "bad", 403}, {detailPath(oldObservation), "bad", 403},
		{"/api/v1/workspaces/" + uuid.NewString() + "/source-schema-observations/exceptions", "exception-secret", 403},
		{list + "?status=DRIFT", "exception-secret", 400}, {list + "?status=RESOLVED", "exception-secret", 400},
		{list + "?limit=0", "exception-secret", 400}, {list + "?limit=101", "exception-secret", 400}, {list + "?offset=-1", "exception-secret", 400}, {list + "?offset=2147483648", "exception-secret", 400}, {list + "?limit=abc", "exception-secret", 400},
		{detailPath(uuid.New()), "exception-secret", 404},
		{"/api/v1/data-resources/" + uuid.NewString() + "/schema-observations/" + oldObservation.String() + "?workspaceId=" + ws.String(), "exception-secret", 404},
		{"/api/v1/data-resources/" + resource.String() + "/schema-observations/" + oldObservation.String() + "?workspaceId=" + foreign.String(), "exception-secret", 404},
	} {
		if w := get(tc.path, tc.token); w.Code != tc.status {
			t.Fatalf("%s: %d want %d", tc.path, w.Code, tc.status)
		}
	}
	w := get("/api/v1/workspaces/"+foreign.String()+"/source-schema-observations/exceptions", "exception-secret")
	var empty infra.SchemaExceptionPage
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &empty) != nil || empty.Page.Total != 0 || len(empty.Items) != 0 || empty.Summary.DeniedCount != 0 || empty.Summary.InterruptedCount != 0 || empty.Summary.UnavailableCount != 0 {
		t.Fatal("empty allowed workspace leaked facts")
	}
	h.Authorize = nil
	if get(list, "exception-secret").Code != 401 {
		t.Fatal("missing authorizer allowed read")
	}
	h.Authorize = auth.Authorize
	readPool.Close()
	if get(list, "exception-secret").Code != http.StatusServiceUnavailable {
		t.Fatal("read failure became empty success")
	}
}
