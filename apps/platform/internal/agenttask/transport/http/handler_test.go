package agenttaskhttp

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthenticationAndStrictCommandBoundary(t *testing.T) {
	const ws = "11111111-1111-4111-8111-111111111111"
	resolver, err := principal.NewStaticResolver(true, "test-secret", "test/operator", "22222222-2222-4222-8222-222222222222", []string{ws}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{Principal: resolver, WriteEnabled: true}
	mux := http.NewServeMux()
	h.Register(mux)
	for _, tc := range []struct {
		name, token, body, path string
		status                  int
	}{
		{"anonymous", "", `{}`, "", 401},
		{"bad token", "bad", `{}`, "", 401},
		{"spoof actor", "test-secret", `{"actorId":"22222222-2222-4222-8222-222222222222"}`, "", 400},
		{"duplicate", "test-secret", `{"reason":"a","reason":"b"}`, "", 400},
		{"trailing object", "test-secret", `{} {}`, "", 400},
		{"create cannot approve", "test-secret", `{"approvalId":"22222222-2222-4222-8222-222222222222"}`, "", 400},
		{"pause cannot change plan", "test-secret", `{"goal":"changed"}`, "/22222222-2222-4222-8222-222222222222/pause", 400},
		{"valid reaches service", "test-secret", `{"reason":"test"}`, "", 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/workspaces/"+ws+"/agent-tasks"+tc.path, strings.NewReader(tc.body))
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			r.Header.Set("Idempotency-Key", "test")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "test-secret") {
				t.Fatal("credential leaked")
			}
		})
	}
	h.WriteEnabled = false
	r := httptest.NewRequest("POST", "/api/v1/workspaces/"+ws+"/agent-tasks", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer test-secret")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("writes not disabled", w.Code)
	}
}
