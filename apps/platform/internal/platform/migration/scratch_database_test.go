package migration_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

// migrationsDir resolves the repository migrations directory from this source
// file, so the tests do not depend on the working directory.
func migrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve migration test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../"))
	return filepath.Join(root, "migrations")
}

// scratchDatabase creates an empty database from template0 and applies every
// migration up to and including maxVersion.
//
// Down-migration guard tests run against a scratch database instead of the
// shared one for two reasons: they must insert rows that a later migration would
// reject, and a guard test that depends on the shared database's latest version
// silently stops running as soon as any newer migration lands.
func scratchDatabase(t *testing.T, maxVersion int64) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()

	adminPool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(adminPool.Close)

	dbName := "migration_scratch_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:20]
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q TEMPLATE template0`, dbName)); err != nil {
		t.Fatalf("create scratch database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
	})

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	poolCfg.ConnConfig.Database = dbName
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("open scratch database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping scratch database: %v", err)
	}

	for _, version := range upMigrationVersions(t, maxVersion) {
		applyMigrationFile(t, pool, version, "up")
	}
	return pool
}

// upMigrationVersions lists migration versions that are <= maxVersion.
func upMigrationVersions(t *testing.T, maxVersion int64) []int64 {
	t.Helper()
	dir := migrationsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations directory: %v", err)
	}
	versions := make([]int64, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatalf("parse migration version from %q: %v", entry.Name(), err)
		}
		if version <= maxVersion {
			versions = append(versions, version)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions
}

// tryApplyMigrationFile executes one migration file and reports the failure
// instead of aborting the test, so a guard test can assert the exact refusal.
func tryApplyMigrationFile(t *testing.T, pool *pgxpool.Pool, version int64, direction string) error {
	t.Helper()
	ctx := context.Background()
	dir := migrationsDir(t)
	matches, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%06d_*.%s.sql", version, direction)))
	if err != nil || len(matches) == 0 {
		t.Fatalf("locate migration %d %s: %v (%v)", version, direction, matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read migration %s: %v", matches[0], err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return err
	}
	if direction == "up" {
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migration(version, name) VALUES ($1,$2)`, version, filepath.Base(matches[0])); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// applyMigrationFile executes one migration file and mirrors the runner's
// bookkeeping for up migrations so later steps see a consistent version table.
func applyMigrationFile(t *testing.T, pool *pgxpool.Pool, version int64, direction string) {
	t.Helper()
	ctx := context.Background()
	dir := migrationsDir(t)
	matches, err := filepath.Glob(filepath.Join(dir, fmt.Sprintf("%06d_*.%s.sql", version, direction)))
	if err != nil || len(matches) == 0 {
		t.Fatalf("locate migration %d %s: %v (%v)", version, direction, matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read migration %s: %v", matches[0], err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin migration %s: %v", matches[0], err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, string(content)); err != nil {
		t.Fatalf("apply migration %s: %v", matches[0], err)
	}
	if direction == "up" {
		if _, err := tx.Exec(ctx, `
			CREATE TABLE IF NOT EXISTS schema_migration (
				version bigint PRIMARY KEY,
				name text NOT NULL,
				applied_at timestamptz NOT NULL DEFAULT now()
			)
		`); err != nil {
			t.Fatalf("ensure schema_migration: %v", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migration(version, name) VALUES ($1,$2)`, version, filepath.Base(matches[0])); err != nil {
			t.Fatalf("record migration %s: %v", matches[0], err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit migration %s: %v", matches[0], err)
	}
}
