package readmodelhttp

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeliveryArchiveReadBoundary(t *testing.T) {
	resolver, err := principal.NewStaticResolver(true, "test-token", "operator", "22222222-2222-4222-8222-222222222222", []string{"11111111-1111-4111-8111-111111111111"}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{Principal: resolver}
	mux := http.NewServeMux()
	h.RegisterDeliveryHistoryReadOnly(mux)
	for _, endpoint := range []string{"delivery-history", "evidence-archives"} {
		for _, tc := range []struct {
			method, ws, query, token string
			status                   int
		}{
			{"GET", "11111111-1111-4111-8111-111111111111", "", "", 401},
			{"GET", "33333333-3333-4333-8333-333333333333", "", "test-token", 403},
			{"GET", "11111111-1111-4111-8111-111111111111", "?id=bad", "test-token", 400},
			{"GET", "11111111-1111-4111-8111-111111111111", "?offset=-1", "test-token", 400},
			{"GET", "11111111-1111-4111-8111-111111111111", "?id=&id=x", "test-token", 400},
			{"GET", "11111111-1111-4111-8111-111111111111", "?consumer=forged", "test-token", 400},
			{"POST", "11111111-1111-4111-8111-111111111111", "", "test-token", 405},
		} {
			r := httptest.NewRequest(tc.method, "/api/v1/workspaces/"+tc.ws+"/"+endpoint+tc.query, nil)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%s %s%s: got %d want %d", endpoint, tc.method, tc.query, w.Code, tc.status)
			}
		}
	}
}
