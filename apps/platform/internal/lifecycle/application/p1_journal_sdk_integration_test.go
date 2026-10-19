package application_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

func TestP1JournalRoutingBeforeSDKPrecheckAndOriginalFailure(t *testing.T) {
	pool, service, copy := app.P1JournalFixtureForTest(t)
	ctx := context.Background()
	t.Cleanup(func() { outbox.ConfigureAppendObligation(nil) })
	var wire atomic.Int64
	var failObservation atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.URL.Query().Get("versionId") != copy.ProviderVersion {
			t.Error("unexpected SDK request")
			w.WriteHeader(403)
			return
		}
		wire.Add(1)
		if failObservation.Load() {
			outbox.ConfigureAppendObligation(nil)
		}
		w.Header().Set("X-Amz-Version-Id", copy.ProviderVersion)
		w.Header().Set("Content-Length", fmt.Sprint(copy.ByteSize))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", `"sdk-loopback"`)
		w.WriteHeader(200)
	}))
	defer server.Close()
	inspector, err := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{
		Endpoint: strings.TrimPrefix(server.URL, "http://"), AccessKey: "test-access", SecretKey: "test-secret",
		WorkspaceID: copy.WorkspaceID, ProviderInstanceID: copy.ProviderInstanceID, IncarnationID: copy.IncarnationID,
		Namespaces: []string{copy.Namespace, "p1-other-namespace"}, Journal: app.NewReconciliationStorageCallJournal(service),
	})
	if err != nil {
		t.Fatal(err)
	}
	count := func(table string) int {
		var n int
		if e := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	baseCalls, baseCost := count("lifecycle_storage_call"), count("cost_event")
	outbox.ConfigureAppendObligation(nil)
	_, err = inspector.InspectExactCopy(ctx, copy)
	if !errors.Is(err, app.ErrStorageUnknown) || !strings.Contains(err.Error(), "routing obligation source is not configured") || wire.Load() != 0 || count("lifecycle_storage_call") != baseCalls || count("cost_event") != baseCost {
		t.Fatal("unconfigured bootstrap lost original start error or reached wire", err, wire.Load())
	}
	err = pilot.WithRecoveryRouting(func(_ *outbox.Router) error {
		if _, e := inspector.InspectExactCopy(ctx, copy); e != nil {
			t.Fatal("actual P1 bootstrap precheck", e)
		}
		if wire.Load() != 1 || count("lifecycle_storage_call") != baseCalls+1 || count("lifecycle_storage_observation") != 1 || count("cost_event") != baseCost+1 {
			t.Fatal("configured physical call/observation/cost missing")
		}
		failObservation.Store(true)
		_, e := inspector.InspectExactCopy(ctx, copy)
		return e
	})
	if !errors.Is(err, app.ErrStorageUnknown) || !strings.Contains(err.Error(), "storage journal observation") || !strings.Contains(err.Error(), "routing obligation source is not configured") || wire.Load() != 2 || count("lifecycle_storage_call") != baseCalls+2 || count("lifecycle_storage_observation") != 1 {
		t.Fatal("real call followed by observation failure lost original cause/uncertainty", err, wire.Load())
	}
}
