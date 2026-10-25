package platformmanagement

import (
	"context"
	"github.com/google/uuid"
	"regexp"
	"sort"
	"strings"
	"time"
)

// User references are deduplicated from the already authorized directory.
// They are not a global account search or canonical platform actors.
type UserReference struct {
	ID              string `json:"id"`
	DisplayName     string `json:"displayName"`
	LoginName       string `json:"loginName"`
	MembershipCount int    `json:"membershipCount"`
}
type UserProfile struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Username       string `json:"username"`
	DisplayName    string `json:"displayName"`
	AccountState   string `json:"accountState"`
	CreatedAt      string `json:"createdAt"`
	ChangedAt      string `json:"changedAt"`
}
type UserReader interface {
	ReadUser(context.Context, string, string) (UserProfile, error)
}
type UserIdentityLink struct {
	WorkspaceID uuid.UUID  `json:"workspaceId"`
	State       string     `json:"state"`
	ActorID     *string    `json:"actorId"`
	Revision    string     `json:"revision"`
	BasisRef    string     `json:"basisRef"`
	ExpiresAt   *time.Time `json:"expiresAt"`
}
type UserIdentityReader interface {
	ReadUserIdentity(context.Context, uuid.UUID, string) (UserIdentityLink, error)
}
type UserDetail struct {
	Profile     UserProfile      `json:"profile"`
	Memberships []Member         `json:"memberships"`
	Identity    UserIdentityLink `json:"identity"`
}
type UsersPage struct {
	Items           []UserReference `json:"items"`
	Total           int             `json:"total"`
	Offset          int             `json:"offset"`
	Limit           int             `json:"limit"`
	Selected        *UserDetail     `json:"selected"`
	RoleDefinitions []Role          `json:"roleDefinitions"`
}

// QueryUsers reuses membership scope, paging and literal search. Only an exact
// in-scope reference may reach the native account reader. The list deliberately
// makes no account-state claim before the separate detail lookup.
func QueryUsers(ctx context.Context, workspace uuid.UUID, b Binding, request PageRequest, selected string) (UsersPage, error) {
	result, err := queryUsers(ctx, workspace, b, request, selected)
	if err != nil {
		return UsersPage{}, err
	}
	return result, nil
}
func queryUsers(ctx context.Context, workspace uuid.UUID, b Binding, request PageRequest, selected string) (UsersPage, error) {
	if workspace == uuid.Nil || b.Directory == nil || b.Catalog == nil || !IsIdentifier(b.OrganizationID) || !IsIdentifier(b.ProjectID) {
		return UsersPage{}, ErrUnconfigured
	}
	result := UsersPage{Items: []UserReference{}, Offset: request.Offset, Limit: 25, RoleDefinitions: b.Catalog.Roles()}
	request, err := request.Normalize()
	if err != nil || request.Filter.Role != "" || request.Filter.State != "" || request.Limit != 25 || selected != "" && !IsIdentifier(selected) {
		return result, ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	all := map[string][]Member{}
	total, previous := -1, ""
	for offset := 0; offset < 10000; offset += 100 {
		page, e := b.Directory.List(ctx, b.OrganizationID, PageRequest{Limit: 100, Offset: offset})
		if e != nil {
			return result, e
		}
		if page.Total < 0 || page.Total > 10000 || total >= 0 && page.Total != total || len(page.Items) != min(100, max(0, page.Total-offset)) {
			return result, ErrInvalidResponse
		}
		total = page.Total
		for _, m := range page.Items {
			if !validUserMember(m, b) || m.ID <= previous {
				return result, ErrInvalidResponse
			}
			previous = m.ID
			if group := all[m.UserID]; len(group) > 0 && (group[0].DisplayName != m.DisplayName || group[0].LoginName != m.LoginName) {
				return result, ErrInvalidResponse
			}
			all[m.UserID] = append(all[m.UserID], m)
		}
		if offset+len(page.Items) >= total {
			break
		}
	}
	refs := []UserReference{}
	for id, members := range all {
		m := members[0]
		if request.Filter.Matches(m) {
			refs = append(refs, UserReference{id, m.DisplayName, m.LoginName, len(members)})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	result.Total = len(refs)
	start := min(request.Offset, len(refs))
	result.Items = refs[start:min(start+25, len(refs))]
	if selected != "" {
		members, ok := all[selected]
		if !ok {
			return result, ErrNotFound
		}
		if b.Users == nil {
			return result, ErrUnconfigured
		}
		profile, e := b.Users.ReadUser(ctx, b.OrganizationID, selected)
		if e != nil {
			return result, e
		}
		if profile.ID != selected || profile.OrganizationID != b.OrganizationID || !validUserProfile(profile) {
			return result, ErrInvalidResponse
		}
		current := make([]Member, 0, len(members))
		for _, m := range members {
			fresh, e := b.Directory.Read(ctx, b.OrganizationID, m.ID)
			if e != nil {
				return result, e
			}
			if fresh.ID != m.ID || fresh.UserID != selected || !validUserMember(fresh, b) {
				return result, ErrInvalidResponse
			}
			fresh.Permissions, e = b.Catalog.Permissions(fresh.Roles)
			if e != nil {
				return result, e
			}
			current = append(current, fresh)
		}
		link := UserIdentityLink{State: "UNCONFIGURED", WorkspaceID: workspace}
		if b.IdentityLinks != nil {
			link, e = b.IdentityLinks.ReadUserIdentity(ctx, workspace, selected)
			if e != nil {
				return result, e
			}
		}
		if !validUserLink(link, workspace) {
			return result, ErrInvalidResponse
		}
		result.Selected = &UserDetail{profile, current, link}
	}
	if ctx.Err() != nil {
		return result, ErrUnavailable
	}
	return result, nil
}

func validUserMember(m Member, b Binding) bool {
	if !IsIdentifier(m.ID) || !IsIdentifier(m.UserID) || m.OrganizationID != b.OrganizationID || m.ProjectID != b.ProjectID || len(m.DisplayName) > 512 || len(m.LoginName) > 512 || len(m.Roles) > 32 || (m.State != "active" && m.State != "inactive") {
		return false
	}
	seen := map[string]bool{}
	for _, r := range m.Roles {
		if !IsIdentifier(r) || seen[r] {
			return false
		}
		seen[r] = true
	}
	for _, at := range []string{m.CreatedAt, m.ChangedAt} {
		if _, e := time.Parse(time.RFC3339Nano, at); e != nil {
			return false
		}
	}
	return true
}
func validUserProfile(p UserProfile) bool {
	if !IsIdentifier(p.ID) || !IsIdentifier(p.OrganizationID) || strings.TrimSpace(p.Username) == "" || len(p.Username) > 512 || len(p.DisplayName) > 512 {
		return false
	}
	switch p.AccountState {
	case "active", "inactive", "locked", "initial", "deleted":
	default:
		return false
	}
	for _, at := range []string{p.CreatedAt, p.ChangedAt} {
		if _, e := time.Parse(time.RFC3339Nano, at); e != nil {
			return false
		}
	}
	return true
}

var userLinkRevision = regexp.MustCompile(`^[1-9][0-9]*:[1-9][0-9]*:[1-9][0-9]*$`)

func validUserLink(l UserIdentityLink, ws uuid.UUID) bool {
	if l.WorkspaceID != ws {
		return false
	}
	if l.State == "UNCONFIGURED" || l.State == "UNLINKED" {
		return l.ActorID == nil && l.Revision == "" && l.BasisRef == "" && l.ExpiresAt == nil
	}
	if l.State != "LINKED" || l.ActorID == nil || l.ExpiresAt == nil || !l.ExpiresAt.After(time.Now()) || !userLinkRevision.MatchString(l.Revision) || strings.TrimSpace(l.BasisRef) == "" || len(l.BasisRef) > 200 {
		return false
	}
	id, e := uuid.Parse(*l.ActorID)
	return e == nil && id != uuid.Nil && id.String() == *l.ActorID
}
