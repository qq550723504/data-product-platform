package principal

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrUnauthenticated  = errors.New("authenticated principal required")
	ErrWorkspaceDenied  = errors.New("principal is not authorized for workspace")
	ErrCapabilityDenied = errors.New("principal lacks required capability")
	ErrNotConfigured    = errors.New("principal resolver is not configured")
)

const CapabilityHumanDecision = "HUMAN_DECISION"

type RequestPrincipal struct {
	Subject      string
	ActorID      uuid.UUID
	WorkspaceID  uuid.UUID
	Capabilities map[string]struct{}
}

func (p RequestPrincipal) HasCapability(capability string) bool {
	_, ok := p.Capabilities[strings.TrimSpace(capability)]
	return ok
}

type Resolver interface {
	Resolve(*http.Request, uuid.UUID, string) (RequestPrincipal, error)
}

type StaticResolver struct {
	enabled      bool
	token        string
	subject      string
	actorID      uuid.UUID
	workspaces   map[uuid.UUID]struct{}
	capabilities map[string]struct{}
}

// NewStaticResolver is a controlled-pilot authenticated principal boundary.
// Credentials, actor identity, workspace scope and capabilities are all
// server-side configuration. Request headers/body fields cannot mint identity.
func NewStaticResolver(enabled bool, token, subject, actorID string, workspaceIDs, capabilities []string) (Resolver, error) {
	r := StaticResolver{enabled: enabled}
	if !enabled {
		return r, nil
	}
	token = strings.TrimSpace(token)
	subject = strings.TrimSpace(subject)
	actorID = strings.TrimSpace(actorID)
	if token == "" || actorID == "" || len(workspaceIDs) == 0 || len(capabilities) == 0 {
		return nil, errors.New("authenticated principal configuration is incomplete")
	}
	actor, err := uuid.Parse(actorID)
	if err != nil || actor == uuid.Nil {
		return nil, errors.New("principal actor id must be a non-nil UUID")
	}
	if subject == "" {
		subject = actor.String()
	}
	r.token = token
	r.subject = subject
	r.actorID = actor
	r.workspaces = make(map[uuid.UUID]struct{}, len(workspaceIDs))
	for _, raw := range workspaceIDs {
		id, parseErr := uuid.Parse(strings.TrimSpace(raw))
		if parseErr != nil || id == uuid.Nil {
			return nil, errors.New("principal workspace ids must contain non-nil UUIDs")
		}
		r.workspaces[id] = struct{}{}
	}
	r.capabilities = make(map[string]struct{}, len(capabilities))
	for _, raw := range capabilities {
		capability := strings.TrimSpace(raw)
		if capability == "" {
			return nil, errors.New("principal capabilities must not be blank")
		}
		r.capabilities[capability] = struct{}{}
	}
	return r, nil
}

func (r StaticResolver) Resolve(request *http.Request, workspaceID uuid.UUID, capability string) (RequestPrincipal, error) {
	if !r.enabled || r.token == "" || r.actorID == uuid.Nil || len(r.workspaces) == 0 || len(r.capabilities) == 0 {
		return RequestPrincipal{}, ErrNotConfigured
	}
	const prefix = "Bearer "
	credential := request.Header.Get("Authorization")
	if !strings.HasPrefix(credential, prefix) {
		return RequestPrincipal{}, ErrUnauthenticated
	}
	supplied := strings.TrimSpace(strings.TrimPrefix(credential, prefix))
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(r.token)) != 1 {
		return RequestPrincipal{}, ErrUnauthenticated
	}
	if workspaceID == uuid.Nil {
		return RequestPrincipal{}, ErrWorkspaceDenied
	}
	if _, ok := r.workspaces[workspaceID]; !ok {
		return RequestPrincipal{}, ErrWorkspaceDenied
	}
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return RequestPrincipal{}, ErrCapabilityDenied
	}
	if _, ok := r.capabilities[capability]; !ok {
		return RequestPrincipal{}, ErrCapabilityDenied
	}
	caps := make(map[string]struct{}, len(r.capabilities))
	for key := range r.capabilities {
		caps[key] = struct{}{}
	}
	return RequestPrincipal{
		Subject: r.subject, ActorID: r.actorID, WorkspaceID: workspaceID, Capabilities: caps,
	}, nil
}
