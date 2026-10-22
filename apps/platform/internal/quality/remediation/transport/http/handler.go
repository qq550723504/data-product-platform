package remediationhttp

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/infrastructure"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"io"
	"net/http"
	"strings"
)

type Handler struct {
	Service   *application.Service
	Repo      *infrastructure.Repository
	Principal principal.Resolver
}

func NewHandler(s *application.Service, r *infrastructure.Repository, p principal.Resolver) *Handler {
	return &Handler{s, r, p}
}
func (h *Handler) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/workspaces/{workspaceId}/quality-remediations/attention", h.attention)
	m.HandleFunc("GET /api/v1/workspaces/{workspaceId}/quality-remediations", h.list)
	m.HandleFunc("GET /api/v1/workspaces/{workspaceId}/quality-remediations/{caseId}", h.detail)
	m.HandleFunc("POST /api/v1/workspaces/{workspaceId}/quality-remediations/detect", h.detect)
	for _, name := range []string{"assign", "execute", "retry", "recheck", "close"} {
		m.HandleFunc("POST /api/v1/workspaces/{workspaceId}/quality-remediations/{caseId}/"+name, h.command)
	}
}
func (h *Handler) authority(w http.ResponseWriter, r *http.Request) (application.Command, bool) {
	var c application.Command
	ws, e := uuid.Parse(r.PathValue("workspaceId"))
	if e != nil || ws == uuid.Nil {
		failure(w, r, 400, "INVALID_WORKSPACE")
		return c, false
	}
	if h.Principal == nil {
		failure(w, r, 401, "AUTHENTICATION_REQUIRED")
		return c, false
	}
	p, e := h.Principal.Resolve(r, ws, principal.CapabilityHumanDecision)
	if e != nil {
		failure(w, r, 403, "REMEDIATION_AUTHORITY_DENIED")
		return c, false
	}
	if h.Service == nil {
		failure(w, r, 503, "REMEDIATION_NOT_CONFIGURED")
		return c, false
	}
	c.WorkspaceID, c.ActorID, c.Key, c.TraceID = ws, p.ActorID, strings.TrimSpace(r.Header.Get("Idempotency-Key")), httpserver.RequestID(r.Context())
	if raw := r.PathValue("caseId"); raw != "" {
		c.CaseID, e = uuid.Parse(raw)
		if e != nil || c.CaseID == uuid.Nil {
			failure(w, r, 400, "INVALID_CASE")
			return c, false
		}
	}
	return c, true
}
func failure(w http.ResponseWriter, r *http.Request, status int, code string) {
	httpserver.WriteError(w, r, status, code, code, nil)
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		failure(w, r, 400, "INVALID_REMEDIATION_COMMAND")
		return false
	}
	if e := d.Decode(new(any)); !errors.Is(e, io.EOF) {
		failure(w, r, 400, "INVALID_REMEDIATION_COMMAND")
		return false
	}
	return true
}
func result(w http.ResponseWriter, r *http.Request, v any, err error) {
	if err == nil {
		respond(w, v)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		failure(w, r, 404, "REMEDIATION_NOT_FOUND")
		return
	}
	switch {
	case errors.Is(err, domain.ErrScope):
		failure(w, r, 403, "REMEDIATION_SCOPE_DENIED")
	case errors.Is(err, domain.ErrAssignee):
		failure(w, r, 403, "REMEDIATION_ASSIGNEE_REQUIRED")
	case errors.Is(err, domain.ErrConflict), errors.Is(err, workflowdomain.ErrIdempotencyConflict):
		failure(w, r, 409, "REMEDIATION_KEY_CONFLICT")
	case errors.Is(err, domain.ErrClosed):
		failure(w, r, 409, "REMEDIATION_MANUALLY_CLOSED")
	default:
		failure(w, r, 409, "REMEDIATION_COMMAND_REJECTED")
	}
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	c, ok := h.authority(w, r)
	if !ok {
		return
	}
	var version uuid.UUID
	if raw := r.URL.Query().Get("versionId"); raw != "" {
		var e error
		version, e = uuid.Parse(raw)
		if e != nil || version == uuid.Nil {
			failure(w, r, 400, "INVALID_VERSION")
			return
		}
	}
	items, e := h.Repo.List(r.Context(), c.WorkspaceID, version)
	result(w, r, map[string]any{"items": items}, e)
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request) {
	c, ok := h.authority(w, r)
	if !ok {
		return
	}
	v, e := h.Repo.Detail(r.Context(), c.WorkspaceID, c.CaseID)
	result(w, r, v, e)
}
func (h *Handler) detect(w http.ResponseWriter, r *http.Request) {
	c, ok := h.authority(w, r)
	if !ok {
		return
	}
	var body struct {
		VersionID uuid.UUID `json:"versionId"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, e := h.Service.Detect(r.Context(), c, body.VersionID)
	result(w, r, v, e)
}
func (h *Handler) command(w http.ResponseWriter, r *http.Request) {
	c, ok := h.authority(w, r)
	if !ok {
		return
	}
	action := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	switch action {
	case "execute":
		var b struct {
			Reason            string    `json:"reason"`
			WorkflowVersionID uuid.UUID `json:"workflowVersionId"`
			OutputDatasetID   uuid.UUID `json:"outputDatasetId"`
			TargetPeriod      string    `json:"targetPeriod"`
			Inputs            []struct {
				Name      string    `json:"name"`
				VersionID uuid.UUID `json:"datasetVersionId"`
			} `json:"inputs"`
		}
		if !decode(w, r, &b) {
			return
		}
		c.Reason = b.Reason
		cmd := application.ExecuteCommand{Command: c, WorkflowVersionID: b.WorkflowVersionID, OutputDatasetID: b.OutputDatasetID, TargetPeriod: b.TargetPeriod}
		for _, i := range b.Inputs {
			cmd.Inputs = append(cmd.Inputs, workflowdomain.InputBinding{Name: i.Name, DatasetVersionID: i.VersionID})
		}
		v, e := h.Service.Execute(r.Context(), cmd)
		result(w, r, map[string]any{"executionId": v.ID, "status": v.Status}, e)
	case "recheck":
		var b struct {
			ExecutionLinkID uuid.UUID `json:"executionLinkId"`
		}
		if !decode(w, r, &b) {
			return
		}
		id, e := h.Service.Recheck(r.Context(), c, b.ExecutionLinkID)
		result(w, r, map[string]any{"recheckId": id}, e)
	default:
		var b struct {
			Reason string `json:"reason"`
		}
		if !decode(w, r, &b) {
			return
		}
		c.Reason = b.Reason
		if action == "retry" {
			v, e := h.Service.Execute(r.Context(), application.ExecuteCommand{Command: c, Retry: true})
			result(w, r, map[string]any{"executionId": v.ID, "status": v.Status}, e)
			return
		}
		var id uuid.UUID
		var e error
		if action == "assign" {
			id, e = h.Service.Assign(r.Context(), c)
		} else {
			id, e = h.Service.Close(r.Context(), c)
		}
		result(w, r, map[string]any{"id": id}, e)
	}
}
