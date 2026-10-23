package agenttaskhttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"io"
	"net/http"
	"strconv"
)

type Handler struct {
	Service      *application.Service
	Principal    principal.Resolver
	WriteEnabled bool
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/agent-task-authority", h.authority)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/agent-tasks", h.list)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/agent-tasks/{taskId}", h.detail)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/agent-tasks", func(w http.ResponseWriter, r *http.Request) { h.command(w, r, "CREATE") })
	for path, kind := range map[string]string{"revisions": "REVISE", "approvals": "APPROVE", "actions": "REQUEST_ACTION", "pause": "PAUSE", "resume": "RESUME", "stop": "STOP"} {
		mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/agent-tasks/{taskId}/"+path, func(w http.ResponseWriter, r *http.Request) { h.command(w, r, kind) })
	}
}
func (h *Handler) auth(w http.ResponseWriter, r *http.Request) (principal.RequestPrincipal, bool) {
	w.Header().Set("Cache-Control", "no-store")
	ws, err := uuid.Parse(r.PathValue("workspaceId"))
	if err != nil || ws == uuid.Nil {
		fail(w, r, 400, "INVALID_WORKSPACE")
		return principal.RequestPrincipal{}, false
	}
	if h.Principal == nil {
		fail(w, r, 401, "AUTHENTICATION_REQUIRED")
		return principal.RequestPrincipal{}, false
	}
	p, err := h.Principal.Resolve(r, ws, principal.CapabilityHumanDecision)
	if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
		fail(w, r, 401, "AUTHENTICATION_REQUIRED")
		return p, false
	}
	if err != nil || p.WorkspaceID != ws || p.ActorID == uuid.Nil || p.Subject == "" || !p.HasCapability(principal.CapabilityHumanDecision) {
		fail(w, r, 403, "AGENT_AUTHORITY_DENIED")
		return p, false
	}
	return p, true
}
func (h *Handler) authority(w http.ResponseWriter, r *http.Request) {
	p, ok := h.auth(w, r)
	if !ok {
		return
	}
	respond(w, map[string]any{"actorId": p.ActorID, "subject": p.Subject, "workspaceId": p.WorkspaceID, "writeEnabled": h.WriteEnabled, "dispatchEnabled": false})
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, ok := h.auth(w, r)
	if !ok {
		return
	}
	offset, valid := pagination(r, "offset")
	if !valid || len(r.URL.Query()) > 1 {
		fail(w, r, 400, "INVALID_QUERY")
		return
	}
	if h.Service == nil {
		fail(w, r, 503, "AGENT_UNAVAILABLE")
		return
	}
	items, total, err := h.Service.Repo.List(r.Context(), p.WorkspaceID, 25, offset)
	if err != nil {
		failure(w, r, err)
		return
	}
	respond(w, map[string]any{"items": items, "page": domain.Page{Limit: 25, Offset: offset, Total: total}})
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	p, ok := h.auth(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(r.PathValue("taskId"))
	if err != nil || id == uuid.Nil {
		fail(w, r, 400, "INVALID_TASK")
		return
	}
	offset, ok := pagination(r, "planOffset")
	if !ok {
		fail(w, r, 400, "INVALID_QUERY")
		return
	}
	plan := uuid.Nil
	for key, values := range r.URL.Query() {
		if len(values) != 1 || (key != "planOffset" && key != "planId") {
			fail(w, r, 400, "INVALID_QUERY")
			return
		}
		if key == "planId" {
			plan, err = uuid.Parse(values[0])
			if err != nil || plan == uuid.Nil {
				fail(w, r, 400, "INVALID_QUERY")
				return
			}
		}
	}
	if h.Service == nil {
		fail(w, r, 503, "AGENT_UNAVAILABLE")
		return
	}
	tx, err := h.Service.Repo.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		failure(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	d, err := h.Service.Repo.DetailTx(r.Context(), tx, p.WorkspaceID, id, plan, offset)
	if err != nil {
		failure(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		failure(w, r, err)
		return
	}
	respond(w, d)
}
func pagination(r *http.Request, key string) (int, bool) {
	v := r.URL.Query()[key]
	if len(v) == 0 {
		return 0, true
	}
	if len(v) != 1 {
		return 0, false
	}
	n, err := strconv.Atoi(v[0])
	return n, err == nil && n >= 0 && n <= 2147483647
}

var allowed = map[string]string{
	"CREATE":         "phase sourceVersionId goal purposeNote acceptanceText reason",
	"REVISE":         "expectedRevision basePlanRevisionId phase sourceVersionId goal purposeNote acceptanceText caseId workflowVersionId outputDatasetId targetPeriod inputs reason",
	"APPROVE":        "expectedRevision planRevisionId contentHash phase reason",
	"REQUEST_ACTION": "expectedRevision planRevisionId contentHash phase approvalId commandKind reason",
	"PAUSE":          "expectedRevision reason", "RESUME": "expectedRevision reason", "STOP": "expectedRevision reason",
}

func decode(r *http.Request, w http.ResponseWriter, kind string) (application.Request, error) {
	var q application.Request
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 65536))
	if err != nil {
		return q, domain.ErrInvalid
	}
	// Token scan rejects duplicate keys, including identity-changing duplicates.
	dec := json.NewDecoder(bytes.NewReader(raw))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return q, domain.ErrInvalid
	}
	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return q, domain.ErrInvalid
		}
		name, ok := key.(string)
		if !ok || seen[name] || !fieldAllowed(kind, name) {
			return q, domain.ErrInvalid
		}
		seen[name] = true
		var value json.RawMessage
		if err = dec.Decode(&value); err != nil {
			return q, domain.ErrInvalid
		}
	}
	if _, err = dec.Token(); err != nil {
		return q, domain.ErrInvalid
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return q, domain.ErrInvalid
	}
	dec = json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&q); err != nil {
		return q, domain.ErrInvalid
	}
	return q, nil
}
func fieldAllowed(kind, key string) bool {
	for _, s := range bytes.Fields([]byte(allowed[kind])) {
		if string(s) == key {
			return true
		}
	}
	return false
}
func (h *Handler) command(w http.ResponseWriter, r *http.Request, kind string) {
	p, ok := h.auth(w, r)
	if !ok {
		return
	}
	if !h.WriteEnabled {
		fail(w, r, 403, "AGENT_WRITES_DISABLED")
		return
	}
	if len(r.URL.Query()) != 0 {
		fail(w, r, 400, "INVALID_QUERY")
		return
	}
	id := uuid.Nil
	var err error
	if kind != "CREATE" {
		id, err = uuid.Parse(r.PathValue("taskId"))
		if err != nil || id == uuid.Nil {
			fail(w, r, 400, "INVALID_TASK")
			return
		}
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 || !domain.Text(keys[0], 1, 128) {
		fail(w, r, 400, "IDEMPOTENCY_KEY_REQUIRED")
		return
	}
	q, err := decode(r, w, kind)
	if err != nil {
		failure(w, r, err)
		return
	}
	if h.Service == nil {
		fail(w, r, 503, "AGENT_UNAVAILABLE")
		return
	}
	result, err := h.Service.Handle(r.Context(), application.Command{Principal: p, TaskID: id, Key: keys[0], TraceID: r.Header.Get("X-Request-ID")}, kind, q)
	if err != nil {
		failure(w, r, err)
		return
	}
	respond(w, result)
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, r *http.Request, status int, code string) {
	httpserver.WriteError(w, r, status, code, code, nil)
}
func failure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		fail(w, r, 400, "INVALID_AGENT_COMMAND")
	case errors.Is(err, domain.ErrNotFound):
		fail(w, r, 404, "AGENT_OBJECT_NOT_FOUND")
	case errors.Is(err, domain.ErrDenied):
		fail(w, r, 403, "AGENT_OWNER_DENIED")
	case application.IsConflict(err):
		fail(w, r, 409, err.Error())
	default:
		fail(w, r, 503, "AGENT_UNAVAILABLE")
	}
}
