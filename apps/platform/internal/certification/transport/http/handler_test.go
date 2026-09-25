package certificationhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
)

func TestHistoryRejectsInvalidWorkspaceID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/not-a-uuid/dataset-versions/not-a-uuid/certifications", nil)
	req.SetPathValue("workspaceId", "not-a-uuid")
	req.SetPathValue("versionId", "not-a-uuid")
	response := httptest.NewRecorder()

	(&Handler{}).history(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "INVALID_WORKSPACE_ID") {
		t.Fatalf("body = %s, want INVALID_WORKSPACE_ID", response.Body.String())
	}
}

func TestHistoryItemResponseIncludesFrozenGoldProductionProof(t *testing.T) {
	bindingID := uuid.New()
	snapshotID := uuid.New()
	item := application.CertificationHistoryItem{
		Certification: domain.DatasetCertification{
			ID:                            uuid.New(),
			WorkspaceID:                   uuid.New(),
			DatasetVersionID:              uuid.New(),
			QualityAssessmentID:           uuid.New(),
			Profile:                       domain.ProfileSnapshot{ProfileRef: domain.GoldCertificationProfileRef},
			GoldProductionBindingID:       &bindingID,
			AnnotationSnapshotID:          &snapshotID,
			AnnotationSnapshotRootHash:    strings.Repeat("a", 64),
			AnnotationSchemaSHA256:        strings.Repeat("b", 64),
			AnnotationTaxonomySHA256:      strings.Repeat("c", 64),
			GoldProductionBindingRootHash: strings.Repeat("d", 64),
			Decision:                      domain.DecisionCertified,
			Blockers:                      []domain.Blocker{},
			Reason:                        "all profile requirements satisfied",
		},
	}

	response := historyItemResponse(item)

	assertUUID := func(key string, want uuid.UUID) {
		t.Helper()
		got, ok := response[key].(*uuid.UUID)
		if !ok || got == nil || *got != want {
			t.Fatalf("%s = %#v, want %s", key, response[key], want)
		}
	}
	assertString := func(key, want string) {
		t.Helper()
		if got, ok := response[key].(string); !ok || got != want {
			t.Fatalf("%s = %#v, want %q", key, response[key], want)
		}
	}

	assertUUID("goldProductionBindingId", bindingID)
	assertUUID("annotationSnapshotId", snapshotID)
	assertString("annotationSnapshotRootHash", strings.Repeat("a", 64))
	assertString("annotationSchemaSha256", strings.Repeat("b", 64))
	assertString("annotationTaxonomySha256", strings.Repeat("c", 64))
	assertString("goldProductionBindingRootHash", strings.Repeat("d", 64))
}
