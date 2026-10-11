package application

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/industrypack"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strings"
)

type Plan struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"metadata"`
	Spec struct {
		DetectionRuleSetRef   string `yaml:"detectionRuleSetRef"`
		DetectionRuleID       string `yaml:"detectionRuleId"`
		WorkflowDefinitionRef string `yaml:"workflowDefinitionRef"`
		InputName             string `yaml:"inputName"`
		QuarantineReasonCode  string `yaml:"quarantineReasonCode"`
		RecheckRuleSetRef     string `yaml:"recheckRuleSetRef"`
	} `yaml:"spec"`
	Ref           string `yaml:"-"`
	Hash          string `yaml:"-"`
	WorkflowHash  string `yaml:"-"`
	RecheckHash   string `yaml:"-"`
	DetectionHash string `yaml:"-"`
}

func digest(content []byte) string { return fmt.Sprintf("%x", sha256.Sum256(content)) }

func LoadPlan(root, ref string) (Plan, error) {
	var p Plan
	path, err := industrypack.ResolvePath(root, ref)
	if err != nil {
		return p, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	decoder.KnownFields(true)
	if err = decoder.Decode(&p); err != nil {
		return p, err
	}
	if p.Kind != "RemediationPlan" || p.Metadata.Version == "" || p.Metadata.Name == "" {
		return p, fmt.Errorf("invalid remediation plan")
	}
	for _, v := range []string{p.Spec.DetectionRuleID, p.Spec.InputName, p.Spec.QuarantineReasonCode, p.Spec.DetectionRuleSetRef, p.Spec.RecheckRuleSetRef, p.Spec.WorkflowDefinitionRef} {
		if strings.TrimSpace(v) == "" {
			return p, fmt.Errorf("incomplete remediation plan")
		}
	}
	p.Ref, p.Hash = ref, digest(content)
	for _, rule := range []struct {
		ref    string
		target *string
	}{{p.Spec.DetectionRuleSetRef, &p.DetectionHash}, {p.Spec.RecheckRuleSetRef, &p.RecheckHash}} {
		policyPath, e := industrypack.ResolvePath(root, rule.ref)
		if e != nil {
			return p, e
		}
		b, e := os.ReadFile(policyPath)
		if e != nil {
			return p, e
		}
		*rule.target = digest(b)
	}
	repoRoot := filepath.Dir(root)
	workflowPath := filepath.Join(repoRoot, filepath.FromSlash(p.Spec.WorkflowDefinitionRef))
	relative, err := filepath.Rel(repoRoot, workflowPath)
	if err != nil || strings.HasPrefix(relative, "..") || filepath.IsAbs(relative) {
		return p, fmt.Errorf("workflow outside repository")
	}
	b, err := os.ReadFile(workflowPath)
	if err != nil {
		return p, err
	}
	p.WorkflowHash = digest(b)
	return p, nil
}
