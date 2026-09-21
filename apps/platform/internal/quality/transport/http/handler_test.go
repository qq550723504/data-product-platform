package qualityhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

func TestRunRejectsExplicitNilAssessmentAttemptID(t *testing.T) {
	versionID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dataset-versions/"+versionID.String()+"/quality-checks", strings.NewReader(`{
		"workspaceId":"11111111-1111-1111-1111-111111111111",
		"assessmentAttemptId":"00000000-0000-0000-0000-000000000000"
	}`))
	req.SetPathValue("versionId", versionID.String())
	response := httptest.NewRecorder()

	(&Handler{}).run(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "INVALID_ASSESSMENT_ATTEMPT_ID") {
		t.Fatalf("response = %s, want invalid attempt id error", response.Body.String())
	}
}

func TestRunRejectsMissingAssessmentAttemptID(t *testing.T) {
	versionID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dataset-versions/"+versionID.String()+"/quality-checks", strings.NewReader(`{
		"workspaceId":"11111111-1111-1111-1111-111111111111"
	}`))
	req.SetPathValue("versionId", versionID.String())
	response := httptest.NewRecorder()

	(&Handler{}).run(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "MISSING_ASSESSMENT_ATTEMPT_ID") {
		t.Fatalf("response = %s, want missing attempt id error", response.Body.String())
	}
}

func TestReportRequiresWorkspaceID(t *testing.T) {
	assessmentID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality-assessments/"+assessmentID.String()+"/report", nil)
	req.SetPathValue("assessmentId", assessmentID.String())
	response := httptest.NewRecorder()

	(&Handler{}).getReport(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "WORKSPACE_REQUIRED") {
		t.Fatalf("response = %s, want workspace-required error", response.Body.String())
	}
}

func TestReportRejectsUnboundedFindingLimit(t *testing.T) {
	assessmentID := uuid.New()
	workspaceID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/quality-assessments/"+assessmentID.String()+"/report?workspaceId="+workspaceID.String()+"&limit=101", nil)
	req.SetPathValue("assessmentId", assessmentID.String())
	response := httptest.NewRecorder()

	(&Handler{}).getReport(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "INVALID_LIMIT") {
		t.Fatalf("response = %s, want invalid-limit error", response.Body.String())
	}
}

func TestReportFindingMapsSkippedToNotApplicableWithoutChangingObservedFact(t *testing.T) {
	finding := domain.Finding{
		ID:        uuid.New(),
		ResultID:  uuid.New(),
		RuleID:    "optional-field",
		Dimension: "ACCURACY",
		Severity:  "WARNING",
		Status:    domain.FindingSkipped,
		Observed:  map[string]any{"affectedCount": 0, "sample": []any{}},
	}

	response := reportFindingResponse(finding)
	if response["status"] != string(domain.DimensionNotApplicable) {
		t.Fatalf("report status = %v, want NOT_APPLICABLE", response["status"])
	}
	if finding.Status != domain.FindingSkipped {
		t.Fatalf("historical finding status changed to %s", finding.Status)
	}
	if _, err := json.Marshal(response); err != nil {
		t.Fatalf("report finding is not JSON encodable: %v", err)
	}
}


func TestAssessmentHistoryRequiresWorkspaceID(t *testing.T) {
	versionID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dataset-versions/"+versionID.String()+"/quality-assessments", nil)
	req.SetPathValue("versionId", versionID.String())
	response := httptest.NewRecorder()

	(&Handler{}).listAssessments(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "WORKSPACE_REQUIRED") {
		t.Fatalf("body = %s, want WORKSPACE_REQUIRED", response.Body.String())
	}
}

func TestLatestAssessmentRequiresWorkspaceID(t *testing.T) {
	versionID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dataset-versions/"+versionID.String()+"/quality-assessments/latest", nil)
	req.SetPathValue("versionId", versionID.String())
	response := httptest.NewRecorder()

	(&Handler{}).latestAssessment(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "WORKSPACE_REQUIRED") {
		t.Fatalf("body = %s, want WORKSPACE_REQUIRED", response.Body.String())
	}
}
