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


func TestCertificationScopeCoversRequestedScope(t *testing.T) {
	resourceID := uuid.New()
	inputs := []rightsinfra.LineageInput{{
		DatasetVersionID: uuid.New(),
		DataResourceID:   resourceID,
		ResourceMapped:   true,
	}}
	profile := certificationdomain.ProfileSnapshot{
		CertificationProfile: certificationdomain.CertificationProfile{
			Rights: certificationdomain.RightsRequirement{
				Required: true,
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

	if !certificationScopeCovers(profile, DeliveryEligibilityQuery{ScopeType: "ALL_RESOURCE"}, inputs) {
		t.Fatal("matching ALL_RESOURCE scope was rejected")
	}
	if !certificationScopeCovers(profile, DeliveryEligibilityQuery{ScopeType: "OBJECT", ScopeRef: "object-1"}, inputs) {
		t.Fatal("resource-wide certified scope did not cover a narrower requested scope")
	}

	profile.Rights.Scopes.Values = []certificationdomain.ScopeRef{{Type: "OBJECT", Ref: "object-1"}}
	if certificationScopeCovers(profile, DeliveryEligibilityQuery{ScopeType: "OBJECT", ScopeRef: "object-2"}, inputs) {
		t.Fatal("different explicit scope was accepted")
	}
	if certificationScopeCovers(profile, DeliveryEligibilityQuery{ScopeType: "ALL_RESOURCE"}, inputs) {
		t.Fatal("narrow frozen scope incorrectly covered ALL_RESOURCE")
	}
}
