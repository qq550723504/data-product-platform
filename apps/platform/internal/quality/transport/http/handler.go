package qualityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
)

type Handler struct {
	service      *application.Service
	repo         *infrastructure.PostgresRepository
	evidenceRepo *evidence.QueryRepository
}

const (
	defaultAssessmentLimit = 25
	maxAssessmentLimit     = 100
)

func NewHandler(service *application.Service, repo *infrastructure.PostgresRepository, evidenceRepos ...*evidence.QueryRepository) *Handler {
	var evidenceRepo *evidence.QueryRepository
	if len(evidenceRepos) > 0 {
		evidenceRepo = evidenceRepos[0]
	}
	return &Handler{service: service, repo: repo, evidenceRepo: evidenceRepo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/dataset-versions/{versionId}/quality-checks", h.run)
	mux.HandleFunc("GET /api/v1/quality-results/{resultId}", h.get)
	mux.HandleFunc("GET /api/v1/quality-assessments/{assessmentId}", h.getAssessment)
	mux.HandleFunc("GET /api/v1/dataset-versions/{versionId}/quality-assessments", h.listAssessments)
	mux.HandleFunc("GET /api/v1/dataset-versions/{versionId}/quality-assessments/latest", h.latestAssessment)
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
	result, err := h.repo.GetAssessment(r.Context(), resultID)
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

func (h *Handler) getAssessment(w http.ResponseWriter, r *http.Request) {
	assessmentID, err := uuid.Parse(r.PathValue("assessmentId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_QUALITY_ASSESSMENT_ID", "assessmentId must be a UUID", nil)
		return
	}
	assessment, err := h.repo.GetAssessment(r.Context(), assessmentID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "QUALITY_ASSESSMENT_NOT_FOUND", "quality assessment not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_ASSESSMENT_READ_FAILED", err.Error(), nil)
		return
	}
	response := resultResponse(assessment)
	if h.evidenceRepo != nil {
		evidenceItems, err := h.evidenceRepo.ListForObject(r.Context(), "QUALITY_RESULT", assessmentID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_ASSESSMENT_EVIDENCE_READ_FAILED", err.Error(), nil)
			return
		}
		auditEvents, err := h.repo.ListAuditEvents(r.Context(), assessmentID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_ASSESSMENT_AUDIT_READ_FAILED", err.Error(), nil)
			return
		}
		response["evidence"] = evidenceItems
		response["auditEvents"] = auditEvents
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) listAssessments(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	limit, offset, ok := assessmentPagination(w, r)
	if !ok {
		return
	}
	page, err := h.repo.ListAssessments(r.Context(), versionID, limit, offset)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_ASSESSMENTS_READ_FAILED", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, assessment := range page.Items {
		items = append(items, resultResponse(assessment))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"datasetVersionId": versionID,
		"items":            items,
		"page": map[string]int{
			"limit":  page.Limit,
			"offset": page.Offset,
			"total":  page.Total,
		},
	})
}

func assessmentPagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit := defaultAssessmentLimit
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxAssessmentLimit {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_LIMIT", "limit must be between 1 and 100", nil)
			return 0, 0, false
		}
		limit = parsed
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OFFSET", "offset must be zero or greater", nil)
			return 0, 0, false
		}
		offset = parsed
	}
	return limit, offset, true
}

func (h *Handler) latestAssessment(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	assessment, err := h.repo.LatestAssessment(r.Context(), versionID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "QUALITY_ASSESSMENT_NOT_FOUND", "quality assessment not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_ASSESSMENT_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, resultResponse(assessment))
}

func resultResponse(result domain.Result) map[string]any {
	return map[string]any{
		"id":                   result.ID,
		"workspaceId":          result.WorkspaceID,
		"datasetVersionId":     result.DatasetVersionID,
		"ruleSetRef":           result.RuleSetRef,
		"ruleSetVersion":       result.RuleSetVersion,
		"ruleSetContentSha256": result.RuleSetContentSHA256,
		"ruleSetContent":       result.RuleSetContent,
		"evaluatorName":        result.EvaluatorName,
		"evaluatorVersion":     result.EvaluatorVersion,
		"gateDecision":         result.GateDecision,
		"metrics":              result.Metrics,
		"findings":             result.Findings,
		"createdAt":            result.CreatedAt,
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
