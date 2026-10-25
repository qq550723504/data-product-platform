// Read-only extraction of the source adapter's requestJSON transport; no write paths.
package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	domain "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
	"io"
	"net/http"
)

func (c *Client) requestJSON(ctx context.Context, method, path string, body any, allowNotFound bool, organization ...string) ([]byte, error) {
	if c == nil || c.http == nil || method != http.MethodPost || path != "/auth/v1/permissions/zitadel/me/_search" || allowNotFound || len(organization) != 1 || !domain.IsIdentifier(organization[0]) {
		return nil, domain.ErrInvalidRequest
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, domain.ErrInvalidRequest
	}
	req, err := http.NewRequestWithContext(ctx, method, c.origin+path, bytes.NewReader(raw))
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
	if res.StatusCode != 200 {
		return nil, domain.ErrUnavailable
	}
	result, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil || len(result) > maxResponseBytes {
		return nil, domain.ErrInvalidResponse
	}
	return result, nil
}
