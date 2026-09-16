package producthttp

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func (h *Handler) RegisterValidation(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/product-releases/{releaseId}/validate", h.validateRelease)
}

type validateReleaseRequest struct {
	ContractVersionID  string `json:"contractVersionId"`
	RightsSnapshotID   string `json:"rightsSnapshotId"`
	QualityResultID    string `json:"qualityResultId"`
	ComplianceResultID string `json:"complianceResultId"`
}

func (h *Handler) validateRelease(w http.ResponseWriter, r *http.Request) {
	releaseID, ok := parsePathUUID(w, r, "releaseId", "INVALID_RELEASE_ID")
	if !ok {
		return
	}
	var req validateReleaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_JSON", "invalid JSON request", nil)
		return
	}
	contractVersionID, err := uuid.Parse(req.ContractVersionID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CONTRACT_VERSION_ID", "contractVersionId must be a UUID", nil)
		return
	}
	rightsSnapshotID, err := uuid.Parse(req.RightsSnapshotID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_RIGHTS_SNAPSHOT_ID", "rightsSnapshotId must be a UUID", nil)
		return
	}
	qualityResultID, err := uuid.Parse(req.QualityResultID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_QUALITY_RESULT_ID", "qualityResultId must be a UUID", nil)
		return
	}
	complianceResultID, err := uuid.Parse(req.ComplianceResultID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_COMPLIANCE_RESULT_ID", "complianceResultId must be a UUID", nil)
		return
	}
	actorID, err := parseActorID(r)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_ACTOR_ID", "X-Actor-ID must be a UUID", nil)
		return
	}

	result, err := h.service.ValidateRelease(r.Context(), application.ValidateReleaseCommand{
		ReleaseID:          releaseID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   rightsSnapshotID,
		QualityResultID:    qualityResultID,
		ComplianceResultID: complianceResultID,
		ActorID:            actorID,
		TraceID:            httpserver.RequestID(r.Context()),
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, infrastructure.ErrNotFound) {
			status = http.StatusNotFound
		}
		httpserver.WriteError(w, r, status, "PRODUCT_RELEASE_VALIDATION_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
