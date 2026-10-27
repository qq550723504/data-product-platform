// Package service composes existing query ports. It owns no business commands.
package service

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	management "github.com/qq550723504/data-product-platform/apps/platform/internal/platformmanagement"
)

var ErrConfiguration = errors.New("management query-only composition unavailable")

type QueryGate interface {
	management.Gate
	management.RuntimeGate
}

// The same composition is used by the product loader and test-only port injection.
// Synthetic sources are never accepted by the product profile loader.
type Dependencies struct {
	Principal      principal.Resolver
	Gate           QueryGate
	Confirmation   http.HandlerFunc
	Ready          httpserver.ReadinessCheck
	IdentitySource string
	Audits         map[uuid.UUID]management.AuditBinding
	Bindings       map[uuid.UUID]management.Binding
	RuntimeSources management.RuntimeSources
}

func Compose(d Dependencies) (http.Handler, error) {
	if d.Principal == nil || d.Gate == nil || d.Confirmation == nil || d.Ready == nil || (d.IdentitySource != "CONFIGURED" && d.IdentitySource != "SYNTHETIC_FIXTURE") {
		return nil, ErrConfiguration
	}
	audits := make(map[uuid.UUID]management.AuditBinding, len(d.Audits))
	for ws, b := range d.Audits {
		if ws == uuid.Nil || !b.Configured() {
			return nil, ErrConfiguration
		}
		audits[ws] = b
	}
	bindings := make(map[uuid.UUID]management.Binding, len(d.Bindings))
	for ws, b := range d.Bindings {
		if ws == uuid.Nil || b.Directory == nil || b.Catalog == nil || !management.IsIdentifier(b.OrganizationID) || !management.IsIdentifier(b.ProjectID) || (b.Source != "CONFIGURED" && b.Source != "SYNTHETIC_FIXTURE") {
			return nil, ErrConfiguration
		}
		bindings[ws] = b
	}
	logs, err := management.NewRuntimeLogHandler(d.Principal, d.Gate, d.RuntimeSources, d.IdentitySource)
	if err != nil {
		return nil, ErrConfiguration
	}
	h := management.Handler{Principal: d.Principal, Gate: d.Gate, Bindings: bindings, Audits: audits, IdentitySource: d.IdentitySource}
	mux := httpserver.NewMuxWithReadiness(d.Ready, h.Register, logs.Register, func(m *http.ServeMux) {
		m.HandleFunc("/internal/identity/platform-management-principal", d.Confirmation)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		method := allowedMethod(r.URL.Path)
		if method == "" || r.URL.EscapedPath() != r.URL.Path || path.Clean(r.URL.Path) != r.URL.Path {
			http.Error(w, "query route unavailable", http.StatusNotFound)
			return
		}
		if r.Method != method {
			http.Error(w, "query method unavailable", http.StatusMethodNotAllowed)
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func allowedMethod(p string) string {
	if p == "/internal/identity/platform-management-principal" {
		return "POST"
	}
	if p == "/health/live" || p == "/health/ready" {
		return "GET"
	}
	s := strings.Split(strings.TrimPrefix(p, "/"), "/")
	if len(s) < 6 || s[0] != "api" || s[1] != "v1" || s[2] != "workspaces" || s[3] == "" || s[4] != "management" {
		return ""
	}
	if len(s) == 6 {
		switch s[5] {
		case "workspace", "configuration", "organization", "members", "roles", "users", "audit":
			return "GET"
		}
	}
	if s[5] == "runtime-logs" && (len(s) == 7 && (s[6] == "source" || s[6] == "entries") || len(s) == 8 && s[6] == "entries" && s[7] != "") {
		return "GET"
	}
	return ""
}
