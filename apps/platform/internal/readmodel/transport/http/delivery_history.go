package readmodelhttp

import (
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
)

func (h *Handler) RegisterDeliveryHistoryReadOnly(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/delivery-history", func(w http.ResponseWriter, r *http.Request) { h.readonlyHistory(w, r, false) })
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/evidence-archives", func(w http.ResponseWriter, r *http.Request) { h.readonlyHistory(w, r, true) })
}
func (h *Handler) readonlyHistory(w http.ResponseWriter, r *http.Request, archive bool) {
	ws, ok := workspaceID(w, r)
	if !ok {
		return
	}
	if h.Principal == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted workspace principal required", nil)
		return
	}
	actor, err := h.Principal.Resolve(r, ws, principal.CapabilityHumanDecision)
	if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted workspace principal required", nil)
		return
	}
	if err != nil || actor.ActorID == uuid.Nil || actor.WorkspaceID != ws || !actor.HasCapability(principal.CapabilityHumanDecision) {
		httpserver.WriteError(w, r, 403, "WORKSPACE_AUTHORITY_DENIED", "workspace authority denied", nil)
		return
	}
	for key, values := range r.URL.Query() {
		if (key != "id" && key != "limit" && key != "offset") || len(values) != 1 {
			httpserver.WriteError(w, r, 400, "INVALID_QUERY", "unsupported or repeated query", nil)
			return
		}
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	var selected *uuid.UUID
	if values, exists := r.URL.Query()["id"]; exists {
		id, e := uuid.Parse(values[0])
		if e != nil || id == uuid.Nil {
			httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "explicit UUID required", nil)
			return
		}
		selected = &id
	}
	result, err := h.repo.ReadonlyHistory(r.Context(), ws, archive, selected, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, 500, "HISTORY_READ_FAILED", "history unavailable", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"workspaceId": ws, "items": result.Items, "page": result.Page})
}
