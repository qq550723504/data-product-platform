package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/signer"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"
)

type LifecycleCopyConfig struct {
	Endpoint, AccessKey, SecretKey                 string
	Secure                                         bool
	WorkspaceID, ProviderInstanceID, IncarnationID uuid.UUID
	Namespaces                                     []string
	Journal                                        lc.StorageCallJournal
}
type LifecycleCopier struct {
	client    *minio.Client
	transport *copyTransport
	config    LifecycleCopyConfig
}
type copyCallKey struct{}
type custodyCall struct {
	op          domain.CopyOperation
	copy        domain.Copy
	operation   string
	id          uuid.UUID
	mu          sync.Mutex
	sent        bool
	observation lc.StorageCallObservation
	readerPin   *domain.ReaderPin
	stoppedAt   time.Time
}

func (c *custodyCall) result() lc.StorageCallObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observation
}

type copyTransport struct {
	base     http.RoundTripper
	endpoint *url.URL
	journal  lc.StorageCallJournal
}

func NewLifecycleCopier(c LifecycleCopyConfig) (*LifecycleCopier, error) {
	if c.Journal == nil || c.AccessKey == "" || c.SecretKey == "" || c.WorkspaceID == uuid.Nil || c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil || len(c.Namespaces) != 2 {
		return nil, lc.ErrStorageScope
	}
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}
	u, e := url.Parse(scheme + "://" + c.Endpoint)
	if e != nil || u.Host == "" || u.Path != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, lc.ErrStorageScope
	}
	t := &copyTransport{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 64 << 10}, endpoint: u, journal: c.Journal}
	client, e := minio.New(c.Endpoint, &minio.Options{Secure: c.Secure, Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: t})
	if e != nil {
		return nil, lc.ErrStorageScope
	}
	return &LifecycleCopier{client, t, c}, nil
}
func (s *LifecycleCopier) allowed(o domain.CopyOperation) bool {
	if o.Validate() != nil || o.WorkspaceID != s.config.WorkspaceID || o.Source.ProviderInstanceID != s.config.ProviderInstanceID || o.Source.IncarnationID != s.config.IncarnationID || o.Source.ByteSize > 128<<10 {
		return false
	}
	src, dst := false, false
	for _, n := range s.config.Namespaces {
		src = src || o.Source.Namespace == n
		dst = dst || o.TargetNamespace == n
	}
	return src && dst
}
func (s *LifecycleCopier) CreateCopy(ctx context.Context, o domain.CopyOperation) (domain.Copy, uuid.UUID, error) {
	if !s.allowed(o) {
		return domain.Copy{}, uuid.Nil, lc.ErrStorageScope
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	call := &custodyCall{op: o, copy: o.Source, operation: o.Action + "_COPY", id: uuid.New()}
	ctx = context.WithValue(ctx, copyCallKey{}, call)
	info, e := s.client.CopyObject(ctx, minio.CopyDestOptions{Bucket: o.TargetNamespace, Object: o.TargetKey, ReplaceMetadata: true, UserMetadata: map[string]string{"lifecycle-operation": o.ID.String()}}, minio.CopySrcOptions{Bucket: o.Source.Namespace, Object: o.Source.Key, VersionID: o.Source.ProviderVersion})
	if e != nil {
		return domain.Copy{}, call.id, lc.ErrStorageUnknown
	}
	target := o.Materialized(info.VersionID)
	if target.Validate() != nil {
		return domain.Copy{}, call.id, lc.ErrStorageContract
	}
	return target, call.id, nil
}
func (s *LifecycleCopier) VerifyCopyBytes(ctx context.Context, o domain.CopyOperation, c domain.Copy) (lc.CopyBytesProof, error) {
	if !s.allowed(o) || c.Validate() != nil || c != o.Materialized(c.ProviderVersion) {
		return lc.CopyBytesProof{}, lc.ErrStorageScope
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	call := &custodyCall{op: o, copy: c, operation: lc.StorageVerifyCopy, id: uuid.New()}
	ctx = context.WithValue(ctx, copyCallKey{}, call)
	u := *s.transport.endpoint
	u.Path = "/" + c.Namespace + "/" + c.Key
	u.RawQuery = url.Values{"versionId": []string{c.ProviderVersion}}.Encode()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return lc.CopyBytesProof{}, lc.ErrStorageScope
	}
	empty := sha256.Sum256(nil)
	req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(empty[:]))
	req = signer.SignV4(*req, s.config.AccessKey, s.config.SecretKey, "", "us-east-1")
	client := &http.Client{Transport: s.transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("custody redirect unsupported") }}
	resp, e := client.Do(req)
	if e != nil {
		return lc.CopyBytesProof{}, lc.ErrStorageUnknown
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return lc.CopyBytesProof{}, lc.ErrStorageDenied
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, c.ByteSize+1))
	if e != nil || int64(len(b)) != c.ByteSize {
		return lc.CopyBytesProof{}, lc.ErrStorageContract
	}
	sum := sha256.Sum256(b)
	hash := hex.EncodeToString(sum[:])
	if hash != c.ContentSHA256 {
		return lc.CopyBytesProof{}, lc.ErrStorageContract
	}
	return lc.CopyBytesProof{Copy: c, CallID: call.id, SHA256: hash, Bytes: int64(len(b)), ObservedAt: time.Now().UTC()}, nil
}
func (t *copyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c, ok := r.Context().Value(copyCallKey{}).(*custodyCall)
	if !ok {
		return nil, lc.ErrStorageScope
	}
	c.mu.Lock()
	if c.sent {
		c.mu.Unlock()
		return nil, lc.ErrIdempotencyConflict
	}
	c.sent = true
	c.mu.Unlock()
	if r.URL.Scheme != t.endpoint.Scheme || r.URL.Host != t.endpoint.Host || r.URL.User != nil || r.Header.Get("X-Amz-Bypass-Governance-Retention") != "" || r.Header.Get("X-Amz-Content-Sha256") == "STREAMING-UNSIGNED-PAYLOAD-TRAILER" {
		return nil, lc.ErrStorageScope
	}
	if c.operation == lc.StorageRecoverCopyVersions {
		q := r.URL.Query()
		if r.Method != "GET" || r.URL.Path != "/"+c.op.TargetNamespace+"/" || r.Header.Get("X-Amz-Copy-Source") != "" || !q.Has("versions") || q.Get("versions") != "" || q.Get("prefix") != "" || q.Get("delimiter") != "" || q.Get("key-marker") != "" || q.Get("version-id-marker") != "" {
			return nil, lc.ErrStorageScope
		}
		for key, values := range q {
			if len(values) != 1 || (key != "versions" && key != "prefix" && key != "delimiter" && key != "max-keys" && key != "encoding-type") {
				return nil, lc.ErrStorageScope
			}
		}
	} else if c.operation == lc.StorageRecoverCopyExact {
		q := r.URL.Query()
		if r.Method != "HEAD" || r.URL.Path != "/"+c.op.TargetNamespace+"/"+c.op.TargetKey || len(q) != 1 || len(q["versionId"]) != 1 || q.Get("versionId") != c.copy.ProviderVersion || r.Header.Get("X-Amz-Copy-Source") != "" {
			return nil, lc.ErrStorageScope
		}
	} else if c.operation == lc.StorageVerifyCopy || c.operation == lc.StorageReadPinned {
		q := r.URL.Query()
		if r.Method != "GET" || r.URL.Path != "/"+c.copy.Namespace+"/"+c.copy.Key || len(q) != 1 || len(q["versionId"]) != 1 || q.Get("versionId") != c.copy.ProviderVersion || r.Header.Get("X-Amz-Copy-Source") != "" {
			return nil, lc.ErrStorageScope
		}
	} else {
		if c.operation != lc.StorageArchiveCopy && c.operation != lc.StorageRestoreCopy {
			return nil, lc.ErrStorageUnsupported
		}
		source, e := url.Parse(r.Header.Get("X-Amz-Copy-Source"))
		if e != nil || source.Host != "" || source.Fragment != "" || strings.TrimPrefix(source.Path, "/") != c.op.Source.Namespace+"/"+c.op.Source.Key || source.Query().Get("versionId") != c.op.Source.ProviderVersion || len(source.Query()) != 1 || len(source.Query()["versionId"]) != 1 || r.Method != "PUT" || r.URL.Path != "/"+c.op.TargetNamespace+"/"+c.op.TargetKey || r.URL.RawQuery != "" || r.Header.Get("X-Amz-Meta-Lifecycle-Operation") != c.op.ID.String() {
			return nil, lc.ErrStorageScope
		}
	}
	started := time.Now().UTC()
	journalCopy := c.copy
	if c.operation == lc.StorageRecoverCopyVersions || c.operation == lc.StorageRecoverCopyExact {
		journalCopy = c.op.Source
	}
	start := lc.StorageCallStart{ID: c.id, Copy: journalCopy, Operation: c.operation, StartedAt: started, CopyOperationID: c.op.ID, CopyIntent: &c.op}
	if c.operation == lc.StorageReadPinned {
		start.CopyIntent = nil
		start.ReaderPin = c.readerPin
	}
	if e := t.journal.StartStorageCall(r.Context(), start); e != nil {
		if c.operation == lc.StorageReadPinned {
			c.mu.Lock()
			c.stoppedAt = time.Now().UTC()
			c.mu.Unlock()
		} // no native I/O started
		return nil, e
	}
	var invoked *time.Time
	trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { at := time.Now().UTC(); invoked = &at }}
	r = r.Clone(httptrace.WithClientTrace(r.Context(), trace))
	resp, e := t.base.RoundTrip(r)
	proof := lc.StorageCallObservation{CallID: c.id, ObservedAt: time.Now().UTC(), InvokedAt: invoked}
	if e != nil {
		proof.Failure = "TRANSPORT"
	} else {
		if proof.InvokedAt == nil {
			at := time.Now().UTC()
			proof.InvokedAt = &at
		}
		proof.HTTPStatus = resp.StatusCode
		proof.RequestRef = resp.Header.Get("X-Amz-Request-Id")
		limit := int64(64 << 10)
		if (c.operation == lc.StorageVerifyCopy || c.operation == lc.StorageReadPinned) && resp.StatusCode == http.StatusOK {
			limit = c.copy.ByteSize
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		closeErr := resp.Body.Close()
		if c.operation == lc.StorageReadPinned && closeErr == nil {
			c.mu.Lock()
			c.stoppedAt = time.Now().UTC()
			c.mu.Unlock()
		}
		sum := sha256.Sum256(b)
		proof.ResponseSHA256 = hex.EncodeToString(sum[:])
		proof.ObservedAt = time.Now().UTC()
		resp.Body = io.NopCloser(bytes.NewReader(b))
		if readErr != nil || closeErr != nil || int64(len(b)) > limit {
			proof.Failure = "CONTRACT"
			e = lc.ErrStorageContract
		}
		if resp.StatusCode == 200 && c.operation == lc.StorageRecoverCopyVersions {
			if validateFixtureInventory(b, c.op.TargetNamespace) != nil {
				proof.Failure = "CONTRACT"
				e = lc.ErrStorageContract
			}
		} else if resp.StatusCode == 200 {
			version := resp.Header.Get("X-Amz-Version-Id")
			if c.operation == lc.StorageRecoverCopyExact {
				proof.CopyOperationTag, _ = uuid.Parse(resp.Header.Get("X-Amz-Meta-Lifecycle-Operation"))
				size := resp.ContentLength
				proof.CopyResultBytes = &size
				sum := sha256.Sum256([]byte(version + "\n" + proof.CopyOperationTag.String() + "\n" + fmt.Sprint(size)))
				proof.ResponseSHA256 = hex.EncodeToString(sum[:])
				if version != c.copy.ProviderVersion || size != c.copy.ByteSize || proof.CopyOperationTag != c.op.ID {
					proof.Failure = "CONTRACT"
					e = lc.ErrStorageContract
				}
			}
			if c.operation != lc.StorageVerifyCopy && c.operation != lc.StorageReadPinned {
				proof.CopyResultVersion = version
			}
			if version == "" || version == "null" || resp.Header.Get("X-Amz-Delete-Marker") == "true" || ((c.operation == lc.StorageVerifyCopy || c.operation == lc.StorageReadPinned) && (version != c.copy.ProviderVersion || int64(len(b)) != c.copy.ByteSize || proof.ResponseSHA256 != c.copy.ContentSHA256)) {
				proof.Failure = "CONTRACT"
				e = lc.ErrStorageContract
			}
		}
	}
	proof.ObservedAt = time.Now().UTC()
	c.mu.Lock()
	c.observation = proof
	c.mu.Unlock()
	proofContext, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	if journalErr := t.journal.ObserveStorageCall(proofContext, proof); journalErr != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, lc.ErrStorageUnknown
	}
	if e != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, lc.ErrStorageUnknown
	}
	return resp, nil
}
