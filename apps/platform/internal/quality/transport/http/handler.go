package qualityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
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
	costRepo     *cost.QueryRepository
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

func NewHandlerWithCost(service *application.Service, repo *infrastructure.PostgresRepository, evidenceRepo *evidence.QueryRepository, costRepo *cost.QueryRepository) *Handler {
	return &Handler{service: service, repo: repo, evidenceRepo: evidenceRepo, costRepo: costRepo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/dataset-versions/{versionId}/quality-checks", h.run)
	mux.HandleFunc("POST /api/v1/dataset-versions/{versionId}/gold-quality-checks", h.runGold)
	mux.HandleFunc("GET /api/v1/quality-results/{resultId}", h.get)
	mux.HandleFunc("GET /api/v1/quality-assessments/{assessmentId}", h.getAssessment)
	mux.HandleFunc("GET /api/v1/quality-assessments/{assessmentId}/report", h.getReport)
	mux.HandleFunc("GET /api/v1/dataset-versions/{versionId}/quality-assessments", h.listAssessments)
	mux.HandleFunc("GET /api/v1/dataset-versions/{versionId}/quality-assessments/latest", h.latestAssessment)
}

type runRequest struct {
	WorkspaceID         string `json:"workspaceId"`
	RuleSetRef          string `json:"ruleSetRef"`
	AssessmentAttemptID string `json:"assessmentAttemptId"`
}

type runGoldRequest struct {
	WorkspaceID         string `json:"workspaceId"`
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
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}
	ruleSetRef := strings.TrimSpace(req.RuleSetRef)
	if ruleSetRef == "" {
		ruleSetRef = "park/quality/enterprise-activity-quality-v1.yaml"
	}
	var assessmentAttemptID uuid.UUID
	assessmentAttemptRef := strings.TrimSpace(req.AssessmentAttemptID)
	if assessmentAttemptRef == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "MISSING_ASSESSMENT_ATTEMPT_ID", "assessmentAttemptId is required for idempotent quality execution", nil)
		return
	}
	assessmentAttemptID, err = uuid.Parse(assessmentAttemptRef)
	if err != nil || assessmentAttemptID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ASSESSMENT_ATTEMPT_ID", "assessmentAttemptId must be a non-nil UUID", nil)
		return
	}
	result, err := h.service.Run(r.Context(), application.RunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    versionID,
		RuleSetRef:          ruleSetRef,
		AssessmentAttemptID: assessmentAttemptID,
		ActorID:             actorID,
		TraceID:             httpserver.RequestID(r.Context()),
		Now:                 time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, application.ErrAssessmentAttemptInProgress) {
			httpserver.WriteError(w, r, http.StatusConflict, "QUALITY_ASSESSMENT_ATTEMPT_IN_PROGRESS", "assessmentAttemptId is already being evaluated", nil)
			return
		}
		if errors.Is(err, application.ErrAssessmentAttemptFailed) {
			httpserver.WriteError(w, r, http.StatusConflict, "QUALITY_ASSESSMENT_ATTEMPT_FAILED", "assessmentAttemptId already has a failed evaluation", nil)
			return
		}
		if errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
			httpserver.WriteError(w, r, http.StatusBadRequest, "DATASET_WORKSPACE_MISMATCH", "the DatasetVersion must belong to the declared workspace", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusBadRequest, "QUALITY_CHECK_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, resultResponse(result))
}

func (h *Handler) runGold(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil || versionID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_VERSION_ID", "versionId must be a non-nil UUID", nil)
		return
	}
	var req runGoldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(strings.TrimSpace(req.WorkspaceID))
	if err != nil || workspaceID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a non-nil UUID", nil)
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

	result, err := h.service.RunGold(r.Context(), application.GoldRunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    versionID,
		AssessmentAttemptID: attemptID,
		ActorID:             actorID,
		TraceID:             httpserver.RequestID(r.Context()),
	})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrAssessmentAttemptInProgress):
			httpserver.WriteError(w, r, http.StatusConflict, "QUALITY_ASSESSMENT_ATTEMPT_IN_PROGRESS", "assessmentAttemptId is already being evaluated", nil)
		case errors.Is(err, application.ErrAssessmentAttemptFailed):
			httpserver.WriteError(w, r, http.StatusConflict, "QUALITY_ASSESSMENT_ATTEMPT_FAILED", "assessmentAttemptId already has a failed evaluation", nil)
		case errors.Is(err, datasetdomain.ErrDatasetWorkspace):
			httpserver.WriteError(w, r, http.StatusBadRequest, "DATASET_WORKSPACE_MISMATCH", "the DatasetVersion must belong to the declared workspace", nil)
		case errors.Is(err, application.ErrGoldQualityNotConfigured):
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "GOLD_QUALITY_NOT_CONFIGURED", "formal Gold quality assessment is not configured", nil)
		case errors.Is(err, application.ErrGoldProductionProof):
			httpserver.WriteError(w, r, http.StatusConflict, "GOLD_PRODUCTION_PROOF_INVALID", "formal Gold quality requires an exact FINALIZED production binding", nil)
		default:
			httpserver.WriteError(w, r, http.StatusBadRequest, "GOLD_QUALITY_CHECK_FAILED", err.Error(), nil)
		}
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
	if h.costRepo != nil {
		costEvents, err := h.costRepo.ListByQualityAssessment(r.Context(), assessmentID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_ASSESSMENT_COST_READ_FAILED", err.Error(), nil)
			return
		}
		response["costEvents"] = costEvents
	}
	writeJSON(w, http.StatusOK, response)
}

// getReport is the bounded customer-facing read model. It requires an explicit
// workspace and never hydrates the complete finding set.
func (h *Handler) getReport(w http.ResponseWriter, r *http.Request) {
	assessmentID, err := uuid.Parse(r.PathValue("assessmentId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_QUALITY_ASSESSMENT_ID", "assessmentId must be a UUID", nil)
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
	limit, offset, ok := findingPagination(w, r)
	if !ok {
		return
	}

	assessment, findings, err := h.repo.GetReport(r.Context(), workspaceID, assessmentID, limit, offset)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "QUALITY_ASSESSMENT_NOT_FOUND", "quality assessment not found in workspace", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_REPORT_READ_FAILED", err.Error(), nil)
		return
	}

	var evidenceItems []evidence.Reference
	if h.evidenceRepo != nil {
		evidenceItems, err = h.evidenceRepo.ListReferencesForObject(r.Context(), "QUALITY_RESULT", assessmentID, assessment.WorkspaceID, maxAssessmentLimit)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_REPORT_EVIDENCE_READ_FAILED", err.Error(), nil)
			return
		}
	}
	auditEvents, err := h.repo.ListAuditEventReferences(r.Context(), assessmentID, maxAssessmentLimit)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_REPORT_AUDIT_READ_FAILED", err.Error(), nil)
		return
	}

	writeJSON(w, http.StatusOK, reportResponse(assessment, findings, evidenceItems, auditEvents))
}

func (h *Handler) listAssessments(w http.ResponseWriter, r *http.Request) {
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_VERSION_ID", "versionId must be a UUID", nil)
		return
	}
	workspaceID, ok := requiredWorkspaceQuery(w, r)
	if !ok {
		return
	}
	belongs, err := h.repo.DatasetVersionBelongsToWorkspace(r.Context(), workspaceID, versionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_DATASET_WORKSPACE_READ_FAILED", err.Error(), nil)
		return
	}
	if !belongs {
		httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_VERSION_NOT_FOUND", "DatasetVersion was not found in workspace", nil)
		return
	}
	limit, offset, ok := assessmentPagination(w, r)
	if !ok {
		return
	}
	summaryOnly := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("summary")), "true")
	var page infrastructure.AssessmentPage
	if summaryOnly {
		page, err = h.repo.ListAssessmentSummaries(r.Context(), versionID, limit, offset)
	} else {
		page, err = h.repo.ListAssessments(r.Context(), versionID, limit, offset)
	}
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_ASSESSMENTS_READ_FAILED", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(page.Items))
	for _, assessment := range page.Items {
		if summaryOnly {
			items = append(items, assessmentSummaryResponse(assessment))
		} else {
			items = append(items, resultResponse(assessment))
		}
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

func requiredWorkspaceQuery(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("workspaceId"))
	if raw == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "WORKSPACE_REQUIRED", "workspaceId is required", nil)
		return uuid.Nil, false
	}
	workspaceID, err := uuid.Parse(raw)
	if err != nil || workspaceID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a non-nil UUID", nil)
		return uuid.Nil, false
	}
	return workspaceID, true
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

func findingPagination(w http.ResponseWriter, r *http.Request) (int, int, bool) {
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
	workspaceID, ok := requiredWorkspaceQuery(w, r)
	if !ok {
		return
	}
	belongs, err := h.repo.DatasetVersionBelongsToWorkspace(r.Context(), workspaceID, versionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "QUALITY_DATASET_WORKSPACE_READ_FAILED", err.Error(), nil)
		return
	}
	if !belongs {
		httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_VERSION_NOT_FOUND", "DatasetVersion was not found in workspace", nil)
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

func assessmentSummaryResponse(result domain.Assessment) map[string]any {
	return map[string]any{
		"id":                   result.ID,
		"workspaceId":          result.WorkspaceID,
		"datasetVersionId":     result.DatasetVersionID,
		"ruleSetRef":           result.RuleSetRef,
		"ruleSetVersion":       result.RuleSetVersion,
		"ruleSetContentSha256": result.RuleSetContentSHA256,
		"evaluatorName":        result.EvaluatorName,
		"evaluatorVersion":     result.EvaluatorVersion,
		"gateDecision":         result.GateDecision,
		"metrics":              result.Metrics,
		"dimensionSummary":     result.DimensionSummaries,
		"createdAt":            result.CreatedAt,
	}
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
		"dimensionSummary":     result.DimensionSummaries,
		"findings":             result.Findings,
		"createdAt":            result.CreatedAt,
	}
}

func reportResponse(result domain.Assessment, page domain.FindingPage, evidenceItems []evidence.Reference, auditEvents []infrastructure.AuditEventReference) map[string]any {
	findings := make([]map[string]any, 0, len(page.Items))
	for _, finding := range page.Items {
		findings = append(findings, reportFindingResponse(finding))
	}

	evidenceRefs := make([]map[string]any, 0, len(evidenceItems))
	for _, item := range evidenceItems {
		evidenceRefs = append(evidenceRefs, map[string]any{
			"id":            item.ID,
			"workspaceId":   item.WorkspaceID,
			"evidenceType":  item.EvidenceType,
			"relationType":  item.RelationType,
			"sourceType":    item.SourceType,
			"sourceId":      item.SourceID,
			"hashAlgorithm": item.HashAlgorithm,
			"hashValue":     item.HashValue,
			"createdAt":     item.CreatedAt,
			"createdBy":     item.CreatedBy,
		})
	}

	auditRefs := make([]map[string]any, 0, len(auditEvents))
	for _, event := range auditEvents {
		auditRefs = append(auditRefs, map[string]any{
			"id":         event.ID,
			"action":     event.Action,
			"objectType": event.ObjectType,
			"objectId":   event.ObjectID,
			"actorType":  event.ActorType,
			"actorId":    event.ActorID,
			"traceId":    event.TraceID,
			"occurredAt": event.OccurredAt,
		})
	}

	return map[string]any{
		"id":                   result.ID,
		"workspaceId":          result.WorkspaceID,
		"datasetVersionId":     result.DatasetVersionID,
		"gateDecision":         result.GateDecision,
		"ruleSetRef":           result.RuleSetRef,
		"ruleSetVersion":       result.RuleSetVersion,
		"ruleSetContentSha256": result.RuleSetContentSHA256,
		"evaluatorName":        result.EvaluatorName,
		"evaluatorVersion":     result.EvaluatorVersion,
		"createdAt":            result.CreatedAt,
		"createdBy":            result.CreatedBy,
		"dimensionSummary":     result.DimensionSummaries,
		"metrics":              map[string]any{"dimensions": result.Metrics["dimensions"]},
		"ruleSet": map[string]any{
			"ref":           result.RuleSetRef,
			"version":       result.RuleSetVersion,
			"contentSha256": result.RuleSetContentSHA256,
		},
		"evaluator": map[string]any{
			"name":    result.EvaluatorName,
			"version": result.EvaluatorVersion,
		},
		"provenance": map[string]any{
			"createdAt": result.CreatedAt,
			"createdBy": result.CreatedBy,
		},
		"findings": map[string]any{
			"items": findings,
			"page": map[string]int{
				"limit":  page.Limit,
				"offset": page.Offset,
				"total":  page.Total,
			},
		},
		"evidence":    evidenceRefs,
		"auditEvents": auditRefs,
	}
}

func reportFindingResponse(finding domain.Finding) map[string]any {
	status := string(finding.Status)
	if finding.Status == domain.FindingSkipped {
		status = string(domain.DimensionNotApplicable)
	}
	response := map[string]any{
		"id":            finding.ID,
		"resultId":      finding.ResultID,
		"ruleId":        finding.RuleID,
		"dimension":     finding.Dimension,
		"severity":      finding.Severity,
		"status":        status,
		"observed":      finding.Observed,
		"affectedCount": observedValue(finding.Observed, "affectedCount"),
		"sample":        observedValue(finding.Observed, "sample"),
		"reason":        finding.Message,
		"createdAt":     finding.CreatedAt,
	}
	return response
}

func observedValue(observed map[string]any, key string) any {
	if observed == nil {
		return nil
	}
	return observed[key]
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
