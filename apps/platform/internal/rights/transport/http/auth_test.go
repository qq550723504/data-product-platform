package rightshttp

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestStaticAuthorizerRequiresCredentialAndWorkspace(t *testing.T) {
	workspace := uuid.New()
	actor := uuid.New()
	authorizer, err := NewStaticAuthorizer("secret", actor.String(), []string{workspace.String()})
	if err != nil {
		t.Fatalf("create authorizer: %v", err)
	}

	request := httptest.NewRequest("POST", "/api/v1/rights-declarations", nil)
	if _, err := authorizer.Authorize(request, workspace); err != ErrRightsAPIUnauthenticated {
		t.Fatalf("missing credential error = %v, want ErrRightsAPIUnauthenticated", err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	resolved, err := authorizer.Authorize(request, workspace)
	if err != nil || resolved == nil || *resolved != actor {
		t.Fatalf("valid credential result = %v, %v, want actor %s", resolved, err, actor)
	}
	if _, err := authorizer.Authorize(request, uuid.New()); err != ErrRightsAPIForbidden {
		t.Fatalf("unauthorized workspace error = %v, want ErrRightsAPIForbidden", err)
	}
}

func TestStaticAuthorizerFailsClosedWhenUnconfigured(t *testing.T) {
	authorizer, err := NewStaticAuthorizer("", "", nil)
	if err != nil {
		t.Fatalf("create unconfigured authorizer: %v", err)
	}
	request := httptest.NewRequest("POST", "/api/v1/rights-declarations", nil)
	if _, err := authorizer.Authorize(request, uuid.New()); err != ErrRightsAPIUnauthenticated {
		t.Fatalf("unconfigured authorizer error = %v, want ErrRightsAPIUnauthenticated", err)
	}
}
