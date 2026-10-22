package metadatahttp

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

func (h *Handler) schemaExceptions(w http.ResponseWriter, r *http.Request) {
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
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "ALL"
	}
	if !infra.ValidSchemaExceptionStatus(status) {
		httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_EXCEPTION_STATUS", "status must be ALL, UNAVAILABLE, DENIED or INTERRUPTED", nil)
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
				httpserver.WriteError(w, r, 400, "INVALID_SCHEMA_EXCEPTION_PAGINATION", "invalid pagination", nil)
				return
			}
			*param.value = n
		}
	}
	result, err := h.repo.SchemaExceptions(r.Context(), ws, status, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, 503, "SCHEMA_EXCEPTIONS_UNAVAILABLE", "schema exception history unavailable", nil)
		return
	}
	writeJSON(w, 200, result)
}
