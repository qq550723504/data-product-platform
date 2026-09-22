package application

import (
	"testing"

	"github.com/google/uuid"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

func TestSameEligibilityLineageRequiresExactMappedMembership(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	firstResource, secondResource := uuid.New(), uuid.New()
	frozen := []rightsdomain.EffectiveRightsInput{
		{InputDatasetVersionID: second, DataResourceID: secondResource},
		{InputDatasetVersionID: first, DataResourceID: firstResource},
	}
	current := []rightsinfra.LineageInput{
		{DatasetVersionID: first, DataResourceID: firstResource, ResourceMapped: true},
		{DatasetVersionID: second, DataResourceID: secondResource, ResourceMapped: true},
	}
	if !sameEligibilityLineage(frozen, current) {
		t.Fatal("same lineage membership was rejected")
	}

	current = append(current, rightsinfra.LineageInput{DatasetVersionID: uuid.New(), DataResourceID: uuid.New(), ResourceMapped: true})
	if sameEligibilityLineage(frozen, current) {
		t.Fatal("additional current lineage input was accepted")
	}

	current = []rightsinfra.LineageInput{
		{DatasetVersionID: first, DataResourceID: firstResource, ResourceMapped: true},
		{DatasetVersionID: second, DataResourceID: secondResource, ResourceMapped: false},
	}
	if sameEligibilityLineage(frozen, current) {
		t.Fatal("unmapped current lineage input was accepted")
	}

	current = []rightsinfra.LineageInput{
		{DatasetVersionID: first, DataResourceID: uuid.New(), ResourceMapped: true},
		{DatasetVersionID: second, DataResourceID: secondResource, ResourceMapped: true},
	}
	if sameEligibilityLineage(frozen, current) {
		t.Fatal("resource drift for the same DatasetVersion was accepted")
	}
}

func TestCertificationRightsContextCoversRequestedContext(t *testing.T) {
	resourceID := uuid.New()
	inputs := []rightsinfra.LineageInput{{
		DatasetVersionID: uuid.New(),
		DataResourceID:   resourceID,
		ResourceMapped:   true,
	}}
	profile := certificationdomain.ProfileSnapshot{
		CertificationProfile: certificationdomain.CertificationProfile{
			Rights: certificationdomain.RightsRequirement{
				Required:  true,
				Purpose:   certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"INTERNAL_USE"}},
				Actions:   certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"READ"}},
				Consumers: certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"consumer-a"}},
				Scopes: certificationdomain.ScopeApplicability{
					Mode: certificationdomain.ApplicabilityExplicit,
					Values: []certificationdomain.ScopeRef{{
						Type: "ALL_RESOURCE",
						Ref:  resourceID.String(),
					}},
				},
			},
		},
	}
	base := DeliveryEligibilityQuery{
		Purpose:   "INTERNAL_USE",
		Action:    "READ",
		Consumer:  "consumer-a",
		ScopeType: "ALL_RESOURCE",
	}

	if !certificationRightsContextCovers(profile, base, inputs) {
		t.Fatal("matching frozen rights context was rejected")
	}
	if !certificationRightsContextCovers(profile, DeliveryEligibilityQuery{
		Purpose: "INTERNAL_USE", Action: "READ", Consumer: "consumer-a",
		ScopeType: "OBJECT", ScopeRef: "object-1",
	}, inputs) {
		t.Fatal("resource-wide certified scope did not cover a narrower requested scope")
	}

	for name, mutate := range map[string]func(*DeliveryEligibilityQuery){
		"purpose":  func(q *DeliveryEligibilityQuery) { q.Purpose = "EXTERNAL_USE" },
		"action":   func(q *DeliveryEligibilityQuery) { q.Action = "SHARE" },
		"consumer": func(q *DeliveryEligibilityQuery) { q.Consumer = "consumer-b" },
	} {
		t.Run(name, func(t *testing.T) {
			query := base
			mutate(&query)
			if certificationRightsContextCovers(profile, query, inputs) {
				t.Fatalf("mismatched %s was accepted", name)
			}
		})
	}

	profile.Rights.Scopes.Values = []certificationdomain.ScopeRef{{Type: "OBJECT", Ref: "object-1"}}
	query := base
	query.ScopeType = "OBJECT"
	query.ScopeRef = "object-2"
	if certificationRightsContextCovers(profile, query, inputs) {
		t.Fatal("different explicit scope was accepted")
	}
	if certificationRightsContextCovers(profile, base, inputs) {
		t.Fatal("narrow frozen scope incorrectly covered ALL_RESOURCE")
	}
}
