package infrastructure

import (
	"encoding/json"
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

func TestDecodeJSONNumbersPreservesAssessmentFacts(t *testing.T) {
	var decoded map[string]any
	if err := decodeJSONNumbers([]byte(`{"metric":9007199254740993,"overflow":1e400}`), &decoded); err != nil {
		t.Fatalf("decode assessment JSON: %v", err)
	}
	if got, ok := decoded["metric"].(json.Number); !ok || got.String() != "9007199254740993" {
		t.Fatalf("metric = %#v, want exact JSON number", decoded["metric"])
	}
	if got, ok := decoded["overflow"].(json.Number); !ok || got.String() != "1e400" {
		t.Fatalf("overflow = %#v, want exact JSON number", decoded["overflow"])
	}
}

func TestRestoreDimensionSummariesUsesPersistedSnapshot(t *testing.T) {
	result := domain.Assessment{
		Metrics: map[string]any{
			"dimensions": map[string]any{
				"COMPLETENESS": map[string]any{
					"dimension":      "COMPLETENESS",
					"status":         "PASS",
					"ruleCount":      3,
					"evaluatedCount": 2,
					"failedCount":    0,
				},
			},
		},
		Findings: []domain.Finding{{
			Dimension: "COMPLETENESS",
			Severity:  "CRITICAL",
			Status:    domain.FindingFail,
		}},
	}

	if err := restoreDimensionSummaries(&result); err != nil {
		t.Fatalf("restore dimension summaries: %v", err)
	}
	summary := result.DimensionSummaries["COMPLETENESS"]
	if summary.Status != domain.DimensionPass || summary.RuleCount != 3 || summary.EvaluatedCount != 2 {
		t.Fatalf("summary = %#v, want persisted snapshot", summary)
	}
}

func TestRestoreDimensionSummariesRejectsMissingSnapshot(t *testing.T) {
	result := domain.Assessment{
		Metrics: map[string]any{},
		Findings: []domain.Finding{{
			Dimension: "COMPLETENESS",
			Severity:  "CRITICAL",
			Status:    domain.FindingFail,
		}},
	}

	if err := restoreDimensionSummaries(&result); err == nil {
		t.Fatal("missing persisted dimension snapshot was accepted")
	}
}
