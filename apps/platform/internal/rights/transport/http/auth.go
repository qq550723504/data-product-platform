package rightshttp

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

var (
	ErrRightsAPIUnauthenticated = errors.New("rights API authentication required")
	ErrRightsAPIForbidden       = errors.New("rights API workspace access denied")
)

// Authorizer is the trust boundary for rights HTTP commands. The request must
// carry an authenticated credential and the resulting principal must be
// allowed to act in the command workspace.
type Authorizer interface {
	Authorize(r *http.Request, workspaceID uuid.UUID) (*uuid.UUID, error)
}

type staticAuthorizer struct {
	token      string
	actorID    uuid.UUID
	workspaces map[uuid.UUID]struct{}
}

// NewStaticAuthorizer provides the minimal control-plane boundary for the
// current deployment. The token is supplied by the trusted runtime, never by
// the client actor header; the configured actor and workspace allowlist are
// also server-side facts. Full enterprise IAM remains a separate integration.
func NewStaticAuthorizer(token, actorID string, workspaceIDs []string) (Authorizer, error) {
	token = strings.TrimSpace(token)
	actorID = strings.TrimSpace(actorID)
	if token == "" || actorID == "" || len(workspaceIDs) == 0 {
		return staticAuthorizer{}, nil
	}
	actor, err := uuid.Parse(actorID)
	if err != nil || actor == uuid.Nil {
		return nil, errors.New("RIGHTS_API_ACTOR_ID must be a non-nil UUID")
	}
	workspaces := make(map[uuid.UUID]struct{}, len(workspaceIDs))
	for _, value := range workspaceIDs {
		workspace, parseErr := uuid.Parse(strings.TrimSpace(value))
		if parseErr != nil || workspace == uuid.Nil {
			return nil, errors.New("RIGHTS_API_WORKSPACE_IDS must contain UUIDs")
		}
		workspaces[workspace] = struct{}{}
	}
	return staticAuthorizer{token: token, actorID: actor, workspaces: workspaces}, nil
}

func (a staticAuthorizer) Authorize(r *http.Request, workspaceID uuid.UUID) (*uuid.UUID, error) {
	if a.token == "" || a.actorID == uuid.Nil || len(a.workspaces) == 0 {
		return nil, ErrRightsAPIUnauthenticated
	}
	const prefix = "Bearer "
	credential := r.Header.Get("Authorization")
	if !strings.HasPrefix(credential, prefix) || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(strings.TrimPrefix(credential, prefix))), []byte(a.token)) != 1 {
		return nil, ErrRightsAPIUnauthenticated
	}
	if _, ok := a.workspaces[workspaceID]; !ok {
		return nil, ErrRightsAPIForbidden
	}
	actor := a.actorID
	return &actor, nil
}

func (h *Handler) authorizeWorkspace(w http.ResponseWriter, r *http.Request, workspaceID uuid.UUID) (*uuid.UUID, bool) {
	actor, err := h.authorizer.Authorize(r, workspaceID)
	if errors.Is(err, ErrRightsAPIUnauthenticated) {
		httpserver.WriteError(w, r, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authenticated rights API credential is required", nil)
		return nil, false
	}
	if errors.Is(err, ErrRightsAPIForbidden) {
		httpserver.WriteError(w, r, http.StatusForbidden, "WORKSPACE_ACCESS_DENIED", "the authenticated principal is not authorized for this workspace", nil)
		return nil, false
	}
	if err != nil {
		httpserver.WriteError(w, r, http.StatusInternalServerError, "AUTHORIZATION_FAILED", err.Error(), nil)
		return nil, false
	}
	return actor, true
}
