package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Health struct {
	Status    string `json:"status"`
	Time      string `json:"time"`
	RequestID string `json:"requestId,omitempty"`
}

type ReadinessCheck func(context.Context) error

func NewMux(registrars ...func(*http.ServeMux)) http.Handler {
	// Preserve the existing lightweight mux contract for standalone tests.
	return NewMuxWithReadiness(func(context.Context) error { return nil }, registrars...)
}

// NewMuxWithReadiness separates dependency-sensitive readiness from liveness.
// A missing check fails closed rather than reporting a false positive.
func NewMuxWithReadiness(check ReadinessCheck, registrars ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", healthHandler)
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if check == nil {
			writeHealth(w, r, http.StatusServiceUnavailable, "unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := check(ctx); err != nil {
			writeHealth(w, r, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeHealth(w, r, http.StatusOK, "ok")
	})
	for _, register := range registrars {
		if register != nil {
			register(mux)
		}
	}
	return WithRequestID(mux)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeHealth(w, r, http.StatusOK, "ok")
}

func writeHealth(w http.ResponseWriter, r *http.Request, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Health{
		Status: state,
		Time: time.Now().UTC().Format(time.RFC3339),
		RequestID: RequestID(r.Context()),
	})
}
