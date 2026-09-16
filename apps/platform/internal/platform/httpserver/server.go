package httpserver

import (
	"encoding/json"
	"net/http"
	"time"
)

type Health struct {
	Status    string `json:"status"`
	Time      string `json:"time"`
	RequestID string `json:"requestId,omitempty"`
}

func NewMux(registrars ...func(*http.ServeMux)) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", healthHandler)
	mux.HandleFunc("GET /health/ready", healthHandler)
	for _, register := range registrars {
		if register != nil {
			register(mux)
		}
	}
	return WithRequestID(mux)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Health{
		Status:    "ok",
		Time:      time.Now().UTC().Format(time.RFC3339),
		RequestID: RequestID(r.Context()),
	})
}
