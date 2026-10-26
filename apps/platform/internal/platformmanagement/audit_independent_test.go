package platformmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

func TestAuditDoesNotRequireDirectoryConfiguration(t *testing.T) {
	_, r, _, _ := setupManagement(t)
	mux := http.NewServeMux()
	Handler{Principal: r, Gate: &overviewGate{}, Audits: map[uuid.UUID]AuditBinding{r.p.WorkspaceID: {Source: "SYNTHETIC_FIXTURE", Reader: testAudit{}}}, IdentitySource: "SYNTHETIC_FIXTURE"}.Register(mux)
	for _, resource := range []string{"audit", "workspace", "configuration"} {
		req := httptest.NewRequest("GET", "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/"+resource, nil)
		req.Header.Set("Authorization", "Bearer synthetic")
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		if out.Code != 200 {
			t.Fatalf("audit-only %s denied by unrelated directory: status=%d", resource, out.Code)
		}
		var wire struct {
			Envelope
			Data struct{ Queries struct{ Audit bool } }
		}
		if json.Unmarshal(out.Body.Bytes(), &wire) != nil || wire.Source != "SYNTHETIC_FIXTURE" || resource != "audit" && !wire.Data.Queries.Audit {
			t.Fatal("audit-only provenance or configuration projection incorrect")
		}
	}
}

type auditFunc func(context.Context, uuid.UUID, uuid.UUID, string) (AuditPage, error)

func (f auditFunc) List(ctx context.Context, ws, actor uuid.UUID, cursor string) (AuditPage, error) {
	return f(ctx, ws, actor, cursor)
}

func TestAuditSourceIsExplicitWorkspaceScopedAndIndependentOfIdentitySource(t *testing.T) {
	for _, kind := range []string{"configured", "missing", "foreign", "unknown-source"} {
		t.Run(kind, func(t *testing.T) {
			_, r, _, _ := setupManagement(t)
			calls := 0
			ws := r.p.WorkspaceID
			reader := auditFunc(func(_ context.Context, gotWS, gotActor uuid.UUID, _ string) (AuditPage, error) {
				calls++
				if gotWS != ws || gotActor != r.p.ActorID {
					t.Fatal("untrusted audit scope")
				}
				return AuditPage{Items: []AuditEvent{}}, nil
			})
			audits := map[uuid.UUID]AuditBinding{ws: {Source: "CONFIGURED", Reader: reader}}
			want := 200
			switch kind {
			case "missing":
				audits = nil
				want = 503
			case "foreign":
				audits = map[uuid.UUID]AuditBinding{uuid.New(): {Source: "CONFIGURED", Reader: reader}}
				want = 503
			case "unknown-source":
				audits[ws] = AuditBinding{Source: "UNCONFIGURED", Reader: reader}
				want = 503
			}
			mux := http.NewServeMux()
			Handler{Principal: r, Gate: &overviewGate{}, Audits: audits, IdentitySource: "SYNTHETIC_FIXTURE"}.Register(mux)
			for _, resource := range []string{"workspace", "configuration", "audit"} {
				req := httptest.NewRequest("GET", "/api/v1/workspaces/"+ws.String()+"/management/"+resource, nil)
				req.Header.Set("Authorization", "Bearer synthetic")
				out := httptest.NewRecorder()
				mux.ServeHTTP(out, req)
				if resource == "audit" {
					if out.Code != want {
						t.Fatalf("status=%d", out.Code)
					}
					if want == 200 && !strings.Contains(out.Body.String(), `"source":"CONFIGURED"`) {
						t.Fatal("audit inherited identity/directory provenance")
					}
					if want != 200 && strings.Contains(out.Body.String(), `"data"`) {
						t.Fatal("unconfigured audit exposed records")
					}
				} else {
					var d struct {
						Data struct{ Queries struct{ Audit bool } }
					}
					if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &d) != nil || d.Data.Queries.Audit != (kind == "configured") {
						t.Fatal("audit configuration projection disagrees")
					}
				}
			}
			if (calls == 1) != (kind == "configured") {
				t.Fatal("unconfigured source invoked")
			}
		})
	}
}

func TestAuditFailureObservationsCannotBypassCurrentAuthority(t *testing.T) {
	for _, failure := range []error{ErrInvalidRequest, ErrUnavailable, errors.New("PRIVATE_AUDIT_FAILURE")} {
		for _, invalidation := range []string{"token", "revision", "grant"} {
			t.Run(fmt.Sprintf("%T/%s", failure, invalidation), func(t *testing.T) {
				_, r, _, _ := setupManagement(t)
				g := &overviewGate{}
				switch invalidation {
				case "token":
					r.denyAfter = 1
				case "revision":
					r.changedAfter = 1
				case "grant":
					g.denyAfter = 1
				}
				reader := auditFunc(func(context.Context, uuid.UUID, uuid.UUID, string) (AuditPage, error) { return AuditPage{}, failure })
				mux := http.NewServeMux()
				Handler{Principal: r, Gate: g, IdentitySource: "SYNTHETIC_FIXTURE", Audits: map[uuid.UUID]AuditBinding{r.p.WorkspaceID: {Source: "CONFIGURED", Reader: reader}}}.Register(mux)
				req := httptest.NewRequest("GET", "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/audit", nil)
				req.Header.Set("Authorization", "Bearer synthetic")
				out := httptest.NewRecorder()
				mux.ServeHTTP(out, req)
				want := 401
				if invalidation == "grant" {
					want = 403
				}
				if out.Code != want || r.calls != 2 || strings.Contains(out.Body.String(), "PRIVATE") || strings.Contains(out.Body.String(), `"data"`) {
					t.Fatal("audit error disclosed before current authority")
				}
			})
		}
	}
}

type renewingAuditGate struct {
	calls int
	first time.Time
}

func (g *renewingAuditGate) Confirm(_ context.Context, p principal.RequestPrincipal) (principal.Authority, error) {
	g.calls++
	expires := time.Now().Add(time.Minute)
	if g.calls == 1 {
		g.first = time.Now().Add(25 * time.Millisecond)
		expires = g.first
	}
	return principal.Authority{Principal: p, EvaluatedAt: time.Now(), ExpiresAt: expires}, nil
}
func TestAuditOriginalDeadlineCannotBeExtendedByFreshConfirmation(t *testing.T) {
	_, r, _, _ := setupManagement(t)
	g := &renewingAuditGate{}
	calls := 0
	reader := auditFunc(func(context.Context, uuid.UUID, uuid.UUID, string) (AuditPage, error) {
		calls++
		time.Sleep(time.Until(g.first) + time.Millisecond)
		return AuditPage{Items: []AuditEvent{}}, nil
	})
	mux := http.NewServeMux()
	Handler{Principal: r, Gate: g, IdentitySource: "SYNTHETIC_FIXTURE", Audits: map[uuid.UUID]AuditBinding{r.p.WorkspaceID: {Source: "CONFIGURED", Reader: reader}}}.Register(mux)
	req := httptest.NewRequest("GET", "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/audit", nil)
	req.Header.Set("Authorization", "Bearer synthetic")
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, req)
	if out.Code != 503 || calls != 1 || g.calls != 2 || strings.Contains(out.Body.String(), `"data"`) {
		t.Fatal("old deadline renewed after source read")
	}
}

type failedAudit struct{}

func (failedAudit) List(context.Context, uuid.UUID, uuid.UUID, string) (AuditPage, error) {
	return AuditPage{}, ErrUnavailable
}

func TestAuditErrorMustReconfirmIdentityBeforeDisclosingFailure(t *testing.T) {
	_, r, _, _ := setupManagement(t)
	r.denyAfter = 1
	c, _ := NewCatalog([]Role{{"viewer", "查看", []string{"management.audit.read"}}})
	mux := http.NewServeMux()
	Handler{Principal: r, Gate: &overviewGate{}, Audits: map[uuid.UUID]AuditBinding{r.p.WorkspaceID: {Source: "SYNTHETIC_FIXTURE", Reader: failedAudit{}}}, IdentitySource: "SYNTHETIC_FIXTURE", Bindings: map[uuid.UUID]Binding{r.p.WorkspaceID: {OrganizationID: "org", ProjectID: "project", Source: "SYNTHETIC_FIXTURE", Directory: &testDirectory{}, Catalog: c}}}.Register(mux)
	req := httptest.NewRequest("GET", "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/audit", nil)
	req.Header.Set("Authorization", "Bearer synthetic")
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, req)
	if out.Code != 401 || r.calls != 2 {
		t.Fatalf("audit error bypassed post-read identity: status=%d calls=%d", out.Code, r.calls)
	}
}
