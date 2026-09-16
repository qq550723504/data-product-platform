package indicator

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Policy struct {
	Metadata struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		Rounding struct {
			Mode     string `yaml:"mode"`
			Decimals int    `yaml:"decimals"`
		} `yaml:"rounding"`
		Indicators []IndicatorDefinition `yaml:"indicators"`
	} `yaml:"spec"`
}

type IndicatorDefinition struct {
	Code       string              `yaml:"code"`
	Parameters IndicatorParameters `yaml:"parameters"`
}

type IndicatorParameters struct {
	TenureFullScoreMonths int                `yaml:"tenureFullScoreMonths"`
	TenureWeight          float64            `yaml:"tenureWeight"`
	CurrentLeaseWeight    float64            `yaml:"currentLeaseWeight"`
	LookbackMonths        int                `yaml:"lookbackMonths"`
	MinimumDueEvents      int                `yaml:"minimumDueEvents"`
	EventScores           map[string]float64 `yaml:"eventScores"`
	MinimumValidMonths    int                `yaml:"minimumValidMonths"`
	MaximumReferenceCV    float64            `yaml:"maximumReferenceCv"`
	VariabilityWeight     float64            `yaml:"variabilityWeight"`
	CoverageWeight        float64            `yaml:"coverageWeight"`
	Weights               map[string]float64 `yaml:"weights"`
	Levels                map[string]struct {
		MinimumInclusive float64  `yaml:"minimumInclusive"`
		MaximumExclusive *float64 `yaml:"maximumExclusive"`
	} `yaml:"levels"`
}

func LoadPolicy(path string) (Policy, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read indicator policy %q: %w", path, err)
	}
	var policy Policy
	if err := yaml.Unmarshal(content, &policy); err != nil {
		return Policy{}, fmt.Errorf("decode indicator policy %q: %w", path, err)
	}
	if strings.TrimSpace(policy.Metadata.Name) == "" || strings.TrimSpace(policy.Metadata.Version) == "" {
		return Policy{}, fmt.Errorf("indicator policy %q is missing name/version", path)
	}
	for _, required := range []string{"tenancy_stability", "rent_performance", "energy_stability", "activity_score"} {
		if _, ok := policy.Definition(required); !ok {
			return Policy{}, fmt.Errorf("indicator policy %q is missing %s", path, required)
		}
	}
	return policy, nil
}

func (p Policy) Definition(code string) (IndicatorDefinition, bool) {
	for _, definition := range p.Spec.Indicators {
		if definition.Code == code {
			return definition, true
		}
	}
	return IndicatorDefinition{}, false
}
