package reference_test

import (
	"context"
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

func TestUsageSharedContracts(t *testing.T) {
	for _, mode := range []string{"fake", "reference"} {
		t.Run(mode, func(t *testing.T) {
			contracttest.RunUsage(t, func(t *testing.T) (ports.UsageEventSource, func() int64) {
				s := &testkit.UsageSource{}
				if mode == "fake" {
					return s, s.Reads.Load
				}
				server := s.Server()
				t.Cleanup(server.Close)
				c, err := reference.New(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				return c, s.Reads.Load
			})
		})
	}
}
func TestUsageReferenceRejectsUntrustedEnvelope(t *testing.T) {
	for _, body := range []string{`{"secret":"leak"}`, `{} {}`, strings.Repeat("x", 128*1024+1)} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		c, err := reference.New(s.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Read(context.Background(), d.UsageReadRequest{Limit: d.UsagePageLimit}, ports.Invocation{ID: uuid.New(), Dispatch: func(context.Context) error { return nil }})
		s.Close()
		if err == nil {
			t.Fatal("invalid/oversized response accepted")
		}
	}
}
