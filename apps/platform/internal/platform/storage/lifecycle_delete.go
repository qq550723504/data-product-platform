package storage

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lifecycle "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Allowlist contains complete registered identities from the approved frozen
// plan. Never accepts a key/URI from an HTTP request or a storage root identity.
type LifecycleDeleteConfig struct {
	Endpoint             string
	Secure               bool
	AccessKey, SecretKey string
	Targets              []domain.Copy
	Journal              lifecycle.ExactDeletionJournal
}
type LifecycleDeleter struct {
	client    *minio.Client
	targets   map[string]domain.Copy
	transport *deleteTransport
}
type deleteContextKey struct{}
type deleteCall struct {
	attempt domain.ProviderAttempt
	copy    domain.Copy
	mu      sync.Mutex
	sent    bool
}
type deleteTransport struct {
	base     http.RoundTripper
	endpoint *url.URL
	journal  lifecycle.ExactDeletionJournal
}

func NewLifecycleDeleter(c LifecycleDeleteConfig) (*LifecycleDeleter, error) {
	if c.Journal == nil || c.AccessKey == "" || c.SecretKey == "" || len(c.Targets) == 0 || len(c.Targets) > 64 {
		return nil, lifecycle.ErrStorageScope
	}
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}
	endpoint, err := url.Parse(scheme + "://" + c.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Path != "" || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, lifecycle.ErrStorageScope
	}
	targets := map[string]domain.Copy{}
	for _, target := range c.Targets {
		if target.Validate() != nil || targets[target.ID.String()].ID == target.ID {
			return nil, lifecycle.ErrStorageScope
		}
		targets[target.ID.String()] = target
	}
	base := &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	transport := &deleteTransport{base: base, endpoint: endpoint, journal: c.Journal}
	client, err := minio.New(c.Endpoint, &minio.Options{Secure: c.Secure, Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: transport})
	if err != nil {
		return nil, lifecycle.ErrStorageScope
	}
	return &LifecycleDeleter{client: client, targets: targets, transport: transport}, nil
}
func (s *LifecycleDeleter) DeleteExactCopy(ctx context.Context, a domain.ProviderAttempt, c domain.Copy) error {
	if s == nil || a.Validate() != nil || c.Validate() != nil || a.CopyID != c.ID || a.WorkspaceID != c.WorkspaceID || s.targets[c.ID.String()] != c {
		return lifecycle.ErrStorageScope
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, deleteContextKey{}, &deleteCall{attempt: a, copy: c})
	// No bare-key remove, governance bypass, batch DELETE or SDK retry.
	err := s.client.RemoveObject(ctx, c.Namespace, c.Key, minio.RemoveObjectOptions{VersionID: c.ProviderVersion})
	if err != nil {
		return inspectionError(err)
	}
	return nil
}
func validDeleteRequest(r *http.Request, endpoint *url.URL, c *deleteCall) bool {
	if r == nil || c == nil || endpoint == nil || r.URL.Scheme != endpoint.Scheme || r.URL.Host != endpoint.Host || r.URL.User != nil || r.URL.Fragment != "" || r.Method != http.MethodDelete || r.URL.Path != "/"+c.copy.Namespace+"/"+c.copy.Key || r.Header.Get("X-Amz-Bypass-Governance-Retention") != "" || r.Header.Get("X-Amz-Copy-Source") != "" || r.Header.Get("X-Amz-Content-Sha256") == "STREAMING-UNSIGNED-PAYLOAD-TRAILER" {
		return false
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	return err == nil && len(q) == 1 && len(q["versionId"]) == 1 && q.Get("versionId") == c.copy.ProviderVersion
}
func (t *deleteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c, ok := r.Context().Value(deleteContextKey{}).(*deleteCall)
	if !ok || !validDeleteRequest(r, t.endpoint, c) {
		return nil, lifecycle.ErrStorageScope
	}
	c.mu.Lock()
	if c.sent {
		c.mu.Unlock()
		return nil, lifecycle.ErrStorageUnsupported
	}
	c.sent = true
	c.mu.Unlock()
	if err := t.journal.ClaimExactSend(r.Context(), c.attempt, c.copy); err != nil {
		return nil, err
	}
	var mu sync.Mutex
	var invoked *time.Time
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			mu.Lock()
			at := time.Now().UTC().Truncate(time.Microsecond)
			invoked = &at
			mu.Unlock()
		}
	}}
	response, err := t.base.RoundTrip(r.Clone(httptrace.WithClientTrace(r.Context(), trace)))
	mu.Lock()
	invocation := invoked
	mu.Unlock()
	status := 0
	var diagnostic *ExactDeleteResponse
	outcome := domain.ProviderUnknown
	if response != nil {
		status = response.StatusCode
		if invocation == nil {
			at := time.Now().UTC().Truncate(time.Microsecond)
			invocation = &at
		}
		body, bodyErr := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
		response.Body.Close()
		// Persist only closed, non-secret diagnostics; never provider error text.
		detail := InspectExactDeleteResponse(status, body, bodyErr == nil && len(body) <= 64<<10)
		diagnostic = &detail
		if bodyErr != nil || len(body) > 64<<10 {
			err = lifecycle.ErrStorageContract
		}
		if status == 204 && len(body) == 0 && response.Header.Get("X-Amz-Delete-Marker") != "true" && (response.Header.Get("X-Amz-Version-Id") == "" || response.Header.Get("X-Amz-Version-Id") == c.copy.ProviderVersion) && err == nil {
			outcome = domain.ProviderAccepted
		} else if status == 401 || status == 403 {
			outcome = domain.ProviderDenied
			err = lifecycle.ErrStorageDenied
		} else if err == nil {
			outcome = domain.ProviderFailed
			err = lifecycle.ErrStorageUnknown
		}
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	proof := domain.ProviderObservation{ID: uuid.New(), WorkspaceID: c.attempt.WorkspaceID, AttemptID: c.attempt.ID, DispatchID: c.attempt.DispatchID, CopyID: c.copy.ID, Certainty: domain.InvocationUncertain, Outcome: domain.ProviderUnknown, ObservedAt: at}
	if invocation != nil {
		proof.Certainty = domain.InvocationConfirmed
		proof.InvokedAt = invocation
		proof.Outcome = outcome
	}
	proof.ProofSHA256 = lifecycle.ObservationHash(struct {
		Attempt  string
		Status   int
		Outcome  string
		Observed time.Time
		Response *ExactDeleteResponse
	}{c.attempt.ID.String(), status, proof.Outcome, at, diagnostic})
	proofCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	journalErr := t.journal.ObserveExactSend(proofCtx, proof)
	cancel()
	if journalErr != nil {
		err = lifecycle.ErrStorageUnknown
	}
	if err != nil {
		if diagnostic != nil {
			return nil, &ExactDeleteFailure{Response: *diagnostic, cause: err}
		}
		return nil, err
	}
	if response == nil {
		return nil, errors.New("exact delete response unknown")
	}
	response.Body = http.NoBody
	return response, nil
}
