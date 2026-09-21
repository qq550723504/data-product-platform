package certificationhttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
