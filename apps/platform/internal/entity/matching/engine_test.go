package matching

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/resolution"
)

type fakeLookup struct {
	byKey  *domain.Entity
	active []domain.Entity
}

func (f *fakeLookup) FindByCanonicalKey(context.Context, uuid.UUID, string) (*domain.Entity, error) {
	return f.byKey, nil
}
func (f *fakeLookup) FindByNameAddress(context.Context, uuid.UUID, string, string) (*domain.Entity, error) {
	return nil, nil
}
func (f *fakeLookup) ListByLegalRepresentative(context.Context, uuid.UUID, string) ([]domain.Entity, error) {
	return nil, nil
}
func (f *fakeLookup) ListActive(context.Context, uuid.UUID) ([]domain.Entity, error) {
	return append([]domain.Entity(nil), f.active...), nil
}

type fakeCandidateGenerator struct {
	calls      int
	descriptor resolution.EngineDescriptor
	candidates []resolution.Candidate
}

func (f *fakeCandidateGenerator) Descriptor() resolution.EngineDescriptor { return f.descriptor }
func (f *fakeCandidateGenerator) Generate(context.Context, resolution.CandidateRequest) ([]resolution.Candidate, error) {
	f.calls++
	return append([]resolution.Candidate(nil), f.candidates...), nil
}

func TestExactUSCCBypassesProbabilisticCandidateGenerator(t *testing.T) {
	entity := testEntity("91440300EXACT")
	lookup := &fakeLookup{byKey: &entity, active: []domain.Entity{entity}}
	generator := &fakeCandidateGenerator{descriptor: resolution.EngineDescriptor{Name: "FAKE", Version: "1"}}
	engine := NewEngineWithCandidateGenerator(lookup, generator)

	result, err := engine.Match(context.Background(), uuid.New(), NormalizedCompany{
		SourceCompanyID:         "SRC-1",
		CompanyName:             "EXACT COMPANY",
		UnifiedSocialCreditCode: "91440300EXACT",
	}, testPolicy())
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if result.Decision != domain.DecisionAutoMatch || result.Entity == nil || result.Entity.ID != entity.ID {
		t.Fatalf("unexpected deterministic result: %#v", result)
	}
	if result.EngineName != ruleEngineName || generator.calls != 0 {
		t.Fatalf("exact rule must bypass probabilistic generator: result=%#v calls=%d", result, generator.calls)
	}
}

func TestProbabilisticCandidateMapsScoreToExistingReviewFlow(t *testing.T) {
	entity := testEntity("")
	lookup := &fakeLookup{active: []domain.Entity{entity}}
	generator := &fakeCandidateGenerator{
		descriptor: resolution.EngineDescriptor{Name: "FAKE", Version: "2.1", ModelVersion: "company-v3"},
		candidates: []resolution.Candidate{{
			EntityID: entity.ID,
			Score:    0.82,
			Method:   "PROBABILISTIC_LINK",
		}},
	}
	engine := NewEngineWithCandidateGenerator(lookup, generator)

	result, err := engine.Match(context.Background(), uuid.New(), NormalizedCompany{
		SourceCompanyID: "SRC-2",
		CompanyName:     "ACME TECH",
	}, testPolicy())
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if result.Decision != domain.DecisionReview || result.Entity == nil || result.Entity.ID != entity.ID {
		t.Fatalf("expected existing REVIEW semantics, got %#v", result)
	}
	if result.EngineName != "FAKE" || result.EngineVersion != "2.1" || result.ModelVersion != "company-v3" {
		t.Fatalf("missing candidate provenance: %#v", result)
	}
}

func TestProbabilisticCandidateCanAutoMatchOnlyAbovePolicyThreshold(t *testing.T) {
	entity := testEntity("")
	lookup := &fakeLookup{active: []domain.Entity{entity}}
	generator := &fakeCandidateGenerator{
		descriptor: resolution.EngineDescriptor{Name: "FAKE", Version: "1"},
		candidates: []resolution.Candidate{{EntityID: entity.ID, Score: 0.97}},
	}
	result, err := NewEngineWithCandidateGenerator(lookup, generator).Match(
		context.Background(), uuid.New(), NormalizedCompany{SourceCompanyID: "SRC-3", CompanyName: "ACME"}, testPolicy(),
	)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if result.Decision != domain.DecisionAutoMatch || result.Entity == nil {
		t.Fatalf("expected auto match above threshold, got %#v", result)
	}
}

func TestRuleOnlyFallbackRemainsUnchangedWhenNoCandidateEngineConfigured(t *testing.T) {
	result, err := NewEngine(&fakeLookup{}).Match(
		context.Background(), uuid.New(), NormalizedCompany{SourceCompanyID: "SRC-4", CompanyName: "UNKNOWN"}, testPolicy(),
	)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if result.Decision != domain.DecisionUnresolved || result.RuleID != "COMPANY-DEFAULT" || result.EngineName != ruleEngineName {
		t.Fatalf("unexpected rule-only fallback: %#v", result)
	}
}

func testPolicy() Policy {
	var policy Policy
	policy.Metadata.Name = "park-company-match"
	policy.Metadata.Version = "1.0.0"
	policy.Spec.EntityType = "COMPANY"
	policy.Spec.Thresholds.AutoMatchMinimum = 0.95
	policy.Spec.Thresholds.ReviewMinimum = 0.75
	policy.Spec.Rules = []Rule{
		{
			ID:       "COMPANY-USCC-EXACT",
			Priority: 10,
			When: &Condition{All: []Predicate{{
				Field: "unified_social_credit_code", Operator: "EXACT_NON_EMPTY",
			}}},
			Decision:   string(domain.DecisionAutoMatch),
			Confidence: 1,
		},
		{ID: "COMPANY-DEFAULT", Priority: 999, Decision: string(domain.DecisionUnresolved)},
	}
	return policy
}

func testEntity(canonicalKey string) domain.Entity {
	return domain.Entity{
		ID:            uuid.New(),
		WorkspaceID:   uuid.New(),
		EntityTypeID:  uuid.New(),
		CanonicalKey:  canonicalKey,
		CanonicalName: "Acme Technology Co., Ltd.",
		Attributes: map[string]any{
			"normalized_company_name":       "ACME TECHNOLOGY CO LTD",
			"normalized_registered_address": "1 MAIN ROAD",
		},
		Status: domain.EntityActive,
	}
}
