package ports_test

import (
	"errors"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/testkit"
	"testing"
)

func TestRegistryRequiresAuthoritativeStableCatalogAndNoDelivery(t *testing.T) {
	a := testkit.New().Adapter("test", "instance")
	for _, change := range []func(*ports.Adapter){func(a *ports.Adapter) { a.Instance = "" }, func(a *ports.Adapter) { a.Capabilities.AuthoritativeLookup = false }, func(a *ports.Adapter) { a.Capabilities.StableRequestKeys = false }, func(a *ports.Adapter) { a.Capabilities.DataDelivery = true }, func(a *ports.Adapter) { a.Catalog = nil }} {
		copy := a
		change(&copy)
		if _, err := ports.NewRegistry(copy); !errors.Is(err, d.ErrUnsupported) {
			t.Fatalf("unsafe registry entry: %v", err)
		}
	}
	if _, err := ports.NewRegistry(a, a); !errors.Is(err, d.ErrConflict) {
		t.Fatalf("duplicate provider: %v", err)
	}
	r, err := ports.NewRegistry()
	if err != nil || len(r.List()) != 0 {
		t.Fatal("empty default registry invalid")
	}
	if _, err = r.Get("missing"); !errors.Is(err, d.ErrUnsupported) {
		t.Fatal("unconfigured provider enabled")
	}
}
