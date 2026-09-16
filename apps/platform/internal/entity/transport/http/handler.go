package entityhttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

type Handler struct {
	service *application.MatchService
	repo    *infrastructure.PostgresRepository
}

func NewHandler(service *application.MatchService, repo *infrastructure.PostgresRepository) *Handler {
	return &Handler{service: service, repo: repo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/entity-match-jobs", h.startJob)
	mux.HandleFunc("GET /api/v1/entity-match-jobs/{jobId}", h.getJob)
	mux.HandleFunc("GET /api/v1/entity-match-jobs/{jobId}/reviews", h.listReviews)
	mux.HandleFunc("POST /api/v1/entity-match-reviews/{candidateId}/confirm", h.confirm)
	mux.HandleFunc("POST /api/v1/entity-match-reviews/{candidateId}/reject", h.reject)
}

type startJobRequest struct {
	WorkspaceID           string `json:"workspaceId"`
	InputDatasetVersionID string `json:"inputDatasetVersionId"`
	OutputDatasetID       string `json:"outputDatasetId"`
	SourceType            string `json:"sourceType"`
	SourceRef             string `json:"sourceRef"`
	SourceRole            string `json:"sourceRole"`
	PolicyRef             string `json:"policyRef"`
}

func (h *Handler) startJob(w http.ResponseWriter, r *http.Request) {
	var req startJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	workspaceID, err := uuid.Parse(req.WorkspaceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a UUID", nil)
		return
	}
	inputID, err := uuid.Parse(req.InputDatasetVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_INPUT_VERSION_ID", "inputDatasetVersionId must be a UUID", nil)
		return
	}
	outputID, err := uuid.Parse(req.OutputDatasetID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OUTPUT_DATASET_ID", "outputDatasetId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}

	job, err := h.service.Start(r.Context(), application.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: inputID,
		OutputDatasetID:       outputID,
		SourceType:            req.SourceType,
		SourceRef:             req.SourceRef,
		SourceRole:            domain.SourceRole(req.SourceRole),
		PolicyRef:             req.PolicyRef,
		ActorID:               actorID,
		TraceID:               httpserver.RequestID(r.Context()),
	})
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "ENTITY_MATCH_JOB_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, jobResponse(job))
}

func (h *Handler) getJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(r.PathValue("jobId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JOB_ID", "jobId must be a UUID", nil)
		return
	}
	job, err := h.repo.GetJob(r.Context(), jobID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "ENTITY_MATCH_JOB_NOT_FOUND", "entity match job not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_MATCH_JOB_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, jobResponse(job))
}

func (h *Handler) listReviews(w http.ResponseWriter, r *http.Request) {
	jobID, err := uuid.Parse(r.PathValue("jobId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JOB_ID", "jobId must be a UUID", nil)
		return
	}
	candidates, err := h.repo.ListCandidates(r.Context(), jobID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_MATCH_REVIEWS_READ_FAILED", err.Error(), nil)
		return
	}
	statusFilter := r.URL.Query().Get("status")
	result := make([]map[string]any, 0)
	for _, candidate := range candidates {
		if statusFilter != "" && string(candidate.Status) != statusFilter {
			continue
		}
		result = append(result, candidateResponse(candidate))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

type reviewRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, true)
}

func (h *Handler) reject(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, false)
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request, confirm bool) {
	candidateID, err := uuid.Parse(r.PathValue("candidateId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CANDIDATE_ID", "candidateId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil || actorID == nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "REVIEWER_REQUIRED", "valid X-Actor-ID is required for manual review", nil)
		return
	}
	var req reviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	command := application.ReviewCommand{
		CandidateID: candidateID,
		ReviewerID:  *actorID,
		Reason:      req.Reason,
		TraceID:     httpserver.RequestID(r.Context()),
	}
	var job domain.MatchJob
	if confirm {
		job, err = h.service.Confirm(r.Context(), command)
	} else {
		job, err = h.service.Reject(r.Context(), command)
	}
	if err != nil {
		status := http.StatusConflict
		code := "ENTITY_MATCH_REVIEW_FAILED"
		if errors.Is(err, infrastructure.ErrNotFound) {
			status = http.StatusNotFound
			code = "ENTITY_MATCH_CANDIDATE_NOT_FOUND"
		} else if errors.Is(err, domain.ErrReviewerReasonRequired) {
			status = http.StatusBadRequest
			code = "REVIEW_REASON_REQUIRED"
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, jobResponse(job))
}

func jobResponse(job domain.MatchJob) map[string]any {
	return map[string]any{
		"id":                     job.ID,
		"workspaceId":            job.WorkspaceID,
		"entityTypeId":           job.EntityTypeID,
		"inputDatasetVersionId":  job.InputDatasetVersionID,
		"outputDatasetId":        job.OutputDatasetID,
		"outputDatasetVersionId": job.OutputDatasetVersionID,
		"sourceRole":             job.SourceRole,
		"policyRef":              job.PolicyRef,
		"policyVersion":          job.PolicyVersion,
		"status":                 job.Status,
		"autoMatchCount":         job.AutoMatchCount,
		"reviewCount":            job.ReviewCount,
		"unresolvedCount":        job.UnresolvedCount,
		"rejectedCount":          job.RejectedCount,
		"errorMessage":           job.ErrorMessage,
	}
}

func candidateResponse(candidate domain.MatchCandidate) map[string]any {
	return map[string]any{
		"id":                candidate.ID,
		"jobId":             candidate.JobID,
		"sourceKey":         candidate.SourceKey,
		"sourceName":        candidate.SourceName,
		"candidateEntityId": candidate.CandidateEntityID,
		"decision":          candidate.Decision,
		"status":            candidate.Status,
		"matchMethod":       candidate.MatchMethod,
		"matchRuleId":       candidate.MatchRuleID,
		"confidence":        candidate.Confidence,
		"source":            candidate.SourcePayload,
		"normalized":        candidate.NormalizedPayload,
		"reviewerReason":    candidate.ReviewerReason,
		"evidenceId":        candidate.EvidenceID,
	}
}

func parseActorID(r *http.Request) (*uuid.UUID, error) {
	value := r.Header.Get("X-Actor-ID")
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
