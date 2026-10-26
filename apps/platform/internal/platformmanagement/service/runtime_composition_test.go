package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

type composedLogGate struct {
	queryGate
	expires time.Time
}

func (g *composedLogGate) ConfirmRuntime(_ context.Context, p principal.RequestPrincipal, _ bool) (principal.Authority, error) {
	return principal.Authority{Principal: p, EvaluatedAt: time.Now(), ExpiresAt: g.expires}, nil
}

type composedLogSource struct {
	binding management.RuntimeBinding
	items   []management.RuntimeRecord
	calls   int
	after   func()
	failure bool
}

func (s *composedLogSource) Lookup(context.Context, uuid.UUID) (management.RuntimeBinding, bool) {
	return s.binding, true
}
func (s *composedLogSource) Query(_ context.Context, _ uuid.UUID, q management.RuntimeQuery) (management.RuntimePage, error) {
	s.calls++
	if s.after != nil {
		s.after()
	}
	if s.failure {
		return management.RuntimePage{}, management.LogUnavailable
	}
	items := []management.RuntimeRecord{}
	for _, r := range s.items {
		if q.After == nil || r.RecordedAt.Before(q.After.At) || r.RecordedAt.Equal(q.After.At) && r.ID < q.After.ID {
			items = append(items, r)
		}
	}
	p := management.RuntimePage{Items: items, Complete: true}
	if len(items) > 25 {
		p.Items = items[:25]
		last := p.Items[24]
		p.Next = &management.RuntimePosition{At: last.RecordedAt, ID: last.ID}
	}
	return p, nil
}
func (s *composedLogSource) ReadExact(_ context.Context, _ uuid.UUID, id string) (management.RuntimeRecord, error) {
	s.calls++
	if s.after != nil {
		s.after()
	}
	if s.failure {
		return management.RuntimeRecord{}, management.LogUnavailable
	}
	for _, r := range s.items {
		if r.ID == id {
			return r, nil
		}
	}
	return management.RuntimeRecord{}, management.LogMissing
}

type composedLogWire struct {
	Code string
	Data *struct {
		Items []struct {
			EntryRef string `json:"entryRef"`
		}
		Next *string
	}
}

func composedLogRead(t *testing.T, h http.Handler, target string, want int) composedLogWire {
	t.Helper()
	r := httptest.NewRequest("GET", target, nil)
	r.Header.Set("Authorization", "Bearer synthetic")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var b composedLogWire
	if w.Code != want || json.Unmarshal(w.Body.Bytes(), &b) != nil || want != 200 && b.Data != nil {
		t.Fatalf("shared runtime response status=%d want=%d", w.Code, want)
	}
	return b
}
func composedLogs(t *testing.T) (http.Handler, *composedLogSource, *composedLogGate, string, string) {
	t.Helper()
	ws := uuid.New()
	s := &composedLogSource{}
	at := time.Now().UTC().Add(-time.Minute)
	for i := 25; i >= 0; i-- {
		s.items = append(s.items, management.RuntimeRecord{ID: fmt.Sprintf("r-%03d", i), RecordedAt: at, Service: "APP_SERVICE", Level: "INFO", EventCode: "READ_COMPLETED"})
	}
	s.binding = management.RuntimeBinding{ID: "synthetic-composed", Version: "v1", RuleVersion: "r1", Source: "SYNTHETIC_FIXTURE", Services: []string{"APP_SERVICE"}, Reader: s}
	g := &composedLogGate{expires: time.Now().Add(time.Minute)}
	h, e := Compose(Dependencies{Principal: queryResolver{ws, uuid.New()}, Gate: g, IdentitySource: "SYNTHETIC_FIXTURE", RuntimeSources: s, Ready: func(context.Context) error { return nil }, Confirmation: func(http.ResponseWriter, *http.Request) {}})
	if e != nil {
		t.Fatal(e)
	}
	prefix := "/api/v1/workspaces/" + ws.String() + "/management/runtime-logs/"
	q := url.Values{"start": {at.Add(-time.Minute).Format(management.LogTimeLayout)}, "end": {time.Now().UTC().Format(management.LogTimeLayout)}}.Encode()
	return h, s, g, prefix, q
}

func TestSharedCompositionRejectsBindingChangesDuringSuccessAndFailure(t *testing.T) {
	for _, detail := range []bool{false, true} {
		for _, failure := range []bool{false, true} {
			for _, dimension := range []string{"binding", "rule"} {
				t.Run(fmt.Sprintf("detail=%t/failure=%t/%s", detail, failure, dimension), func(t *testing.T) {
					h, s, _, prefix, q := composedLogs(t)
					first := composedLogRead(t, h, prefix+"entries?"+q, 200)
					entry := first.Data.Items[0].EntryRef
					cursor := *first.Data.Next
					s.failure = failure
					s.after = func() {
						if dimension == "binding" {
							s.binding.Version = "v2"
						} else {
							s.binding.RuleVersion = "r2"
						}
					}
					target := prefix + "entries?" + q
					if detail {
						target = prefix + "entries/" + entry
					}
					b := composedLogRead(t, h, target, 409)
					if b.Code != "LOG_QUERY_EXPIRED" {
						t.Fatal("binding mutation observation leaked")
					}
					s.after = nil
					s.failure = false
					calls := s.calls
					composedLogRead(t, h, prefix+"entries/"+entry, 409)
					composedLogRead(t, h, prefix+"entries?"+q+"&cursor="+url.QueryEscape(cursor), 409)
					if s.calls != calls {
						t.Fatal("old binding reference reached source")
					}
				})
			}
		}
	}
}
func TestSharedCompositionExpiresOriginalReferencesWithoutClockOrProductHooks(t *testing.T) {
	h, s, g, prefix, q := composedLogs(t)
	deadline := time.Now().Add(150 * time.Millisecond)
	g.expires = deadline
	first := composedLogRead(t, h, prefix+"entries?"+q, 200)
	entry := first.Data.Items[0].EntryRef
	cursor := *first.Data.Next
	g.expires = time.Now().Add(time.Minute)
	time.Sleep(time.Until(deadline) + time.Millisecond)
	calls := s.calls
	for _, target := range []string{prefix + "entries/" + entry, prefix + "entries?" + q + "&cursor=" + url.QueryEscape(cursor)} {
		b := composedLogRead(t, h, target, 409)
		if b.Code != "LOG_QUERY_EXPIRED" {
			t.Fatal("reference expiry bypassed")
		}
	}
	if s.calls != calls {
		t.Fatal("expired reference was renewed or read source")
	}
}
