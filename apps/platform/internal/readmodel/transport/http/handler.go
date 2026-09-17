package readmodelhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel"
)

const (
	defaultLimit = 25
	maxLimit     = 100
)

type Handler struct {
	repo *readmodel.Repository
}

func NewHandler(repo *readmodel.Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/workbench", h.workbench)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/data-resources", h.listDataResources)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/data-resources/{resourceId}", h.getDataResource)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/datasets", h.listDatasets)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/datasets/{datasetId}", h.getDataset)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/datasets/{datasetId}/versions", h.listDatasetVersions)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/executions", h.listExecutions)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/entity-match-reviews", h.listEntityReviews)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/data-products", h.listDataProducts)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/data-products/{productId}/releases", h.listProductReleases)
}

func (h *Handler) workbench(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	result, err := h.repo.Workbench(r.Context(), workspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "WORKBENCH_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listDataResources(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	result, err := h.repo.ListDataResources(r.Context(), workspaceID, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATA_RESOURCES_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getDataResource(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	resourceID, ok := pathUUID(w, r, "resourceId", "INVALID_DATA_RESOURCE_ID")
	if !ok {
		return
	}
	result, err := h.repo.GetDataResource(r.Context(), resourceID)
	if err != nil || result.WorkspaceID != workspaceID {
		if errors.Is(err, readmodel.ErrNotFound) || (err == nil && result.WorkspaceID != workspaceID) {
			httpserver.WriteError(w, r, http.StatusNotFound, "DATA_RESOURCE_NOT_FOUND", "data resource not found in workspace", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATA_RESOURCE_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listDatasets(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	result, err := h.repo.ListDatasets(r.Context(), workspaceID, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASETS_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) getDataset(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	datasetID, ok := pathUUID(w, r, "datasetId", "INVALID_DATASET_ID")
	if !ok {
		return
	}
	result, err := h.repo.GetDataset(r.Context(), datasetID)
	if err != nil || result.WorkspaceID != workspaceID {
		if errors.Is(err, readmodel.ErrNotFound) || (err == nil && result.WorkspaceID != workspaceID) {
			httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_NOT_FOUND", "dataset not found in workspace", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASET_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listDatasetVersions(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	datasetID, ok := pathUUID(w, r, "datasetId", "INVALID_DATASET_ID")
	if !ok {
		return
	}
	dataset, err := h.repo.GetDataset(r.Context(), datasetID)
	if err != nil {
		if errors.Is(err, readmodel.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_NOT_FOUND", "dataset not found in workspace", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASET_READ_FAILED", err.Error(), nil)
		return
	}
	if dataset.WorkspaceID != workspaceID {
		httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_NOT_FOUND", "dataset not found in workspace", nil)
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	result, err := h.repo.ListDatasetVersions(r.Context(), datasetID, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASET_VERSIONS_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listExecutions(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	result, err := h.repo.ListExecutions(r.Context(), workspaceID, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "EXECUTIONS_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listEntityReviews(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	status := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && !validReviewStatus(status) {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_REVIEW_STATUS", "unsupported review status", map[string]any{"status": status})
		return
	}
	result, err := h.repo.ListEntityReviews(r.Context(), workspaceID, status, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_REVIEWS_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listDataProducts(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	result, err := h.repo.ListDataProducts(r.Context(), workspaceID, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATA_PRODUCTS_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) listProductReleases(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := workspaceID(w, r)
	if !ok {
		return
	}
	productID, ok := pathUUID(w, r, "productId", "INVALID_PRODUCT_ID")
	if !ok {
		return
	}
	belongs, err := h.repo.DataProductBelongsToWorkspace(r.Context(), productID, workspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATA_PRODUCT_READ_FAILED", err.Error(), nil)
		return
	}
	if !belongs {
		httpserver.WriteError(w, r, http.StatusNotFound, "DATA_PRODUCT_NOT_FOUND", "data product not found in workspace", nil)
		return
	}
	limit, offset, ok := pagination(w, r)
	if !ok {
		return
	}
	result, err := h.repo.ListProductReleases(r.Context(), productID, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "PRODUCT_RELEASES_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func workspaceID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return pathUUID(w, r, "workspaceId", "INVALID_WORKSPACE_ID")
}

func pathUUID(w http.ResponseWriter, r *http.Request, name, code string) (uuid.UUID, bool) {
	value, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, code, name+" must be a UUID", nil)
		return uuid.Nil, false
	}
	return value, true
}

func pagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit := defaultLimit
	offset := 0
	var err error
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxLimit {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 100", nil)
			return 0, 0, false
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OFFSET", "offset must be zero or greater", nil)
			return 0, 0, false
		}
	}
	return limit, offset, true
}

func validReviewStatus(value string) bool {
	switch value {
	case "AUTO_CONFIRMED", "PENDING", "CONFIRMED", "REJECTED", "UNRESOLVED":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
