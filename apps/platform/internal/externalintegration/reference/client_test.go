package reference_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports/contracttest"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/reference"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/testkit"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReferenceRejectsRedirectOversizeAndRawDiagnostics(t *testing.T) {
	q := d.Request{Instance: "test", RequestKey: "key"}
	r := testkit.Result(q, "PUBLISHED")
	r.ExternalID = "release"
	r.RemoteID = "remote"
	raw, _ := json.Marshal(r)
	for _, mode := range []string{"redirect", "oversize", "diagnostic", "unknown-field"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				calls++
				switch mode {
				case "redirect":
					w.Header().Set("Location", "/elsewhere")
					w.WriteHeader(307)
				case "oversize":
					_, _ = w.Write(append(raw, []byte(strings.Repeat(" ", 16*1024))...))
				case "diagnostic":
					w.WriteHeader(503)
					_, _ = w.Write([]byte("secret-provider-diagnostic"))
				case "unknown-field":
					_, _ = w.Write([]byte(strings.TrimSuffix(string(raw), "}") + `,"secret":"value"}`))
				}
			}))
			defer s.Close()
			c, err := reference.New(s.URL)
			if err != nil {
				t.Fatal(err)
			}
			dispatches := 0
			_, err = c.Submit(context.Background(), q, ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { dispatches++; return nil }})
			if !errors.Is(err, d.ErrUnavailable) || calls != 1 || dispatches != 1 {
				t.Fatalf("unbounded/raw/retried response: %v calls=%d dispatches=%d", err, calls, dispatches)
			}
		})
	}
}

func TestSharedPublicationAdapterContract(t *testing.T) {
	for _, httpAdapter := range []bool{false, true} {
		name := "fake"
		if httpAdapter {
			name = "reference-http"
		}
		t.Run(name, func(t *testing.T) {
			contracttest.Run(t, func(t *testing.T, scenario string) ports.Adapter {
				p := testkit.New()
				if scenario == "registered" {
					p.SubmitState = "REGISTERED"
					p.PollState = "PUBLISHED"
				}
				if scenario == "failed" {
					p.SubmitState = "FAILED"
				}
				if scenario == "lost-response" {
					p.LostResponse = true
				}
				if !httpAdapter {
					return p.Adapter("reference", "synthetic")
				}
				s := p.Server()
				t.Cleanup(s.Close)
				c, err := reference.New(s.URL)
				if err != nil {
					t.Fatal(err)
				}
				return c.Adapter("reference", "synthetic")
			})
		})
	}
}
func TestReferenceRequiresExplicitLoopbackOrigin(t *testing.T) {
	for _, s := range []string{"https://127.0.0.1:10", "http://localhost:10", "http://example.com:10", "http://user:secret@127.0.0.1:10", "http://127.0.0.1:10/path", "http://127.0.0.1:10?token=secret"} {
		if _, err := reference.New(s); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
}
func TestReferenceOriginIsPartOfFrozenAdapterIdentity(t *testing.T) {
	a, err := reference.New("http://127.0.0.1:10001")
	if err != nil {
		t.Fatal(err)
	}
	b, err := reference.New("http://127.0.0.1:10002")
	if err != nil {
		t.Fatal(err)
	}
	if a.Adapter("reference", "same-name").Revision == b.Adapter("reference", "same-name").Revision {
		t.Fatal("changed origin reused frozen provider identity")
	}
}
