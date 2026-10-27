package platformmanagement

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type observationDirectory struct {
	*usersDirectory
	fault  string
	before func()
}

func (d observationDirectory) List(ctx context.Context, org string, p PageRequest) (Page, error) {
	if d.before != nil {
		d.before()
	}
	if d.fault == "unavailable" {
		return Page{}, ErrUnavailable
	}
	page, err := d.usersDirectory.List(ctx, org, p)
	if d.fault == "invalid" {
		page.Total = -1
	}
	return page, err
}

type observationOrganization struct {
	fault  string
	before func()
}

func (d observationOrganization) ReadOrganization(context.Context, string) (DirectoryOrganization, error) {
	if d.before != nil {
		d.before()
	}
	if d.fault == "unavailable" {
		return DirectoryOrganization{}, ErrUnavailable
	}
	if d.fault == "missing" {
		return DirectoryOrganization{}, ErrNotFound
	}
	id := "org"
	if d.fault == "invalid" {
		id = "foreign"
	}
	return DirectoryOrganization{ID: id, State: "active"}, nil
}

type observationGate struct {
	calls   int
	expires time.Time
}

func (g *observationGate) Confirm(_ context.Context, p principal.RequestPrincipal) (principal.Authority, error) {
	g.calls++
	expires := g.expires
	if g.calls > 1 {
		expires = time.Now().Add(time.Minute)
	}
	return principal.Authority{Principal: p, EvaluatedAt: time.Now(), ExpiresAt: expires}, nil
}

// A failed or invalid source observation is still an observation. It must pass
// the same uncached identity and current grant checks as a successful result.
func TestManagementFailedObservationsReconfirmAuthority(t *testing.T) {
	for _, resource := range []string{"members", "users", "organization"} {
		for _, fault := range []string{"unavailable", "missing", "invalid"} {
			for _, authority := range []string{"token-revoked", "identity-changed", "grant-revoked"} {
				t.Run(resource+"/"+fault+"/"+authority, func(t *testing.T) {
					b, d := usersBinding(t, 1)
					b.Directory = observationDirectory{usersDirectory: d, fault: fault}
					b.Organizations = observationOrganization{fault: fault}
					_, r, g, _ := setupManagement(t)
					want := 401
					switch authority {
					case "token-revoked":
						r.denyAfter = 1
					case "identity-changed":
						r.changedAfter = 1
					case "grant-revoked":
						g.denyAfter = 1
						want = 403
					}
					mux := http.NewServeMux()
					Handler{Principal: r, Gate: g, Bindings: map[uuid.UUID]Binding{r.p.WorkspaceID: b}}.Register(mux)
					path := "/api/v1/workspaces/" + r.p.WorkspaceID.String() + "/management/" + resource
					if resource == "users" {
						if fault == "missing" {
							path += "?userId=missing"
						} else {
							path += "?userId=user-000"
						}
					}
					if resource == "members" && fault == "missing" {
						path += "?memberId=missing"
					}
					req := httptest.NewRequest("GET", path, nil)
					req.Header.Set("Authorization", "Bearer synthetic")
					out := httptest.NewRecorder()
					mux.ServeHTTP(out, req)
					if out.Code != want || strings.Contains(out.Body.String(), `"data"`) || r.calls != 2 {
						t.Fatalf("observation bypassed current authority: status=%d calls=%d", out.Code, r.calls)
					}
				})
			}
		}
	}
}

func TestManagementOriginalObservationDeadlineCannotBeRenewed(t *testing.T) {
	for _, resource := range []string{"members", "users", "organization"} {
		t.Run(resource, func(t *testing.T) {
			b, d := usersBinding(t, 1)
			_, r, _, _ := setupManagement(t)
			g := &observationGate{expires: time.Now().Add(30 * time.Millisecond)}
			wait := func() { time.Sleep(time.Until(g.expires) + time.Millisecond) }
			b.Directory = observationDirectory{usersDirectory: d, before: wait}
			b.Organizations = observationOrganization{before: wait}
			mux := http.NewServeMux()
			Handler{Principal: r, Gate: g, Bindings: map[uuid.UUID]Binding{r.p.WorkspaceID: b}}.Register(mux)
			req := httptest.NewRequest("GET", "/api/v1/workspaces/"+r.p.WorkspaceID.String()+"/management/"+resource, nil)
			req.Header.Set("Authorization", "Bearer synthetic")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, req)
			if out.Code != 503 || !strings.Contains(out.Body.String(), "READ_UNAVAILABLE") || g.calls != 2 {
				t.Fatalf("original deadline replaced by fresh authority: %d %s", out.Code, out.Body.String())
			}
		})
	}
}
