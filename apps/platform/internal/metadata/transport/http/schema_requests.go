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

func schemaRequestPagination(w http.ResponseWriter, r *http.Request, name string, fallback int) (int, bool) {
	value := fallback
	if raw := r.URL.Query().Get(name); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 2147483647 || (name == "limit" && (n < 1 || n > 100)) {
			httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_REQUEST_PAGINATION", "invalid pagination", nil)
			return 0, false
		}
		value = n
	}
	return value, true
}

func (h *Handler) unobservedSchemaRequests(w http.ResponseWriter, r *http.Request) {
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
		view = "ALL"
	}
	if !infra.ValidSchemaRequestView(view) {
		httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_REQUEST_VIEW", "view must be ALL, NO_ATTEMPT or ATTEMPT_RECORDED", nil)
		return
	}
	limit, ok := schemaRequestPagination(w, r, "limit", 25)
	if !ok {
		return
	}
	offset, ok := schemaRequestPagination(w, r, "offset", 0)
	if !ok {
		return
	}
	result, err := h.repo.UnobservedSchemaRequests(r.Context(), ws, view, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, 503, "SCHEMA_REQUESTS_UNAVAILABLE", "schema requests unavailable", nil)
		return
	}
	writeJSON(w, 200, result)
}

func (h *Handler) schemaRequestDetail(w http.ResponseWriter, r *http.Request) {
	resource, ok := h.authorizeResource(w, r)
	if !ok {
		return
	}
	ws, _ := uuid.Parse(r.URL.Query().Get("workspaceId"))
	request, err := uuid.Parse(r.PathValue("requestId"))
	if err != nil || request == uuid.Nil {
		httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_REQUEST_ID", "request required", nil)
		return
	}
	var attempt *uuid.UUID
	if values, exists := r.URL.Query()["attemptId"]; exists {
		id, err := uuid.Parse(values[0])
		if err != nil || id == uuid.Nil {
			httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_ATTEMPT_ID", "attempt required when selected", nil)
			return
		}
		attempt = &id
	}
	limit, ok := schemaRequestPagination(w, r, "limit", 25)
	if !ok {
		return
	}
	aOffset, ok := schemaRequestPagination(w, r, "attemptOffset", 0)
	if !ok {
		return
	}
	oOffset, ok := schemaRequestPagination(w, r, "observationOffset", 0)
	if !ok {
		return
	}
	result, err := h.repo.SchemaRequestDetail(r.Context(), ws, resource, request, attempt, limit, aOffset, oOffset)
	if errors.Is(err, pgx.ErrNoRows) {
		httpserver.WriteError(w, r, 404, "SCHEMA_REQUEST_NOT_FOUND", "request/attempt not found in resource/workspace", nil)
		return
	}
	if err != nil {
		httpserver.WriteError(w, r, 503, "SCHEMA_REQUEST_UNAVAILABLE", "schema request unavailable", nil)
		return
	}
	writeJSON(w, 200, result)
}
