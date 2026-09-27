package application

import (
	"context"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/native"
)

type testQualityEngine struct {
	descriptor qualityengine.Descriptor
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
