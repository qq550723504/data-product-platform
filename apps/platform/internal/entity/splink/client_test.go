package splink

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/resolution"
)

func TestProbeAndGenerateCandidates(t *testing.T) {
	entityID := uuid.New()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status":        "ok",
				"engineName":    "SPLINK",
				"engineVersion": "4.0.17",
				"modelRef":      "park-company-v1",
				"modelVersion":  "1.0.0",
			})
		case "/v1/candidates":
			if r.Header.Get("Authorization") != "Bearer secret" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if req["modelRef"] != "park-company-v1" || req["modelVersion"] != "1.0.0" {
				t.Fatalf("unexpected model binding: %#v", req)
			}
			if req["policyRef"] != "park-company-match" || req["policyVersion"] != "1.0.0" {
				t.Fatalf("unexpected policy binding: %#v", req)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"engine": map[string]any{
					"name":         "SPLINK",
					"version":      "4.0.17",
					"modelVersion": "1.0.0",
				},
				"candidates": []map[string]any{{
					"entityId": entityID.String(),
					"score":    0.87,
					"method":   "FELLEGI_SUNTER",
					"metadata": map[string]any{"matchWeight": 2.73},
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := newTestClient(server.URL, "secret")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Probe(context.Background()); err != nil {
		t.Fatalf("probe: %v", err)
	}
	candidates, err := client.Generate(context.Background(), resolution.CandidateRequest{
		EntityType: "COMPANY",
		Source: resolution.MatchRecord{
			ID: "SRC-1", Name: "Acme Tech", Fields: map[string]string{"normalized_company_name": "ACME TECH"},
		},
		References: []resolution.ReferenceRecord{{
			EntityID: entityID, Name: "Acme Technology", Fields: map[string]string{"normalized_company_name": "ACME TECHNOLOGY"},
		}},
		PolicyRef:     "park-company-match",
		PolicyVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(candidates) != 1 || candidates[0].EntityID != entityID || candidates[0].Score != 0.87 {
		t.Fatalf("unexpected candidates: %#v", candidates)
	}
	if candidates[0].Engine.Version != "4.0.17" || candidates[0].Engine.ModelVersion != "1.0.0" {
		t.Fatalf("missing provenance: %#v", candidates[0].Engine)
	}
}

func TestGenerateMapsProviderFailureToStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "python traceback with private details", http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := newTestClient(server.URL, "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Generate(context.Background(), resolution.CandidateRequest{
		PolicyRef: "park-company-match", PolicyVersion: "1.0.0",
	})
	if err == nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if strings.Contains(err.Error(), "traceback") || strings.Contains(err.Error(), "private") {
		t.Fatalf("raw provider error leaked: %v", err)
	}
}

func TestGenerateRejectsMismatchedPolicyBeforeProviderCall(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client, err := newTestClient(server.URL, "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Generate(context.Background(), resolution.CandidateRequest{
		PolicyRef: "manufacturing-company-match", PolicyVersion: "1.0.0",
	})
	if !errors.Is(err, ErrPolicyMismatch) {
		t.Fatalf("expected ErrPolicyMismatch, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("policy mismatch must not reach Splink provider, calls=%d", calls)
	}
}

func TestProbeRejectsUnexpectedVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "engineName": "SPLINK", "engineVersion": "5.0.0.dev4",
			"modelRef": "park-company-v1", "modelVersion": "1.0.0",
		})
	}))
	defer server.Close()
	client, err := newTestClient(server.URL, "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Probe(context.Background()); err == nil || !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected version contract failure, got %v", err)
	}
}

func TestProbeRejectsMissingModelIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "engineName": "SPLINK", "engineVersion": "4.0.17",
		})
	}))
	defer server.Close()
	client, err := newTestClient(server.URL, "")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Probe(context.Background()); err == nil || !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("expected missing model identity failure, got %v", err)
	}
}

func newTestClient(baseURL, token string) (*Client, error) {
	return NewClient(Config{
		BaseURL:               baseURL,
		Token:                 token,
		ExpectedEngineVersion: "4.0.17",
		ModelRef:              "park-company-v1",
		ModelVersion:          "1.0.0",
		PolicyRef:             "park-company-match",
		PolicyVersion:         "1.0.0",
	}, nil)
}
