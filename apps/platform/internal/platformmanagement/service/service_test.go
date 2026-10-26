package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func validProfile(t *testing.T) Profile {
	t.Helper()
	f := func(s string) string { return filepath.Join(t.TempDir(), s) }
	return Profile{Version: "platform-management-query.v1", Mode: "QUERY_ONLY", Listen: "127.0.0.1:18560", TLS: ServerTLS{f("cert"), f("key")}, Identity: Identity{Source: "CONFIGURED", Issuer: "https://identity.invalid", TokenEndpoint: "https://identity.invalid/token", IntrospectionEndpoint: "https://identity.invalid/introspect", ClientID: "api", ClientSecretFile: f("secret"), Audience: "api", RequiredScope: "platform-management", AllowedClientIDs: []string{"rp"}, CAFile: f("ca")}, Database: Database{URLFile: f("url"), CAFile: f("ca"), Name: "isolated", Role: "query"}}
}
func TestClosedProfileRejectsAmbiguousAndSyntheticConfiguration(t *testing.T) {
	p := validProfile(t)
	b, _ := json.Marshal(p)
	for name, raw := range map[string][]byte{
		"valid":              b,
		"directory-unopened": []byte(strings.Replace(string(b), `"version":`, `"directory":{},"version":`, 1)),
		"logs-unopened":      []byte(strings.Replace(string(b), `"version":`, `"runtimeSources":{},"version":`, 1)),
		"fixture-switch":     []byte(strings.Replace(string(b), `"version":`, `"mainServiceCompat":true,"version":`, 1)),
		"unknown":            []byte(strings.Replace(string(b), `"version":`, `"unknown":true,"version":`, 1)),
		"duplicate":          []byte(strings.Replace(string(b), `"mode":`, `"mode":"QUERY_ONLY","mode":`, 1)),
		"nested-duplicate":   []byte(strings.Replace(string(b), `"source":`, `"source":"CONFIGURED","source":`, 1)),
		"case-alias":         []byte(strings.Replace(string(b), `"version":`, `"Version":`, 1)),
		"trailing":           append(append([]byte{}, b...), []byte(" {}")...),
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "profile.json")
			if os.WriteFile(file, raw, 0600) != nil {
				t.Fatal("fixture")
			}
			_, e := LoadProfile(file)
			if (e == nil) != (name == "valid") {
				t.Fatalf("profile boundary %s", name)
			}
		})
	}
	for _, bad := range []string{"mode", "synthetic-identity", "synthetic-audit", "empty-audit", "duplicate-workspace", "null-workspace", "public-listen", "relative-secret", "scope", "duplicate-client"} {
		t.Run(bad, func(t *testing.T) {
			p := validProfile(t)
			ws := uuid.New()
			switch bad {
			case "mode":
				p.Mode = "FULL"
			case "synthetic-identity":
				p.Identity.Source = "SYNTHETIC_FIXTURE"
			case "synthetic-audit":
				p.Audit = &Audit{Source: "SYNTHETIC_FIXTURE", Workspaces: []uuid.UUID{ws}}
			case "empty-audit":
				p.Audit = &Audit{Source: "CONFIGURED"}
			case "duplicate-workspace":
				p.Audit = &Audit{Source: "CONFIGURED", Workspaces: []uuid.UUID{ws, ws}}
			case "null-workspace":
				p.Audit = &Audit{Source: "CONFIGURED", Workspaces: []uuid.UUID{uuid.Nil}}
			case "public-listen":
				p.Listen = "0.0.0.0:18560"
			case "relative-secret":
				p.Identity.ClientSecretFile = "secret"
			case "scope":
				p.Identity.RequiredScope = "business-acceptance"
			case "duplicate-client":
				p.Identity.AllowedClientIDs = []string{"rp", "rp"}
			}
			if p.Validate() == nil {
				t.Fatal("invalid composition accepted")
			}
		})
	}
	if strings.Contains(fmt.Sprintf("%#v", p), "identity.invalid") {
		t.Fatal("private profile diagnostic")
	}
}
func TestDisabledAndInvalidSelectionNeverFallsBack(t *testing.T) {
	for _, path := range []string{"", "  "} {
		selected, e := RunProfile(context.Background(), path)
		if selected || e != nil {
			t.Fatal("off must not load any dependency")
		}
	}
	selected, e := RunProfile(context.Background(), filepath.Join(t.TempDir(), "PRIVATE_MISSING_PROFILE"))
	if !selected || e != ErrConfiguration || strings.Contains(e.Error(), "PRIVATE") {
		t.Fatal("selected profile fell back or leaked")
	}
}

type queryResolver struct{ ws, actor uuid.UUID }

func (r queryResolver) Resolve(_ *http.Request, ws uuid.UUID, capability string) (principal.RequestPrincipal, error) {
	if ws != r.ws || capability != principal.CapabilityManagementRead {
		return principal.RequestPrincipal{}, principal.ErrWorkspaceDenied
	}
	return principal.RequestPrincipal{ActorID: r.actor, WorkspaceID: ws, IdentityRevision: "1:1:1"}, nil
}

type queryGate struct{}

func (queryGate) Confirm(_ context.Context, p principal.RequestPrincipal) (principal.Authority, error) {
	return principal.Authority{Principal: p, EvaluatedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}, nil
}
func (g queryGate) ConfirmRuntime(ctx context.Context, p principal.RequestPrincipal, records bool) (principal.Authority, error) {
	if records {
		return principal.Authority{}, principal.ErrCapabilityDenied
	}
	return g.Confirm(ctx, p)
}

func TestCompositionExposesOnlyClosedQueryRoutes(t *testing.T) {
	ws := uuid.New()
	calls := 0
	d := Dependencies{Principal: queryResolver{ws, uuid.New()}, Gate: queryGate{}, IdentitySource: "SYNTHETIC_FIXTURE", Ready: func(context.Context) error { return nil }, Confirmation: func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(204) }}
	h, e := Compose(d)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/health/live", 200}, {"HEAD", "/health/live", 405}, {"GET", "/health/ready", 200},
		{"POST", "/internal/identity/platform-management-principal", 204}, {"GET", "/internal/identity/platform-management-principal", 405},
		{"GET", "/api/v1/workspaces/" + ws.String() + "/management/configuration", 200},
		{"GET", "/api/v1/workspaces/" + ws.String() + "/management/members", 503},
		{"GET", "/api/v1/workspaces/" + ws.String() + "/management/audit", 503},
		{"GET", "/api/v1/workspaces/" + ws.String() + "/management/runtime-logs/source", 200},
		{"GET", "/api/v1/workspaces/" + ws.String() + "/management/runtime-logs/entries?start=2026-10-20T00:00:00.000000000Z&end=2026-10-20T01:00:00.000000000Z", 403},
		{"POST", "/api/v1/workspaces/" + ws.String() + "/management/audit", 405},
		{"OPTIONS", "/api/v1/workspaces/" + ws.String() + "/management/audit", 405},
		{"POST", "/api/v1/workspaces/" + ws.String() + "/deliveries", 404}, {"GET", "/api/v1/workspaces/" + ws.String() + "/agent-tasks", 404},
		{"GET", "/api/v1/products/download", 404}, {"GET", "/api/v1/workspaces/" + ws.String() + "/management/audit/../members", 404},
		{"GET", "/internal/identity/business-acceptance-principal", 404},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer synthetic")
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		cache := "private, no-store"
		if tc.method == "GET" && strings.HasPrefix(tc.path, "/health/") {
			cache = "no-store"
		}
		if out.Code != tc.want || out.Header().Get("Cache-Control") != cache {
			t.Fatalf("%s %s = %d want %d", tc.method, tc.path, out.Code, tc.want)
		}
	}
	if calls != 1 {
		t.Fatal("unexpected confirmation call")
	}
	d.Audits = map[uuid.UUID]management.AuditBinding{ws: {Source: "CONFIGURED"}}
	if _, e = Compose(d); e == nil {
		t.Fatal("declared audit without reader accepted")
	}
}
