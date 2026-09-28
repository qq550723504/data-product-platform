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
			ID:                  uuid.New(),
			WorkspaceID:         uuid.New(),
			DatasetVersionID:    uuid.New(),
			QualityAssessmentID: uuid.New(),
			Profile: domain.ProfileSnapshot{
				CertificationProfile: domain.CertificationProfile{
					ProfileRef: domain.GoldCertificationProfileRef,
				},
			},
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


func TestParseHistoryPageBounds(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
		wantOK     bool
	}{
		{name: "defaults", query: "", wantLimit: 25, wantOffset: 0, wantOK: true},
		{name: "explicit", query: "?limit=50&offset=100", wantLimit: 50, wantOffset: 100, wantOK: true},
		{name: "max limit", query: "?limit=100", wantLimit: 100, wantOffset: 0, wantOK: true},
		{name: "zero limit", query: "?limit=0", wantOK: false},
		{name: "too large limit", query: "?limit=101", wantOK: false},
		{name: "negative offset", query: "?offset=-1", wantOK: false},
		{name: "non numeric", query: "?limit=nope", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/certifications"+tt.query, nil)
			response := httptest.NewRecorder()
			limit, offset, ok := parseHistoryPage(response, req)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v; body=%s", ok, tt.wantOK, response.Body.String())
			}
			if !ok {
				if response.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
				}
				return
			}
			if limit != tt.wantLimit || offset != tt.wantOffset {
				t.Fatalf("page = limit %d offset %d, want limit %d offset %d", limit, offset, tt.wantLimit, tt.wantOffset)
			}
		})
	}
}
