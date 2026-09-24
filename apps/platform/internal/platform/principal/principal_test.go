package principal

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestStaticResolverAuthenticatesServerOwnedPrincipal(t *testing.T) {
	workspaceID := uuid.New()
	actorID := uuid.New()
	resolver, err := NewStaticResolver(true, "secret", "reviewer:alice", actorID.String(), []string{workspaceID.String()}, []string{CapabilityHumanDecision})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}
	request := httptest.NewRequest("POST", "/", nil)
	request.Header.Set("X-Actor-ID", uuid.New().String())

	if _, err := resolver.Resolve(request, workspaceID, CapabilityHumanDecision); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("missing credential error = %v, want ErrUnauthenticated", err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	if _, err := resolver.Resolve(request, uuid.New(), CapabilityHumanDecision); !errors.Is(err, ErrWorkspaceDenied) {
		t.Fatalf("foreign workspace error = %v, want ErrWorkspaceDenied", err)
	}
	if _, err := resolver.Resolve(request, workspaceID, "ADMIN"); !errors.Is(err, ErrCapabilityDenied) {
		t.Fatalf("capability error = %v, want ErrCapabilityDenied", err)
	}
	principal, err := resolver.Resolve(request, workspaceID, CapabilityHumanDecision)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if principal.ActorID != actorID || principal.Subject != "reviewer:alice" || principal.WorkspaceID != workspaceID {
		t.Fatalf("resolved principal = %#v", principal)
	}
	if !principal.HasCapability(CapabilityHumanDecision) {
		t.Fatal("resolved principal missing HUMAN_DECISION capability")
	}
	if principal.ActorID.String() == request.Header.Get("X-Actor-ID") {
		t.Fatal("forged X-Actor-ID changed authoritative principal")
	}
}

func TestStaticResolverDisabledFailsClosed(t *testing.T) {
	resolver, err := NewStaticResolver(false, "", "", "", nil, nil)
	if err != nil {
		t.Fatalf("create disabled resolver: %v", err)
	}
	request := httptest.NewRequest("POST", "/", nil)
	request.Header.Set("Authorization", "Bearer anything")
	if _, err := resolver.Resolve(request, uuid.New(), CapabilityHumanDecision); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("disabled resolver error = %v, want ErrNotConfigured", err)
	}
}
