package resolution

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
)

type fakeGenerator struct {
	descriptor EngineDescriptor
	candidates []Candidate
}

func (f *fakeGenerator) Descriptor() EngineDescriptor { return f.descriptor }
func (f *fakeGenerator) Generate(context.Context, CandidateRequest) ([]Candidate, error) {
	return append([]Candidate(nil), f.candidates...), nil
}

func TestRegistryUsesStableEngineNamesAndAllowsRuleOnlyFallback(t *testing.T) {
	registry, err := NewRegistry(&fakeGenerator{descriptor: EngineDescriptor{Name: " splink ", Version: "1"}})
	if err != nil {
		t.Fatalf("new registry: %v", err)
	}
	engine, ok := registry.Get("SPLINK")
	if !ok || engine.Descriptor().Name != " splink " {
		t.Fatalf("expected registered SPLINK engine, got %#v ok=%v", engine, ok)
	}
	if _, ok := registry.Get("missing"); ok {
		t.Fatal("missing optional engine should allow rule-only fallback")
	}
}

func TestCandidateValidation(t *testing.T) {
	candidate := Candidate{
		EntityID: uuid.New(),
		Score:    0.91,
		Engine:   EngineDescriptor{Name: "FAKE", Version: "1"},
	}
	if err := candidate.Validate(); err != nil {
		t.Fatalf("valid candidate: %v", err)
	}
	candidate.Score = 1.01
	if err := candidate.Validate(); err == nil {
		t.Fatal("expected score validation error")
	}
	for _, score := range []float64{
		math.NaN(),
		math.Inf(1),
		math.Inf(-1),
	} {
		candidate.Score = score
		if err := candidate.Validate(); err == nil {
			t.Fatalf("expected validation error for non-finite score %v", score)
		}
	}
}
