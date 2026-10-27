// Independent synthetic acceptance host. Never mounts business Commands or real sources.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
	managementservice "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement/service"
	directory "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement/zitadel"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type config struct {
	DatabaseURL, Issuer, ResourceSecret, ReadCredential, Cert, Key, CA, Root, APIPassword, RPPassword string
	RuntimeLogs, MainService, MainServiceCompat, DirectoryProduct, UserProduct                        bool
}

var wsA = uuid.MustParse("11111111-1111-4111-8111-111111111111")
var wsB = uuid.MustParse("12121212-1212-4212-8212-121212121212")
var people = map[string]uuid.UUID{"admin-a": uuid.MustParse("83000000-0000-4000-8000-000000000001"), "admin-b": uuid.MustParse("83000000-0000-4000-8000-000000000002"), "both": uuid.MustParse("83000000-0000-4000-8000-000000000003"), "ordinary": uuid.MustParse("83000000-0000-4000-8000-000000000004")}

func main() {
	if os.Getenv("MANAGEMENT_FIXTURE_ACK") != "ISOLATED_READONLY_MANAGEMENT" {
		log.Fatal("explicit isolated management fixture required")
	}
	var c config
	b, e := os.ReadFile(os.Getenv("MANAGEMENT_FIXTURE_CONFIG"))
	must(e)
	must(json.Unmarshal(b, &c))
	issuer, dbName, apiPort := "https://localhost:19130", "management_readonly_20261025", "18499"
	if c.RuntimeLogs {
		issuer, dbName, apiPort = "https://localhost:19150", "runtime_logs_readonly_20261026", "18550"
	}
	if c.MainService {
		if c.RuntimeLogs || c.MainServiceCompat || c.DirectoryProduct || c.UserProduct || os.Getenv("MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK") != "ISOLATED_MAIN_SERVICE_READONLY" {
			log.Fatal("disjoint main service fixture required")
		}
		issuer, dbName, apiPort = "https://localhost:19160", "management_main_service_20261026", "18560"
	}
	if c.MainServiceCompat {
		if c.RuntimeLogs || c.MainService || c.DirectoryProduct || c.UserProduct || os.Getenv("MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK") != "ISOLATED_MAIN_SERVICE_COMPAT_READONLY" {
			log.Fatal("disjoint main service compatibility fixture required")
		}
		issuer, dbName, apiPort = "https://localhost:19170", "management_main_service_compat_20261026", "18570"
	}
	if c.DirectoryProduct {
		if c.RuntimeLogs || c.MainService || c.MainServiceCompat || c.UserProduct || os.Getenv("MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK") != "ISOLATED_DIRECTORY_PRODUCT_READONLY" {
			log.Fatal("disjoint directory product fixture required")
		}
		issuer, dbName, apiPort = "https://localhost:19180", "management_directory_product_20261026", "18580"
	}
	if c.UserProduct {
		if c.RuntimeLogs || c.MainService || c.MainServiceCompat || c.DirectoryProduct || os.Getenv("MANAGEMENT_USER_PRODUCT_FIXTURE_ACK") != "ISOLATED_USER_PRODUCT_READONLY" {
			log.Fatal("disjoint user product fixture required")
		}
		issuer, dbName, apiPort = "https://localhost:19220", "management_user_product_20261027", "18620"
	}
	if c.Issuer != issuer {
		log.Fatal("synthetic issuer required")
	}
	cfg, e := pgxpool.ParseConfig(c.DatabaseURL)
	must(e)
	if cfg.ConnConfig.Database != dbName {
		log.Fatal("independent database required")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	if len(os.Args) == 2 && os.Args[1] == "prepare" {
		prepare(c)
		return
	}
	if c.DirectoryProduct || c.UserProduct {
		log.Fatal("directory product runtime must use cmd/api with private profile")
	}
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	must(e)
	defer pool.Close()
	must(pool.Ping(context.Background()))
	ca, e := os.ReadFile(c.CA)
	must(e)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		log.Fatal("invalid process CA")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}, Timeout: 10 * time.Second}
	store := bindings.NewStore(pool)
	resolver, e := oidc.NewResolver(oidc.Config{Issuer: c.Issuer, TokenEndpoint: c.Issuer + "/token", IntrospectionEndpoint: c.Issuer + "/introspect", ClientID: "dpp-management-api", ClientSecret: c.ResourceSecret, Audience: "dpp-management-api", RequiredScope: "platform-management", AllowedClientIDs: []string{"dpp-management-rp"}}, store, client)
	must(e)
	scopes := map[uuid.UUID]management.Binding{}
	if !c.MainService {
		catalog, e := management.NewCatalog([]management.Role{{ID: "management_viewer", Name: "管理查询员", Permissions: []string{"management.members.read", "management.roles.read", "management.audit.read", "management.users.read"}}, {ID: "audit_viewer", Name: "审计观察员", Permissions: []string{"management.audit.read"}}})
		must(e)
		native, e := directory.NewClient(c.Issuer, c.ReadCredential, "governance", client)
		must(e)
		links := management.BoundUserIdentities{Store: store, Workspace: wsA, References: map[string]management.UserSubjectReference{"person-a-2": {Issuer: c.Issuer, Subject: "directory-person-linked", BasisRef: "synthetic-user-link-v1"}}}
		scopes = map[uuid.UUID]management.Binding{wsA: {OrganizationID: "organization-a", ProjectID: "governance", Source: "SYNTHETIC_FIXTURE", Directory: native, Users: native, IdentityLinks: links, Catalog: catalog, Organizations: native}, wsB: {OrganizationID: "organization-b", ProjectID: "governance", Source: "SYNTHETIC_FIXTURE", Directory: native, Users: native, Catalog: catalog, Organizations: native}}

	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/identity/platform-management-principal", store.ManagementPrincipal(resolver))
	logs, e := management.NewRuntimeLogHandler(resolver, management.PostgresGate{Pool: pool, Store: store}, runtimeSources(c, client), "SYNTHETIC_FIXTURE")
	must(e)
	logs.Register(mux)
	management.Handler{Principal: resolver, Gate: management.PostgresGate{Pool: pool, Store: store}, Bindings: scopes, Audits: map[uuid.UUID]management.AuditBinding{wsA: {Source: "SYNTHETIC_FIXTURE", Reader: management.PostgresAudit{Pool: pool}}, wsB: {Source: "SYNTHETIC_FIXTURE", Reader: management.PostgresAudit{Pool: pool}}}, IdentitySource: "SYNTHETIC_FIXTURE"}.Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"profile": "platform-management.v1", "realSources": "UNCONFIGURED", "businessWrites": false, "models": false, "dispatch": false})
	})
	next := onlyQueries(mux)
	if c.MainService || c.MainServiceCompat {
		must(managementservice.CheckDatabase(context.Background(), pool, dbName, "management_api", true))
		next, e = managementservice.Compose(managementservice.Dependencies{Principal: resolver, Gate: management.PostgresGate{Pool: pool, Store: store}, IdentitySource: "SYNTHETIC_FIXTURE", Confirmation: store.ManagementPrincipal(resolver), Ready: func(ctx context.Context) error {
			return managementservice.CheckDatabase(ctx, pool, dbName, "management_api", true)
		}, Bindings: scopes, RuntimeSources: runtimeSources(c, client), Audits: map[uuid.UUID]management.AuditBinding{wsA: {Source: "SYNTHETIC_FIXTURE", Reader: management.PostgresAudit{Pool: pool}}, wsB: {Source: "SYNTHETIC_FIXTURE", Reader: management.PostgresAudit{Pool: pool}}}})
		must(e)
	}
	server := http.Server{Addr: "127.0.0.1:" + apiPort, Handler: next, ReadHeaderTimeout: 5 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	log.Print("Independent management query host ready; real sources and business writes disabled")
	must(server.ListenAndServeTLS(c.Cert, c.Key))
}
func onlyQueries(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method != "GET" && !(r.Method == "POST" && r.URL.Path == "/internal/identity/platform-management-principal") {
			w.WriteHeader(405)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func prepare(c config) {
	ctx := context.Background()
	raw := os.Getenv("MANAGEMENT_FIXTURE_ADMIN_DSN")
	cfg, e := pgxpool.ParseConfig(raw)
	must(e)
	expectedDB, expectedPort := "management_readonly_20261025", uint16(25496)
	if c.RuntimeLogs {
		expectedDB, expectedPort = "runtime_logs_readonly_20261026", 25550
	}
	if c.MainService {
		if c.RuntimeLogs || c.MainServiceCompat || c.DirectoryProduct || c.UserProduct || os.Getenv("MANAGEMENT_MAIN_SERVICE_FIXTURE_ACK") != "ISOLATED_MAIN_SERVICE_READONLY" {
			log.Fatal("main service prepare boundary")
		}
		expectedDB, expectedPort = "management_main_service_20261026", 25560
	}
	if c.MainServiceCompat {
		if c.RuntimeLogs || c.MainService || c.DirectoryProduct || c.UserProduct || os.Getenv("MANAGEMENT_MAIN_SERVICE_COMPAT_FIXTURE_ACK") != "ISOLATED_MAIN_SERVICE_COMPAT_READONLY" {
			log.Fatal("compatibility prepare boundary")
		}
		expectedDB, expectedPort = "management_main_service_compat_20261026", 25570
	}
	if c.DirectoryProduct {
		if c.RuntimeLogs || c.MainService || c.MainServiceCompat || c.UserProduct || os.Getenv("MANAGEMENT_DIRECTORY_PRODUCT_FIXTURE_ACK") != "ISOLATED_DIRECTORY_PRODUCT_READONLY" {
			log.Fatal("directory product prepare boundary")
		}
		expectedDB, expectedPort = "management_directory_product_20261026", 25580
	}
	if c.UserProduct {
		if c.RuntimeLogs || c.MainService || c.MainServiceCompat || c.DirectoryProduct || os.Getenv("MANAGEMENT_USER_PRODUCT_FIXTURE_ACK") != "ISOLATED_USER_PRODUCT_READONLY" {
			log.Fatal("user product prepare boundary")
		}
		expectedDB, expectedPort = "management_user_product_20261027", 25620
	}
	if cfg.ConnConfig.Database != expectedDB || cfg.ConnConfig.Host != "127.0.0.1" || cfg.ConnConfig.Port != expectedPort {
		log.Fatal("prepare target differs")
	}
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	must(e)
	defer pool.Close()
	var exists *string
	must(pool.QueryRow(ctx, "SELECT to_regclass('public.schema_migration')::text").Scan(&exists))
	if exists != nil {
		log.Fatal("prepared fixture retained; refusing overwrite")
	}
	must(migration.NewRunner(pool, filepath.Join(c.Root, "migrations")).Up(ctx))
	for name, id := range people {
		_, e = pool.Exec(ctx, `INSERT INTO platform_actor_state(actor_id)VALUES($1);`, id)
		must(e)
		_, e = pool.Exec(ctx, `INSERT INTO platform_subject_binding(id,issuer,subject,actor_id,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '8 hours')`, uuid.New(), c.Issuer, name, id)
		must(e)
		targets := []uuid.UUID{wsA}
		if name == "admin-b" {
			targets = []uuid.UUID{wsB}
		}
		if name == "both" {
			targets = append(targets, wsB)
			if c.DirectoryProduct || c.UserProduct {
				targets = append(targets, uuid.MustParse("13131313-1313-4313-8313-131313131313"))
			}
		}
		cap := principal.CapabilityManagementRead
		if name == "ordinary" {
			cap = principal.CapabilityAcceptanceRead
		}
		for _, ws := range targets {
			_, e = pool.Exec(ctx, `INSERT INTO platform_workspace_grant(id,actor_id,workspace_id,capabilities,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '8 hours')`, uuid.New(), id, ws, fixtureCapabilities(c, name, cap))
			must(e)
		}
	}
	// A directory reference is deliberately different from both the pairwise
	// OIDC subject and canonical actor. This selected user has no management grant.
	selected := uuid.MustParse("84000000-0000-4000-8000-000000000002")
	_, e = pool.Exec(ctx, `INSERT INTO platform_actor_state(actor_id,enabled)VALUES($1,true)`, selected)
	must(e)
	_, e = pool.Exec(ctx, `INSERT INTO platform_subject_binding(id,issuer,subject,actor_id,valid_from,valid_until)VALUES($1,$2,'directory-person-linked',$3,now()-interval '1 hour',now()+interval '8 hours')`, uuid.New(), c.Issuer, selected)
	must(e)
	_, e = pool.Exec(ctx, `INSERT INTO platform_workspace_grant(id,actor_id,workspace_id,capabilities,valid_from,valid_until)VALUES($1,$2,$3,$4,now()-interval '1 hour',now()+interval '8 hours')`, uuid.New(), selected, wsA, []string{principal.CapabilityAcceptanceRead})
	must(e)
	auditAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	for _, ws := range []uuid.UUID{wsA, wsB} {
		for i := 0; i < 58; i++ {
			_, e = pool.Exec(ctx, `INSERT INTO audit_event(id,workspace_id,actor_type,actor_id,action,object_type,object_id,metadata,occurred_at)VALUES($1,$2,'USER',$3,'SYNTHETIC_REVIEW','Dataset',$4,'{"private":"NEVER_RENDER_RAW_PAYLOAD"}',CASE WHEN $6::boolean THEN $7::timestamptz ELSE now()-($5::int*interval '1 minute') END)`, uuid.New(), ws, people["both"], uuid.New(), i+1, c.MainService, auditAt)
			must(e)
		}
	}
	// Each login/session store has separate minimal runtime authority.
	for role, password := range map[string]string{"management_api": c.APIPassword, "management_rp": c.RPPassword} {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{20,100}$`).MatchString(password) {
			log.Fatal("invalid generated password")
		}
		_, e = pool.Exec(ctx, fmt.Sprintf("CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '%s'", role, password))
		must(e)
	}
	_, e = pool.Exec(ctx, strings.ReplaceAll(`REVOKE ALL ON DATABASE management_readonly_20261025 FROM PUBLIC; REVOKE CREATE ON SCHEMA public FROM PUBLIC; REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA public FROM PUBLIC;
 GRANT CONNECT ON DATABASE management_readonly_20261025 TO management_api,management_rp; GRANT USAGE ON SCHEMA public TO management_api,management_rp;
 GRANT SELECT ON platform_actor_state,platform_subject_binding,platform_workspace_grant,audit_event TO management_api;
 ALTER ROLE management_api SET default_transaction_read_only=on;
 GRANT SELECT,INSERT,DELETE ON platform_browser_transaction,platform_browser_session TO management_rp;`, "management_readonly_20261025", cfg.ConnConfig.Database))
	must(e)
	log.Print("Independent management fixture prepared; API SELECT-only, RP session-only, no original database accessed")
}
func must(e error) {
	if e != nil {
		log.Fatal("management fixture configuration or operation failed; protected diagnostic retained")
	}
}
