package metadatahttp

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

func (h *Handler) schemaHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := h.authorizeResource(w, r)
	if !ok {
		return
	}
	ws, _ := uuid.Parse(r.URL.Query().Get("workspaceId"))
	if h.service == nil || h.service.Schema == nil {
		writeJSON(w, 200, map[string]any{"enabled": false, "items": []any{}})
		return
	}
	items, err := h.repo.SchemaHistory(r.Context(), id, ws)
	if err != nil {
		httpserver.WriteError(w, r, 503, "SCHEMA_HISTORY_UNAVAILABLE", "schema history unavailable", nil)
		return
	}
	plan, err := h.service.Schema.Observer.Plan(r.Context())
	enabled := err == nil && plan.ResourceID == id && plan.WorkspaceID == ws
	writeJSON(w, 200, map[string]any{"enabled": enabled, "items": items})
}
func (h *Handler) requestSchema(w http.ResponseWriter, r *http.Request) {
	id, ok := h.authorizeResource(w, r)
	if !ok {
		return
	}
	ws, _ := uuid.Parse(r.URL.Query().Get("workspaceId"))
	actor, err := h.Authorize(r, ws)
	if err != nil || actor == nil || h.service == nil || h.service.Schema == nil {
		schemaError(w, r, app.ErrSchemaDenied)
		return
	}
	// Structural scope, credentials, rule and consumer are server-owned.
	if r.ContentLength != 0 {
		httpserver.WriteError(w, r, 400, "SCHEMA_REQUEST_BODY_REJECTED", "schema scan accepts no caller-provided scope", nil)
		return
	}
	req, err := h.service.Schema.Request(r.Context(), id, ws, *actor, r.Header.Get("Idempotency-Key"))
	if err != nil {
		schemaError(w, r, err)
		return
	}
	writeJSON(w, 202, map[string]any{"id": req.ID, "status": "REQUESTED", "message": "worker will execute native source discovery; history remains independent from version quality"})
}
func (h *Handler) handleSchema(w http.ResponseWriter, r *http.Request) {
	id, ok := h.authorizeResource(w, r)
	if !ok {
		return
	}
	ws, _ := uuid.Parse(r.URL.Query().Get("workspaceId"))
	actor, err := h.Authorize(r, ws)
	if err != nil || actor == nil || h.service == nil || h.service.Schema == nil {
		schemaError(w, r, app.ErrSchemaDenied)
		return
	}
	observation, err := uuid.Parse(r.PathValue("observationId"))
	if err != nil {
		schemaError(w, r, infra.ErrSchemaConflict)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&body) != nil {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "a bounded handling reason is required", nil)
		return
	}
	if err := h.service.Schema.Handle(r.Context(), id, ws, *actor, observation, r.Header.Get("Idempotency-Key"), body.Reason); err != nil {
		schemaError(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"progress": "INVESTIGATING", "message": "handling recorded; recovery requires a new native scan; quality and certification unchanged"})
}
func schemaError(w http.ResponseWriter, r *http.Request, err error) {
	code, status := "SCHEMA_COMMAND_UNAVAILABLE", 503
	if !errors.Is(err, app.ErrSchemaDenied) && !errors.Is(err, infra.ErrSchemaConflict) {
		slog.Warn("source schema command unavailable", "error", err)
	}
	if errors.Is(err, app.ErrSchemaDenied) {
		code = "SCHEMA_AUTHORITY_DENIED"
		status = 403
	}
	if errors.Is(err, infra.ErrSchemaConflict) {
		code = "SCHEMA_REQUEST_CONFLICT"
		status = 409
	}
	httpserver.WriteError(w, r, status, code, "source schema command was not accepted", nil)
}
