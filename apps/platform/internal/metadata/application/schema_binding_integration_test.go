package application

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

func TestSourceSchemaRejectsReplacedPrimaryBinding(t *testing.T) {
	if os.Getenv("TEST_POSTGRES_DSN") == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, os.Getenv("TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	resource, workspace := uuid.New(), uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'schema binding test','TABLE_LIKE')`, resource, workspace, resource.String())
	if err != nil {
		t.Fatal(err)
	}
	provider := d.Provider("TEST_METADATA")
	b, err := infra.NewPostgresRepository(pool).UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: resource, Provider: provider, EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: "test.energy", IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	s := &SchemaService{Service: &Service{provider: provider}}
	req := d.SchemaRequest{BindingID: b.ID, ExternalID: b.ExternalID, Plan: d.SchemaPlan{ResourceID: resource, ExternalFQN: b.ExternalFQN}}
	if err := s.currentBinding(ctx, tx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE resource_binding SET is_primary=false WHERE id=$1`, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.currentBinding(ctx, tx, req); !errors.Is(err, ErrSchemaDenied) {
		t.Fatalf("replaced primary: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE resource_binding SET is_primary=true,external_id=$2 WHERE id=$1`, b.ID, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err := s.currentBinding(ctx, tx, req); !errors.Is(err, ErrSchemaDenied) {
		t.Fatalf("changed provider identity: %v", err)
	}
}

func waitSchemaLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, pid uint32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT cardinality(pg_blocking_pids($1))>0`, pid).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected PostgreSQL lock contention was not observed")
}

func TestSourceSchemaBindingFinalizationSerializesMutations(t *testing.T) {
	for _, mutation := range []string{"native refresh/demotion", "direct identity update"} {
		t.Run("observation first/"+mutation, func(t *testing.T) {
			ctx, pool, req, _, _, s := schemaRetryFixture(t)
			final, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer final.Rollback(ctx)
			if err = s.authorize(ctx, final, req.Plan); err != nil {
				t.Fatal(err)
			}
			if err = s.currentBinding(ctx, final, req); err != nil {
				t.Fatal(err)
			}
			writer, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Rollback(ctx)
			done := make(chan error, 1)
			go func() {
				if mutation == "native refresh/demotion" {
					_, err := s.Service.repo.UpsertBinding(ctx, writer, d.ResourceBinding{ResourceID: req.Plan.ResourceID, Provider: s.Service.provider, EntityType: "TABLE", ExternalID: req.ExternalID, ExternalFQN: req.Plan.ExternalFQN, IsPrimary: false})
					done <- err
				} else {
					_, err := writer.Exec(ctx, `UPDATE resource_binding SET external_id=$2 WHERE id=$1`, req.BindingID, uuid.NewString())
					done <- err
				}
			}()
			waitSchemaLock(t, ctx, pool, writer.Conn().PgConn().PID())
			attempt := uuid.New()
			if err = s.Service.repo.BeginSchemaAttempt(ctx, final, req, attempt, []byte(`{"fixture":"binding-runtime"}`)); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if err = s.record(ctx, final, req, d.SchemaObservation{ID: uuid.New(), RequestID: req.ID, AttemptID: &attempt, Status: "MATCH", Reason: "BASELINE_MATCH", Columns: req.Plan.Columns, SchemaHash: d.Hash(req.Plan.Columns), StartedAt: &now, CompletedAt: &now}); err != nil {
				t.Fatal(err)
			}
			if err = final.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
			if err = writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			check, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer check.Rollback(ctx)
			if err = s.currentBinding(ctx, check, req); !errors.Is(err, ErrSchemaDenied) {
				t.Fatalf("changed binding accepted: %v", err)
			}
			var status string
			if err = pool.QueryRow(ctx, `SELECT status FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status); err != nil || status != "MATCH" {
				t.Fatalf("historical observation changed: %s %v", status, err)
			}
		})
	}
	t.Run("binding mutation first", func(t *testing.T) {
		ctx, pool, req, _, _, s := schemaRetryFixture(t)
		writer, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Rollback(ctx)
		if _, err = s.Service.repo.UpsertBinding(ctx, writer, d.ResourceBinding{ResourceID: req.Plan.ResourceID, Provider: s.Service.provider, EntityType: "TABLE", ExternalID: req.ExternalID, ExternalFQN: req.Plan.ExternalFQN, IsPrimary: false}); err != nil {
			t.Fatal(err)
		}
		final, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer final.Rollback(ctx)
		done := make(chan error, 1)
		go func() {
			if err := s.authorize(ctx, final, req.Plan); err != nil {
				done <- err
				return
			}
			if err := s.currentBinding(ctx, final, req); !errors.Is(err, ErrSchemaDenied) {
				done <- errors.New("committed binding mutation was not denied")
				return
			}
			done <- s.record(ctx, final, req, d.SchemaObservation{ID: uuid.New(), RequestID: req.ID, Status: "DENIED", Reason: "SOURCE_BINDING_CHANGED"})
		}()
		waitSchemaLock(t, ctx, pool, final.Conn().PgConn().PID())
		if err = writer.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err = <-done; err != nil {
			t.Fatal(err)
		}
		if err = final.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var status string
		if err = pool.QueryRow(ctx, `SELECT status FROM source_schema_observation WHERE request_id=$1`, req.ID).Scan(&status); err != nil || status != "DENIED" {
			t.Fatalf("stale observation committed: %s %v", status, err)
		}
	})
}
