package qualityhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

func TestAllQualityRoutesRejectBeforeRepositoryOrEngine(t *testing.T) {
	ws, actor, id := uuid.New(), uuid.New(), uuid.NewString()
	routes := []struct{ method, path string }{
		{"GET", "/quality-results/" + id},
		{"GET", "/quality-assessments/" + id},
		{"GET", "/quality-assessments/" + id + "/report"},
		{"GET", "/quality-assessment-attempts/" + id},
		{"GET", "/dataset-versions/" + id + "/quality-assessments"},
		{"GET", "/dataset-versions/" + id + "/quality-assessments/latest"},
		{"POST", "/dataset-versions/" + id + "/quality-checks"},
		{"POST", "/dataset-versions/" + id + "/gold-quality-checks"},
	}
	for _, scenario := range []string{"nil", "disabled", "missing", "wrong", "foreign", "capability"} {
		t.Run(scenario, func(t *testing.T) {
			allowedWS := ws
			caps := []string{principal.CapabilityHumanDecision}
			if scenario == "foreign" {
				allowedWS = uuid.New()
			}
			if scenario == "capability" {
				caps = []string{"OTHER"}
			}
			resolver, err := principal.NewStaticResolver(scenario != "disabled", "quality-secret", "quality-service", actor.String(), []string{allowedWS.String()}, caps)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "nil" {
				resolver = nil
			}
			// Nil dependencies make any query or engine invocation fail the test.
			h := &Handler{Principal: resolver}
			mux := http.NewServeMux()
			h.Register(mux)
			for _, route := range routes {
				t.Run(route.method+route.path, func(t *testing.T) {
					req := httptest.NewRequest(route.method, "/api/v1"+route.path+"?workspaceId="+ws.String(), strings.NewReader(`{"workspaceId":"`+ws.String()+`","assessmentAttemptId":"`+id+`"}`))
					req.Header.Set("X-Actor-ID", actor.String())
					if scenario != "missing" {
						req.Header.Set("Authorization", "Bearer quality-secret")
					}
					if scenario == "wrong" {
						req.Header.Set("Authorization", "Bearer forged-secret")
					}
					response := httptest.NewRecorder()
					mux.ServeHTTP(response, req)
					want := 401
					if scenario == "foreign" || scenario == "capability" {
						want = 403
					}
					if response.Code != want {
						t.Fatalf("status=%d body=%s want=%d", response.Code, response.Body, want)
					}
					if strings.Contains(response.Body.String(), "secret") {
						t.Fatal("credential exposed")
					}
				})
			}
		})
	}
}

func TestQualityActorComesFromTrustedPrincipal(t *testing.T) {
	ws, actor := uuid.New(), uuid.New()
	resolver, err := principal.NewStaticResolver(true, "quality-secret", "quality-service", actor.String(), []string{ws.String()}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	for _, forged := range []string{uuid.NewString(), "invalid-actor", ""} {
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("Authorization", "Bearer quality-secret")
		r.Header.Set("X-Actor-ID", forged)
		got, ok := (&Handler{Principal: resolver}).authorizeWorkspace(httptest.NewRecorder(), r, ws)
		if !ok || got == nil || *got != actor {
			t.Fatalf("actor=%v allowed=%v", got, ok)
		}
	}
}
