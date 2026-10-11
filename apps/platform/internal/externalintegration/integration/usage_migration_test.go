package integration_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUsageMigrationRoundTripAndConcurrentHistoricalDown(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	must(t, err)
	name := "external_usage_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` TEMPLATE template0`)
	must(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH(FORCE)`)
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	must(t, err)
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(t, err)
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../migrations"))
	pinned := t.TempDir()
	entries, err := os.ReadDir(dir)
	must(t, err)
	for _, entry := range entries {
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok || entry.IsDir() {
			continue
		}
		v, err := strconv.Atoi(prefix)
		if err != nil || v > 58 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		must(t, err)
		must(t, os.WriteFile(filepath.Join(pinned, entry.Name()), raw, 0600))
	}
	runner := migration.NewRunner(pool, pinned)
	must(t, runner.Up(ctx))
	must(t, runner.Down(ctx))
	must(t, runner.Up(ctx))
	u, err := url.Parse(dsn)
	must(t, err)
	u.Path = "/" + name
	u.RawPath = ""
	t.Setenv("TEST_POSTGRES_DSN", u.String())
	f := executionSetup(t, false)
	writer, err := pool.Begin(ctx)
	must(t, err)
	t.Cleanup(func() { _ = writer.Rollback(ctx) })
	_, err = writer.Exec(ctx, `SELECT id FROM external_identity_binding WHERE id=$1 FOR UPDATE`, f.binding)
	must(t, err)
	started := make(chan error, 1)
	go func() { started <- runner.Down(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var n int
		must(t, pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%LOCK TABLE external_identity_binding,external_usage_source%'`).Scan(&n))
		if n > 0 {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("down did not wait for parent-first writer")
	}
	// The writer held the parent before attempting any child INSERT. Downgrade
	// must observe its late commit, never drop the newly admitted history.
	id := uuid.New()
	_, err = writer.Exec(ctx, `INSERT INTO external_usage_source(id,workspace_id,binding_id,provider,instance_ref,adapter_revision,stream,external_subject_id,actor_id,principal_ref,evidence_id)VALUES($1,$2,$3,$4,'synthetic','fake-v1',$5,$6,$7,$8,$9)`, id, f.ws, f.binding, f.providerName, "urn:late:"+id.String(), "subject:"+f.providerName, f.actor, func() string { p, _ := f.caller(ctx); return p.Subject }(), f.basis)
	must(t, err)
	must(t, writer.Commit(ctx))
	select {
	case err = <-started:
		if err == nil || !strings.Contains(err.Error(), "cannot downgrade immutable external usage history") {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("down deadlocked")
	}
	var count int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM external_usage_source WHERE id=$1`, id).Scan(&count))
	if count != 1 {
		t.Fatal("late history lost")
	}
}
