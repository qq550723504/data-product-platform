package metadatahttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	rightshttp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/transport/http"
)

func TestUnobservedSchemaRequestsReadOnly(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := infra.NewPostgresRepository(pool)
	ws, foreign, actor, resource, authorization := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	p := d.SchemaPlan{Enabled: true, WorkspaceID: ws, ResourceID: resource, ActorID: actor, AuthorizationID: authorization,
		ConsumerRef: "TEST", Purpose: "TEST", ExternalFQN: "unobserved." + resource.String(), RuleRef: "test/schema", RuleVersion: "1", RuleHash: fmt.Sprintf("%064d", 1), ConnectorHash: fmt.Sprintf("%064d", 2), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, sql := range []string{
		`INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'unobserved request fixture','TABLE_LIKE')`,
	} {
		if _, err = tx.Exec(ctx, sql, resource, ws, resource.String()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, authorization, ws, authorization.String()); err != nil {
		t.Fatal(err)
	}
	binding, err := repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: resource, Provider: "TEST_METADATA", EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: p.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	requests := []d.SchemaRequest{}
	attemptIDs := map[uuid.UUID]uuid.UUID{}
	newRequest := func(key string) d.SchemaRequest {
		t.Helper()
		v, _, e := repo.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: p, PlanHash: d.Hash(p), BindingID: binding.ID, ExternalID: binding.ExternalID, Key: key})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	newAttempt := func(req d.SchemaRequest) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if e := repo.BeginSchemaAttempt(ctx, tx, req, id, json.RawMessage(`{"fixture":"private-runtime-sentinel"}`)); e != nil {
			t.Fatal(e)
		}
		return id
	}
	for i := 0; i < 105; i++ {
		req := newRequest(fmt.Sprintf("unobserved-%d", i))
		requests = append(requests, req)
		if i%2 == 1 {
			attemptIDs[req.ID] = newAttempt(req)
		}
	}
	// All observation kinds exclude their requests, even a retry attempt without
	// a new observation. These are separate realistic request lifecycles.
	statuses := []string{"MATCH", "DRIFT", "UNAVAILABLE", "DENIED", "INTERRUPTED"}
	var history d.SchemaRequest
	var firstAttempt uuid.UUID
	for _, status := range statuses {
		req := newRequest("observed-" + status)
		if status == "UNAVAILABLE" {
			history = req
		}
		for j := 0; j < 1 || (status == "UNAVAILABLE" && j < 31); j++ {
			var a *uuid.UUID
			if status != "DENIED" {
				id := newAttempt(req)
				a = &id
				if j == 0 && status == "UNAVAILABLE" {
					firstAttempt = id
				}
			}
			o := d.SchemaObservation{ID: uuid.New(), RequestID: req.ID, AttemptID: a, Status: status, Reason: "RECORDED_" + status}
			if status == "MATCH" || status == "DRIFT" {
				now := time.Now().UTC()
				o.Columns = append([]d.SchemaColumn{}, p.Columns...)
				if status == "DRIFT" {
					o.Columns = append(o.Columns, d.SchemaColumn{Name: "extra", Type: "TEXT", Constraint: "NULL"})
				}
				o.SchemaHash = d.Hash(o.Columns)
				o.StartedAt = &now
				o.CompletedAt = &now
			}
			if e := repo.AppendSchemaObservation(ctx, tx, o); e != nil {
				t.Fatal(e)
			}
		}
		if status == "UNAVAILABLE" {
			newAttempt(req)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	readPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer readPool.Close()
	readRepo := infra.NewPostgresRepository(readPool)
	svc := app.NewService("TEST_METADATA", "", nil, readRepo, nil, nil)
	svc.Schema = &app.SchemaService{Observer: forbiddenSchemaObserver{}}
	h := NewHandler(svc, readRepo)
	auth, err := rightshttp.NewStaticAuthorizer("request-secret", uuid.NewString(), []string{ws.String(), foreign.String()})
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
	list := "/api/v1/workspaces/" + ws.String() + "/source-schema-requests/unobserved"
	detail := func(req uuid.UUID) string {
		return "/api/v1/data-resources/" + resource.String() + "/schema-requests/" + req.String() + "?workspaceId=" + ws.String()
	}
	fingerprint := func() string {
		t.Helper()
		var s string
		if e := pool.QueryRow(ctx, `SELECT md5(jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM source_schema_request r WHERE workspace_id=$1),
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM source_schema_attempt a WHERE workspace_id=$1),
 (SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM source_schema_observation o JOIN source_schema_request r ON r.id=o.request_id WHERE r.workspace_id=$1),
 (SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM cost_event c WHERE workspace_id=$1))::text)`, ws).Scan(&s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	before := fingerprint()
	seen := map[uuid.UUID]bool{}
	for offset := 0; offset <= 125; offset += 25 {
		w := get(fmt.Sprintf("%s?limit=25&offset=%d", list, offset), "request-secret")
		var out infra.UnobservedSchemaRequests
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("list %d %s", w.Code, w.Body.String())
		}
		if out.Page.Total != 105 || out.Summary.NoAttemptCount != 53 || out.Summary.AttemptRecordedCount != 52 || out.ReadAt.IsZero() {
			t.Fatalf("snapshot counts %+v", out)
		}
		for _, item := range out.Items {
			if seen[item.RequestID] || item.WorkspaceID != ws || item.ResponsibleActorID != actor || item.ExternalFQN != p.ExternalFQN {
				t.Fatalf("duplicate/foreign/substitution %+v", item)
			}
			seen[item.RequestID] = true
		}
		if strings.Contains(w.Body.String(), "private-runtime-sentinel") {
			t.Fatal("runtime leaked")
		}
	}
	if len(seen) != 105 || seen[history.ID] {
		t.Fatal("collection membership incorrect")
	}
	for view, total := range map[string]int{"NO_ATTEMPT": 53, "ATTEMPT_RECORDED": 52} {
		w := get(list+"?view="+view, "request-secret")
		var out infra.UnobservedSchemaRequests
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Page.Total != total {
			t.Fatalf("filter %s: %s", view, w.Body.String())
		}
	}
	for _, req := range []d.SchemaRequest{requests[0], requests[1], history} {
		path := detail(req.ID)
		if req.ID == history.ID {
			path += "&attemptId=" + firstAttempt.String() + "&attemptOffset=25&observationOffset=25"
		}
		w := get(path, "request-secret")
		var out infra.SchemaRequestDetail
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.RequestID != req.ID || out.BindingID != binding.ID || out.ExternalID != binding.ExternalID || out.PlanHash != d.Hash(p) || !reflect.DeepEqual(out.Baseline, p.Columns) {
			t.Fatalf("exact detail: %d %s", w.Code, w.Body.String())
		}
		if req.ID == history.ID {
			if out.SelectedAttempt == nil || out.SelectedAttempt.ID != firstAttempt || out.Attempts.Page.Total != 32 || len(out.Attempts.Items) != 7 || out.Observations.Page.Total != 31 || len(out.Observations.Items) != 6 {
				t.Fatalf("paged independent selection: %+v", out)
			}
		} else if out.SelectedAttempt != nil {
			t.Fatal("silently selected latest attempt")
		}
		if strings.Contains(w.Body.String(), "private-runtime-sentinel") {
			t.Fatal("runtime leaked")
		}
	}
	for _, q := range []string{"view=RUNNING", "view=NO_OBSERVATION", "limit=0", "limit=101", "offset=-1", "offset=2147483648", "limit=abc"} {
		if w := get(list+"?"+q, "request-secret"); w.Code != 400 {
			t.Fatalf("invalid %s: %d", q, w.Code)
		}
	}
	for _, q := range []string{"attemptId=", "attemptId=bad", "limit=0", "attemptOffset=-1", "observationOffset=2147483648"} {
		if w := get(detail(requests[0].ID)+"&"+q, "request-secret"); w.Code != 400 {
			t.Fatalf("invalid detail %s: %d", q, w.Code)
		}
	}
	for _, path := range []string{detail(requests[0].ID) + "&attemptId=" + attemptIDs[requests[1].ID].String(), detail(uuid.New()), strings.Replace(detail(requests[0].ID), resource.String(), uuid.NewString(), 1), strings.Replace(detail(requests[0].ID), ws.String(), foreign.String(), 1)} {
		if w := get(path, "request-secret"); w.Code != 404 {
			t.Fatalf("scope %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, token := range []string{"", "wrong"} {
		for _, path := range []string{list, detail(requests[0].ID)} {
			if w := get(path, token); w.Code != 403 {
				t.Fatalf("credentials %d", w.Code)
			}
		}
	}
	foreignPage := get(strings.Replace(list, ws.String(), foreign.String(), 1), "request-secret")
	var empty infra.UnobservedSchemaRequests
	if json.Unmarshal(foreignPage.Body.Bytes(), &empty) != nil || empty.Page.Total != 0 {
		t.Fatal("foreign workspace leaked")
	}
	if w := get(strings.Replace(list, ws.String(), uuid.NewString(), 1), "request-secret"); w.Code != 403 {
		t.Fatalf("workspace allowlist bypass: %d", w.Code)
	}
	h.Authorize = nil
	for _, path := range []string{list, detail(requests[0].ID)} {
		if w := get(path, "request-secret"); w.Code != 401 {
			t.Fatalf("nil authorizer allowed read: %d", w.Code)
		}
	}
	h.Authorize = auth.Authorize
	if after := fingerprint(); before != after {
		t.Fatal("GET modified history")
	}
	// Two connections establish before/after commit visibility for all five kinds.
	for i, status := range statuses {
		req := requests[2*i+1]
		id := attemptIDs[req.ID]
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		o := d.SchemaObservation{ID: uuid.New(), RequestID: req.ID, AttemptID: &id, Status: status, Reason: "ARRIVED_" + status}
		if status == "MATCH" || status == "DRIFT" {
			now := time.Now().UTC()
			o.Columns = append([]d.SchemaColumn{}, p.Columns...)
			if status == "DRIFT" {
				o.Columns = append(o.Columns, d.SchemaColumn{Name: "added", Type: "TEXT", Constraint: "NULL"})
			}
			o.SchemaHash = d.Hash(o.Columns)
			o.StartedAt = &now
			o.CompletedAt = &now
		}
		if e = repo.AppendSchemaObservation(ctx, tx, o); e != nil {
			tx.Rollback(ctx)
			t.Fatal(e)
		}
		old, e := readRepo.UnobservedSchemaRequests(ctx, ws, "ALL", 100, 0)
		if e != nil || old.Page.Total != 105-i {
			tx.Rollback(ctx)
			t.Fatalf("uncommitted observation leaked: %+v %v", old, e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		out, e := readRepo.UnobservedSchemaRequests(ctx, ws, "ALL", 100, 0)
		if e != nil || out.Page.Total != 104-i {
			t.Fatalf("arrival membership: %+v %v", out, e)
		}
		w := get(detail(req.ID)+"&attemptId="+id.String(), "request-secret")
		var exact infra.SchemaRequestDetail
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &exact) != nil || exact.SelectedAttempt == nil || exact.SelectedAttempt.ID != id || exact.SelectedAttempt.ObservationID == nil || *exact.SelectedAttempt.ObservationID != o.ID || exact.Observations.Page.Total != 1 {
			t.Fatalf("arrival replaced exact identity: %d %s", w.Code, w.Body.String())
		}
		if w = get("/api/v1/data-resources/"+resource.String()+"/schema-observations/"+o.ID.String()+"?workspaceId="+ws.String(), "request-secret"); w.Code != 200 {
			t.Fatal("exact observation link unavailable")
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE resource_binding SET external_fqn=$2 WHERE id=$1`, binding.ID, "current."+resource.String()); err != nil {
		t.Fatal(err)
	}
	w := get(detail(requests[0].ID), "request-secret")
	var frozen infra.SchemaRequestDetail
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &frozen) != nil || frozen.ExternalFQN != p.ExternalFQN || frozen.BindingID != binding.ID || frozen.ResponsibleActorID != actor {
		t.Fatal("mutable binding/actor replaced frozen request")
	}
	readPool.Close()
	if w := get(list, "request-secret"); w.Code != 503 {
		t.Fatalf("read failure reported success: %d", w.Code)
	}
}
