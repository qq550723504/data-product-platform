// Reuses the existing shuomi-derived verified transport/permission adapter.
// The narrow organization lookup follows ZITADEL's Organization v2 API.
package zitadel

import (
	"context"
	"encoding/json"
	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement/zitadel/protojson"
	"net/http"
	"time"
)

type organizationReadRequest struct {
	Query struct {
		Offset string `json:"offset"`
		Limit  int    `json:"limit"`
		Asc    bool   `json:"asc"`
	} `json:"query"`
	SortingColumn string `json:"sortingColumn"`
	Queries       [1]struct {
		IDQuery struct {
			ID string `json:"id"`
		} `json:"idQuery"`
	} `json:"queries"`
}

func organizationRequest(id string) organizationReadRequest {
	var q organizationReadRequest
	q.Query.Offset = "0"
	q.Query.Limit = 1
	q.Query.Asc = true
	q.SortingColumn = "ORGANIZATION_FIELD_NAME_NAME"
	q.Queries[0].IDQuery.ID = id
	return q
}
func (c *Client) ReadOrganization(ctx context.Context, id string) (domain.DirectoryOrganization, error) {
	var empty domain.DirectoryOrganization
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.requirePermission(ctx, id, "org.read"); err != nil {
		return empty, err
	}
	raw, err := c.requestJSON(ctx, http.MethodPost, "/v2/organizations/_search", organizationRequest(id), false, id)
	if err != nil {
		return empty, err
	}
	var response struct {
		Details struct {
			Total *protojson.Uint64 `json:"totalResult"`
		} `json:"details"`
		Result []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			State   string `json:"state"`
			Details struct {
				Created string `json:"creationDate"`
				Changed string `json:"changeDate"`
			} `json:"details"`
		} `json:"result"`
	}
	if json.Unmarshal(raw, &response) != nil || response.Details.Total == nil {
		return empty, domain.ErrInvalidResponse
	}
	if err := c.requirePermission(ctx, id, "org.read"); err != nil {
		return empty, err
	}
	if ctx.Err() != nil {
		return empty, domain.ErrUnavailable
	}
	total := uint64(*response.Details.Total)
	if total == 0 && len(response.Result) == 0 {
		return empty, domain.ErrNotFound
	}
	if total != 1 || len(response.Result) != 1 || response.Result[0].ID != id {
		return empty, domain.ErrInvalidResponse
	}
	o := response.Result[0]
	state := map[string]string{"ORGANIZATION_STATE_ACTIVE": "active", "ORGANIZATION_STATE_INACTIVE": "inactive", "ORGANIZATION_STATE_REMOVED": "removed"}[o.State]
	optional := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	projected := domain.DirectoryOrganization{ID: o.ID, Name: optional(o.Name), State: state, CreatedAt: optional(o.Details.Created), ChangedAt: optional(o.Details.Changed)}
	if !domain.ValidDirectoryOrganization(projected) {
		return empty, domain.ErrInvalidResponse
	}
	return projected, nil
}
