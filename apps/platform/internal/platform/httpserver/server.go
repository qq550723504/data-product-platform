package httpserver

import (
	"encoding/json"
	"net/http"
	"time"
)

type Health struct {
	Status string `json:"status"`
	Time   string `json:"time"`
}

func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", healthHandler)
	mux.HandleFunc("GET /health/ready", healthHandler)
	return mux
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Health{
		Status: "ok",
		Time:   time.Now().UTC().Format(time.RFC3339),
	})
}
