package bindings

import (
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type rejectedResolver struct {
	err    error
	called *bool
}

func (r rejectedResolver) Resolve(_ *http.Request, _ uuid.UUID, cap string) (principal.RequestPrincipal, error) {
	*r.called = true
	if cap != principal.CapabilityAcceptanceRead {
		panic("cross-module capability")
	}
	return principal.RequestPrincipal{}, r.err
}
func TestAcceptancePrincipalRejectsUntrustedEnvelopeBeforeResolution(t *testing.T) {
	good := `{"workspace_id":"11111111-1111-4111-8111-111111111111"}`
	for _, tc := range []struct{ body, path, method, header string }{
		{good, "/?actor=other", "POST", ""}, {good, "/", "GET", ""}, {good, "/", "POST", "Cookie"},
		{`{"workspace_id":"11111111-1111-4111-8111-111111111111","actor":"other"}`, "/", "POST", ""},
		{`{"workspace_id":"11111111-1111-4111-8111-111111111111","workspace_id":"11111111-1111-4111-8111-111111111111"}`, "/", "POST", ""},
		{good + `{}`, "/", "POST", ""}, {`{"workspace_id":"00000000-0000-0000-0000-000000000000"}`, "/", "POST", ""}, {strings.Repeat(" ", 4097) + good, "/", "POST", ""},
	} {
		called := false
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer synthetic")
		if tc.header != "" {
			req.Header.Set(tc.header, "synthetic")
		}
		NewStore(nil).AcceptancePrincipal(rejectedResolver{principal.ErrUnauthenticated, &called})(rr, req)
		if called || rr.Code < 400 {
			t.Fatalf("untrusted envelope resolved: %d", rr.Code)
		}
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{principal.ErrUnauthenticated, 401}, {principal.ErrWorkspaceDenied, 403}, {principal.ErrAuthorityUnavailable, 503}} {
		called := false
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", strings.NewReader(good))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer synthetic")
		NewStore(nil).AcceptancePrincipal(rejectedResolver{tc.err, &called})(rr, req)
		if !called || rr.Code != tc.status {
			t.Fatalf("identity failure %d", rr.Code)
		}
	}
}
