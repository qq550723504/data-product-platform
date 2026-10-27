package platformmanagement

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testOrganization struct {
	foreign bool
	calls   int
}

func (r *testOrganization) ReadOrganization(_ context.Context, id string) (DirectoryOrganization, error) {
	r.calls++
	if r.foreign {
		id = "foreign"
	}
	return DirectoryOrganization{ID: id, State: "active"}, nil
}
func TestOrganizationReadReusesScopeAndRevocationGates(t *testing.T) {
	for _, fault := range []string{"ok", "foreign-record", "foreign-workspace", "token-revoked", "revision-changed", "grant-revoked", "unconfigured", "spoofed-id", "write"} {
		t.Run(fault, func(t *testing.T) {
			b, _ := usersBinding(t, 1)
			org := &testOrganization{}
			b.Organizations = org
			_, r, g, _ := setupManagement(t)
			ws := r.p.WorkspaceID
			path := "/api/v1/workspaces/" + ws.String() + "/management/organization"
			method, want := "GET", 200
			switch fault {
			case "foreign-record":
				org.foreign = true
				want = 503
			case "foreign-workspace":
				path = "/api/v1/workspaces/" + uuid.NewString() + "/management/organization"
				want = 403
			case "token-revoked":
				r.denyAfter = 1
				want = 401
			case "revision-changed":
				r.changedAfter = 1
				want = 401
			case "grant-revoked":
				g.denyAfter = 1
				want = 403
			case "unconfigured":
				b.Organizations = nil
				want = 503
			case "spoofed-id":
				path += "?organizationId=foreign"
				want = 400
			case "write":
				method = "POST"
				want = 405
			}
			mux := http.NewServeMux()
			Handler{Principal: r, Gate: g, Bindings: map[uuid.UUID]Binding{ws: b}}.Register(mux)
			req := httptest.NewRequest(method, path, nil)
			req.Header.Set("Authorization", "Bearer synthetic")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, req)
			var body map[string]any
			_ = json.Unmarshal(out.Body.Bytes(), &body)
			if out.Code != want || want != 200 && body["data"] != nil {
				t.Fatal("organization scope/revocation failure", fault, out.Code)
			}
			if want == 200 {
				var response struct{ Data OrganizationOverview }
				_ = json.Unmarshal(out.Body.Bytes(), &response)
				if response.Data.Organization.Name != nil || response.Data.DepartmentHierarchyState != "UNCONFIGURED" || org.calls != 1 || g.calls != 2 {
					t.Fatal("invented organization facts")
				}
			}
		})
	}
}
