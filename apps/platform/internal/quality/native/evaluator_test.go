package native

import (
	"math"
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

func evaluateSingleRule(t *testing.T, rule Rule, ctx DatasetContext) domain.Finding {
	t.Helper()
	policy := Policy{}
	policy.Spec.Rules = []Rule{rule}
	findings, _, err := Evaluate(policy, ctx)
	if err != nil {
		t.Fatalf("evaluate rule %s: %v", rule.ID, err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	return findings[0]
}

func TestRangeRulesRejectNonFiniteCells(t *testing.T) {
	tests := []struct {
		name   string
		ruleID string
		column string
	}{
		{name: "activity score", ruleID: "QA-ACTIVITY-SCORE-RANGE", column: "activity_score"},
		{name: "indicator coverage", ruleID: "QA-INDICATOR-COVERAGE-RANGE", column: "indicator_coverage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, value := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "Infinity"} {
				ctx := DatasetContext{
					Table: tabular.Table{
						Headers: []string{tt.column},
						Rows:    []map[string]string{{tt.column: value}},
					},
				}
				finding := evaluateSingleRule(t, Rule{ID: tt.ruleID, Dimension: "accuracy", Severity: "CRITICAL"}, ctx)
				if finding.Status != domain.FindingFail {
					t.Fatalf("rule %s accepted non-finite value %q", tt.ruleID, value)
				}
			}

			ctx := DatasetContext{
				Table: tabular.Table{
					Headers: []string{tt.column},
					Rows:    []map[string]string{{tt.column: "97.5"}},
				},
			}
			finding := evaluateSingleRule(t, Rule{ID: tt.ruleID, Dimension: "accuracy", Severity: "CRITICAL"}, ctx)
			if finding.Status != domain.FindingPass {
				t.Fatalf("rule %s rejected a valid value: %#v", tt.ruleID, finding)
			}
		})
	}
}

func TestIndicatorCoveragePreservesNullSemantics(t *testing.T) {
	rule := Rule{ID: "QA-INDICATOR-COVERAGE-RANGE", Dimension: "CONFORMITY", Severity: "CRITICAL"}

	ctx := DatasetContext{
		Table: tabular.Table{
			Headers: []string{"indicator_coverage"},
			Rows: []map[string]string{
				{"indicator_coverage": "100"},
				{"indicator_coverage": ""},
				{"indicator_coverage": "   "},
				{"indicator_coverage": "33.33"},
			},
		},
	}
	finding := evaluateSingleRule(t, rule, ctx)
	if finding.Status != domain.FindingPass {
		t.Fatalf("null indicator_coverage must be allowed: %#v", finding)
	}
	if got := finding.Observed["missing"]; got != 2 {
		t.Fatalf("missing = %v, want 2", got)
	}
	if got := finding.Observed["invalid"]; got != 0 {
		t.Fatalf("invalid = %v, want 0", got)
	}

	bad := DatasetContext{
		Table: tabular.Table{
			Headers: []string{"indicator_coverage"},
			Rows: []map[string]string{
				{"indicator_coverage": ""},
				{"indicator_coverage": "101"},
			},
		},
	}
	if stopped := evaluateSingleRule(t, rule, bad); stopped.Status != domain.FindingFail {
		t.Fatalf("present out-of-range indicator_coverage must fail even beside nulls: %#v", stopped)
	}
}

func TestMetricRulesFailClosedOnNonFiniteEvidence(t *testing.T) {
	tests := []struct {
		name   string
		ruleID string
		key    string
	}{
		{name: "unresolved entity rate", ruleID: "QA-ENTITY-UNRESOLVED", key: "unresolvedEntityRate"},
		{name: "negative energy rate", ruleID: "QA-ENERGY-NONNEGATIVE", key: "acceptedNegativeEnergyRate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Rule{ID: tt.ruleID, Dimension: "accuracy", Severity: "CRITICAL"}

			nan := evaluateSingleRule(t, rule, DatasetContext{
				Metadata: map[string]any{tt.key: math.NaN()},
			})
			if nan.Status != domain.FindingFail {
				t.Fatalf("rule %s passed on NaN evidence: %#v", tt.ruleID, nan)
			}

			infinite := evaluateSingleRule(t, rule, DatasetContext{
				Metadata: map[string]any{tt.key: "Inf"},
			})
			if infinite.Status != domain.FindingFail {
				t.Fatalf("rule %s passed on infinite evidence: %#v", tt.ruleID, infinite)
			}

			// QA-ENERGY-NONNEGATIVE requires an exact zero, so only the
			// unresolved-entity rule has a passing numeric baseline to assert.
			if tt.key == "unresolvedEntityRate" {
				ok := evaluateSingleRule(t, rule, DatasetContext{
					Metadata: map[string]any{tt.key: 0.001},
				})
				if ok.Status != domain.FindingPass {
					t.Fatalf("rule %s rejected finite evidence: %#v", tt.ruleID, ok)
				}
			}
		})
	}
}

func TestValidateRejectsNonFiniteMetrics(t *testing.T) {
	if _, ok := numericMetadata(map[string]any{"rate": math.NaN()}, "rate"); ok {
		t.Fatal("numericMetadata accepted NaN")
	}
	if _, ok := numericMetadata(map[string]any{"rate": "Inf"}, "rate"); ok {
		t.Fatal("numericMetadata accepted +Inf")
	}
	if _, ok := numericMetadata(map[string]any{"rate": "-Inf"}, "rate"); ok {
		t.Fatal("numericMetadata accepted -Inf")
	}
	if value, ok := numericMetadata(map[string]any{"rate": "0.25"}, "rate"); !ok || value != 0.25 {
		t.Fatalf("numericMetadata rejected a finite value: value=%v ok=%v", value, ok)
	}
}
