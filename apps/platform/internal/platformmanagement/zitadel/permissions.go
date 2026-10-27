// Adapted read-only from shuomi-ai-commerce@e29058c56df4294e1c612b3a705ab767bc333188; see docs/architecture/platform-management-readonly-v1.md.
package zitadel

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

// The native permission API retains project/grant context as a colon suffix.
// Only a bare permission in the explicitly selected organization proves this
// dedicated org credential can read the complete directory. Never strip suffixes.
func (c *Client) requirePermission(ctx context.Context, organization, permission string) error {
	if !domain.IsIdentifier(organization) {
		return domain.ErrInvalidRequest
	}
	raw, err := c.requestJSON(ctx, http.MethodPost, "/auth/v1/permissions/zitadel/me/_search", nil, false, organization)
	if err != nil {
		return domain.ErrUnavailable
	}
	var result struct {
		Permissions []string `json:"result"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Permissions) > 1024 || !slices.Contains(result.Permissions, permission) {
		return domain.ErrUnavailable
	}
	return nil
}
