package application

import (
	"errors"
	"testing"
)

func TestAnnotationEngineErrorDoesNotLeakProviderCause(t *testing.T) {
	err := NewAnnotationEngineError(
		ErrAnnotationEngineUnavailable,
		"submit tasks",
		true,
		503,
		errors.New("provider secret response"),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "submit tasks: annotation engine unavailable" {
		t.Fatalf("error = %q", got)
	}
	if !errors.Is(err, ErrAnnotationEngineUnavailable) {
		t.Fatal("expected error kind to unwrap")
	}
}
