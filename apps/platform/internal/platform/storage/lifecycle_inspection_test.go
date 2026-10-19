package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	lifecycle "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Test-only journal. It deliberately is not available to runtime wiring.
type inspectionTestJournal struct {
	mu                       sync.Mutex
	starts                   []lifecycle.StorageCallStart
	observations             []lifecycle.StorageCallObservation
	startErr, observationErr error
	observedCanceled         bool
}

func (j *inspectionTestJournal) StartStorageCall(_ context.Context, c lifecycle.StorageCallStart) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.startErr != nil {
		return j.startErr
	}
	j.starts = append(j.starts, c)
	return nil
}
func (j *inspectionTestJournal) ObserveStorageCall(ctx context.Context, o lifecycle.StorageCallObservation) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.observedCanceled = ctx.Err() != nil
	if j.observationErr != nil {
		return j.observationErr
	}
	j.observations = append(j.observations, o)
	return nil
}

func inspectionFixture() domain.Copy {
	return domain.Copy{ID: uuid.New(), WorkspaceID: uuid.New(), DatasetVersionID: uuid.New(), ProviderInstanceID: uuid.New(),
		IncarnationID: uuid.New(), Namespace: "lifecycle307-primary", Key: "synthetic/fixture +汉字.csv", ProviderVersion: "version-307",
		ContentSHA256: strings.Repeat("a", 64), ByteSize: 9}
}
func inspectorForTest(t *testing.T, c domain.Copy, j *inspectionTestJournal, handler http.HandlerFunc) *LifecycleInspector {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	s, err := NewLifecycleInspector(LifecycleInspectionConfig{Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "test-access", SecretKey: "test-secret",
		WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID,
		Namespaces: []string{c.Namespace, "lifecycle307-archive"}, Journal: j})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func exactStatResponse(w http.ResponseWriter, c domain.Copy) {
	w.Header().Set("X-Amz-Version-Id", c.ProviderVersion)
	w.Header().Set("Content-Length", fmt.Sprint(c.ByteSize))
	w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	w.Header().Set("ETag", `"etag-is-not-a-content-hash"`)
	w.WriteHeader(http.StatusOK)
}

func TestLifecycleInspectorNativeAbsentRetentionIsExactAndCosted(t *testing.T) {
	c := inspectionFixture()
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"native-none", 400, `<Error><Code>NoSuchObjectLockConfiguration</Code><Resource>/` + c.Namespace + `/` + c.Key + `</Resource></Error>`, nil},
		{"duplicate-code", 400, `<Error><Code>NoSuchObjectLockConfiguration</Code><Code>NoSuchObjectLockConfiguration</Code><Resource>/` + c.Namespace + `/` + c.Key + `</Resource></Error>`, lifecycle.ErrStorageContract},
		{"foreign-resource", 400, `<Error><Code>NoSuchObjectLockConfiguration</Code><Resource>/other/key</Resource></Error>`, lifecycle.ErrStorageContract},
		{"generic-400", 400, `<Error><Code>InvalidRequest</Code></Error>`, lifecycle.ErrStorageUnknown},
		{"denied-403", 403, `<Error><Code>NoSuchObjectLockConfiguration</Code></Error>`, lifecycle.ErrStorageDenied},
		{"missing-404", 404, `<Error><Code>NoSuchObjectLockConfiguration</Code></Error>`, lifecycle.ErrStorageNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := &inspectionTestJournal{}
			s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Has("retention") {
					w.WriteHeader(tc.status)
					fmt.Fprint(w, tc.body)
				} else if tc.name == "native-none" {
					w.WriteHeader(400)
					fmt.Fprint(w, tc.body)
				} else {
					fmt.Fprint(w, `<LegalHold><Status>OFF</Status></LegalHold>`)
				}
			})
			p, err := s.InspectRetentionAndHold(context.Background(), c)
			if tc.want != nil {
				if !errors.Is(err, tc.want) {
					t.Fatal(err)
				}
				return
			}
			if err != nil || p.RetentionMode != "NONE" || p.RetainUntil != nil || p.LegalHold || len(p.CallIDs) != 2 {
				t.Fatalf("native none: %+v %v", p, err)
			}
			if len(j.observations) != 2 || j.observations[0].HTTPStatus != 400 || j.observations[0].InvokedAt == nil || j.observations[0].ResponseSHA256 == "" || j.observations[0].Failure != "" {
				t.Fatal("native response not separately observed/costable")
			}
		})
	}
}

func TestLifecycleInspectorExactScopeAndJournal(t *testing.T) {
	c, j := inspectionFixture(), &inspectionTestJournal{}
	var calls atomic.Int64
	s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		j.mu.Lock()
		before := len(j.starts)
		j.mu.Unlock()
		if before == 0 {
			t.Error("HTTP occurred before durable start")
		}
		if r.Method != http.MethodHead || r.URL.Path != "/"+c.Namespace+"/"+c.Key || r.URL.Query().Get("versionId") != c.ProviderVersion {
			t.Errorf("wrong exact request: %s %s", r.Method, r.URL.Path)
		}
		exactStatResponse(w, c)
	})
	got, err := s.InspectExactCopy(context.Background(), c)
	if err != nil || got.Copy != c || got.CallID == uuid.Nil || got.ObservedAt.IsZero() {
		t.Fatalf("inspection: %+v %v", got, err)
	}
	if calls.Load() != 1 || len(j.starts) != 1 || len(j.observations) != 1 {
		t.Fatal("expected one physical request")
	}
	start, obs := j.starts[0], j.observations[0]
	if start.Copy != c || start.Operation != lifecycle.StorageStatExact || obs.CallID != start.ID || obs.InvokedAt == nil ||
		obs.InvokedAt.Before(start.StartedAt) || obs.ObservedAt.Before(*obs.InvokedAt) || obs.HTTPStatus != 200 {
		t.Fatalf("unbound start/observation: %+v %+v", start, obs)
	}
	for _, mutate := range []func(*domain.Copy){
		func(c *domain.Copy) { c.WorkspaceID = uuid.New() }, func(c *domain.Copy) { c.ProviderInstanceID = uuid.New() },
		func(c *domain.Copy) { c.IncarnationID = uuid.New() }, func(c *domain.Copy) { c.Namespace = "other-namespace" },
		func(c *domain.Copy) { c.ProviderVersion = "" }, func(c *domain.Copy) { c.ProviderVersion = "null" },
	} {
		foreign := c
		mutate(&foreign)
		if _, err := s.InspectExactCopy(context.Background(), foreign); !errors.Is(err, lifecycle.ErrStorageScope) {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatal("invalid scope reached provider")
	}
}

func TestLifecycleInspectorNoRetriesAndFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   error
	}{
		{"denied", 403, lifecycle.ErrStorageDenied}, {"not-found-is-not-absence", 404, lifecycle.ErrStorageNotFound},
		{"retryable-error", 503, lifecycle.ErrStorageUnknown}, {"unsupported", 501, lifecycle.ErrStorageUnsupported},
		{"redirect", 307, lifecycle.ErrStorageUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, j := inspectionFixture(), &inspectionTestJournal{}
			var calls atomic.Int64
			s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Location", "http://127.0.0.1:1/credential-leak")
				w.WriteHeader(tc.status)
			})
			got, err := s.InspectExactCopy(context.Background(), c)
			if !errors.Is(err, tc.want) || got.CallID != uuid.Nil || calls.Load() != 1 || len(j.observations) != 1 {
				t.Fatalf("got=%+v err=%v calls=%d", got, err, calls.Load())
			}
			if j.observations[0].InvokedAt == nil || j.observations[0].HTTPStatus != tc.status {
				t.Fatal("failed invocation disappeared")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(http.ResponseWriter)
	}{
		{"wrong-version", func(w http.ResponseWriter) { w.Header().Set("X-Amz-Version-Id", "other-version") }},
		{"bare-head", func(w http.ResponseWriter) { w.Header().Del("X-Amz-Version-Id") }},
		{"delete-marker", func(w http.ResponseWriter) { w.Header().Set("X-Amz-Delete-Marker", "true") }},
		{"wrong-size", func(w http.ResponseWriter) { w.Header().Set("Content-Length", "10") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := inspectionFixture()
			s := inspectorForTest(t, c, &inspectionTestJournal{}, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Amz-Version-Id", c.ProviderVersion)
				w.Header().Set("Content-Length", "9")
				w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
				tc.mutate(w)
				w.WriteHeader(200)
			})
			if _, err := s.InspectExactCopy(context.Background(), c); !errors.Is(err, lifecycle.ErrStorageContract) {
				t.Fatal(err)
			}
		})
	}
}

func TestLifecycleInspectorProtectionCallsAreSeparate(t *testing.T) {
	c, j := inspectionFixture(), &inspectionTestJournal{}
	s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Query().Get("versionId") != c.ProviderVersion {
			t.Error("unversioned protection request")
		}
		w.Header().Set("X-Amz-Version-Id", c.ProviderVersion)
		if r.URL.Query().Has("retention") {
			fmt.Fprint(w, `<Retention><Mode>COMPLIANCE</Mode><RetainUntilDate>2030-01-01T00:00:00Z</RetainUntilDate></Retention>`)
		} else {
			fmt.Fprint(w, `<LegalHold><Status>ON</Status></LegalHold>`)
		}
	})
	got, err := s.InspectRetentionAndHold(context.Background(), c)
	if err != nil || got.RetentionMode != "COMPLIANCE" || got.RetainUntil == nil || !got.LegalHold || len(got.CallIDs) != 2 || got.CallIDs[0] == got.CallIDs[1] {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if len(j.starts) != 2 || len(j.observations) != 2 || j.starts[0].Operation != lifecycle.StorageRetention || j.starts[1].Operation != lifecycle.StorageLegalHold {
		t.Fatal("collapsed physical requests")
	}
	for _, o := range j.observations {
		if o.InvokedAt == nil || len(o.ResponseSHA256) != 64 {
			t.Fatal("missing response proof")
		}
	}
	for _, tc := range []struct{ name, retention, hold string }{
		{"unknown-mode", `<Retention><Mode>UNKNOWN</Mode></Retention>`, `<LegalHold><Status>OFF</Status></LegalHold>`},
		{"missing-date", `<Retention><Mode>COMPLIANCE</Mode></Retention>`, `<LegalHold><Status>OFF</Status></LegalHold>`},
		{"missing-hold", `<Retention/>`, `<LegalHold/>`},
		{"unknown-hold", `<Retention/>`, `<LegalHold><Status>UNKNOWN</Status></LegalHold>`},
		{"duplicate-mode", `<Retention><Mode>COMPLIANCE</Mode><Mode/></Retention>`, `<LegalHold><Status>OFF</Status></LegalHold>`},
		{"unrecognised-retention-field", `<Retention><UnknownRetention>COMPLIANCE</UnknownRetention></Retention>`, `<LegalHold><Status>OFF</Status></LegalHold>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := inspectorForTest(t, c, &inspectionTestJournal{}, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Has("retention") {
					fmt.Fprint(w, tc.retention)
				} else {
					fmt.Fprint(w, tc.hold)
				}
			})
			if _, err := bad.InspectRetentionAndHold(context.Background(), c); !errors.Is(err, lifecycle.ErrStorageContract) {
				t.Fatal(err)
			}
		})
	}
}

func TestLifecycleInspectorInventoryRequiresCompleteEnvelope(t *testing.T) {
	c, j := inspectionFixture(), &inspectionTestJournal{}
	empty := `<ListVersionsResult><Name>` + c.Namespace + `</Name><Prefix/><IsTruncated>false</IsTruncated></ListVersionsResult>`
	s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.Query().Has("versions") || r.URL.Query().Get("prefix") != "" || r.URL.Path != "/"+c.Namespace+"/" {
			t.Error("partial inventory request")
		}
		fmt.Fprint(w, empty)
	})
	got, err := s.InspectNamespace(context.Background(), c)
	if err != nil || len(got.Versions) != 0 || got.CallID == uuid.Nil || got.Namespace != c.Namespace || len(got.ResponseSHA256) != 64 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	for _, tc := range []struct {
		name, body string
		want       error
	}{
		{"missing-envelope", `<ListVersionsResult/>`, lifecycle.ErrStorageContract},
		{"missing-prefix", strings.Replace(empty, `<Prefix/>`, "", 1), lifecycle.ErrStorageContract},
		{"wrong-namespace", strings.Replace(empty, c.Namespace, "other-namespace", 1), lifecycle.ErrStorageContract},
		{"empty-truncated", strings.Replace(empty, `<IsTruncated>false</IsTruncated>`, `<IsTruncated/>`, 1), lifecycle.ErrStorageContract},
		{"truncated", strings.Replace(empty, `<IsTruncated>false`, `<IsTruncated>true`, 1), lifecycle.ErrStorageUnsupported},
		{"duplicate-header", strings.Replace(empty, `<Prefix/>`, `<Prefix/><Prefix/>`, 1), lifecycle.ErrStorageContract},
		{"ignored-object-field", strings.Replace(empty, `</ListVersionsResult>`, `<Contents><Key>hidden.csv</Key></Contents></ListVersionsResult>`, 1), lifecycle.ErrStorageContract},
		{"trailing-document", empty + `<ListVersionsResult/>`, lifecycle.ErrStorageContract},
		{"unversioned-member", strings.Replace(empty, `</ListVersionsResult>`, `<Version><Key>x</Key><Size>9</Size></Version></ListVersionsResult>`, 1), lifecycle.ErrStorageContract},
		{"null-member", strings.Replace(empty, `</ListVersionsResult>`, `<Version><Key>x</Key><Size>9</Size><VersionId>null</VersionId></Version></ListVersionsResult>`, 1), lifecycle.ErrStorageContract},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int64
			bad := inspectorForTest(t, c, &inspectionTestJournal{}, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); fmt.Fprint(w, tc.body) })
			got, err := bad.InspectNamespace(context.Background(), c)
			if !errors.Is(err, tc.want) || got.CallID != uuid.Nil || calls.Load() != 1 {
				t.Fatalf("got=%+v err=%v calls=%d", got, err, calls.Load())
			}
		})
	}
	member := `<Version><Key>synthetic/test.csv</Key><VersionId>v1</VersionId><Size>9</Size><IsLatest>false</IsLatest></Version><DeleteMarker><Key>synthetic/test.csv</Key><VersionId>v2</VersionId><IsLatest>true</IsLatest></DeleteMarker>`
	withVersions := inspectorForTest(t, c, &inspectionTestJournal{}, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, strings.Replace(empty, `</ListVersionsResult>`, member+`</ListVersionsResult>`, 1))
	})
	got, err = withVersions.InspectNamespace(context.Background(), c)
	if err != nil || len(got.Versions) != 2 || got.Versions[0].DeleteMarker || !got.Versions[1].DeleteMarker {
		t.Fatalf("versions=%+v err=%v", got, err)
	}
}

func TestLifecycleInspectorJournalAndLostResponse(t *testing.T) {
	for _, failStart := range []bool{true, false} {
		c, j := inspectionFixture(), &inspectionTestJournal{}
		if failStart {
			j.startErr = errors.New("fixture start failure")
		} else {
			j.observationErr = errors.New("fixture observation failure")
		}
		var calls atomic.Int64
		s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); exactStatResponse(w, c) })
		_, err := s.InspectExactCopy(context.Background(), c)
		original := j.observationErr
		if failStart {
			original = j.startErr
		}
		if !errors.Is(err, lifecycle.ErrStorageUnknown) || !errors.Is(err, original) || !strings.Contains(err.Error(), original.Error()) {
			t.Fatal("journal cause or fail-closed category lost", err)
		}
		want := int64(1)
		if failStart {
			want = 0
		}
		if calls.Load() != want || len(j.starts) != int(want) {
			t.Fatal("journal boundary bypassed")
		}
	}
	c, j := inspectionFixture(), &inspectionTestJournal{}
	var calls atomic.Int64
	s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	})
	if _, err := s.InspectExactCopy(context.Background(), c); !errors.Is(err, lifecycle.ErrStorageUnknown) {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(j.observations) != 1 || j.observations[0].InvokedAt == nil || j.observations[0].Failure != "TRANSPORT" {
		t.Fatal("lost response retried or erased invocation")
	}
}

func TestLifecycleInspectorHasNoUnmanagedDataOrWritePath(t *testing.T) {
	c, j := inspectionFixture(), &inspectionTestJournal{}
	var calls atomic.Int64
	s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) })
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPost} {
		ctx, cancel, _ := s.callContext(context.Background(), c, lifecycle.StorageStatExact)
		req, _ := http.NewRequestWithContext(ctx, method, "http://"+s.config.Endpoint+"/"+c.Namespace+"/"+c.Key+"?versionId="+c.ProviderVersion, nil)
		if _, err := s.transport.RoundTrip(req); !errors.Is(err, lifecycle.ErrStorageScope) {
			t.Fatal(err)
		}
		cancel()
	}
	if calls.Load() != 0 || len(j.starts) != 0 {
		t.Fatal("unmanaged byte read or write reached provider")
	}
}

func TestLifecycleInspectorRequiresBoundConfigAndPreservesCanceledCall(t *testing.T) {
	c := inspectionFixture()
	config := LifecycleInspectionConfig{Endpoint: "127.0.0.1:1", AccessKey: "test-access", SecretKey: "test-secret",
		WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID,
		Namespaces: []string{c.Namespace, "lifecycle307-archive"}, Journal: &inspectionTestJournal{}}
	for _, mutate := range []func(*LifecycleInspectionConfig){
		func(c *LifecycleInspectionConfig) { c.Journal = nil }, func(c *LifecycleInspectionConfig) { c.Namespaces = []string{"same", "same"} },
		func(c *LifecycleInspectionConfig) { c.Namespaces = []string{"one"} }, func(c *LifecycleInspectionConfig) { c.IncarnationID = uuid.Nil },
		func(c *LifecycleInspectionConfig) { c.Endpoint = "user:secret@127.0.0.1:1" }, func(c *LifecycleInspectionConfig) { c.Endpoint = "127.0.0.1:1/extra-path" },
	} {
		bad := config
		mutate(&bad)
		if _, err := NewLifecycleInspector(bad); !errors.Is(err, lifecycle.ErrStorageScope) {
			t.Fatal(err)
		}
	}
	j := &inspectionTestJournal{}
	started := make(chan struct{}, 1)
	s := inspectorForTest(t, c, j, func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := s.InspectExactCopy(ctx, c); result <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not reach synthetic provider")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, lifecycle.ErrStorageUnknown) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled inspection did not finish")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if len(j.starts) != 1 || len(j.observations) != 1 || j.observedCanceled || j.observations[0].InvokedAt == nil {
		t.Fatal("cancellation erased an already-written request or canceled its proof transaction")
	}
}
