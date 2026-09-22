package datasethttp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestGetVersionRequiresWorkspaceID(t *testing.T) {
	versionID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dataset-versions/"+versionID.String(), nil)
	req.SetPathValue("versionId", versionID.String())
	response := httptest.NewRecorder()

	(&Handler{}).getVersion(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "INVALID_WORKSPACE_ID") {
		t.Fatalf("body = %s, want INVALID_WORKSPACE_ID", response.Body.String())
	}
}
