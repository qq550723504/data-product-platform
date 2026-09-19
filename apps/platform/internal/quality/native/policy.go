package native

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	EvaluatorName    = "native-quality"
	EvaluatorVersion = "1"
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
	policy.SourceContent = string(content)
	policy.SourceContentSHA256 = fmt.Sprintf("%x", sha256.Sum256(content))
	return policy, nil
}
