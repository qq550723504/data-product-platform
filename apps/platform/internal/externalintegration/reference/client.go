// Package reference implements an explicitly configured, synthetic loopback protocol.
// It is not an NDI adapter and never releases data or credentials.
package reference

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"io"
	"net/http"
	"net/url"
	"time"
)

const Revision = "reference-http-v1"

type Client struct {
	base string
	http *http.Client
}

func New(raw string) (*Client, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, fmt.Errorf("reference adapter requires an explicit HTTP 127.0.0.1:port origin")
	}
	tr := &http.Transport{DisableKeepAlives: true, ForceAttemptHTTP2: false}
	return &Client{base: u.String(), http: &http.Client{Transport: tr, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) invoke(ctx context.Context, path string, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	var out d.ProviderResult
	body, err := json.Marshal(struct {
		Request d.Request `json:"request"`
		Hash    string    `json:"requestHash"`
	}{q, q.Hash()})
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	if i.ID == [16]byte{} || i.Dispatch == nil {
		return out, d.ErrInvalid
	}
	if err = i.Dispatch(ctx); err != nil {
		return out, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return out, d.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return out, d.ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024+1))
	if err != nil || len(raw) > 16*1024 {
		return out, d.ErrUnavailable
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(&out) != nil {
		return d.ProviderResult{}, d.ErrUnavailable
	}
	var tail any
	if dec.Decode(&tail) != io.EOF {
		return d.ProviderResult{}, d.ErrUnavailable
	}
	return out, nil
}
func (c *Client) CheckIdentity(ctx context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return c.invoke(ctx, "/identity/check", q, i)
}
func (c *Client) CheckIdentifier(ctx context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return c.invoke(ctx, "/identifier/check", q, i)
}
func (c *Client) Submit(ctx context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return c.invoke(ctx, "/catalog/submit", q, i)
}
func (c *Client) Poll(ctx context.Context, q d.Request, i ports.Invocation) (d.ProviderResult, error) {
	return c.invoke(ctx, "/catalog/poll", q, i)
}
func (c *Client) PrepareDelivery(context.Context, d.Request, ports.Invocation) error {
	return d.ErrUnsupported
}
func (c *Client) Adapter(provider, instance string) ports.Adapter {
	// Changing the endpoint is a different adapter identity even if a deployment
	// reuses its display name. Recovery must never query another provider origin.
	hash := sha256.Sum256([]byte(Revision + "\n" + c.base))
	return ports.Adapter{Descriptor: ports.Descriptor{Provider: provider, Instance: instance, Revision: hex.EncodeToString(hash[:]), Capabilities: ports.Capabilities{IdentityVerification: true, IdentifierVerification: true, CatalogPublication: true, StableRequestKeys: true, AuthoritativeLookup: true}}, Identity: c, Identifier: c, Catalog: c, Delivery: c}
}
