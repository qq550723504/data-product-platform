package metadatahttp

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

func (h *Handler) schemaAttention(w http.ResponseWriter, r *http.Request) {
	ws, err := uuid.Parse(r.PathValue("workspaceId"))
	if err != nil || ws == uuid.Nil {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE_ID", "workspace required", nil)
		return
	}
	if h.Authorize == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted principal required", nil)
		return
	}
	if actor, err := h.Authorize(r, ws); err != nil || actor == nil {
		httpserver.WriteError(w, r, 403, "AUTHORITY_DENIED", "workspace authority denied", nil)
		return
	}
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "UNHANDLED"
	}
	if !infra.ValidSchemaAttentionView(view) {
		httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_ATTENTION_VIEW", "view must be UNHANDLED or ALL", nil)
		return
	}
	limit, offset := 25, 0
	for _, param := range []struct {
		name  string
		value *int
	}{{"limit", &limit}, {"offset", &offset}} {
		if raw := r.URL.Query().Get(param.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || n > 2147483647 || (param.name == "limit" && (n < 1 || n > 100)) {
				httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_ATTENTION_PAGINATION", "invalid pagination", nil)
				return
			}
			*param.value = n
		}
	}
	result, err := h.repo.SchemaAttention(r.Context(), ws, view, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, 503, "SCHEMA_ATTENTION_UNAVAILABLE", "schema attention unavailable", nil)
		return
	}
	writeJSON(w, 200, result)
}

func (h *Handler) schemaObservationDetail(w http.ResponseWriter, r *http.Request) {
	resource, ok := h.authorizeResource(w, r)
	if !ok {
		return
	}
	ws, _ := uuid.Parse(r.URL.Query().Get("workspaceId"))
	observation, err := uuid.Parse(r.PathValue("observationId"))
	if err != nil || observation == uuid.Nil {
		httpserver.WriteError(w, r, 400, "INVALID_OBSERVATION_ID", "observation required", nil)
		return
	}
	result, err := h.repo.SchemaObservationDetail(r.Context(), ws, resource, observation)
	if errors.Is(err, pgx.ErrNoRows) {
		httpserver.WriteError(w, r, 404, "SCHEMA_OBSERVATION_NOT_FOUND", "observation not found in resource/workspace", nil)
		return
	}
	if err != nil {
		httpserver.WriteError(w, r, 503, "SCHEMA_OBSERVATION_UNAVAILABLE", "schema observation unavailable", nil)
		return
	}
	writeJSON(w, 200, result)
}
