package traceabilityhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
)

type Handler struct {
	evidenceRepo *evidence.QueryRepository
	costRepo     *cost.QueryRepository
	releaseRepo  *traceability.Repository
}

func NewHandler(evidenceRepo *evidence.QueryRepository, costRepo *cost.QueryRepository, releaseRepo ...*traceability.Repository) *Handler {
	handler := &Handler{evidenceRepo: evidenceRepo, costRepo: costRepo}
	if len(releaseRepo) > 0 {
		handler.releaseRepo = releaseRepo[0]
	}
	return handler
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/traceability/{objectType}/{objectId}", h.getTraceability)
	if h.releaseRepo != nil {
		mux.HandleFunc("GET /api/v1/traceability/product-releases/{releaseId}", h.getProductReleaseTraceability)
	}
}

type response struct {
	ObjectType string          `json:"objectType"`
	ObjectID   uuid.UUID       `json:"objectId"`
	Evidence   []evidence.Item `json:"evidence"`
	CostEvents []cost.Item     `json:"costEvents"`
}

func (h *Handler) getTraceability(w http.ResponseWriter, r *http.Request) {
	objectType := strings.ToUpper(strings.TrimSpace(r.PathValue("objectType")))
	if objectType == "" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OBJECT_TYPE", "objectType is required", nil)
		return
	}
	objectID, err := uuid.Parse(r.PathValue("objectId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OBJECT_ID", "objectId must be a UUID", nil)
		return
	}

	evidenceItems, err := h.evidenceRepo.ListForObject(r.Context(), objectType, objectID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "EVIDENCE_QUERY_FAILED", err.Error(), nil)
		return
	}
	costItems := make([]cost.Item, 0)
	if objectType == "EXECUTION" {
		costItems, err = h.costRepo.ListByExecution(r.Context(), objectID)
		if err != nil {
			httpserver.WriteError(w, r, http.StatusInternalServerError, "COST_QUERY_FAILED", err.Error(), nil)
			return
		}
	}

	writeJSON(w, http.StatusOK, response{
		ObjectType: objectType,
		ObjectID:   objectID,
		Evidence:   evidenceItems,
		CostEvents: costItems,
	})
}

func (h *Handler) getProductReleaseTraceability(w http.ResponseWriter, r *http.Request) {
	releaseID, err := uuid.Parse(r.PathValue("releaseId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_RELEASE_ID", "releaseId must be a UUID", nil)
		return
	}
	result, err := h.releaseRepo.ProductRelease(r.Context(), releaseID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "PRODUCT_RELEASE_TRACEABILITY_FAILED"
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusNotFound
			code = "PRODUCT_RELEASE_NOT_FOUND"
		}
		httpserver.WriteError(w, r, status, code, err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
