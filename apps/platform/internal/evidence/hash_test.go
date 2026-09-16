package evidence

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEvidenceHashVerification(t *testing.T) {
	workspaceID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	sourceID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	createdBy := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	record := Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "PROCESSING_EXECUTION",
		Title:        "Workflow execution succeeded",
		SourceType:   "EXECUTION",
		SourceID:     &sourceID,
		StorageURI:   "s3://evidence/example.json",
		Metadata:     map[string]any{"attempt": 1, "workflowVersion": "1.0.0"},
		CreatedAt:    time.Date(2026, 9, 16, 10, 11, 12, 123456789, time.FixedZone("test", 8*60*60)),
		CreatedBy:    &createdBy,
	}

	v1Hash, err := ComputeHash(record, HashAlgorithmEvidenceV1)
	if err != nil {
		t.Fatalf("compute V1 hash: %v", err)
	}
	if !VerifyHash(record, HashAlgorithmEvidenceV1, v1Hash) {
		t.Fatal("V1 Evidence hash did not verify")
	}
	tampered := record
	tampered.Title = "tampered title"
	if VerifyHash(tampered, HashAlgorithmEvidenceV1, v1Hash) {
		t.Fatal("tampered V1 Evidence unexpectedly verified")
	}

	legacyHash, err := ComputeHash(record, HashAlgorithmLegacy)
	if err != nil {
		t.Fatalf("compute legacy hash: %v", err)
	}
	legacyTitleChange := record
	legacyTitleChange.Title = "legacy title was not hashed"
	if !VerifyHash(legacyTitleChange, HashAlgorithmLegacy, legacyHash) {
		t.Fatal("legacy metadata-only Evidence hash should ignore title")
	}
	legacyMetadataChange := record
	legacyMetadataChange.Metadata = map[string]any{"attempt": 2, "workflowVersion": "1.0.0"}
	if VerifyHash(legacyMetadataChange, HashAlgorithmLegacy, legacyHash) {
		t.Fatal("legacy Evidence with modified metadata unexpectedly verified")
	}
}
