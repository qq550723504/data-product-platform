package matching

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Policy struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		EntityType    string        `yaml:"entityType"`
		Normalization Normalization `yaml:"normalization"`
		Rules         []Rule        `yaml:"rules"`
		Thresholds    Thresholds    `yaml:"thresholds"`
		Evidence      Evidence      `yaml:"evidence"`
	} `yaml:"spec"`
}

type Normalization struct {
	CompanyName struct {
		Trim                        bool `yaml:"trim"`
		NormalizeWhitespace         bool `yaml:"normalizeWhitespace"`
		NormalizeFullWidthHalfWidth bool `yaml:"normalizeFullWidthHalfWidth"`
		Aliases                     []struct {
			Suffix    string `yaml:"suffix"`
			Canonical string `yaml:"canonical"`
		} `yaml:"aliases"`
	} `yaml:"companyName"`
	Address struct {
		Trim                bool `yaml:"trim"`
		NormalizeWhitespace bool `yaml:"normalizeWhitespace"`
	} `yaml:"address"`
	UnifiedSocialCreditCode struct {
		Uppercase bool `yaml:"uppercase"`
		Trim      bool `yaml:"trim"`
	} `yaml:"unifiedSocialCreditCode"`
}

type Rule struct {
	ID         string     `yaml:"id"`
	Priority   int        `yaml:"priority"`
	When       *Condition `yaml:"when,omitempty"`
	Decision   string     `yaml:"decision"`
	Confidence float64    `yaml:"confidence,omitempty"`
}

type Condition struct {
	All []Predicate `yaml:"all"`
}

type Predicate struct {
	Field    string  `yaml:"field"`
	Operator string  `yaml:"operator"`
	Value    float64 `yaml:"value,omitempty"`
}

type Thresholds struct {
	AutoMatchMinimum float64 `yaml:"autoMatchMinimum"`
	ReviewMinimum    float64 `yaml:"reviewMinimum"`
}

type Evidence struct {
	RecordRuleVersion                      bool `yaml:"recordRuleVersion"`
	RecordConfidence                       bool `yaml:"recordConfidence"`
	RequireReviewerReasonForManualDecision bool `yaml:"requireReviewerReasonForManualDecision"`
}

func ResolvePolicyPath(root, ref string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve industry pack root: %w", err)
	}
	ref = filepath.Clean(strings.TrimSpace(ref))
	if ref == "." || filepath.IsAbs(ref) {
		return "", fmt.Errorf("policy ref must be a relative path")
	}
	candidate, err := filepath.Abs(filepath.Join(rootAbs, ref))
	if err != nil {
		return "", fmt.Errorf("resolve policy ref: %w", err)
	}
	relative, err := filepath.Rel(rootAbs, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("policy ref escapes industry pack root")
	}
	return candidate, nil
}

func LoadPolicy(path string) (Policy, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read matching policy %q: %w", path, err)
	}
	var policy Policy
	if err := yaml.Unmarshal(content, &policy); err != nil {
		return Policy{}, fmt.Errorf("decode matching policy %q: %w", path, err)
	}
	if strings.TrimSpace(policy.Metadata.Version) == "" || strings.TrimSpace(policy.Spec.EntityType) == "" {
		return Policy{}, fmt.Errorf("matching policy %q is missing version or entity type", path)
	}
	if err := validateThresholds(policy.Spec.Thresholds); err != nil {
		return Policy{}, fmt.Errorf("matching policy %q has invalid thresholds: %w", path, err)
	}
	return policy, nil
}

func validateThresholds(thresholds Thresholds) error {
	// A non-finite threshold compares false against every bound below and would
	// silently disable the decision it guards: NaN makes every comparison the
	// matcher performs fall through to the reject branch.
	if !isFinite(thresholds.ReviewMinimum) {
		return fmt.Errorf("reviewMinimum must be a finite number")
	}
	if !isFinite(thresholds.AutoMatchMinimum) {
		return fmt.Errorf("autoMatchMinimum must be a finite number")
	}
	if thresholds.ReviewMinimum <= 0 || thresholds.ReviewMinimum > 1 {
		return fmt.Errorf("reviewMinimum must be > 0 and <= 1")
	}
	if thresholds.AutoMatchMinimum <= 0 || thresholds.AutoMatchMinimum > 1 {
		return fmt.Errorf("autoMatchMinimum must be > 0 and <= 1")
	}
	if thresholds.ReviewMinimum > thresholds.AutoMatchMinimum {
		return fmt.Errorf("reviewMinimum must be <= autoMatchMinimum")
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
