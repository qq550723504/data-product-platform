package platformmanagement

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type overviewGate struct {
	testGate
	expired bool
}

func (g *overviewGate) Confirm(ctx context.Context, p principal.RequestPrincipal) (principal.Authority, error) {
	a, e := g.testGate.Confirm(ctx, p)
	a.ExpiresAt = time.Now().Add(time.Minute)
	if g.expired {
		a.ExpiresAt = time.Now().Add(-time.Second)
	}
	return a, e
}
func TestWorkspaceOverviewUsesCurrentAuthorityWithoutDirectoryCalls(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "unconfigured-directory", true: "configured-directory"}[configured], func(t *testing.T) {
			ws := uuid.New()
			r := &testResolver{p: principal.RequestPrincipal{ActorID: uuid.New(), WorkspaceID: ws, IdentityRevision: "1:1:1"}}
			g := &overviewGate{}
			d := &testDirectory{}
			bindings := map[uuid.UUID]Binding{}
			if configured {
				c, e := NewCatalog([]Role{{ID: "viewer", Name: "查询员", Permissions: []string{"management.members.read"}}})
				if e != nil {
					t.Fatal(e)
				}
				bindings[ws] = Binding{OrganizationID: "native-org", ProjectID: "native-app", Source: "SYNTHETIC_FIXTURE", Directory: d, Catalog: c}
			}
			mux := http.NewServeMux()
			Handler{Principal: r, Gate: g, Bindings: bindings, Audits: map[uuid.UUID]AuditBinding{ws: {Source: "SYNTHETIC_FIXTURE", Reader: testAudit{}}}, IdentitySource: "SYNTHETIC_FIXTURE"}.Register(mux)
			req := httptest.NewRequest("GET", "/api/v1/workspaces/"+ws.String()+"/management/workspace", nil)
			req.Header.Set("Authorization", "Bearer synthetic")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, req)
			if out.Code != 200 || r.calls != 2 || g.calls != 2 || d.read != 0 || out.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("overview bypassed authority or called directory", out.Code)
			}
			var wire struct {
				Envelope
				Data WorkspaceOverview
			}
			if json.Unmarshal(out.Body.Bytes(), &wire) != nil || wire.WorkspaceID != ws || wire.PrincipalID != r.p.ActorID || wire.Source != "SYNTHETIC_FIXTURE" || wire.Data.RecordState != "UNCONFIGURED" || wire.Data.Directory.Availability != "NOT_CHECKED" || wire.WriteEnabled {
				t.Fatal("invented workspace record/health or wrong provenance")
			}
			if wire.Data.Queries.Members != configured || wire.Data.Queries.Roles != configured || !wire.Data.Queries.Audit || wire.Data.Queries.Users {
				t.Fatal("overclaimed query readiness")
			}
			if !configured && (wire.Data.Directory.OrganizationID != nil || wire.Data.Directory.ProjectID != nil || wire.Data.Directory.Source != "UNCONFIGURED") {
				t.Fatal("invented native organization")
			}
		})
	}
}
func TestWorkspaceOverviewRejectsRevocationExpirySpoofingAndWrites(t *testing.T) {
	for _, fault := range []string{"foreign-space", "token-revoked", "identity-changed", "grant-revoked", "expired", "unknown-source", "query", "method"} {
		t.Run(fault, func(t *testing.T) {
			ws := uuid.New()
			r := &testResolver{p: principal.RequestPrincipal{ActorID: uuid.New(), WorkspaceID: ws, IdentityRevision: "1:1:1"}}
			g := &overviewGate{}
			h := Handler{Principal: r, Gate: g, IdentitySource: "CONFIGURED"}
			path := "/api/v1/workspaces/" + ws.String() + "/management/workspace"
			method, want := "GET", 200
			switch fault {
			case "foreign-space":
				path = "/api/v1/workspaces/" + uuid.NewString() + "/management/workspace"
				want = 403
			case "token-revoked":
				r.denyAfter = 1
				want = 401
			case "identity-changed":
				r.changedAfter = 1
				want = 401
			case "grant-revoked":
				g.denyAfter = 1
				want = 403
			case "expired":
				g.expired = true
				want = 503
			case "unknown-source":
				h.IdentitySource = ""
				want = 503
			case "query":
				path += "?from=workspace&workspaceId=foreign"
				want = 400
			case "method":
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
			if out.Code != want || body["data"] != nil {
				t.Fatal("workspace metadata released despite failed authority/query", fault, out.Code)
			}
		})
	}
}
