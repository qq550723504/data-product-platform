package resourcehttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
)

type Handler struct {
	create *application.CreateService
}

func NewHandler(create *application.CreateService) *Handler {
	return &Handler{create: create}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/data-resources", h.createDataResource)
}

type createDataResourceRequest struct {
	WorkspaceID      string `json:"workspaceId"`
	ProjectID        string `json:"projectId,omitempty"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	DomainCode       string `json:"domainCode,omitempty"`
	ResourceType     string `json:"resourceType"`
	OwnerID          string `json:"ownerId,omitempty"`
	SensitivityLevel string `json:"sensitivityLevel,omitempty"`
}

func (h *Handler) createDataResource(w http.ResponseWriter, r *http.Request) {
	var req createDataResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}

	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	projectID, err := parseOptionalUUID(req.ProjectID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", "projectId must be a UUID", nil)
		return
	}
	ownerID, err := parseOptionalUUID(req.OwnerID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OWNER_ID", "ownerId must be a UUID", nil)
		return
	}
	actorID, err := parseOptionalUUID(r.Header.Get("X-Actor-ID"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}

	resource, err := h.create.Handle(r.Context(), application.CreateDataResourceCommand{
		WorkspaceID:      workspaceID,
		ProjectID:        projectID,
		Code:             req.Code,
		Name:             req.Name,
		Description:      req.Description,
		DomainCode:       req.DomainCode,
		ResourceType:     domain.ResourceType(req.ResourceType),
		OwnerID:          ownerID,
		SensitivityLevel: req.SensitivityLevel,
		ActorID:          actorID,
		TraceID:          httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "DATA_RESOURCE_CREATE_FAILED"
		if errors.Is(err, domain.ErrInvalidWorkspace) || errors.Is(err, domain.ErrInvalidCode) || errors.Is(err, domain.ErrInvalidName) || errors.Is(err, domain.ErrInvalidType) {
			status = http.StatusBadRequest
			code = "INVALID_DATA_RESOURCE"
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":              resource.ID,
		"workspaceId":     resource.WorkspaceID,
		"code":            resource.Code,
		"name":            resource.Name,
		"resourceType":    resource.ResourceType,
		"lifecycleStatus": resource.LifecycleStatus,
	})
}

func parseOptionalUUID(value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
