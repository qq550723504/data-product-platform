package producthttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

type Handler struct {
	service *application.Service
	repo    *infrastructure.PostgresRepository
}

func NewHandler(service *application.Service, repo *infrastructure.PostgresRepository) *Handler {
	return &Handler{service: service, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/data-products", h.createProduct)
	mux.HandleFunc("GET /api/v1/data-products/{productId}", h.getProduct)
	mux.HandleFunc("POST /api/v1/data-products/{productId}/versions", h.createVersion)
	mux.HandleFunc("GET /api/v1/product-versions/{versionId}", h.getVersion)
	mux.HandleFunc("POST /api/v1/data-products/{productId}/releases", h.createRelease)
	mux.HandleFunc("GET /api/v1/product-releases/{releaseId}", h.getRelease)
	mux.HandleFunc("GET /api/v1/product-releases/{releaseId}/readiness", h.getReadiness)
}

type createProductRequest struct {
	WorkspaceID string         `json:"workspaceId"`
	ProjectID   string         `json:"projectId"`
	UseCaseID   string         `json:"useCaseId"`
	Code        string         `json:"code"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	DomainCode  string         `json:"domainCode"`
	OwnerID     string         `json:"ownerId"`
	Metadata    map[string]any `json:"metadata"`
}

func (h *Handler) createProduct(w http.ResponseWriter, r *http.Request) {
	var req createProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	projectID, err := optionalUUID(req.ProjectID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", "projectId must be a UUID", nil)
		return
	}
	useCaseID, err := optionalUUID(req.UseCaseID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_USE_CASE_ID", "useCaseId must be a UUID", nil)
		return
	}
	ownerID, err := optionalUUID(req.OwnerID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OWNER_ID", "ownerId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	product, err := h.service.CreateProduct(r.Context(), application.CreateProductCommand{
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		UseCaseID:   useCaseID,
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		DomainCode:  req.DomainCode,
		OwnerID:     ownerID,
		Metadata:    req.Metadata,
		ActorID:     actorID,
		TraceID:     httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "DATA_PRODUCT_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, productResponse(product))
}

func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	productID, ok := parsePathUUID(w, r, "productId", "INVALID_PRODUCT_ID")
	if !ok {
		return
	}
	product, err := h.repo.GetProduct(r.Context(), productID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "DATA_PRODUCT_NOT_FOUND", "data product not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATA_PRODUCT_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, productResponse(product))
}

type createVersionRequest struct {
	Version           string             `json:"version"`
	WorkflowVersionID string             `json:"workflowVersionId"`
	ContractVersionID string             `json:"contractVersionId"`
	EntityPolicyRef   string             `json:"entityPolicyRef"`
	IndicatorSetRef   string             `json:"indicatorSetRef"`
	Definition        map[string]any     `json:"definition"`
	Assets            []assetRequest     `json:"assets"`
}

type assetRequest struct {
	AssetType      string         `json:"assetType"`
	Name           string         `json:"name"`
	DatasetID      string         `json:"datasetId"`
	ExternalRef    string         `json:"externalRef"`
	DeliveryConfig map[string]any `json:"deliveryConfig"`
	SchemaSnapshot map[string]any `json:"schemaSnapshot"`
}

func (h *Handler) createVersion(w http.ResponseWriter, r *http.Request) {
	productID, ok := parsePathUUID(w, r, "productId", "INVALID_PRODUCT_ID")
	if !ok {
		return
	}
	var req createVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	major, minor, patch, err := parseSemver(req.Version)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_PRODUCT_VERSION", err.Error(), nil)
		return
	}
	workflowVersionID, err := optionalUUID(req.WorkflowVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKFLOW_VERSION_ID", "workflowVersionId must be a UUID", nil)
		return
	}
	contractVersionID, err := optionalUUID(req.ContractVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CONTRACT_VERSION_ID", "contractVersionId must be a UUID", nil)
		return
	}
	assets := make([]domain.AssetSpec, 0, len(req.Assets))
	for _, item := range req.Assets {
		datasetID, err := optionalUUID(item.DatasetID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ASSET_DATASET_ID", "asset datasetId must be a UUID", nil)
			return
		}
		assets = append(assets, domain.AssetSpec{
			AssetType:      domain.AssetType(item.AssetType),
			Name:           item.Name,
			DatasetID:      datasetID,
			ExternalRef:    item.ExternalRef,
			DeliveryConfig: item.DeliveryConfig,
			SchemaSnapshot: item.SchemaSnapshot,
		})
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	version, err := h.service.CreateVersion(r.Context(), application.CreateVersionCommand{
		ProductID:         productID,
		MajorVersion:      major,
		MinorVersion:      minor,
		PatchVersion:      patch,
		WorkflowVersionID: workflowVersionID,
		ContractVersionID: contractVersionID,
		EntityPolicyRef:   req.EntityPolicyRef,
		IndicatorSetRef:   req.IndicatorSetRef,
		Definition:        req.Definition,
		Assets:            assets,
		ActorID:           actorID,
		TraceID:           httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "PRODUCT_VERSION_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, versionResponse(version))
}

func (h *Handler) getVersion(w http.ResponseWriter, r *http.Request) {
	versionID, ok := parsePathUUID(w, r, "versionId", "INVALID_PRODUCT_VERSION_ID")
	if !ok {
		return
	}
	version, err := h.repo.GetVersion(r.Context(), versionID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "PRODUCT_VERSION_NOT_FOUND", "product version not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "PRODUCT_VERSION_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, versionResponse(version))
}

type createReleaseRequest struct {
	ProductVersionID string                  `json:"productVersionId"`
	ReleaseNo        string                  `json:"releaseNo"`
	Datasets         []releaseDatasetRequest `json:"datasets"`
	ReleaseNotes     string                  `json:"releaseNotes"`
	Metadata         map[string]any          `json:"metadata"`
}

type releaseDatasetRequest struct {
	DatasetVersionID string `json:"datasetVersionId"`
	Role             string `json:"role"`
}

func (h *Handler) createRelease(w http.ResponseWriter, r *http.Request) {
	productID, ok := parsePathUUID(w, r, "productId", "INVALID_PRODUCT_ID")
	if !ok {
		return
	}
	var req createReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	productVersionID, err := uuid.Parse(req.ProductVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_PRODUCT_VERSION_ID", "productVersionId must be a UUID", nil)
		return
	}
	datasets := make([]domain.ReleaseDataset, 0, len(req.Datasets))
	for _, item := range req.Datasets {
		datasetVersionID, err := uuid.Parse(item.DatasetVersionID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_VERSION_ID", "datasetVersionId must be a UUID", nil)
			return
		}
		datasets = append(datasets, domain.ReleaseDataset{DatasetVersionID: datasetVersionID, Role: domain.DatasetRole(item.Role)})
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	release, err := h.service.CreateRelease(r.Context(), application.CreateReleaseCommand{
		ProductID:        productID,
		ProductVersionID: productVersionID,
		ReleaseNo:        req.ReleaseNo,
		Datasets:         datasets,
		ReleaseNotes:     req.ReleaseNotes,
		Metadata:         req.Metadata,
		ActorID:          actorID,
		TraceID:          httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "PRODUCT_RELEASE_CREATE_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, releaseResponse(release))
}

func (h *Handler) getRelease(w http.ResponseWriter, r *http.Request) {
	releaseID, ok := parsePathUUID(w, r, "releaseId", "INVALID_RELEASE_ID")
	if !ok {
		return
	}
	release, err := h.repo.GetRelease(r.Context(), releaseID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "PRODUCT_RELEASE_NOT_FOUND", "product release not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "PRODUCT_RELEASE_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, releaseResponse(release))
}

func (h *Handler) getReadiness(w http.ResponseWriter, r *http.Request) {
	releaseID, ok := parsePathUUID(w, r, "releaseId", "INVALID_RELEASE_ID")
	if !ok {
		return
	}
	result, err := h.service.Readiness(r.Context(), releaseID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "PRODUCT_RELEASE_NOT_FOUND", "product release not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "RELEASE_READINESS_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func productResponse(product domain.DataProduct) map[string]any {
	return map[string]any{
		"id":               product.ID,
		"workspaceId":      product.WorkspaceID,
		"projectId":        product.ProjectID,
		"useCaseId":        product.UseCaseID,
		"code":             product.Code,
		"name":             product.Name,
		"description":      product.Description,
		"domainCode":       product.DomainCode,
		"ownerId":          product.OwnerID,
		"lifecycleStatus":  product.LifecycleStatus,
		"healthStatus":     product.HealthStatus,
		"currentVersionId": product.CurrentVersionID,
		"latestReleaseId":  product.LatestReleaseID,
		"metadata":         product.Metadata,
		"createdAt":        product.CreatedAt,
	}
}

func versionResponse(version domain.ProductVersion) map[string]any {
	assets := make([]map[string]any, 0, len(version.Assets))
	for _, asset := range version.Assets {
		assets = append(assets, map[string]any{
			"id":             asset.ID,
			"assetType":      asset.AssetType,
			"name":           asset.Name,
			"datasetId":      asset.DatasetID,
			"externalRef":    asset.ExternalRef,
			"deliveryConfig": asset.DeliveryConfig,
			"schemaSnapshot": asset.SchemaSnapshot,
		})
	}
	return map[string]any{
		"id":                version.ID,
		"productId":         version.ProductID,
		"version":           version.Semver(),
		"workflowVersionId": version.WorkflowVersionID,
		"contractVersionId": version.ContractVersionID,
		"entityPolicyRef":   version.EntityPolicyRef,
		"indicatorSetRef":   version.IndicatorSetRef,
		"definition":        version.DefinitionSnapshot,
		"assets":            assets,
		"createdAt":         version.CreatedAt,
	}
}

func releaseResponse(release domain.ProductRelease) map[string]any {
	datasets := make([]map[string]any, 0, len(release.Datasets))
	for _, binding := range release.Datasets {
		datasets = append(datasets, map[string]any{
			"datasetVersionId": binding.DatasetVersionID,
			"role":             binding.Role,
		})
	}
	return map[string]any{
		"id":                   release.ID,
		"productId":            release.ProductID,
		"productVersionId":     release.ProductVersionID,
		"releaseNo":            release.ReleaseNo,
		"status":               release.Status,
		"contractVersionId":    release.ContractVersionID,
		"rightsSnapshotId":     release.RightsSnapshotID,
		"qualityResultId":      release.QualityResultID,
		"complianceResultId":   release.ComplianceResultID,
		"evidenceSnapshotId":   release.EvidenceSnapshotID,
		"datasets":             datasets,
		"releaseNotes":         release.ReleaseNotes,
		"metadata":             release.Metadata,
		"createdAt":            release.CreatedAt,
		"releasedAt":           release.ReleasedAt,
	}
}

func parseSemver(value string) (int, int, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("version must be MAJOR.MINOR.PATCH")
	}
	values := make([]int, 3)
	for i, part := range parts {
		parsed, err := strconv.Atoi(part)
		if err != nil || parsed < 0 {
			return 0, 0, 0, fmt.Errorf("version must contain non-negative integer components")
		}
		values[i] = parsed
	}
	return values[0], values[1], values[2], nil
}

func optionalUUID(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseActorID(r *http.Request) (*uuid.UUID, error) {
	return optionalUUID(r.Header.Get("X-Actor-ID"))
}

func parsePathUUID(w http.ResponseWriter, r *http.Request, name, code string) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, code, name+" must be a UUID", nil)
		return uuid.Nil, false
	}
	return parsed, true
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
