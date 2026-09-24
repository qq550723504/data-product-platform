package goldhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	goldapp "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

type Handler struct {
	service *goldapp.Service
}

func NewHandler(service *goldapp.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/gold-builds", h.create)
}

type createBuildRequest struct {
	WorkflowVersionID                string `json:"workflowVersionId"`
	OutputDatasetID                  string `json:"outputDatasetId"`
	InputDatasetVersionID            string `json:"inputDatasetVersionId"`
	InputCertificationID             string `json:"inputCertificationId"`
	AnnotationCampaignID             string `json:"annotationCampaignId"`
	AnnotationSnapshotID             string `json:"annotationSnapshotId"`
	AnnotationContributionResourceID string `json:"annotationContributionResourceId"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.service == nil {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "GOLD_BUILDER_NOT_CONFIGURED", "Gold dataset builder is not configured", nil)
		return
	}
	workspaceID, err := parseUUID(r.PathValue("workspaceId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a non-nil UUID", nil)
		return
	}
	var body createBuildRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_GOLD_BUILD_REQUEST", "Gold build request must be valid JSON", nil)
		return
	}
	workflowVersionID, err := parseUUID(body.WorkflowVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKFLOW_VERSION_ID", "workflowVersionId must be a non-nil UUID", nil)
		return
	}
	outputDatasetID, err := parseUUID(body.OutputDatasetID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OUTPUT_DATASET_ID", "outputDatasetId must be a non-nil UUID", nil)
		return
	}
	inputDatasetVersionID, err := parseUUID(body.InputDatasetVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_INPUT_DATASET_VERSION_ID", "inputDatasetVersionId must be a non-nil UUID", nil)
		return
	}
	inputCertificationID, err := parseUUID(body.InputCertificationID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_INPUT_CERTIFICATION_ID", "inputCertificationId must be a non-nil UUID", nil)
		return
	}
	campaignID, err := parseUUID(body.AnnotationCampaignID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ANNOTATION_CAMPAIGN_ID", "annotationCampaignId must be a non-nil UUID", nil)
		return
	}
	snapshotID, err := parseUUID(body.AnnotationSnapshotID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ANNOTATION_SNAPSHOT_ID", "annotationSnapshotId must be a non-nil UUID", nil)
		return
	}
	contributionID, err := parseUUID(body.AnnotationContributionResourceID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ANNOTATION_CONTRIBUTION_RESOURCE_ID", "annotationContributionResourceId must be a non-nil UUID", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key is required", nil)
		return
	}
	actorID, err := parseOptionalActor(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a non-nil UUID when supplied", nil)
		return
	}

	execution, err := h.service.CreateBuild(r.Context(), goldapp.CreateBuildCommand{
		WorkspaceID:                      workspaceID,
		WorkflowVersionID:                workflowVersionID,
		OutputDatasetID:                  outputDatasetID,
		InputDatasetVersionID:            inputDatasetVersionID,
		InputCertificationID:             inputCertificationID,
		AnnotationCampaignID:             campaignID,
		AnnotationSnapshotID:             snapshotID,
		AnnotationContributionResourceID: contributionID,
		IdempotencyKey:                   idempotencyKey,
		ActorID:                          actorID,
		TraceID:                          httpserver.RequestID(r.Context()),
	})
	if err != nil {
		switch {
		case errors.Is(err, goldapp.ErrBuildConflict), errors.Is(err, workflowdomain.ErrIdempotencyConflict):
			httpserver.WriteError(w, r, http.StatusConflict, "GOLD_BUILD_IDEMPOTENCY_CONFLICT", "Idempotency-Key is already bound to a different Gold build", nil)
		case errors.Is(err, goldapp.ErrInvalidBuildRequest):
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_GOLD_BUILD_REQUEST", "Gold build request does not match frozen input/certification/snapshot facts", nil)
		default:
			httpserver.WriteError(w, r, http.StatusConflict, "GOLD_BUILD_REJECTED", "Gold build could not be queued", nil)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"executionId":           execution.ID,
		"status":                execution.Status,
		"workflowVersionId":     execution.WorkflowVersionID,
		"outputDatasetId":       execution.OutputDatasetID,
		"inputDatasetVersionId": inputDatasetVersionID,
		"annotationSnapshotId":  snapshotID,
	})
}

func parseUUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, errors.New("invalid UUID")
	}
	return id, nil
}

func parseOptionalActor(r *http.Request) (*uuid.UUID, error) {
	value := strings.TrimSpace(r.Header.Get("X-Actor-ID"))
	if value == "" {
		return nil, nil
	}
	id, err := parseUUID(value)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
