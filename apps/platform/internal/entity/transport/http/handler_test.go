package entityhttp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

// Mappings are only unique inside a workspace, so the read endpoints must reject
// a missing or malformed workspaceId instead of falling back to a global lookup.
func TestGetMappingBySourceRequiresWorkspace(t *testing.T) {
	handler := &Handler{}
	for name, target := range map[string]string{
		"missing": "/api/v1/entity-mappings?sourceType=CSV&sourceRef=companies.csv&sourceKey=ENT-1",
		"invalid": "/api/v1/entity-mappings?workspaceId=not-a-uuid&sourceType=CSV&sourceRef=companies.csv&sourceKey=ENT-1",
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.getMappingBySource(recorder, httptest.NewRequest(http.MethodGet, target, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
			var envelope httpserver.ErrorEnvelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Code != "WORKSPACE_REQUIRED" {
				t.Fatalf("error code = %q, want WORKSPACE_REQUIRED", envelope.Code)
			}
		})
	}
}

func TestListEntityMappingsRequiresWorkspace(t *testing.T) {
	handler := &Handler{}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/entities/00000000-0000-0000-0000-000000000001/mappings", nil)
	request.SetPathValue("entityId", "00000000-0000-0000-0000-000000000001")
	recorder := httptest.NewRecorder()
	handler.listEntityMappings(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var envelope httpserver.ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Code != "WORKSPACE_REQUIRED" {
		t.Fatalf("error code = %q, want WORKSPACE_REQUIRED", envelope.Code)
	}
}
