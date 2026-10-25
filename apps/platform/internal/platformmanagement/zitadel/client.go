// Adapted read-only from shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188; see docs/architecture/platform-management-readonly-v1.md.
package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement/zitadel/protojson"
)

const maxResponseBytes = 1 << 20

// Client uses a dedicated read-only credential. Caller authentication and
// organization authorization belong to the current application boundary.
type Client struct {
	origin, token, project string
	http                   *http.Client
}

func (c *Client) Read(ctx context.Context, organization, id string) (domain.Member, error) {
	if !domain.IsIdentifier(id) {
		return domain.Member{}, domain.ErrInvalidRequest
	}
	page, err := c.list(ctx, organization, domain.PageRequest{Limit: 1}, id)
	if err != nil {
		return domain.Member{}, err
	}
	if page.Total == 0 {
		return domain.Member{}, domain.ErrNotFound
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != id {
		return domain.Member{}, domain.ErrInvalidResponse
	}
	return page.Items[0], nil
}

func NewClient(origin, token, project string, client *http.Client) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") || !domain.IsIdentifier(project) {
		return nil, domain.ErrUnavailable
	}
	transport := http.DefaultTransport
	if client != nil && client.Transport != nil {
		transport = client.Transport
	}
	// Do not inherit cookie jars or credential-bearing redirect behavior.
	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Client{origin: strings.TrimRight(origin, "/"), token: token, project: project, http: httpClient}, nil
}

type assignment struct {
	ID      string `json:"id"`
	Created string `json:"creationDate"`
	Changed string `json:"changeDate"`
	Project struct {
		ID string `json:"id"`
	} `json:"project"`
	Organization struct {
		ID string `json:"id"`
	} `json:"organization"`
	User struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		LoginName   string `json:"preferredLoginName"`
	} `json:"user"`
	State string `json:"state"`
	Roles []struct {
		Key string `json:"key"`
	} `json:"roles"`
}

func (c *Client) list(ctx context.Context, organization string, page domain.PageRequest, assignmentID string, extra ...any) (domain.Page, error) {
	if c == nil || c.http == nil {
		return domain.Page{}, domain.ErrUnavailable
	}
	if !domain.IsIdentifier(organization) || page.Limit < 1 || page.Limit > 100 || page.Offset < 0 || page.Offset > 10000 {
		return domain.Page{}, domain.ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.requirePermission(ctx, organization, "user.grant.read"); err != nil {
		return domain.Page{}, err
	}
	filters := []any{map[string]any{"organizationId": map[string]string{"id": organization}}, map[string]any{"projectId": map[string]string{"id": c.project}}}
	filters = append(filters, extra...)
	if assignmentID != "" {
		filters = append(filters, map[string]any{"authorizationIds": map[string][]string{"ids": {assignmentID}}})
	}
	body, err := json.Marshal(map[string]any{
		"pagination":    map[string]any{"offset": page.Offset, "limit": page.Limit, "asc": true},
		"sortingColumn": "AUTHORIZATION_FIELD_NAME_ID",
		"filters":       filters,
	})
	if err != nil {
		return domain.Page{}, domain.ErrInvalidRequest
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.origin+"/zitadel.authorization.v2.AuthorizationService/ListAuthorizations", bytes.NewReader(body))
	if err != nil {
		return domain.Page{}, domain.ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")
	response, err := c.http.Do(request)
	if err != nil {
		return domain.Page{}, domain.ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return domain.Page{}, domain.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if ctx.Err() != nil {
		return domain.Page{}, domain.ErrUnavailable
	}
	if err != nil || len(raw) > maxResponseBytes {
		return domain.Page{}, domain.ErrInvalidResponse
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil {
		return domain.Page{}, domain.ErrInvalidResponse
	}
	var pagination map[string]json.RawMessage
	if json.Unmarshal(envelope["pagination"], &pagination) != nil || pagination == nil {
		return domain.Page{}, domain.ErrInvalidResponse
	}
	var total protojson.Uint64
	if value, present := pagination["totalResult"]; present {
		if string(value) == "null" || json.Unmarshal(value, &total) != nil {
			return domain.Page{}, domain.ErrInvalidResponse
		}
	}
	if uint64(total) > 10000 {
		return domain.Page{}, domain.ErrInvalidResponse
	}
	var assignments []assignment
	if value, present := envelope["authorizations"]; present {
		if string(value) == "null" || json.Unmarshal(value, &assignments) != nil {
			return domain.Page{}, domain.ErrInvalidResponse
		}
	} else if total != 0 {
		return domain.Page{}, domain.ErrInvalidResponse
	}
	if len(assignments) > page.Limit || len(assignments) > int(total) || (len(assignments) == 0 && page.Offset < int(total)) || (len(assignments) > 0 && page.Offset+len(assignments) > int(total)) {
		return domain.Page{}, domain.ErrInvalidResponse
	}
	result := domain.Page{Items: make([]domain.Member, 0, len(assignments)), Total: int(total)}
	seen := make(map[string]bool)
	for _, item := range assignments {
		if !domain.IsIdentifier(item.ID) || !domain.IsIdentifier(item.User.ID) || item.Organization.ID != organization || item.Project.ID != c.project || seen[item.ID] || len(item.User.DisplayName) > 512 || len(item.User.LoginName) > 512 || len(item.Roles) > 32 {
			return domain.Page{}, domain.ErrInvalidResponse
		}
		seen[item.ID] = true
		state := ""
		switch item.State {
		case "STATE_ACTIVE":
			state = "active"
		case "STATE_INACTIVE":
			state = "inactive"
		default:
			return domain.Page{}, domain.ErrInvalidResponse
		}
		for _, date := range []string{item.Created, item.Changed} {
			if _, err := time.Parse(time.RFC3339Nano, date); err != nil {
				return domain.Page{}, domain.ErrInvalidResponse
			}
		}
		roles := make([]string, 0, len(item.Roles))
		seenRoles := make(map[string]bool)
		for _, role := range item.Roles {
			if strings.TrimSpace(role.Key) == "" || len(role.Key) > 128 || seenRoles[role.Key] {
				return domain.Page{}, domain.ErrInvalidResponse
			}
			seenRoles[role.Key] = true
			roles = append(roles, role.Key)
		}
		result.Items = append(result.Items, domain.Member{ID: item.ID, UserID: item.User.ID, OrganizationID: organization, ProjectID: c.project, DisplayName: item.User.DisplayName, LoginName: item.User.LoginName, State: state, Roles: roles, CreatedAt: item.Created, ChangedAt: item.Changed})
	}
	if ctx.Err() != nil {
		return domain.Page{}, domain.ErrUnavailable
	}
	if err := c.requirePermission(ctx, organization, "user.grant.read"); err != nil {
		return domain.Page{}, err
	}
	return result, nil
}
