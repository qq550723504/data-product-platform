package traceabilityhttp

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

type EvidenceReader interface {
	ListForObject(ctx interface{ Done() <-chan struct{} }, objectType string, objectID uuid.UUID) ([]evidence.Item, error)
}

type CostReader interface {
	ListByExecution(ctx interface{ Done() <-chan struct{} }, executionID uuid.UUID) ([]cost.Item, error)
}

type Handler struct {
	evidenceRepo *evidence.QueryRepository
	costRepo     *cost.QueryRepository
}

func NewHandler(evidenceRepo *evidence.QueryRepository, costRepo *cost.QueryRepository) *Handler {
	return &Handler{evidenceRepo: evidenceRepo, costRepo: costRepo}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/traceability/{objectType}/{objectId}", h.getTraceability)
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
