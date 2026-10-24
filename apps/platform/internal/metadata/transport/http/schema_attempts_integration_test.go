package metadatahttp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestUnobservedAttemptsInObservedRequestsReadOnly(t *testing.T) {
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
	plan := d.SchemaPlan{Enabled: true, WorkspaceID: ws, ResourceID: resource, ActorID: actor, AuthorizationID: authorization,
		ConsumerRef: "TEST", Purpose: "TEST", ExternalFQN: "attempts." + resource.String(), RuleRef: "test/schema", RuleVersion: "1", RuleHash: fmt.Sprintf("%064d", 1), ConnectorHash: fmt.Sprintf("%064d", 2), Columns: []d.SchemaColumn{{Name: "value", Type: "TEXT", Constraint: "NOT_NULL"}}}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'attempt fixture','TABLE_LIKE')`, resource, ws, resource.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, authorization, ws, authorization.String()); err != nil {
		t.Fatal(err)
	}
	binding, err := repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: resource, Provider: "TEST_METADATA", EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: plan.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	request := func(key string) d.SchemaRequest {
		t.Helper()
		r, _, e := repo.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: plan, PlanHash: d.Hash(plan), BindingID: binding.ID, ExternalID: binding.ExternalID, Key: key})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	attempt := func(r d.SchemaRequest) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if e := repo.BeginSchemaAttempt(ctx, tx, r, id, json.RawMessage(`{"fixture":"private-attempt-runtime"}`)); e != nil {
			t.Fatal(e)
		}
		return id
	}
	history := request("observed")
	// Many old observations must not multiply the 105 unobserved attempt rows.
	for i := 0; i < 31; i++ {
		a := attempt(history)
		if e := repo.AppendSchemaObservation(ctx, tx, d.SchemaObservation{ID: uuid.New(), RequestID: history.ID, AttemptID: &a, Status: "UNAVAILABLE", Reason: "OLD_UNAVAILABLE"}); e != nil {
			t.Fatal(e)
		}
	}
	pending := []uuid.UUID{}
	for i := 0; i < 105; i++ {
		pending = append(pending, attempt(history))
	}
	noHistory := request("no-observations")
	attempt(noHistory)
	request("no-attempt")
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	reads, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer reads.Close()
	readRepo := infra.NewPostgresRepository(reads)
	service := app.NewService("TEST_METADATA", "", nil, readRepo, nil, nil)
	service.Schema = &app.SchemaService{Observer: forbiddenSchemaObserver{}}
	h := NewHandler(service, readRepo)
	auth, err := rightshttp.NewStaticAuthorizer("attempt-secret", uuid.NewString(), []string{ws.String(), foreign.String()})
	if err != nil {
		t.Fatal(err)
	}
	h.Authorize = auth.Authorize
	mux := http.NewServeMux()
	h.Register(mux)
	get := func(path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Actor-ID", uuid.NewString())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	list := "/api/v1/workspaces/" + ws.String() + "/source-schema-requests/observed/unobserved-attempts"
	detail := "/api/v1/data-resources/" + resource.String() + "/schema-requests/" + history.ID.String() + "?workspaceId=" + ws.String()
	read := func(path string) infra.SchemaUnobservedAttempts {
		t.Helper()
		w := get(path, "attempt-secret")
		var out infra.SchemaUnobservedAttempts
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("read %d: %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "private-attempt-runtime") || strings.Contains(w.Body.String(), "attempt-secret") {
			t.Fatal("private contract leaked")
		}
		return out
	}
	fingerprint := func() string {
		t.Helper()
		var out string
		if e := pool.QueryRow(ctx, `SELECT md5(jsonb_build_array(
 (SELECT jsonb_agg(to_jsonb(r) ORDER BY r.id) FROM source_schema_request r WHERE workspace_id=$1),
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM source_schema_attempt a WHERE workspace_id=$1),
 (SELECT jsonb_agg(to_jsonb(o) ORDER BY o.id) FROM source_schema_observation o JOIN source_schema_request r ON r.id=o.request_id WHERE r.workspace_id=$1),
 (SELECT jsonb_agg(to_jsonb(c) ORDER BY c.id) FROM cost_event c WHERE workspace_id=$1))::text)`, ws).Scan(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	before := fingerprint()
	seen := map[uuid.UUID]bool{}
	for offset := 0; offset < 105; offset += 25 {
		out := read(fmt.Sprintf("%s?limit=25&offset=%d", list, offset))
		if out.Page.Total != 105 || out.Page.Offset != offset || out.Page.Limit != 25 || out.ReadAt.IsZero() {
			t.Fatalf("page/count mismatch: %+v", out)
		}
		for _, a := range out.Items {
			if seen[a.AttemptID] || a.RequestID != history.ID || a.WorkspaceID != ws || a.ResourceID != resource || a.ResponsibleActorID != actor || a.RequestObservationCount != 31 || a.AttemptCount != 136 {
				t.Fatalf("wrong attempt facts: %+v", a)
			}
			seen[a.AttemptID] = true
		}
	}
	if len(seen) != 105 {
		t.Fatalf("lost pagination: %d", len(seen))
	}
	old, err := readRepo.UnobservedSchemaRequests(ctx, ws, "ALL", 25, 0)
	if err != nil || old.Page.Total != 2 {
		t.Fatalf("old request collection changed: %+v %v", old, err)
	}
	selected := pending[0]
	w := get(detail+"&attemptId="+selected.String()+"&attemptOffset=25&observationOffset=25", "attempt-secret")
	var exact infra.SchemaRequestDetail
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &exact) != nil || exact.SelectedAttempt == nil || exact.SelectedAttempt.ID != selected || exact.SelectedAttempt.ObservationID != nil {
		t.Fatalf("selected attempt unavailable: %s", w.Body.String())
	}
	for _, a := range exact.Attempts.Items {
		if a.ID == selected {
			t.Fatal("fixture did not test off-page selection")
		}
	}
	for _, query := range []string{"?limit=0", "?limit=101", "?offset=-1", "?offset=2147483648", "?limit=nope"} {
		if w := get(list+query, "attempt-secret"); w.Code != 400 {
			t.Fatalf("invalid pagination accepted: %s %d", query, w.Code)
		}
	}
	for _, path := range []string{list, detail} {
		for _, token := range []string{"", "wrong"} {
			if w := get(path, token); w.Code != 403 {
				t.Fatalf("credentials accepted: %d", w.Code)
			}
		}
	}
	if out := read(strings.Replace(list, ws.String(), foreign.String(), 1)); out.Page.Total != 0 || len(out.Items) != 0 {
		t.Fatal("workspace leaked")
	}
	if w := get(strings.Replace(list, ws.String(), uuid.NewString(), 1), "attempt-secret"); w.Code != 403 {
		t.Fatal("workspace allowlist bypassed")
	}
	for _, path := range []string{strings.Replace(detail, resource.String(), uuid.NewString(), 1), strings.Replace(detail, ws.String(), foreign.String(), 1), strings.Replace(detail, history.ID.String(), noHistory.ID.String(), 1) + "&attemptId=" + selected.String(), detail + "&attemptId=" + uuid.NewString()} {
		if w := get(path, "attempt-secret"); w.Code != 404 {
			t.Fatalf("identity mismatch accepted: %d", w.Code)
		}
	}
	h.Authorize = nil
	if w := get(list, "attempt-secret"); w.Code != 401 {
		t.Fatal("nil authorizer accepted")
	}
	h.Authorize = auth.Authorize
	if fingerprint() != before {
		t.Fatal("GET changed durable facts")
	}
	// An unbound observation cannot conclude any of the selected attempts.
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.AppendSchemaObservation(ctx, tx, d.SchemaObservation{ID: uuid.New(), RequestID: history.ID, Status: "DENIED", Reason: "UNBOUND"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if out := read(list); out.Page.Total != 105 {
		t.Fatal("unbound observation removed attempts")
	}
	// Two-connection visibility for each observation type; only its own attempt exits.
	for i, status := range []string{"MATCH", "DRIFT", "UNAVAILABLE", "DENIED", "INTERRUPTED"} {
		a := pending[i]
		tx, err = pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		o := d.SchemaObservation{ID: uuid.New(), RequestID: history.ID, AttemptID: &a, Status: status, Reason: "ARRIVED_" + status}
		if status == "MATCH" || status == "DRIFT" {
			now := time.Now().UTC()
			o.Columns = append([]d.SchemaColumn{}, plan.Columns...)
			if status == "DRIFT" {
				o.Columns = append(o.Columns, d.SchemaColumn{Name: "extra", Type: "TEXT", Constraint: "NULL"})
			}
			o.SchemaHash = d.Hash(o.Columns)
			o.StartedAt = &now
			o.CompletedAt = &now
		}
		if err = repo.AppendSchemaObservation(ctx, tx, o); err != nil {
			t.Fatal(err)
		}
		if out := read(list); out.Page.Total != 105-i {
			t.Fatal("uncommitted observation became visible")
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if out := read(list); out.Page.Total != 104-i {
			t.Fatal("observation removed wrong collection members")
		}
		w = get(detail+"&attemptId="+a.String(), "attempt-secret")
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &exact) != nil || exact.SelectedAttempt == nil || exact.SelectedAttempt.ID != a || exact.SelectedAttempt.ObservationID == nil || *exact.SelectedAttempt.ObservationID != o.ID {
			t.Fatal("arrival replaced selected attempt")
		}
		if w = get("/api/v1/data-resources/"+resource.String()+"/schema-observations/"+o.ID.String()+"?workspaceId="+ws.String(), "attempt-secret"); w.Code != 200 {
			t.Fatal("exact observation unavailable")
		}
	}
	var historical int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM source_schema_observation WHERE request_id=$1 AND reason='OLD_UNAVAILABLE'`, history.ID).Scan(&historical); err != nil || historical != 31 {
		t.Fatal("old observations changed")
	}
	reads.Close()
	if w := get(list, "attempt-secret"); w.Code != 503 {
		t.Fatalf("database failure became successful empty: %d", w.Code)
	}
}
