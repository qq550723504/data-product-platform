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
		ProductCode string `yaml:"productCode"`
		Rules       []Rule `yaml:"rules"`
		Gate        struct {
			CriticalFailure string `yaml:"criticalFailure"`
			HighFailure     string `yaml:"highFailure"`
			WarningFailure  string `yaml:"warningFailure"`
		} `yaml:"gate"`
	} `yaml:"spec"`
}

type Rule struct {
	ID         string `yaml:"id"`
	Stage      string `yaml:"stage"`
	Dimension  string `yaml:"dimension"`
	Target     string `yaml:"target"`
	Expression string `yaml:"expression"`
	Severity   string `yaml:"severity"`
	Note       string `yaml:"note"`
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
	if strings.TrimSpace(policy.Metadata.Version) == "" || len(policy.Spec.Rules) == 0 {
		return Policy{}, fmt.Errorf("quality policy %q is missing version or rules", path)
	}
	return policy, nil
}
