package deliveryhttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliverydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

type DirectDataCommandService interface {
	Deliver(rctx context.Context, command deliveryapp.DirectDataCommand) (deliveryapp.DirectDataResult, error)
}

type ObjectStore interface {
	Get(context.Context, string) (io.ReadCloser, error)
}

type Handler struct {
	service  DirectDataCommandService
	resolver PrincipalResolver
	store    ObjectStore
}

func NewHandler(service DirectDataCommandService, resolver PrincipalResolver, store ObjectStore) *Handler {
	return &Handler{service: service, resolver: resolver, store: store}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/deliveries", h.deliver)
}

type deliverRequest struct {
	ProfileID string `json:"profileId"`
	Consumer  string `json:"consumer"`
	Purpose   string `json:"purpose"`
	Action    string `json:"action"`
	ScopeType string `json:"scopeType"`
	ScopeRef  string `json:"scopeRef,omitempty"`
}

func (h *Handler) deliver(w http.ResponseWriter, r *http.Request) {
	workspaceID, versionID, ok := parseWorkspaceVersion(w, r)
	if !ok {
		return
	}
	if h == nil || h.service == nil || h.resolver == nil || h.store == nil {
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, "DIRECT_DATA_DELIVERY_NOT_CONFIGURED", "direct data delivery is not configured", nil)
		return
	}

	var request deliverRequest
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DELIVERY_REQUEST", "delivery request must be valid JSON with only supported fields", nil)
		return
	}
	profileID, err := uuid.Parse(strings.TrimSpace(request.ProfileID))
	if err != nil || profileID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CERTIFICATION_PROFILE_ID", "profileId must be a non-nil UUID", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 255 {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is required and must be at most 255 characters", nil)
		return
	}

	caller, err := h.resolver.Resolve(r, workspaceID, request.Consumer)
	if err != nil {
		switch {
		case errors.Is(err, ErrDeliveryNotConfigured):
			httpserver.WriteError(w, r, http.StatusServiceUnavailable, "DIRECT_DATA_DELIVERY_NOT_CONFIGURED", "direct data delivery trusted caller boundary is not configured", nil)
		case errors.Is(err, ErrCallerIdentityUntrusted):
			httpserver.WriteError(w, r, http.StatusUnauthorized, "CALLER_IDENTITY_UNTRUSTED", "a trusted authenticated caller is required", nil)
		case errors.Is(err, ErrCallerWorkspaceDenied):
			httpserver.WriteError(w, r, http.StatusForbidden, "CALLER_WORKSPACE_ACCESS_DENIED", "the authenticated caller is not authorized for this workspace", nil)
		case errors.Is(err, ErrConsumerPrincipalMismatch):
			httpserver.WriteError(w, r, http.StatusForbidden, "CONSUMER_PRINCIPAL_MISMATCH", "requested consumer is not represented by the authenticated caller", nil)
		default:
			httpserver.WriteError(w, r, http.StatusInternalServerError, "CALLER_RESOLUTION_FAILED", "trusted caller resolution failed", nil)
		}
		return
	}

	result, err := h.service.Deliver(r.Context(), deliveryapp.DirectDataCommand{
		WorkspaceID: workspaceID, DatasetVersionID: versionID, ProfileID: profileID,
		PrincipalRef: caller.PrincipalRef, EffectiveConsumerRef: caller.EffectiveConsumerRef,
		Purpose: request.Purpose, Action: request.Action, ScopeType: request.ScopeType, ScopeRef: request.ScopeRef,
		IdempotencyKey: idempotencyKey, TraceID: strings.TrimSpace(r.Header.Get("X-Trace-ID")),
	})
	if errors.Is(err, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"code":        "DIRECT_DATA_REPLAY_REQUIRES_NEW_ATTEMPT",
			"operationId": result.Operation.ID, "status": result.Operation.Status,
			"message": "the original direct-data authorization was already linearized; use a new idempotency key for another delivery attempt",
		})
		return
	}
	if errors.Is(err, deliveryapp.ErrDeliveryIdempotencyConflict) {
		httpserver.WriteError(w, r, http.StatusConflict, "DELIVERY_IDEMPOTENCY_CONFLICT", "Idempotency-Key is already bound to a different delivery request", nil)
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, deliverydomain.ErrInvalidOperation):
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DELIVERY_REQUEST", err.Error(), nil)
		case errors.Is(err, datasetinfra.ErrNotFound), errors.Is(err, certificationinfra.ErrProfileNotFound):
			httpserver.WriteError(w, r, http.StatusNotFound, "DELIVERY_TARGET_NOT_FOUND", "DatasetVersion or CertificationProfile was not found", nil)
		default:
			httpserver.WriteError(w, r, http.StatusInternalServerError, "DIRECT_DATA_DELIVERY_FAILED", "direct data delivery command failed", nil)
		}
		return
	}
	if result.Operation.Status == deliverydomain.StatusBlocked || !result.PayloadReady {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"operationId": result.Operation.ID, "status": result.Operation.Status,
			"allowed": false, "blockers": result.Blockers,
		})
		return
	}

	object, err := h.store.Get(r.Context(), result.DatasetVersion.StorageURI)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DIRECT_DATA_OBJECT_READ_FAILED", "delivery was authorized but the immutable object could not be opened; retry requires a new delivery attempt", map[string]any{"operationId": result.Operation.ID})
		return
	}
	defer object.Close()

	w.Header().Set("X-Delivery-Operation-Id", result.Operation.ID.String())
	contentType := strings.TrimSpace(result.DatasetVersion.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if result.DatasetVersion.ByteSize != nil && *result.DatasetVersion.ByteSize >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(*result.DatasetVersion.ByteSize, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, object)
}

func parseWorkspaceVersion(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	workspaceID, err := uuid.Parse(r.PathValue("workspaceId"))
	if err != nil || workspaceID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_WORKSPACE_ID", "workspaceId must be a non-nil UUID", nil)
		return uuid.Nil, uuid.Nil, false
	}
	versionID, err := uuid.Parse(r.PathValue("versionId"))
	if err != nil || versionID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DATASET_VERSION_ID", "versionId must be a non-nil UUID", nil)
		return uuid.Nil, uuid.Nil, false
	}
	return workspaceID, versionID, true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
