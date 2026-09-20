package evidence

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
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

func TestEvidenceHashVerificationSurvivesQualityMetricsJSONRoundTrip(t *testing.T) {
	workspaceID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	createdBy := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	metadata := map[string]any{
		"datasetVersionId": uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		"metrics": map[string]any{
			"QA-COMPANY-ID-COMPLETE": map[string]any{
				"observedValue": 1,
				"threshold":     0.999,
				"affectedCount": 0,
			},
			"dimensions": map[string]qualitydomain.DimensionSummary{
				"COMPLETENESS": {
					Dimension:      "COMPLETENESS",
					Status:         qualitydomain.DimensionPass,
					RuleCount:      1,
					EvaluatedCount: 1,
					FailedCount:    0,
				},
			},
		},
	}
	record := Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "QUALITY_RESULT",
		SourceType:   "QUALITY_RESULT",
		Metadata:     metadata,
		CreatedAt:    time.Date(2026, 9, 20, 10, 11, 12, 123456789, time.UTC),
		CreatedBy:    &createdBy,
	}
	hashValue, err := ComputeHash(record, HashAlgorithmEvidenceV1)
	if err != nil {
		t.Fatalf("compute quality evidence hash: %v", err)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal quality evidence metadata: %v", err)
	}
	var roundTripped map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&roundTripped); err != nil {
		t.Fatalf("unmarshal quality evidence metadata: %v", err)
	}
	record.Metadata = roundTripped
	if !VerifyHash(record, HashAlgorithmEvidenceV1, hashValue) {
		t.Fatalf("quality evidence hash did not survive JSON round trip: hash=%s metadata=%#v", hashValue, roundTripped)
	}
}

func TestEvidenceHashPreservesLargeJSONIntegers(t *testing.T) {
	base := Record{
		WorkspaceID:  uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		EvidenceType: "QUALITY_RESULT",
		SourceType:   "QUALITY_RESULT",
		CreatedAt:    time.Date(2026, 9, 20, 10, 11, 12, 0, time.UTC),
		Metadata:     map[string]any{"large": json.Number("9007199254740993")},
	}
	highHash, err := ComputeHash(base, HashAlgorithmEvidenceV1)
	if err != nil {
		t.Fatalf("compute high integer hash: %v", err)
	}
	encoded, err := json.Marshal(base.Metadata)
	if err != nil {
		t.Fatalf("marshal large integer metadata: %v", err)
	}
	var roundTripped map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&roundTripped); err != nil {
		t.Fatalf("decode large integer metadata: %v", err)
	}
	base.Metadata = roundTripped
	if !VerifyHash(base, HashAlgorithmEvidenceV1, highHash) {
		t.Fatalf("large integer hash did not survive JSON round trip: metadata=%#v", roundTripped)
	}

	base.Metadata = map[string]any{"large": json.Number("9007199254740992")}
	lowHash, err := ComputeHash(base, HashAlgorithmEvidenceV1)
	if err != nil {
		t.Fatalf("compute low integer hash: %v", err)
	}
	if highHash == lowHash {
		t.Fatalf("distinct large integers produced the same hash: %s", highHash)
	}
}
