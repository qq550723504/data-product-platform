package splink

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	client, err := NewClient(Config{
		BaseURL:               server.URL,
		Token:                 "secret",
		ExpectedEngineVersion: "4.0.17",
		ModelRef:              "park-company-v1",
		ModelVersion:          "1.0.0",
	}, nil)
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
		PolicyRef: "park-company-match",
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
	client, err := NewClient(Config{
		BaseURL: server.URL, ExpectedEngineVersion: "4.0.17", ModelRef: "m", ModelVersion: "1",
	}, nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Generate(context.Background(), resolution.CandidateRequest{})
	if err == nil || !errorsIs(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
	if contains(err.Error(), "traceback") || contains(err.Error(), "private") {
		t.Fatalf("raw provider error leaked: %v", err)
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
	client, err := NewClient(Config{
		BaseURL: server.URL, ExpectedEngineVersion: "4.0.17", ModelRef: "park-company-v1", ModelVersion: "1.0.0",
	}, nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.Probe(context.Background()); err == nil || !errorsIs(err, ErrInvalidResponse) {
		t.Fatalf("expected version contract failure, got %v", err)
	}
}

func errorsIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func contains(value, sub string) bool {
	for i := 0; i+len(sub) <= len(value); i++ {
		if value[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
