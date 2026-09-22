package deliveryhttp

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestStaticPrincipalResolverFailsClosedAndRejectsConsumerSpoofing(t *testing.T) {
	workspaceID := uuid.New()
	resolver, err := NewStaticPrincipalResolver(true, "secret", "principal-a", "consumer-a", []string{workspaceID.String()})
	if err != nil {
		t.Fatalf("create resolver: %v", err)
	}
	request := httptest.NewRequest("POST", "/", nil)

	if _, err := resolver.Resolve(request, workspaceID, "consumer-a"); !errors.Is(err, ErrCallerIdentityUntrusted) {
		t.Fatalf("missing credential error = %v, want ErrCallerIdentityUntrusted", err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	if _, err := resolver.Resolve(request, uuid.New(), "consumer-a"); !errors.Is(err, ErrCallerWorkspaceDenied) {
		t.Fatalf("foreign workspace error = %v, want ErrCallerWorkspaceDenied", err)
	}
	if _, err := resolver.Resolve(request, workspaceID, "consumer-b"); !errors.Is(err, ErrConsumerPrincipalMismatch) {
		t.Fatalf("consumer spoof error = %v, want ErrConsumerPrincipalMismatch", err)
	}
	caller, err := resolver.Resolve(request, workspaceID, "consumer-a")
	if err != nil || caller.PrincipalRef != "principal-a" || caller.EffectiveConsumerRef != "consumer-a" || caller.DelegationRef != "" {
		t.Fatalf("resolved caller = %#v, err=%v", caller, err)
	}
}

func TestStaticPrincipalResolverDisabledIsNotAuthentication(t *testing.T) {
	resolver, err := NewStaticPrincipalResolver(false, "", "", "", nil)
	if err != nil {
		t.Fatalf("create disabled resolver: %v", err)
	}
	request := httptest.NewRequest("POST", "/", nil)
	request.Header.Set("Authorization", "Bearer anything")
	if _, err := resolver.Resolve(request, uuid.New(), "consumer-a"); !errors.Is(err, ErrDeliveryNotConfigured) {
		t.Fatalf("disabled resolver error = %v, want ErrDeliveryNotConfigured", err)
	}
}
