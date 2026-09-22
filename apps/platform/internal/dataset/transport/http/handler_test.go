package datasethttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
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

func TestWriteVersionIncludesExecutionProducer(t *testing.T) {
	executionID := uuid.New()
	version := domain.DatasetVersion{
		ID:                     uuid.New(),
		DatasetID:              uuid.New(),
		VersionNo:              3,
		Status:                 domain.VersionReady,
		ChecksumAlgorithm:      "SHA256",
		ChecksumValue:          "abc123",
		GeneratedByExecutionID: &executionID,
	}
	response := httptest.NewRecorder()

	writeVersion(response, http.StatusOK, version)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload struct {
		GeneratedByExecutionID *uuid.UUID `json:"generatedByExecutionId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode DatasetVersion response: %v", err)
	}
	if payload.GeneratedByExecutionID == nil || *payload.GeneratedByExecutionID != executionID {
		t.Fatalf("generatedByExecutionId = %v, want %s", payload.GeneratedByExecutionID, executionID)
	}
}
