package platformmanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type logTestGate struct{ allow bool }

func (g *logTestGate) ConfirmRuntime(_ context.Context, p principal.RequestPrincipal, records bool) (principal.Authority, error) {
	if records && !g.allow {
		return principal.Authority{}, principal.ErrCapabilityDenied
	}
	return principal.Authority{Principal: p, EvaluatedAt: time.Now().UTC(), ExpiresAt: p.IdentityExpiresAt}, nil
}

type logTestSource struct {
	binding   RuntimeBinding
	items     []RuntimeRecord
	after     func()
	calls     int
	malformed bool
	err       error
}

func (s *logTestSource) Lookup(context.Context, uuid.UUID) (RuntimeBinding, bool) {
	return s.binding, true
}
func (s *logTestSource) Query(_ context.Context, _ uuid.UUID, q RuntimeQuery) (RuntimePage, error) {
	s.calls++
	out := RuntimePage{Items: []RuntimeRecord{}, Complete: true}
	for _, r := range s.items {
		if r.RecordedAt.Before(q.Start) || !r.RecordedAt.Before(q.End) || r.RecordedAt.After(q.AsOf) || q.After != nil && !beforePosition(r, *q.After) {
			continue
		}
		out.Items = append(out.Items, r)
	}
	if len(out.Items) > 25 {
		out.Items = out.Items[:25]
		v := out.Items[24]
		out.Next = &RuntimePosition{v.RecordedAt, v.ID}
	}
	if s.malformed {
		out.Items = append(out.Items, out.Items[0])
	}
	if s.after != nil {
		s.after()
	}
	return out, s.err
}
func (s *logTestSource) ReadExact(_ context.Context, _ uuid.UUID, id string) (RuntimeRecord, error) {
	s.calls++
	if s.after != nil {
		defer s.after()
	}
	for _, r := range s.items {
		if r.ID == id {
			return r, s.err
		}
	}
	return RuntimeRecord{}, LogMissing
}

func TestRuntimeLogSourceErrorsStillRecheckCurrentAuthority(t *testing.T) {
	for _, operation := range []string{"query", "detail", "missing", "changed"} {
		for _, change := range []string{"capability", "identity", "source"} {
			t.Run(operation+"/"+change, func(t *testing.T) {
				h, p, g, s, path := logTestSetup(t)
				if operation != "query" {
					first := logRequest(t, h, path, 200)
					path = strings.Split(path, "entries?")[0] + "entries/" + first.Data.Items[0].EntryRef
				}
				switch operation {
				case "query", "detail":
					s.err = LogUnavailable
				case "missing":
					s.items = nil
				case "changed":
					s.items[0].Level = "FATAL"
				}
				want := 403
				s.after = func() {
					switch change {
					case "capability":
						g.allow = false
					case "identity":
						p.p.IdentityRevision = "1:1:2"
					case "source":
						s.binding.Version = "v2"
					}
				}
				if change == "identity" {
					want = 401
				} else if change == "source" {
					want = 409
				}
				wire := logRequest(t, h, path, want)
				if wire.Data.Items != nil || wire.Data.Item.EntryRef != "" {
					t.Fatal("failed current authority exposed source facts")
				}
			})
		}
	}
}

func TestRuntimeLogReferenceMustRemainValidThroughSourceRead(t *testing.T) {
	for _, kind := range []string{"cursor", "entry"} {
		t.Run(kind, func(t *testing.T) {
			h, p, _, s, path := logTestSetup(t)
			now := time.Now().UTC()
			h.now = func() time.Time { return now }
			first := logRequest(t, h, path, 200)
			token := first.Data.Items[0].EntryRef
			if kind == "cursor" {
				token = *first.Data.Next
			}
			ref, err := h.decode(token, kind, p.p, s.binding)
			if err != nil {
				t.Fatal(err)
			}
			ref.Expires = now.Add(time.Second)
			token, err = h.encode(ref, kind)
			if err != nil {
				t.Fatal(err)
			}
			target := strings.Split(path, "entries?")[0] + "entries/" + token
			if kind == "cursor" {
				target = path + "&cursor=" + token
			}
			s.after = func() { now = ref.Expires }
			wire := logRequest(t, h, target, 409)
			if wire.Code != string(LogExpired) || wire.Data.Items != nil || wire.Data.Item.EntryRef != "" {
				t.Fatal("expired reference exposed records or source state")
			}
		})
	}
}

type logWire struct {
	Envelope
	Data struct {
		Window runtimeWindow  `json:"window"`
		Items  []runtimeEntry `json:"items"`
		Next   *string        `json:"next"`
		Item   runtimeEntry   `json:"item"`
		State  string         `json:"state"`
	} `json:"data"`
	Code string `json:"code"`
}

func logTestSetup(t *testing.T) (*RuntimeLogHandler, *testResolver, *logTestGate, *logTestSource, string) {
	t.Helper()
	ws, actor := uuid.New(), uuid.New()
	p := principal.RequestPrincipal{ActorID: actor, Subject: actor.String(), WorkspaceID: ws, IdentityIssuer: "https://test.invalid", IdentitySubject: "one", IdentityRevision: "1:1:1", IdentityExpiresAt: time.Now().Add(time.Minute)}
	resolver := &testResolver{p: p}
	gate := &logTestGate{true}
	source := &logTestSource{}
	at := time.Now().UTC().Add(-time.Minute)
	for i := 57; i >= 0; i-- {
		source.items = append(source.items, RuntimeRecord{ID: fmt.Sprintf("id-%03d", i), RecordedAt: at, Service: "APP_SERVICE", Level: "INFO", EventCode: "READ_COMPLETED"})
	}
	source.binding = RuntimeBinding{"synthetic", "v1", "r1", "SYNTHETIC_FIXTURE", []string{"APP_SERVICE"}, source}
	h, e := NewRuntimeLogHandler(resolver, gate, source, "SYNTHETIC_FIXTURE")
	if e != nil {
		t.Fatal(e)
	}
	query := url.Values{"start": {time.Now().UTC().Add(-time.Hour).Format(LogTimeLayout)}, "end": {time.Now().UTC().Format(LogTimeLayout)}}.Encode()
	return h, resolver, gate, source, "/api/v1/workspaces/" + ws.String() + "/management/runtime-logs/entries?" + query
}
func logRequest(t *testing.T, h *RuntimeLogHandler, target string, want int) logWire {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	r := httptest.NewRequest("GET", target, nil)
	r.Header.Set("Authorization", "Bearer synthetic")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != want {
		var reason struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &reason)
		t.Fatalf("status=%d want=%d code=%s", w.Code, want, reason.Code)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("cacheable response")
	}
	var out logWire
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatal("invalid json")
	}
	return out
}
func TestRuntimeLogEqualTimestampPagingExactDetailAndOpaqueScope(t *testing.T) {
	h, p, _, source, path := logTestSetup(t)
	seen := map[string]bool{}
	cursor := ""
	var entry runtimeEntry
	for _, size := range []int{25, 25, 8} {
		target := path
		if cursor != "" {
			target += "&cursor=" + cursor
		}
		wire := logRequest(t, h, target, 200)
		if len(wire.Data.Items) != size {
			t.Fatal("lost equal-time rows")
		}
		for _, item := range wire.Data.Items {
			if seen[item.RowKey] {
				t.Fatal("duplicate record reference")
			}
			seen[item.RowKey] = true
			entry = item
		}
		if wire.Data.Next != nil {
			cursor = *wire.Data.Next
		} else {
			cursor = ""
		}
	}
	if len(seen) != 58 || cursor != "" {
		t.Fatal("pagination did not exhaust exact records")
	}
	detail := strings.Split(path, "entries?")[0] + "entries/" + entry.EntryRef
	d := logRequest(t, h, detail, 200)
	if d.Data.Item.RowKey != entry.RowKey || d.Data.Item.EntryRef != entry.EntryRef {
		t.Fatal("detail identity changed")
	}
	tampered := []byte(entry.EntryRef)
	if tampered[len(tampered)/2] == 'A' {
		tampered[len(tampered)/2] = 'B'
	} else {
		tampered[len(tampered)/2] = 'A'
	}
	logRequest(t, h, strings.Replace(detail, entry.EntryRef, string(tampered), 1), 409)
	p.p.ActorID = uuid.New()
	p.p.Subject = p.p.ActorID.String()
	logRequest(t, h, detail, 403)
	p.p.WorkspaceID = uuid.New()
	logRequest(t, h, strings.Replace(detail, hSourceWorkspace(detail), p.p.WorkspaceID.String(), 1), 403)
	if source.calls < 4 {
		t.Fatal("reader not exercised")
	}
}
func hSourceWorkspace(path string) string { return strings.Split(path, "/")[4] }
func TestRuntimeLogReadRechecksRevocationBindingAndExpiry(t *testing.T) {
	for _, change := range []string{"capability", "identity", "source"} {
		t.Run(change, func(t *testing.T) {
			h, p, g, s, path := logTestSetup(t)
			s.after = func() {
				switch change {
				case "capability":
					g.allow = false
				case "identity":
					p.p.IdentityRevision = "1:1:2"
				case "source":
					s.binding.Version = "v2"
				}
			}
			want := 403
			if change == "identity" {
				want = 401
			}
			if change == "source" {
				want = 409
			}
			wire := logRequest(t, h, path, want)
			if wire.Data.Items != nil {
				t.Fatal("revoked data escaped")
			}
		})
	}
	h, p, _, _, path := logTestSetup(t)
	wire := logRequest(t, h, path, 200)
	entry := wire.Data.Items[0]
	ref, e := h.decode(entry.EntryRef, "entry", p.p, func() RuntimeBinding { b, _ := h.lookup(context.Background(), p.p.WorkspaceID); return b }())
	if e != nil {
		t.Fatal(e)
	}
	ref.Expires = time.Now().Add(-time.Second)
	expired, e := h.encode(ref, "entry")
	if e != nil {
		t.Fatal(e)
	}
	logRequest(t, h, strings.Split(path, "entries?")[0]+"entries/"+expired, 409)
}
func TestRuntimeLogSourceStatesStrictQueriesAndChangedRecords(t *testing.T) {
	h, _, g, s, path := logTestSetup(t)
	sourcePath := strings.Split(path, "entries?")[0] + "source"
	g.allow = false
	logRequest(t, h, sourcePath, 200)
	logRequest(t, h, path, 403)
	if s.calls != 0 {
		t.Fatal("unauthorized source read")
	}
	g.allow = true
	h.Sources = nil
	wire := logRequest(t, h, sourcePath, 200)
	if wire.Data.State != "UNCONFIGURED" {
		t.Fatal("invented source")
	}
	h.Sources = s
	s.binding.Reader = nil
	wire = logRequest(t, h, sourcePath, 200)
	if wire.Data.State != "UNSUPPORTED" {
		t.Fatal("unsupported binding accepted")
	}
	s.binding.Reader = s
	for _, extra := range []string{"&tenant=foreign", "&start=x", "&limit=100", "&service=arbitrary", "&cursor=bad"} {
		logRequest(t, h, path+extra, 400)
	}
	wire = logRequest(t, h, path, 200)
	entry := wire.Data.Items[0]
	s.items[0].Level = "FATAL"
	logRequest(t, h, strings.Split(path, "entries?")[0]+"entries/"+entry.EntryRef, 409)
	s.items[0].Level = "INFO"
	filtered := logRequest(t, h, path+"&level=INFO", 200)
	s.items[0].Level = "FATAL"
	logRequest(t, h, strings.Split(path, "entries?")[0]+"entries/"+filtered.Data.Items[0].EntryRef, 409)
	s.malformed = true
	logRequest(t, h, path, 503)
}
