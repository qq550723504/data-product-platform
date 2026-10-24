package qualityhttp

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

// All eight Quality routes share the same trust boundary as remediation.
// Request workspace/actor fields never establish a principal.
func (h *Handler) authorizeWorkspace(w http.ResponseWriter, r *http.Request, ws uuid.UUID) (*uuid.UUID, bool) {
	if h.Principal == nil {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted Quality principal required", nil)
		return nil, false
	}
	p, err := h.Principal.Resolve(r, ws, principal.CapabilityHumanDecision)
	if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
		httpserver.WriteError(w, r, 401, "AUTHENTICATION_REQUIRED", "trusted Quality principal required", nil)
		return nil, false
	}
	if err != nil || p.ActorID == uuid.Nil || p.WorkspaceID != ws || !p.HasCapability(principal.CapabilityHumanDecision) {
		httpserver.WriteError(w, r, 403, "QUALITY_AUTHORITY_DENIED", "Quality workspace authority denied", nil)
		return nil, false
	}
	return &p.ActorID, true
}

func (h *Handler) authorizedQuery(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	ws, ok := requiredWorkspaceQuery(w, r)
	if !ok {
		return uuid.Nil, false
	}
	_, ok = h.authorizeWorkspace(w, r, ws)
	return ws, ok
}

func (h *Handler) versionInWorkspace(w http.ResponseWriter, r *http.Request, ws, version uuid.UUID) bool {
	ok, err := h.repo.DatasetVersionBelongsToWorkspace(r.Context(), ws, version)
	return scopeResult(w, r, ok, err)
}

func (h *Handler) objectInWorkspace(w http.ResponseWriter, r *http.Request, ws, id uuid.UUID, attempt bool) bool {
	var ok bool
	var err error
	if attempt {
		ok, err = h.repo.AssessmentAttemptBelongsToWorkspace(r.Context(), ws, id)
	} else {
		ok, err = h.repo.AssessmentBelongsToWorkspace(r.Context(), ws, id)
	}
	return scopeResult(w, r, ok, err)
}

func scopeResult(w http.ResponseWriter, r *http.Request, ok bool, err error) bool {
	if err != nil {
		httpserver.WriteError(w, r, 503, "QUALITY_SCOPE_UNAVAILABLE", "Quality scope could not be verified", nil)
		return false
	}
	if !ok {
		httpserver.WriteError(w, r, 404, "QUALITY_OBJECT_NOT_FOUND", "Quality object not found in workspace", nil)
		return false
	}
	return true
}
