package readmodelhttp

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHistoryRegistrationRequiresPrincipalAndExcludesCommands(t *testing.T) {
	mux := http.NewServeMux()
	h := &Handler{}
	h.RegisterReleaseHistoryReadOnly(mux)
	path := "/api/v1/workspaces/11111111-1111-4111-8111-111111111111/product-releases"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 401 {
		t.Fatalf("missing principal reached repository: %d", w.Code)
	}
	resolver, err := principal.NewStaticResolver(true, "test-token", "test-actor", "22222222-2222-4222-8222-222222222222", []string{"11111111-1111-4111-8111-111111111111"}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	h.Principal = resolver
	r := httptest.NewRequest("GET", strings.Replace(path, "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", 1), nil)
	r.Header.Set("Authorization", "Bearer test-token")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("foreign scope reached repository: %d", w.Code)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
	if w.Code != 404 && w.Code != 405 {
		t.Fatalf("command exposed: %d", w.Code)
	}
}
