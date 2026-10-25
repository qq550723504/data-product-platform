package platformmanagement

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"strconv"
	"time"
)

type Handler struct {
	Principal principal.Resolver
	Gate      Gate
	Bindings  map[uuid.UUID]Binding
	Audit     AuditReader
	// Explicit provenance of the current identity in workspace-only projections.
	// Kept separate from native directory configuration and availability.
	IdentitySource string
}

func (h Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/workspaces/{workspaceId}/management/{resource}", h.ServeHTTP)
}
func failure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
func authorityFailure(w http.ResponseWriter, err error) {
	if errors.Is(err, principal.ErrUnauthenticated) || errors.Is(err, principal.ErrNotConfigured) {
		failure(w, 401, "AUTHENTICATION_REQUIRED")
	} else if errors.Is(err, principal.ErrWorkspaceDenied) || errors.Is(err, principal.ErrCapabilityDenied) {
		failure(w, 403, "MANAGEMENT_ACCESS_DENIED")
	} else {
		failure(w, 503, "IDENTITY_UNAVAILABLE")
	}
}
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != "GET" {
		failure(w, 405, "READ_ONLY")
		return
	}
	ws, err := uuid.Parse(r.PathValue("workspaceId"))
	if err != nil || ws == uuid.Nil || r.ContentLength > 0 || len(r.TransferEncoding) > 0 || r.Header.Get("Cookie") != "" || len(r.Header.Values("Authorization")) != 1 {
		failure(w, 400, "INVALID_QUERY")
		return
	}
	resource := r.PathValue("resource")
	if resource != "members" && resource != "roles" && resource != "audit" && resource != "users" && resource != "workspace" && resource != "organization" && resource != "configuration" {
		failure(w, 404, "NOT_FOUND")
		return
	}
	allowed := map[string]bool{}
	if resource == "members" {
		for _, k := range []string{"offset", "q", "role", "state", "memberId"} {
			allowed[k] = true
		}
	}
	if resource == "audit" {
		allowed["cursor"] = true
	}
	if resource == "users" {
		for _, k := range []string{"offset", "q", "userId"} {
			allowed[k] = true
		}
	}
	q, err := strictQuery(r, allowed)
	if err != nil {
		failure(w, 400, "INVALID_QUERY")
		return
	}
	request := PageRequest{Limit: 25}
	if q["offset"] != "" {
		request.Offset, err = strconv.Atoi(q["offset"])
		if err != nil || strconv.Itoa(request.Offset) != q["offset"] {
			failure(w, 400, "INVALID_QUERY")
			return
		}
	}
	request.Filter = ListFilter{q["q"], q["role"], q["state"]}
	request, err = request.Normalize()
	if err != nil || q["memberId"] != "" && !IsIdentifier(q["memberId"]) || q["userId"] != "" && !IsIdentifier(q["userId"]) {
		failure(w, 400, "INVALID_QUERY")
		return
	}
	if h.Principal == nil || h.Gate == nil {
		failure(w, 503, "IDENTITY_UNAVAILABLE")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	p, err := h.Principal.Resolve(r, ws, principal.CapabilityManagementRead)
	if err != nil {
		authorityFailure(w, err)
		return
	}
	if p.WorkspaceID != ws {
		authorityFailure(w, principal.ErrWorkspaceDenied)
		return
	}
	if _, err = h.Gate.Confirm(ctx, p); err != nil {
		authorityFailure(w, err)
		return
	}
	b, ok := h.Bindings[ws]
	if resource == "workspace" || resource == "configuration" {
		if h.IdentitySource != "SYNTHETIC_FIXTURE" && h.IdentitySource != "CONFIGURED" {
			failure(w, 503, "SOURCE_UNCONFIGURED")
			return
		}
	} else if !ok || b.Directory == nil || b.Catalog == nil || !IsIdentifier(b.OrganizationID) || !IsIdentifier(b.ProjectID) || (b.Source != "SYNTHETIC_FIXTURE" && b.Source != "CONFIGURED") {
		failure(w, 503, "SOURCE_UNCONFIGURED")
		return
	}
	var payload any
	switch resource {
	case "organization":
		page, e := QueryOrganization(ctx, b)
		if e != nil {
			if errors.Is(e, ErrUnconfigured) {
				failure(w, 503, "SOURCE_UNCONFIGURED")
			} else if errors.Is(e, ErrNotFound) {
				failure(w, 404, "ORGANIZATION_UNAVAILABLE")
			} else {
				failure(w, 503, "ORGANIZATION_DIRECTORY_UNAVAILABLE")
			}
			return
		}
		payload = page
	case "users":
		if b.Users == nil {
			failure(w, 503, "SOURCE_UNCONFIGURED")
			return
		}
		page, e := QueryUsers(ctx, ws, b, request, q["userId"])
		if e != nil {
			if errors.Is(e, ErrNotFound) {
				failure(w, 404, "USER_UNAVAILABLE")
			} else if errors.Is(e, ErrInvalidRequest) {
				failure(w, 400, "INVALID_QUERY")
			} else if errors.Is(e, ErrUnconfigured) {
				failure(w, 503, "SOURCE_UNCONFIGURED")
			} else {
				failure(w, 503, "USER_DIRECTORY_UNAVAILABLE")
			}
			return
		}
		payload = page
	case "members":
		if request.Filter.Role != "" && !b.Catalog.Has(request.Filter.Role) {
			failure(w, 400, "INVALID_QUERY")
			return
		}
		page, e := b.Directory.List(ctx, b.OrganizationID, request)
		if e != nil {
			failure(w, 503, "DIRECTORY_UNAVAILABLE")
			return
		}
		if page.Total < 0 || page.Total > 10000 || len(page.Items) != min(25, max(0, page.Total-request.Offset)) {
			failure(w, 503, "INVALID_SOURCE_RESPONSE")
			return
		}
		seen := map[string]bool{}
		for i := range page.Items {
			m := &page.Items[i]
			if !IsIdentifier(m.ID) || !IsIdentifier(m.UserID) || seen[m.ID] || !request.Filter.Matches(*m) || m.OrganizationID != b.OrganizationID || m.ProjectID != b.ProjectID {
				failure(w, 503, "INVALID_SOURCE_RESPONSE")
				return
			}
			seen[m.ID] = true
			m.Permissions, e = b.Catalog.Permissions(m.Roles)
			if e != nil {
				failure(w, 503, "INVALID_SOURCE_RESPONSE")
				return
			}
		}
		var selected *Member
		if id := q["memberId"]; id != "" {
			m, e := b.Directory.Read(ctx, b.OrganizationID, id)
			if errors.Is(e, ErrNotFound) {
				failure(w, 404, "MEMBER_UNAVAILABLE")
				return
			}
			if e != nil || m.ID != id || m.OrganizationID != b.OrganizationID || m.ProjectID != b.ProjectID {
				failure(w, 503, "DIRECTORY_UNAVAILABLE")
				return
			}
			m.Permissions, e = b.Catalog.Permissions(m.Roles)
			if e != nil {
				failure(w, 503, "INVALID_SOURCE_RESPONSE")
				return
			}
			selected = &m
		}
		payload = struct {
			Items           []Member `json:"items"`
			Total           int      `json:"total"`
			Offset          int      `json:"offset"`
			Limit           int      `json:"limit"`
			Selected        *Member  `json:"selected"`
			RoleDefinitions []Role   `json:"roleDefinitions"`
		}{page.Items, page.Total, request.Offset, 25, selected, b.Catalog.Roles()}
	case "roles":
		payload = struct {
			Items        []Role `json:"items"`
			PolicySource string `json:"policySource"`
		}{b.Catalog.Roles(), "APPLICATION_CATALOG"}
	case "audit":
		if h.Audit == nil {
			failure(w, 503, "AUDIT_UNAVAILABLE")
			return
		}
		page, e := h.Audit.List(ctx, ws, p.ActorID, q["cursor"])
		if e != nil {
			if errors.Is(e, ErrInvalidRequest) {
				failure(w, 400, "INVALID_QUERY")
			} else {
				failure(w, 503, "AUDIT_UNAVAILABLE")
			}
			return
		}
		payload = page
	}
	// A local grant check cannot detect IdP token revocation during directory I/O.
	// Reuse the uncached resolver, then validate the original authority revision;
	// a newly granted identity must never legitimize data fetched under an old one.
	fresh, err := h.Principal.Resolve(r, ws, principal.CapabilityManagementRead)
	if err != nil {
		authorityFailure(w, err)
		return
	}
	if fresh.ActorID != p.ActorID || fresh.Subject != p.Subject || fresh.WorkspaceID != p.WorkspaceID || fresh.IdentityIssuer != p.IdentityIssuer || fresh.IdentitySubject != p.IdentitySubject || fresh.IdentityRevision != p.IdentityRevision {
		authorityFailure(w, principal.ErrUnauthenticated)
		return
	}
	if fresh.IdentityExpiresAt.Before(p.IdentityExpiresAt) {
		p.IdentityExpiresAt = fresh.IdentityExpiresAt
	}
	a, err := h.Gate.Confirm(ctx, p)
	if err != nil {
		authorityFailure(w, err)
		return
	}
	if ctx.Err() != nil {
		failure(w, 503, "READ_UNAVAILABLE")
		return
	}
	env := Envelope{"platform-management.v1", ws, p.ActorID, a.Principal.IdentityRevision, a.EvaluatedAt, b.Source, false}
	if resource == "workspace" {
		payload, err = workspaceOverview(b, h.Audit, a)
		if err != nil {
			failure(w, 503, "READ_UNAVAILABLE")
			return
		}
		env.Source = h.IdentitySource
	}
	if resource == "configuration" {
		payload, err = configurationOverview(b, h.Audit, a)
		if err != nil {
			failure(w, 503, "READ_UNAVAILABLE")
			return
		}
		env.Source = h.IdentitySource
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Envelope
		Data any `json:"data"`
	}{env, payload})
}
func strictQuery(r *http.Request, allowed map[string]bool) (map[string]string, error) {
	values, err := urlValues(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, v := range values {
		if !allowed[k] || len(v) != 1 || v[0] == "" {
			return nil, ErrInvalidRequest
		}
		out[k] = v[0]
	}
	return out, nil
}
