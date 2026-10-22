package metadatahttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	metadataapp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
)

func TestWorkspaceResourceLookupFailureIsUnavailable(t *testing.T) {
	// pgxpool connects lazily; closing it produces a real repository error without a network call.
	pool, err := pgxpool.New(context.Background(), "postgres://postgres:private-password@127.0.0.1:1/private-db")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	h := NewHandler(nil, metadatainfra.NewPostgresRepository(pool))
	actor, workspace, resource := uuid.New(), uuid.New(), uuid.New()
	h.Authorize = func(*http.Request, uuid.UUID) (*uuid.UUID, error) { return &actor, nil }
	mux := http.NewServeMux()
	h.Register(mux)
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPost, "metadata-bindings"},
		{http.MethodGet, "metadata-bindings"},
		{http.MethodGet, "metadata-observation"},
	} {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			req := httptest.NewRequest(endpoint.method, "/api/v1/data-resources/"+resource.String()+"/"+endpoint.path+"?workspaceId="+workspace.String(), strings.NewReader(`{}`))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			var envelope httpserver.ErrorEnvelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != http.StatusServiceUnavailable || envelope.Code != "METADATA_RESOURCE_LOOKUP_UNAVAILABLE" {
				t.Fatalf("status/code = %d/%s, want 503/METADATA_RESOURCE_LOOKUP_UNAVAILABLE", recorder.Code, envelope.Code)
			}
			for _, private := range []string{"private-password", "private-db", "pool closed"} {
				if strings.Contains(recorder.Body.String(), private) {
					t.Fatalf("response exposed repository detail: %s", recorder.Body.String())
				}
			}
		})
	}
}

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

func TestResourceMetadataRequiresWorkspaceAuthorityBeforeStorageOrProvider(t *testing.T) {
	workspace, resource := uuid.New(), uuid.New()
	for _, endpoint := range []struct{ method, path string }{
		{http.MethodPost, "metadata-bindings"},
		{http.MethodGet, "metadata-bindings"},
		{http.MethodGet, "metadata-observation"},
	} {
		for _, denied := range []bool{false, true} {
			h := NewHandler(nil, nil)
			want := http.StatusUnauthorized
			if denied {
				want = http.StatusForbidden
				h.Authorize = func(*http.Request, uuid.UUID) (*uuid.UUID, error) { return nil, errors.New("denied") }
			}
			mux := http.NewServeMux()
			h.Register(mux)
			req := httptest.NewRequest(endpoint.method, "/api/v1/data-resources/"+resource.String()+"/"+endpoint.path+"?workspaceId="+workspace.String(), strings.NewReader(`{}`))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)
			if recorder.Code != want {
				t.Fatalf("%s %s denied=%v status=%d, want %d", endpoint.method, endpoint.path, denied, recorder.Code, want)
			}
		}
	}
}
