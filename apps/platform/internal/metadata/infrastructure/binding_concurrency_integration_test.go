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
	for _, renamed := range []bool{false, true} {
		t.Run(map[bool]string{false: "same name", true: "renamed asset"}[renamed], func(t *testing.T) {
			testConcurrentBindingOwnership(t, renamed)
		})
	}
}

func testConcurrentBindingOwnership(t *testing.T, renamed bool) {
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
	externalID := uuid.NewString()
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
			name := fqn
			if renamed {
				name += "." + id.String()
			}
			binding, err := repo.UpsertBinding(ctx, tx, domain.ResourceBinding{ResourceID: id, Provider: testProvider, EntityType: "TABLE", ExternalID: externalID, ExternalFQN: name, IsPrimary: true})
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

func TestBindingRenamePreservesOwnerAndIdentity(t *testing.T) {
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
	owner, other := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{owner, other} {
		if _, err := pool.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'Rename binding','TABLE_LIKE')`, id, uuid.New(), "META-"+id.String()); err != nil {
			t.Fatal(err)
		}
	}
	repo := infrastructure.NewPostgresRepository(pool)
	upsert := func(binding domain.ResourceBinding) (domain.ResourceBinding, error) {
		t.Helper()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		result, err := repo.UpsertBinding(ctx, tx, binding)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return result, err
	}
	initial := domain.ResourceBinding{ResourceID: owner, Provider: testProvider, EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: "rename." + uuid.NewString()}
	first, err := upsert(initial)
	if err != nil {
		t.Fatal(err)
	}
	initial.ExternalFQN += ".renamed"
	stolen := initial
	stolen.ResourceID = other
	if _, err := upsert(stolen); !errors.Is(err, infrastructure.ErrBindingConflict) {
		t.Fatalf("renamed asset takeover = %v, want ownership conflict", err)
	}
	for i := 0; i < 2; i++ {
		renamed, err := upsert(initial)
		if err != nil || renamed.ID != first.ID || renamed.ResourceID != owner || renamed.ExternalID != first.ExternalID || renamed.ExternalFQN != initial.ExternalFQN {
			t.Fatalf("rename/replay changed stable identity: %+v, %v", renamed, err)
		}
	}
	// A deleted/recreated provider object cannot silently replace a binding by name.
	for _, resource := range []uuid.UUID{owner, other} {
		replacement := initial
		replacement.ResourceID, replacement.ExternalID = resource, uuid.NewString()
		if _, err := upsert(replacement); !errors.Is(err, infrastructure.ErrBindingConflict) {
			t.Fatalf("same name replacement resource=%s: %v, want conflict", resource, err)
		}
	}
	otherAsset := domain.ResourceBinding{ResourceID: other, Provider: testProvider, EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: "other." + uuid.NewString()}
	otherBinding, err := upsert(otherAsset)
	if err != nil {
		t.Fatal(err)
	}
	otherAsset.ExternalFQN = initial.ExternalFQN
	if _, err := upsert(otherAsset); !errors.Is(err, infrastructure.ErrBindingConflict) {
		t.Fatalf("rename onto another identity's name = %v, want conflict", err)
	}
	for _, missing := range []string{"", "  "} {
		invalid := initial
		invalid.ExternalID = missing
		invalid.ExternalFQN = "invalid." + uuid.NewString()
		if _, err := upsert(invalid); err == nil {
			t.Fatalf("database accepted missing stable identity %q", missing)
		}
	}
	bindings, err := repo.ListBindings(ctx, owner)
	if err != nil || len(bindings) != 1 || bindings[0].ID != first.ID {
		t.Fatalf("owner bindings after rename/replay/conflicts: %+v, %v", bindings, err)
	}
	bindings, err = repo.ListBindings(ctx, other)
	if err != nil || len(bindings) != 1 || bindings[0].ID != otherBinding.ID || bindings[0].ExternalFQN != otherBinding.ExternalFQN {
		t.Fatalf("conflict changed other resource's separate binding: %+v, %v", bindings, err)
	}
}
