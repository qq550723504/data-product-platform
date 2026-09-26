package native

import (
	"context"

	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
)

type Engine struct{}

func NewEngine() Engine { return Engine{} }

func (Engine) Descriptor() qualityengine.Descriptor {
	return qualityengine.Descriptor{
		Name:    EvaluatorName,
		Version: EvaluatorVersion,
		Capabilities: []string{
			"not_null", "completeness_ratio", "unique", "duplicate_ratio",
			"range", "enum", "regex", "freshness", "reference_match",
			"reconciliation", "conditional_consistency", "lineage_present",
			"evidence_present",
		},
	}
}

func (Engine) Evaluate(_ context.Context, request qualityengine.Request) (qualityengine.Result, error) {
	policy, err := LoadPolicyBytes(request.RuleSet.Content, request.RuleSet.Ref)
	if err != nil {
		return qualityengine.Result{}, err
	}
	findings, metrics, err := Evaluate(policy, DatasetContext{
		Table:           request.Dataset.Table,
		Metadata:        request.Dataset.Metadata,
		ReadyAt:         request.Dataset.ReadyAt,
		Now:             request.Dataset.Now,
		LineagePresent:  request.Dataset.LineagePresent,
		EvidencePresent: request.Dataset.EvidencePresent,
	})
	if err != nil {
		return qualityengine.Result{}, err
	}
	gate, err := policy.GateDecision(findings)
	if err != nil {
		return qualityengine.Result{}, err
	}
	return qualityengine.Result{
		RuleSetVersion:       policy.Metadata.Version,
		RuleSetContentSHA256: policy.SourceContentSHA256,
		Findings:             findings,
		Metrics:              metrics,
		GateDecision:         gate,
		ExecutionMetadata: map[string]any{
			"engine":  EvaluatorName,
			"version": EvaluatorVersion,
		},
	}, nil
}
