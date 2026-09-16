package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthIncludesRequestID(t *testing.T) {
	handler := NewMux()
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	req.Header.Set(RequestIDHeader, "test-request-id")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get(RequestIDHeader); got != "test-request-id" {
		t.Fatalf("request id header = %q, want %q", got, "test-request-id")
	}

	var response Health
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %q, want ok", response.Status)
	}
	if response.RequestID != "test-request-id" {
		t.Fatalf("response request id = %q, want test-request-id", response.RequestID)
	}
}

func TestRequestIDGeneratedWhenMissing(t *testing.T) {
	handler := NewMux()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get(RequestIDHeader) == "" {
		t.Fatal("expected generated request id header")
	}
}
