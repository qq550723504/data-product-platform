package platformmanagement

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type usersDirectory struct {
	rows         []Member
	fault        string
	accountCalls int
}

func (d *usersDirectory) List(_ context.Context, org string, p PageRequest) (Page, error) {
	rows := append([]Member{}, d.rows...)
	total := len(rows)
	if d.fault == "changed-total" && p.Offset > 0 {
		total++
	}
	start := min(p.Offset, len(rows))
	end := min(start+p.Limit, len(rows))
	rows = rows[start:end]
	if d.fault == "foreign" && len(rows) > 0 {
		rows[0].OrganizationID = "foreign"
	}
	return Page{Items: rows, Total: total}, nil
}
func (d *usersDirectory) Read(_ context.Context, org, id string) (Member, error) {
	for _, m := range d.rows {
		if m.ID == id {
			if d.fault == "removed" {
				return Member{}, ErrNotFound
			}
			return m, nil
		}
	}
	return Member{}, ErrNotFound
}
func (d *usersDirectory) ReadUser(_ context.Context, org, id string) (UserProfile, error) {
	d.accountCalls++
	if d.fault == "profile-failure" {
		return UserProfile{}, ErrUnavailable
	}
	state := "active"
	if d.fault == "bad-state" {
		state = "unknown"
	}
	return UserProfile{ID: id, OrganizationID: org, Username: "directory-user", DisplayName: "Directory user", AccountState: state, CreatedAt: "2026-10-24T00:00:00Z", ChangedAt: "2026-10-24T00:00:00Z"}, nil
}
func usersBinding(t *testing.T, n int) (Binding, *usersDirectory) {
	t.Helper()
	directory := &usersDirectory{}
	for i := 0; i < n; i++ {
		directory.rows = append(directory.rows, Member{ID: fmt.Sprintf("grant-%03d", i), UserID: fmt.Sprintf("user-%03d", i), OrganizationID: "org", ProjectID: "project", DisplayName: fmt.Sprintf("Person %03d", i), LoginName: fmt.Sprintf("p-%03d", i), Roles: []string{"viewer"}, State: "inactive", CreatedAt: "2026-10-24T00:00:00Z", ChangedAt: "2026-10-24T00:00:00Z"})
	}
	c, e := NewCatalog([]Role{{ID: "viewer", Name: "查询", Permissions: []string{"management.members.read"}}})
	if e != nil {
		t.Fatal(e)
	}
	return Binding{OrganizationID: "org", ProjectID: "project", Source: "SYNTHETIC_FIXTURE", Directory: directory, Users: directory, Catalog: c}, directory
}
func TestUsersScopedDeduplicationSearchAndPaging(t *testing.T) {
	b, d := usersBinding(t, 125)
	d.rows[1].UserID = d.rows[0].UserID
	d.rows[1].DisplayName = d.rows[0].DisplayName
	d.rows[1].LoginName = d.rows[0].LoginName
	ws := uuid.New()
	page, e := QueryUsers(context.Background(), ws, b, PageRequest{Limit: 25, Offset: 25}, "")
	if e != nil || page.Total != 124 || len(page.Items) != 25 || page.Items[0].ID != "user-026" || d.accountCalls != 0 {
		t.Fatalf("page=%+v e=%v", page, e)
	}
	page, e = QueryUsers(context.Background(), ws, b, PageRequest{Limit: 25, Filter: ListFilter{Search: "Person 124"}}, "user-000")
	if e != nil || page.Total != 1 || page.Items[0].ID != "user-124" || len(page.Selected.Memberships) != 2 {
		t.Fatalf("explicit detail/search failed: %v", e)
	}
	if page.Selected.Profile.AccountState != "active" || page.Selected.Memberships[0].State != "inactive" || page.Selected.Identity.State != "UNCONFIGURED" || page.Selected.Identity.ActorID != nil {
		t.Fatal("account/member state or identity inferred")
	}
	_, e = QueryUsers(context.Background(), ws, b, PageRequest{Limit: 25}, "outside")
	if e != ErrNotFound || d.accountCalls != 1 {
		t.Fatal("unscoped ID reached native account reader")
	}
}
func TestUsersRejectPartialScopeProfileAndMembershipChanges(t *testing.T) {
	for _, fault := range []string{"changed-total", "foreign", "profile-failure", "bad-state", "removed"} {
		t.Run(fault, func(t *testing.T) {
			b, d := usersBinding(t, 101)
			d.fault = fault
			page, e := QueryUsers(context.Background(), uuid.New(), b, PageRequest{Limit: 25}, "user-001")
			if e == nil || len(page.Items) != 0 || page.Selected != nil {
				t.Fatal("published incomplete/foreign/invalid user")
			}
		})
	}
}
func TestUsersHTTPReusesIdentityAndWorkspaceBoundary(t *testing.T) {
	for _, fault := range []string{"ok", "foreign-space", "revoked", "changed", "invalid-query", "unknown-user"} {
		t.Run(fault, func(t *testing.T) {
			b, _ := usersBinding(t, 30)
			ws := uuid.New()
			resolver := &testResolver{p: principal.RequestPrincipal{ActorID: uuid.New(), WorkspaceID: ws, IdentityRevision: "1:1:1"}}
			gate := &testGate{}
			path := "/api/v1/workspaces/" + ws.String() + "/management/users?offset=25"
			want := 200
			switch fault {
			case "foreign-space":
				path = strings.Replace(path, ws.String(), uuid.NewString(), 1)
				want = 403
			case "revoked":
				resolver.denyAfter = 1
				want = 401
			case "changed":
				resolver.changedAfter = 1
				want = 401
			case "invalid-query":
				path += "&subject=forged"
				want = 400
			case "unknown-user":
				path += "&userId=outside"
				want = 404
			}
			mux := http.NewServeMux()
			Handler{Principal: resolver, Gate: gate, Bindings: map[uuid.UUID]Binding{ws: b}}.Register(mux)
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Authorization", "Bearer synthetic")
			out := httptest.NewRecorder()
			mux.ServeHTTP(out, r)
			if out.Code != want {
				t.Fatalf("code=%d want=%d", out.Code, want)
			}
			if want != 200 && strings.Contains(out.Body.String(), `"items"`) {
				t.Fatal("denial leaked user data")
			}
		})
	}
}
