package openmetadata_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/openmetadata"
)

func TestClientGetTableAndUpsertDataProduct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want Bearer test-token", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tables/name/sample_data.ecommerce.public.orders":
			writeJSON(t, w, map[string]any{
				"id":                 "table-001",
				"name":               "orders",
				"fullyQualifiedName": "sample_data.ecommerce.public.orders",
				"displayName":        "Orders",
				"description":        "Orders table",
				"serviceType":        "PostgreSQL",
				"version":            1.2,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/v1/dataProducts":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if body["name"] != "DP-ENTERPRISE-ACTIVITY" || body["domain"] != "Park" || body["displayName"] != "企业经营活跃度" {
				t.Fatalf("unexpected data product request: %+v", body)
			}
			writeJSON(t, w, map[string]any{
				"id":                 "product-001",
				"name":               "DP-ENTERPRISE-ACTIVITY",
				"fullyQualifiedName": "Park.DP-ENTERPRISE-ACTIVITY",
				"version":            0.1,
			})
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := openmetadata.NewClient(server.URL, "test-token", server.Client())
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	asset, err := client.GetAsset(context.Background(), "TABLE", "sample_data.ecommerce.public.orders")
	if err != nil {
		t.Fatalf("get table: %v", err)
	}
	if asset.ID != "table-001" || asset.FullyQualifiedName != "sample_data.ecommerce.public.orders" || asset.Metadata["serviceType"] != "PostgreSQL" {
		t.Fatalf("asset = %+v", asset)
	}

	entity, err := client.UpsertDataProduct(context.Background(), metadataengine.GovernanceProduct{
		Name:        "DP-ENTERPRISE-ACTIVITY",
		DisplayName: "企业经营活跃度",
		Description: "Enterprise activity reference product",
		Domain:      "Park",
	})
	if err != nil {
		t.Fatalf("upsert data product: %v", err)
	}
	if entity.ID != "product-001" || entity.FullyQualifiedName != "Park.DP-ENTERPRISE-ACTIVITY" {
		t.Fatalf("external entity = %+v", entity)
	}
}

func TestClientMapsOpenMetadataErrorsToProviderNeutralContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"not authorized secret detail"}`, http.StatusForbidden)
	}))
	defer server.Close()
	client, err := openmetadata.NewClient(server.URL, "", server.Client())
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	_, err = client.UpsertDataProduct(context.Background(), metadataengine.GovernanceProduct{Name: "x", Domain: "Park"})
	if err == nil {
		t.Fatal("expected adapter error")
	}
	var external *metadataengine.ExternalError
	if !errors.As(err, &external) {
		t.Fatalf("error = %T %v, want provider-neutral ExternalError", err, err)
	}
	if external.Kind != metadataengine.ErrorUnauthorized || external.StatusCode != http.StatusForbidden {
		t.Fatalf("external error = %+v, want UNAUTHORIZED/403", external)
	}
	if strings.Contains(err.Error(), "not authorized secret detail") || strings.Contains(err.Error(), "OpenMetadata") {
		t.Fatalf("public error string leaked provider detail: %q", err.Error())
	}
	if external.Cause == nil || !strings.Contains(external.Cause.Error(), "not authorized secret detail") {
		t.Fatalf("internal cause was not retained for diagnostics: %+v", external)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write response: %v", err)
	}
}
