package deliveryhttp

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrDeliveryNotConfigured      = errors.New("direct data delivery is not configured")
	ErrCallerIdentityUntrusted    = errors.New("trusted caller identity is required")
	ErrCallerWorkspaceDenied      = errors.New("caller is not authorized for the workspace")
	ErrConsumerPrincipalMismatch  = errors.New("requested consumer does not match the trusted principal binding")
)

type CallerContext struct {
	PrincipalRef         string
	EffectiveConsumerRef string
	DelegationRef        string
}

type PrincipalResolver interface {
	Resolve(*http.Request, uuid.UUID, string) (CallerContext, error)
}

type staticPrincipalResolver struct {
	enabled      bool
	token        string
	principalRef string
	consumerRef  string
	workspaces   map[uuid.UUID]struct{}
}

// NewStaticPrincipalResolver is the controlled-pilot trust boundary. The
// principal/consumer/workspace binding comes only from runtime configuration;
// ordinary HTTP headers/body fields cannot mint or impersonate an identity.
func NewStaticPrincipalResolver(enabled bool, token, principalRef, consumerRef string, workspaceIDs []string) (PrincipalResolver, error) {
	resolver := staticPrincipalResolver{enabled: enabled}
	if !enabled {
		return resolver, nil
	}
	token = strings.TrimSpace(token)
	principalRef = strings.TrimSpace(principalRef)
	consumerRef = strings.TrimSpace(consumerRef)
	if token == "" || principalRef == "" || consumerRef == "" || len(workspaceIDs) == 0 {
		return nil, errors.New("direct delivery trusted principal configuration is incomplete")
	}
	resolver.token = token
	resolver.principalRef = principalRef
	resolver.consumerRef = consumerRef
	resolver.workspaces = make(map[uuid.UUID]struct{}, len(workspaceIDs))
	for _, value := range workspaceIDs {
		workspaceID, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil || workspaceID == uuid.Nil {
			return nil, errors.New("DELIVERY_API_WORKSPACE_IDS must contain non-nil UUIDs")
		}
		resolver.workspaces[workspaceID] = struct{}{}
	}
	return resolver, nil
}

func (r staticPrincipalResolver) Resolve(request *http.Request, workspaceID uuid.UUID, requestedConsumer string) (CallerContext, error) {
	if !r.enabled || r.token == "" || r.principalRef == "" || r.consumerRef == "" || len(r.workspaces) == 0 {
		return CallerContext{}, ErrDeliveryNotConfigured
	}
	const prefix = "Bearer "
	credential := request.Header.Get("Authorization")
	if !strings.HasPrefix(credential, prefix) {
		return CallerContext{}, ErrCallerIdentityUntrusted
	}
	supplied := strings.TrimSpace(strings.TrimPrefix(credential, prefix))
	if subtle.ConstantTimeCompare([]byte(supplied), []byte(r.token)) != 1 {
		return CallerContext{}, ErrCallerIdentityUntrusted
	}
	if _, ok := r.workspaces[workspaceID]; !ok {
		return CallerContext{}, ErrCallerWorkspaceDenied
	}
	requestedConsumer = strings.TrimSpace(requestedConsumer)
	if requestedConsumer == "" || requestedConsumer != r.consumerRef {
		return CallerContext{}, ErrConsumerPrincipalMismatch
	}
	return CallerContext{
		PrincipalRef:         r.principalRef,
		EffectiveConsumerRef: r.consumerRef,
	}, nil
}
