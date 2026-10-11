package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

func TestConcurrentBindingsCannotMoveAnotherWorkspacesAsset(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	resources := []uuid.UUID{uuid.New(), uuid.New()}
	for _, id := range resources {
		if _, err := pool.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'Concurrent metadata binding','TABLE_LIKE')`, id, uuid.New(), "META-"+id.String()); err != nil {
			t.Fatal(err)
		}
	}
	repo := infrastructure.NewPostgresRepository(pool)
	fqn := "pgi09." + uuid.NewString()
	type result struct {
		binding domain.ResourceBinding
		err     error
	}
	start, done := make(chan struct{}), make(chan result, 2)
	for _, resource := range resources {
		go func(id uuid.UUID) {
			<-start
			tx, err := pool.Begin(ctx)
			if err != nil {
				done <- result{err: err}
				return
			}
			binding, err := repo.UpsertBinding(ctx, tx, domain.ResourceBinding{ResourceID: id, Provider: testProvider, EntityType: "TABLE", ExternalID: "pgi09-table", ExternalFQN: fqn, IsPrimary: true})
			if err != nil {
				_ = tx.Rollback(ctx)
			} else {
				err = tx.Commit(ctx)
			}
			done <- result{binding, err}
		}(resource)
	}
	close(start)
	var winner domain.ResourceBinding
	wins, conflicts := 0, 0
	for range resources {
		r := <-done
		switch {
		case r.err == nil:
			winner, wins = r.binding, wins+1
		case errors.Is(r.err, infrastructure.ErrBindingConflict):
			conflicts++
		default:
			t.Fatal(r.err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d; want one owner and one rejection", wins, conflicts)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	winner.BindingMetadata = map[string]any{"version": 0.2}
	refreshed, err := repo.UpsertBinding(ctx, tx, winner)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if refreshed.ID != winner.ID || refreshed.ResourceID != winner.ResourceID || refreshed.BindingMetadata["version"] != 0.2 {
		t.Fatalf("same-resource refresh changed stable binding: %+v", refreshed)
	}
}
