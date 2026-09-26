package application

import (
	"context"
	"testing"

	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
)

type testQualityEngine struct {
	descriptor qualityengine.Descriptor
}

func (e testQualityEngine) Descriptor() qualityengine.Descriptor { return e.descriptor }
func (testQualityEngine) Evaluate(context.Context, qualityengine.Request) (qualityengine.Result, error) {
	return qualityengine.Result{}, nil
}

func TestServiceRegistersAndSelectsQualityEngine(t *testing.T) {
	service := NewService("", nil, nil, nil, nil)
	custom := testQualityEngine{descriptor: qualityengine.Descriptor{Name: "REFERENCE", Version: "1"}}
	if err := service.RegisterEngine(custom); err != nil {
		t.Fatalf("register engine: %v", err)
	}
	selected, err := service.resolveEngine(" reference ")
	if err != nil {
		t.Fatalf("resolve custom engine: %v", err)
	}
	if selected.Descriptor().Name != "REFERENCE" {
		t.Fatalf("selected engine = %#v", selected.Descriptor())
	}
	nativeEngine, err := service.resolveEngine("")
	if err != nil {
		t.Fatalf("resolve default engine: %v", err)
	}
	if nativeEngine.Descriptor().Name != "native-quality" {
		t.Fatalf("default engine = %#v", nativeEngine.Descriptor())
	}
}

func TestServiceRejectsInvalidQualityEngineDescriptor(t *testing.T) {
	service := NewService("", nil, nil, nil, nil)
	for _, descriptor := range []qualityengine.Descriptor{
		{Name: "", Version: "1"},
		{Name: "REFERENCE", Version: ""},
	} {
		if err := service.RegisterEngine(testQualityEngine{descriptor: descriptor}); err == nil {
			t.Fatalf("descriptor %#v unexpectedly accepted", descriptor)
		}
	}
	if _, err := service.resolveEngine("missing"); err == nil {
		t.Fatal("unregistered engine unexpectedly resolved")
	}
}
