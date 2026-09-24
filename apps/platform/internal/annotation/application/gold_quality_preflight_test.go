package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

func TestEvaluateGoldQualityPreflightPassesCompleteAcceptAndCorrectSnapshot(t *testing.T) {
	workspaceID := uuid.New()
	campaignID := uuid.New()
	taskA := uuid.New()
	taskB := uuid.New()
	resultA := uuid.New()
	resultB := uuid.New()
	originalB := uuid.New()
	campaign := goldPreflightTestCampaign(workspaceID, campaignID)
	manifest := goldPreflightManifest(t, map[string]any{
		"tasks": []map[string]any{
			{"id": taskA.String(), "sourceItemRef": "row:1"},
			{"id": taskB.String(), "sourceItemRef": "row:2"},
		},
		"decisions": []map[string]any{
			{"id": uuid.NewString(), "taskId": taskA.String(), "outcome": "ACCEPT", "selectedResultId": resultA.String()},
			{"id": uuid.NewString(), "taskId": taskB.String(), "outcome": "CORRECT", "selectedResultId": resultB.String()},
		},
		"outputs": []map[string]any{
			{"taskId": taskA.String(), "selectedResultId": resultA.String()},
			{"taskId": taskB.String(), "selectedResultId": resultB.String()},
		},
	})
	selected := []annotationinfra.SnapshotSelectedResult{
		{TaskID: taskA, ResultID: resultA, CanonicalPayload: []byte(`{"label":"A"}`), CanonicalPayloadSHA256: strings.Repeat("a", 64)},
		{TaskID: taskB, ResultID: resultB, CanonicalPayload: []byte(`{"label":"B"}`), CanonicalPayloadSHA256: strings.Repeat("b", 64), CorrectedFromResultID: &originalB},
	}

	result, err := evaluateGoldQualityPreflight(campaign, uuid.New(), strings.Repeat("f", 64), manifest, selected)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if result.Blocking {
		t.Fatalf("complete snapshot unexpectedly blocking: %+v", result.Findings)
	}
	if result.Metrics["taskCount"] != 2 || result.Metrics["selectedCount"] != 2 ||
		result.Metrics["reviewedCount"] != 2 || result.Metrics["correctedCount"] != 1 ||
		result.Metrics["rejectedCount"] != 0 {
		t.Fatalf("unexpected metrics: %+v", result.Metrics)
	}
	assertGoldFindingStatus(t, result.Findings, "GOLD-ANNOTATION-COVERAGE", qualitydomain.FindingPass)
	assertGoldFindingStatus(t, result.Findings, "GOLD-REVIEWED-COVERAGE", qualitydomain.FindingPass)
	assertGoldFindingStatus(t, result.Findings, "GOLD-REJECTED-COUNT", qualitydomain.FindingPass)
	assertGoldFindingStatus(t, result.Findings, "GOLD-SCHEMA-VALIDITY", qualitydomain.FindingPass)
	assertGoldFindingStatus(t, result.Findings, "GOLD-AGREEMENT", qualitydomain.FindingSkipped)
}

func TestEvaluateGoldQualityPreflightBlocksRejectedDuplicateAndInvalidSelectedResult(t *testing.T) {
	workspaceID := uuid.New()
	campaignID := uuid.New()
	taskA := uuid.New()
	taskB := uuid.New()
	resultA := uuid.New()
	campaign := goldPreflightTestCampaign(workspaceID, campaignID)
	manifest := goldPreflightManifest(t, map[string]any{
		"tasks": []map[string]any{
			{"id": taskA.String(), "sourceItemRef": "row:1"},
			{"id": taskB.String(), "sourceItemRef": "row:1"},
		},
		"decisions": []map[string]any{
			{"id": uuid.NewString(), "taskId": taskA.String(), "outcome": "ACCEPT", "selectedResultId": resultA.String()},
			{"id": uuid.NewString(), "taskId": taskB.String(), "outcome": "REJECT"},
		},
		"outputs": []map[string]any{
			{"taskId": taskA.String(), "selectedResultId": resultA.String()},
		},
	})
	selected := []annotationinfra.SnapshotSelectedResult{
		{TaskID: taskA, ResultID: resultA, CanonicalPayload: []byte(`{"label":"C"}`), CanonicalPayloadSHA256: strings.Repeat("c", 64)},
	}

	result, err := evaluateGoldQualityPreflight(campaign, uuid.New(), strings.Repeat("f", 64), manifest, selected)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if !result.Blocking {
		t.Fatalf("invalid snapshot preflight must block: %+v", result.Findings)
	}
	if result.Metrics["rejectedCount"] != 1 || result.Metrics["schemaInvalidCount"] != 1 {
		t.Fatalf("unexpected blocking metrics: %+v", result.Metrics)
	}
	assertGoldFindingStatus(t, result.Findings, "GOLD-ANNOTATION-COVERAGE", qualitydomain.FindingFail)
	assertGoldFindingStatus(t, result.Findings, "GOLD-REJECTED-COUNT", qualitydomain.FindingFail)
	assertGoldFindingStatus(t, result.Findings, "GOLD-SOURCE-UNIQUENESS", qualitydomain.FindingFail)
	assertGoldFindingStatus(t, result.Findings, "GOLD-SCHEMA-VALIDITY", qualitydomain.FindingFail)
}

func TestEvaluateGoldQualityPreflightNeverTreatsEmptySnapshotAsPassing(t *testing.T) {
	campaign := goldPreflightTestCampaign(uuid.New(), uuid.New())
	manifest := goldPreflightManifest(t, map[string]any{
		"tasks": []any{}, "decisions": []any{}, "outputs": []any{},
	})
	result, err := evaluateGoldQualityPreflight(campaign, uuid.New(), strings.Repeat("f", 64), manifest, nil)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if !result.Blocking {
		t.Fatal("0/0 coverage must not pass Gold preflight")
	}
	assertGoldFindingStatus(t, result.Findings, "GOLD-ANNOTATION-COVERAGE", qualitydomain.FindingFail)
	assertGoldFindingStatus(t, result.Findings, "GOLD-REVIEWED-COVERAGE", qualitydomain.FindingFail)
}

func goldPreflightTestCampaign(workspaceID, campaignID uuid.UUID) annotationdomain.Campaign {
	schema := `{"kind":"single-label-v1","labels":["A","B"]}`
	return annotationdomain.Campaign{
		ID: campaignID,
		WorkspaceID: workspaceID,
		Schema: annotationdomain.FrozenSpec{
			Ref: "schema", Version: "1", ContentSHA256: strings.Repeat("1", 64), ContentSnapshot: schema,
		},
	}
}

func goldPreflightManifest(t *testing.T, value any) goldSnapshotManifest {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	var manifest goldSnapshotManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	return manifest
}

func assertGoldFindingStatus(t *testing.T, findings []qualitydomain.Finding, ruleID string, want qualitydomain.FindingStatus) {
	t.Helper()
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			if finding.Status != want {
				t.Fatalf("%s status=%s want=%s observed=%+v", ruleID, finding.Status, want, finding.Observed)
			}
			return
		}
	}
	t.Fatalf("missing finding %s", ruleID)
}
