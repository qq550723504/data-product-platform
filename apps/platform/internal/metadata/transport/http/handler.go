package metadatahttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	metadataapp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

type Handler struct {
	service *metadataapp.Service
	repo    *metadatainfra.PostgresRepository
}

func NewHandler(service *metadataapp.Service, repo *metadatainfra.PostgresRepository) *Handler {
	return &Handler{service: service, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/data-resources/{resourceId}/metadata-bindings", h.listBindings)
	mux.HandleFunc("POST /api/v1/data-resources/{resourceId}/metadata-bindings", h.bindResource)
	mux.HandleFunc("GET /api/v1/governance-projections/{provider}/{objectType}/{objectId}", h.getProjection)
}

type bindResourceRequest struct {
	EntityType         string `json:"entityType"`
	FullyQualifiedName string `json:"fullyQualifiedName"`
	Primary            bool   `json:"primary"`
}

func (h *Handler) bindResource(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "METADATA_ENGINE_DISABLED", "metadata engine is disabled", nil)
		return
	}
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_RESOURCE_ID", "resourceId must be a UUID", nil)
		return
	}
	var req bindResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	binding, err := h.service.BindResource(r.Context(), metadataapp.BindResourceCommand{
		ResourceID:         resourceID,
		EntityType:         req.EntityType,
		FullyQualifiedName: req.FullyQualifiedName,
		Primary:            req.Primary,
	})
	if err != nil {
		writeBindingError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, bindingResponse(binding))
}

func (h *Handler) listBindings(w http.ResponseWriter, r *http.Request) {
	resourceID, err := uuid.Parse(r.PathValue("resourceId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_RESOURCE_ID", "resourceId must be a UUID", nil)
		return
	}
	bindings, err := h.repo.ListBindings(r.Context(), resourceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "METADATA_BINDINGS_READ_FAILED", "failed to read metadata bindings", nil)
		return
	}
	items := make([]map[string]any, 0, len(bindings))
	for _, binding := range bindings {
		items = append(items, bindingResponse(binding))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) getProjection(w http.ResponseWriter, r *http.Request) {
	objectID, err := uuid.Parse(r.PathValue("objectId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OBJECT_ID", "objectId must be a UUID", nil)
		return
	}
	provider := metadatadomain.Provider(strings.ToUpper(strings.TrimSpace(r.PathValue("provider"))))
	objectType := strings.ToUpper(strings.TrimSpace(r.PathValue("objectType")))
	projection, err := h.repo.GetProjection(r.Context(), provider, objectType, objectID)
	if err != nil {
		if errors.Is(err, metadatainfra.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "GOVERNANCE_PROJECTION_NOT_FOUND", "governance projection not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "GOVERNANCE_PROJECTION_READ_FAILED", "failed to read governance projection", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":            projection.ID,
		"workspaceId":   projection.WorkspaceID,
		"provider":      projection.Provider,
		"objectType":    projection.ObjectType,
		"objectId":      projection.ObjectID,
		"sourceEventId": projection.SourceEventID,
		"externalId":    projection.ExternalID,
		"externalFqn":   projection.ExternalFQN,
		"status":        projection.Status,
		"attempts":      projection.Attempts,
		"lastError":     publicProjectionError(projection.LastError),
		"metadata":      projection.Metadata,
		"projectedAt":   projection.ProjectedAt,
		"updatedAt":     projection.UpdatedAt,
	})
}

func writeBindingError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, metadataapp.ErrInvalidBindingRequest) {
		httpserver.WriteError(w, r, http.StatusBadRequest, "METADATA_BINDING_INVALID", "metadata binding request is invalid", nil)
		return
	}
	var external *metadataengine.ExternalError
	if errors.As(err, &external) {
		switch external.Kind {
		case metadataengine.ErrorInvalidRequest:
			httpserver.WriteError(w, r, http.StatusBadRequest, "METADATA_BINDING_INVALID", "metadata binding request is invalid", nil)
		case metadataengine.ErrorNotFound:
			httpserver.WriteError(w, r, http.StatusNotFound, "METADATA_ASSET_NOT_FOUND", "metadata asset was not found", nil)
		case metadataengine.ErrorUnavailable:
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "METADATA_PROVIDER_UNAVAILABLE", "metadata provider is unavailable", nil)
		case metadataengine.ErrorUnauthorized:
			httpserver.WriteError(w, r, http.StatusBadGateway, "METADATA_PROVIDER_AUTH_FAILED", "metadata provider rejected platform credentials", nil)
		case metadataengine.ErrorInvalidResponse:
			httpserver.WriteError(w, r, http.StatusBadGateway, "METADATA_PROVIDER_INVALID_RESPONSE", "metadata provider returned an invalid response", nil)
		default:
			httpserver.WriteError(w, r, http.StatusBadGateway, "METADATA_PROVIDER_REJECTED", "metadata provider rejected the request", nil)
		}
		return
	}
	httpserver.WriteError(w, r, http.StatusInternalServerError, "METADATA_BINDING_FAILED", "metadata binding failed", nil)
}

func publicProjectionError(lastError string) string {
	if strings.TrimSpace(lastError) == "" {
		return ""
	}
	return "metadata projection failed"
}

func bindingResponse(binding metadatadomain.ResourceBinding) map[string]any {
	return map[string]any{
		"id":              binding.ID,
		"resourceId":      binding.ResourceID,
		"provider":        binding.Provider,
		"entityType":      binding.EntityType,
		"externalId":      binding.ExternalID,
		"externalFqn":     binding.ExternalFQN,
		"bindingMetadata": binding.BindingMetadata,
		"primary":         binding.IsPrimary,
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
