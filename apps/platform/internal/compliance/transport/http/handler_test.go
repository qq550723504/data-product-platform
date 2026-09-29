package compliancehttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRunRejectsMissingAssessmentAttemptID(t *testing.T) {
	versionID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dataset-versions/"+versionID.String()+"/compliance-checks", strings.NewReader(`{
		"workspaceId":"11111111-1111-4111-8111-111111111111"
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

func TestRunRejectsNilAssessmentAttemptID(t *testing.T) {
	versionID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dataset-versions/"+versionID.String()+"/compliance-checks", strings.NewReader(`{
		"workspaceId":"11111111-1111-4111-8111-111111111111",
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

func TestAssessmentAttemptStatusRequiresWorkspaceID(t *testing.T) {
	attemptID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance-assessment-attempts/"+attemptID.String(), nil)
	req.SetPathValue("attemptId", attemptID.String())
	response := httptest.NewRecorder()

	(&Handler{}).getAssessmentAttempt(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "WORKSPACE_REQUIRED") {
		t.Fatalf("response = %s, want workspace-required error", response.Body.String())
	}
}

func TestAssessmentAttemptStatusRejectsInvalidAttemptID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/compliance-assessment-attempts/not-a-uuid?workspaceId="+uuid.NewString(), nil)
	req.SetPathValue("attemptId", "not-a-uuid")
	response := httptest.NewRecorder()

	(&Handler{}).getAssessmentAttempt(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "INVALID_ASSESSMENT_ATTEMPT_ID") {
		t.Fatalf("response = %s, want invalid attempt id error", response.Body.String())
	}
}
