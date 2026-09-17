package application

import (
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

func TestManagedEngineType(t *testing.T) {
	version := domain.WorkflowVersion{
		Definition: map[string]any{
			"spec": map[string]any{
				"managedExecution": map[string]any{
					"engine": " hop ",
				},
			},
		},
	}
	if got := ManagedEngineType(version); got != "HOP" {
		t.Fatalf("expected HOP, got %q", got)
	}
}

func TestManagedEngineTypeReturnsEmptyForNativeDefinition(t *testing.T) {
	version := domain.WorkflowVersion{
		Definition: map[string]any{
			"spec": map[string]any{
				"tasks": []any{},
			},
		},
	}
	if got := ManagedEngineType(version); got != "" {
		t.Fatalf("expected empty engine, got %q", got)
	}
}
