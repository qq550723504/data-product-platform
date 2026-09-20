package native

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	"gopkg.in/yaml.v3"
)

const (
	EvaluatorName    = "native-quality"
	EvaluatorVersion = "2"
)

type Policy struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		ProductCode string `yaml:"productCode"`
		Rules       []Rule `yaml:"rules"`
		Gate        struct {
			CriticalFailure string `yaml:"criticalFailure"`
			HighFailure     string `yaml:"highFailure"`
			WarningFailure  string `yaml:"warningFailure"`
		} `yaml:"gate"`
	} `yaml:"spec"`
	SourceContent       string
	SourceContentSHA256 string
}

type Rule struct {
	ID          string         `yaml:"id"`
	Stage       string         `yaml:"stage"`
	Dimension   string         `yaml:"dimension"`
	Type        string         `yaml:"type"`
	Target      string         `yaml:"target"`
	Threshold   any            `yaml:"threshold"`
	Parameters  map[string]any `yaml:"parameters"`
	Required    bool           `yaml:"required"`
	Description string         `yaml:"description"`
	Expectation string         `yaml:"expectation"`
	Expression  string         `yaml:"expression"` // retained as human-readable expectation text
	Severity    string         `yaml:"severity"`
	Note        string         `yaml:"note"`
	requiredSet bool
}

// UnmarshalYAML preserves whether required was present. A plain bool cannot
// distinguish an omitted field from an explicit false, but that distinction
// is part of the rule-set contract for fail-closed evaluation.
func (r *Rule) UnmarshalYAML(node *yaml.Node) error {
	type ruleAlias Rule
	var decoded ruleAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*r = Rule(decoded)
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("rule must be a mapping")
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == "required" {
			r.requiredSet = true
			requiredNode := node.Content[index+1]
			requiredValue := strings.ToLower(strings.TrimSpace(requiredNode.Value))
			if requiredNode.Tag != "!!bool" || (requiredValue != "true" && requiredValue != "false") {
				return fmt.Errorf("rule required must be a non-null boolean")
			}
			break
		}
	}
	return nil
}

const (
	RuleTypeNotNull                = "not_null"
	RuleTypeCompletenessRatio      = "completeness_ratio"
	RuleTypeUnique                 = "unique"
	RuleTypeDuplicateRatio         = "duplicate_ratio"
	RuleTypeRange                  = "range"
	RuleTypeEnum                   = "enum"
	RuleTypeRegex                  = "regex"
	RuleTypeFreshness              = "freshness"
	RuleTypeReferenceMatch         = "reference_match"
	RuleTypeReconciliation         = "reconciliation"
	RuleTypeConditionalConsistency = "conditional_consistency"
	RuleTypeLineagePresent         = "lineage_present"
	RuleTypeEvidencePresent        = "evidence_present"
)

var qualityRuleTypes = map[string]struct{}{
	RuleTypeNotNull: {}, RuleTypeCompletenessRatio: {}, RuleTypeUnique: {},
	RuleTypeDuplicateRatio: {}, RuleTypeRange: {}, RuleTypeEnum: {}, RuleTypeRegex: {},
	RuleTypeFreshness: {}, RuleTypeReferenceMatch: {}, RuleTypeReconciliation: {},
	RuleTypeConditionalConsistency: {}, RuleTypeLineagePresent: {}, RuleTypeEvidencePresent: {},
}

func LoadPolicy(path string) (Policy, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read quality policy %q: %w", path, err)
	}
	var policy Policy
	if err := yaml.Unmarshal(content, &policy); err != nil {
		return Policy{}, fmt.Errorf("decode quality policy %q: %w", path, err)
	}
	if err := validatePolicy(policy, true); err != nil {
		return Policy{}, fmt.Errorf("validate quality policy %q: %w", path, err)
	}
	for i := range policy.Spec.Rules {
		policy.Spec.Rules[i].Dimension = normalizeDimension(policy.Spec.Rules[i].Dimension)
		policy.Spec.Rules[i].Type = strings.ToLower(strings.TrimSpace(policy.Spec.Rules[i].Type))
		policy.Spec.Rules[i].Severity = normalizeSeverity(policy.Spec.Rules[i].Severity)
	}
	policy.SourceContent = string(content)
	policy.SourceContentSHA256 = fmt.Sprintf("%x", sha256.Sum256(content))
	return policy, nil
}

func validatePolicy(policy Policy, requireRequired bool) error {
	if strings.TrimSpace(policy.APIVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(policy.Kind) != "QualityRuleSet" {
		return fmt.Errorf("kind must be QualityRuleSet")
	}
	if strings.TrimSpace(policy.Metadata.Version) == "" {
		return fmt.Errorf("metadata.version is required")
	}
	if len(policy.Spec.Rules) == 0 {
		return fmt.Errorf("spec.rules must not be empty")
	}
	seen := make(map[string]struct{}, len(policy.Spec.Rules))
	for i, rule := range policy.Spec.Rules {
		if strings.TrimSpace(rule.ID) == "" {
			return fmt.Errorf("spec.rules[%d].id is required", i)
		}
		if _, exists := seen[rule.ID]; exists {
			return fmt.Errorf("spec.rules[%d].id %q is duplicated", i, rule.ID)
		}
		seen[rule.ID] = struct{}{}
		if requireRequired && !rule.requiredSet {
			return fmt.Errorf("rule %s required must be explicitly declared", rule.ID)
		}
		if !isQualityDimension(rule.Dimension) {
			return fmt.Errorf("rule %s has unsupported dimension %q", rule.ID, rule.Dimension)
		}
		ruleType := strings.ToLower(strings.TrimSpace(rule.Type))
		if _, ok := qualityRuleTypes[ruleType]; !ok {
			return fmt.Errorf("rule %s has unknown rule type %q", rule.ID, rule.Type)
		}
		severity := normalizeSeverity(rule.Severity)
		if severity == "" {
			return fmt.Errorf("rule %s severity is required", rule.ID)
		}
		switch severity {
		case "CRITICAL", "HIGH", "WARNING":
		default:
			return fmt.Errorf("rule %s has unsupported severity %q", rule.ID, rule.Severity)
		}
		if requiresTarget(ruleType) && strings.TrimSpace(rule.Target) == "" {
			return fmt.Errorf("rule %s target is required for %s", rule.ID, ruleType)
		}
		if ruleType == RuleTypeRange {
			if _, err := parameterNumber(rule, "min"); err != nil {
				return fmt.Errorf("rule %s range min: %w", rule.ID, err)
			}
			if _, err := parameterNumber(rule, "max"); err != nil {
				return fmt.Errorf("rule %s range max: %w", rule.ID, err)
			}
		}
		if ruleType == RuleTypeNotNull || ruleType == RuleTypeCompletenessRatio || ruleType == RuleTypeUnique {
			if err := validateRatioThreshold(rule, 1); err != nil {
				return fmt.Errorf("rule %s: %w", rule.ID, err)
			}
		}
		if ruleType == RuleTypeDuplicateRatio {
			if err := validateRatioThreshold(rule, 0); err != nil {
				return fmt.Errorf("rule %s: %w", rule.ID, err)
			}
		}
		if ruleType == RuleTypeRange || ruleType == RuleTypeEnum || ruleType == RuleTypeRegex {
			if _, err := parameterBool(rule, "allowNull", true); err != nil {
				return fmt.Errorf("rule %s allowNull: %w", rule.ID, err)
			}
		}
		if ruleType == RuleTypeEnum && len(parameterStrings(rule, "values", "allowedValues")) == 0 {
			return fmt.Errorf("rule %s enum values are required", rule.ID)
		}
		if ruleType == RuleTypeRegex && strings.TrimSpace(parameterString(rule, "pattern")) == "" {
			return fmt.Errorf("rule %s regex pattern is required", rule.ID)
		}
		if ruleType == RuleTypeConditionalConsistency {
			if strings.TrimSpace(parameterString(rule, "conditionField")) == "" ||
				strings.TrimSpace(parameterString(rule, "whenMissing")) == "" ||
				len(parameterStrings(rule, "whenPresentValues")) == 0 {
				return fmt.Errorf("rule %s conditional_consistency parameters are incomplete", rule.ID)
			}
		}
		if ruleType == RuleTypeReferenceMatch || ruleType == RuleTypeReconciliation {
			if strings.TrimSpace(parameterString(rule, "metric", "metadataKey")) == "" {
				return fmt.Errorf("rule %s metric parameter is required", rule.ID)
			}
			if !hasExplicitThreshold(rule) {
				return fmt.Errorf("rule %s threshold is required", rule.ID)
			}
			if _, err := ruleThreshold(rule, 0); err != nil {
				return fmt.Errorf("rule %s threshold: %w", rule.ID, err)
			}
			operator, present, err := parameterStringValue(rule, "operator")
			if err != nil {
				return fmt.Errorf("rule %s operator: %w", rule.ID, err)
			}
			if !present || operator == "" {
				return fmt.Errorf("rule %s operator is required", rule.ID)
			}
			operator = strings.ToLower(operator)
			switch operator {
			case "lt", "lte", "le", "eq", "equal", "gte", "ge", "gt":
			default:
				return fmt.Errorf("rule %s has unsupported operator %q", rule.ID, operator)
			}
		}
		if ruleType == RuleTypeFreshness {
			if !hasExplicitThreshold(rule) {
				return fmt.Errorf("rule %s threshold is required", rule.ID)
			}
			if _, err := ruleThreshold(rule, 0); err != nil {
				return fmt.Errorf("rule %s threshold: %w", rule.ID, err)
			}
		}
	}
	return nil
}

func isQualityDimension(value string) bool {
	normalized := normalizeDimension(value)
	for _, dimension := range domain.QualityDimensions {
		if normalized == dimension {
			return true
		}
	}
	return false
}

func validateRules(rules []Rule) error {
	policy := Policy{APIVersion: "inline", Kind: "QualityRuleSet"}
	policy.Metadata.Version = "inline"
	policy.Spec.Rules = rules
	return validatePolicy(policy, false)
}

func requiresTarget(ruleType string) bool {
	switch ruleType {
	case RuleTypeNotNull, RuleTypeCompletenessRatio, RuleTypeUnique, RuleTypeDuplicateRatio,
		RuleTypeRange, RuleTypeEnum, RuleTypeRegex, RuleTypeConditionalConsistency:
		return true
	default:
		return false
	}
}

func normalizeDimension(value string) string {
	dimension := strings.ToUpper(strings.TrimSpace(value))
	if dimension == "CONFORMITY" {
		return "ACCURACY"
	}
	return dimension
}

func normalizeSeverity(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func validateRatioThreshold(rule Rule, fallback float64) error {
	threshold, err := ruleThreshold(rule, fallback)
	if err != nil {
		return fmt.Errorf("ratio threshold: %w", err)
	}
	if threshold < 0 || threshold > 1 {
		return fmt.Errorf("ratio threshold must be between 0 and 1, got %v", threshold)
	}
	return nil
}

func hasExplicitThreshold(rule Rule) bool {
	if rule.Threshold != nil {
		return true
	}
	_, ok := rule.Parameters["threshold"]
	return ok
}

func parameterString(rule Rule, keys ...string) string {
	for _, key := range keys {
		value, ok := rule.Parameters[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func parameterStringValue(rule Rule, key string) (string, bool, error) {
	value, ok := rule.Parameters[key]
	if !ok {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", true, fmt.Errorf("parameter %s must be a string", key)
	}
	return strings.TrimSpace(text), true, nil
}

func parameterStrings(rule Rule, keys ...string) []string {
	for _, key := range keys {
		value, ok := rule.Parameters[key]
		if !ok {
			continue
		}
		switch values := value.(type) {
		case []any:
			result := make([]string, 0, len(values))
			for _, item := range values {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					result = append(result, strings.TrimSpace(text))
				}
			}
			return result
		case []string:
			result := make([]string, 0, len(values))
			for _, item := range values {
				if strings.TrimSpace(item) != "" {
					result = append(result, strings.TrimSpace(item))
				}
			}
			return result
		}
	}
	return nil
}
