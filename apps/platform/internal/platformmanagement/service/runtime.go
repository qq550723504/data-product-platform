package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

func roots(file string) (*x509.CertPool, error) {
	b, e := readPrivate(file, 65536)
	if e != nil {
		return nil, ErrConfiguration
	}
	r := x509.NewCertPool()
	if !r.AppendCertsFromPEM(b) {
		return nil, ErrConfiguration
	}
	return r, nil
}

func poolConfig(d Database) (*pgxpool.Config, error) {
	b, e := readPrivate(d.URLFile, 16384)
	if e != nil {
		return nil, ErrConfiguration
	}
	u, e := url.Parse(strings.TrimSpace(string(b)))
	if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" || u.Path != "/"+d.Name || u.User.Username() != d.Role {
		return nil, ErrConfiguration
	}
	password, present := u.User.Password()
	port, e := strconv.Atoi(u.Port())
	if !present || password == "" || e != nil || port < 1 || port > 65535 || strconv.Itoa(port) != u.Port() {
		return nil, ErrConfiguration
	}
	r, e := roots(d.CAFile)
	if e != nil {
		return nil, e
	}
	// Explicit URL fields, TLS and runtime parameters replace ambient PG defaults.
	c, e := pgxpool.ParseConfig(u.String())
	if e != nil {
		return nil, ErrConfiguration
	}
	c.ConnConfig.Host, c.ConnConfig.Port, c.ConnConfig.Database = u.Hostname(), uint16(port), d.Name
	c.ConnConfig.User, c.ConnConfig.Password = d.Role, password
	c.ConnConfig.Fallbacks = nil
	c.ConnConfig.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: r, ServerName: u.Hostname()}
	c.ConnConfig.RuntimeParams = map[string]string{"default_transaction_read_only": "on", "search_path": "pg_catalog,public", "application_name": "management-query-only"}
	c.ConnConfig.ConnectTimeout = 5 * time.Second
	c.MaxConns, c.MinConns = 5, 0
	return c, nil
}

// CheckDatabase examines only schema/privilege metadata, not business records.
// Default read-only alone is insufficient: this role must have no write grants,
// ownership, role escalation, public-schema executable functions or extra reads.
func CheckDatabase(ctx context.Context, pool *pgxpool.Pool, database, role string, audit bool) error {
	var ok bool
	err := pool.QueryRow(ctx, `SELECT current_database()=$1 AND current_user=$2
 AND NOT pg_is_in_recovery() AND current_setting('default_transaction_read_only')='on'
 AND EXISTS(SELECT 1 FROM pg_stat_ssl WHERE pid=pg_backend_pid() AND ssl)
 AND NOT r.rolsuper AND NOT r.rolcreatedb AND NOT r.rolcreaterole AND NOT r.rolreplication AND NOT r.rolbypassrls
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=r.oid)
 AND NOT EXISTS(SELECT 1 FROM pg_database WHERE datname=current_database() AND datdba=r.oid)
 AND NOT has_database_privilege(current_database(),'CREATE,TEMP')
 AND NOT EXISTS(SELECT 1 FROM pg_namespace n WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND (n.nspowner=r.oid OR has_schema_privilege(n.oid,'CREATE')))
 AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND has_function_privilege(p.oid,'EXECUTE'))
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND CASE WHEN c.relkind='S' THEN has_sequence_privilege(c.oid,'USAGE,SELECT,UPDATE') ELSE false END)
 FROM pg_roles r WHERE r.rolname=current_user`, database, role).Scan(&ok)
	if err != nil || !ok {
		return ErrConfiguration
	}
	allowed := []string{"platform_actor_state", "platform_subject_binding", "platform_workspace_grant"}
	if audit {
		allowed = append(allowed, "audit_event")
	}
	err = pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE n.nspname='public' AND c.relname=ANY($1::text[]))=cardinality($1::text[])
 AND bool_and(c.relowner<>(SELECT oid FROM pg_roles WHERE rolname=current_user)
 AND NOT has_table_privilege(c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
 AND NOT has_any_column_privilege(c.oid,'INSERT,UPDATE,REFERENCES')
 AND has_any_column_privilege(c.oid,'SELECT')=(n.nspname='public' AND c.relname=ANY($1::text[]))
 AND has_table_privilege(c.oid,'SELECT')=(n.nspname='public' AND c.relname=ANY($1::text[])))
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname !~ '^pg_' AND n.nspname<>'information_schema' AND c.relkind IN ('r','p','v','m','f')`, allowed).Scan(&ok)
	if err != nil || !ok {
		return ErrConfiguration
	}
	columns := []string{"platform_actor_state.actor_id", "platform_actor_state.enabled", "platform_actor_state.revision", "platform_subject_binding.id", "platform_subject_binding.issuer", "platform_subject_binding.subject", "platform_subject_binding.actor_id", "platform_subject_binding.enabled", "platform_subject_binding.revision", "platform_subject_binding.valid_from", "platform_subject_binding.valid_until", "platform_workspace_grant.id", "platform_workspace_grant.actor_id", "platform_workspace_grant.workspace_id", "platform_workspace_grant.enabled", "platform_workspace_grant.revision", "platform_workspace_grant.capabilities", "platform_workspace_grant.valid_from", "platform_workspace_grant.valid_until"}
	if audit {
		for _, column := range []string{"id", "workspace_id", "actor_id", "actor_type", "action", "object_type", "object_id", "occurred_at"} {
			columns = append(columns, "audit_event."+column)
		}
	}
	err = pool.QueryRow(ctx, `SELECT bool_and(EXISTS(SELECT 1 FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname=split_part(ref,'.',1) AND a.attname=split_part(ref,'.',2) AND a.attnum>0 AND NOT a.attisdropped)) FROM unnest($1::text[]) AS required(ref)`, columns).Scan(&ok)
	if err != nil || !ok {
		return ErrConfiguration
	}
	return nil
}

// RunProfile returns unselected without loading files or creating dependencies.
// An invalid selected profile always returns selected=true and a fixed safe error.
func RunProfile(ctx context.Context, file string) (bool, error) {
	if strings.TrimSpace(file) == "" {
		return false, nil
	}
	p, err := LoadProfile(file)
	if err != nil {
		return true, ErrConfiguration
	}
	cert, e := readPrivate(p.TLS.CertFile, 65536)
	if e != nil {
		return true, e
	}
	key, e := readPrivate(p.TLS.KeyFile, 65536)
	if e != nil {
		return true, e
	}
	pair, e := tls.X509KeyPair(cert, key)
	if e != nil {
		return true, ErrConfiguration
	}
	secret, e := readPrivate(p.Identity.ClientSecretFile, 16384)
	if e != nil || strings.TrimSpace(string(secret)) == "" {
		return true, ErrConfiguration
	}
	ca, e := roots(p.Identity.CAFile)
	if e != nil {
		return true, e
	}
	directories, closeDirectories, e := directoryBindings(p.Directory)
	if e != nil {
		return true, ErrConfiguration
	}
	defer closeDirectories()
	c, e := poolConfig(p.Database)
	if e != nil {
		return true, e
	}
	start, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, e := pgxpool.NewWithConfig(start, c)
	if e != nil {
		return true, ErrConfiguration
	}
	defer pool.Close()
	check := func(ctx context.Context) error {
		return CheckDatabase(ctx, pool, p.Database.Name, p.Database.Role, p.Audit != nil)
	}
	if check(start) != nil {
		return true, ErrConfiguration
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: ca, MinVersion: tls.VersionTLS12}}, Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	store := bindings.NewStore(pool)
	resolver, e := oidc.NewResolver(oidc.Config{Issuer: p.Identity.Issuer, TokenEndpoint: p.Identity.TokenEndpoint, IntrospectionEndpoint: p.Identity.IntrospectionEndpoint, ClientID: p.Identity.ClientID, ClientSecret: strings.TrimSpace(string(secret)), Audience: p.Identity.Audience, RequiredScope: p.Identity.RequiredScope, AllowedClientIDs: p.Identity.AllowedClientIDs}, store, client)
	if e != nil {
		return true, ErrConfiguration
	}
	audits := map[uuid.UUID]management.AuditBinding{}
	if p.Audit != nil {
		for _, ws := range p.Audit.Workspaces {
			audits[ws] = management.AuditBinding{Source: p.Audit.Source, Reader: management.PostgresAudit{Pool: pool}}
		}
	}
	h, e := Compose(Dependencies{Principal: resolver, Gate: management.PostgresGate{Pool: pool, Store: store}, Confirmation: store.ManagementPrincipal(resolver), Ready: check, IdentitySource: p.Identity.Source, Audits: audits, Bindings: directories})
	if e != nil {
		return true, ErrConfiguration
	}
	server := &http.Server{Addr: p.Listen, Handler: h, ReadHeaderTimeout: 5 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}}
	listener, e := net.Listen("tcp", p.Listen)
	if e != nil {
		return true, ErrConfiguration
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	select {
	case e := <-done:
		if !errors.Is(e, http.ErrServerClosed) {
			return true, ErrConfiguration
		}
	case <-ctx.Done():
		stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if server.Shutdown(stop) != nil {
			_ = server.Close()
			return true, ErrConfiguration
		}
	}
	return true, nil
}
