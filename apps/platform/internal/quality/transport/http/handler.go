package qualityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
)

type Handler struct {
	service *application.Service
	repo    *infrastructure.PostgresRepository
}

func NewHandler(service *application.Service, repo *infrastructure.PostgresRepository) *Handler {
	return &Handler{service: service, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/dataset-versions/{versionId}/quality-checks", h.run)
	mux.HandleFunc("GET /api/v1/quality-results/{resultId}", h.get)
}

type runRequest struct {
	WorkspaceID string `json:"workspaceId"`
	RuleSetRef  string `json:"ruleSetRef"`
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
	ruleSetRef := strings.TrimSpace(req.RuleSetRef)
	if ruleSetRef == "" {
		ruleSetRef = "park/quality/enterprise-activity-quality-v1.yaml"
	}
	result, err := h.service.Run(r.Context(), application.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: versionID,
		RuleSetRef:       ruleSetRef,
		ActorID:          actorID,
		TraceID:          httpserver.RequestID(r.Context()),
		Now:              time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "DATASET_WORKSPACE_MISMATCH", "the DatasetVersion must belong to the declared workspace", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusBadRequest, "QUALITY_CHECK_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, resultResponse(result))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	resultID, err := uuid.Parse(r.PathValue("resultId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_QUALITY_RESULT_ID", "resultId must be a UUID", nil)
		return
	}
	result, err := h.repo.GetResult(r.Context(), resultID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "QUALITY_RESULT_NOT_FOUND", "quality result not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_RESULT_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, resultResponse(result))
}

func resultResponse(result domain.Result) map[string]any {
	return map[string]any{
		"id":               result.ID,
		"workspaceId":      result.WorkspaceID,
		"datasetVersionId": result.DatasetVersionID,
		"ruleSetRef":       result.RuleSetRef,
		"ruleSetVersion":   result.RuleSetVersion,
		"gateDecision":     result.GateDecision,
		"metrics":          result.Metrics,
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
