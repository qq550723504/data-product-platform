package native

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

type DatasetContext struct {
	Table    tabular.Table
	Metadata map[string]any
	ReadyAt  *time.Time
	Now      time.Time
}

func Evaluate(policy Policy, ctx DatasetContext) ([]domain.Finding, map[string]any, error) {
	if ctx.Now.IsZero() {
		ctx.Now = time.Now().UTC()
	}
	metrics := map[string]any{}
	findings := make([]domain.Finding, 0, len(policy.Spec.Rules))
	for _, rule := range policy.Spec.Rules {
		finding, observed, err := evaluateRule(rule, ctx)
		if err != nil {
			return nil, nil, err
		}
		findings = append(findings, finding)
		metrics[rule.ID] = observed
	}
	return findings, metrics, nil
}

func evaluateRule(rule Rule, ctx DatasetContext) (domain.Finding, map[string]any, error) {
	finding := domain.Finding{
		RuleID:    rule.ID,
		Dimension: rule.Dimension,
		Severity:  rule.Severity,
		Status:    domain.FindingPass,
		Observed:  map[string]any{},
	}
	fail := func(message string, observed map[string]any) (domain.Finding, map[string]any, error) {
		finding.Status = domain.FindingFail
		finding.Message = message
		finding.Observed = observed
		return finding, observed, nil
	}
	pass := func(observed map[string]any) (domain.Finding, map[string]any, error) {
		finding.Observed = observed
		return finding, observed, nil
	}

	switch rule.ID {
	case "QA-COMPANY-ID-COMPLETE":
		total := len(ctx.Table.Rows)
		nonNull := 0
		for _, row := range ctx.Table.Rows {
			if firstValue(row, "company_id", "canonical_company_id") != "" {
				nonNull++
			}
		}
		rate := ratio(nonNull, total)
		observed := map[string]any{"nonNull": nonNull, "total": total, "rate": rate}
		if total == 0 || rate < 0.999 {
			return fail("company_id completeness is below 99.9%", observed)
		}
		return pass(observed)

	case "QA-CANONICAL-UNIQUE":
		seen := map[string]struct{}{}
		nonNull := 0
		for _, row := range ctx.Table.Rows {
			value := firstValue(row, "company_id", "canonical_company_id")
			if value == "" {
				continue
			}
			nonNull++
			seen[value] = struct{}{}
		}
		rate := ratio(len(seen), nonNull)
		observed := map[string]any{"unique": len(seen), "nonNull": nonNull, "rate": rate}
		if nonNull == 0 || rate < 0.99 {
			return fail("canonical company unique mapping rate is below 99%", observed)
		}
		return pass(observed)

	case "QA-ENTITY-UNRESOLVED":
		rate, ok := numericMetadata(ctx.Metadata, "unresolvedEntityRate")
		observed := map[string]any{"rate": rate, "available": ok}
		if !ok || rate > 0.005 {
			return fail("unresolved entity rate is unavailable or above 0.5%", observed)
		}
		return pass(observed)

	case "QA-ENERGY-NONNEGATIVE":
		rate, ok := numericMetadata(ctx.Metadata, "acceptedNegativeEnergyRate")
		observed := map[string]any{"rate": rate, "available": ok}
		if !ok || rate != 0 {
			return fail("accepted energy records contain negative values or evidence is unavailable", observed)
		}
		return pass(observed)

	case "QA-PERIOD-PRESENT":
		missing := 0
		for _, row := range ctx.Table.Rows {
			if firstValue(row, "period", "target_period") == "" {
				missing++
			}
		}
		observed := map[string]any{"missing": missing, "total": len(ctx.Table.Rows)}
		if len(ctx.Table.Rows) == 0 || missing != 0 {
			return fail("period must be present on every row", observed)
		}
		return pass(observed)

	case "QA-ACTIVITY-SCORE-RANGE":
		invalid := 0
		for _, row := range ctx.Table.Rows {
			value := strings.TrimSpace(row["activity_score"])
			if value == "" {
				continue
			}
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil || parsed < 0 || parsed > 100 {
				invalid++
			}
		}
		observed := map[string]any{"invalid": invalid, "total": len(ctx.Table.Rows)}
		if invalid != 0 {
			return fail("activity_score contains values outside 0..100", observed)
		}
		return pass(observed)

	case "QA-ACTIVITY-LEVEL-CONSISTENCY":
		invalid := 0
		for _, row := range ctx.Table.Rows {
			score := strings.TrimSpace(row["activity_score"])
			level := strings.TrimSpace(row["activity_level"])
			if score == "" {
				if level != "INSUFFICIENT_DATA" {
					invalid++
				}
				continue
			}
			if level != "HIGH" && level != "MEDIUM" && level != "LOW" {
				invalid++
			}
		}
		observed := map[string]any{"invalid": invalid, "total": len(ctx.Table.Rows)}
		if invalid != 0 {
			return fail("activity_level is inconsistent with activity_score", observed)
		}
		return pass(observed)

	case "QA-INDICATOR-COVERAGE-RANGE":
		invalid := 0
		for _, row := range ctx.Table.Rows {
			parsed, err := strconv.ParseFloat(strings.TrimSpace(row["indicator_coverage"]), 64)
			if err != nil || parsed < 0 || parsed > 100 {
				invalid++
			}
		}
		observed := map[string]any{"invalid": invalid, "total": len(ctx.Table.Rows)}
		if len(ctx.Table.Rows) == 0 || invalid != 0 {
			return fail("indicator_coverage contains values outside 0..100", observed)
		}
		return pass(observed)

	case "QA-FRESHNESS":
		if ctx.ReadyAt == nil {
			return fail("DatasetVersion ready timestamp is unavailable", map[string]any{"available": false})
		}
		hours := ctx.Now.Sub(ctx.ReadyAt.UTC()).Hours()
		if hours < 0 {
			hours = 0
		}
		observed := map[string]any{"freshnessHours": hours}
		if hours > 24 {
			return fail("DatasetVersion freshness exceeds 24 hours", observed)
		}
		return pass(observed)
	default:
		return domain.Finding{}, nil, fmt.Errorf("native quality engine does not implement rule %s", rule.ID)
	}
}

func firstValue(row map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(row[name]); value != "" {
			return value
		}
	}
	return ""
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func numericMetadata(metadata map[string]any, key string) (float64, bool) {
	if metadata == nil {
		return 0, false
	}
	value, ok := metadata[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}
