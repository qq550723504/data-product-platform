package compliancehttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

type Handler struct {
	service *application.Service
	repo    *infrastructure.PostgresRepository
}

func NewHandler(service *application.Service, repo *infrastructure.PostgresRepository) *Handler {
	return &Handler{service: service, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/dataset-versions/{versionId}/compliance-checks", h.run)
	mux.HandleFunc("GET /api/v1/compliance-results/{resultId}", h.get)
}

type runRequest struct {
	WorkspaceID string `json:"workspaceId"`
	PolicyRef   string `json:"policyRef"`
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	var req runRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	policyRef := strings.TrimSpace(req.PolicyRef)
	if policyRef == "" {
		policyRef = "park/compliance/enterprise-activity-compliance-v1.yaml"
	}
	result, err := h.service.Run(r.Context(), application.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: versionID,
		PolicyRef:        policyRef,
		ActorID:          actorID,
		TraceID:          httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "COMPLIANCE_CHECK_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, resultResponse(result))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	resultID, err := uuid.Parse(r.PathValue("resultId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_COMPLIANCE_RESULT_ID", "resultId must be a UUID", nil)
		return
	}
	result, err := h.repo.GetResult(r.Context(), resultID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "COMPLIANCE_RESULT_NOT_FOUND", "compliance result not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "COMPLIANCE_RESULT_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, resultResponse(result))
}

func resultResponse(result domain.Result) map[string]any {
	return map[string]any{
		"id":               result.ID,
		"workspaceId":      result.WorkspaceID,
		"datasetVersionId": result.DatasetVersionID,
		"policyRef":        result.PolicyRef,
		"policyVersion":    result.PolicyVersion,
		"gateDecision":     result.GateDecision,
		"summary":          result.Summary,
		"findings":         result.Findings,
		"createdAt":        result.CreatedAt,
	}
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
