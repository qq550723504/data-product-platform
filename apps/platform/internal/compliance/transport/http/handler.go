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
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
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
	mux.HandleFunc("GET /api/v1/compliance-assessment-attempts/{attemptId}", h.getAssessmentAttempt)
}

type runRequest struct {
	WorkspaceID         string `json:"workspaceId"`
	PolicyRef           string `json:"policyRef"`
	AssessmentAttemptID string `json:"assessmentAttemptId"`
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
	attemptID, err := uuid.Parse(strings.TrimSpace(req.AssessmentAttemptID))
	if err != nil || attemptID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ASSESSMENT_ATTEMPT_ID", "assessmentAttemptId must be a non-nil UUID", nil)
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
		AssessmentAttemptID: attemptID,
		WorkspaceID:          workspaceID,
		DatasetVersionID: versionID,
		PolicyRef:        policyRef,
		ActorID:          actorID,
		TraceID:          httpserver.RequestID(r.Context()),
	})
	if err != nil {
		if errors.Is(err, infrastructure.ErrAssessmentAttemptConflict) {
			httpserver.WriteError(w, r, http.StatusConflict, "COMPLIANCE_ASSESSMENT_ATTEMPT_CONFLICT", "assessmentAttemptId is already bound to a different compliance request", nil)
			return
		}
		if errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "DATASET_WORKSPACE_MISMATCH", "the DatasetVersion must belong to the declared workspace", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusBadRequest, "COMPLIANCE_CHECK_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, resultResponse(result))
}

func (h *Handler) getAssessmentAttempt(w http.ResponseWriter, r *http.Request) {
	attemptID, err := uuid.Parse(strings.TrimSpace(r.PathValue("attemptId")))
	if err != nil || attemptID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ASSESSMENT_ATTEMPT_ID", "attemptId must be a non-nil UUID", nil)
		return
	}
	workspaceValue := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if workspaceValue == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "WORKSPACE_REQUIRED", "workspaceId is required", nil)
		return
	}
	workspaceID, err := uuid.Parse(workspaceValue)
	if err != nil || workspaceID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a non-nil UUID", nil)
		return
	}
	result, err := h.repo.GetResultByAssessmentAttempt(r.Context(), attemptID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "COMPLIANCE_ASSESSMENT_ATTEMPT_NOT_FOUND", "compliance assessment attempt not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "COMPLIANCE_ASSESSMENT_ATTEMPT_READ_FAILED", "compliance assessment attempt read failed", nil)
		return
	}
	if result.WorkspaceID != workspaceID {
		httpserver.WriteError(w, r, http.StatusNotFound, "COMPLIANCE_ASSESSMENT_ATTEMPT_NOT_FOUND", "compliance assessment attempt not found in workspace", nil)
		return
	}
	writeJSON(w, http.StatusOK, resultResponse(result))
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
	response := map[string]any{
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
	if result.AssessmentAttemptID != uuid.Nil {
		response["assessmentAttemptId"] = result.AssessmentAttemptID
	}
	return response
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
