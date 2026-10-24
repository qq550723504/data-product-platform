package contracthttp

import (
	"errors"
	"github.com/google/uuid"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"strconv"
)

type ReadOnlyHandler struct {
	Reader    q.Reader
	Principal principal.Resolver
}

func (h ReadOnlyHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/contract-versions/{versionId}", func(w http.ResponseWriter, r *http.Request) { h.read(w, r, "version") })
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/contract-versions", func(w http.ResponseWriter, r *http.Request) { h.read(w, r, "list") })
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/acceptance-context", func(w http.ResponseWriter, r *http.Request) { h.read(w, r, "context") })
}
func (h ReadOnlyHandler) read(w http.ResponseWriter, r *http.Request, kind string) {
	w.Header().Set("Cache-Control", "private, no-store")
	params := r.URL.Query()
	wsValue := r.PathValue("workspaceId")
	if kind == "version" {
		wsValue = params.Get("workspaceId")
	}
	ws, err := uuid.Parse(wsValue)
	if err != nil || ws == uuid.Nil {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE_ID", "explicit workspace UUID required", nil)
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
	if err != nil || actor.ActorID == uuid.Nil || actor.Subject == "" || actor.WorkspaceID != ws || !actor.HasCapability(principal.CapabilityHumanDecision) {
		httpserver.WriteError(w, r, 403, "WORKSPACE_AUTHORITY_DENIED", "workspace read authority denied", nil)
		return
	}
	allowed := map[string]bool{"limit": true, "offset": true}
	if kind == "version" {
		allowed = map[string]bool{"workspaceId": true}
	}
	if kind == "context" {
		allowed["deliveryOperationId"] = true
		allowed["datasetVersionId"] = true
		allowed["contractVersionId"] = true
	}
	for key, values := range params {
		if !allowed[key] || len(values) != 1 {
			httpserver.WriteError(w, r, 400, "INVALID_QUERY", "unsupported or repeated query", nil)
			return
		}
	}
	query := q.Query{WorkspaceID: ws, Limit: 25}
	for key, target := range map[string]*int{"limit": &query.Limit, "offset": &query.Offset} {
		if raw, ok := params[key]; ok {
			n, e := strconv.Atoi(raw[0])
			if e != nil || n < 0 || n > 2147483647 || (key == "limit" && (n < 1 || n > 100)) {
				httpserver.WriteError(w, r, 400, "INVALID_PAGE", "invalid pagination", nil)
				return
			}
			*target = n
		}
	}
	parse := func(value string) (*uuid.UUID, bool) {
		id, e := uuid.Parse(value)
		return &id, e == nil && id != uuid.Nil
	}
	if kind == "version" {
		var ok bool
		query.VersionID, ok = parse(r.PathValue("versionId"))
		if !ok {
			httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "explicit version UUID required", nil)
			return
		}
	}
	if kind == "context" {
		var ok bool
		query.DeliveryID, ok = parse(params.Get("deliveryOperationId"))
		if !ok {
			httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "explicit delivery UUID required", nil)
			return
		}
		query.DatasetVersionID, ok = parse(params.Get("datasetVersionId"))
		if !ok {
			httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "explicit dataset version UUID required", nil)
			return
		}
		if raw, exists := params["contractVersionId"]; exists {
			query.VersionID, ok = parse(raw[0])
			if !ok {
				httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "explicit contract version UUID required", nil)
				return
			}
			query.Offset = 0
		}
	}
	p, c, err := (q.Service{Reader: h.Reader}).Read(r.Context(), query)
	if errors.Is(err, q.ErrNotFound) {
		httpserver.WriteError(w, r, 404, "ACCEPTANCE_SOURCE_NOT_FOUND", "scoped source not found", nil)
		return
	}
	if err != nil {
		httpserver.WriteError(w, r, 500, "ACCEPTANCE_SOURCE_READ_FAILED", "source unavailable", nil)
		return
	}
	if kind == "version" {
		if len(p.Items) != 1 {
			httpserver.WriteError(w, r, 404, "ACCEPTANCE_SOURCE_NOT_FOUND", "scoped version not found", nil)
			return
		}
		writeJSON(w, 200, p.Items[0])
		return
	}
	if kind == "context" {
		writeJSON(w, 200, c)
		return
	}
	writeJSON(w, 200, p)
}
