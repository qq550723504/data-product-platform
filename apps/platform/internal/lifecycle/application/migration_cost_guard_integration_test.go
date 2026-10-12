package application

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Real PostgreSQL compares the whole compiled main59 guard before/after an
// empty 60 up/down. External provider/usage subjects must survive exactly;
// checking only the newly added lifecycle branch would miss this regression.
func TestMigration60DownRestoresEntireMainCostAllocationGuard(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; PostgreSQL NOT_RUN")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	db := "lifecycle_cost_guard_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, e = admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q TEMPLATE template0`, db)); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if _, e := admin.Exec(ctx, fmt.Sprintf(`DROP DATABASE %q WITH (FORCE)`, db)); e != nil {
			t.Error(e)
		}
	}()
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.Database = db
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../migrations"))
	dir := t.TempDir()
	entries, e := os.ReadDir(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range entries {
		if f.Name()[:6] > "000059" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(root, f.Name()))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, f.Name()), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	r := migration.NewRunner(p, dir)
	if e = r.Up(ctx); e != nil {
		t.Fatal(e)
	}
	read := func() (string, string) {
		var fn, check string
		if e := p.QueryRow(ctx, `SELECT pg_get_functiondef('guard_cost_allocation_workspace()'::regprocedure)`).Scan(&fn); e != nil {
			t.Fatal(e)
		}
		if e := p.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='cost_allocation'::regclass AND conname='ck_cost_allocation_single_subject'`).Scan(&check); e != nil {
			t.Fatal(e)
		}
		return fn, check
	}
	beforeFn, beforeCheck := read()
	if !strings.Contains(beforeFn, "external_provider_call_id") || !strings.Contains(beforeFn, "external_usage_call_id") {
		t.Fatal("main external subjects missing")
	}
	for _, suffix := range []string{"up", "down"} {
		name := "000060_lifecycle_fixture_plan." + suffix + ".sql"
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(dir, name), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e = r.Up(ctx); e != nil {
		t.Fatal(e)
	}
	upFn, upCheck := read()
	if !strings.Contains(upFn, "external_provider_call_id") || !strings.Contains(upFn, "external_usage_call_id") || !strings.Contains(upCheck, "external_provider_call_id") || !strings.Contains(upCheck, "external_usage_call_id") {
		t.Fatal("migration60 discarded main external subjects")
	}
	if e = r.Down(ctx); e != nil {
		t.Fatal(e)
	}
	afterFn, afterCheck := read()
	if beforeFn != afterFn || beforeCheck != afterCheck {
		t.Fatal("empty60 down did not restore the exact main59 cost guard")
	}
}
