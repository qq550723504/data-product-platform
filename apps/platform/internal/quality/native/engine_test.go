package native

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
)

func TestEngineMatchesNativeEvaluationContract(t *testing.T) {
	content := []byte(`apiVersion: quality/v1
kind: QualityRuleSet
metadata:
  name: engine-contract
  version: 1.0.0
spec:
  rules:
    - id: QA-ID
      dimension: COMPLETENESS
      type: not_null
      target: company_id
      threshold: 1
      required: true
      severity: CRITICAL
  gate:
    criticalFailure: FAIL
    highFailure: REVIEW
    warningFailure: PASS_WITH_WARNING
`)
	table, err := tabular.ReadCSV(strings.NewReader("company_id\nCOMPANY-1\n\n"))
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	policy, err := LoadPolicyBytes(content, "engine-contract.yaml")
	if err != nil {
		t.Fatalf("load policy bytes: %v", err)
	}
	directFindings, directMetrics, err := Evaluate(policy, DatasetContext{Table: table})
	if err != nil {
		t.Fatalf("direct evaluate: %v", err)
	}
	directGate, err := policy.GateDecision(directFindings)
	if err != nil {
		t.Fatalf("direct gate: %v", err)
	}

	provider := NewEngine()
	result, err := provider.Evaluate(context.Background(), qualityengine.Request{
		AttemptID:        uuid.New(),
		DatasetVersionID: uuid.New(),
		RuleSet: qualityengine.RuleSet{
			Ref:     "engine-contract.yaml",
			Content: content,
		},
		Dataset: qualityengine.DatasetContext{Table: table},
	})
	if err != nil {
		t.Fatalf("engine evaluate: %v", err)
	}
	if result.RuleSetVersion != policy.Metadata.Version {
		t.Fatalf("rule set version = %q, want %q", result.RuleSetVersion, policy.Metadata.Version)
	}
	if result.RuleSetContentSHA256 != policy.SourceContentSHA256 {
		t.Fatalf("rule set hash = %q, want %q", result.RuleSetContentSHA256, policy.SourceContentSHA256)
	}
	if len(result.Findings) != len(directFindings) || result.Findings[0].Status != directFindings[0].Status {
		t.Fatalf("engine findings = %#v, direct = %#v", result.Findings, directFindings)
	}
	if result.GateDecision != directGate {
		t.Fatalf("engine gate = %s, direct = %s", result.GateDecision, directGate)
	}
	if result.Metrics["QA-ID"] == nil || directMetrics["QA-ID"] == nil {
		t.Fatalf("normalized metrics missing: engine=%#v direct=%#v", result.Metrics, directMetrics)
	}
	descriptor := provider.Descriptor()
	if descriptor.Name != EvaluatorName || descriptor.Version != EvaluatorVersion || len(descriptor.Capabilities) == 0 {
		t.Fatalf("unexpected descriptor: %#v", descriptor)
	}
}
