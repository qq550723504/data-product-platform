package annotationhttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	platformprincipal "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type ReviewService interface {
	ReviewAnnotation(context.Context, annotationapp.ReviewAnnotationCommand) (annotationapp.ReviewAnnotationResult, error)
}

type GoldPreflightService interface {
	GoldQualityPreflight(context.Context, uuid.UUID, uuid.UUID) (annotationapp.GoldQualityPreflight, error)
}

type Handler struct {
	service  ReviewService
	resolver platformprincipal.Resolver
}

func NewHandler(service ReviewService, resolver platformprincipal.Resolver) *Handler {
	return &Handler{service: service, resolver: resolver}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(
		"POST /api/v1/workspaces/{workspaceId}/annotation-campaigns/{campaignId}/tasks/{taskId}/review",
		h.review,
	)
	mux.HandleFunc(
		"GET /api/v1/workspaces/{workspaceId}/annotation-campaigns/{campaignId}/gold-quality-preflight",
		h.goldQualityPreflight,
	)
}

type reviewRequest struct {
	ExpectedTaskRevision int64           `json:"expectedTaskRevision"`
	Action               string          `json:"action"`
	Reason               string          `json:"reason"`
	ReviewedResultID     string          `json:"reviewedResultId,omitempty"`
	CorrectedPayload     json.RawMessage `json:"correctedPayload,omitempty"`
}

func (h *Handler) review(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil || h.resolver == nil {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "HUMAN_DECISION_NOT_CONFIGURED", "human decision principal boundary is not configured", nil)
		return
	}

	workspaceID, err := parseNonNilUUID(r.PathValue("workspaceId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a non-nil UUID", nil)
		return
	}
	campaignID, err := parseNonNilUUID(r.PathValue("campaignId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ANNOTATION_CAMPAIGN_ID", "campaignId must be a non-nil UUID", nil)
		return
	}
	taskID, err := parseNonNilUUID(r.PathValue("taskId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ANNOTATION_TASK_ID", "taskId must be a non-nil UUID", nil)
		return
	}

	principal, err := h.resolver.Resolve(r, workspaceID, platformprincipal.CapabilityHumanDecision)
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

	var req reviewRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_REVIEW_REQUEST", "review request must be valid JSON with only supported fields", nil)
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	req.Reason = strings.TrimSpace(req.Reason)
	if req.ExpectedTaskRevision < 1 {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_EXPECTED_TASK_REVISION", "expectedTaskRevision must be positive", nil)
		return
	}
	if req.Action != annotationdomain.ReviewAccept && req.Action != annotationdomain.ReviewReject && req.Action != annotationdomain.ReviewCorrect {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_REVIEW_ACTION", "action must be ACCEPT, REJECT, or CORRECT", nil)
		return
	}
	if req.Reason == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "REVIEW_REASON_REQUIRED", "review reason is required", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required and must be at most 255 characters", nil)
		return
	}

	var reviewedResultID *uuid.UUID
	if value := strings.TrimSpace(req.ReviewedResultID); value != "" {
		id, parseErr := parseNonNilUUID(value)
		if parseErr != nil {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_REVIEWED_RESULT_ID", "reviewedResultId must be a non-nil UUID", nil)
			return
		}
		reviewedResultID = &id
	}
	if (req.Action == annotationdomain.ReviewAccept || req.Action == annotationdomain.ReviewCorrect) && reviewedResultID == nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "REVIEWED_RESULT_REQUIRED", "reviewedResultId is required for ACCEPT and CORRECT", nil)
		return
	}

	var correctedPayload []byte
	var correctedHash string
	if req.Action == annotationdomain.ReviewCorrect {
		correctedPayload = bytesTrimSpace(req.CorrectedPayload)
		if len(correctedPayload) == 0 || string(correctedPayload) == "null" {
			httpserver.WriteError(w, r, http.StatusBadRequest, "CORRECTED_PAYLOAD_REQUIRED", "correctedPayload is required for CORRECT", nil)
			return
		}
		sum := sha256.Sum256(correctedPayload)
		correctedHash = hex.EncodeToString(sum[:])
	} else if len(bytesTrimSpace(req.CorrectedPayload)) != 0 {
		httpserver.WriteError(w, r, http.StatusBadRequest, "CORRECTED_PAYLOAD_NOT_ALLOWED", "correctedPayload is only allowed for CORRECT", nil)
		return
	}

	actorID := principal.ActorID
	result, err := h.service.ReviewAnnotation(r.Context(), annotationapp.ReviewAnnotationCommand{
		WorkspaceID:          workspaceID,
		CampaignID:           campaignID,
		TaskID:               taskID,
		ExpectedTaskRevision: req.ExpectedTaskRevision,
		ReviewerRef:          actorID.String(),
		Action:               req.Action,
		Reason:               req.Reason,
		IdempotencyKey:       idempotencyKey,
		ReviewedResultID:     reviewedResultID,
		CorrectedPayload:     correctedPayload,
		CorrectedPayloadHash: correctedHash,
		ActorID:              &actorID,
		TraceID:              httpserver.RequestID(r.Context()),
	})
	if err != nil {
		switch {
		case errors.Is(err, annotationinfra.ErrStaleRevision):
			httpserver.WriteError(w, r, http.StatusConflict, "ANNOTATION_REVIEW_STALE", "annotation task changed; reload before reviewing again", nil)
		case errors.Is(err, annotationapp.ErrIdempotencyConflict):
			httpserver.WriteError(w, r, http.StatusConflict, "ANNOTATION_REVIEW_IDEMPOTENCY_CONFLICT", "Idempotency-Key is already bound to a different review request", nil)
		case errors.Is(err, annotationdomain.ErrInvalidReviewAttempt), errors.Is(err, annotationdomain.ErrInvalidReviewDecision):
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ANNOTATION_REVIEW", "annotation review request is invalid for the current facts", nil)
		default:
			httpserver.WriteError(w, r, http.StatusConflict, "ANNOTATION_REVIEW_FAILED", "annotation review failed closed", nil)
		}
		return
	}

	response := map[string]any{
		"attemptId": result.Attempt.ID,
		"outcome":   result.Outcome.Outcome,
	}
	if result.Decision != nil {
		response["decisionId"] = result.Decision.ID
		response["decision"] = result.Decision.Outcome
		response["reviewerRef"] = result.Decision.ReviewerRef
		response["reviewedResultId"] = result.Decision.ReviewedResultID
		response["selectedResultId"] = result.Decision.SelectedResultID
	}
	writeJSON(w, http.StatusOK, response)
}

func parseNonNilUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, errors.New("invalid UUID")
	}
	return id, nil
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}


func (h *Handler) goldQualityPreflight(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "ANNOTATION_SERVICE_NOT_CONFIGURED", "annotation service is not configured", nil)
		return
	}
	service, ok := h.service.(GoldPreflightService)
	if !ok {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "GOLD_QUALITY_PREFLIGHT_NOT_CONFIGURED", "Gold quality preflight is not configured", nil)
		return
	}
	workspaceID, err := parseNonNilUUID(r.PathValue("workspaceId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a non-nil UUID", nil)
		return
	}
	campaignID, err := parseNonNilUUID(r.PathValue("campaignId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ANNOTATION_CAMPAIGN_ID", "campaignId must be a non-nil UUID", nil)
		return
	}
	result, err := service.GoldQualityPreflight(r.Context(), workspaceID, campaignID)
	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows), errors.Is(err, annotationinfra.ErrCampaignNotFound):
			httpserver.WriteError(w, r, http.StatusNotFound, "ANNOTATION_SNAPSHOT_NOT_FOUND", "finalized annotation snapshot was not found", nil)
		case errors.Is(err, annotationdomain.ErrInvalidSnapshot):
			httpserver.WriteError(w, r, http.StatusConflict, "GOLD_QUALITY_PREFLIGHT_UNAVAILABLE", "Gold quality preflight requires a valid FINALIZED annotation snapshot", nil)
		default:
			httpserver.WriteError(w, r, http.StatusInternalServerError, "GOLD_QUALITY_PREFLIGHT_FAILED", "Gold quality preflight failed", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
