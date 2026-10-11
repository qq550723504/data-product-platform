package integration_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
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

func TestPublicationExecutionMigrationRoundTripAndConcurrentHistoricalDown(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	must(t, err)
	name := "external_execution_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` TEMPLATE template0`)
	must(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
		admin.Close()
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	must(t, err)
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(t, err)
	t.Cleanup(pool.Close)
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../../migrations"))
	pinned := t.TempDir()
	entries, err := os.ReadDir(dir)
	must(t, err)
	for _, entry := range entries {
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok || entry.IsDir() {
			continue
		}
		v, err := strconv.Atoi(prefix)
		if err != nil || v > 57 {
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
	p, _ := f.caller(ctx)
	ids, err := f.service.Repo.Identifiers(ctx, f.ws, 50, 0)
	must(t, err)
	a := d.Attempt{ID: uuid.New(), Number: 1, ActorID: f.actor, PrincipalRef: p.Subject, EvidenceID: f.basis, Request: d.Request{WorkspaceID: f.ws, PublicationID: f.pub, ReleaseID: f.release, BindingID: f.binding, IdentifierID: f.identifier, Provider: f.providerName, Instance: "synthetic", AdapterRevision: "fake-v1", ExternalSubjectID: "subject:" + f.providerName, ExternalIdentifier: ids[0].ExternalID, RequestKey: "down-guard/" + uuid.NewString()}}
	writer, err := pool.Begin(ctx)
	must(t, err)
	defer writer.Rollback(ctx)
	_, err = f.service.Repo.Binding(ctx, writer, f.ws, f.binding, true)
	must(t, err)
	must(t, f.service.Repo.Release(ctx, writer, f.ws, f.release, true))
	_, err = f.service.Repo.Identifier(ctx, writer, f.ws, f.identifier, true)
	must(t, err)
	_, err = f.service.Repo.Publication(ctx, writer, f.ws, f.pub, true)
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- runner.Down(ctx) }()
	deadline := time.Now().Add(8 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var n int
		must(t, admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND wait_event_type='Lock' AND query LIKE '%LOCK TABLE external_identity_binding%'`, name).Scan(&n))
		if n > 0 {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("down bypassed concurrent execution writer")
	}
	// The writer was admitted before down but had not inserted its child yet.
	// Down must not hold child table locks while waiting on these parent locks.
	must(t, f.service.Repo.InsertAttempt(ctx, writer, a))
	must(t, writer.Commit(ctx))
	if err := <-done; err == nil {
		t.Fatal("down destroyed concurrently committed attempt")
	}
	var n int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM external_publication_attempt WHERE id=$1`, a.ID).Scan(&n))
	if n != 1 {
		t.Fatal("failed down lost history")
	}
}
