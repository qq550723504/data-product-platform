package metadatahttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

// Persisted HTTP query contract; controlled facts/principal, no provider acceptance.
func TestSourceSchemaHistorySurvivesDisabledScanner(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repo := infra.NewPostgresRepository(pool)
	p := d.SchemaPlan{Enabled: true, WorkspaceID: uuid.New(), ResourceID: uuid.New(), ActorID: uuid.New(), AuthorizationID: uuid.New(), ConsumerRef: "TEST", Purpose: "TEST", ExternalFQN: "test." + uuid.NewString(), RuleRef: "test/schema", RuleVersion: "1", RuleHash: strings.Repeat("a", 64), ConnectorHash: strings.Repeat("b", 64), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'history query test','TABLE_LIKE')`, p.ResourceID, p.WorkspaceID, p.ResourceID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, p.AuthorizationID, p.WorkspaceID, p.AuthorizationID.String()); err != nil {
		t.Fatal(err)
	}
	b, err := repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: p.ResourceID, Provider: "TEST_METADATA", EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: p.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	req, _, err := repo.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: p, PlanHash: d.Hash(p), BindingID: b.ID, ExternalID: b.ExternalID, Key: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	attempt, observation := uuid.New(), uuid.New()
	if err = repo.BeginSchemaAttempt(ctx, tx, req, attempt, []byte(`{"fixture":"history-runtime"}`)); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err = repo.AppendSchemaObservation(ctx, tx, d.SchemaObservation{ID: observation, RequestID: req.ID, AttemptID: &attempt, Status: "DRIFT", Reason: "SOURCE_SCHEMA_CHANGED", Columns: p.Columns, SchemaHash: d.Hash(p.Columns), StartedAt: &now, CompletedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.HandleSchema(ctx, tx, p.ResourceID, p.WorkspaceID, p.ActorID, observation, uuid.NewString(), "investigating"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var original string
	for _, service := range []*app.Service{nil, app.NewService("TEST_METADATA", "", nil, repo, nil, nil)} {
		h := NewHandler(service, repo)
		h.Authorize = func(*http.Request, uuid.UUID) (*uuid.UUID, error) { return &p.ActorID, nil }
		mux := http.NewServeMux()
		h.Register(mux)
		get := func(workspace uuid.UUID) *httptest.ResponseRecorder {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/data-resources/"+p.ResourceID.String()+"/schema-observations?workspaceId="+workspace.String(), nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			return w
		}
		w := get(p.WorkspaceID)
		var result struct {
			Enabled bool `json:"enabled"`
			Items   []struct {
				ID           uuid.UUID `json:"id"`
				Observations []struct {
					ID     uuid.UUID `json:"id"`
					Status string    `json:"status"`
				} `json:"observations"`
				Handling []map[string]any `json:"handling"`
			} `json:"items"`
		}
		if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || result.Enabled || len(result.Items) != 1 || result.Items[0].ID != req.ID || len(result.Items[0].Observations) != 1 || result.Items[0].Observations[0].ID != observation || result.Items[0].Observations[0].Status != "DRIFT" || len(result.Items[0].Handling) != 1 {
			t.Fatalf("disabled scanner hid history: %d %s", w.Code, w.Body.String())
		}
		if original != "" && original != w.Body.String() {
			t.Fatal("immutable HTTP history changed")
		}
		original = w.Body.String()
		if foreign := get(uuid.New()); foreign.Code != http.StatusNotFound {
			t.Fatalf("foreign workspace read history: %d", foreign.Code)
		}
		h.Authorize = nil
		if anonymous := get(p.WorkspaceID); anonymous.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous read history: %d", anonymous.Code)
		}
	}
}
