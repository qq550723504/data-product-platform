package remediationhttp

import (
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTrustedWorkspaceAndBodyCannotMintActor(t *testing.T) {
	ws, actor := uuid.New(), uuid.New()
	resolver, err := principal.NewStaticResolver(true, "test-secret", "test-subject", actor.String(), []string{ws.String()}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&application.Service{}, nil, resolver)
	mux := http.NewServeMux()
	handler.Register(mux)
	for _, test := range []struct {
		workspace   uuid.UUID
		token, body string
		status      int
	}{{ws, "", `{"versionId":"` + uuid.NewString() + `"}`, 403}, {uuid.New(), "test-secret", `{}`, 403}, {ws, "test-secret", `{"versionId":"` + uuid.NewString() + `","actorId":"` + uuid.NewString() + `"}`, 400}, {ws, "test-secret", `{} {}`, 400}} {
		r := httptest.NewRequest("POST", "/api/v1/workspaces/"+test.workspace.String()+"/quality-remediations/detect", strings.NewReader(test.body))
		r.Header.Set("Authorization", "Bearer "+test.token)
		r.Header.Set("X-Actor-ID", uuid.NewString())
		r.Header.Set("Idempotency-Key", "test")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != test.status {
			t.Fatalf("status %d want %d", w.Code, test.status)
		}
		if strings.Contains(w.Body.String(), "test-secret") {
			t.Fatal("credential disclosed")
		}
	}
}
