// Package pilot composes #84 with the existing controlled-pilot principal resolver.
// Production identity authentication remains behind principal.Resolver.
package pilot

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/reference"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func resolver(get func(string) string) (principal.Resolver, error) {
	enabled := false
	if raw := get("EXTERNAL_INTEGRATION_API_ENABLED"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid external integration enabled setting")
		}
		enabled = v
	}
	var workspaces []string
	for _, v := range strings.Split(get("EXTERNAL_INTEGRATION_API_WORKSPACE_IDS"), ",") {
		if s := strings.TrimSpace(v); s != "" {
			workspaces = append(workspaces, s)
		}
	}
	return principal.NewStaticResolver(enabled, get("EXTERNAL_INTEGRATION_API_TOKEN"), get("EXTERNAL_INTEGRATION_API_SUBJECT"), get("EXTERNAL_INTEGRATION_API_ACTOR_ID"), workspaces, []string{domain.Capability})
}

// Authority reloads controlled-pilot configuration at every dispatch/projection.
// The recorded actor cannot authenticate itself; only current server configuration
// can authorize it. No bearer credential is stored in an attempt.
func Authority(get func(string) string) app.Authority {
	return func(ctx context.Context, ws, actor uuid.UUID, subject string) (principal.RequestPrincipal, error) {
		r, err := resolver(get)
		if err != nil {
			return principal.RequestPrincipal{}, domain.ErrDenied
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://pilot.invalid/authority", nil)
		req.Header.Set("Authorization", "Bearer "+get("EXTERNAL_INTEGRATION_API_TOKEN"))
		p, err := r.Resolve(req, ws, domain.Capability)
		if err == nil && (p.ActorID != actor || p.Subject != subject) {
			err = domain.ErrDenied
		}
		return p, err
	}
}
func BuildExecutor(pool *pgxpool.Pool) (*app.Executor, error) {
	if _, err := resolver(os.Getenv); err != nil {
		return nil, err
	}
	entries := []ports.Adapter{}
	if raw := os.Getenv("EXTERNAL_INTEGRATION_REFERENCE_URL"); raw != "" {
		c, err := reference.New(raw)
		if err != nil {
			return nil, err
		}
		entries = append(entries, c.Adapter("reference", os.Getenv("EXTERNAL_INTEGRATION_REFERENCE_INSTANCE")))
	}
	registry, err := ports.NewRegistry(entries...)
	if err != nil {
		return nil, err
	}
	core := app.New(transaction.NewManager(pool), infra.New(pool))
	return app.NewExecutor(core, registry, Authority(os.Getenv)), nil
}
func Build(pool *pgxpool.Pool) (*transport.Handler, error) {
	r, err := resolver(os.Getenv)
	if err != nil {
		return nil, err
	}
	e, err := BuildExecutor(pool)
	if err != nil {
		return nil, err
	}
	return transport.New(e.Core, r).WithExecutor(e).WithUsage(app.NewUsageImporter(e)), nil
}
