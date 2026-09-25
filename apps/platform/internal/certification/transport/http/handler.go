package certificationhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

type Handler struct {
	certifications *application.CertificationService
	eligibility    *application.EligibilityService
	datasets       *datasetinfra.PostgresRepository
}

func NewHandler(certifications *application.CertificationService, eligibility *application.EligibilityService, datasets *datasetinfra.PostgresRepository) *Handler {
	return &Handler{certifications: certifications, eligibility: eligibility, datasets: datasets}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/certifications", h.history)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/delivery-eligibility", h.deliveryEligibility)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	workspaceID, versionID, ok := parseWorkspaceVersion(w, r)
	if !ok {
		return
	}
	if !h.requireVersionWorkspace(w, r, workspaceID, versionID) {
		return
	}
	asOf, ok := parseAsOf(w, r)
	if !ok {
		return
	}
	items, err := h.certifications.ListDatasetHistory(r.Context(), workspaceID, versionID, asOf)
	if err != nil {
		if errors.Is(err, certificationinfra.ErrProfileNotFound) || errors.Is(err, datasetinfra.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "CERTIFICATION_HISTORY_NOT_FOUND", "certification history was not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "CERTIFICATION_HISTORY_READ_FAILED", err.Error(), nil)
		return
	}
	response := make([]map[string]any, 0, len(items))
	for _, item := range items {
		response = append(response, historyItemResponse(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspaceId":      workspaceID,
		"datasetVersionId": versionID,
		"asOf":             asOf,
		"items":            response,
	})
}

func (h *Handler) deliveryEligibility(w http.ResponseWriter, r *http.Request) {
	workspaceID, versionID, ok := parseWorkspaceVersion(w, r)
	if !ok {
		return
	}
	if !h.requireVersionWorkspace(w, r, workspaceID, versionID) {
		return
	}
	profileID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("profileId")))
	if err != nil || profileID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CERTIFICATION_PROFILE_ID", "profileId must be a non-nil UUID", nil)
		return
	}
	// Current Delivery Eligibility is intentionally evaluated at server current
	// time. Historical asOf selection belongs to certification history reads and
	// must not allow callers to rewind revocation/expiry-sensitive current gates.
	result, err := h.eligibility.Check(r.Context(), application.DeliveryEligibilityQuery{
		WorkspaceID: workspaceID, DatasetVersionID: versionID, ProfileID: profileID,
		Consumer: r.URL.Query().Get("consumer"), Purpose: r.URL.Query().Get("purpose"),
		Action: r.URL.Query().Get("action"), Delivery: r.URL.Query().Get("delivery"),
		ScopeType: r.URL.Query().Get("scopeType"), ScopeRef: r.URL.Query().Get("scopeRef"), AsOf: time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, datasetinfra.ErrNotFound) || errors.Is(err, certificationinfra.ErrProfileNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "DELIVERY_ELIGIBILITY_TARGET_NOT_FOUND", "DatasetVersion or CertificationProfile was not found", nil)
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "scope") || strings.Contains(err.Error(), "workspace boundary") {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_DELIVERY_ELIGIBILITY_QUERY", err.Error(), nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DELIVERY_ELIGIBILITY_READ_FAILED", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, eligibilityResponse(result))
}

func (h *Handler) requireVersionWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, versionID uuid.UUID) bool {
	if h == nil || h.datasets == nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "CERTIFIED_DATASET_READ_NOT_CONFIGURED", "certified dataset read dependencies are incomplete", nil)
		return false
	}
	version, err := h.datasets.GetVersion(r.Context(), versionID)
	if err != nil {
		if errors.Is(err, datasetinfra.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_VERSION_NOT_FOUND", "DatasetVersion was not found", nil)
			return false
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASET_VERSION_READ_FAILED", err.Error(), nil)
		return false
	}
	actualWorkspace, _, err := h.datasets.GetWorkspaceAndType(r.Context(), version.DatasetID)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "DATASET_WORKSPACE_READ_FAILED", err.Error(), nil)
		return false
	}
	if actualWorkspace != workspaceID {
		httpserver.WriteError(w, r, http.StatusNotFound, "DATASET_VERSION_NOT_FOUND", "DatasetVersion was not found in workspace", nil)
		return false
	}
	return true
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

func parseAsOf(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("asOf"))
	if raw == "" {
		return time.Now().UTC(), true
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_AS_OF", "asOf must be RFC3339", nil)
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func historyItemResponse(item application.CertificationHistoryItem) map[string]any {
	certification := item.Certification
	dispositions := make([]map[string]any, 0, len(item.Dispositions))
	for _, disposition := range item.Dispositions {
		dispositions = append(dispositions, map[string]any{
			"id": disposition.ID, "disposition": disposition.Disposition, "effectiveAt": disposition.EffectiveAt,
			"reason": disposition.Reason, "supersededByCertificationId": disposition.SupersededByCertificationID,
			"evidenceSnapshotId": disposition.EvidenceSnapshotID,
		})
	}
	return map[string]any{
		"id": certification.ID, "workspaceId": certification.WorkspaceID, "datasetVersionId": certification.DatasetVersionID,
		"qualityAssessmentId": certification.QualityAssessmentID, "decision": certification.Decision,
		"blockers": certification.Blockers, "reason": certification.Reason, "issuedAt": certification.IssuedAt,
		"rightsSnapshotId": certification.RightsSnapshotID, "effectiveRightsSnapshotId": certification.EffectiveRightsSnapshotID,
		"effectiveRightsSnapshotHash": certification.EffectiveRightsSnapshotHash, "frozenRightsContextHash": certification.FrozenRightsContextHash,
		"complianceResultId": certification.ComplianceResultID, "contractVersionId": certification.ContractVersionID,
		"traceabilityEvidenceId": certification.TraceabilityEvidenceID, "evidenceSnapshotId": certification.EvidenceSnapshotID,
		"goldProductionBindingId":       certification.GoldProductionBindingID,
		"annotationSnapshotId":          certification.AnnotationSnapshotID,
		"annotationSnapshotRootHash":    certification.AnnotationSnapshotRootHash,
		"annotationSchemaSha256":        certification.AnnotationSchemaSHA256,
		"annotationTaxonomySha256":      certification.AnnotationTaxonomySHA256,
		"goldProductionBindingRootHash": certification.GoldProductionBindingRootHash,
		"profile":                       profileResponse(certification.Profile),
		"dispositions":                  dispositions,
	}
}

func profileResponse(profile domain.ProfileSnapshot) map[string]any {
	return map[string]any{
		"id": profile.ID, "workspaceId": profile.WorkspaceID, "profileRef": profile.ProfileRef,
		"code": profile.Code, "name": profile.Name, "version": profile.Version, "contentSha256": profile.ContentSHA256,
		"purpose": profile.Purpose, "actions": profile.Actions, "consumers": profile.Consumers, "delivery": profile.Delivery,
		"requiredQualityDimensions": profile.RequiredQualityDimensions, "requiredCriticalRules": profile.RequiredCriticalRules,
		"qualityGateRequired": profile.QualityGateRequired, "rights": profile.Rights,
		"complianceRequired": profile.ComplianceRequired, "contractRequired": profile.ContractRequired,
		"contractCode": profile.ContractCode, "traceabilityRequired": profile.TraceabilityRequired,
		"evidenceRequired": profile.EvidenceRequired,
	}
}

func eligibilityResponse(result application.DeliveryEligibilityResult) map[string]any {
	checks := make([]map[string]any, 0, len(result.EntitlementChecks))
	for _, check := range result.EntitlementChecks {
		checks = append(checks, map[string]any{
			"dataResourceId": check.DataResourceID, "path": check.Path,
			"decision": check.Decision.Decision, "reason": check.Decision.Reason,
			"authorizationId": check.Decision.AuthorizationID, "declarationId": check.Decision.DeclarationID,
			"bindingId": check.Decision.BindingID,
		})
	}
	var certification any
	if result.Certification.ID != uuid.Nil {
		certification = historyItemResponse(application.CertificationHistoryItem{Certification: result.Certification})
	}
	return map[string]any{
		"allowed":  result.Allowed,
		"blockers": result.Blockers,
		"datasetVersion": map[string]any{
			"status":   result.DatasetVersionStatus,
			"allowed":  result.DatasetVersionGate.Allowed,
			"blockers": result.DatasetVersionGate.Blockers,
		},
		"certification": map[string]any{
			"allowed":  result.CertificationGate.Allowed,
			"blockers": result.CertificationGate.Blockers,
			"current":  certification,
		},
		"entitlement": map[string]any{
			"allowed":  result.EntitlementGate.Allowed,
			"blockers": result.EntitlementGate.Blockers,
			"checks":   checks,
		},
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
