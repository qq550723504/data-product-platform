package reference

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"io"
	"net/http"
)

func (c *Client) Read(ctx context.Context, q d.UsageReadRequest, i ports.Invocation) (d.UsagePage, error) {
	var page d.UsagePage
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if i.ID == uuid.Nil || i.Dispatch == nil || q.Limit != d.UsagePageLimit {
		return page, d.ErrInvalid
	}
	body, err := json.Marshal(q)
	if err != nil {
		return page, d.ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/usage/read", bytes.NewReader(body))
	if err != nil {
		return page, d.ErrInvalid
	}
	req.Header.Set("Content-Type", "application/json")
	if err = i.Dispatch(ctx); err != nil {
		return page, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return page, d.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return page, d.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024+1))
	if err != nil || len(raw) > 128*1024 {
		return page, d.ErrUnavailable
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&page) != nil {
		return d.UsagePage{}, d.ErrUnavailable
	}
	var tail any
	if dec.Decode(&tail) != io.EOF {
		return d.UsagePage{}, d.ErrUnavailable
	}
	return page.Normalize(q)
}
