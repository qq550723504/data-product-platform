package producthttp

import (
	"errors"
	"net/http"
	"strings"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func (h *Handler) RegisterPublish(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/product-releases/{releaseId}/publish", h.publishRelease)
}

func (h *Handler) publishRelease(w http.ResponseWriter, r *http.Request) {
	releaseID, ok := parsePathUUID(w, r, "releaseId", "INVALID_RELEASE_ID")
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}

	release, err := h.service.PublishRelease(r.Context(), application.PublishReleaseCommand{
		ReleaseID:      releaseID,
		IdempotencyKey: idempotencyKey,
		ActorID:        actorID,
		TraceID:        httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusBadRequest
		code := "PRODUCT_RELEASE_PUBLISH_FAILED"
		switch {
		case errors.Is(err, infrastructure.ErrNotFound):
			status = http.StatusNotFound
			code = "PRODUCT_RELEASE_NOT_FOUND"
		case errors.Is(err, domain.ErrReleaseNotReady):
			status = http.StatusConflict
			code = "PRODUCT_RELEASE_NOT_READY"
		case errors.Is(err, domain.ErrIdempotencyConflict):
			status = http.StatusConflict
			code = "IDEMPOTENCY_KEY_CONFLICT"
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, releaseResponse(release))
}
