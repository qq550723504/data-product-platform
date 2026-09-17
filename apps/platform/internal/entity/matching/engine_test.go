package matching

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/resolution"
)

type fakeLookup struct {
	byKey  *domain.Entity
	byName *domain.Entity
	legal  []domain.Entity
	active []domain.Entity
}

func (f *fakeLookup) FindByCanonicalKey(context.Context, uuid.UUID, string) (*domain.Entity, error) {
	return f.byKey, nil
}
func (f *fakeLookup) FindByNameAddress(context.Context, uuid.UUID, string, string) (*domain.Entity, error) {
	return f.byName, nil
}
func (f *fakeLookup) ListByLegalRepresentative(context.Context, uuid.UUID, string) ([]domain.Entity, error) {
	return append([]domain.Entity(nil), f.legal...), nil
}
func (f *fakeLookup) ListActive(context.Context, uuid.UUID) ([]domain.Entity, error) {
	return append([]domain.Entity(nil), f.active...), nil
}

type fakeCandidateGenerator struct {
	calls      int
	descriptor resolution.EngineDescriptor
	candidates []resolution.Candidate
	err        error
}

func (f *fakeCandidateGenerator) Descriptor() resolution.EngineDescriptor { return f.descriptor }
func (f *fakeCandidateGenerator) Generate(context.Context, resolution.CandidateRequest) ([]resolution.Candidate, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
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

func TestProbabilisticAutoMatchRunsBeforeDeterministicReviewFallback(t *testing.T) {
	entity := testEntity("")
	entity.Attributes["normalized_company_name"] = "ACME TECHNOLOGY LTD"
	entity.Attributes["legal_representative"] = "Alice"
	lookup := &fakeLookup{legal: []domain.Entity{entity}, active: []domain.Entity{entity}}
	generator := &fakeCandidateGenerator{
		descriptor: resolution.EngineDescriptor{Name: "FAKE", Version: "2.1", ModelVersion: "company-v3"},
		candidates: []resolution.Candidate{{
			EntityID: entity.ID,
			Score:    0.97,
			Method:   "PROBABILISTIC_LINK",
		}},
	}

	result, err := NewEngineWithCandidateGenerator(lookup, generator).Match(context.Background(), uuid.New(), NormalizedCompany{
		SourceCompanyID:     "SRC-PROB-AUTO",
		CompanyName:         "ACME TECHNOLOGY LTD.",
		LegalRepresentative: "Alice",
	}, testPolicy())
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if generator.calls != 1 {
		t.Fatalf("probabilistic engine should run after deterministic review fallback, calls=%d", generator.calls)
	}
	if result.Decision != domain.DecisionAutoMatch || result.Entity == nil || result.Entity.ID != entity.ID {
		t.Fatalf("expected probabilistic AUTO_MATCH, got %#v", result)
	}
	if result.EngineName != "FAKE" || result.ModelVersion != "company-v3" {
		t.Fatalf("expected probabilistic provenance, got %#v", result)
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

func TestCandidateEngineFailureFallsBackToDeterministicReview(t *testing.T) {
	entity := testEntity("")
	entity.Attributes["normalized_company_name"] = "ACME TECHNOLOGY LTD"
	entity.Attributes["legal_representative"] = "Alice"
	lookup := &fakeLookup{legal: []domain.Entity{entity}, active: []domain.Entity{entity}}
	generator := &fakeCandidateGenerator{
		descriptor: resolution.EngineDescriptor{Name: "SPLINK", Version: "4.0.17", ModelVersion: "1.0.0"},
		err:        errors.New("temporary candidate service outage"),
	}

	result, err := NewEngineWithCandidateGenerator(lookup, generator).Match(context.Background(), uuid.New(), NormalizedCompany{
		SourceCompanyID:     "SRC-FALLBACK-ERROR",
		CompanyName:         "ACME TECHNOLOGY LTD.",
		LegalRepresentative: "Alice",
	}, testPolicy())
	if err != nil {
		t.Fatalf("optional candidate engine failure must not fail Core matching: %v", err)
	}
	if result.Decision != domain.DecisionReview || result.EngineName != ruleEngineName || result.RuleID != "COMPANY-NAME-LEGAL-REVIEW" {
		t.Fatalf("expected deterministic REVIEW fallback, got %#v", result)
	}
}

func TestBelowThresholdProbabilisticCandidateKeepsDeterministicReview(t *testing.T) {
	entity := testEntity("")
	entity.Attributes["normalized_company_name"] = "ACME TECHNOLOGY LTD"
	entity.Attributes["legal_representative"] = "Alice"
	lookup := &fakeLookup{legal: []domain.Entity{entity}, active: []domain.Entity{entity}}
	generator := &fakeCandidateGenerator{
		descriptor: resolution.EngineDescriptor{Name: "SPLINK", Version: "4.0.17", ModelVersion: "1.0.0"},
		candidates: []resolution.Candidate{{
			EntityID: entity.ID,
			Score:    0.61,
			Method:   "FELLEGI_SUNTER",
		}},
	}

	result, err := NewEngineWithCandidateGenerator(lookup, generator).Match(context.Background(), uuid.New(), NormalizedCompany{
		SourceCompanyID:     "SRC-FALLBACK-LOW",
		CompanyName:         "ACME TECHNOLOGY LTD.",
		LegalRepresentative: "Alice",
	}, testPolicy())
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if result.Decision != domain.DecisionReview || result.EngineName != ruleEngineName || result.RuleID != "COMPANY-NAME-LEGAL-REVIEW" {
		t.Fatalf("low probabilistic score must not erase deterministic REVIEW, got %#v", result)
	}
}

func TestRuleOnlyFuzzyReviewRemainsUnchangedWithoutCandidateEngine(t *testing.T) {
	entity := testEntity("")
	entity.Attributes["normalized_company_name"] = "ACME TECHNOLOGY LTD"
	entity.Attributes["legal_representative"] = "Alice"
	lookup := &fakeLookup{legal: []domain.Entity{entity}, active: []domain.Entity{entity}}

	result, err := NewEngine(lookup).Match(context.Background(), uuid.New(), NormalizedCompany{
		SourceCompanyID:     "SRC-RULE-ONLY",
		CompanyName:         "ACME TECHNOLOGY LTD.",
		LegalRepresentative: "Alice",
	}, testPolicy())
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if result.Decision != domain.DecisionReview || result.EngineName != ruleEngineName || result.RuleID != "COMPANY-NAME-LEGAL-REVIEW" {
		t.Fatalf("expected rule-only fuzzy REVIEW, got %#v", result)
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
		{
			ID:       "COMPANY-NAME-LEGAL-REVIEW",
			Priority: 30,
			When: &Condition{All: []Predicate{
				{Field: "normalized_company_name", Operator: "SIMILARITY_GTE", Value: 0.88},
				{Field: "legal_representative", Operator: "EXACT_NON_EMPTY"},
			}},
			Decision:   string(domain.DecisionReview),
			Confidence: 0.85,
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
