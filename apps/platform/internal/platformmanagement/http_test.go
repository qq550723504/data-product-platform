package platformmanagement

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testResolver struct {
	p            principal.RequestPrincipal
	calls        int
	err          error
	denyAfter    int
	changedAfter int
}

func (r *testResolver) Resolve(_ *http.Request, ws uuid.UUID, c string) (principal.RequestPrincipal, error) {
	r.calls++
	if r.denyAfter > 0 && r.calls > r.denyAfter {
		return principal.RequestPrincipal{}, principal.ErrUnauthenticated
	}
	if c != principal.CapabilityManagementRead {
		panic("wrong capability")
	}
	if r.err != nil {
		return r.p, r.err
	}
	if ws != r.p.WorkspaceID {
		return r.p, principal.ErrWorkspaceDenied
	}
	if r.changedAfter > 0 && r.calls > r.changedAfter {
		p := r.p
		p.IdentityRevision = "1:1:2"
		return p, nil
	}
	return r.p, nil
}

type testGate struct {
	calls     int
	denyAfter int
}

func (g *testGate) Confirm(_ context.Context, p principal.RequestPrincipal) (principal.Authority, error) {
	g.calls++
	if g.denyAfter > 0 && g.calls > g.denyAfter {
		return principal.Authority{}, principal.ErrWorkspaceDenied
	}
	return principal.Authority{Principal: p, EvaluatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}, nil
}

type testDirectory struct {
	read    int
	foreign bool
}

type testAudit struct{}

func (testAudit) List(context.Context, uuid.UUID, uuid.UUID, string) (AuditPage, error) {
	return AuditPage{Items: []AuditEvent{}}, nil
}

func (d *testDirectory) List(_ context.Context, org string, p PageRequest) (Page, error) {
	d.read++
	if d.foreign {
		org = "foreign"
	}
	return Page{Items: []Member{{ID: "grant", UserID: "user", OrganizationID: org, ProjectID: "project", Roles: []string{"viewer"}}}, Total: 1}, nil
}
func (d *testDirectory) Read(ctx context.Context, org, id string) (Member, error) {
	return Member{}, ErrNotFound
}
func setupManagement(t *testing.T) (*http.ServeMux, *testResolver, *testGate, *testDirectory) {
	t.Helper()
	ws := uuid.New()
	r := &testResolver{p: principal.RequestPrincipal{ActorID: uuid.New(), WorkspaceID: ws, IdentityRevision: "1:1:1"}}
	g := &testGate{}
	d := &testDirectory{}
	c, err := NewCatalog([]Role{{"viewer", "查询成员", []string{"management.members.read"}}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Handler{Principal: r, Gate: g, Audits: map[uuid.UUID]AuditBinding{ws: {Source: "SYNTHETIC_FIXTURE", Reader: testAudit{}}}, IdentitySource: "SYNTHETIC_FIXTURE", Bindings: map[uuid.UUID]Binding{ws: {OrganizationID: "org", ProjectID: "project", Source: "SYNTHETIC_FIXTURE", Directory: d, Catalog: c}}}.Register(mux)
	return mux, r, g, d
}
func TestManagementScopeRevocationAndProjection(t *testing.T) {
	for _, tc := range []string{"ok", "foreignWorkspace", "revokedBefore", "revokedDuring", "tokenRevokedDuring", "identityChangedDuring", "foreignProvider"} {
		t.Run(tc, func(t *testing.T) {
			mux, r, g, d := setupManagement(t)
			ws := r.p.WorkspaceID
			status := 200
			switch tc {
			case "foreignWorkspace":
				ws = uuid.New()
				status = 403
			case "revokedBefore":
				r.err = principal.ErrCapabilityDenied
				status = 403
			case "revokedDuring":
				g.denyAfter = 1
				status = 403
			case "tokenRevokedDuring":
				r.denyAfter = 1
				status = 401
			case "identityChangedDuring":
				r.changedAfter = 1
				status = 401
			case "foreignProvider":
				d.foreign = true
				status = 503
			}
			req := httptest.NewRequest("GET", "/api/v1/workspaces/"+ws.String()+"/management/members", nil)
			req.Header.Set("Authorization", "Bearer synthetic")
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != status {
				t.Fatalf("code %d %s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("cacheable authority")
			}
			if status != 200 && strings.Contains(rr.Body.String(), `"items"`) {
				t.Fatal("denial leaked records")
			}
			if status == 200 {
				var wire map[string]any
				_ = json.Unmarshal(rr.Body.Bytes(), &wire)
				if wire["writeEnabled"] != false || wire["principalId"] != r.p.ActorID.String() || g.calls != 2 {
					t.Fatal("unsafe projection")
				}
			}
		})
	}
}
func TestManagementRejectsInvalidEnvelopeWithoutReadingSources(t *testing.T) {
	for _, path := range []string{"?offset=-1", "?offset=01", "?offset=10001", "?offset=0&offset=25", "?workspaceId=other", "?q=", "?state=pending", "?memberId=../other", "?q=%00"} {
		mux, r, _, d := setupManagement(t)
		req := httptest.NewRequest("GET", "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/members"+path, nil)
		req.Header.Set("Authorization", "Bearer synthetic")
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != 400 || d.read != 0 {
			t.Fatalf("accepted %s: %d", path, rr.Code)
		}
	}
	for _, method := range []string{"POST", "PATCH", "DELETE", "PUT", "HEAD"} {
		mux, r, _, d := setupManagement(t)
		req := httptest.NewRequest(method, "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/members", nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != 405 || d.read != 0 {
			t.Fatal("write/method mounted")
		}
	}
}

func TestRoleAndAuditReadsReconfirmTokenBeforeReturningData(t *testing.T) {
	for _, resource := range []string{"roles", "audit"} {
		for _, changed := range []bool{false, true} {
			t.Run(resource+"/"+map[bool]string{false: "revoked", true: "changed"}[changed], func(t *testing.T) {
				mux, resolver, _, _ := setupManagement(t)
				if changed {
					resolver.changedAfter = 1
				} else {
					resolver.denyAfter = 1
				}
				request := httptest.NewRequest("GET", "/api/v1/workspaces/"+resolver.p.WorkspaceID.String()+"/management/"+resource, nil)
				request.Header.Set("Authorization", "Bearer synthetic")
				response := httptest.NewRecorder()
				mux.ServeHTTP(response, request)
				if response.Code != 401 || strings.Contains(response.Body.String(), `"data"`) || resolver.calls != 2 {
					t.Fatal("released role/audit data after identity revocation")
				}
			})
		}
	}
}
func TestAuditCursorBindsCallerAndWorkspace(t *testing.T) {
	ws, actor := uuid.New(), uuid.New()
	c := auditCursor{ws, actor, time.Now().UTC(), time.Now().UTC().Add(-time.Hour), uuid.New()}
	b, _ := json.Marshal(c)
	raw := encodeTestCursor(b)
	if _, err := decodeCursor(raw, ws, actor); err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][2]uuid.UUID{{uuid.New(), actor}, {ws, uuid.New()}} {
		if _, err := decodeCursor(raw, ids[0], ids[1]); err == nil {
			t.Fatal("cross-scope cursor accepted")
		}
	}
}
func TestCatalogCannotAmplifyUnknownRolesOrWritePermissions(t *testing.T) {
	c, e := NewCatalog([]Role{{"viewer", "查看", []string{"management.members.read"}}})
	if e != nil {
		t.Fatal(e)
	}
	p, e := c.Permissions([]string{"platform_admin", "unknown"})
	if e != nil || len(p) != 0 {
		t.Fatal("unknown role gained permission")
	}
	if _, e = NewCatalog([]Role{{"writer", "写", []string{"management.members.write"}}}); e == nil {
		t.Fatal("write permission accepted")
	}
}
