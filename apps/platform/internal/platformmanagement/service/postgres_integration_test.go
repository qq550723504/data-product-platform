package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func TestIsolatedQueryRoleMetadataAndTLSAreValidated(t *testing.T) {
	first := os.Getenv("MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK") == "ISOLATED_MAIN_SERVICE_READONLY"
	compat := os.Getenv("MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK") == "ISOLATED_MAIN_SERVICE_COMPAT_READONLY"
	if !first && !compat {
		t.Skip("new isolated main service fixture required")
	}
	if first && compat {
		t.Fatal("disjoint isolated fixture required")
	}
	expectedPort, expectedDatabase, expectedIssuer := "25560", "management_main_service_20261026", "https://localhost:19160"
	if compat {
		expectedPort, expectedDatabase, expectedIssuer = "25570", "management_main_service_compat_20261026", "https://localhost:19170"
	}
	var fixture struct {
		DatabaseURL, CA, Issuer, ResourceSecret string
		MainService, MainServiceCompat          bool
	}
	b, e := os.ReadFile(os.Getenv("MANAGEMENT_FIXTURE_CONFIG"))
	if e != nil || json.Unmarshal(b, &fixture) != nil || fixture.MainService != first || fixture.MainServiceCompat != compat {
		t.Fatal("independent fixture config unavailable")
	}
	u, e := url.Parse(fixture.DatabaseURL)
	if e != nil || u.Hostname() != "127.0.0.1" || u.Port() != expectedPort || u.Path != "/"+expectedDatabase || u.User == nil || u.User.Username() != "management_api" {
		t.Fatal("wrong fixture target")
	}
	u.RawQuery = ""
	file := filepath.Join(t.TempDir(), "private-query-url")
	if os.WriteFile(file, []byte(u.String()), 0600) != nil {
		t.Fatal("private fixture input")
	}
	d := Database{URLFile: file, CAFile: fixture.CA, Name: expectedDatabase, Role: "management_api"}
	c, e := poolConfig(d)
	if e != nil {
		t.Fatal("verified pool configuration failed")
	}
	ctx := context.Background()
	pool, e := pgxpool.NewWithConfig(ctx, c)
	if e != nil {
		t.Fatal("verified pool unavailable")
	}
	defer pool.Close()
	if CheckDatabase(ctx, pool, d.Name, d.Role, true) != nil {
		t.Fatal("minimal query role rejected")
	}
	if CheckDatabase(ctx, pool, d.Name, d.Role, false) == nil {
		t.Fatal("extra audit SELECT permission accepted for identity-only composition")
	}
	if CheckDatabase(ctx, pool, "foreign", d.Role, true) == nil || CheckDatabase(ctx, pool, d.Name, "postgres", true) == nil {
		t.Fatal("database or role mismatch accepted")
	}
	var profile struct{ AdminDSN, Control string }
	b, e = os.ReadFile(filepath.Join(filepath.Dir(os.Getenv("MANAGEMENT_FIXTURE_CONFIG")), "profile.json"))
	if e != nil || json.Unmarshal(b, &profile) != nil {
		t.Fatal("isolated role controller unavailable")
	}
	a, e := url.Parse(profile.AdminDSN)
	if e != nil || a.Hostname() != "127.0.0.1" || a.Port() != expectedPort || a.Path != "/"+expectedDatabase || a.User == nil || a.User.Username() != "postgres" {
		t.Fatal("wrong role controller target")
	}
	admin, e := pgxpool.New(ctx, profile.AdminDSN)
	if e != nil {
		t.Fatal("isolated role controller unavailable")
	}
	defer admin.Close()
	if _, e = admin.Exec(ctx, "REVOKE SELECT ON audit_event FROM management_api"); e != nil {
		t.Fatal("isolated role transition failed")
	}
	defer func() {
		if _, e := admin.Exec(ctx, "GRANT SELECT ON audit_event TO management_api"); e != nil {
			t.Error("isolated role restore failed")
		}
	}()
	if CheckDatabase(ctx, pool, d.Name, d.Role, false) != nil || CheckDatabase(ctx, pool, d.Name, d.Role, true) == nil {
		t.Fatal("identity-only permissions or required audit permission not enforced")
	}
	// All optional readers absent: actual OIDC and current PG authority still
	// explain configuration, without granting audit access or fabricating records.
	if fixture.Issuer != expectedIssuer {
		t.Fatal("wrong synthetic issuer")
	}
	ca, e := os.ReadFile(fixture.CA)
	if e != nil {
		t.Fatal("fixture CA unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("fixture CA invalid")
	}
	idp := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}, Timeout: 5 * time.Second}
	request, e := http.NewRequestWithContext(ctx, "POST", fixture.Issuer+"/_control", bytes.NewBufferString(`{"issueSubject":"admin-a"}`))
	if e != nil {
		t.Fatal("fixture request")
	}
	request.Header.Set("x-fixture-control", profile.Control)
	response, e := idp.Do(request)
	if e != nil {
		t.Fatal("fixture identity controller unavailable")
	}
	var token struct{ TestAccessToken string }
	decodeErr := json.NewDecoder(response.Body).Decode(&token)
	response.Body.Close()
	if response.StatusCode != 200 || decodeErr != nil || token.TestAccessToken == "" {
		t.Fatal("synthetic test identity unavailable")
	}
	store := bindings.NewStore(pool)
	resolver, e := oidc.NewResolver(oidc.Config{Issuer: fixture.Issuer, TokenEndpoint: fixture.Issuer + "/token", IntrospectionEndpoint: fixture.Issuer + "/introspect", ClientID: "dpp-management-api", ClientSecret: fixture.ResourceSecret, Audience: "dpp-management-api", RequiredScope: "platform-management", AllowedClientIDs: []string{"dpp-management-rp"}}, store, idp)
	if e != nil {
		t.Fatal("synthetic resolver configuration")
	}
	h, e := Compose(Dependencies{Principal: resolver, Gate: management.PostgresGate{Pool: pool, Store: store}, IdentitySource: "SYNTHETIC_FIXTURE", Confirmation: store.ManagementPrincipal(resolver), Ready: func(ctx context.Context) error { return CheckDatabase(ctx, pool, d.Name, d.Role, false) }})
	if e != nil {
		t.Fatal("identity-only composition")
	}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	for _, resource := range []string{"workspace", "configuration", "audit", "members", "runtime-logs/source"} {
		req, _ := http.NewRequest("GET", server.URL+"/api/v1/workspaces/11111111-1111-4111-8111-111111111111/management/"+resource, nil)
		req.Header.Set("Authorization", "Bearer "+token.TestAccessToken)
		r, e := server.Client().Do(req)
		if e != nil {
			t.Fatal("identity-only query unavailable")
		}
		var body map[string]any
		decodeErr := json.NewDecoder(r.Body).Decode(&body)
		r.Body.Close()
		if decodeErr != nil {
			t.Fatal("query response invalid")
		}
		want := 200
		if resource == "audit" || resource == "members" {
			want = 503
		}
		if r.StatusCode != want {
			t.Fatal("identity-only query status")
		}
		if want == 503 {
			if body["data"] != nil || body["code"] != "SOURCE_UNCONFIGURED" {
				t.Fatal("unconfigured source fabricated records")
			}
		} else if resource != "runtime-logs/source" {
			queries := body["data"].(map[string]any)["queries"].(map[string]any)
			if queries["audit"] != false || queries["members"] != false {
				t.Fatal("unconfigured source overclaimed")
			}
		}
	}
	if _, e = admin.Exec(ctx, "GRANT SELECT ON dataset TO management_api"); e != nil {
		t.Fatal("isolated excess privilege case failed")
	}
	defer func() {
		if _, e := admin.Exec(ctx, "REVOKE SELECT ON dataset FROM management_api"); e != nil {
			t.Error("isolated excess privilege restore failed")
		}
	}()
	if CheckDatabase(ctx, pool, d.Name, d.Role, false) == nil {
		t.Fatal("unexpected business table SELECT accepted")
	}
	if _, e = admin.Exec(ctx, "REVOKE SELECT ON dataset FROM management_api"); e != nil {
		t.Fatal("isolated table privilege restore failed")
	}
	for _, test := range []struct{ grant, revoke string }{
		{"GRANT SELECT(id) ON dataset TO management_api", "REVOKE SELECT(id) ON dataset FROM management_api"},
		{"GRANT UPDATE(enabled) ON platform_workspace_grant TO management_api", "REVOKE UPDATE(enabled) ON platform_workspace_grant FROM management_api"},
	} {
		if _, e = admin.Exec(ctx, test.grant); e != nil {
			t.Fatal("isolated column privilege case failed")
		}
		blocked := CheckDatabase(ctx, pool, d.Name, d.Role, false) != nil
		if _, e = admin.Exec(ctx, test.revoke); e != nil {
			t.Fatal("isolated column privilege restore failed")
		}
		if !blocked {
			t.Fatal("column-only privilege bypassed minimum role validation")
		}
	}
}
