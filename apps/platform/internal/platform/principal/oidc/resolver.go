// Package oidc adapts RFC 7662 access-token introspection to the existing
// principal boundary. It issues no credentials and owns no authorization policy.
package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/client/rs"
	zitadel "github.com/zitadel/oidc/v3/pkg/oidc"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

var ErrProviderUnavailable = principal.ErrAuthorityUnavailable

// Identity is a provider-scoped subject, never an email address.
type Identity struct{ Issuer, Subject string }

// Binding is current server-owned human identity and workspace authority.
// Subject is the canonical platform subject, which can differ between clients
// using pairwise OIDC subjects. Historical actors must not be remapped.
type Binding struct {
	Active             bool
	Subject            string
	ActorID            uuid.UUID
	WorkspaceID        uuid.UUID
	Capabilities       map[string]struct{}
	AuthorityRevision  string
	AuthorityExpiresAt time.Time
}

// Bindings must read current authority on every call; token roles are not grants.
// Missing, disabled, ambiguous, or expired bindings must return no active binding.
type Bindings interface {
	Lookup(context.Context, Identity, uuid.UUID) (Binding, error)
}

type Config struct {
	Issuer, TokenEndpoint, IntrospectionEndpoint string
	ClientID, ClientSecret                       string
	Audience, RequiredScope                      string
	AllowedClientIDs                             []string
}

// Prevent ordinary diagnostics from printing resource-client credentials.
func (Config) String() string     { return "OIDC Config (credentials redacted)" }
func (c Config) GoString() string { return c.String() }

type Resolver struct {
	config   Config
	server   rs.ResourceServer
	bindings Bindings
	now      func() time.Time
}

// NewResolver uses explicitly configured HTTPS endpoints, without runtime
// discovery, redirects, a token cache, or a fallback to pilot credentials.
func NewResolver(c Config, bindings Bindings, client *http.Client) (*Resolver, error) {
	for _, raw := range []string{c.Issuer, c.TokenEndpoint, c.IntrospectionEndpoint} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, principal.ErrNotConfigured
		}
	}
	if bindings == nil || c.ClientID == "" || c.ClientSecret == "" || c.Audience == "" || c.RequiredScope == "" ||
		strings.ContainsAny(c.RequiredScope, " \t\r\n") || len(c.AllowedClientIDs) == 0 {
		return nil, principal.ErrNotConfigured
	}
	for _, id := range c.AllowedClientIDs {
		if strings.TrimSpace(id) == "" {
			return nil, principal.ErrNotConfigured
		}
	}
	c.AllowedClientIDs = append([]string(nil), c.AllowedClientIDs...)
	if client == nil {
		client = &http.Client{}
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	standard, ok := transport.(*http.Transport)
	if !ok || (standard.TLSClientConfig != nil && standard.TLSClientConfig.InsecureSkipVerify) {
		return nil, principal.ErrNotConfigured
	}
	secure := standard.Clone()
	secure.MaxResponseHeaderBytes = 16 << 10
	timeout := client.Timeout
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	bounded := &http.Client{Transport: boundedTransport{secure}, Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	server, err := rs.NewResourceServerClientCredentials(context.Background(), c.Issuer, c.ClientID, c.ClientSecret,
		rs.WithClient(bounded), rs.WithStaticEndpoints(c.TokenEndpoint, c.IntrospectionEndpoint))
	if err != nil {
		return nil, principal.ErrNotConfigured
	}
	return &Resolver{config: c, server: server, bindings: bindings, now: time.Now}, nil
}

// Resolve authenticates an access token, then asks the binding owner for current
// workspace authority. Body/header actor fields and provider roles are ignored.
func (r *Resolver) Resolve(req *http.Request, workspace uuid.UUID, capability string) (principal.RequestPrincipal, error) {
	deny := principal.RequestPrincipal{}
	if r == nil || r.server == nil || r.bindings == nil {
		return deny, principal.ErrNotConfigured
	}
	claims, err := r.authenticate(req)
	if err != nil {
		return deny, err
	}
	if workspace == uuid.Nil {
		return deny, principal.ErrWorkspaceDenied
	}
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return deny, principal.ErrCapabilityDenied
	}
	binding, err := r.bindings.Lookup(req.Context(), Identity{claims.Issuer, claims.Subject}, workspace)
	if err != nil {
		return deny, err
	}
	if !binding.Active || binding.ActorID == uuid.Nil || strings.TrimSpace(binding.Subject) == "" {
		return deny, principal.ErrUnauthenticated
	}
	if binding.WorkspaceID != workspace {
		return deny, principal.ErrWorkspaceDenied
	}
	if _, ok := binding.Capabilities[capability]; !ok {
		return deny, principal.ErrCapabilityDenied
	}
	caps := make(map[string]struct{}, len(binding.Capabilities))
	for key := range binding.Capabilities {
		caps[key] = struct{}{}
	}
	if binding.AuthorityExpiresAt.After(claims.Expiration.AsTime()) {
		binding.AuthorityExpiresAt = claims.Expiration.AsTime()
	}
	return principal.RequestPrincipal{IdentityIssuer: claims.Issuer, IdentitySubject: claims.Subject,
		IdentityRevision: binding.AuthorityRevision, IdentityExpiresAt: binding.AuthorityExpiresAt,
		Subject: binding.Subject, ActorID: binding.ActorID, WorkspaceID: workspace, Capabilities: caps}, nil
}

func contains[S ~[]string](values S, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// The SDK handles the protocol; this transport bounds untrusted responses and
// rejects redirects, non-JSON, oversized responses and trailing JSON documents.
type boundedTransport struct{ http.RoundTripper }

func (t boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	typ, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK || err != nil || typ != "application/json" {
		return nil, ErrProviderUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 || !json.Valid(body) {
		return nil, ErrProviderUnavailable
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil || raw == nil {
		return nil, ErrProviderUnavailable
	}
	// The SDK deliberately accepts some provider-specific time/scope formats.
	// Our shared contract requires integral NumericDate and a space-delimited scope.
	for _, key := range []string{"exp", "iat", "nbf"} {
		if value, exists := raw[key]; exists {
			var number int64
			if string(value) == "null" || json.Unmarshal(value, &number) != nil {
				return nil, ErrProviderUnavailable
			}
		}
	}
	if _, delegated := raw["act"]; delegated {
		return nil, ErrProviderUnavailable
	}
	if value, exists := raw["scope"]; exists {
		var scope string
		if json.Unmarshal(value, &scope) != nil {
			return nil, ErrProviderUnavailable
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

// authenticate is shared by human and service adapters; it never grants application permissions.
func (r *Resolver) authenticate(req *http.Request) (*zitadel.IntrospectionResponse, error) {
	if req == nil {
		return nil, principal.ErrUnauthenticated
	}
	headers := req.Header.Values("Authorization")
	if len(headers) != 1 {
		return nil, principal.ErrUnauthenticated
	}
	parts := strings.Fields(headers[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 8192 || strings.Contains(parts[1], ",") {
		return nil, principal.ErrUnauthenticated
	}
	claims, err := rs.Introspect[*zitadel.IntrospectionResponse](req.Context(), r.server, parts[1])
	if err != nil {
		return nil, ErrProviderUnavailable
	} // Never expose upstream errors containing credentials.
	now := r.now()
	if claims == nil || !claims.Active || claims.Issuer != r.config.Issuer || strings.TrimSpace(claims.Subject) == "" ||
		!strings.EqualFold(claims.TokenType, "Bearer") || !claims.Expiration.AsTime().After(now) ||
		claims.NotBefore.AsTime().After(now) || claims.IssuedAt.AsTime().After(now) || claims.Actor != nil ||
		!contains(claims.Audience, r.config.Audience) || !contains(r.config.AllowedClientIDs, claims.ClientID) ||
		!contains(claims.Scope, r.config.RequiredScope) {
		return nil, principal.ErrUnauthenticated
	}
	return claims, nil
}
