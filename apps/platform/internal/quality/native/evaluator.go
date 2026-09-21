package native

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

type DatasetContext struct {
	Table           tabular.Table
	Metadata        map[string]any
	ReadyAt         *time.Time
	Now             time.Time
	LineagePresent  *bool
	EvidencePresent *bool
}

func Evaluate(policy Policy, ctx DatasetContext) ([]domain.Finding, map[string]any, error) {
	if err := validateRules(policy.Spec.Rules); err != nil {
		return nil, nil, err
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
	rule.Type = strings.ToLower(strings.TrimSpace(rule.Type))
	rule.Dimension = normalizeDimension(rule.Dimension)
	rule.Severity = normalizeSeverity(rule.Severity)
	finding := domain.Finding{RuleID: rule.ID, Dimension: rule.Dimension, Severity: rule.Severity, Status: domain.FindingPass, Observed: map[string]any{}}
	pass := func(observed map[string]any, value any, threshold any, affected int, samples []any) (domain.Finding, map[string]any, error) {
		finding.Observed = standardObservation(observed, value, threshold, affected, samples)
		addRuleContext(finding.Observed, rule)
		return finding, finding.Observed, nil
	}
	fail := func(message string, observed map[string]any, value any, threshold any, affected int, samples []any) (domain.Finding, map[string]any, error) {
		finding.Status = domain.FindingFail
		finding.Message = message
		finding.Observed = standardObservation(observed, value, threshold, affected, samples)
		addRuleContext(finding.Observed, rule)
		return finding, finding.Observed, nil
	}
	skip := func(message string, observed map[string]any) (domain.Finding, map[string]any, error) {
		finding.Status = domain.FindingSkipped
		finding.Message = message
		finding.Observed = standardObservation(observed, nil, nil, 0, nil)
		addRuleContext(finding.Observed, rule)
		return finding, finding.Observed, nil
	}

	switch rule.Type {
	case RuleTypeNotNull, RuleTypeCompletenessRatio:
		if result, ok, err := targetAvailability(rule, ctx, skip, fail); err != nil || ok {
			return result, result.Observed, err
		}
		threshold, err := ruleThreshold(rule, 1)
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		nonNull := 0
		samples := make([]any, 0, 5)
		for index, row := range ctx.Table.Rows {
			if strings.TrimSpace(row[rule.Target]) != "" {
				nonNull++
			} else if len(samples) < 5 {
				samples = append(samples, map[string]any{"row": index, "field": rule.Target})
			}
		}
		rate := ratio(nonNull, len(ctx.Table.Rows))
		observed := map[string]any{"nonNull": nonNull, "total": len(ctx.Table.Rows), "rate": rate}
		if rate < threshold {
			return fail(fmt.Sprintf("%s completeness %.6f is below %.6f", rule.Target, rate, threshold), observed, rate, threshold, len(ctx.Table.Rows)-nonNull, samples)
		}
		return pass(observed, rate, threshold, 0, nil)

	case RuleTypeUnique, RuleTypeDuplicateRatio:
		if result, ok, err := targetAvailability(rule, ctx, skip, fail); err != nil || ok {
			return result, result.Observed, err
		}
		seen := map[string]struct{}{}
		nonNull := 0
		samples := make([]any, 0, 5)
		for index, row := range ctx.Table.Rows {
			value := strings.TrimSpace(row[rule.Target])
			if value == "" {
				continue
			}
			nonNull++
			if _, exists := seen[value]; exists && len(samples) < 5 {
				samples = append(samples, map[string]any{"row": index, "field": rule.Target})
			}
			seen[value] = struct{}{}
		}
		if nonNull == 0 {
			if rule.Required {
				return fail(fmt.Sprintf("%s has no non-null values to evaluate uniqueness", rule.Target), map[string]any{"nonNull": 0}, 0, 1, 0, nil)
			}
			return skip("rule not applicable: no non-null values", map[string]any{"nonNull": 0})
		}
		uniqueRatio := ratio(len(seen), nonNull)
		duplicateRatio := ratio(nonNull-len(seen), nonNull)
		if rule.Type == RuleTypeUnique {
			threshold, err := ruleThreshold(rule, 1)
			if err != nil {
				return domain.Finding{}, nil, fmt.Errorf("rule %s: %w", rule.ID, err)
			}
			observed := map[string]any{"unique": len(seen), "nonNull": nonNull, "rate": uniqueRatio, "duplicateRate": duplicateRatio}
			if uniqueRatio < threshold {
				return fail(fmt.Sprintf("%s uniqueness %.6f is below %.6f", rule.Target, uniqueRatio, threshold), observed, uniqueRatio, threshold, nonNull-len(seen), samples)
			}
			return pass(observed, uniqueRatio, threshold, 0, nil)
		}
		threshold, err := ruleThreshold(rule, 0)
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		observed := map[string]any{"unique": len(seen), "nonNull": nonNull, "rate": uniqueRatio, "duplicateRate": duplicateRatio}
		if duplicateRatio > threshold {
			return fail(fmt.Sprintf("%s duplicate ratio %.6f is above %.6f", rule.Target, duplicateRatio, threshold), observed, duplicateRatio, threshold, nonNull-len(seen), samples)
		}
		return pass(observed, duplicateRatio, threshold, 0, nil)

	case RuleTypeRange:
		if result, ok, err := targetAvailability(rule, ctx, skip, fail); err != nil || ok {
			return result, result.Observed, err
		}
		minimumValue, err := parameterValue(rule, "min")
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: range min: %w", rule.ID, err)
		}
		maximumValue, err := parameterValue(rule, "max")
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: range max: %w", rule.ID, err)
		}
		minimum, err := numericRat(minimumValue)
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: range min: %w", rule.ID, err)
		}
		maximum, err := numericRat(maximumValue)
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: range max: %w", rule.ID, err)
		}
		minimumFloat, _ := minimum.Float64()
		maximumFloat, _ := maximum.Float64()
		invalid := 0
		samples := make([]any, 0, 5)
		for index, row := range ctx.Table.Rows {
			value := strings.TrimSpace(row[rule.Target])
			allowNull, err := parameterBool(rule, "allowNull", true)
			if err != nil {
				return domain.Finding{}, nil, fmt.Errorf("rule %s allowNull: %w", rule.ID, err)
			}
			if value == "" && allowNull {
				continue
			}
			parsed, parseErr := decimalRat(value)
			if parseErr != nil || parsed.Cmp(minimum) < 0 || parsed.Cmp(maximum) > 0 {
				invalid++
				if len(samples) < 5 {
					samples = append(samples, map[string]any{"row": index, "field": rule.Target})
				}
			}
		}
		minimumObserved := ratJSONNumber(minimum)
		maximumObserved := ratJSONNumber(maximum)
		observed := map[string]any{"invalid": invalid, "total": len(ctx.Table.Rows), "min": minimumObserved, "max": maximumObserved}
		threshold := map[string]any{"min": minimumObserved, "max": maximumObserved}
		if invalid > 0 {
			return fail(fmt.Sprintf("%s contains values outside %.6f..%.6f", rule.Target, minimumFloat, maximumFloat), observed, invalid, threshold, invalid, samples)
		}
		return pass(observed, 0, threshold, 0, nil)

	case RuleTypeEnum:
		if result, ok, err := targetAvailability(rule, ctx, skip, fail); err != nil || ok {
			return result, result.Observed, err
		}
		allowed := parameterStrings(rule, "values", "allowedValues")
		if len(allowed) == 0 {
			return domain.Finding{}, nil, fmt.Errorf("rule %s enum values are required", rule.ID)
		}
		allowedSet := make(map[string]struct{}, len(allowed))
		for _, value := range allowed {
			allowedSet[value] = struct{}{}
		}
		invalid := 0
		samples := make([]any, 0, 5)
		for index, row := range ctx.Table.Rows {
			value := strings.TrimSpace(row[rule.Target])
			allowNull, err := parameterBool(rule, "allowNull", true)
			if err != nil {
				return domain.Finding{}, nil, fmt.Errorf("rule %s allowNull: %w", rule.ID, err)
			}
			if value == "" && allowNull {
				continue
			}
			if _, ok := allowedSet[value]; !ok {
				invalid++
				if len(samples) < 5 {
					samples = append(samples, map[string]any{"row": index, "field": rule.Target})
				}
			}
		}
		observed := map[string]any{"invalid": invalid, "total": len(ctx.Table.Rows), "allowedValues": allowed}
		if invalid > 0 {
			return fail(fmt.Sprintf("%s contains values outside the declared enum", rule.Target), observed, invalid, allowed, invalid, samples)
		}
		return pass(observed, 0, allowed, 0, nil)

	case RuleTypeRegex:
		if result, ok, err := targetAvailability(rule, ctx, skip, fail); err != nil || ok {
			return result, result.Observed, err
		}
		pattern := parameterString(rule, "pattern")
		re, err := regexp.Compile(pattern)
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s regex pattern: %w", rule.ID, err)
		}
		invalid := 0
		samples := make([]any, 0, 5)
		for index, row := range ctx.Table.Rows {
			value := strings.TrimSpace(row[rule.Target])
			allowNull, err := parameterBool(rule, "allowNull", true)
			if err != nil {
				return domain.Finding{}, nil, fmt.Errorf("rule %s allowNull: %w", rule.ID, err)
			}
			if value == "" && allowNull {
				continue
			}
			if !re.MatchString(value) {
				invalid++
				if len(samples) < 5 {
					samples = append(samples, map[string]any{"row": index, "field": rule.Target})
				}
			}
		}
		observed := map[string]any{"invalid": invalid, "total": len(ctx.Table.Rows), "pattern": pattern}
		if invalid > 0 {
			return fail(fmt.Sprintf("%s contains values that do not match the declared pattern", rule.Target), observed, invalid, pattern, invalid, samples)
		}
		return pass(observed, 0, pattern, 0, nil)

	case RuleTypeFreshness:
		if ctx.ReadyAt == nil || ctx.Now.IsZero() {
			if rule.Required {
				return fail("freshness requires DatasetVersion ready time and explicit evaluation time", map[string]any{"readyAtAvailable": ctx.ReadyAt != nil, "evaluationTimeAvailable": !ctx.Now.IsZero()}, nil, nil, 1, nil)
			}
			return skip("rule not applicable: freshness timestamps are unavailable", map[string]any{"readyAtAvailable": ctx.ReadyAt != nil, "evaluationTimeAvailable": !ctx.Now.IsZero()})
		}
		threshold, err := ruleThreshold(rule, 24)
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		hours := ctx.Now.Sub(ctx.ReadyAt.UTC()).Hours()
		if hours < 0 {
			hours = 0
		}
		observed := map[string]any{"freshnessHours": hours}
		if hours > threshold {
			return fail(fmt.Sprintf("DatasetVersion freshness %.6f hours exceeds %.6f", hours, threshold), observed, hours, threshold, 1, nil)
		}
		return pass(observed, hours, threshold, 0, nil)

	case RuleTypeReferenceMatch, RuleTypeReconciliation:
		metric := parameterString(rule, "metric", "metadataKey")
		valueRat, ok := numericMetadataRat(ctx.Metadata, metric)
		thresholdRat, err := ruleThresholdRat(rule, 0)
		if err != nil {
			return domain.Finding{}, nil, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
		operator := strings.ToLower(parameterString(rule, "operator"))
		var valueObserved any
		if ok {
			valueObserved = ratJSONNumber(valueRat)
		}
		thresholdObserved := ratJSONNumber(thresholdRat)
		threshold, _ := thresholdRat.Float64()
		observed := map[string]any{"metric": metric, "value": valueObserved, "available": ok, "operator": operator}
		if !ok && !rule.Required {
			return skip(fmt.Sprintf("rule not applicable: reference metric %s is unavailable", metric), observed)
		}
		if !ok || !compareRat(valueRat, operator, thresholdRat) {
			return fail(fmt.Sprintf("reference metric %s is unavailable or violates %s %.6f", metric, operator, threshold), observed, valueObserved, thresholdObserved, 1, nil)
		}
		return pass(observed, valueObserved, thresholdObserved, 0, nil)

	case RuleTypeConditionalConsistency:
		if result, ok, err := targetAvailability(rule, ctx, skip, fail); err != nil || ok {
			return result, result.Observed, err
		}
		conditionField := parameterString(rule, "conditionField")
		missingValue := parameterString(rule, "whenMissing")
		presentValues := parameterStrings(rule, "whenPresentValues")
		if conditionField == "" || missingValue == "" || len(presentValues) == 0 {
			return domain.Finding{}, nil, fmt.Errorf("rule %s conditional_consistency requires conditionField, whenMissing and whenPresentValues", rule.ID)
		}
		if _, ok := tabular.HeaderSet(ctx.Table.Headers)[conditionField]; !ok {
			observed := map[string]any{"conditionField": conditionField, "available": false}
			if rule.Required {
				return fail(fmt.Sprintf("condition field %s is unavailable", conditionField), observed, nil, nil, 1, nil)
			}
			return skip(fmt.Sprintf("rule not applicable: condition field %s is unavailable", conditionField), observed)
		}
		allowedPresent := make(map[string]struct{}, len(presentValues))
		for _, value := range presentValues {
			allowedPresent[value] = struct{}{}
		}
		invalid := 0
		samples := make([]any, 0, 5)
		for index, row := range ctx.Table.Rows {
			condition := strings.TrimSpace(row[conditionField])
			value := strings.TrimSpace(row[rule.Target])
			valid := (condition == "" && value == missingValue) || (condition != "" && hasString(allowedPresent, value))
			if !valid {
				invalid++
				if len(samples) < 5 {
					samples = append(samples, map[string]any{"row": index, "conditionField": conditionField, "target": rule.Target})
				}
			}
		}
		observed := map[string]any{"invalid": invalid, "total": len(ctx.Table.Rows), "conditionField": conditionField}
		if invalid > 0 {
			return fail(fmt.Sprintf("%s is inconsistent with %s", rule.Target, conditionField), observed, invalid, nil, invalid, samples)
		}
		return pass(observed, 0, nil, 0, nil)

	case RuleTypeLineagePresent, RuleTypeEvidencePresent:
		available := ctx.LineagePresent
		label := "lineage"
		if rule.Type == RuleTypeEvidencePresent {
			available = ctx.EvidencePresent
			label = "evidence"
		}
		if available == nil {
			if rule.Required {
				return fail(fmt.Sprintf("%s fact is unavailable", label), map[string]any{"available": false}, false, true, 1, nil)
			}
			return skip(fmt.Sprintf("rule not applicable: %s fact is unavailable", label), map[string]any{"available": false})
		}
		observed := map[string]any{"available": true, "present": *available}
		if !*available {
			if !rule.Required {
				return skip(fmt.Sprintf("rule not applicable: %s is absent and the rule is optional", label), observed)
			}
			return fail(fmt.Sprintf("required %s is absent", label), observed, false, true, 1, nil)
		}
		return pass(observed, true, true, 0, nil)

	default:
		return domain.Finding{}, nil, fmt.Errorf("native quality engine does not implement rule type %q", rule.Type)
	}
}

func targetAvailability(rule Rule, ctx DatasetContext, skip func(string, map[string]any) (domain.Finding, map[string]any, error), fail func(string, map[string]any, any, any, int, []any) (domain.Finding, map[string]any, error)) (domain.Finding, bool, error) {
	if _, ok := tabular.HeaderSet(ctx.Table.Headers)[rule.Target]; !ok {
		observed := map[string]any{"target": rule.Target, "available": false}
		if rule.Required {
			finding, _, err := fail(fmt.Sprintf("target field %s is unavailable", rule.Target), observed, nil, nil, 1, nil)
			return finding, true, err
		}
		finding, _, err := skip(fmt.Sprintf("rule not applicable: target field %s is unavailable", rule.Target), observed)
		return finding, true, err
	}
	if len(ctx.Table.Rows) == 0 && rule.Required {
		finding, _, err := fail("rule cannot evaluate an empty dataset", map[string]any{"total": 0}, 0, nil, 0, nil)
		return finding, true, err
	}
	if len(ctx.Table.Rows) == 0 {
		finding, _, err := skip("rule not applicable: dataset is empty", map[string]any{"total": 0})
		return finding, true, err
	}
	return domain.Finding{}, false, nil
}

func standardObservation(observed map[string]any, value, threshold any, affected int, samples []any) map[string]any {
	if observed == nil {
		observed = map[string]any{}
	}
	observed["observedValue"] = value
	if threshold != nil {
		observed["threshold"] = threshold
	}
	observed["affectedCount"] = affected
	if len(samples) > 0 {
		observed["sample"] = samples
	}
	return observed
}

func addRuleContext(observed map[string]any, rule Rule) {
	expectation := strings.TrimSpace(rule.Expectation)
	if expectation == "" {
		expectation = strings.TrimSpace(rule.Expression)
	}
	if expectation != "" {
		observed["expectation"] = expectation
	}
	if description := strings.TrimSpace(rule.Description); description != "" {
		observed["description"] = description
	}
}

func ruleThreshold(rule Rule, fallback float64) (float64, error) {
	if rule.Threshold != nil {
		return numericValue(rule.Threshold)
	}
	if value, ok := rule.Parameters["threshold"]; ok {
		return numericValue(value)
	}
	return fallback, nil
}

func ruleThresholdRat(rule Rule, fallback float64) (*big.Rat, error) {
	if rule.Threshold != nil {
		return numericRat(rule.Threshold)
	}
	if value, ok := rule.Parameters["threshold"]; ok {
		return numericRat(value)
	}
	return numericRat(fallback)
}

func parameterValue(rule Rule, key string) (any, error) {
	value, ok := rule.Parameters[key]
	if !ok {
		return nil, fmt.Errorf("parameter %s is required", key)
	}
	return value, nil
}

func parameterNumber(rule Rule, key string) (float64, error) {
	value, err := parameterValue(rule, key)
	if err != nil {
		return 0, err
	}
	return numericValue(value)
}

func numericValue(value any) (float64, error) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, fmt.Errorf("must be finite")
		}
		return typed, nil
	case float32:
		return numericValue(float64(typed))
	case int:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case uint:
		return float64(typed), nil
	case uint32:
		return float64(typed), nil
	case uint64:
		return float64(typed), nil
	case json.Number:
		return numericValue(string(typed))
	case string:
		parsed, ok := finiteFloat(typed)
		if !ok {
			return 0, fmt.Errorf("must be a finite number")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("must be numeric, got %T", value)
	}
}

func numericRat(value any) (*big.Rat, error) {
	switch typed := value.(type) {
	case int:
		return new(big.Rat).SetInt64(int64(typed)), nil
	case int32:
		return new(big.Rat).SetInt64(int64(typed)), nil
	case int64:
		return new(big.Rat).SetInt64(typed), nil
	case uint:
		return new(big.Rat).SetUint64(uint64(typed)), nil
	case uint32:
		return new(big.Rat).SetUint64(uint64(typed)), nil
	case uint64:
		return new(big.Rat).SetUint64(typed), nil
	case float32:
		return numericRat(float64(typed))
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil, fmt.Errorf("must be a finite number")
		}
		if math.Trunc(typed) == typed && math.Abs(typed) > 1<<53 {
			return nil, fmt.Errorf("integer-valued float %v exceeds exact range", typed)
		}
		return decimalRat(strconv.FormatFloat(typed, 'g', -1, 64))
	case json.Number:
		return decimalRat(typed.String())
	case string:
		return decimalRat(typed)
	default:
		return nil, fmt.Errorf("must be numeric, got %T", value)
	}
}

func ratJSONNumber(value *big.Rat) json.Number {
	if value == nil {
		return "0"
	}
	return json.Number(ratDecimalString(value))
}

func ratDecimalString(value *big.Rat) string {
	if value.Sign() == 0 {
		return "0"
	}
	denominator := new(big.Int).Set(value.Denom())
	two := big.NewInt(2)
	five := big.NewInt(5)
	one := big.NewInt(1)
	twoCount, fiveCount := 0, 0
	for new(big.Int).Mod(denominator, two).Cmp(big.NewInt(0)) == 0 {
		denominator.Div(denominator, two)
		twoCount++
	}
	for new(big.Int).Mod(denominator, five).Cmp(big.NewInt(0)) == 0 {
		denominator.Div(denominator, five)
		fiveCount++
	}
	if denominator.Cmp(one) != 0 {
		return value.RatString()
	}
	scale := twoCount
	if fiveCount > scale {
		scale = fiveCount
	}
	text := value.FloatString(scale)
	if scale > 0 {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	return text
}

func parameterBool(rule Rule, key string, fallback bool) (bool, error) {
	value, ok := rule.Parameters[key]
	if !ok {
		return fallback, nil
	}
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err == nil {
			return parsed, nil
		}
	}
	return false, fmt.Errorf("must be boolean")
}

func compare(value float64, operator string, threshold float64) bool {
	switch operator {
	case "lt":
		return value < threshold
	case "lte", "le":
		return value <= threshold
	case "eq", "equal":
		return value == threshold
	case "gte", "ge":
		return value >= threshold
	case "gt":
		return value > threshold
	default:
		return false
	}
}

func compareRat(value *big.Rat, operator string, threshold *big.Rat) bool {
	if value == nil || threshold == nil {
		return false
	}
	comparison := value.Cmp(threshold)
	switch operator {
	case "lt":
		return comparison < 0
	case "lte", "le":
		return comparison <= 0
	case "eq", "equal":
		return comparison == 0
	case "gte", "ge":
		return comparison >= 0
	case "gt":
		return comparison > 0
	default:
		return false
	}
}

func hasString(values map[string]struct{}, value string) bool {
	_, ok := values[value]
	return ok
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func numericMetadataRat(metadata map[string]any, key string) (*big.Rat, bool) {
	if metadata == nil {
		return nil, false
	}
	value, ok := metadata[key]
	if !ok {
		return nil, false
	}
	parsed, err := numericRat(value)
	return parsed, err == nil
}

// finiteFloat parses a decimal value and rejects NaN and infinities. Non-finite
// values compare false against every range bound, so accepting one would let a
// malformed CSV cell or metric satisfy a rule that it must fail.
func finiteFloat(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, false
	}
	return parsed, true
}

var decimalNumberPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func decimalRat(value string) (*big.Rat, error) {
	text := strings.TrimSpace(value)
	if !decimalNumberPattern.MatchString(text) {
		return nil, fmt.Errorf("must be a finite decimal number")
	}
	rat, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, fmt.Errorf("must be a finite decimal number")
	}
	return rat, nil
}
