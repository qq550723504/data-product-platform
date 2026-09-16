package native

import (
	"strings"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
)

func Evaluate(policy Policy, table tabular.Table) ([]domain.Finding, map[string]any) {
	policyByName := map[string]FieldPolicy{}
	for _, fieldPolicy := range policy.Spec.FieldPolicies {
		for _, name := range fieldPolicy.Match.Names {
			policyByName[strings.TrimSpace(name)] = fieldPolicy
		}
	}

	findings := make([]domain.Finding, 0, len(table.Headers))
	kept := 0
	blocked := 0
	reviewed := 0
	for _, rawField := range table.Headers {
		field := canonicalProductField(strings.TrimSpace(rawField))
		fieldPolicy, ok := policyByName[field]
		if !ok {
			action := strings.ToUpper(strings.TrimSpace(policy.Spec.ProductOutput.UnknownFieldAction))
			if action == "" {
				action = "REVIEW"
			}
			finding := domain.Finding{
				FieldName: rawField,
				Action:    action,
				Status:    domain.FindingReview,
				Message:   "field is not covered by the committed product compliance policy",
			}
			if action == "BLOCK" || action == "REMOVE" {
				finding.Status = domain.FindingFail
				blocked++
			} else {
				reviewed++
			}
			findings = append(findings, finding)
			continue
		}

		action := strings.ToUpper(strings.TrimSpace(fieldPolicy.Action))
		finding := domain.Finding{
			FieldName: rawField,
			Category:  fieldPolicy.Category,
			Action:    action,
			Status:    domain.FindingPass,
			Message:   fieldPolicy.Reason,
		}
		switch action {
		case "KEEP":
			kept++
		case "REVIEW":
			finding.Status = domain.FindingReview
			reviewed++
		case "REMOVE", "MASK", "TOKENIZE", "PSEUDONYMIZE", "ANONYMIZE", "AGGREGATE", "BLOCK":
			// At product-output stage these actions mean the field must not survive as-is.
			finding.Status = domain.FindingFail
			blocked++
		default:
			finding.Status = domain.FindingReview
			reviewed++
		}
		findings = append(findings, finding)
	}

	summary := map[string]any{
		"fieldCount": len(table.Headers),
		"kept":       kept,
		"blocked":    blocked,
		"review":     reviewed,
		"principle":  policy.Spec.Principle,
	}
	return findings, summary
}

func canonicalProductField(field string) string {
	switch field {
	case "canonical_company_id":
		return "company_id"
	case "target_period":
		return "period"
	default:
		return field
	}
}
