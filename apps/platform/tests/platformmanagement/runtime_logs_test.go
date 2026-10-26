package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func TestSyntheticRuntimeQueryValidatesLookaheadBeforeTruncation(t *testing.T) {
	for _, change := range []string{"valid", "duplicate", "unordered", "service", "level", "event", "time", "foreign"} {
		t.Run(change, func(t *testing.T) {
			at := time.Now().UTC().Add(-time.Minute)
			items := make([]nativeLog, 26)
			for i := range items {
				items[i] = nativeLog{Workspace: wsA, ID: fmt.Sprintf("A-%03d", 30-i), At: at, Service: "APP_SERVICE", Level: "INFO", Event: "READ_COMPLETED"}
			}
			switch change {
			case "duplicate":
				items[25] = items[24]
			case "unordered":
				items[25].ID = "A-999"
			case "service":
				items[25].Service = "PRIVATE_SERVICE"
			case "level":
				items[25].Level = "PRIVATE_LEVEL"
			case "event":
				items[25].Event = "PRIVATE_EVENT"
			case "time":
				items[25].At = at.Add(time.Hour)
			case "foreign":
				items[25].Workspace = wsB
			}
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(struct {
					Items    []nativeLog `json:"items"`
					Complete bool        `json:"complete"`
				}{items, true})
			}))
			defer srv.Close()
			s := &syntheticLogSource{client: srv.Client(), url: srv.URL, credential: "synthetic-unit-only"}
			page, err := s.Query(context.Background(), wsA, management.RuntimeQuery{Start: at.Add(-time.Minute), End: at.Add(time.Minute), AsOf: time.Now().UTC()})
			if change != "valid" {
				if !errors.Is(err, management.LogUnavailable) || len(page.Items) != 0 || page.Next != nil {
					t.Fatal("invalid lookahead was accepted as proof of an earlier page")
				}
			} else if err != nil || len(page.Items) != 25 || page.Next == nil || page.Next.ID != items[24].ID {
				t.Fatal("valid bounded lookahead did not preserve the page boundary")
			}
		})
	}
}
