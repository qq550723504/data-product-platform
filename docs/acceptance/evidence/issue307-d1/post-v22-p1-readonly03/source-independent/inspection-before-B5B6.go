package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/s3utils"
	lifecycle "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// LifecycleInspectionConfig is server-owned deployment configuration. This
// read-only adapter grants neither deletion nor a reader path to object bytes.
type LifecycleInspectionConfig struct {
	Endpoint                                       string
	Secure                                         bool
	AccessKey, SecretKey                           string
	WorkspaceID, ProviderInstanceID, IncarnationID uuid.UUID
	Namespaces                                     []string
	Journal                                        lifecycle.StorageCallJournal
}

type LifecycleInspector struct {
	client    *minio.Client
	config    LifecycleInspectionConfig
	allowed   map[string]bool
	transport *inspectionTransport
}

var _ lifecycle.StorageInspector = (*LifecycleInspector)(nil)

func NewLifecycleInspector(c LifecycleInspectionConfig) (*LifecycleInspector, error) {
	if c.WorkspaceID == uuid.Nil || c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil ||
		c.Journal == nil || c.AccessKey == "" || c.SecretKey == "" || len(c.Namespaces) != 2 {
		return nil, lifecycle.ErrStorageScope
	}
	scheme := "http"
	if c.Secure {
		scheme = "https"
	}
	u, err := url.Parse(scheme + "://" + c.Endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, lifecycle.ErrStorageScope
	}
	allowed := make(map[string]bool, 2)
	for _, bucket := range c.Namespaces {
		if s3utils.CheckValidBucketName(bucket) != nil || allowed[bucket] {
			return nil, lifecycle.ErrStorageScope
		}
		allowed[bucket] = true
	}
	// Disable reused-connection retries in net/http as well as SDK retries. Do
	// not inherit a proxy, HTTP/2 multiplexing or arbitrary injected transport.
	base := &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false,
		DialContext:     (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	transport := &inspectionTransport{base: base, journal: c.Journal, endpoint: u, now: func() time.Time { return time.Now().UTC() }}
	client, err := minio.New(c.Endpoint, &minio.Options{Secure: c.Secure,
		Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Region: "us-east-1",
		BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: transport})
	if err != nil {
		return nil, lifecycle.ErrStorageScope
	}
	c.Namespaces = append([]string(nil), c.Namespaces...)
	return &LifecycleInspector{client: client, config: c, allowed: allowed, transport: transport}, nil
}

func (s *LifecycleInspector) validate(c domain.Copy) error {
	if s == nil || s.client == nil || c.Validate() != nil || c.WorkspaceID != s.config.WorkspaceID ||
		c.ProviderInstanceID != s.config.ProviderInstanceID || c.IncarnationID != s.config.IncarnationID || !s.allowed[c.Namespace] {
		return lifecycle.ErrStorageScope
	}
	return nil
}

type inspectionCall struct {
	copy        domain.Copy
	operation   string
	mu          sync.Mutex
	started     bool
	observation lifecycle.StorageCallObservation
}
type inspectionContextKey struct{}

func (s *LifecycleInspector) callContext(ctx context.Context, c domain.Copy, op string) (context.Context, context.CancelFunc, *inspectionCall) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	call := &inspectionCall{copy: c, operation: op}
	return context.WithValue(ctx, inspectionContextKey{}, call), cancel, call
}
func (c *inspectionCall) result() lifecycle.StorageCallObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.observation
}

func (s *LifecycleInspector) InspectExactCopy(ctx context.Context, c domain.Copy) (lifecycle.ExactCopyInspection, error) {
	if err := s.validate(c); err != nil {
		return lifecycle.ExactCopyInspection{}, err
	}
	ctx, cancel, call := s.callContext(ctx, c, lifecycle.StorageStatExact)
	defer cancel()
	info, err := s.client.StatObject(ctx, c.Namespace, c.Key, minio.StatObjectOptions{VersionID: c.ProviderVersion})
	if err != nil {
		return lifecycle.ExactCopyInspection{}, inspectionError(err)
	}
	if info.VersionID != c.ProviderVersion || info.IsDeleteMarker || info.Size != c.ByteSize {
		return lifecycle.ExactCopyInspection{}, lifecycle.ErrStorageContract
	}
	obs := call.result()
	return lifecycle.ExactCopyInspection{Copy: c, ObservedAt: obs.ObservedAt, CallID: obs.CallID}, nil
}

func (s *LifecycleInspector) InspectRetentionAndHold(ctx context.Context, c domain.Copy) (lifecycle.CopyProtection, error) {
	if err := s.validate(c); err != nil {
		return lifecycle.CopyProtection{}, err
	}
	rctx, rcancel, retentionCall := s.callContext(ctx, c, lifecycle.StorageRetention)
	mode, until, err := s.client.GetObjectRetention(rctx, c.Namespace, c.Key, c.ProviderVersion)
	rcancel()
	if err != nil {
		// Pinned MinIO GetObjectRetention checks bucket lock and exact-version
		// existence first, then returns 400/NoSuchObjectLockConfiguration when
		// that object's retention mode is absent (not an empty 200 envelope).
		// The transport must have validated the exact error envelope/resource.
		if !nativeProtectionAbsent(err, retentionCall) {
			return lifecycle.CopyProtection{}, inspectionError(err)
		}
		empty := minio.RetentionMode("")
		mode = &empty
		until = nil
	}
	if mode == nil {
		return lifecycle.CopyProtection{}, lifecycle.ErrStorageContract
	}
	retentionMode := string(*mode)
	switch retentionMode {
	case "":
		if until != nil {
			return lifecycle.CopyProtection{}, lifecycle.ErrStorageContract
		}
		retentionMode = "NONE"
	case "GOVERNANCE", "COMPLIANCE":
		if until == nil || until.IsZero() {
			return lifecycle.CopyProtection{}, lifecycle.ErrStorageContract
		}
	default:
		return lifecycle.CopyProtection{}, lifecycle.ErrStorageContract
	}
	hctx, hcancel, holdCall := s.callContext(ctx, c, lifecycle.StorageLegalHold)
	hold, err := s.client.GetObjectLegalHold(hctx, c.Namespace, c.Key, minio.GetObjectLegalHoldOptions{VersionID: c.ProviderVersion})
	hcancel()
	if err != nil {
		// The same pinned handler returns this exact error for missing hold
		// metadata after verifying the bucket and requested object version.
		if !nativeProtectionAbsent(err, holdCall) {
			return lifecycle.CopyProtection{}, inspectionError(err)
		}
		off := minio.LegalHoldDisabled
		hold = &off
	}
	if hold == nil || (*hold != minio.LegalHoldEnabled && *hold != minio.LegalHoldDisabled) {
		return lifecycle.CopyProtection{}, lifecycle.ErrStorageContract
	}
	r, h := retentionCall.result(), holdCall.result()
	return lifecycle.CopyProtection{Copy: c, RetentionMode: retentionMode, RetainUntil: until,
		LegalHold: *hold == minio.LegalHoldEnabled, ObservedAt: h.ObservedAt, CallIDs: []uuid.UUID{r.CallID, h.CallID}}, nil
}

func nativeProtectionAbsent(err error, call *inspectionCall) bool {
	native, obs := minio.ToErrorResponse(err), call.result()
	return native.StatusCode == http.StatusBadRequest && native.Code == "NoSuchObjectLockConfiguration" && obs.ResponseSHA256 != "" && obs.Failure == ""
}

func (s *LifecycleInspector) InspectNamespace(ctx context.Context, c domain.Copy) (lifecycle.NamespaceInventory, error) {
	if err := s.validate(c); err != nil {
		return lifecycle.NamespaceInventory{}, err
	}
	ctx, cancel, call := s.callContext(ctx, c, lifecycle.StorageListVersions)
	defer cancel()
	var versions []lifecycle.ListedVersion
	seen := make(map[string]bool)
	var contractErr error
	for info := range s.client.ListObjects(ctx, c.Namespace, minio.ListObjectsOptions{WithVersions: true, Recursive: true, MaxKeys: 1000}) {
		if info.Err != nil {
			contractErr = inspectionError(info.Err)
			continue
		}
		identity := info.Key + "\x00" + info.VersionID
		if info.Key == "" || info.VersionID == "" || info.VersionID == "null" || seen[identity] || info.Size < 0 {
			contractErr = lifecycle.ErrStorageContract
			continue
		}
		seen[identity] = true
		versions = append(versions, lifecycle.ListedVersion{Key: info.Key, Version: info.VersionID, ByteSize: info.Size, DeleteMarker: info.IsDeleteMarker})
	}
	if contractErr != nil {
		return lifecycle.NamespaceInventory{}, contractErr
	}
	if ctx.Err() != nil {
		return lifecycle.NamespaceInventory{}, lifecycle.ErrStorageUnknown
	}
	obs := call.result()
	if obs.CallID == uuid.Nil || obs.HTTPStatus != http.StatusOK || obs.ResponseSHA256 == "" {
		return lifecycle.NamespaceInventory{}, lifecycle.ErrStorageUnknown
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Key != versions[j].Key {
			return versions[i].Key < versions[j].Key
		}
		return versions[i].Version < versions[j].Version
	})
	return lifecycle.NamespaceInventory{WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID,
		IncarnationID: c.IncarnationID, Namespace: c.Namespace, Versions: versions, ObservedAt: obs.ObservedAt,
		CallID: obs.CallID, ResponseSHA256: obs.ResponseSHA256}, nil
}

func inspectionError(err error) error {
	for _, typed := range []error{lifecycle.ErrStorageScope, lifecycle.ErrStorageDenied, lifecycle.ErrStorageNotFound,
		lifecycle.ErrStorageContract, lifecycle.ErrStorageUnsupported, lifecycle.ErrStorageUnknown} {
		if errors.Is(err, typed) {
			return typed
		}
	}
	r := minio.ToErrorResponse(err)
	switch r.StatusCode {
	case http.StatusForbidden, http.StatusUnauthorized:
		return lifecycle.ErrStorageDenied
	case http.StatusNotFound:
		return lifecycle.ErrStorageNotFound
	case http.StatusNotImplemented:
		return lifecycle.ErrStorageUnsupported
	default:
		return lifecycle.ErrStorageUnknown
	}
}

type inspectionTransport struct {
	base     http.RoundTripper
	journal  lifecycle.StorageCallJournal
	endpoint *url.URL
	now      func() time.Time
}

func (t *inspectionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	call, ok := req.Context().Value(inspectionContextKey{}).(*inspectionCall)
	if !ok || !validInspectionRequest(req, t.endpoint, call) {
		return nil, lifecycle.ErrStorageScope
	}
	call.mu.Lock()
	if call.started {
		call.mu.Unlock()
		return nil, lifecycle.ErrStorageUnsupported
	}
	call.started = true
	call.mu.Unlock()
	start := lifecycle.StorageCallStart{ID: uuid.New(), Copy: call.copy, Operation: call.operation, StartedAt: t.now()}
	if err := t.journal.StartStorageCall(req.Context(), start); err != nil {
		return nil, lifecycle.ErrStorageUnknown
	}
	var mu sync.Mutex
	var invokedAt *time.Time
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			mu.Lock()
			at := t.now()
			invokedAt = &at
			mu.Unlock()
		}
	}}
	resp, err := t.base.RoundTrip(req.Clone(httptrace.WithClientTrace(req.Context(), trace)))
	mu.Lock()
	invoked := invokedAt
	mu.Unlock()
	obs := lifecycle.StorageCallObservation{CallID: start.ID, InvokedAt: invoked}
	if err != nil {
		obs.Failure = "TRANSPORT"
	}
	if resp != nil {
		if invoked == nil {
			at := t.now()
			obs.InvokedAt = &at
		}
		obs.HTTPStatus = resp.StatusCode
		ref := resp.Header.Get("X-Amz-Request-Id")
		if len(ref) <= 256 && !strings.ContainsAny(ref, "\x00\r\n") {
			obs.RequestRef = ref
		}
		if resp.StatusCode == http.StatusPartialContent {
			err = lifecycle.ErrStorageContract
			obs.Failure = "CONTRACT"
		}
		if resp.StatusCode == http.StatusOK {
			if echoed := resp.Header.Get("X-Amz-Version-Id"); echoed != "" && echoed != call.copy.ProviderVersion && call.operation != lifecycle.StorageListVersions {
				err = lifecycle.ErrStorageContract
			}
			if call.operation == lifecycle.StorageStatExact {
				if resp.Header.Get("X-Amz-Delete-Marker") == "true" {
					err = lifecycle.ErrStorageContract
				}
			} else {
				var body []byte
				var bodyErr error
				body, bodyErr = boundedInspectionBody(resp.Body, call.operation)
				if err == nil {
					err = bodyErr
				}
				if err == nil && call.operation == lifecycle.StorageListVersions {
					err = validateFixtureInventory(body, call.copy.Namespace)
				}
				if err == nil && (call.operation == lifecycle.StorageRetention || call.operation == lifecycle.StorageLegalHold) {
					err = validateProtectionEnvelope(body, call.operation)
				}
				if err == nil {
					sum := sha256.Sum256(body)
					obs.ResponseSHA256 = hex.EncodeToString(sum[:])
				}
				resp.Body = io.NopCloser(bytes.NewReader(body))
			}
			if err != nil {
				obs.Failure = "CONTRACT"
			}
		} else if req.Method != http.MethodHead {
			body, bodyErr := boundedInspectionBody(resp.Body, call.operation)
			resp.Body = io.NopCloser(bytes.NewReader(body))
			if bodyErr == nil && (call.operation == lifecycle.StorageRetention || call.operation == lifecycle.StorageLegalHold) && resp.StatusCode == http.StatusBadRequest {
				var envelope struct {
					Code []string `xml:"Code"`
				}
				if xml.Unmarshal(body, &envelope) == nil && len(envelope.Code) > 0 && envelope.Code[0] == "NoSuchObjectLockConfiguration" {
					bodyErr = validateAbsentObjectRetention(body, call.copy)
					if bodyErr == nil {
						sum := sha256.Sum256(body)
						obs.ResponseSHA256 = hex.EncodeToString(sum[:])
					}
				}
			}
			if bodyErr != nil {
				err = bodyErr
				obs.Failure = "CONTRACT"
			}
		}
	}
	obs.ObservedAt = t.now()
	// Cancellation must not silently drop already-incurred invocation facts.
	proofCtx, proofCancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
	journalErr := t.journal.ObserveStorageCall(proofCtx, obs)
	proofCancel()
	call.mu.Lock()
	call.observation = obs
	call.mu.Unlock()
	if journalErr != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, lifecycle.ErrStorageUnknown
	}
	if err != nil {
		if resp != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	return resp, nil
}

func validateAbsentObjectRetention(body []byte, c domain.Copy) error {
	if validXMLRootChildren(body, "Error", []string{"Code", "Message", "BucketName", "Key", "Resource", "RequestId", "HostId"}) != nil {
		return lifecycle.ErrStorageContract
	}
	var e struct {
		Code     []string `xml:"Code"`
		Resource []string `xml:"Resource"`
		Bucket   []string `xml:"BucketName"`
		Key      []string `xml:"Key"`
	}
	if xml.Unmarshal(body, &e) != nil || len(e.Code) != 1 || e.Code[0] != "NoSuchObjectLockConfiguration" || len(e.Resource) != 1 || e.Resource[0] != "/"+c.Namespace+"/"+c.Key || len(e.Bucket) > 1 || len(e.Key) > 1 {
		return lifecycle.ErrStorageContract
	}
	if (len(e.Bucket) == 1 && e.Bucket[0] != c.Namespace) || (len(e.Key) == 1 && e.Key[0] != c.Key) {
		return lifecycle.ErrStorageContract
	}
	return nil
}

func validInspectionRequest(req *http.Request, endpoint *url.URL, c *inspectionCall) bool {
	if req.URL.Scheme != endpoint.Scheme || req.URL.Host != endpoint.Host || req.URL.User != nil {
		return false
	}
	q := req.URL.Query()
	if c.operation == lifecycle.StorageListVersions {
		return req.Method == http.MethodGet && req.URL.Path == "/"+c.copy.Namespace+"/" &&
			q.Has("versions") && q.Get("prefix") == "" && q.Get("delimiter") == "" &&
			q.Get("key-marker") == "" && q.Get("version-id-marker") == "" &&
			onlyQuery(q, "versions", "prefix", "delimiter", "max-keys", "encoding-type")
	}
	if req.URL.Path != "/"+c.copy.Namespace+"/"+c.copy.Key || q.Get("versionId") != c.copy.ProviderVersion {
		return false
	}
	switch c.operation {
	case lifecycle.StorageStatExact:
		return req.Method == http.MethodHead && onlyQuery(q, "versionId")
	case lifecycle.StorageRetention:
		return req.Method == http.MethodGet && q.Has("retention") && onlyQuery(q, "versionId", "retention")
	case lifecycle.StorageLegalHold:
		return req.Method == http.MethodGet && q.Has("legal-hold") && onlyQuery(q, "versionId", "legal-hold")
	default:
		return false
	}
}

func onlyQuery(q url.Values, allowed ...string) bool {
	for key, values := range q {
		ok := false
		for _, name := range allowed {
			if key == name {
				ok = true
			}
		}
		if !ok || len(values) != 1 {
			return false
		}
	}
	return true
}

func boundedInspectionBody(r io.ReadCloser, op string) ([]byte, error) {
	defer r.Close()
	limit := int64(16 << 10)
	if op == lifecycle.StorageListVersions {
		limit = 2 << 20
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, lifecycle.ErrStorageUnknown
	}
	if int64(len(body)) > limit {
		return nil, lifecycle.ErrStorageContract
	}
	return body, nil
}

func validateFixtureInventory(body []byte, namespace string) error {
	if validXMLRootChildren(body, "ListVersionsResult", []string{"Name", "Prefix", "KeyMarker", "VersionIdMarker", "NextKeyMarker", "NextVersionIdMarker", "MaxKeys", "IsTruncated", "EncodingType", "Version", "DeleteMarker"}) != nil {
		return lifecycle.ErrStorageContract
	}
	type version struct {
		Key  []string `xml:"Key"`
		ID   []string `xml:"VersionId"`
		Size []int64  `xml:"Size"`
	}
	var inventory struct {
		XMLName        xml.Name
		Name           []string  `xml:"Name"`
		Prefix         []string  `xml:"Prefix"`
		IsTruncated    []string  `xml:"IsTruncated"`
		CommonPrefixes []string  `xml:"CommonPrefixes>Prefix"`
		Versions       []version `xml:"Version"`
		Markers        []version `xml:"DeleteMarker"`
	}
	if xml.Unmarshal(body, &inventory) != nil || inventory.XMLName.Local != "ListVersionsResult" ||
		len(inventory.Name) != 1 || inventory.Name[0] != namespace || len(inventory.Prefix) != 1 || inventory.Prefix[0] != "" ||
		len(inventory.IsTruncated) != 1 || (inventory.IsTruncated[0] != "true" && inventory.IsTruncated[0] != "false") || len(inventory.CommonPrefixes) != 0 {
		return lifecycle.ErrStorageContract
	}
	if inventory.IsTruncated[0] == "true" {
		return lifecycle.ErrStorageUnsupported
	}
	seen := make(map[string]bool)
	for _, v := range append(inventory.Versions, inventory.Markers...) {
		if len(v.Key) != 1 || v.Key[0] == "" || strings.ContainsRune(v.Key[0], '\x00') ||
			len(v.ID) != 1 || v.ID[0] == "" || v.ID[0] == "null" || seen[v.Key[0]+"\x00"+v.ID[0]] {
			return lifecycle.ErrStorageContract
		}
		seen[v.Key[0]+"\x00"+v.ID[0]] = true
	}
	for _, v := range inventory.Versions {
		if len(v.Size) != 1 || v.Size[0] < 0 {
			return lifecycle.ErrStorageContract
		}
	}
	return nil
}

func validateProtectionEnvelope(body []byte, operation string) error {
	root, fields := "Retention", []string{"Mode", "RetainUntilDate"}
	if operation == lifecycle.StorageLegalHold {
		root, fields = "LegalHold", []string{"Status"}
	}
	if err := validXMLRootChildren(body, root, fields); err != nil {
		return err
	}
	var envelope struct {
		Mode   []string `xml:"Mode"`
		Until  []string `xml:"RetainUntilDate"`
		Status []string `xml:"Status"`
	}
	if xml.Unmarshal(body, &envelope) != nil || len(envelope.Mode) > 1 || len(envelope.Until) > 1 || len(envelope.Status) > 1 {
		return lifecycle.ErrStorageContract
	}
	return nil
}

// Reject unknown root children and trailing documents rather than treating an
// unrecognised provider payload as a complete empty inventory or no retention.
// Parsing/decoding the native API itself remains the existing SDK's job.
func validXMLRootChildren(body []byte, root string, allowed []string) error {
	d := xml.NewDecoder(bytes.NewReader(body))
	depth := 0
	seenRoot, closed := false, false
	for {
		token, err := d.Token()
		if err == io.EOF {
			if !closed {
				return lifecycle.ErrStorageContract
			}
			return nil
		}
		if err != nil {
			return lifecycle.ErrStorageContract
		}
		switch v := token.(type) {
		case xml.StartElement:
			if closed {
				return lifecycle.ErrStorageContract
			}
			if depth == 0 {
				if seenRoot || v.Name.Local != root {
					return lifecycle.ErrStorageContract
				}
				seenRoot = true
			}
			if depth == 1 {
				ok := false
				for _, f := range allowed {
					if v.Name.Local == f {
						ok = true
					}
				}
				if !ok {
					return lifecycle.ErrStorageContract
				}
			}
			depth++
		case xml.EndElement:
			depth--
			if depth == 0 {
				closed = true
			}
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(v)) != "" {
				return lifecycle.ErrStorageContract
			}
		}
	}
}
