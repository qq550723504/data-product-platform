package main

import (
	agenthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPreviewDefaultsReadOnlyAndRefusesRetainedDatabase(t *testing.T) {
	for _, value := range []string{"", "false"} {
		cfg, enabled, err := previewConfig("postgres://reader@127.0.0.1/agent_q2_acceptance_unit", value)
		if err != nil || enabled || cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] != "on" {
			t.Fatal("read-only default missing")
		}
	}
	for _, dsn := range []string{"", "postgres://reader@127.0.0.1/agent_q2_test"} {
		if _, _, err := previewConfig(dsn, ""); err == nil {
			t.Fatal("retained/implicit database accepted")
		}
	}
	if _, _, err := previewConfig("postgres://reader@127.0.0.1/agent_q2_acceptance_unit", "invalid"); err == nil {
		t.Fatal("invalid switch accepted")
	}
}
func TestReadonlyPreviewRejectsAuthenticatedCommandBeforeService(t *testing.T) {
	ws := "11111111-1111-4111-8111-111111111111"
	resolver, err := principal.NewStaticResolver(true, "unit-only", "reader", "22222222-2222-4222-8222-222222222222", []string{ws}, []string{principal.CapabilityHumanDecision})
	if err != nil {
		t.Fatal(err)
	}
	h := agenthttp.Handler{Principal: resolver, WriteEnabled: false}
	mux := http.NewServeMux()
	h.Register(mux)
	r := httptest.NewRequest("POST", "/api/v1/workspaces/"+ws+"/agent-tasks", strings.NewReader(`{}`))
	r.Header.Set("Authorization", "Bearer unit-only")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "AGENT_WRITES_DISABLED") {
		t.Fatalf("command not rejected before nil service: %d", w.Code)
	}
}
