package pilot

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

// WithRecoveryRouting installs the existing event obligations before any
// journaled fixture inspection. The callback includes prechecks, not only the
// later RequestRecovery/dispatcher phase. It grants no storage authority.
func WithRecoveryRouting(run func(*outbox.Router) error) error {
	router, err := routing.NewRouter(false)
	if err != nil {
		return err
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	return run(router)
}
