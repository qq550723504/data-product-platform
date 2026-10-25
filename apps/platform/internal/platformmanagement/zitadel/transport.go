// Read-only extraction of the source adapter's requestJSON transport; no write paths.
package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
	"io"
	"net/http"
	"strings"
)

func (c *Client) requestJSON(ctx context.Context, method, path string, body any, allowNotFound bool, organization ...string) ([]byte, error) {
	permissionQuery := method == http.MethodPost && path == "/auth/v1/permissions/zitadel/me/_search" && !allowNotFound
	userQuery := method == http.MethodGet && strings.HasPrefix(path, "/v2/users/") && domain.IsIdentifier(strings.TrimPrefix(path, "/v2/users/")) && allowNotFound && body == nil
	if c == nil || c.http == nil || (!permissionQuery && !userQuery) || len(organization) != 1 || !domain.IsIdentifier(organization[0]) {
		return nil, domain.ErrInvalidRequest
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, domain.ErrInvalidRequest
	}
	var payload io.Reader = bytes.NewReader(raw)
	if userQuery {
		payload = nil
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, payload)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-zitadel-orgid", organization[0])
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return nil, domain.ErrUnavailable
	}
	defer res.Body.Close()
	if userQuery && res.StatusCode == 404 {
		return nil, domain.ErrNotFound
	}
	if res.StatusCode != 200 {
		return nil, domain.ErrUnavailable
	}
	result, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(result) > maxResponseBytes {
		return nil, domain.ErrInvalidResponse
	}
	return result, nil
}
