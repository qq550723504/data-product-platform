package matching

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

type Lookup interface {
	FindByCanonicalKey(ctx context.Context, entityTypeID uuid.UUID, canonicalKey string) (*domain.Entity, error)
	FindByNameAddress(ctx context.Context, entityTypeID uuid.UUID, normalizedName, normalizedAddress string) (*domain.Entity, error)
	ListByLegalRepresentative(ctx context.Context, entityTypeID uuid.UUID, legalRepresentative string) ([]domain.Entity, error)
}

type Result struct {
	Entity     *domain.Entity
	Decision   domain.MatchDecision
	Method     string
	RuleID     string
	Confidence float64
}

type Engine struct {
	lookup Lookup
}

func NewEngine(lookup Lookup) *Engine {
	return &Engine{lookup: lookup}
}

func (e *Engine) Match(ctx context.Context, entityTypeID uuid.UUID, company NormalizedCompany, policy Policy) (Result, error) {
	rules := append([]Rule(nil), policy.Spec.Rules...)
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Priority < rules[j].Priority })

	for _, rule := range rules {
		result, matched, err := e.evaluateRule(ctx, entityTypeID, company, rule)
		if err != nil {
			return Result{}, err
		}
		if !matched {
			continue
		}
		result.RuleID = rule.ID
		result.Confidence = rule.Confidence
		result.Decision = domain.MatchDecision(rule.Decision)
		if result.Method == "" {
			result.Method = rule.ID
		}
		return result, nil
	}
	return Result{Decision: domain.DecisionUnresolved, Method: "NO_POLICY_RULE"}, nil
}

func (e *Engine) evaluateRule(ctx context.Context, entityTypeID uuid.UUID, company NormalizedCompany, rule Rule) (Result, bool, error) {
	if rule.When == nil || len(rule.When.All) == 0 {
		return Result{Decision: domain.MatchDecision(rule.Decision), Method: "DEFAULT"}, true, nil
	}

	// Strong identifier rule.
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

	// Deterministic normalized business-key rule.
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

	// Review candidate rule: fuzzy normalized name + exact legal representative.
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
