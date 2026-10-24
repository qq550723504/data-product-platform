package traceabilityhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
)

type Handler struct {
	Principal   principal.Resolver
	releaseRepo *traceability.Repository
}

func NewHandler(repo *traceability.Repository) *Handler { return &Handler{releaseRepo: repo} }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/traceability/{objectType}/{objectId}", h.getTraceability)
	if h.releaseRepo != nil {
		mux.HandleFunc("GET /api/v1/traceability/product-releases/{releaseId}", h.getProductReleaseTraceability)
	}
}

type response struct {
	WorkspaceID uuid.UUID       `json:"workspaceId"`
	ObjectType  string          `json:"objectType"`
	ObjectID    uuid.UUID       `json:"objectId"`
	Evidence    []evidence.Item `json:"evidence"`
	CostEvents  []cost.Item     `json:"costEvents"`
}

func (h *Handler) getTraceability(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	workspaceID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	objectType := strings.ToUpper(strings.TrimSpace(r.PathValue("objectType")))
	if objectType != "EXECUTION" {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OBJECT_TYPE", "only EXECUTION object traces are supported", nil)
		return
	}
	objectID, err := uuid.Parse(r.PathValue("objectId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_OBJECT_ID", "objectId must be a UUID", nil)
		return
	}

	if h.releaseRepo == nil {
		httpserver.WriteError(w, r, 503, "TRACE_UNAVAILABLE", "trace ownership reader unavailable", nil)
		return
	}
	evidenceItems, costItems, err := h.releaseRepo.ExecutionInWorkspace(r.Context(), workspaceID, objectID)
	if err != nil {
		status := 500
		code := "TRACE_QUERY_FAILED"
		if errors.Is(err, pgx.ErrNoRows) {
			status = 404
			code = "OBJECT_NOT_FOUND"
		}
		if errors.Is(err, traceability.ErrScope) {
			status = 409
			code = "TRACE_SCOPE_INVALID"
		}
		httpserver.WriteError(w, r, status, code, "trace read unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, response{
		WorkspaceID: workspaceID,
		ObjectType:  objectType,
		ObjectID:    objectID,
		Evidence:    evidenceItems,
		CostEvents:  costItems,
	})
}

func (h *Handler) getProductReleaseTraceability(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	workspaceID, ok := h.authorize(w, r)
	if !ok {
		return
	}
	releaseID, err := uuid.Parse(r.PathValue("releaseId"))
	if err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, "INVALID_RELEASE_ID", "releaseId must be a UUID", nil)
		return
	}
	result, err := h.releaseRepo.ProductReleaseInWorkspace(r.Context(), workspaceID, releaseID)
	if err != nil {
		status := http.StatusInternalServerError
		code := "PRODUCT_RELEASE_TRACEABILITY_FAILED"
		if errors.Is(err, traceability.ErrScope) {
			status = http.StatusConflict
			code = "TRACE_SCOPE_INVALID"
		}
		if errors.Is(err, pgx.ErrNoRows) {
			status = http.StatusNotFound
			code = "PRODUCT_RELEASE_NOT_FOUND"
		}
		httpserver.WriteError(w, r, status, code, "release trace unavailable", nil)
		return
	}
	writeJSON(w, http.StatusOK, safeTrace(result, workspaceID))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(redactTrace(body))
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	w.Header().Set("Cache-Control", "private, no-store")
	values := r.URL.Query()["workspaceId"]
	if len(values) != 1 {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE_ID", "exactly one workspaceId is required", nil)
		return uuid.Nil, false
	}
	ws, err := uuid.Parse(r.URL.Query().Get("workspaceId"))
	if err != nil || ws == uuid.Nil {
		httpserver.WriteError(w, r, 400, "INVALID_WORKSPACE_ID", "workspaceId is required", nil)
		return uuid.Nil, false
	}
	if h.Principal == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted principal required", nil)
		return uuid.Nil, false
	}
	actor, err := h.Principal.Resolve(r, ws, principal.CapabilityHumanDecision)
	if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted principal required", nil)
		return uuid.Nil, false
	}
	if err != nil || actor.ActorID == uuid.Nil || actor.WorkspaceID != ws || !actor.HasCapability(principal.CapabilityHumanDecision) {
		httpserver.WriteError(w, r, 403, "WORKSPACE_AUTHORITY_DENIED", "workspace authority denied", nil)
		return uuid.Nil, false
	}
	return ws, true
}

// Only historical identity, status and integrity fields leave this read endpoint.
// Free-form metadata, manifests, storage locations and source keys are excluded.
func redactTrace(value any) any {
	raw, _ := json.Marshal(value)
	var data any
	_ = json.Unmarshal(raw, &data)
	allowed := map[string]bool{}
	for _, key := range strings.Fields("workspaceId objectType objectId releaseId releaseNo status productId productVersionId evidenceSnapshot datasetVersions executions entityMatchJobs entityMappings evidence costEvents auditEvents id rootHash integrityValid items category createdAt datasetId datasetCode datasetType versionNo releaseRole generatedByExecutionId workflowVersionId workflowVersion workflowDefinitionHash attempt engineType outputDatasetVersionId dependencyPreparationStatus decisionId entityId evidenceId sourceOrigin sourceJobId decidedAt evidenceType hashAlgorithm hashValue relationType costType quantity unit amount currency pricingMode occurredAt action actorType") {
		allowed[key] = true
	}
	var clean func(any) any
	clean = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, v := range x {
				if allowed[k] {
					out[k] = clean(v)
				}
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, v := range x {
				out[i] = clean(v)
			}
			return out
		default:
			return v
		}
	}
	return clean(data)
}
func safeTrace(t traceability.ReleaseTrace, ws uuid.UUID) any {
	return struct {
		WorkspaceID uuid.UUID `json:"workspaceId"`
		traceability.ReleaseTrace
	}{ws, t}
}
