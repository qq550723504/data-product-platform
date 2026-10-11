// Package pilot composes #84 with the existing controlled-pilot principal resolver.
// Production identity authentication remains behind principal.Resolver.
package pilot

import (
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/infrastructure"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"os"
	"strconv"
	"strings"
)

func Build(pool *pgxpool.Pool) (*transport.Handler, error) {
	enabled := false
	if raw := os.Getenv("EXTERNAL_INTEGRATION_API_ENABLED"); raw != "" {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid external integration enabled setting")
		}
		enabled = v
	}
	var workspaces []string
	for _, v := range strings.Split(os.Getenv("EXTERNAL_INTEGRATION_API_WORKSPACE_IDS"), ",") {
		if s := strings.TrimSpace(v); s != "" {
			workspaces = append(workspaces, s)
		}
	}
	resolver, err := principal.NewStaticResolver(enabled, os.Getenv("EXTERNAL_INTEGRATION_API_TOKEN"), os.Getenv("EXTERNAL_INTEGRATION_API_SUBJECT"), os.Getenv("EXTERNAL_INTEGRATION_API_ACTOR_ID"), workspaces, []string{domain.Capability})
	if err != nil {
		return nil, err
	}
	return transport.New(app.New(transaction.NewManager(pool), infra.New(pool)), resolver), nil
}
