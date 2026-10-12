package storage

import (
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"io"
	"net/http"
)

// Finite acceptance fault, not a retry policy. The native server receives one
// signed COPY; the response is discarded at the client transport boundary.
// The ordinary instrumented transport records invocation + UNKNOWN, never ACK.
type copyResponseLoss struct{ base http.RoundTripper }

func (t copyResponseLoss) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, e := t.base.RoundTrip(r)
	if e != nil {
		return nil, e
	}
	if r.Method == http.MethodPut && r.Header.Get("X-Amz-Copy-Source") != "" {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		return nil, lc.ErrStorageUnknown
	}
	return resp, nil
}
func NewLifecycleCopyResponseLossFixture(c LifecycleCopyConfig) (*LifecycleCopier, error) {
	if c.Endpoint != "127.0.0.1:9000" || c.Secure || len(c.Namespaces) != 2 || c.Namespaces[0] != "dpp-lifecycle307-online" || c.Namespaces[1] != "dpp-lifecycle307-quarantine" {
		return nil, lc.ErrStorageScope
	}
	s, e := NewLifecycleCopier(c)
	if e != nil {
		return nil, e
	}
	s.transport.base = copyResponseLoss{s.transport.base}
	return s, nil
}
