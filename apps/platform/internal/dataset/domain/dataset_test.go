package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestDatasetVersionReadyThenInvalidate(t *testing.T) {
	version, err := NewVersion(uuid.New(), uuid.New(), 1, nil)
	if err != nil {
		t.Fatalf("NewVersion() error = %v", err)
	}
	if err := version.StartProcessing(); err != nil {
		t.Fatalf("StartProcessing() error = %v", err)
	}
	if err := version.MarkReady("OBJECT_STORAGE", "s3://bucket/object.csv", "text/csv", "SHA256", "abc123", 5, 100); err != nil {
		t.Fatalf("MarkReady() error = %v", err)
	}
	if version.Status != VersionReady {
		t.Fatalf("status = %s, want READY", version.Status)
	}
	if err := version.Invalidate("source file was incorrect"); err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}
	if version.Status != VersionInvalid {
		t.Fatalf("status = %s, want INVALID", version.Status)
	}
	if version.InvalidatedAt == nil {
		t.Fatal("InvalidatedAt should be set")
	}
}

func TestDatasetVersionRejectsInvalidTransitions(t *testing.T) {
	version, err := NewVersion(uuid.New(), uuid.New(), 1, nil)
	if err != nil {
		t.Fatalf("NewVersion() error = %v", err)
	}

	if err := version.Invalidate("too early"); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Invalidate() error = %v, want ErrInvalidTransition", err)
	}
	if err := version.MarkReady("OBJECT_STORAGE", "", "text/csv", "SHA256", "abc", 1, 10); !errors.Is(err, ErrInvalidReadyMetadata) {
		t.Fatalf("MarkReady() error = %v, want ErrInvalidReadyMetadata", err)
	}
	if err := version.MarkReady("OBJECT_STORAGE", "s3://bucket/object.csv", "text/csv", "SHA256", "abc", 1, 10); err != nil {
		t.Fatalf("MarkReady() error = %v", err)
	}
	if err := version.StartProcessing(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("StartProcessing() after READY error = %v, want ErrInvalidTransition", err)
	}
	if err := version.MarkFailed(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("MarkFailed() after READY error = %v, want ErrInvalidTransition", err)
	}
}

func TestDatasetValidation(t *testing.T) {
	_, err := NewDataset(uuid.New(), uuid.Nil, "DS", "Dataset", DatasetTypeRaw, nil, nil)
	if !errors.Is(err, ErrInvalidWorkspace) {
		t.Fatalf("NewDataset() error = %v, want ErrInvalidWorkspace", err)
	}
	_, err = NewDataset(uuid.New(), uuid.New(), "DS", "Dataset", DatasetType("UNKNOWN"), nil, nil)
	if !errors.Is(err, ErrInvalidDatasetType) {
		t.Fatalf("NewDataset() error = %v, want ErrInvalidDatasetType", err)
	}
}
