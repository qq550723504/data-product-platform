package matching

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/resolution"
)

const (
	ruleEngineName    = "RULES"
	ruleEngineVersion = "1"
)

type Lookup interface {
	FindByCanonicalKey(ctx context.Context, entityTypeID uuid.UUID, canonicalKey string) (*domain.Entity, error)
	FindByNameAddress(ctx context.Context, entityTypeID uuid.UUID, normalizedName, normalizedAddress string) (*domain.Entity, error)
	ListByLegalRepresentative(ctx context.Context, entityTypeID uuid.UUID, legalRepresentative string) ([]domain.Entity, error)
	ListActive(ctx context.Context, entityTypeID uuid.UUID) ([]domain.Entity, error)
}

type Result struct {
	Entity        *domain.Entity
	Decision      domain.MatchDecision
	Method        string
	RuleID        string
	Confidence    float64
	EngineName    string
	EngineVersion string
	ModelVersion  string
}

type Engine struct {
	lookup    Lookup
	candidate resolution.CandidateGenerator
}

func NewEngine(lookup Lookup) *Engine {
	return &Engine{lookup: lookup}
}

func NewEngineWithCandidateGenerator(lookup Lookup, generator resolution.CandidateGenerator) *Engine {
	return &Engine{lookup: lookup, candidate: generator}
}

func (e *Engine) Match(ctx context.Context, entityTypeID uuid.UUID, company NormalizedCompany, policy Policy) (Result, error) {
	rules := append([]Rule(nil), policy.Spec.Rules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })

	fallback := Result{
		Decision:      domain.DecisionUnresolved,
		Method:        "NO_POLICY_RULE",
		EngineName:    ruleEngineName,
		EngineVersion: ruleEngineVersion,
	}
	for _, rule := range rules {
		// An unconditional UNRESOLVED rule is a deterministic fallback, not a reason
		// to bypass an optional probabilistic candidate generator.
		if rule.When == nil && domain.MatchDecision(rule.Decision) == domain.DecisionUnresolved {
			fallback = Result{
				Decision:      domain.DecisionUnresolved,
				Method:        "DEFAULT",
				RuleID:        rule.ID,
				Confidence:    rule.Confidence,
				EngineName:    ruleEngineName,
				EngineVersion: ruleEngineVersion,
			}
			continue
		}

		result, matched, err := e.evaluateRule(ctx, entityTypeID, company, rule)
		if err != nil {
			return Result{}, err
		}
		if !matched {
			continue
		}
		result.RuleID = rule.ID
		if result.Confidence == 0 {
			result.Confidence = rule.Confidence
		}
		result.Decision = domain.MatchDecision(rule.Decision)
		if result.Method == "" {
			result.Method = rule.ID
		}
		result.EngineName = ruleEngineName
		result.EngineVersion = ruleEngineVersion
		return result, nil
	}

	if e.candidate != nil {
		result, found, err := e.probabilisticCandidate(ctx, entityTypeID, company, policy)
		if err != nil {
			return Result{}, err
		}
		if found {
			return result, nil
		}
	}
	return fallback, nil
}

func (e *Engine) probabilisticCandidate(ctx context.Context, entityTypeID uuid.UUID, company NormalizedCompany, policy Policy) (Result, bool, error) {
	entities, err := e.lookup.ListActive(ctx, entityTypeID)
	if err != nil {
		return Result{}, false, err
	}
	if len(entities) == 0 {
		return Result{}, false, nil
	}

	references := make([]resolution.ReferenceRecord, 0, len(entities))
	entityByID := make(map[uuid.UUID]*domain.Entity, len(entities))
	for i := range entities {
		entity := entities[i]
		entityCopy := entity
		entityByID[entity.ID] = &entityCopy
		references = append(references, resolution.ReferenceRecord{
			EntityID: entity.ID,
			Name:     entity.CanonicalName,
			Fields:   entityFields(entity),
		})
	}

	generated, err := e.candidate.Generate(ctx, resolution.CandidateRequest{
		EntityType: policy.Spec.EntityType,
		Source: resolution.MatchRecord{
			ID:   company.SourceCompanyID,
			Name: company.CompanyName,
			Fields: map[string]string{
				"unified_social_credit_code":    company.UnifiedSocialCreditCode,
				"normalized_company_name":       company.CompanyName,
				"legal_representative":          company.LegalRepresentative,
				"normalized_registered_address": company.RegisteredAddress,
				"entry_date":                    company.EntryDate,
				"company_status":                company.CompanyStatus,
			},
		},
		References:    references,
		PolicyRef:     policy.Metadata.Name,
		PolicyVersion: policy.Metadata.Version,
	})
	if err != nil {
		return Result{}, false, fmt.Errorf("generate probabilistic entity candidates: %w", err)
	}
	if len(generated) == 0 {
		return Result{}, false, nil
	}

	descriptor := e.candidate.Descriptor()
	candidates := append([]resolution.Candidate(nil), generated...)
	for i := range candidates {
		if candidates[i].Engine.Name == "" {
			candidates[i].Engine = descriptor
		}
		if err := candidates[i].Validate(); err != nil {
			return Result{}, false, fmt.Errorf("invalid candidate %d: %w", i, err)
		}
		if _, ok := entityByID[candidates[i].EntityID]; !ok {
			return Result{}, false, fmt.Errorf("candidate engine returned unknown entity %s", candidates[i].EntityID)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			return candidates[i].EntityID.String() < candidates[j].EntityID.String()
		}
		return candidates[i].Score > candidates[j].Score
	})
	best := candidates[0]
	result := Result{
		Confidence:    best.Score,
		Method:        strings.TrimSpace(best.Method),
		RuleID:        "PROBABILISTIC_CANDIDATE",
		EngineName:    strings.ToUpper(strings.TrimSpace(best.Engine.Name)),
		EngineVersion: strings.TrimSpace(best.Engine.Version),
		ModelVersion:  strings.TrimSpace(best.Engine.ModelVersion),
	}
	if result.Method == "" {
		result.Method = "PROBABILISTIC"
	}

	switch {
	case best.Score >= policy.Spec.Thresholds.AutoMatchMinimum:
		result.Decision = domain.DecisionAutoMatch
		result.Entity = entityByID[best.EntityID]
	case best.Score >= policy.Spec.Thresholds.ReviewMinimum:
		result.Decision = domain.DecisionReview
		result.Entity = entityByID[best.EntityID]
	default:
		// Keep low-confidence proposals out of canonical state. The score and engine
		// remain useful for diagnostics, while Core treats the record as unresolved.
		result.Decision = domain.DecisionUnresolved
	}
	return result, true, nil
}

func (e *Engine) evaluateRule(ctx context.Context, entityTypeID uuid.UUID, company NormalizedCompany, rule Rule) (Result, bool, error) {
	if rule.When == nil || len(rule.When.All) == 0 {
		return Result{Decision: domain.MatchDecision(rule.Decision), Method: "DEFAULT"}, true, nil
	}

	if hasPredicate(rule, "unified_social_credit_code", "EXACT_NON_EMPTY") {
		if company.UnifiedSocialCreditCode == "" {
			return Result{}, false, nil
		}
		entity, err := e.lookup.FindByCanonicalKey(ctx, entityTypeID, company.UnifiedSocialCreditCode)
		if err != nil {
			return Result{}, false, err
		}
		if entity == nil {
			return Result{}, false, nil
		}
		return Result{Entity: entity, Method: "USCC_EXACT"}, true, nil
	}

	if hasPredicate(rule, "normalized_company_name", "EXACT") && hasPredicate(rule, "normalized_registered_address", "EXACT") {
		entity, err := e.lookup.FindByNameAddress(ctx, entityTypeID, company.CompanyName, company.RegisteredAddress)
		if err != nil {
			return Result{}, false, err
		}
		if entity == nil {
			return Result{}, false, nil
		}
		return Result{Entity: entity, Method: "NAME_ADDRESS_EXACT"}, true, nil
	}

	threshold, hasSimilarity := predicateThreshold(rule, "normalized_company_name", "SIMILARITY_GTE")
	if hasSimilarity && hasPredicate(rule, "legal_representative", "EXACT_NON_EMPTY") {
		if company.LegalRepresentative == "" || company.CompanyName == "" {
			return Result{}, false, nil
		}
		entities, err := e.lookup.ListByLegalRepresentative(ctx, entityTypeID, company.LegalRepresentative)
		if err != nil {
			return Result{}, false, err
		}
		var best *domain.Entity
		bestSimilarity := 0.0
		for i := range entities {
			candidateName := normalizedAttribute(entities[i], "normalized_company_name")
			if candidateName == "" {
				candidateName = entities[i].CanonicalName
			}
			similarity := Similarity(company.CompanyName, candidateName)
			if similarity >= threshold && similarity > bestSimilarity {
				candidate := entities[i]
				best = &candidate
				bestSimilarity = similarity
			}
		}
		if best == nil {
			return Result{}, false, nil
		}
		confidence := rule.Confidence
		if bestSimilarity < confidence {
			confidence = bestSimilarity
		}
		return Result{Entity: best, Method: "NAME_SIMILAR_LEGAL_EXACT", Confidence: confidence}, true, nil
	}

	return Result{}, false, fmt.Errorf("unsupported matching rule %q", rule.ID)
}

func entityFields(entity domain.Entity) map[string]string {
	fields := make(map[string]string, len(entity.Attributes)+2)
	fields["canonical_key"] = entity.CanonicalKey
	fields["canonical_name"] = entity.CanonicalName
	for key, value := range entity.Attributes {
		fields[key] = strings.TrimSpace(fmt.Sprint(value))
	}
	return fields
}

func hasPredicate(rule Rule, field, operator string) bool {
	if rule.When == nil {
		return false
	}
	for _, predicate := range rule.When.All {
		if predicate.Field == field && predicate.Operator == operator {
			return true
		}
	}
	return false
}

func predicateThreshold(rule Rule, field, operator string) (float64, bool) {
	if rule.When == nil {
		return 0, false
	}
	for _, predicate := range rule.When.All {
		if predicate.Field == field && predicate.Operator == operator {
			return predicate.Value, true
		}
	}
	return 0, false
}

func normalizedAttribute(entity domain.Entity, key string) string {
	value, ok := entity.Attributes[key]
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
