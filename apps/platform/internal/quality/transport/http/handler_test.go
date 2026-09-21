package qualityhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
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
