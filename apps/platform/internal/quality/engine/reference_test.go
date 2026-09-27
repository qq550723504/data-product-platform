package engine

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSafeReferenceNeverPersistsRawProviderText(t *testing.T) {
	secret := "https://provider.internal/run/123?token=super-secret"
	got := SafeReference(secret)
	if got == "" || got == secret || strings.Contains(got, "provider.internal") || strings.Contains(got, "super-secret") {
		t.Fatalf("unsafe reference normalization: %q", got)
	}
	if !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("reference = %q, want sha256 digest", got)
	}

	id := uuid.New()
	if canonical := SafeReference(id.String()); canonical != id.String() {
		t.Fatalf("uuid reference = %q, want %q", canonical, id.String())
	}
	if SafeReference("   ") != "" {
		t.Fatal("blank reference should remain empty")
	}
}
