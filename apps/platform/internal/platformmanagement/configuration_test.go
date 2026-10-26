package platformmanagement

import (
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigurationProjectsOnlyCurrentManagementCompositionWithoutSecretsOrNativeCalls(t *testing.T) {
	t.Setenv("DATABASE_URL", "CONFIGURATION_PRIVATE_DATABASE_CANARY")
	t.Setenv("BROWSER_IAM_CLIENT_SECRET", "CONFIGURATION_PRIVATE_SECRET_CANARY")
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "unconfigured", true: "configured"}[configured], func(t *testing.T) {
			_, r, _, _ := setupManagement(t)
			g := &overviewGate{}
			bindings := map[uuid.UUID]Binding{}
			var native *usersDirectory
			var org *testOrganization
			if configured {
				b, d := usersBinding(t, 1)
				native = d
				org = &testOrganization{}
				b.OrganizationID = "CONFIGURATION_PRIVATE_ORG_CANARY"
				b.ProjectID = "CONFIGURATION_PRIVATE_PROJECT_CANARY"
				b.Organizations = org
				bindings[r.p.WorkspaceID] = b
			}
			mux := http.NewServeMux()
			Handler{Principal: r, Gate: g, Bindings: bindings, Audits: map[uuid.UUID]AuditBinding{r.p.WorkspaceID: {Source: "SYNTHETIC_FIXTURE", Reader: testAudit{}}}, IdentitySource: "SYNTHETIC_FIXTURE"}.Register(mux)
			req := httptest.NewRequest("GET", "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/configuration", nil)
			req.Header.Set("Authorization", "Bearer synthetic")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, req)
			if out.Code != 200 || r.calls != 2 || g.calls != 2 || strings.Contains(out.Body.String(), "CONFIGURATION_PRIVATE_") {
				t.Fatal("configuration scope, authority or secret projection failed", out.Code)
			}
			var wire struct {
				Envelope
				Data ConfigurationOverview
			}
			if json.Unmarshal(out.Body.Bytes(), &wire) != nil || wire.Data.Scope != "CURRENT_WORKSPACE_MANAGEMENT" || wire.Data.Mode != "QUERY_ONLY" || wire.Data.Directory.Availability != "NOT_CHECKED" || !wire.Data.Queries.Workspace || wire.Data.Queries.Members != configured || wire.Data.Queries.Organization != configured || wire.Source != "SYNTHETIC_FIXTURE" || wire.WriteEnabled {
				t.Fatal("overclaimed scope, health or capabilities")
			}
			if configured && (native.accountCalls != 0 || org.calls != 0) {
				t.Fatal("configuration called native source")
			}
		})
	}
}
func TestConfigurationRejectsRevokedExpiredForeignAndUnallowlistedQueries(t *testing.T) {
	for _, fault := range []string{"foreign-workspace", "revoked", "changed", "local-revoked", "expired", "unknown-source", "secret-selector", "write"} {
		t.Run(fault, func(t *testing.T) {
			_, r, _, _ := setupManagement(t)
			g := &overviewGate{}
			h := Handler{Principal: r, Gate: g, IdentitySource: "SYNTHETIC_FIXTURE"}
			path := "/api/v1/workspaces/" + r.p.WorkspaceID.String() + "/management/configuration"
			method, want := "GET", 200
			switch fault {
			case "foreign-workspace":
				path = "/api/v1/workspaces/" + uuid.NewString() + "/management/configuration"
				want = 403
			case "revoked":
				r.denyAfter = 1
				want = 401
			case "changed":
				r.changedAfter = 1
				want = 401
			case "local-revoked":
				g.denyAfter = 1
				want = 403
			case "expired":
				g.expired = true
				want = 503
			case "unknown-source":
				h.IdentitySource = "CONFIGURATION_PRIVATE_CANARY"
				want = 503
			case "secret-selector":
				path += "?key=DATABASE_URL"
				want = 400
			case "write":
				method = "POST"
				want = 405
			}
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer synthetic")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, req)
			var body map[string]any
			_ = json.Unmarshal(out.Body.Bytes(), &body)
			if out.Code != want || body["data"] != nil || strings.Contains(out.Body.String(), "PRIVATE") {
				t.Fatal("configuration exposed data despite rejection", fault, out.Code)
			}
		})
	}
}
