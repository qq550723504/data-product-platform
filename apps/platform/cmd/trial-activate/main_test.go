package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A new template0 database isolates this test from all application packages and
// retained trial data. No migrations, provider or storage client are needed.
func TestSnapshotOriginalColumnsPrecisionAndReadOnlyInspection(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	defer admin.Close()
	name := "trial_activation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("test config unavailable")
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE "original odd" (id bigint, amount numeric); INSERT INTO "original odd" VALUES(1,9007199254740993),(1,9007199254740993)`); err != nil {
		t.Fatal(err)
	}
	before, err := snapshot(ctx, pool, inventory{})
	if err != nil {
		t.Fatal(err)
	}
	old := before.Tables["original odd"]
	if len(old.Rows) != 2 || !strings.Contains(old.Rows[0], "9007199254740993") || old.Rows[0] != old.Rows[1] {
		t.Fatal("precision or duplicate fact lost")
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE "original odd" ADD COLUMN added text DEFAULT 'new'; INSERT INTO "original odd"(id,amount) VALUES(2,4); CREATE TABLE newly_added(id int)`); err != nil {
		t.Fatal(err)
	}
	after, err := snapshot(ctx, pool, before)
	if err != nil {
		t.Fatal(err)
	}
	projected := after.Tables["original odd"]
	if len(projected.Columns) != 2 || len(projected.Rows) != 3 || projected.Rows[0] != old.Rows[0] || projected.Rows[1] != old.Rows[1] {
		t.Fatal("old column projection or multiset changed")
	}
	if _, ok := after.Tables["newly_added"]; !ok {
		t.Fatal("new table missing")
	}
	if _, err = pool.Exec(ctx, `ALTER TABLE "original odd" DROP COLUMN amount`); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshot(ctx, pool, before); err == nil {
		t.Fatal("original column removal accepted")
	}
	if _, err = pool.Exec(ctx, `DROP TABLE "original odd"`); err != nil {
		t.Fatal(err)
	}
	if _, err = snapshot(ctx, pool, before); err == nil {
		t.Fatal("original table removal accepted")
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("inspection unexpectedly wrote schema")
	}
}

func TestVerifyRejectsForeignProfileBeforeDatabaseOrProvider(t *testing.T) {
	// No pool is supplied: malformed composition must fail before any call.
	if _, err := verify(context.Background(), nil, config{}, input{Profile: "missing"}); err == nil {
		t.Fatal("foreign profile accepted")
	}
}
