package metadatahttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	metadataapp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

func TestWriteBindingErrorMapsProviderFailuresWithoutLeakingCause(t *testing.T) {
	tests := []struct {
		name       string
		kind       metadataengine.ErrorKind
		statusCode int
		wantStatus int
		wantCode   string
	}{
		{name: "invalid request", kind: metadataengine.ErrorInvalidRequest, statusCode: http.StatusBadRequest, wantStatus: http.StatusBadRequest, wantCode: "METADATA_BINDING_INVALID"},
		{name: "not found", kind: metadataengine.ErrorNotFound, statusCode: http.StatusNotFound, wantStatus: http.StatusNotFound, wantCode: "METADATA_ASSET_NOT_FOUND"},
		{name: "unauthorized", kind: metadataengine.ErrorUnauthorized, statusCode: http.StatusForbidden, wantStatus: http.StatusBadGateway, wantCode: "METADATA_PROVIDER_AUTH_FAILED"},
		{name: "unavailable", kind: metadataengine.ErrorUnavailable, statusCode: http.StatusServiceUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "METADATA_PROVIDER_UNAVAILABLE"},
		{name: "invalid response", kind: metadataengine.ErrorInvalidResponse, statusCode: http.StatusOK, wantStatus: http.StatusBadGateway, wantCode: "METADATA_PROVIDER_INVALID_RESPONSE"},
		{name: "rejected", kind: metadataengine.ErrorRejected, statusCode: http.StatusConflict, wantStatus: http.StatusBadGateway, wantCode: "METADATA_PROVIDER_REJECTED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/data-resources/test/metadata-bindings", nil)
			writeBindingError(recorder, request, metadataengine.NewExternalError(tt.kind, "request", tt.statusCode, errors.New("OpenMetadata secret provider detail")))
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			var envelope httpserver.ErrorEnvelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Code != tt.wantCode {
				t.Fatalf("code = %q, want %q", envelope.Code, tt.wantCode)
			}
			body := recorder.Body.String()
			if strings.Contains(body, "OpenMetadata") || strings.Contains(body, "secret provider detail") {
				t.Fatalf("response leaked provider-specific detail: %s", body)
			}
		})
	}
}

func TestWriteBindingErrorMapsApplicationValidation(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/data-resources/test/metadata-bindings", nil)
	writeBindingError(recorder, request, metadataapp.ErrInvalidBindingRequest)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestPublicProjectionErrorNeverExposesStoredProviderDetail(t *testing.T) {
	if got := publicProjectionError("OpenMetadata response included a secret"); got != "metadata projection failed" {
		t.Fatalf("public projection error = %q", got)
	}
	if got := publicProjectionError("   "); got != "" {
		t.Fatalf("empty projection error = %q", got)
	}
}
