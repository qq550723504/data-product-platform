package rightshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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
	h.registerProvenance(mux)
}

type resourceGrantRequest struct {
	DataResourceID   string         `json:"dataResourceId"`
	Actions          []string       `json:"actions"`
	Scope            map[string]any `json:"scope"`
	ScopeType        string         `json:"scopeType"`
	ScopeRef         string         `json:"scopeRef"`
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
			ScopeType:        resource.ScopeType,
			ScopeRef:         resource.ScopeRef,
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
		if errors.Is(err, domain.ErrResourceWorkspace) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "DATA_RESOURCE_WORKSPACE_MISMATCH", "every granted data resource must belong to the authorization workspace", nil)
			return
		}
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

type transitionFn func(context.Context, application.TransitionCommand) (domain.Authorization, error)

func (h *Handler) transitionAuthorization(w http.ResponseWriter, r *http.Request, fn transitionFn) {
	authorizationID, ok := parsePathUUID(w, r, "authorizationId", "INVALID_AUTHORIZATION_ID")
	if !ok {
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	authorization, err := fn(r.Context(), application.TransitionCommand{
		AuthorizationID: authorizationID,
		ActorID:         actorID,
		TraceID:         httpserver.RequestID(r.Context()),
		At:              time.Now().UTC(),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, infrastructure.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpserver.WriteError(w, r, status, "AUTHORIZATION_TRANSITION_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, authorizationResponse(authorization))
}

type createSnapshotRequest struct {
	WorkspaceID      string     `json:"workspaceId"`
	ProductReleaseID string     `json:"productReleaseId"`
	Purpose          string     `json:"purpose"`
	ConsumerRef      string     `json:"consumerRef"`
	AsOf             *time.Time `json:"asOf"`
	AuthorizationIDs []string   `json:"authorizationIds"`
}

func (h *Handler) createSnapshot(w http.ResponseWriter, r *http.Request) {
	var req createSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	var releaseID *uuid.UUID
	if strings.TrimSpace(req.ProductReleaseID) != "" {
		parsed, err := uuid.Parse(req.ProductReleaseID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_PRODUCT_RELEASE_ID", "productReleaseId must be a UUID", nil)
			return
		}
		releaseID = &parsed
	}
	authorizationIDs := make([]uuid.UUID, 0, len(req.AuthorizationIDs))
	for _, value := range req.AuthorizationIDs {
		parsed, err := uuid.Parse(value)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_AUTHORIZATION_ID", "authorizationIds must contain UUIDs", nil)
			return
		}
		authorizationIDs = append(authorizationIDs, parsed)
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	asOf := time.Now().UTC()
	if req.AsOf != nil {
		asOf = req.AsOf.UTC()
	}
	snapshot, err := h.service.CreateSnapshot(r.Context(), application.CreateSnapshotCommand{
		WorkspaceID:      workspaceID,
		ProductReleaseID: releaseID,
		Purpose:          req.Purpose,
		ConsumerRef:      req.ConsumerRef,
		AsOf:             asOf,
		AuthorizationIDs: authorizationIDs,
		ActorID:          actorID,
		TraceID:          httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "RIGHTS_SNAPSHOT_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, snapshotResponse(snapshot))
}

func (h *Handler) getSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshotID, ok := parsePathUUID(w, r, "snapshotId", "INVALID_RIGHTS_SNAPSHOT_ID")
	if !ok {
		return
	}
	snapshot, err := h.repo.GetSnapshot(r.Context(), snapshotID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "RIGHTS_SNAPSHOT_NOT_FOUND", "rights snapshot not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "RIGHTS_SNAPSHOT_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, snapshotResponse(snapshot))
}

func authorizationResponse(authorization domain.Authorization) map[string]any {
	resources := make([]map[string]any, 0, len(authorization.Resources))
	for _, resource := range authorization.Resources {
		resources = append(resources, map[string]any{
			"id":               resource.ID,
			"dataResourceId":   resource.DataResourceID,
			"actions":          resource.Actions,
			"scope":            resource.Scope,
			"scopeType":        resource.ScopeType,
			"scopeRef":         resource.ScopeRef,
			"rawExportAllowed": resource.RawExportAllowed,
		})
	}
	return map[string]any{
		"id":          authorization.ID,
		"workspaceId": authorization.WorkspaceID,
		"code":        authorization.Code,
		"grantorRef":  authorization.GrantorRef,
		"granteeRef":  authorization.GranteeRef,
		"purpose":     authorization.Purpose,
		"status":      authorization.Status,
		"validFrom":   authorization.ValidFrom,
		"validTo":     authorization.ValidTo,
		"metadata":    authorization.Metadata,
		"resources":   resources,
	}
}

func snapshotResponse(snapshot domain.RightsSnapshot) map[string]any {
	return map[string]any{
		"id":               snapshot.ID,
		"workspaceId":      snapshot.WorkspaceID,
		"productReleaseId": snapshot.ProductReleaseID,
		"purpose":          snapshot.Purpose,
		"consumerRef":      snapshot.ConsumerRef,
		"asOf":             snapshot.AsOf,
		"manifest":         snapshot.Manifest,
		"rootHash":         snapshot.RootHash,
		"declarationIds":   snapshot.DeclarationIDs,
		"bindingIds":       snapshot.BindingIDs,
		"createdAt":        snapshot.CreatedAt,
	}
}

func parsePathUUID(w http.ResponseWriter, r *http.Request, name, code string) (uuid.UUID, bool) {
	value, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, code, name+" must be a UUID", nil)
		return uuid.Nil, false
	}
	return value, true
}

func parseActorID(r *http.Request) (*uuid.UUID, error) {
	value := strings.TrimSpace(r.Header.Get("X-Actor-ID"))
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
