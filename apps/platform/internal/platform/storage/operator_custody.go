package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

// OperatorCustodyAdapter is never wired into Core object readers. Its private
// operator credentials may be broader than its exact per-dispatch wire guard.
type OperatorCustodyAdapter struct {
	plan      lc.CustodyPlan
	creds     *credentials.Credentials
	transport http.RoundTripper // Private contract-test seam; CLI cannot set it.
}

func NewOperatorCustodyAdapter(plan lc.CustodyPlan, creds *credentials.Credentials) (*OperatorCustodyAdapter, error) {
	if plan.Validate() != nil || creds == nil {
		return nil, lc.ErrCustodyScope
	}
	return &OperatorCustodyAdapter{plan: plan, creds: creds}, nil
}

func (a *OperatorCustodyAdapter) ReadCustody(ctx context.Context, member int, target lc.CustodyTarget, journal lc.CustodyJournal) ([]byte, error) {
	if journal == nil || !time.Now().Before(a.plan.ExpiresAt) || (member != 1 && member != 2) ||
		(member == 1 && target != a.plan.First) || (member == 2 && target != a.plan.Second) {
		return nil, lc.ErrCustodyScope
	}
	base := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, ForceAttemptHTTP2: false, ResponseHeaderTimeout: 15 * time.Second}
	defer base.CloseIdleConnections()
	wire := &custodyWire{base: base, endpoint: a.plan.Endpoint, path: "/raw/" + target.Key(), journal: journal, member: member, limit: target.Bytes + 1, expiresAt: a.plan.ExpiresAt}
	if a.transport != nil {
		wire.base = a.transport
	}
	client, err := minio.New(a.plan.Endpoint, &minio.Options{Creds: a.creds, Secure: false, Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: wire})
	if err != nil {
		return nil, lc.ErrCustodyScope
	}
	// Core.GetObject returns a synchronous response body. The higher-level lazy
	// Object reader can hide asynchronous Close errors and introduce Stat/Seek.
	core := minio.Core{Client: client}
	body, _, _, getErr := core.GetObject(ctx, "raw", target.Key(), minio.GetObjectOptions{})
	var data []byte
	if getErr == nil {
		data, err = io.ReadAll(io.LimitReader(body, target.Bytes+1))
		getErr = errors.Join(err, body.Close())
	}
	if wire.body != nil {
		getErr = errors.Join(getErr, wire.body.Close())
	}
	if wire.callErr != nil {
		return nil, lc.ErrCustodyScope
	} // Start failed: zero dispatches.
	if !wire.dispatched {
		return nil, lc.ErrCustodyScope
	}
	wire.mu.Lock()
	o := wire.observation
	wire.mu.Unlock()
	o.ObservedAt = time.Now().UTC()
	if wire.body != nil {
		o.Bytes = wire.body.bytes
		o.EOF = wire.body.eof
		o.CloseOK = wire.body.closeErr == nil && wire.body.closed
	}
	if o.HTTPStatus != 0 {
		o.InvocationCertainty = "CONFIRMED"
	}
	if getErr != nil || wire.body == nil {
		o.Failure = "TRANSPORT"
	} else if o.HTTPStatus != 200 {
		o.Failure = "HTTP"
	} else if !o.EOF || !o.CloseOK {
		o.Failure = "BODY"
	} else {
		hash := sha256.Sum256(data)
		o.SHA256 = hex.EncodeToString(hash[:])
		if int64(len(data)) != target.Bytes || o.SHA256 != target.SHA256 {
			o.Failure = "CONTENT"
		}
	}
	// Record a bounded error/redirect as a failed invocation too. The SDK may
	// have consumed its response internally; only content success returns bytes.
	if o.HTTPStatus != 0 && o.HTTPStatus != 200 {
		o.Failure = "HTTP"
	}
	observeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err = journal.ObserveCustodyCall(observeCtx, o); err != nil {
		return nil, lc.ErrCustodyInterrupted
	}
	if o.Failure != "" || getErr != nil {
		return nil, lc.ErrCustodyInterrupted
	}
	return data, nil
}

type custodyWire struct {
	base              http.RoundTripper
	endpoint, path    string
	journal           lc.CustodyJournal
	member            int
	limit             int64
	spent, dispatched bool
	callErr           error
	body              *custodyBody
	mu                sync.Mutex
	observation       lc.CustodyObservation
	expiresAt         time.Time
}

func (w *custodyWire) RoundTrip(req *http.Request) (*http.Response, error) {
	if w.spent || req.Method != "GET" || req.URL.Scheme != "http" || req.URL.Host != w.endpoint || req.Host != w.endpoint ||
		req.URL.Path != w.path || req.URL.RawPath != "" || req.URL.RawQuery != "" || req.URL.Fragment != "" || req.URL.User != nil ||
		req.Header.Get("Range") != "" || req.Header.Get("If-Match") != "" || req.Header.Get("If-None-Match") != "" {
		return nil, lc.ErrCustodyScope
	}
	w.spent = true
	id, err := w.journal.StartCustodyCall(req.Context(), w.member)
	if err != nil {
		w.callErr = err
		return nil, lc.ErrCustodyScope
	}
	if !w.expiresAt.IsZero() && !time.Now().Before(w.expiresAt) {
		return nil, lc.ErrCustodyScope // Committed start stays UNKNOWN; no dispatch.
	}
	w.observation = lc.CustodyObservation{CallID: id, InvokedAt: time.Now().UTC(), InvocationCertainty: "UNKNOWN"}
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			w.mu.Lock()
			w.observation.InvocationCertainty = "CONFIRMED"
			w.mu.Unlock()
		}
	}}
	w.dispatched = true
	resp, err := w.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil {
		return nil, lc.ErrCustodyInterrupted
	}
	w.observation.HTTPStatus = resp.StatusCode
	limit := w.limit
	if resp.StatusCode != 200 {
		limit = 8192
	}
	w.body = &custodyBody{ReadCloser: resp.Body, remaining: limit}
	resp.Body = w.body
	return resp, nil
}

type custodyBody struct {
	io.ReadCloser
	remaining, bytes int64
	eof, closed      bool
	closeErr         error
}

func (b *custodyBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, lc.ErrCustodyScope
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.bytes += int64(n)
	b.remaining -= int64(n)
	if err == io.EOF {
		b.eof = true
	}
	return n, err
}
func (b *custodyBody) Close() error {
	if !b.closed {
		b.closed = true
		b.closeErr = b.ReadCloser.Close()
	}
	return b.closeErr
}
