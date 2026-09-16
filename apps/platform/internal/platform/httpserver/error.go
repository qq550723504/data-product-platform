package httpserver

import (
	"encoding/json"
	"net/http"
)

type ErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: RequestID(r.Context()),
	})
}
