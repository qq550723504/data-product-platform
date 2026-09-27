package gx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
)

func TestClientProbeAndEvaluate(t *testing.T) {
	attemptID := uuid.New()
	versionID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok", "engineName": EngineName, "engineVersion": "1.23.2",
				"capabilities": []string{"not_null", "completeness_ratio", "unique", "duplicate_ratio", "range", "enum"},
			})
		case "/v1/evaluate":
			var req evaluateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req.AttemptID != attemptID.String() || req.DatasetVersionID != versionID.String() {
				t.Fatalf("request identity = %s/%s", req.AttemptID, req.DatasetVersionID)
			}
			if req.RuleSetRef != "quality/test.yaml" || req.RuleSetContent == "" {
				t.Fatalf("ruleset = %q/%q", req.RuleSetRef, req.RuleSetContent)
			}
			if len(req.Rows) != 2 || req.Rows[1]["id"] != "" {
				t.Fatalf("rows = %#v", req.Rows)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"engine": map[string]any{
					"name": EngineName, "version": "1.23.2",
					"capabilities": []string{"not_null"},
				},
				"findings": []map[string]any{{
					"ruleId": "R1", "status": "FAIL",
					"observed": map[string]any{"affectedCount": 1, "total": 2, "observedValue": 0.5, "threshold": 1},
				}},
				"execution": map[string]any{"ref": attemptID.String(), "durationMillis": 12},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL, ExpectedEngineVersion: "1.23.2",
	}, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.Probe(context.Background()); err != nil {
		t.Fatalf("probe: %v", err)
	}

	result, err := client.Evaluate(context.Background(), qualityengine.Request{
		AttemptID: attemptID, DatasetVersionID: versionID,
		RuleSet: qualityengine.RuleSet{Ref: "quality/test.yaml", Content: []byte("kind: QualityRuleSet")},
		Dataset: qualityengine.DatasetContext{Table: tabular.Table{
			Headers: []string{"id"},
			Rows:    []map[string]string{{"id": "A"}, {"id": ""}},
		}},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].RuleID != "R1" || result.Findings[0].Status != domain.FindingFail {
		t.Fatalf("findings = %#v", result.Findings)
	}
	if result.Execution.ExecutionRef != attemptID.String() || result.Execution.DurationMillis != 12 {
		t.Fatalf("execution = %#v", result.Execution)
	}
}

func TestClientRejectsProviderIdentityMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"engine":    map[string]any{"name": "other", "version": "1.23.2"},
			"findings":  []any{},
			"execution": map[string]any{"ref": uuid.NewString(), "durationMillis": 1},
		})
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, ExpectedEngineVersion: "1.23.2"}, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Evaluate(context.Background(), qualityengine.Request{
		AttemptID: uuid.New(), DatasetVersionID: uuid.New(),
	})
	if err == nil {
		t.Fatal("provider identity mismatch was accepted")
	}
}

func TestClientClassifiesHTTPClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpClient := server.Client()
	httpClient.Timeout = 10 * time.Millisecond
	client, err := NewClient(Config{
		BaseURL: server.URL, ExpectedEngineVersion: "1.23.2",
	}, httpClient)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Evaluate(context.Background(), qualityengine.Request{
		AttemptID: uuid.New(), DatasetVersionID: uuid.New(),
	})
	if err == nil || err.Error() != "quality engine execution failed: PROVIDER_TIMEOUT" {
		t.Fatalf("timeout error = %v, want PROVIDER_TIMEOUT", err)
	}
}

func TestClientProbeRejectsMissingCapability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "engineName": EngineName, "engineVersion": "1.23.2",
			"capabilities": []string{"not_null"},
		})
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, ExpectedEngineVersion: "1.23.2"}, server.Client())
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if err := client.Probe(context.Background()); err == nil {
		t.Fatal("probe accepted incomplete capability set")
	}
}
