package hop

import (
	"context"
	"encoding/json"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	w "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	whop "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/hop"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestMetadataUsesTheProbedEndpoint(t *testing.T) {
	for _, scenario := range []string{"valid", "hostname", "port", "custom-url", "alternate-mode", "unknown-field"} {
		t.Run(scenario, func(t *testing.T) {
			pg := map[string]any{"pluginId": "POSTGRESQL", "accessType": 0, "hostname": "127.0.0.1", "port": "15432", "databaseName": "source", "username": "reader", "password": "${SOURCE_DB_PASSWORD}", "attributes": map[string]string{"SUPPORTS_BOOLEAN_DATA_TYPE": "Y", "SUPPORTS_TIMESTAMP_DATA_TYPE": "Y"}}
			switch scenario {
			case "hostname":
				pg["hostname"] = "other-instance"
			case "port":
				pg["port"] = "25432"
			case "custom-url":
				pg["attributes"].(map[string]string)["CUSTOM_URL"] = "jdbc:postgresql://other-instance/source"
			case "alternate-mode":
				pg["accessType"] = 2
			case "unknown-field":
				pg["customUrl"] = "jdbc:postgresql://other-instance/source"
			}
			raw, _ := json.Marshal(map[string]any{"name": "dpp-source", "rdbms": map[string]any{"POSTGRESQL": pg}})
			c := Config{ConnectionRef: "source", InstanceRef: "operator-ref", Database: "source", Reader: "reader", SourceDSN: "postgres://reader:test-value@127.0.0.1:15432/source?sslmode=disable", Metadata: map[string][]json.RawMessage{"rdbms": {raw}}, Tables: []Table{{Code: "a"}, {Code: "b"}, {Code: "c"}}}
			ch, th, sh := c.Identity()
			a := Adapter{Load: func(context.Context) (Config, error) { return c, nil }}
			_, err := a.config(context.Background(), d.Request{ConnectionRef: c.ConnectionRef, InstanceRef: c.InstanceRef, ConfigHash: ch, TemplateHash: th, SchemaHash: sh})
			if (err == nil) != (scenario == "valid") {
				t.Fatalf("endpoint contract scenario %s: %v", scenario, err)
			}
		})
	}
}

func TestRequestDispatchObservation(t *testing.T) {
	writes := atomic.Int64{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	client, err := whop.NewClient(server.URL, "unit-reader", "unit-only", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	call, written := observeRequest(ctx)
	_, _ = client.StartSubmission(call, w.ManagedSubmitRequest{Name: "unit-pipeline"}, "unit-id")
	if written.Load() || writes.Load() != 0 {
		t.Fatal("cancelled pre-dispatch was charged as written")
	}
	call, written = observeRequest(context.Background())
	_, _ = client.StartSubmission(call, w.ManagedSubmitRequest{Name: "unit-pipeline"}, "unit-id")
	if !written.Load() || writes.Load() != 1 {
		t.Fatal("real failed HTTP call was not observed")
	}
}
