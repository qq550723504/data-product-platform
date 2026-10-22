package metadatahttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

func TestSuccessfulResourceLookupStillReturnsNotFoundOutsideWorkspace(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	pool, err := database.Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	resource, workspace, actor := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(context.Background(), `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'HTTP lookup','TABLE_LIKE')`, resource, uuid.New(), "META-"+resource.String()); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil, metadatainfra.NewPostgresRepository(pool))
	h.Authorize = func(*http.Request, uuid.UUID) (*uuid.UUID, error) { return &actor, nil }
	mux := http.NewServeMux()
	h.Register(mux)
	for _, id := range []uuid.UUID{resource, uuid.New()} {
		for _, endpoint := range []struct{ method, path string }{
			{http.MethodPost, "metadata-bindings"},
			{http.MethodGet, "metadata-bindings"},
			{http.MethodGet, "metadata-observation"},
		} {
			req := httptest.NewRequest(endpoint.method, "/api/v1/data-resources/"+id.String()+"/"+endpoint.path+"?workspaceId="+workspace.String(), strings.NewReader(`{}`))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "RESOURCE_NOT_FOUND") {
				t.Fatalf("%s %s successful lookup: %d %s, want 404", endpoint.method, endpoint.path, recorder.Code, recorder.Body.String())
			}
		}
	}
}
