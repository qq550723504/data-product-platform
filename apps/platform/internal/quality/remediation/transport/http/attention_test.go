package remediationhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
)

func TestAttentionAuthenticatesAndRejectsInvalidQueriesBeforeReading(t *testing.T) {
	ws, actor := uuid.New(), uuid.New()
	resolver, err := principal.NewStaticResolver(true, "attention-test-secret", "attention-test", actor.String(), []string{ws.String()}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	NewHandler(&application.Service{}, nil, resolver).Register(mux)
	for _, test := range []struct {
		name, workspace, token, query string
		status                        int
	}{
		{"missing token", ws.String(), "", "", 403},
		{"wrong token", ws.String(), "wrong", "", 403},
		{"wrong workspace", uuid.NewString(), "attention-test-secret", "", 403},
		{"invalid workspace", "invalid", "attention-test-secret", "", 400},
		{"invalid view", ws.String(), "attention-test-secret", "?view=PASS", 400},
		{"invalid limit", ws.String(), "attention-test-secret", "?limit=101", 400},
		{"empty page size", ws.String(), "attention-test-secret", "?limit=0", 400},
		{"negative offset", ws.String(), "attention-test-secret", "?offset=-1", 400},
		{"overflow offset", ws.String(), "attention-test-secret", "?offset=2147483648", 400},
		{"invalid offset", ws.String(), "attention-test-secret", "?offset=no", 400},
		{"unavailable projection", ws.String(), "attention-test-secret", "?view=ALL&limit=1&offset=0", 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/workspaces/"+test.workspace+"/quality-remediations/attention"+test.query, nil)
			r.Header.Set("Authorization", "Bearer "+test.token)
			r.Header.Set("X-Actor-ID", uuid.NewString())
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != test.status || strings.Contains(w.Body.String(), "attention-test-secret") || strings.Contains(w.Body.String(), "\"items\"") {
				t.Fatalf("unexpected projection response: %d %s", w.Code, w.Body.String())
			}
		})
	}

	denied, err := principal.NewStaticResolver(true, "other-test-secret", "attention-other", actor.String(), []string{ws.String()}, []string{"OTHER"})
	if err != nil {
		t.Fatal(err)
	}
	other := http.NewServeMux()
	NewHandler(&application.Service{}, nil, denied).Register(other)
	r := httptest.NewRequest("GET", "/api/v1/workspaces/"+ws.String()+"/quality-remediations/attention", nil)
	r.Header.Set("Authorization", "Bearer other-test-secret")
	w := httptest.NewRecorder()
	other.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("missing capability: %d", w.Code)
	}
}
