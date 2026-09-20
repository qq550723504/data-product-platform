package infrastructure

import (
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

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

func TestRestoreDimensionSummariesFallsBackForLegacyRows(t *testing.T) {
	result := domain.Assessment{
		Metrics: map[string]any{"legacy": true},
		Findings: []domain.Finding{{
			Dimension: "COMPLETENESS",
			Severity:  "CRITICAL",
			Status:    domain.FindingFail,
		}},
	}

	if err := restoreDimensionSummaries(&result); err != nil {
		t.Fatalf("restore legacy dimension summaries: %v", err)
	}
	if got := result.DimensionSummaries["COMPLETENESS"].Status; got != domain.DimensionFail {
		t.Fatalf("legacy summary status = %s, want FAIL", got)
	}
}
