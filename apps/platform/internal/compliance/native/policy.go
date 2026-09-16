package native

import (
	"fmt"
	"os"
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
		ProductCode   string        `yaml:"productCode"`
		Principle     string        `yaml:"principle"`
		FieldPolicies []FieldPolicy `yaml:"fieldPolicies"`
		ProductOutput struct {
			AllowOnlyContractFields bool   `yaml:"allowOnlyContractFields"`
			UnknownFieldAction      string `yaml:"unknownFieldAction"`
		} `yaml:"productOutput"`
	} `yaml:"spec"`
}

type FieldPolicy struct {
	Match struct {
		Names []string `yaml:"names"`
	} `yaml:"match"`
	Category string `yaml:"category"`
	Action   string `yaml:"action"`
	Reason   string `yaml:"reason"`
}

func LoadPolicy(path string) (Policy, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read compliance policy %q: %w", path, err)
	}
	var policy Policy
	if err := yaml.Unmarshal(content, &policy); err != nil {
		return Policy{}, fmt.Errorf("decode compliance policy %q: %w", path, err)
	}
	if strings.TrimSpace(policy.Metadata.Version) == "" || len(policy.Spec.FieldPolicies) == 0 {
		return Policy{}, fmt.Errorf("compliance policy %q is missing version or field policies", path)
	}
	return policy, nil
}
