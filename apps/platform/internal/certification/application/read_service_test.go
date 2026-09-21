package application

import (
	"testing"

	"github.com/google/uuid"
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
