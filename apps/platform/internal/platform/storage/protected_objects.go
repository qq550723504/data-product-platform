package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

// RuntimeStore is the existing application's object I/O shape. Provisioning is
// intentionally absent: a scoped runtime cannot list/create a bucket.
type RuntimeStore interface {
	Put(context.Context, string, io.Reader, int64, string) (string, error)
	Get(context.Context, string) (io.ReadCloser, error)
	Bucket() string
}

type ProtectedAdapter struct{ store *Store }
type protectedIntentKey struct{}

type protectedResultKey struct{}
type protectedResult struct {
	mu  sync.Mutex
	err error
}

func (s *protectedResult) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = errors.Join(s.err, err)
}
func (s *protectedResult) failure() error { s.mu.Lock(); defer s.mu.Unlock(); return s.err }

func NewProtectedAdapter(endpoint, bucket string, secure bool, creds *credentials.Credentials, j lc.ProtectedObjectJournal) (*ProtectedAdapter, error) {
	if j == nil || creds == nil {
		return nil, lc.ErrStorageScope
	}
	transport := &protectedTransport{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, ResponseHeaderTimeout: 15 * time.Second}, endpoint: endpoint, bucket: bucket, journal: j}
	client, err := minio.New(endpoint, &minio.Options{Creds: creds, Secure: secure, Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: transport})
	if err != nil {
		return nil, lc.ErrStorageScope
	}
	return &ProtectedAdapter{store: &Store{client: client, bucket: bucket}}, nil
}

func (p *ProtectedAdapter) CreateProtected(ctx context.Context, i lc.ProtectedObjectIntent, data []byte) error {
	h := sha256.Sum256(data)
	if int64(len(data)) != i.ByteSize || hex.EncodeToString(h[:]) != i.SHA256 {
		return lc.ErrStorageContract
	}
	ctx = context.WithValue(ctx, protectedIntentKey{}, i)
	state := &protectedResult{}
	ctx = context.WithValue(ctx, protectedResultKey{}, state)
	_, err := p.store.PutImmutable(ctx, i.Key, bytes.NewReader(data), i.ByteSize, i.ContentType)
	if journalErr := state.failure(); journalErr != nil {
		return errors.Join(lc.ErrProtectedJournal, journalErr)
	}
	if errors.Is(err, ErrObjectExists) {
		return nil
	}
	return protectedError(err)
}

func (p *ProtectedAdapter) ReadProtected(ctx context.Context, i lc.ProtectedObjectIntent) ([]byte, error) {
	ctx = context.WithValue(ctx, protectedIntentKey{}, i)
	state := &protectedResult{}
	ctx = context.WithValue(ctx, protectedResultKey{}, state)
	r, err := p.store.Get(ctx, "s3://"+p.store.bucket+"/"+i.Key)
	if journalErr := state.failure(); journalErr != nil {
		if r != nil {
			_ = r.Close()
		}
		return nil, errors.Join(lc.ErrProtectedJournal, journalErr)
	}
	if err != nil {
		return nil, protectedError(err)
	}
	data, err := io.ReadAll(io.LimitReader(r, i.ByteSize+1))
	closeErr := r.Close()
	if journalErr := state.failure(); journalErr != nil {
		return nil, errors.Join(lc.ErrProtectedJournal, journalErr)
	}
	if err != nil || closeErr != nil {
		return nil, errors.Join(protectedError(err), closeErr)
	}
	if int64(len(data)) != i.ByteSize {
		return nil, lc.ErrStorageContract
	}
	return data, nil
}

func protectedError(err error) error {
	if err == nil {
		return nil
	}
	var response minio.ErrorResponse
	if errors.As(err, &response) {
		if response.StatusCode == 403 {
			return lc.ErrStorageDenied
		}
		if response.StatusCode == 404 {
			return lc.ErrStorageNotFound
		}
	}
	return errors.Join(lc.ErrStorageUnknown, err)
}

type protectedTransport struct {
	base             http.RoundTripper
	endpoint, bucket string
	journal          lc.ProtectedObjectJournal
}

func (t *protectedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	i, ok := req.Context().Value(protectedIntentKey{}).(lc.ProtectedObjectIntent)
	if !ok || req.URL.Host != t.endpoint || req.URL.Path != "/"+t.bucket+"/"+i.Key || req.URL.RawQuery != "" ||
		(req.Method != "PUT" && req.Method != "GET" && req.Method != "HEAD") ||
		(req.Method == "PUT" && req.Header.Get("If-None-Match") != "*") {
		return nil, lc.ErrStorageScope
	}
	id, err := t.journal.StartProtectedCall(req.Context(), i, req.Method)
	if err != nil {
		return nil, errors.Join(lc.ErrProtectedJournal, err)
	}
	observation := lc.StorageCallObservation{CallID: id}
	var mu sync.Mutex
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err != nil {
			return
		} // partial/uncertain sends retain their start fact
		mu.Lock()
		defer mu.Unlock()
		if observation.InvokedAt == nil {
			at := time.Now().UTC()
			observation.InvokedAt = &at
		}
	}}
	response, err := t.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	mu.Lock()
	observed := observation
	mu.Unlock()
	if err != nil {
		observed.ObservedAt = time.Now().UTC()
		observed.Failure = "TRANSPORT"
		if journalErr := recordProtectedObservation(req.Context(), t.journal, observed); journalErr != nil {
			return nil, errors.Join(err, lc.ErrProtectedJournal, journalErr)
		}
		return nil, err
	}
	observed.HTTPStatus = response.StatusCode
	if observed.InvokedAt == nil {
		at := time.Now().UTC()
		observed.InvokedAt = &at
	}
	ref := response.Header.Get("X-Amz-Request-Id")
	if len(ref) <= 256 && !strings.ContainsAny(ref, "\x00\r\n") {
		observed.RequestRef = ref
	}
	state, _ := req.Context().Value(protectedResultKey{}).(*protectedResult)
	response.Body = &protectedBody{ReadCloser: response.Body, journal: t.journal, ctx: req.Context(), observation: observed, digest: sha256.New(), method: req.Method, result: state}
	return response, nil
}

func recordProtectedObservation(ctx context.Context, j lc.ProtectedObjectJournal, o lc.StorageCallObservation) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	return j.ObserveProtectedCall(ctx, o)
}

type protectedBody struct {
	io.ReadCloser
	journal     lc.ProtectedObjectJournal
	ctx         context.Context
	observation lc.StorageCallObservation
	digest      hash.Hash
	method      string
	eof         bool
	closed      bool
	result      *protectedResult
	finishOnce  sync.Once
	finishErr   error
}

func (b *protectedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		_, _ = b.digest.Write(p[:n])
	}
	if err == io.EOF {
		b.eof = true
		if e := b.finish(nil); e != nil {
			return n, errors.Join(lc.ErrProtectedJournal, e)
		}
	}
	return n, err
}
func (b *protectedBody) Close() error {
	if b.closed {
		return nil
	}
	b.closed = true
	err := b.ReadCloser.Close()
	return errors.Join(err, b.finish(err))
}

// MinIO's asynchronous Object reader can deliver EOF before its deferred Close
// runs. Persist the body proof at EOF, before callers can publish the version.
func (b *protectedBody) finish(readErr error) error {
	b.finishOnce.Do(func() {
		b.observation.ObservedAt = time.Now().UTC()
		if b.eof || b.method == "HEAD" {
			b.observation.ResponseSHA256 = hex.EncodeToString(b.digest.Sum(nil))
		}
		if readErr != nil || (b.method == "GET" && !b.eof) {
			b.observation.Failure = "TRANSPORT"
		}
		b.finishErr = recordProtectedObservation(b.ctx, b.journal, b.observation)
		if b.result != nil {
			b.result.record(b.finishErr)
		}
	})
	return b.finishErr
}
