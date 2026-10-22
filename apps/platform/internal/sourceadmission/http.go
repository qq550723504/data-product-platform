package sourceadmission

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type Handler struct {
	Service  *Service
	Resolver principal.Resolver
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/source-admissions", h.admit)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/source-admissions/{operationId}", h.lookup)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/source-admissions", h.lookupKey)
}
func (h *Handler) lookupKey(w http.ResponseWriter, r *http.Request) {
	ws, e := uuid.Parse(r.PathValue("workspaceId"))
	if e != nil {
		respond(w, nil, ErrDenied)
		return
	}
	rec, e := h.Service.LookupKey(r.Context(), ws, r.URL.Query().Get("key"), h.caller(r))
	respond(w, rec, e)
}
func (h *Handler) caller(r *http.Request) Caller {
	return func(_ context.Context, ws uuid.UUID) (principal.RequestPrincipal, error) {
		if h.Resolver == nil {
			return principal.RequestPrincipal{}, ErrDenied
		}
		return h.Resolver.Resolve(r, ws, Capability)
	}
}
func respond(w http.ResponseWriter, value any, e error) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if e != nil {
		code := "SOURCE_ADMISSION_UNAVAILABLE"
		status := 503
		switch {
		case errors.Is(e, ErrDenied):
			code = ErrDenied.Error()
			status = 403
		case errors.Is(e, ErrConflict):
			code = ErrConflict.Error()
			status = 409
		case errors.Is(e, ErrPublicationConflict):
			code = ErrPublicationConflict.Error()
			status = 409
		case errors.Is(e, ErrIntegrity):
			code = ErrIntegrity.Error()
			status = 409
		case errors.Is(e, ErrUnsupported):
			code = ErrUnsupported.Error()
			status = 422
		case errors.Is(e, ErrUnknown):
			code = ErrUnknown.Error()
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code}})
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
func (h *Handler) admit(w http.ResponseWriter, r *http.Request) {
	ws, e := uuid.Parse(r.PathValue("workspaceId"))
	if e != nil {
		respond(w, nil, ErrDenied)
		return
	}
	caller := h.caller(r)
	if _, e = caller(r.Context(), ws); e != nil {
		respond(w, nil, ErrDenied)
		return
	}
	var req Request
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF {
		respond(w, nil, ErrConflict)
		return
	}
	req.WorkspaceID = ws
	req.Key = r.Header.Get("Idempotency-Key")
	rec, e := h.Service.Admit(r.Context(), req, caller)
	respond(w, rec, e)
}
func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	ws, e := uuid.Parse(r.PathValue("workspaceId"))
	if e != nil {
		respond(w, nil, ErrDenied)
		return
	}
	id, e := uuid.Parse(r.PathValue("operationId"))
	if e != nil {
		respond(w, nil, ErrDenied)
		return
	}
	rec, e := h.Service.Lookup(r.Context(), ws, id, h.caller(r))
	respond(w, rec, e)
}
