package datasethttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

const maxUploadBytes = 32 << 20

type Handler struct {
	createDataset     *application.CreateDatasetService
	uploadVersion     *application.UploadVersionService
	invalidateVersion *application.InvalidateVersionService
	failVersion       *application.FailVersionService
	repo              *infrastructure.PostgresRepository
}

func NewHandler(createDataset *application.CreateDatasetService, uploadVersion *application.UploadVersionService, invalidateVersion *application.InvalidateVersionService, failVersion *application.FailVersionService, repo *infrastructure.PostgresRepository) *Handler {
	return &Handler{
		createDataset:     createDataset,
		uploadVersion:     uploadVersion,
		invalidateVersion: invalidateVersion,
		failVersion:       failVersion,
		repo:              repo,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/datasets", h.create)
	mux.HandleFunc("POST /api/v1/datasets/{datasetId}/versions", h.upload)
	mux.HandleFunc("GET /api/v1/dataset-versions/{versionId}", h.getVersion)
	mux.HandleFunc("POST /api/v1/dataset-versions/{versionId}/invalidate", h.invalidate)
	mux.HandleFunc("POST /api/v1/dataset-versions/{versionId}/fail", h.fail)
}

type createDatasetRequest struct {
	WorkspaceID      string `json:"workspaceId"`
	ProjectID        string `json:"projectId,omitempty"`
	Code             string `json:"code"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	DatasetType      string `json:"datasetType"`
	SourceResourceID string `json:"sourceResourceId,omitempty"`
	OwnerID          string `json:"ownerId,omitempty"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createDatasetRequest
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
	sourceResourceID, err := parseOptionalUUID(req.SourceResourceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_SOURCE_RESOURCE_ID", "sourceResourceId must be a UUID", nil)
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

	dataset, err := h.createDataset.Handle(r.Context(), application.CreateDatasetCommand{
		WorkspaceID:      workspaceID,
		ProjectID:        projectID,
		Code:             req.Code,
		Name:             req.Name,
		Description:      req.Description,
		DatasetType:      domain.DatasetType(req.DatasetType),
		SourceResourceID: sourceResourceID,
		OwnerID:          ownerID,
		ActorID:          actorID,
		TraceID:          httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "DATASET_CREATE_FAILED"
		if errors.Is(err, domain.ErrInvalidWorkspace) || errors.Is(err, domain.ErrInvalidDatasetCode) || errors.Is(err, domain.ErrInvalidDatasetName) || errors.Is(err, domain.ErrInvalidDatasetType) {
			status = http.StatusBadRequest
			code = "INVALID_DATASET"
		}
		if errors.Is(err, resourceinfra.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "SOURCE_RESOURCE_NOT_FOUND", "source resource does not exist or is unavailable", nil)
			return
		}
		if errors.Is(err, domain.ErrSourceResourceWorkspace) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "SOURCE_RESOURCE_WORKSPACE_MISMATCH", "source resource must belong to the dataset workspace", nil)
			return
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":              dataset.ID,
		"workspaceId":     dataset.WorkspaceID,
		"code":            dataset.Code,
		"name":            dataset.Name,
		"datasetType":     dataset.DatasetType,
		"lifecycleStatus": dataset.LifecycleStatus,
	})
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	datasetID, err := uuid.Parse(r.PathValue("datasetId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_ID", "datasetId must be a UUID", nil)
		return
	}
	actorID, err := parseOptionalUUID(r.Header.Get("X-Actor-ID"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_MULTIPART", "multipart form is invalid or too large", nil)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "FILE_REQUIRED", "multipart field 'file' is required", nil)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "FILE_READ_FAILED", "could not read uploaded file", nil)
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}

	version, err := h.uploadVersion.Handle(r.Context(), application.UploadVersionCommand{
		DatasetID:   datasetID,
		Filename:    header.Filename,
		ContentType: contentType,
		Content:     content,
		ActorID:     actorID,
		TraceID:     httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusInternalServerError
		code := "DATASET_VERSION_UPLOAD_FAILED"
		if errors.Is(err, infrastructure.ErrNotFound) {
			status = http.StatusNotFound
			code = "DATASET_NOT_FOUND"
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}
	writeVersion(w, http.StatusCreated, version)
}

func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	workspaceID, err := uuid.Parse(r.URL.Query().Get("workspaceId"))
	if err != nil || workspaceID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId query parameter must be a non-nil UUID", nil)
		return
	}
	version, err := h.repo.GetVersion(r.Context(), versionID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_VERSION_NOT_FOUND", "dataset version not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASET_VERSION_READ_FAILED", err.Error(), nil)
		return
	}
	actualWorkspace, _, err := h.repo.GetWorkspaceAndType(r.Context(), version.DatasetID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASET_WORKSPACE_READ_FAILED", err.Error(), nil)
		return
	}
	if actualWorkspace != workspaceID {
		httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_VERSION_NOT_FOUND", "dataset version not found in workspace", nil)
		return
	}
	writeVersion(w, http.StatusOK, version)
}

type invalidateRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) invalidate(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	var req invalidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	actorID, err := parseOptionalUUID(r.Header.Get("X-Actor-ID"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	version, err := h.invalidateVersion.Handle(r.Context(), application.InvalidateVersionCommand{
		VersionID: versionID,
		Reason:    req.Reason,
		ActorID:   actorID,
		TraceID:   httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusConflict
		code := "DATASET_VERSION_INVALID_TRANSITION"
		if errors.Is(err, infrastructure.ErrNotFound) {
			status = http.StatusNotFound
			code = "DATASET_VERSION_NOT_FOUND"
		} else if !errors.Is(err, domain.ErrInvalidTransition) && !errors.Is(err, domain.ErrImmutableVersion) {
			status = http.StatusInternalServerError
			code = "DATASET_VERSION_INVALIDATE_FAILED"
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}
	writeVersion(w, http.StatusOK, version)
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	var req invalidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	actorID, err := parseOptionalUUID(r.Header.Get("X-Actor-ID"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	version, err := h.failVersion.Handle(r.Context(), application.FailVersionCommand{
		VersionID: versionID,
		Reason:    req.Reason,
		ActorID:   actorID,
		TraceID:   httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusConflict
		code := "DATASET_VERSION_INVALID_TRANSITION"
		if errors.Is(err, infrastructure.ErrNotFound) {
			status = http.StatusNotFound
			code = "DATASET_VERSION_NOT_FOUND"
		} else if !errors.Is(err, domain.ErrInvalidTransition) {
			status = http.StatusInternalServerError
			code = "DATASET_VERSION_FAIL_FAILED"
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}
	writeVersion(w, http.StatusOK, version)
}

func writeVersion(w http.ResponseWriter, status int, version domain.DatasetVersion) {
	writeJSON(w, status, map[string]any{
		"id":                 version.ID,
		"datasetId":          version.DatasetID,
		"versionNo":          version.VersionNo,
		"status":             version.Status,
		"storageType":        version.StorageType,
		"storageUri":         version.StorageURI,
		"contentType":        version.ContentType,
		"rowCount":           version.RowCount,
		"byteSize":           version.ByteSize,
		"checksumAlgorithm":  version.ChecksumAlgorithm,
		"checksum":           version.ChecksumValue,
		"readyAt":            version.ReadyAt,
		"invalidatedAt":      version.InvalidatedAt,
		"invalidationReason": version.InvalidationReason,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
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
