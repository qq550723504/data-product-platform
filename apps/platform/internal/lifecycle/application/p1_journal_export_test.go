package application

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// Test-only bridge lets the external composition test reuse the real existing
// fixture while importing the storage adapter without an application import cycle.
func P1JournalFixtureForTest(t *testing.T) (*pgxpool.Pool, *OccupancyService, domain.Copy) {
	t.Helper()
	pool := planDatabase(t)
	_, copies, service, authority, _, _, _ := occupancyFixture(t, pool)
	authority.caller.Capabilities[CapabilityInspectStorage] = struct{}{}
	authority.caller.Capabilities[CapabilityReconcileDisposition] = struct{}{}
	return pool, service, copies[0]
}
