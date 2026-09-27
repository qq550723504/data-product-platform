package gx

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/native"
)

func TestGXLiveReferenceMatchesNativePassFailSemantics(t *testing.T) {
	baseURL := os.Getenv("TEST_GX_URL")
	if baseURL == "" {
		t.Skip("TEST_GX_URL is not set")
	}
	client, err := NewClient(Config{
		BaseURL: baseURL, ExpectedEngineVersion: "1.23.2",
	}, nil)
	if err != nil {
		t.Fatalf("new GX client: %v", err)
	}
	ctx := context.Background()
	if err := client.Probe(ctx); err != nil {
		t.Fatalf("probe GX: %v", err)
	}

	ruleSet := []byte(`apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: gx-live-contract
  version: 1.0.0
spec:
  rules:
    - id: NOT_NULL
      stage: PRODUCT
      dimension: COMPLETENESS
      type: not_null
      target: id
      threshold: 1
      required: true
      severity: CRITICAL
    - id: COMPLETE
      stage: PRODUCT
      dimension: COMPLETENESS
      type: completeness_ratio
      target: id
      threshold: 0.5
      required: true
      severity: CRITICAL
    - id: UNIQUE
      stage: PRODUCT
      dimension: UNIQUENESS
      type: unique
      target: id
      threshold: 1
      required: true
      severity: HIGH
    - id: DUPLICATE_RATIO
      stage: PRODUCT
      dimension: UNIQUENESS
      type: duplicate_ratio
      target: id
      threshold: 1
      required: true
      severity: HIGH
    - id: RANGE
      stage: PRODUCT
      dimension: ACCURACY
      type: range
      target: score
      parameters:
        min: 0
        max: 100
        allowNull: false
      required: true
      severity: CRITICAL
    - id: ENUM
      stage: PRODUCT
      dimension: CONSISTENCY
      type: enum
      target: level
      parameters:
        values: [HIGH, LOW]
        allowNull: false
      required: true
      severity: CRITICAL
  gate:
    criticalFailure: FAIL
    highFailure: REVIEW
    warningFailure: PASS_WITH_WARNING
`)
	request := qualityengine.Request{
		AttemptID: uuid.New(), DatasetVersionID: uuid.New(),
		RuleSet: qualityengine.RuleSet{Ref: "quality/gx-live.yaml", Content: ruleSet},
		Dataset: qualityengine.DatasetContext{Table: tabular.Table{
			Headers: []string{"id", "score", "level"},
			Rows: []map[string]string{
				{"id": "A", "score": "10", "level": "HIGH"},
				{"id": "", "score": "200", "level": "OTHER"},
				{"id": "A", "score": "50", "level": "LOW"},
			},
		}},
	}

	nativeResult, err := native.NewEngine().Evaluate(ctx, request)
	if err != nil {
		t.Fatalf("native evaluate: %v", err)
	}
	gxResult, err := client.Evaluate(ctx, request)
	if err != nil {
		t.Fatalf("GX evaluate: %v", err)
	}
	nativeStatus := findingStatusByRule(nativeResult)
	gxStatus := findingStatusByRule(gxResult)
	if len(nativeStatus) != len(gxStatus) {
		t.Fatalf("finding counts differ: native=%v gx=%v", nativeStatus, gxStatus)
	}
	for ruleID, want := range nativeStatus {
		if got, ok := gxStatus[ruleID]; !ok || got != want {
			t.Fatalf("rule %s status: GX=%s native=%s (all GX=%v)", ruleID, got, want, gxStatus)
		}
	}
}

func findingStatusByRule(result qualityengine.Result) map[string]string {
	statuses := make(map[string]string, len(result.Findings))
	for _, finding := range result.Findings {
		statuses[finding.RuleID] = string(finding.Status)
	}
	return statuses
}
