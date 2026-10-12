package application

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

// These are real PostgreSQL lock/start races with a controlled Observer and
// entitlement Port, not native discovery or full provenance acceptance.
type schemaRunEntitlements struct{ *schemaTestEntitlements }

func (g schemaRunEntitlements) CheckCurrentEntitlementTx(ctx context.Context, tx pgx.Tx, req rd.EntitlementRequest) (rd.EntitlementDecision, error) {
	decision, err := g.schemaTestEntitlements.CheckCurrentEntitlementTx(ctx, tx, req)
	if err != nil {
		return decision, err
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM data_authorization WHERE id=$1`, req.AuthorizationID).Scan(&status); err != nil {
		return decision, err
	}
	if status != "APPROVED" {
		decision.Decision = "BLOCKED"
	}
	return decision, nil
}

func schemaRunMutation(ctx context.Context, tx pgx.Tx, s *SchemaService, req d.SchemaRequest, kind string) error {
	if kind == "revocation" {
		if _, err := deliveryfence.Advance(ctx, tx, req.Plan.WorkspaceID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE data_authorization SET status='REVOKED' WHERE id=$1`, req.Plan.AuthorizationID)
		return err
	}
	_, err := s.Service.repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: req.Plan.ResourceID, Provider: s.Service.provider, EntityType: "TABLE", ExternalID: req.ExternalID, ExternalFQN: req.Plan.ExternalFQN, IsPrimary: false})
	return err
}

func TestSourceSchemaRunHandoffSerializesAuthorityMutations(t *testing.T) {
	for _, kind := range []string{"revocation", "binding demotion"} {
		t.Run("RUN first/"+kind, func(t *testing.T) {
			ctx, pool, req, observer, gate, s := schemaRetryFixture(t)
			if _, err := pool.Exec(ctx, `UPDATE data_authorization SET status='APPROVED' WHERE id=$1`, req.Plan.AuthorizationID); err != nil {
				t.Fatal(err)
			}
			s.Rights = schemaRunEntitlements{gate}
			entered, release, started, finish := make(chan struct{}), make(chan struct{}, 1), make(chan struct{}), make(chan struct{}, 1)
			defer close(finish)
			defer close(release)
			observer.onRun = func() {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			observer.onScan = func() {
				close(started)
				select {
				case <-finish:
				case <-ctx.Done():
				}
			}
			process := make(chan error, 1)
			go func() { process <- s.Process(ctx, req.ID) }()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("RUN handoff was not reached")
			}
			writer, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback(ctx)
			writerPID := writer.Conn().PgConn().PID()
			mutation := make(chan error, 1)
			go func() {
				if err := schemaRunMutation(ctx, writer, s, req, kind); err != nil {
					mutation <- err
					return
				}
				mutation <- writer.Commit(ctx)
			}()
			waitSchemaLock(t, ctx, pool, writerPID)
			// Buffered sends release the barriers; deferred closes also unblock on failure.
			release <- struct{}{}
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("RUN did not complete")
			}
			if err := <-mutation; err != nil {
				t.Fatal(err)
			}
			finish <- struct{}{}
			if err := <-process; err != nil {
				t.Fatal(err)
			}
			assertSchemaRunDenied(t, ctx, pool, req, observer, 1)
			if err := s.Process(ctx, req.ID); err != nil {
				t.Fatal(err)
			}
			assertSchemaRunDenied(t, ctx, pool, req, observer, 1)
		})
		t.Run("mutation first/"+kind, func(t *testing.T) {
			ctx, pool, req, observer, gate, s := schemaRetryFixture(t)
			if _, err := pool.Exec(ctx, `UPDATE data_authorization SET status='APPROVED' WHERE id=$1`, req.Plan.AuthorizationID); err != nil {
				t.Fatal(err)
			}
			s.Rights = schemaRunEntitlements{gate}
			var mutationErr error
			observer.onStartup = func() {
				writer, err := pool.Begin(ctx)
				if err != nil {
					mutationErr = err
					return
				}
				defer writer.Rollback(ctx)
				if mutationErr = schemaRunMutation(ctx, writer, s, req, kind); mutationErr == nil {
					mutationErr = writer.Commit(ctx)
				}
			}
			if err := s.Process(ctx, req.ID); err != nil {
				t.Fatal(err)
			}
			if mutationErr != nil {
				t.Fatal(mutationErr)
			}
			assertSchemaRunDenied(t, ctx, pool, req, observer, 0)
		})
	}
}

func assertSchemaRunDenied(t *testing.T, ctx context.Context, pool *pgxpool.Pool, req d.SchemaRequest, observer *retrySchemaObserver, sourceIO int) {
	t.Helper()
	if observer.sourceIO != sourceIO || observer.scans != 1 {
		t.Fatalf("source start/replay: reads=%d scans=%d", observer.sourceIO, observer.scans)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status); err != nil || status != "DENIED" {
		t.Fatalf("final observation: %s %v", status, err)
	}
	var costs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, req.ID).Scan(&costs); err != nil || costs != 1 {
		t.Fatal("native startup cost lost or duplicated", costs, err)
	}
}
