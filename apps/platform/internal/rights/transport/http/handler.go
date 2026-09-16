package rightshttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

type Handler struct {
	service *application.Service
	repo    *infrastructure.PostgresRepository
}

func NewHandler(service *application.Service, repo *infrastructure.PostgresRepository) *Handler {
	return &Handler{service: service, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/authorizations", h.createAuthorization)
	mux.HandleFunc("GET /api/v1/authorizations/{authorizationId}", h.getAuthorization)
	mux.HandleFunc("POST /api/v1/authorizations/{authorizationId}/submit", h.submitAuthorization)
	mux.HandleFunc("POST /api/v1/authorizations/{authorizationId}/approve", h.approveAuthorization)
	mux.HandleFunc("POST /api/v1/authorizations/{authorizationId}/activate", h.activateAuthorization)
	mux.HandleFunc("POST /api/v1/authorizations/{authorizationId}/suspend", h.suspendAuthorization)
	mux.HandleFunc("POST /api/v1/authorizations/{authorizationId}/revoke", h.revokeAuthorization)
	mux.HandleFunc("POST /api/v1/rights-snapshots", h.createSnapshot)
	mux.HandleFunc("GET /api/v1/rights-snapshots/{snapshotId}", h.getSnapshot)
}

type resourceGrantRequest struct {
	DataResourceID   string         `json:"dataResourceId"`
	Actions          []string       `json:"actions"`
	Scope            map[string]any `json:"scope"`
	RawExportAllowed bool           `json:"rawExportAllowed"`
}

type createAuthorizationRequest struct {
	WorkspaceID string                 `json:"workspaceId"`
	Code        string                 `json:"code"`
	GrantorRef  string                 `json:"grantorRef"`
	GranteeRef  string                 `json:"granteeRef"`
	Purpose     string                 `json:"purpose"`
	ValidFrom   *time.Time             `json:"validFrom"`
	ValidTo     *time.Time             `json:"validTo"`
	Metadata    map[string]any         `json:"metadata"`
	Resources   []resourceGrantRequest `json:"resources"`
}

func (h *Handler) createAuthorization(w http.ResponseWriter, r *http.Request) {
	var req createAuthorizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	resources := make([]domain.ResourceGrantSpec, 0, len(req.Resources))
	for _, resource := range req.Resources {
		resourceID, err := uuid.Parse(resource.DataResourceID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATA_RESOURCE_ID", "dataResourceId must be a UUID", nil)
			return
		}
		resources = append(resources, domain.ResourceGrantSpec{
			DataResourceID:   resourceID,
			Actions:          resource.Actions,
			Scope:            resource.Scope,
			RawExportAllowed: resource.RawExportAllowed,
		})
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	authorization, err := h.service.Create(r.Context(), application.CreateAuthorizationCommand{
		WorkspaceID: workspaceID,
		Code:        req.Code,
		GrantorRef:  req.GrantorRef,
		GranteeRef:  req.GranteeRef,
		Purpose:     req.Purpose,
		ValidFrom:   req.ValidFrom,
		ValidTo:     req.ValidTo,
		Metadata:    req.Metadata,
		Resources:   resources,
		ActorID:     actorID,
		TraceID:     httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "AUTHORIZATION_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, authorizationResponse(authorization))
}

func (h *Handler) getAuthorization(w http.ResponseWriter, r *http.Request) {
	authorizationID, ok := parsePathUUID(w, r, "authorizationId", "INVALID_AUTHORIZATION_ID")
	if !ok {
		return
	}
	authorization, err := h.repo.GetAuthorization(r.Context(), authorizationID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "AUTHORIZATION_NOT_FOUND", "authorization not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "AUTHORIZATION_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, authorizationResponse(authorization))
}

func (h *Handler) submitAuthorization(w http.ResponseWriter, r *http.Request) {
	h.transitionAuthorization(w, r, h.service.Submit)
}

func (h *Handler) approveAuthorization(w http.ResponseWriter, r *http.Request) {
	h.transitionAuthorization(w, r, h.service.Approve)
}

func (h *Handler) activateAuthorization(w http.ResponseWriter, r *http.Request) {
	h.transitionAuthorization(w, r, h.service.Activate)
}

func (h *Handler) suspendAuthorization(w http.ResponseWriter, r *http.Request) {
	h.transitionAuthorization(w, r, h.service.Suspend)
}

func (h *Handler) revokeAuthorization(w http.ResponseWriter, r *http.Request) {
	h.transitionAuthorization(w, r, h.service.Revoke)
}

func (h *Handler) transitionAuthorization(w http.ResponseWriter, r *http.Request, fn func(r.Context, application.TransitionCommand) (domain.Authorization, error)) {
}
