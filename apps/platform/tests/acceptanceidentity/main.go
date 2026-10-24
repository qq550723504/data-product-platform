// Independent 1A host. No business Commands, bootstrap, model or dispatch.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	contracthttp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	if os.Getenv("ACCEPTANCE_IDENTITY_ACK") != "ISOLATED_READONLY_1A" {
		log.Fatal("explicit isolated 1A configuration required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("ACCEPTANCE_IDENTITY_DSN"))
	must(err)
	if !strings.HasPrefix(cfg.ConnConfig.Database, "acceptance_1a_") {
		log.Fatal("independent 1A database required")
	}
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	db, err := pgxpool.NewWithConfig(context.Background(), cfg)
	must(err)
	defer db.Close()
	must(db.Ping(context.Background()))
	ca, err := os.ReadFile(os.Getenv("ACCEPTANCE_IDENTITY_CA"))
	must(err)
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		log.Fatal("valid process-scoped CA required")
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	issuer := os.Getenv("ACCEPTANCE_IDENTITY_ISSUER")
	store := bindings.NewStore(db)
	resolver, err := oidc.NewResolver(oidc.Config{Issuer: issuer, TokenEndpoint: issuer + "/protocol/openid-connect/token", IntrospectionEndpoint: issuer + "/protocol/openid-connect/token/introspect", ClientID: os.Getenv("ACCEPTANCE_IDENTITY_RESOURCE_CLIENT"), ClientSecret: os.Getenv("ACCEPTANCE_IDENTITY_RESOURCE_SECRET"), Audience: "dpp-acceptance-api", RequiredScope: "business-acceptance", AllowedClientIDs: []string{"dpp-acceptance-rp"}}, store, client)
	must(err)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/identity/business-acceptance-principal", store.AcceptancePrincipal(resolver))
	reader := contractinfra.AcceptanceIdentityReader{Repository: contractinfra.NewPostgresRepository(db), Authority: store}
	mux.HandleFunc("GET /api/v1/workspaces/{workspaceId}/acceptance-targets", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		ws, e := uuid.Parse(r.PathValue("workspaceId"))
		if e != nil || ws == uuid.Nil || r.URL.RawQuery != "" {
			http.Error(w, "invalid explicit workspace", 400)
			return
		}
		p, e := resolver.Resolve(r, ws, principal.CapabilityAcceptanceRead)
		if e != nil {
			status := http.StatusForbidden
			if errors.Is(e, principal.ErrAuthorityUnavailable) {
				status = http.StatusServiceUnavailable
			}
			writeError(w, status)
			return
		}
		targets, e := reader.Targets(r.Context(), p)
		if e != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(e, principal.ErrWorkspaceDenied) {
				status = http.StatusForbidden
			}
			writeError(w, status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(targets)
	})
	(contracthttp.ReadOnlyHandler{Reader: contractinfra.AcceptanceIdentityReader{Repository: contractinfra.NewPostgresRepository(db), Authority: store}, Principal: resolver, ReadCapability: principal.CapabilityAcceptanceRead}).Register(mux)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"profile": "business-acceptance.v1", "writeEnabled": false, "dispatchEnabled": false})
	})
	server := http.Server{Addr: "127.0.0.1:18498", Handler: queryOnly(mux), TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	log.Print("Independent 1A TLS query host; business commands and dispatch absent")
	log.Fatal(server.ListenAndServeTLS(os.Getenv("ACCEPTANCE_IDENTITY_CERT"), os.Getenv("ACCEPTANCE_IDENTITY_KEY")))
}
func queryOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		if r.Method != http.MethodGet && !(r.Method == http.MethodPost && r.URL.Path == "/internal/identity/business-acceptance-principal") {
			writeError(w, http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "identity_or_workspace_unavailable"})
}
func must(err error) {
	if err != nil {
		log.Fatal("isolated 1A configuration or dependency unavailable")
	}
}
