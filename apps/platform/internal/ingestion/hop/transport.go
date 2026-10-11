package hop

import (
	"context"
	"net"
	"net/http"
	"time"
)

type observationKey struct{}

type observedConn struct {
	net.Conn
	observation *requestObservation
}

func (c observedConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if c.observation != nil {
		if n > 0 {
			c.observation.uncertain()
		}
		written := c.observation.networkBytes.Add(int64(n))
		expected := c.observation.expectedBytes.Load()
		// Standard Request.Write counted this exact HTTP/1 envelope before
		// dispatch. Trace callbacks alone cannot prove the final flush happened.
		if expected > 0 && written >= expected && n == len(p) && err == nil {
			c.observation.confirm()
		}
	}
	return n, err
}

type observedTransport struct{ base *http.Transport }

type byteCount int64

func (n *byteCount) Write(p []byte) (int, error) { *n += byteCount(len(p)); return len(p), nil }

func (t observedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Close = true
	if o, ok := r.Context().Value(observationKey{}).(*requestObservation); ok && r.URL.Scheme == "http" {
		// The fixed Hop client uses replayable byte bodies. Count with Go's
		// existing serializer, without retaining bytes or invoking trace hooks.
		copy := r.Clone(context.Background())
		if r.Body == nil || r.Body == http.NoBody {
			copy.Body = nil
		} else if r.GetBody != nil {
			body, err := r.GetBody()
			if err != nil {
				return nil, err
			}
			defer body.Close()
			copy.Body = body
		} else {
			copy = nil
		}
		if copy != nil {
			var n byteCount
			if err := copy.Write(&n); err != nil {
				return nil, err
			}
			o.expectedBytes.Store(int64(n))
		}
	}
	response, err := t.base.RoundTrip(r)
	// A real HTTP response is also proof of a provider invocation, including
	// rejected requests. It does not imply a successful ingestion operation.
	if response != nil {
		if o, ok := r.Context().Value(observationKey{}).(*requestObservation); ok {
			o.confirm()
		}
	}
	return response, err
}
func (t observedTransport) CloseIdleConnections() { t.base.CloseIdleConnections() }

// NewHTTPClient instruments Go's existing HTTP/1 transport; it does not
// implement an HTTP protocol. A fresh connection prevents implicit retries on
// a reused socket and gives each invocation its own network observation.
func NewHTTPClient(timeout time.Duration, transport *http.Transport) *http.Client {
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport)
	}
	base := transport.Clone()
	dial := base.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		connection, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		o, _ := ctx.Value(observationKey{}).(*requestObservation)
		return observedConn{Conn: connection, observation: o}, nil
	}
	base.DisableKeepAlives = true
	base.DisableCompression = true
	base.Proxy = nil
	base.ForceAttemptHTTP2 = false
	return &http.Client{Timeout: timeout, Transport: observedTransport{base: base}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
