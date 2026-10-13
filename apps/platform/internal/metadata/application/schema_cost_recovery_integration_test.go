package application

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	me "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
)

func failSchemaCost(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ws uuid.UUID) func() {
	t.Helper()
	name := pgx.Identifier{"schema_cost_failure_" + strings.ReplaceAll(ws.String(), "-", "")}.Sanitize()
	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='%s' AND NEW.cost_type='SOURCE_SCHEMA_SCAN_INVOCATION' THEN RAISE EXCEPTION 'injected cost database outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON cost_event FOR EACH ROW EXECUTE FUNCTION %s();`, name, ws, name, name))
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	drop := func() {
		if removed {
			return
		}
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON cost_event; DROP FUNCTION %s();`, name, name)); err != nil {
			t.Fatal(err)
		}
		removed = true
	}
	t.Cleanup(drop)
	return drop
}

func schemaCostCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, request uuid.UUID) (int, int) {
	t.Helper()
	var costs, observations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, request).Scan(&costs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_schema_observation WHERE request_id=$1`, request).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	return costs, observations
}

func TestSourceSchemaCostFailureRemainsRecoverable(t *testing.T) {
	ctx, pool, req, observer, _, s := schemaRetryFixture(t)
	restore := failSchemaCost(t, ctx, pool, req.Plan.WorkspaceID)
	if err := s.Process(ctx, req.ID); err == nil {
		t.Fatal("cost outage accepted")
	}
	if costs, observations := schemaCostCounts(t, ctx, pool, req.ID); costs != 0 || observations != 0 || observer.scans != 1 || observer.sourceIO != 0 {
		t.Fatalf("cost-unconfirmed attempt closed: costs=%d observations=%d scans=%d io=%d", costs, observations, observer.scans, observer.sourceIO)
	}
	observer.invocationErr = me.ErrSourceSchemaInvocationUnknown
	if err := s.Process(ctx, req.ID); err == nil {
		t.Fatal("unknown receipt accepted")
	}
	if _, observations := schemaCostCounts(t, ctx, pool, req.ID); observations != 0 || observer.scans != 1 {
		t.Fatal("unknown receipt admitted another scan")
	}
	restore()
	observer.invocationErr = nil
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Process(ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if costs, observations := schemaCostCounts(t, ctx, pool, req.ID); costs != 2 || observations != 2 || observer.scans != 2 {
		t.Fatalf("recovery dropped/duplicated physical cost: costs=%d observations=%d scans=%d", costs, observations, observer.scans)
	}
}
