package certificationhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
)

type Handler struct {
	Principal      principal.Resolver
	certifications *application.CertificationService
	eligibility    *application.EligibilityService
	datasets       *datasetinfra.PostgresRepository
	Authorize      func(*http.Request, uuid.UUID) (*uuid.UUID, error)
	Rights         *rightsapp.Service
}

func NewHandler(certifications *application.CertificationService, eligibility *application.EligibilityService, datasets *datasetinfra.PostgresRepository) *Handler {
	return &Handler{certifications: certifications, eligibility: eligibility, datasets: datasets}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/certification-profiles", h.profiles)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/certification-inputs", h.inputs)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/certification-inputs", h.prepareInputs)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/certifications/{certificationId}/supersede", h.supersede)
	h.RegisterHistoryReadOnly(mux)
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/delivery-eligibility", h.deliveryEligibility)
	mux.HandleFunc("POST /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/certifications", h.evaluate)
}

func (h *Handler) supersede(w http.ResponseWriter, r *http.Request) {
	ws, e1 := uuid.Parse(r.PathValue("workspaceId"))
	id, e2 := uuid.Parse(r.PathValue("certificationId"))
	if e1 != nil || e2 != nil {
		httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "workspace and certification required", nil)
		return
	}
	if h.Authorize == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted principal required", nil)
		return
	}
	actor, err := h.Authorize(r, ws)
	if err != nil || actor == nil {
		httpserver.WriteError(w, r, 403, "AUTHORITY_DENIED", "workspace authority denied", nil)
		return
	}
	var body struct {
		ReplacementID uuid.UUID `json:"replacementId"`
		Reason        string    `json:"reason"`
		EffectiveAt   time.Time `json:"effectiveAt"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF || body.ReplacementID == uuid.Nil || body.ReplacementID == id || body.EffectiveAt.IsZero() || strings.TrimSpace(body.Reason) == "" || len(body.Reason) > 4096 {
		httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "distinct replacement and reason required", nil)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	activity := cost.CertificationActivity{ActivityID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("certification-supersede:"+ws.String()+":"+key)), Quantity: 1, Unit: "DECISION", PricingMode: "ACTUAL_QUANTITY"}
	v, err := h.certifications.ChangeDisposition(r.Context(), application.ChangeCertificationDispositionCommand{WorkspaceID: ws, CertificationID: id, Disposition: domain.DispositionSuperseded, SupersededByCertificationID: &body.ReplacementID, EffectiveAt: body.EffectiveAt, Reason: strings.TrimSpace(body.Reason), ActorID: actor, IdempotencyKey: key, TraceID: httpserver.RequestID(r.Context()), CostActivity: &activity})
	if err != nil {
		httpserver.WriteError(w, r, 409, "CERTIFICATION_SUPERSEDE_REJECTED", "replacement must be a same-target profile fact", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"id": v.ID, "disposition": v.Disposition})
}

func (h *Handler) inputsAuthority(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, *uuid.UUID, bool) {
	ws, v, ok := parseWorkspaceVersion(w, r)
	if !ok {
		return ws, v, nil, false
	}
	if h.Authorize == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted principal required", nil)
		return ws, v, nil, false
	}
	actor, err := h.Authorize(r, ws)
	if err != nil || actor == nil {
		httpserver.WriteError(w, r, 403, "AUTHORITY_DENIED", "workspace authority denied", nil)
		return ws, v, nil, false
	}
	if !h.requireVersionWorkspace(w, r, ws, v) {
		return ws, v, nil, false
	}
	return ws, v, actor, true
}
func (h *Handler) inputs(w http.ResponseWriter, r *http.Request) {
	ws, v, _, ok := h.inputsAuthority(w, r)
	if !ok {
		return
	}
	p, e1 := uuid.Parse(r.URL.Query().Get("profileId"))
	q, e2 := uuid.Parse(r.URL.Query().Get("qualityAssessmentId"))
	if e1 != nil || e2 != nil {
		httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "profile and quality references required", nil)
		return
	}
	refs, err := h.certifications.PreparedInputs(r.Context(), ws, v, p, q)
	if err != nil {
		httpserver.WriteError(w, r, 500, "INPUTS_READ_FAILED", "prepared evidence unavailable", nil)
		return
	}
	writeJSON(w, 200, refs)
}
func (h *Handler) prepareInputs(w http.ResponseWriter, r *http.Request) {
	ws, v, actor, ok := h.inputsAuthority(w, r)
	if !ok {
		return
	}
	var body struct {
		ProfileID           uuid.UUID `json:"profileId"`
		QualityAssessmentID uuid.UUID `json:"qualityAssessmentId"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF || body.ProfileID == uuid.Nil || body.QualityAssessmentID == uuid.Nil || h.Rights == nil {
		httpserver.WriteError(w, r, 400, "INVALID_REFERENCE", "profile and quality references required", nil)
		return
	}
	refs, err := h.certifications.PrepareInputs(r.Context(), h.Rights, application.PrepareCertificationInputsCommand{WorkspaceID: ws, DatasetVersionID: v, ProfileID: body.ProfileID, QualityAssessmentID: body.QualityAssessmentID, ActorID: actor, IdempotencyKey: r.Header.Get("Idempotency-Key"), TraceID: httpserver.RequestID(r.Context())})
	if err != nil {
		httpserver.WriteError(w, r, 409, "CERTIFICATION_INPUTS_NOT_PREPARED", "requires completed production, quality, one compliance result and one published contract; refresh to recover the same request", nil)
		return
	}
	writeJSON(w, 200, refs)
}

func (h *Handler) profiles(w http.ResponseWriter, r *http.Request) {
	ws, err := uuid.Parse(r.PathValue("workspaceId"))
	if err != nil || ws == uuid.Nil {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE_ID", "workspace required", nil)
		return
	}
	if h.Authorize == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted principal required", nil)
		return
	}
	if actor, err := h.Authorize(r, ws); err != nil || actor == nil {
		httpserver.WriteError(w, r, 403, "AUTHORITY_DENIED", "workspace authority denied", nil)
		return
	}
	profiles, err := h.certifications.ListProfiles(r.Context(), ws)
	if err != nil {
		httpserver.WriteError(w, r, 500, "PROFILES_READ_FAILED", "certification profiles unavailable", nil)
		return
	}
	items := make([]map[string]any, 0, len(profiles))
	for _, p := range profiles {
		items = append(items, profileResponse(p))
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

// References identify immutable facts. The application rebinds their contents
// from PostgreSQL; no caller-supplied PASS/rights decision is accepted.
func (h *Handler) evaluate(w http.ResponseWriter, r *http.Request) {
	ws, version, ok := parseWorkspaceVersion(w, r)
	if !ok {
		return
	}
	if h.Authorize == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "certification command requires trusted principal", nil)
		return
	}
	actor, err := h.Authorize(r, ws)
	if err != nil || actor == nil {
		httpserver.WriteError(w, r, 403, "AUTHORITY_DENIED", "certification authority denied", nil)
		return
	}
	if !h.requireVersionWorkspace(w, r, ws, version) {
		return
	}
	var body struct {
		ProfileID                 uuid.UUID  `json:"profileId"`
		QualityAssessmentID       uuid.UUID  `json:"qualityAssessmentId"`
		RightsSnapshotID          *uuid.UUID `json:"rightsSnapshotId"`
		EffectiveRightsSnapshotID *uuid.UUID `json:"effectiveRightsSnapshotId"`
		ComplianceResultID        *uuid.UUID `json:"complianceResultId"`
		ContractVersionID         *uuid.UUID `json:"contractVersionId"`
		TraceabilityEvidenceID    *uuid.UUID `json:"traceabilityEvidenceId"`
		EvidenceSnapshotID        *uuid.UUID `json:"evidenceSnapshotId"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&body) != nil || dec.Decode(new(any)) != io.EOF {
		httpserver.WriteError(w, r, 400, "INVALID_JSON", "invalid certification references", nil)
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	activity := cost.CertificationActivity{WorkspaceID: ws, ActivityID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("certification-http:"+ws.String()+":"+version.String()+":"+key)), Quantity: 1, Unit: "EVALUATION", PricingMode: "ACTUAL_QUANTITY"}
	v, err := h.certifications.Evaluate(r.Context(), application.EvaluateDatasetCertificationCommand{WorkspaceID: ws, DatasetVersionID: version, ProfileID: body.ProfileID, QualityAssessmentID: body.QualityAssessmentID, RightsSnapshotID: body.RightsSnapshotID, EffectiveRightsSnapshotID: body.EffectiveRightsSnapshotID, ComplianceResultID: body.ComplianceResultID, ContractVersionID: body.ContractVersionID, TraceabilityEvidenceID: body.TraceabilityEvidenceID, EvidenceSnapshotID: body.EvidenceSnapshotID, IdempotencyKey: key, ActorID: actor, TraceID: httpserver.RequestID(r.Context()), Now: time.Now().UTC(), CostActivity: &activity})
	if err != nil {
		status := 400
		if errors.Is(err, application.ErrCertificationIdempotencyConflict) {
			status = 409
		}
		httpserver.WriteError(w, r, status, "CERTIFICATION_REJECTED", "certification references or command rejected", nil)
		return
	}
	writeJSON(w, 200, map[string]any{"id": v.ID, "decision": v.Decision, "blockers": v.Blockers})
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	workspaceID, versionID, ok := parseWorkspaceVersion(w, r)
	if !ok {
		return
	}
	if h.Principal == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted workspace principal required", nil)
		return
	}
	actor, authErr := h.Principal.Resolve(r, workspaceID, principal.CapabilityHumanDecision)
	if errors.Is(authErr, principal.ErrUnauthenticated) || errors.Is(authErr, principal.ErrNotConfigured) {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted workspace principal required", nil)
		return
	}
	if authErr != nil || actor.ActorID == uuid.Nil || actor.WorkspaceID != workspaceID || !actor.HasCapability(principal.CapabilityHumanDecision) {
		httpserver.WriteError(w, r, 403, "WORKSPACE_AUTHORITY_DENIED", "workspace authority denied", nil)
		return
	}
	if !h.requireVersionWorkspace(w, r, workspaceID, versionID) {
		return
	}
	asOf, ok := parseAsOf(w, r)
	if !ok {
		return
	}
	limit, offset, anchorRevision, ok := parseHistoryPage(w, r)
	if !ok {
		return
	}
	selectedProfileID, ok := parseOptionalHistoryProfile(w, r)
	if !ok {
		return
	}
	page, err := h.certifications.ListDatasetHistoryPage(r.Context(), workspaceID, versionID, asOf, limit, offset, anchorRevision, selectedProfileID)
	if err != nil {
		if errors.Is(err, certificationinfra.ErrProfileNotFound) || errors.Is(err, datasetinfra.ErrNotFound) {
			httpserver.WriteError(w, r, http.StatusNotFound, "CERTIFICATION_HISTORY_NOT_FOUND", "certification history was not found", nil)
			return
		}
		httpserver.WriteError(w, r, http.StatusInternalServerError, "CERTIFICATION_HISTORY_READ_FAILED", err.Error(), nil)
		return
	}
	response := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		response = append(response, historyItemResponse(item))
	}
	profiles := make([]map[string]any, 0, len(page.Profiles))
	for _, profile := range page.Profiles {
		profiles = append(profiles, profileResponse(profile))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspaceId":      workspaceID,
		"datasetVersionId": versionID,
		"asOf":             asOf,
		"items":            response,
		"profiles":         profiles,
		"page": map[string]any{
			"limit": page.Limit, "offset": page.Offset, "total": page.Total, "anchorRevision": page.AnchorRevision,
		},
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

func parseHistoryPage(w http.ResponseWriter, r *http.Request) (int, int, *int64, bool) {
	const (
		defaultLimit = 25
		maxLimit     = 100
	)
	limit := defaultLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > maxLimit {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CERTIFICATION_HISTORY_LIMIT", "limit must be an integer between 1 and 100", nil)
			return 0, 0, nil, false
		}
		limit = value
	}
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CERTIFICATION_HISTORY_OFFSET", "offset must be a non-negative integer", nil)
			return 0, 0, nil, false
		}
		offset = value
	}
	var anchorRevision *int64
	if raw := strings.TrimSpace(r.URL.Query().Get("anchorRevision")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 {
			httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CERTIFICATION_HISTORY_ANCHOR", "anchorRevision must be a non-negative integer", nil)
			return 0, 0, nil, false
		}
		anchorRevision = &value
	}
	return limit, offset, anchorRevision, true
}

func parseOptionalHistoryProfile(w http.ResponseWriter, r *http.Request) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("profileId"))
	if raw == "" {
		return nil, true
	}
	profileID, err := uuid.Parse(raw)
	if err != nil || profileID == uuid.Nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_CERTIFICATION_PROFILE_ID", "profileId must be a non-nil UUID", nil)
		return nil, false
	}
	return &profileID, true
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

// RegisterHistoryReadOnly exposes only immutable certification history queries.
func (h *Handler) RegisterHistoryReadOnly(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/dataset-versions/{versionId}/certifications", h.history)
}
