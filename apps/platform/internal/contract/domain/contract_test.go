package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestContractVersionPublishLifecycle(t *testing.T) {
	version, err := NewContractVersion(uuid.New(), 1, 0, 0, map[string]any{"spec": map[string]any{"product": "test"}}, "contract.yaml", "abc123", nil)
	if err != nil {
		t.Fatalf("new contract version: %v", err)
	}
	if version.Status != VersionDraft || version.Semver() != "1.0.0" {
		t.Fatalf("initial version = %s/%s, want 1.0.0/DRAFT", version.Semver(), version.Status)
	}
	if err := version.Publish(nil); err != nil {
		t.Fatalf("publish contract version: %v", err)
	}
	if version.Status != VersionPublished || version.PublishedAt == nil {
		t.Fatalf("published version = %s publishedAt %v", version.Status, version.PublishedAt)
	}
	if err := version.Publish(nil); err != ErrInvalidTransition {
		t.Fatalf("second publish error = %v, want ErrInvalidTransition", err)
	}
}
