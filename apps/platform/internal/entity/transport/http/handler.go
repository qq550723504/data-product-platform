package entityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	platformprincipal "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type Handler struct {
	service  *application.MatchService
	repo     *infrastructure.PostgresRepository
	resolver platformprincipal.Resolver
}

func NewHandler(service *application.MatchService, repo *infrastructure.PostgresRepository, resolvers ...platformprincipal.Resolver) *Handler {
	var resolver platformprincipal.Resolver
	if len(resolvers) > 0 {
		resolver = resolvers[0]
	}
	return &Handler{service: service, repo: repo, resolver: resolver}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/entity-match-jobs", h.startJob)
	mux.HandleFunc("GET /api/v1/entity-match-jobs/{jobId}", h.getJob)
	mux.HandleFunc("GET /api/v1/entity-match-jobs/{jobId}/reviews", h.listReviews)
	// Targeted candidate lookup: review actions must verify one candidate without
	// transferring every candidate payload of a large match job.
	mux.HandleFunc("GET /api/v1/entity-match-reviews/{candidateId}", h.getReviewCandidate)
	mux.HandleFunc("POST /api/v1/entity-match-reviews/{candidateId}/confirm", h.confirm)
	mux.HandleFunc("POST /api/v1/entity-match-reviews/{candidateId}/reject", h.reject)
	mux.HandleFunc("GET /api/v1/entity-mappings", h.getMappingBySource)
	mux.HandleFunc("GET /api/v1/entities/{entityId}/mappings", h.listEntityMappings)
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
		code, message := "ENTITY_MATCH_JOB_FAILED", err.Error()
		switch {
		case errors.Is(err, datasetdomain.ErrDatasetWorkspace):
			code, message = "DATASET_WORKSPACE_MISMATCH", "input and output datasets must belong to the job workspace"
		case errors.Is(err, domain.ErrOutputDatasetType):
			code, message = "OUTPUT_DATASET_TYPE_INVALID", "entity resolution requires a STANDARDIZED output dataset"
		}
		httpserver.WriteError(w, r, http.StatusBadRequest, code, message, nil)
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

func (h *Handler) getReviewCandidate(w http.ResponseWriter, r *http.Request) {
	candidateID, err := uuid.Parse(r.PathValue("candidateId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CANDIDATE_ID", "candidateId must be a UUID", nil)
		return
	}
	candidate, err := h.repo.GetCandidate(r.Context(), candidateID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "ENTITY_MATCH_CANDIDATE_NOT_FOUND", "entity match candidate not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_MATCH_CANDIDATE_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, candidateResponse(candidate))
}

func (h *Handler) getMappingBySource(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := requiredWorkspaceID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "WORKSPACE_REQUIRED", "workspaceId must be a UUID", nil)
		return
	}
	sourceType := r.URL.Query().Get("sourceType")
	sourceRef := r.URL.Query().Get("sourceRef")
	sourceKey := r.URL.Query().Get("sourceKey")
	if sourceType == "" || sourceRef == "" || sourceKey == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "SOURCE_MAPPING_QUERY_REQUIRED", "sourceType, sourceRef and sourceKey are required", nil)
		return
	}
	mapping, err := h.repo.GetMappingBySource(r.Context(), workspaceID, sourceType, sourceRef, sourceKey)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "ENTITY_MAPPING_NOT_FOUND", "entity mapping not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_MAPPING_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, mappingResponse(mapping))
}

func (h *Handler) listEntityMappings(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := requiredWorkspaceID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "WORKSPACE_REQUIRED", "workspaceId must be a UUID", nil)
		return
	}
	entityID, err := uuid.Parse(r.PathValue("entityId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ENTITY_ID", "entityId must be a UUID", nil)
		return
	}
	mappings, err := h.repo.ListMappingsByEntity(r.Context(), workspaceID, entityID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_MAPPINGS_READ_FAILED", err.Error(), nil)
		return
	}
	items := make([]map[string]any, 0, len(mappings))
	for _, mapping := range mappings {
		items = append(items, mappingResponse(mapping))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type reviewRequest struct {
	Reason string `json:"reason"`
	// ExpectedDecisionID is an optional optimistic concurrency token. When set,
	// the review only succeeds while this decision is still the current one.
	ExpectedDecisionID string `json:"expectedDecisionId"`
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
	if h == nil || h.repo == nil || h.service == nil || h.resolver == nil {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "HUMAN_DECISION_NOT_CONFIGURED", "human decision principal boundary is not configured", nil)
		return
	}
	candidate, err := h.repo.GetCandidate(r.Context(), candidateID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "ENTITY_MATCH_CANDIDATE_NOT_FOUND", "entity match candidate not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_MATCH_CANDIDATE_READ_FAILED", "entity match candidate lookup failed", nil)
		return
	}
	job, err := h.repo.GetJob(r.Context(), candidate.JobID)
	if err != nil {
		if errors.Is(err, infrastructure.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "ENTITY_MATCH_JOB_NOT_FOUND", "entity match job not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "ENTITY_MATCH_JOB_READ_FAILED", "entity match job lookup failed", nil)
		return
	}
	principal, err := h.resolver.Resolve(r, job.WorkspaceID, platformprincipal.CapabilityHumanDecision)
	if err != nil {
		switch {
		case errors.Is(err, platformprincipal.ErrNotConfigured):
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "HUMAN_DECISION_NOT_CONFIGURED", "human decision principal boundary is not configured", nil)
		case errors.Is(err, platformprincipal.ErrUnauthenticated):
			httpserver.WriteError(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authenticated reviewer is required", nil)
		case errors.Is(err, platformprincipal.ErrWorkspaceDenied), errors.Is(err, platformprincipal.ErrCapabilityDenied):
			httpserver.WriteError(w, r, http.StatusForbidden, "WORKSPACE_ACCESS_DENIED", "authenticated reviewer is not authorized for this workspace", nil)
		default:
			httpserver.WriteError(w, r, http.StatusInternalServerError, "PRINCIPAL_RESOLUTION_FAILED", "reviewer principal resolution failed", nil)
		}
		return
	}
	actorID := principal.ActorID
	var req reviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	var expectedDecisionID *uuid.UUID
	if strings.TrimSpace(req.ExpectedDecisionID) != "" {
		parsed, err := uuid.Parse(req.ExpectedDecisionID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_EXPECTED_DECISION_ID", "expectedDecisionId must be a UUID", nil)
			return
		}
		expectedDecisionID = &parsed
	}
	command := application.ReviewCommand{
		CandidateID:        candidateID,
		ReviewerID:         actorID,
		Reason:             req.Reason,
		TraceID:            httpserver.RequestID(r.Context()),
		ExpectedDecisionID: expectedDecisionID,
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
		switch {
		case errors.Is(err, infrastructure.ErrNotFound):
			status = http.StatusNotFound
			code = "ENTITY_MATCH_CANDIDATE_NOT_FOUND"
		case errors.Is(err, domain.ErrReviewerReasonRequired):
			status = http.StatusBadRequest
			code = "REVIEW_REASON_REQUIRED"
		case errors.Is(err, domain.ErrMappingDecisionConflict):
			code = "ENTITY_MAPPING_DECISION_CONFLICT"
		case errors.Is(err, domain.ErrMappingDecisionExpectationRequired):
			code = "ENTITY_MAPPING_DECISION_EXPECTATION_REQUIRED"
		case errors.Is(err, domain.ErrMappingConfirmedImmutable):
			code = "ENTITY_MAPPING_CONFIRMED_IMMUTABLE"
		case errors.Is(err, domain.ErrMappingDecisionKeyConflict):
			code = "ENTITY_MAPPING_DECISION_KEY_CONFLICT"
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
		"id":                 candidate.ID,
		"jobId":              candidate.JobID,
		"sourceKey":          candidate.SourceKey,
		"sourceName":         candidate.SourceName,
		"candidateEntityId":  candidate.CandidateEntityID,
		"decision":           candidate.Decision,
		"status":             candidate.Status,
		"matchMethod":        candidate.MatchMethod,
		"matchRuleId":        candidate.MatchRuleID,
		"matchEngineName":    candidate.MatchEngineName,
		"matchEngineVersion": candidate.MatchEngineVersion,
		"matchModelVersion":  candidate.MatchModelVersion,
		"confidence":         candidate.Confidence,
		"source":             candidate.SourcePayload,
		"normalized":         candidate.NormalizedPayload,
		"reviewerReason":     candidate.ReviewerReason,
		"evidenceId":         candidate.EvidenceID,
	}
}

func mappingResponse(mapping domain.EntityMapping) map[string]any {
	return map[string]any{
		"id":                 mapping.ID,
		"workspaceId":        mapping.WorkspaceID,
		"entityId":           mapping.EntityID,
		"sourceType":         mapping.SourceType,
		"sourceRef":          mapping.SourceRef,
		"sourceKey":          mapping.SourceKey,
		"sourceName":         mapping.SourceName,
		"matchMethod":        mapping.MatchMethod,
		"matchRuleId":        mapping.MatchRuleID,
		"matchPolicyVersion": mapping.MatchPolicyVersion,
		"matchEngineName":    mapping.MatchEngineName,
		"matchEngineVersion": mapping.MatchEngineVersion,
		"matchModelVersion":  mapping.MatchModelVersion,
		"confidence":         mapping.Confidence,
		"status":             mapping.Status,
		"reviewedBy":         mapping.ReviewedBy,
		"reviewedAt":         mapping.ReviewedAt,
		"reviewerReason":     mapping.ReviewerReason,
		"evidenceId":         mapping.EvidenceID,
		"currentDecisionId":  mapping.CurrentDecisionID,
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

// requiredWorkspaceID enforces an explicit workspace on mapping reads. Mappings
// are only unique inside a workspace, so there is no safe global fallback.
func requiredWorkspaceID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(r.URL.Query().Get("workspaceId"))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
