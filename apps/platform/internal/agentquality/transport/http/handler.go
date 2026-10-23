package agentqualityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agentquality"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type Handler struct {
	Service   *agentquality.Service
	Principal principal.Resolver
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/agent-quality-context", h.read)
}
func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ws, err := uuid.Parse(r.PathValue("workspaceId"))
	if err != nil || ws == uuid.Nil {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE", "valid workspace required", nil)
		return
	}
	// Authenticate before touching repositories or returning template configuration.
	if h.Principal == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted Quality principal required", nil)
		return
	}
	p, err := h.Principal.Resolve(r, ws, principal.CapabilityHumanDecision)
	if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted Quality principal required", nil)
		return
	}
	if err != nil || p.ActorID == uuid.Nil || p.WorkspaceID != ws || !p.HasCapability(principal.CapabilityHumanDecision) {
		httpserver.WriteError(w, r, 403, "QUALITY_AUTHORITY_DENIED", "Quality workspace authority denied", nil)
		return
	}
	q := agentquality.Query{WorkspaceID: ws, Limit: 25}
	values := r.URL.Query()
	// A single explicit contract avoids ambiguous duplicate selection/scope values.
	for key, value := range values {
		if len(value) != 1 {
			httpserver.WriteError(w, r, 400, "INVALID_QUERY", "ambiguous context query", nil)
			return
		}
		switch key {
		case "datasetId", "versionId", "limit", "offset":
		default:
			httpserver.WriteError(w, r, 400, "INVALID_QUERY", "unsupported context query", nil)
			return
		}
	}
	for key, dst := range map[string]*uuid.UUID{"datasetId": &q.DatasetID, "versionId": &q.VersionID} {
		if values.Has(key) {
			*dst, err = uuid.Parse(values.Get(key))
			if err != nil || *dst == uuid.Nil {
				httpserver.WriteError(w, r, 400, "INVALID_QUERY", "valid source identity required", nil)
				return
			}
		}
	}
	for key, dst := range map[string]*int{"limit": &q.Limit, "offset": &q.Offset} {
		if values.Has(key) {
			*dst, err = strconv.Atoi(values.Get(key))
			if err != nil || *dst < 0 || *dst > 2147483647 {
				httpserver.WriteError(w, r, 400, "INVALID_QUERY", "invalid pagination", nil)
				return
			}
		}
	}
	if q.Limit < 1 || q.Limit > 100 {
		httpserver.WriteError(w, r, 400, "INVALID_QUERY", "limit must be between 1 and 100", nil)
		return
	}
	if h.Service == nil || h.Service.Reader == nil {
		httpserver.WriteError(w, r, 503, "CONTEXT_UNAVAILABLE", "context unavailable", nil)
		return
	}
	result, err := h.Service.Read(r.Context(), q)
	if errors.Is(err, agentquality.ErrNotFound) {
		httpserver.WriteError(w, r, 404, "SOURCE_NOT_FOUND", "source not found in workspace", nil)
		return
	}
	if err != nil {
		httpserver.WriteError(w, r, 503, "CONTEXT_UNAVAILABLE", "context unavailable", nil)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
