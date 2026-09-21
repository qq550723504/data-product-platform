package native

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

func TestLoadPolicyCapturesExactSourceSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quality.yaml")
	content := []byte("apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\nspec:\n  rules:\n    - id: QA-COMPANY-ID-COMPLETE\n      dimension: COMPLETENESS\n      type: completeness_ratio\n      target: company_id\n      threshold: 1\n      required: true\n      severity: CRITICAL\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	policy, err := LoadPolicy(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if policy.SourceContent != string(content) {
		t.Fatalf("source content changed during load")
	}
	want := fmt.Sprintf("%x", sha256.Sum256(content))
	if policy.SourceContentSHA256 != want {
		t.Fatalf("source sha256 = %s, want %s", policy.SourceContentSHA256, want)
	}
	if err := os.WriteFile(path, append(content, []byte("# changed after evaluation\n")...), 0o600); err != nil {
		t.Fatalf("modify policy: %v", err)
	}
	if policy.SourceContent != string(content) || policy.SourceContentSHA256 != want {
		t.Fatal("loaded policy snapshot changed after source file mutation")
	}
}

func TestLoadPolicyRejectsUnknownGateFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quality.yaml")
	content := []byte("apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\nspec:\n  gate:\n    warningFailures: FAIL\n  rules:\n    - id: R-1\n      dimension: COMPLETENESS\n      type: not_null\n      target: amount\n      required: true\n      severity: WARNING\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	if _, err := LoadPolicy(path); err == nil {
		t.Fatal("policy with unknown gate field was accepted")
	}
}

func TestLoadPolicyRejectsUnknownRuleFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "quality.yaml")
	content := []byte("apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\nspec:\n  rules:\n    - id: R-1\n      dimension: ACCURACY\n      type: range\n      target: amount\n      min: 0\n      max: 1\n      required: true\n      severity: CRITICAL\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	if _, err := LoadPolicy(path); err == nil {
		t.Fatal("policy with unknown rule field was accepted")
	}
}

func TestRegexRuleMatchesUnmodifiedCellValues(t *testing.T) {
	policy := Policy{}
	policy.Spec.Rules = []Rule{{
		ID: "R-REGEX", Dimension: "CONSISTENCY", Type: RuleTypeRegex, Target: "code",
		Parameters: map[string]any{"pattern": "^[A-Z]+$", "allowNull": false},
		Required:   true, Severity: "CRITICAL",
	}}
	findings, _, err := Evaluate(policy, DatasetContext{Table: tabular.Table{
		Headers: []string{"code"}, Rows: []map[string]string{{"code": " BAD "}},
	}})
	if err != nil {
		t.Fatalf("evaluate regex rule: %v", err)
	}
	if findings[0].Status != domain.FindingFail {
		t.Fatalf("space-padded regex value passed: %#v", findings[0])
	}
}

func TestEnumRuleMatchesUnmodifiedCellValues(t *testing.T) {
	policy := Policy{}
	policy.Spec.Rules = []Rule{{
		ID: "R-ENUM", Dimension: "CONSISTENCY", Type: RuleTypeEnum, Target: "role",
		Parameters: map[string]any{"values": []any{"ADMIN"}, "allowNull": false},
		Required:   true, Severity: "CRITICAL",
	}}
	findings, _, err := Evaluate(policy, DatasetContext{Table: tabular.Table{
		Headers: []string{"role"}, Rows: []map[string]string{{"role": " ADMIN "}},
	}})
	if err != nil {
		t.Fatalf("evaluate enum rule: %v", err)
	}
	if findings[0].Status != domain.FindingFail {
		t.Fatalf("space-padded enum value passed: %#v", findings[0])
	}
}

func TestLoadPolicyRejectsUnknownRuleTypeAndMissingParameters(t *testing.T) {
	dir := t.TempDir()
	unknown := filepath.Join(dir, "unknown.yaml")
	missing := filepath.Join(dir, "missing.yaml")
	base := "apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\nspec:\n  rules:\n    - id: R-1\n      dimension: ACCURACY\n      type: %s\n      target: amount\n      severity: CRITICAL\n"
	if err := os.WriteFile(unknown, []byte(fmt.Sprintf(base, "made_up")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(unknown); err == nil {
		t.Fatal("unknown rule type was accepted")
	}
	if err := os.WriteFile(missing, []byte(fmt.Sprintf(base, RuleTypeRange)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(missing); err == nil {
		t.Fatal("range rule without min/max was accepted")
	}
	wrongKind := filepath.Join(dir, "wrong-kind.yaml")
	content := fmt.Sprintf(base, RuleTypeCompletenessRatio)
	content = "apiVersion: quality/v1\nkind: NotAQualityRuleSet\n" + content[strings.Index(content, "metadata:"):]
	if err := os.WriteFile(wrongKind, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(wrongKind); err == nil {
		t.Fatal("wrong policy kind was accepted")
	}
	missingRequired := filepath.Join(dir, "missing-required.yaml")
	content = fmt.Sprintf(base, RuleTypeCompletenessRatio)
	if err := os.WriteFile(missingRequired, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(missingRequired); err == nil {
		t.Fatal("rule without explicit required was accepted")
	}
	explicitFalse := filepath.Join(dir, "explicit-false.yaml")
	content = strings.Replace(content, "severity: CRITICAL", "required: false\n      severity: CRITICAL", 1)
	if err := os.WriteFile(explicitFalse, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(explicitFalse); err != nil {
		t.Fatalf("explicit required=false was rejected: %v", err)
	}
	badSeverity := filepath.Join(dir, "bad-severity.yaml")
	content = strings.Replace(content, "severity: CRITICAL", "severity: CRITCAL", 1)
	if err := os.WriteFile(badSeverity, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(badSeverity); err == nil {
		t.Fatal("unknown severity was accepted")
	}
}

func TestLoadPolicyNormalizesSeverity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "severity.yaml")
	content := []byte("apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\nspec:\n  rules:\n    - id: R-1\n      dimension: ACCURACY\n      type: not_null\n      target: amount\n      required: true\n      severity: ' critical '\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicy(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	if policy.Spec.Rules[0].Severity != "CRITICAL" {
		t.Fatalf("severity = %q, want CRITICAL", policy.Spec.Rules[0].Severity)
	}
}

func TestLoadPolicyPreservesExactDecimalBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "decimal.yaml")
	content := []byte("apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\nspec:\n  rules:\n    - id: R-RANGE\n      dimension: ACCURACY\n      type: range\n      target: amount\n      parameters:\n        min: 0.10000000000000001\n        max: 1\n        allowNull: false\n      required: true\n      severity: CRITICAL\n")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	policy, err := LoadPolicy(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	min, ok := policy.Spec.Rules[0].Parameters["min"].(json.Number)
	if !ok || min.String() != "0.10000000000000001" {
		t.Fatalf("min = %#v, want exact JSON number", policy.Spec.Rules[0].Parameters["min"])
	}
	findings, _, err := Evaluate(policy, DatasetContext{Table: tabular.Table{
		Headers: []string{"amount"},
		Rows:    []map[string]string{{"amount": "0.100000000000000005"}},
	}})
	if err != nil {
		t.Fatalf("evaluate exact decimal range: %v", err)
	}
	if findings[0].Status != domain.FindingFail {
		t.Fatalf("decimal below exact minimum passed: %#v", findings[0])
	}
}

func TestRatioThresholdUsesExactDecimalComparison(t *testing.T) {
	rows := make([]map[string]string, 0, 10)
	for index := 0; index < 9; index++ {
		rows = append(rows, map[string]string{"amount": fmt.Sprintf("%d", index)})
	}
	rows = append(rows, map[string]string{"amount": ""})
	finding := evaluateSingleRule(t, Rule{
		ID:        "R-COMPLETE",
		Dimension: "COMPLETENESS",
		Type:      RuleTypeCompletenessRatio,
		Target:    "amount",
		Threshold: json.Number("0.90000000000000001"),
		Required:  true,
		Severity:  "CRITICAL",
	}, DatasetContext{Table: tabular.Table{Headers: []string{"amount"}, Rows: rows}})
	if finding.Status != domain.FindingFail {
		t.Fatalf("exact ratio below threshold passed: %#v", finding)
	}
}

func TestNonTerminatingRatioObservationIsJSONSafe(t *testing.T) {
	finding := evaluateSingleRule(t, Rule{
		ID:        "R-COMPLETE",
		Dimension: "COMPLETENESS",
		Type:      RuleTypeCompletenessRatio,
		Target:    "amount",
		Threshold: json.Number("0.2"),
		Required:  true,
		Severity:  "CRITICAL",
	}, DatasetContext{Table: tabular.Table{
		Headers: []string{"amount"},
		Rows:    []map[string]string{{"amount": "1"}, {"amount": ""}, {"amount": ""}},
	}})
	encoded, err := json.Marshal(finding.Observed)
	if err != nil {
		t.Fatalf("marshal non-terminating ratio observation: %v", err)
	}
	if !strings.Contains(string(encoded), `"numerator":1`) || !strings.Contains(string(encoded), `"denominator":3`) {
		t.Fatalf("ratio observation = %s, want numerator/denominator", encoded)
	}
}

func TestPolicyGateDecisionHonorsDeclaredMapping(t *testing.T) {
	policy := Policy{}
	policy.Spec.Gate.CriticalFailure = "FAIL"
	policy.Spec.Gate.HighFailure = "REVIEW"
	policy.Spec.Gate.WarningFailure = "FAIL"
	decision, err := policy.GateDecision([]domain.Finding{{Status: domain.FindingFail, Severity: "WARNING"}})
	if err != nil {
		t.Fatalf("derive declared gate decision: %v", err)
	}
	if decision != domain.GateFail {
		t.Fatalf("warning gate decision = %s, want FAIL", decision)
	}
	policy.Spec.Gate.CriticalFailure = "PASS_WITH_WARNING"
	if _, err := policy.GateDecision([]domain.Finding{{Status: domain.FindingFail, Severity: "CRITICAL"}}); err == nil {
		t.Fatal("nonblocking critical gate mapping was accepted")
	}
}

func TestLoadPolicyRejectsInvalidRatioThresholdsAndAllowNull(t *testing.T) {
	tests := []struct {
		name string
		rule string
	}{
		{name: "completeness threshold above one", rule: "type: completeness_ratio\n      target: amount\n      threshold: 1.1"},
		{name: "unique threshold below zero", rule: "type: unique\n      target: amount\n      threshold: -0.1"},
		{name: "duplicate threshold above one", rule: "type: duplicate_ratio\n      target: amount\n      threshold: 2"},
		{name: "freshness threshold omitted", rule: "type: freshness"},
		{name: "reference threshold omitted", rule: "type: reference_match\n      parameters:\n        metric: errorRate"},
		{name: "comparison operator misspelled", rule: "type: reference_match\n      threshold: 0.1\n      parameters:\n        metric: errorRate\n        operater: gte"},
		{name: "required null", rule: "type: not_null\n      target: amount\n      required:"},
		{name: "malformed comparison operator", rule: "type: reference_match\n      threshold: 0.1\n      parameters:\n        metric: errorRate\n        operator: true"},
		{name: "allowNull parameter misspelled", rule: "type: range\n      target: amount\n      parameters:\n        min: 0\n        max: 1\n        allowNul: false"},
		{name: "malformed allowNull", rule: "type: range\n      target: amount\n      parameters:\n        min: 0\n        max: 1\n        allowNull: flase"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "quality.yaml")
			content := fmt.Sprintf("apiVersion: quality/v1\nkind: QualityRuleSet\nmetadata:\n  version: 1.0.0\nspec:\n  rules:\n    - id: R-1\n      dimension: ACCURACY\n      required: true\n      severity: CRITICAL\n      %s\n", testCase.rule)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadPolicy(path); err == nil {
				t.Fatal("invalid rule parameter was accepted")
			}
		})
	}
}

func TestGenericRuleTypesUsePackConfiguration(t *testing.T) {
	readyAt := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	lineage := true
	evidence := true
	policy := Policy{}
	policy.Spec.Rules = []Rule{
		{ID: "R-COMPLETE", Dimension: "COMPLETENESS", Type: RuleTypeCompletenessRatio, Target: "id", Threshold: 1.0, Required: true, Severity: "CRITICAL"},
		{ID: "R-UNIQUE", Dimension: "UNIQUENESS", Type: RuleTypeUnique, Target: "id", Threshold: 1.0, Required: true, Severity: "CRITICAL"},
		{ID: "R-RANGE", Dimension: "ACCURACY", Type: RuleTypeRange, Target: "score", Parameters: map[string]any{"min": 0, "max": 100}, Required: true, Severity: "CRITICAL"},
		{ID: "R-ENUM", Dimension: "CONSISTENCY", Type: RuleTypeEnum, Target: "status", Parameters: map[string]any{"values": []any{"ACTIVE", "INACTIVE"}}, Required: true, Severity: "HIGH"},
		{ID: "R-REGEX", Dimension: "CONSISTENCY", Type: RuleTypeRegex, Target: "code", Parameters: map[string]any{"pattern": "^[A-Z]{2}-[0-9]+$"}, Required: true, Severity: "HIGH"},
		{ID: "R-FRESH", Dimension: "TIMELINESS", Type: RuleTypeFreshness, Threshold: 24, Required: true, Severity: "CRITICAL"},
		{ID: "R-REF", Dimension: "ACCURACY", Type: RuleTypeReferenceMatch, Threshold: 0.01, Parameters: map[string]any{"metric": "errorRate", "operator": "lte"}, Required: true, Severity: "HIGH"},
		{ID: "R-LINEAGE", Dimension: "TRACEABILITY", Type: RuleTypeLineagePresent, Required: true, Severity: "HIGH"},
		{ID: "R-EVIDENCE", Dimension: "TRACEABILITY", Type: RuleTypeEvidencePresent, Required: true, Severity: "HIGH"},
	}
	findings, _, err := Evaluate(policy, DatasetContext{
		Table: tabular.Table{
			Headers: []string{"id", "score", "status", "code"},
			Rows:    []map[string]string{{"id": "1", "score": "80", "status": "ACTIVE", "code": "AA-1"}, {"id": "2", "score": "90", "status": "INACTIVE", "code": "BB-2"}},
		},
		Metadata:        map[string]any{"errorRate": 0.001},
		ReadyAt:         &readyAt,
		Now:             readyAt.Add(2 * time.Hour),
		LineagePresent:  &lineage,
		EvidencePresent: &evidence,
	})
	if err != nil {
		t.Fatalf("evaluate generic rules: %v", err)
	}
	if len(findings) != len(policy.Spec.Rules) {
		t.Fatalf("findings = %d, want %d", len(findings), len(policy.Spec.Rules))
	}
	for _, finding := range findings {
		if finding.Status != domain.FindingPass {
			t.Fatalf("rule %s status = %s, want PASS; observed=%#v", finding.RuleID, finding.Status, finding.Observed)
		}
		if _, ok := finding.Observed["affectedCount"]; !ok {
			t.Fatalf("rule %s lacks standard affectedCount", finding.RuleID)
		}
	}
}

func TestGenericEvaluatorIsDeterministicForFixedEvaluationTime(t *testing.T) {
	readyAt := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	lineage := true
	policy := singleRulePolicySet([]Rule{
		{ID: "R-COMPLETE", Dimension: "COMPLETENESS", Type: RuleTypeCompletenessRatio, Target: "id", Threshold: 1, Required: true, Severity: "CRITICAL"},
		{ID: "R-FRESH", Dimension: "TIMELINESS", Type: RuleTypeFreshness, Threshold: 24, Required: true, Severity: "CRITICAL"},
		{ID: "R-LINEAGE", Dimension: "TRACEABILITY", Type: RuleTypeLineagePresent, Required: true, Severity: "HIGH"},
	})
	ctx := DatasetContext{
		Table:          tabular.Table{Headers: []string{"id"}, Rows: []map[string]string{{"id": "1"}, {"id": "2"}}},
		ReadyAt:        &readyAt,
		Now:            readyAt.Add(2 * time.Hour),
		LineagePresent: &lineage,
	}

	findingsA, metricsA, err := Evaluate(policy, ctx)
	if err != nil {
		t.Fatalf("first evaluation: %v", err)
	}
	findingsB, metricsB, err := Evaluate(policy, ctx)
	if err != nil {
		t.Fatalf("second evaluation: %v", err)
	}
	if !reflect.DeepEqual(findingsA, findingsB) {
		t.Fatalf("fixed-time findings are not deterministic:\nA=%#v\nB=%#v", findingsA, findingsB)
	}
	if !reflect.DeepEqual(metricsA, metricsB) {
		t.Fatalf("fixed-time metrics are not deterministic:\nA=%#v\nB=%#v", metricsA, metricsB)
	}
}

func TestSixDimensionsProduceSummariesAndNotApplicableStates(t *testing.T) {
	lineage := true
	findings, _, err := Evaluate(Policy{Spec: struct {
		ProductCode string `yaml:"productCode"`
		Rules       []Rule `yaml:"rules"`
		Gate        struct {
			CriticalFailure string `yaml:"criticalFailure"`
			HighFailure     string `yaml:"highFailure"`
			WarningFailure  string `yaml:"warningFailure"`
		} `yaml:"gate"`
	}{Rules: []Rule{
		{ID: "C", Dimension: "COMPLETENESS", Type: RuleTypeNotNull, Target: "id", Threshold: 1, Required: true, Severity: "CRITICAL"},
		{ID: "A", Dimension: "ACCURACY", Type: RuleTypeRange, Target: "score", Parameters: map[string]any{"min": 0, "max": 100}, Required: true, Severity: "CRITICAL"},
		{ID: "S", Dimension: "CONSISTENCY", Type: RuleTypeEnum, Target: "status", Parameters: map[string]any{"values": []any{"OK"}}, Required: true, Severity: "CRITICAL"},
		{ID: "U", Dimension: "UNIQUENESS", Type: RuleTypeUnique, Target: "id", Threshold: 1, Required: true, Severity: "CRITICAL"},
		{ID: "T", Dimension: "TIMELINESS", Type: RuleTypeFreshness, Threshold: 24, Required: true, Severity: "CRITICAL"},
		{ID: "L", Dimension: "TRACEABILITY", Type: RuleTypeLineagePresent, Required: true, Severity: "HIGH"},
	}}}, DatasetContext{
		Table:          tabular.Table{Headers: []string{"id", "score", "status"}, Rows: []map[string]string{{"id": "1", "score": "10", "status": "OK"}}},
		ReadyAt:        timePtr(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)),
		Now:            time.Date(2026, 9, 20, 1, 0, 0, 0, time.UTC),
		LineagePresent: &lineage,
	})
	if err != nil {
		t.Fatalf("evaluate six dimensions: %v", err)
	}
	summaries := domain.SummarizeDimensions(findings)
	for _, dimension := range domain.QualityDimensions {
		if summaries[dimension].Status == domain.DimensionNotApplicable {
			t.Fatalf("dimension %s unexpectedly N/A", dimension)
		}
	}
	if summaries["COMPLETENESS"].Status != domain.DimensionPass || summaries["TRACEABILITY"].Status != domain.DimensionPass {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
	optional, _, err := Evaluate(Policy{Spec: struct {
		ProductCode string `yaml:"productCode"`
		Rules       []Rule `yaml:"rules"`
		Gate        struct {
			CriticalFailure string `yaml:"criticalFailure"`
			HighFailure     string `yaml:"highFailure"`
			WarningFailure  string `yaml:"warningFailure"`
		} `yaml:"gate"`
	}{Rules: []Rule{{ID: "optional", Dimension: "TRACEABILITY", Type: RuleTypeEvidencePresent, Required: false, Severity: "HIGH"}}}}, DatasetContext{})
	if err != nil || optional[0].Status != domain.FindingSkipped {
		t.Fatalf("optional unavailable traceability = %#v, err=%v", optional, err)
	}
	if got := domain.SummarizeDimensions(optional)["TRACEABILITY"].Status; got != domain.DimensionNotApplicable {
		t.Fatalf("optional traceability status = %s, want N/A", got)
	}
}

func TestGenericRulesFailClosedOnBadEvidence(t *testing.T) {
	for _, value := range []any{math.NaN(), "Inf"} {
		findings, _, err := Evaluate(singleRulePolicy(Rule{ID: "R", Dimension: "ACCURACY", Type: RuleTypeReferenceMatch, Threshold: 0.1, Parameters: map[string]any{"metric": "rate", "operator": "lte"}, Required: true, Severity: "CRITICAL"}), DatasetContext{Metadata: map[string]any{"rate": value}})
		if err != nil {
			t.Fatalf("evaluate bad evidence: %v", err)
		}
		if findings[0].Status != domain.FindingFail {
			t.Fatalf("bad evidence %v passed", value)
		}
	}
}

func TestReferenceMatchPreservesLargeIntegerPrecision(t *testing.T) {
	rule := Rule{ID: "R", Dimension: "ACCURACY", Type: RuleTypeReferenceMatch, Threshold: int64(9007199254740992), Parameters: map[string]any{
		"metric": "count", "operator": "eq",
	}, Required: true, Severity: "CRITICAL"}
	finding := evaluateSingleRule(t, rule, DatasetContext{Metadata: map[string]any{"count": int64(9007199254740993)}})
	if finding.Status != domain.FindingFail {
		t.Fatalf("distinct large integer metric passed equality comparison: %#v", finding)
	}
}

func TestRangeAndNullSemantics(t *testing.T) {
	rule := Rule{ID: "R", Dimension: "ACCURACY", Type: RuleTypeRange, Target: "coverage", Parameters: map[string]any{"min": 0, "max": 100, "allowNull": true}, Required: true, Severity: "CRITICAL"}
	ctx := DatasetContext{Table: tabular.Table{Headers: []string{"coverage"}, Rows: []map[string]string{{"coverage": "100"}, {"coverage": ""}, {"coverage": "33.33"}}}}
	finding := evaluateSingleRule(t, rule, ctx)
	if finding.Status != domain.FindingPass || finding.Observed["invalid"] != 0 {
		t.Fatalf("null coverage handling = %#v", finding)
	}
	for _, value := range []string{"NaN", "Inf", "-Inf"} {
		ctx.Table.Rows = []map[string]string{{"coverage": value}}
		if finding := evaluateSingleRule(t, rule, ctx); finding.Status != domain.FindingFail {
			t.Fatalf("non-finite value %q passed: %#v", value, finding)
		}
	}
}

func TestRangePreservesLargeIntegerPrecision(t *testing.T) {
	rule := Rule{ID: "R", Dimension: "ACCURACY", Type: RuleTypeRange, Target: "amount", Parameters: map[string]any{
		"min": int64(0), "max": int64(9007199254740992), "allowNull": false,
	}, Required: true, Severity: "CRITICAL"}
	finding := evaluateSingleRule(t, rule, DatasetContext{Table: tabular.Table{
		Headers: []string{"amount"}, Rows: []map[string]string{{"amount": "9007199254740993"}},
	}})
	if finding.Status != domain.FindingFail {
		t.Fatalf("out-of-range large integer passed: %#v", finding)
	}
}

func TestEvaluateRejectsMalformedAllowNull(t *testing.T) {
	policy := singleRulePolicy(Rule{
		ID:        "R",
		Dimension: "ACCURACY",
		Type:      RuleTypeRange,
		Target:    "amount",
		Parameters: map[string]any{
			"min":       0,
			"max":       1,
			"allowNull": "flase",
		},
		Required: true,
		Severity: "CRITICAL",
	})
	if _, _, err := Evaluate(policy, DatasetContext{Table: tabular.Table{Headers: []string{"amount"}}}); err == nil {
		t.Fatal("malformed allowNull was accepted during evaluation")
	}
}

func TestOptionalConditionalConsistencySkipsMissingConditionField(t *testing.T) {
	policy := singleRulePolicy(Rule{
		ID:        "R",
		Dimension: "CONSISTENCY",
		Type:      RuleTypeConditionalConsistency,
		Target:    "value",
		Parameters: map[string]any{
			"conditionField":    "status",
			"whenMissing":       "N/A",
			"whenPresentValues": []any{"VALID"},
		},
		Required: false,
		Severity: "HIGH",
	})
	findings, _, err := Evaluate(policy, DatasetContext{Table: tabular.Table{
		Headers: []string{"value"},
		Rows:    []map[string]string{{"value": "N/A"}},
	}})
	if err != nil {
		t.Fatalf("evaluate optional conditional rule: %v", err)
	}
	if findings[0].Status != domain.FindingSkipped {
		t.Fatalf("finding status = %s, want SKIPPED", findings[0].Status)
	}
}

func TestFindingSamplesNeverContainRawCells(t *testing.T) {
	secret := "secret@example.com"
	policy := singleRulePolicySet([]Rule{
		{ID: "R-ENUM", Dimension: "CONSISTENCY", Type: RuleTypeEnum, Target: "email", Parameters: map[string]any{"values": []any{"known@example.com"}}, Required: true, Severity: "HIGH"},
		{ID: "R-REGEX", Dimension: "CONSISTENCY", Type: RuleTypeRegex, Target: "email", Parameters: map[string]any{"pattern": `^known@`}, Required: true, Severity: "HIGH"},
		{ID: "R-RANGE", Dimension: "ACCURACY", Type: RuleTypeRange, Target: "email", Parameters: map[string]any{"min": 0, "max": 1, "allowNull": false}, Required: true, Severity: "HIGH"},
		{ID: "R-UNIQUE", Dimension: "UNIQUENESS", Type: RuleTypeUnique, Target: "email", Threshold: 1, Required: true, Severity: "HIGH"},
	})
	findings, _, err := Evaluate(policy, DatasetContext{Table: tabular.Table{
		Headers: []string{"email"},
		Rows:    []map[string]string{{"email": secret}, {"email": secret}},
	}})
	if err != nil {
		t.Fatalf("evaluate sensitive samples: %v", err)
	}
	for _, finding := range findings {
		if strings.Contains(fmt.Sprintf("%#v", finding.Observed["sample"]), secret) {
			t.Fatalf("rule %s leaked raw cell in sample: %#v", finding.RuleID, finding.Observed)
		}
	}
}

func TestParkRuleSetUsesGenericTypes(t *testing.T) {
	root := repoRoot(t)
	policy, err := LoadPolicy(filepath.Join(root, "industry-packs", "park", "quality", "enterprise-activity-quality-v1.yaml"))
	if err != nil {
		t.Fatalf("load Park policy: %v", err)
	}
	for _, rule := range policy.Spec.Rules {
		if rule.Type == "" || rule.Type == "made_up" {
			t.Fatalf("rule %s has no generic type", rule.ID)
		}
		if rule.Dimension == "CONFORMITY" {
			t.Fatalf("rule %s still uses retired CONFORMITY dimension", rule.ID)
		}
	}
}

func TestEnterpriseLeaseEnergyFixturesUseTheSameEvaluator(t *testing.T) {
	root := repoRoot(t)
	cases := []struct {
		name  string
		file  string
		rules []Rule
	}{
		{
			name: "enterprise",
			file: filepath.Join(root, "examples", "enterprise-activity", "data", "enterprise.csv"),
			rules: []Rule{
				{ID: "enterprise-code", Dimension: "COMPLETENESS", Type: RuleTypeCompletenessRatio, Target: "unified_social_credit_code", Threshold: 0.5, Required: true, Severity: "HIGH"},
				{ID: "enterprise-status", Dimension: "CONSISTENCY", Type: RuleTypeEnum, Target: "company_status", Parameters: map[string]any{"values": []any{"ACTIVE"}}, Required: true, Severity: "HIGH"},
			},
		},
		{
			name: "lease",
			file: filepath.Join(root, "examples", "enterprise-activity", "data", "lease.csv"),
			rules: []Rule{
				{ID: "lease-id", Dimension: "UNIQUENESS", Type: RuleTypeUnique, Target: "lease_id", Threshold: 1, Required: true, Severity: "CRITICAL"},
				{ID: "lease-status", Dimension: "CONSISTENCY", Type: RuleTypeEnum, Target: "lease_status", Parameters: map[string]any{"values": []any{"ACTIVE"}}, Required: true, Severity: "HIGH"},
			},
		},
		{
			name: "energy",
			file: filepath.Join(root, "examples", "enterprise-activity", "data", "energy.csv"),
			rules: []Rule{
				{ID: "energy-meter", Dimension: "COMPLETENESS", Type: RuleTypeNotNull, Target: "meter_id", Threshold: 1, Required: true, Severity: "CRITICAL"},
				{ID: "energy-nonnegative", Dimension: "ACCURACY", Type: RuleTypeRange, Target: "energy_kwh", Parameters: map[string]any{"min": 0, "max": 100000, "allowNull": false}, Required: true, Severity: "CRITICAL"},
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			file, err := os.Open(testCase.file)
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			table, err := tabular.ReadCSV(file)
			_ = file.Close()
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			findings, _, err := Evaluate(singleRulePolicySet(testCase.rules), DatasetContext{Table: table})
			if err != nil {
				t.Fatalf("evaluate fixture: %v", err)
			}
			if len(findings) != len(testCase.rules) {
				t.Fatalf("findings = %d, want %d", len(findings), len(testCase.rules))
			}
			summaries := domain.SummarizeDimensions(findings)
			if len(summaries) != len(domain.QualityDimensions) {
				t.Fatalf("dimension summaries = %d, want %d", len(summaries), len(domain.QualityDimensions))
			}
			for _, rule := range testCase.rules {
				if summaries[rule.Dimension].RuleCount == 0 {
					t.Fatalf("dimension %s has no summary for fixture rule %s", rule.Dimension, rule.ID)
				}
			}
			for _, finding := range findings {
				if finding.Observed["affectedCount"] == nil {
					t.Fatalf("finding %s lacks affected count: %#v", finding.RuleID, finding)
				}
			}
		})
	}
}

func evaluateSingleRule(t *testing.T, rule Rule, ctx DatasetContext) domain.Finding {
	t.Helper()
	findings, _, err := Evaluate(singleRulePolicy(rule), ctx)
	if err != nil {
		t.Fatalf("evaluate rule %s: %v", rule.ID, err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	return findings[0]
}

func singleRulePolicy(rule Rule) Policy {
	policy := Policy{}
	policy.Metadata.Version = "inline"
	policy.Spec.Rules = []Rule{rule}
	return policy
}

func singleRulePolicySet(rules []Rule) Policy {
	policy := Policy{}
	policy.Metadata.Version = "inline"
	policy.Spec.Rules = rules
	return policy
}

func timePtr(value time.Time) *time.Time { return &value }

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../"))
}
