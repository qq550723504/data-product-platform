// Read-only adaptation of shuomi-ai-commerce@e29058c's ReadHuman. No writer,
// invitation or provisioning path is imported. Account state is native User v2.
package zitadel

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func (c *Client) ReadUser(ctx context.Context, organization, id string) (domain.UserProfile, error) {
	var empty domain.UserProfile
	if !domain.IsIdentifier(id) {
		return empty, domain.ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.requirePermission(ctx, organization, "user.read"); err != nil {
		return empty, err
	}
	raw, err := c.requestJSON(ctx, http.MethodGet, "/v2/users/"+id, nil, true, organization)
	if err != nil {
		return empty, err
	}
	var response struct {
		User *struct {
			ID       string `json:"userId"`
			Username string `json:"username"`
			State    string `json:"state"`
			Details  struct {
				Owner   string `json:"resourceOwner"`
				Created string `json:"creationDate"`
				Changed string `json:"changeDate"`
			} `json:"details"`
			Machine json.RawMessage `json:"machine"`
			Human   *struct {
				Profile *struct {
					DisplayName string `json:"displayName"`
				} `json:"profile"`
			} `json:"human"`
		} `json:"user"`
	}
	if json.Unmarshal(raw, &response) != nil || response.User == nil {
		return empty, domain.ErrInvalidResponse
	}
	u := response.User
	if u.ID != id || u.Details.Owner != organization || u.Human == nil || u.Human.Profile == nil || len(u.Machine) != 0 || len(u.Username) == 0 || len(u.Username) > 512 || len(u.Human.Profile.DisplayName) > 512 {
		return empty, domain.ErrInvalidResponse
	}
	state := map[string]string{"USER_STATE_ACTIVE": "active", "USER_STATE_INACTIVE": "inactive", "USER_STATE_LOCKED": "locked", "USER_STATE_INITIAL": "initial", "USER_STATE_DELETED": "deleted"}[u.State]
	if state == "" {
		return empty, domain.ErrInvalidResponse
	}
	for _, date := range []string{u.Details.Created, u.Details.Changed} {
		if _, e := time.Parse(time.RFC3339Nano, date); e != nil {
			return empty, domain.ErrInvalidResponse
		}
	}
	if err := c.requirePermission(ctx, organization, "user.read"); err != nil {
		return empty, err
	}
	if ctx.Err() != nil {
		return empty, domain.ErrUnavailable
	}
	return domain.UserProfile{ID: u.ID, OrganizationID: organization, Username: u.Username, DisplayName: u.Human.Profile.DisplayName, AccountState: state, CreatedAt: u.Details.Created, ChangedAt: u.Details.Changed}, nil
}
