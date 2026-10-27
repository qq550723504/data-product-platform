package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
	managementservice "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement/service"
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
			// Exercise that same native lookahead through the shared closed mux,
			// rather than assuming the adapter-only test proves composition.
			h, e := managementservice.Compose(managementservice.Dependencies{Principal: lookaheadIdentity{uuid.New()}, Gate: lookaheadGate{}, IdentitySource: "SYNTHETIC_FIXTURE", RuntimeSources: s, Ready: func(context.Context) error { return nil }, Confirmation: func(http.ResponseWriter, *http.Request) {}})
			if e != nil {
				t.Fatal(e)
			}
			values := url.Values{"start": {at.Add(-time.Minute).Format(management.LogTimeLayout)}, "end": {at.Add(time.Minute).Format(management.LogTimeLayout)}}
			req := httptest.NewRequest("GET", "/api/v1/workspaces/"+wsA.String()+"/management/runtime-logs/entries?"+values.Encode(), nil)
			req.Header.Set("Authorization", "Bearer synthetic")
			out := httptest.NewRecorder()
			h.ServeHTTP(out, req)
			want := 200
			if change != "valid" {
				want = 503
			}
			var body map[string]any
			if out.Code != want || json.Unmarshal(out.Body.Bytes(), &body) != nil || want != 200 && body["data"] != nil {
				t.Fatal("shared mux accepted invalid native lookahead")
			}
		})
	}
}

type lookaheadIdentity struct{ actor uuid.UUID }

func (r lookaheadIdentity) Resolve(_ *http.Request, ws uuid.UUID, capability string) (principal.RequestPrincipal, error) {
	if ws != wsA || capability != principal.CapabilityManagementRead {
		return principal.RequestPrincipal{}, principal.ErrWorkspaceDenied
	}
	return principal.RequestPrincipal{ActorID: r.actor, WorkspaceID: ws, IdentityRevision: "1:1:1"}, nil
}

type lookaheadGate struct{}

func (lookaheadGate) Confirm(_ context.Context, p principal.RequestPrincipal) (principal.Authority, error) {
	return principal.Authority{Principal: p, EvaluatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (g lookaheadGate) ConfirmRuntime(ctx context.Context, p principal.RequestPrincipal, _ bool) (principal.Authority, error) {
	return g.Confirm(ctx, p)
}
