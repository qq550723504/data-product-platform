package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

func TestReadinessDependencyFailureDoesNotAffectLiveness(t *testing.T) {
	handler := NewMuxWithReadiness(func(context.Context) error { return errors.New("database offline") })
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/health/ready", http.StatusServiceUnavailable},
		{"/health/live", http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.Header.Set(RequestIDHeader, "readiness-test")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s = %d, want %d", tc.path, rec.Code, tc.want)
		}
		var response Health
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode %s response: %v", tc.path, err)
		}
		if response.RequestID != "readiness-test" {
			t.Errorf("%s lost request ID", tc.path)
		}
		if response.Status == "ok" && tc.path == "/health/ready" {
			t.Error("failed readiness returned ok status")
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s must not be cached", tc.path)
		}
	}
}

func TestReadinessMissingProbeFailsClosed(t *testing.T) {
	handler := NewMuxWithReadiness(nil)
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing readiness probe = %d, want 503", rec.Code)
	}
}

func TestReadinessUsesBoundedContext(t *testing.T) {
	handler := NewMuxWithReadiness(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 3*time.Second {
			return errors.New("readiness probe has no short deadline")
		}
		return nil
	})
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bounded readiness = %d, want 200", rec.Code)
	}
}
