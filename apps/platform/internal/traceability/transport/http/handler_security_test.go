package traceabilityhttp

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTraceBoundaryBeforeRepository(t *testing.T) {
	ws := uuid.NewString()
	other := uuid.NewString()
	resolver, err := principal.NewStaticResolver(true, "test-token", "operator", uuid.NewString(), []string{ws}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(nil)
	h.Principal = resolver
	mux := http.NewServeMux()
	h.Register(mux)
	for _, tc := range []struct {
		query, token string
		want         int
	}{{"", "", 400}, {"?workspaceId=" + ws + "&workspaceId=" + other, "test-token", 400}, {"?workspaceId=" + ws, "", 401}, {"?workspaceId=" + other, "test-token", 403}, {"?workspaceId=" + ws, "wrong", 401}, {"?workspaceId=" + ws, "test-token", 503}} {
		r := httptest.NewRequest("GET", "/api/v1/traceability/EXECUTION/"+uuid.NewString()+tc.query, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d: %s", w.Code, tc.want, w.Body)
		}
		if w.Header().Get("Cache-Control") != "private, no-store" {
			t.Fatal("read must not be cached")
		}
	}
}
func TestTraceProjectionExcludesSecrets(t *testing.T) {
	raw := map[string]any{"releaseId": "release", "metadata": map[string]any{"secret": "secret"}, "evidence": []any{map[string]any{"id": "evidence", "storageUri": "secret", "title": "secret", "hashValue": "hash"}}, "executions": []any{map[string]any{"id": "execution", "metrics": map[string]any{"secret": "secret"}}}}
	encoded, _ := json.Marshal(redactTrace(raw))
	if strings.Contains(string(encoded), "secret") {
		t.Fatal(string(encoded))
	}
	if !strings.Contains(string(encoded), "hash") {
		t.Fatal("integrity lost")
	}
}

func TestReleaseTraceRequiresTrustedPrincipalBeforeRootLookup(t *testing.T) {
	ws := uuid.NewString()
	resolver, _ := principal.NewStaticResolver(true, "test-token", "operator", uuid.NewString(), []string{ws}, []string{principal.CapabilityHumanDecision})
	h := NewHandler(traceability.NewRepository(nil))
	h.Principal = resolver
	mux := http.NewServeMux()
	h.Register(mux)
	for _, tc := range []struct {
		ws, token string
		want      int
	}{{ws, "", 401}, {uuid.NewString(), "test-token", 403}} {
		req := httptest.NewRequest("GET", "/api/v1/traceability/product-releases/"+uuid.NewString()+"?workspaceId="+tc.ws, nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		req.Header.Set("X-Actor-ID", uuid.NewString())
		req.Header.Set("X-Workspace-ID", ws)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("got %d want %d", w.Code, tc.want)
		}
	}
	h.Principal = nil
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/traceability/product-releases/"+uuid.NewString()+"?workspaceId="+ws, nil))
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
