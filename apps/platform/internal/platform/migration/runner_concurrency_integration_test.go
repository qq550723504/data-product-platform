package migration_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	migration "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
)

func TestRunnerConcurrentUpRechecksStateAfterAdvisoryLock(t *testing.T) {
	pool := scratchDatabase(t, 0)
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "000001_concurrent.up.sql"), []byte(`
		SELECT pg_sleep(0.4);
		CREATE TABLE migration_lock_probe (
			id integer PRIMARY KEY
		);
		INSERT INTO migration_lock_probe(id) VALUES (1);
	`), 0o600); err != nil {
		t.Fatalf("write concurrent migration: %v", err)
	}

	ctx := context.Background()
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	run := func() {
		defer wg.Done()
		<-start
		errs <- migration.NewRunner(pool, dir).Up(ctx)
	}
	go run()
	go run()
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent migration runner failed: %v", err)
		}
	}

	var applied int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM schema_migration
		WHERE version=1
	`).Scan(&applied); err != nil {
		t.Fatalf("count migration bookkeeping rows: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration bookkeeping rows = %d, want 1", applied)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM migration_lock_probe`).Scan(&rows); err != nil {
		t.Fatalf("count migration side effects: %v", err)
	}
	if rows != 1 {
		t.Fatalf("migration side effects = %d, want 1", rows)
	}
}

func TestRunnerUpRejectsChangedMigrationPredecessor(t *testing.T) {
	pool := scratchDatabase(t, 0)
	dir := t.TempDir()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_migration (
		version bigint PRIMARY KEY,
		name text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		t.Fatalf("prepare migration state: %v", err)
	}
	// A newer committed version must not allow an older unapplied migration
	// to execute, even if its pre-lock check reported that version as missing.
	if _, err := pool.Exec(ctx, `INSERT INTO schema_migration(version,name) VALUES (2,'concurrent-down-up')`); err != nil {
		t.Fatalf("seed newer migration: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000001_predecessor.up.sql"), []byte(`CREATE TABLE invalid_out_of_order_migration(id integer)`), 0o600); err != nil {
		t.Fatalf("write migration fixture: %v", err)
	}
	if err := migration.NewRunner(pool, dir).Up(ctx); err == nil {
		t.Fatal("up unexpectedly applied an out-of-order migration")
	}
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('invalid_out_of_order_migration') IS NOT NULL`).Scan(&exists); err != nil {
		t.Fatalf("inspect migration side effect: %v", err)
	}
	if exists {
		t.Fatal("out-of-order migration SQL executed")
	}
}
