package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/native"
)

type testQualityEngine struct {
	descriptor qualityengine.Descriptor
}

type mutableQualityEngine struct {
	descriptor qualityengine.Descriptor
}

func (e *mutableQualityEngine) Descriptor() qualityengine.Descriptor { return e.descriptor }
func (*mutableQualityEngine) Evaluate(context.Context, qualityengine.Request) (qualityengine.Result, error) {
	return qualityengine.Result{}, nil
}

func (e testQualityEngine) Descriptor() qualityengine.Descriptor { return e.descriptor }
func (testQualityEngine) Evaluate(context.Context, qualityengine.Request) (qualityengine.Result, error) {
	return qualityengine.Result{}, nil
}

func TestServiceRegistersAndSelectsQualityEngine(t *testing.T) {
	service := NewService("", nil, nil, nil, nil)
	custom := testQualityEngine{descriptor: qualityengine.Descriptor{Name: "REFERENCE", Version: "1"}}
	if err := service.RegisterEngine(custom); err != nil {
		t.Fatalf("register engine: %v", err)
	}
	selected, err := service.resolveEngine(" reference ")
	if err != nil {
		t.Fatalf("resolve custom engine: %v", err)
	}
	if selected.Descriptor().Name != "REFERENCE" {
		t.Fatalf("selected engine = %#v", selected.Descriptor())
	}
	nativeEngine, err := service.resolveEngine("")
	if err != nil {
		t.Fatalf("resolve default engine: %v", err)
	}
	if nativeEngine.Descriptor().Name != "native-quality" {
		t.Fatalf("default engine = %#v", nativeEngine.Descriptor())
	}
}

func TestServiceRejectsInvalidQualityEngineDescriptor(t *testing.T) {
	service := NewService("", nil, nil, nil, nil)
	for _, descriptor := range []qualityengine.Descriptor{
		{Name: "", Version: "1"},
		{Name: "REFERENCE", Version: ""},
		{Name: strings.Repeat("n", 129), Version: "1"},
		{Name: "REFERENCE", Version: strings.Repeat("v", 65)},
	} {
		if err := service.RegisterEngine(testQualityEngine{descriptor: descriptor}); err == nil {
			t.Fatalf("descriptor %#v unexpectedly accepted", descriptor)
		}
	}
	if _, err := service.resolveEngine("missing"); err == nil {
		t.Fatal("unregistered engine unexpectedly resolved")
	}
}

func TestNormalizeEngineFindingsRejectsSkippedRequiredRule(t *testing.T) {
	policy := native.Policy{}
	policy.Spec.Rules = []native.Rule{{
		ID: "REQUIRED", Dimension: "COMPLETENESS", Type: native.RuleTypeNotNull,
		Target: "id", Required: true, Severity: "CRITICAL",
	}}
	if _, err := normalizeEngineFindings(policy, []domain.Finding{{
		RuleID: "REQUIRED", Status: domain.FindingSkipped,
	}}); err == nil {
		t.Fatal("skipped required rule was accepted")
	}
}

func TestNormalizeEngineFindingsAllowsSkippedOptionalRule(t *testing.T) {
	policy := native.Policy{}
	policy.Spec.Rules = []native.Rule{{
		ID: "OPTIONAL", Dimension: "COMPLETENESS", Type: native.RuleTypeNotNull,
		Target: "id", Required: false, Severity: "WARNING",
	}}
	findings, err := normalizeEngineFindings(policy, []domain.Finding{{
		RuleID: "OPTIONAL", Status: domain.FindingSkipped, CreatedAt: time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("optional skipped rule rejected: %v", err)
	}
	if findings[0].Severity != "WARNING" || findings[0].Dimension != "COMPLETENESS" {
		t.Fatalf("Core metadata not restored: %#v", findings[0])
	}
	if !findings[0].CreatedAt.IsZero() {
		t.Fatalf("provider finding timestamp survived normalization: %s", findings[0].CreatedAt)
	}
}

func TestSanitizeErrorRejectsFreeFormClassifiedCode(t *testing.T) {
	raw := qualityengine.NewExecutionError(
		qualityengine.ExecutionErrorCode("https://provider.internal?token=secret"),
		true,
	)
	safe := qualityengine.SanitizeError(raw)
	if safe.Error() != "quality engine execution failed: PROVIDER_EXECUTION_FAILED" {
		t.Fatalf("unsafe classified error survived: %q", safe.Error())
	}
}

func TestSanitizeErrorPreservesAllowlistedCode(t *testing.T) {
	raw := qualityengine.NewExecutionError(qualityengine.ErrorProviderTimeout, true)
	safe := qualityengine.SanitizeError(raw)
	if safe.Error() != "quality engine execution failed: PROVIDER_TIMEOUT" {
		t.Fatalf("allowlisted classified error changed: %q", safe.Error())
	}
}

func TestNormalizeExternalEngineFindingsDropsFreeFormProviderPayloads(t *testing.T) {
	findings, metrics, err := normalizeExternalEngineFindings([]domain.Finding{{
		RuleID:  "R1",
		Status:  domain.FindingFail,
		Message: "https://provider.internal/error?token=secret",
		Observed: map[string]any{
			"observedValue": 0.5,
			"threshold":     1.0,
			"affectedCount": 2,
		},
	}})
	if err != nil {
		t.Fatalf("sanitize external finding: %v", err)
	}
	if findings[0].Message != "quality rule failed" {
		t.Fatalf("provider message survived: %q", findings[0].Message)
	}
	if _, ok := metrics["R1"]; !ok {
		t.Fatalf("metrics were not rebuilt from safe observation: %#v", metrics)
	}
}

func TestNormalizeExternalEngineFindingsRejectsSamplesAndFreeFormMetrics(t *testing.T) {
	if _, _, err := normalizeExternalEngineFindings([]domain.Finding{{
		RuleID: "R1", Status: domain.FindingFail,
		Observed: map[string]any{"sample": []any{"secret-row-value"}},
	}}); err == nil {
		t.Fatal("provider sample payload was accepted")
	}
	if _, _, err := normalizeExternalEngineFindings([]domain.Finding{{
		RuleID: "R1", Status: domain.FindingFail,
		Observed: map[string]any{"observedValue": "https://internal/token=secret"},
	}}); err == nil {
		t.Fatal("provider string observation was accepted")
	}
}

func TestRegisterEngineRejectsReservedOrDuplicateIdentity(t *testing.T) {
	service := NewService("", nil, nil, nil, nil)
	if err := service.RegisterEngine(testQualityEngine{
		descriptor: qualityengine.Descriptor{Name: "native-quality", Version: "999"},
	}); err == nil {
		t.Fatal("reserved native-quality identity was replaceable")
	}

	custom := testQualityEngine{descriptor: qualityengine.Descriptor{Name: "REFERENCE", Version: "1"}}
	if err := service.RegisterEngine(custom); err != nil {
		t.Fatalf("register reference engine: %v", err)
	}
	if err := service.RegisterEngine(testQualityEngine{
		descriptor: qualityengine.Descriptor{Name: " reference ", Version: "2"},
	}); err == nil {
		t.Fatal("duplicate normalized engine identity was replaceable")
	}
}

func TestSafeProviderNumberRejectsInvalidJSONNumber(t *testing.T) {
	if _, ok := safeProviderNumber(json.Number("https://internal?token=secret")); ok {
		t.Fatal("invalid json.Number was accepted")
	}
	if _, ok := safeProviderNumber(json.Number("1e10000")); ok {
		t.Fatal("non-finite json.Number was accepted")
	}
	if value, ok := safeProviderNumber(json.Number("1.25")); !ok || value == nil {
		t.Fatalf("valid json.Number rejected: %#v %v", value, ok)
	}
}

func TestRegisteredEngineDescriptorIsFrozen(t *testing.T) {
	service := NewService("", nil, nil, nil, nil)
	provider := &mutableQualityEngine{descriptor: qualityengine.Descriptor{
		Name: "external-reference", Version: "1", Capabilities: []string{"not_null"},
	}}
	if err := service.RegisterEngine(provider); err != nil {
		t.Fatalf("register mutable engine: %v", err)
	}
	provider.descriptor = qualityengine.Descriptor{
		Name: "native-quality", Version: "999", Capabilities: []string{"*"},
	}

	selected, descriptor, registeredName, err := service.resolveEngineRegistration("external-reference")
	if err != nil {
		t.Fatalf("resolve registered engine: %v", err)
	}
	if selected != provider {
		t.Fatal("resolved provider changed")
	}
	if registeredName != "external-reference" || descriptor.Name != "external-reference" ||
		descriptor.Version != "1" || len(descriptor.Capabilities) != 1 || descriptor.Capabilities[0] != "not_null" {
		t.Fatalf("registered descriptor was not frozen: name=%q descriptor=%#v", registeredName, descriptor)
	}
}

func TestSafeProviderNumberRequiresExactJSONNumberGrammar(t *testing.T) {
	for _, raw := range []string{"+1", "01", ".5", "1.", "NaN", "Infinity", "-Infinity"} {
		if _, ok := safeProviderNumber(json.Number(raw)); ok {
			t.Fatalf("non-JSON number %q was accepted", raw)
		}
	}
	for _, raw := range []string{"0", "-0", "1", "-1", "1.25", "1e3", "-2.5E-2"} {
		if _, ok := safeProviderNumber(json.Number(raw)); !ok {
			t.Fatalf("valid JSON number %q was rejected", raw)
		}
	}
}

func TestSafeProviderNumberCanonicalizesHugeJSONNumber(t *testing.T) {
	raw := json.Number("0." + strings.Repeat("0", 20000) + "1")
	value, ok := safeProviderNumber(raw)
	if !ok {
		t.Fatal("finite JSON number should be accepted")
	}
	if _, isFloat := value.(float64); !isFloat {
		t.Fatalf("provider json.Number was not canonicalized: %#v", value)
	}
}
