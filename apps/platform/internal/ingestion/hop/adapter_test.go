package hop

import (
	"bytes"
	"context"
	"encoding/json"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	w "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	whop "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/hop"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type unwrittenConn struct{ net.Conn }

func (c unwrittenConn) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestBufferedHeadersAreNotConfirmedDispatch(t *testing.T) {
	clientSide, peer := net.Pipe()
	defer peer.Close()
	transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) { return unwrittenConn{clientSide}, nil }}
	defer transport.CloseIdleConnections()
	client, err := whop.NewClient("http://controlled.invalid", "unit-reader", "unit-only", NewHTTPClient(10*time.Second, transport))
	if err != nil {
		t.Fatal(err)
	}
	recorded := atomic.Int64{}
	ctx, observed := observeRequest(context.Background(), func() { recorded.Add(1) })
	_, err = client.StartSubmission(ctx, w.ManagedSubmitRequest{Name: "unit-pipeline"}, "unit-id")
	if err == nil {
		t.Fatal("controlled network write failure did not occur")
	}
	if observed.State() != d.DispatchUnconfirmed {
		t.Fatal("ambiguous write was treated as safe to redispatch")
	}
	if recorded.Load() != 0 {
		t.Fatal("buffered headers invented an ACTUAL dispatch before any network write")
	}
}

func TestDispatchIsConfirmedBeforeProviderReply(t *testing.T) {
	for _, kind := range []string{"start", "large-register"} {
		t.Run(kind, func(t *testing.T) {
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				<-release
				w.WriteHeader(503)
			}))
			defer server.Close()
			defer close(release)
			client, err := whop.NewClient(server.URL, "unit-reader", "unit-only", NewHTTPClient(5*time.Second, server.Client().Transport.(*http.Transport)))
			if err != nil {
				t.Fatal(err)
			}
			recorded := make(chan struct{})
			ctx, observed := observeRequest(context.Background(), func() { close(recorded) })
			go func() {
				if kind == "start" {
					_, _ = client.StartSubmission(ctx, w.ManagedSubmitRequest{Name: "unit-pipeline"}, "unit-id")
				} else {
					_, _ = client.PrepareSubmission(ctx, w.ManagedSubmitRequest{Name: "unit-pipeline", Definition: bytes.Repeat([]byte("x"), 16385), ContentType: "application/xml"})
				}
			}()
			select {
			case <-recorded:
				if observed.State() != d.Dispatched {
					t.Fatal("successful full network write was not confirmed")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("dispatch observation waited for the provider reply")
			}
		})
	}
}

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
	client, err := whop.NewClient(server.URL, "unit-reader", "unit-only", NewHTTPClient(10*time.Second, server.Client().Transport.(*http.Transport)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorded := atomic.Int64{}
	call, written := observeRequest(ctx, func() { recorded.Add(1) })
	_, _ = client.StartSubmission(call, w.ManagedSubmitRequest{Name: "unit-pipeline"}, "unit-id")
	if written.State() != d.NotDispatched || writes.Load() != 0 || recorded.Load() != 0 {
		t.Fatal("cancelled pre-dispatch was charged as written")
	}
	call, written = observeRequest(context.Background(), func() { recorded.Add(1) })
	_, _ = client.StartSubmission(call, w.ManagedSubmitRequest{Name: "unit-pipeline"}, "unit-id")
	if written.State() != d.Dispatched || writes.Load() != 1 || recorded.Load() != 1 {
		t.Fatal("real failed HTTP call was not observed")
	}
}
