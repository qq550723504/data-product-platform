package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// SQL-only synthetic facts. No storage provider is configured or called, even
// during cleanup. Each test owns a new database, never an existing pilot DB.
func planDatabase(t *testing.T) *pgxpool.Pool {
	pool, _ := planDatabaseAtVersion(t, 0)
	return pool
}

// Down guards must run at their exact migration boundary. A later unrelated
// empty migration can legitimately roll back without removing lifecycle facts.
// Zero keeps ordinary application tests on the complete current schema.
func planDatabaseAtVersion(t *testing.T, version int64) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set; PostgreSQL verification NOT_RUN")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "lifecycle_plan_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q TEMPLATE template0`, name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE %q WITH (FORCE)`, name)); err != nil {
			t.Errorf("drop owned scratch DB: %v", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate migrations")
	}
	dir := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../migrations"))
	if version > 0 {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		selected := t.TempDir()
		for _, entry := range entries {
			if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".up.sql") && !strings.HasSuffix(entry.Name(), ".down.sql")) {
				continue
			}
			prefix, _, valid := strings.Cut(entry.Name(), "_")
			v, err := strconv.ParseInt(prefix, 10, 64)
			if !valid || err != nil {
				t.Fatalf("invalid migration filename %s", entry.Name())
			}
			if v > version {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(selected, entry.Name()), body, 0600); err != nil {
				t.Fatal(err)
			}
		}
		dir = selected
	}
	if err := migration.NewRunner(pool, dir).Up(ctx); err != nil {
		t.Fatal(err)
	}
	if version > 0 {
		var applied int64
		if err := pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&applied); err != nil || applied != version {
			t.Fatalf("scratch migration boundary = %d, want %d: %v", applied, version, err)
		}
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	t.Cleanup(func() { outbox.ConfigureAppendObligation(nil) })
	return pool, dir
}

type testAuthority struct {
	mu     sync.Mutex
	caller principal.RequestPrincipal
	denied bool
	calls  int
}

func (a *testAuthority) AuthorizePlan(_ context.Context, _ pgx.Tx, _ uuid.UUID) (principal.RequestPrincipal, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	if a.denied {
		return principal.RequestPrincipal{}, principal.ErrUnauthenticated
	}
	return a.caller, nil
}

func seedPlanFixture(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, []domain.Copy, *testAuthority) {
	t.Helper()
	ctx := context.Background()
	w, dataset, version, namespace, provider, incarnation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'Synthetic lifecycle plan fixture','RAW')`, dataset, w, "LC-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size)
		VALUES($1,$2,1,'READY','OBJECT_STORAGE','s3://no-provider-configured/sql-only.csv','SHA256',repeat('a',64),12)`, version, dataset); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace) VALUES($1,$2,$3,$4,'synthetic-not-created')`, namespace, w, provider, incarnation); err != nil {
		t.Fatal(err)
	}
	var copies []domain.Copy
	for i := 0; i < 2; i++ {
		c := domain.Copy{ID: uuid.New(), WorkspaceID: w, DatasetVersionID: version, ProviderInstanceID: provider, IncarnationID: incarnation,
			Namespace: "synthetic-not-created", Key: fmt.Sprintf("sql-only/合成:%d.csv", i), ProviderVersion: fmt.Sprintf("not-a-live-version-%d", i), ContentSHA256: strings.Repeat("a", 64), ByteSize: 12}
		if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.ID, w, namespace, version, c.Key, c.ProviderVersion, c.ContentSHA256, c.ByteSize); err != nil {
			t.Fatal(err)
		}
		copies = append(copies, c)
	}
	a := &testAuthority{caller: principal.RequestPrincipal{Subject: "trusted-synthetic-operator", ActorID: uuid.New(), WorkspaceID: w, Capabilities: map[string]struct{}{CapabilityPreparePlan: {}}}}
	return w, copies, a
}

func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPlanCommandFrozenScopeReplayAuthorityAndAtomicFacts(t *testing.T) {
	pool := planDatabase(t)
	w, copies, a := seedPlanFixture(t, pool)
	s := NewPlanService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, a)
	s.now = func() time.Time { return time.Date(2026, 10, 11, 1, 2, 3, 123456789, time.UTC) }
	cmd := PrepareDeletePlanCommand{WorkspaceID: w, CopyIDs: []uuid.UUID{copies[1].ID, copies[0].ID}, IdempotencyKey: "scope-1"}
	p, err := s.PrepareDeletePlan(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.VerifyFrozenScope(); err != nil {
		t.Fatal(err)
	}
	if len(p.Targets) != 2 {
		t.Fatal("scope missing member")
	}
	var dbHash string
	if err := pool.QueryRow(context.Background(), `SELECT lifecycle_plan_scope_hash($1)`, p.ID).Scan(&dbHash); err != nil {
		t.Fatal(err)
	}
	if dbHash != p.ContentHash {
		t.Fatal("Go/PostgreSQL framing differs for UTF-8 key")
	}
	cmd.CopyIDs = []uuid.UUID{copies[0].ID, copies[1].ID}
	replay, err := s.PrepareDeletePlan(context.Background(), cmd)
	if err != nil || replay.ID != p.ID {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	for _, query := range []string{
		`SELECT count(*) FROM lifecycle_disposition_plan`,
		`SELECT count(*) FROM command_idempotency WHERE command_type='LIFECYCLE.PREPARE_DELETE_PLAN'`,
		`SELECT count(*) FROM outbox_event WHERE event_type='LifecycleDispositionPlanPrepared'`,
		`SELECT count(*) FROM audit_event WHERE action='LifecycleDispositionPlanPrepared'`,
		`SELECT count(*) FROM evidence WHERE evidence_type='LIFECYCLE_PLAN'`,
	} {
		if n := count(t, pool, query); n != 1 {
			t.Fatalf("duplicate/missing atomic fact: %d / %s", n, query)
		}
	}
	if n := count(t, pool, `SELECT count(*) FROM cost_event`); n != 0 {
		t.Fatal("invented provider work/cost for planning")
	}
	cmd.CopyIDs = []uuid.UUID{copies[0].ID}
	if _, err := s.PrepareDeletePlan(context.Background(), cmd); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("modified scope same key accepted: %v", err)
	}
	a.mu.Lock()
	a.denied = true
	a.mu.Unlock()
	cmd.CopyIDs = []uuid.UUID{copies[0].ID, copies[1].ID}
	if _, err := s.PrepareDeletePlan(context.Background(), cmd); !errors.Is(err, principal.ErrUnauthenticated) {
		t.Fatalf("replay bypassed current authority: %v", err)
	}
	if a.calls != 4 {
		t.Fatalf("authority calls=%d", a.calls)
	}
	var payload string
	if err := pool.QueryRow(context.Background(), `SELECT payload::text FROM outbox_event WHERE aggregate_id=$1`, p.ID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(payload, "sql-only") || strings.Contains(payload, "provider_version") || strings.Contains(payload, "storage_uri") {
		t.Fatal("object details leaked into event proof")
	}
	for _, query := range []string{
		`UPDATE lifecycle_disposition_plan SET prepared_by=gen_random_uuid() WHERE id=$1`,
		`DELETE FROM lifecycle_disposition_plan_target WHERE plan_id=$1`,
		`DELETE FROM lifecycle_disposition_plan WHERE id=$1`,
	} {
		if _, err := pool.Exec(context.Background(), query, p.ID); err == nil {
			t.Fatalf("frozen mutation accepted: %s", query)
		}
	}
	if _, err := pool.Exec(context.Background(), `TRUNCATE lifecycle_disposition_plan_target`); err == nil {
		t.Fatal("truncate erased frozen membership")
	}
}

func TestPlanCommandRejectsForeignScopeAndRollsBackProofFailure(t *testing.T) {
	pool := planDatabase(t)
	w, copies, a := seedPlanFixture(t, pool)
	s := NewPlanService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, a)
	cmd := PrepareDeletePlanCommand{WorkspaceID: w, CopyIDs: []uuid.UUID{uuid.New()}, IdempotencyKey: "foreign-or-missing"}
	if _, err := s.PrepareDeletePlan(context.Background(), cmd); !errors.Is(err, infrastructure.ErrScopeUnavailable) {
		t.Fatalf("unregistered accepted: %v", err)
	}
	foreign, foreignCopies, _ := seedPlanFixture(t, pool)
	cmd.CopyIDs = []uuid.UUID{foreignCopies[0].ID}
	if _, err := s.PrepareDeletePlan(context.Background(), cmd); !errors.Is(err, infrastructure.ErrScopeUnavailable) {
		t.Fatalf("foreign copy accepted: %v", err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO lifecycle_disposition_plan_target(plan_id,workspace_id,copy_id) VALUES(gen_random_uuid(),$1,$2)`, foreign, copies[0].ID); err == nil {
		t.Fatal("cross-workspace FK accepted")
	}
	cmd.CopyIDs = []uuid.UUID{copies[0].ID}
	outbox.ConfigureAppendObligation(nil)
	if _, err := s.PrepareDeletePlan(context.Background(), cmd); err == nil {
		t.Fatal("command silently dropped required event")
	}
	for _, query := range []string{`SELECT count(*) FROM lifecycle_disposition_plan`, `SELECT count(*) FROM lifecycle_disposition_plan_target`,
		`SELECT count(*) FROM command_idempotency WHERE command_type='LIFECYCLE.PREPARE_DELETE_PLAN'`,
		`SELECT count(*) FROM audit_event WHERE action='LifecycleDispositionPlanPrepared'`, `SELECT count(*) FROM evidence WHERE evidence_type='LIFECYCLE_PLAN'`} {
		if count(t, pool, query) != 0 {
			t.Fatalf("partial commit after proof failure: %s", query)
		}
	}
}

func TestPlanConcurrentSameKeyHasOneHistory(t *testing.T) {
	pool := planDatabase(t)
	w, copies, a := seedPlanFixture(t, pool)
	s := NewPlanService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, a)
	cmd := PrepareDeletePlanCommand{WorkspaceID: w, CopyIDs: []uuid.UUID{copies[0].ID}, IdempotencyKey: "concurrent"}
	start := make(chan struct{})
	results := make(chan domain.Plan, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { <-start; p, err := s.PrepareDeletePlan(context.Background(), cmd); results <- p; errs <- err }()
	}
	close(start)
	first, second := <-results, <-results
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if first.ID != second.ID || count(t, pool, `SELECT count(*) FROM lifecycle_disposition_plan`) != 1 {
		t.Fatal("duplicate plan under concurrency")
	}
	if count(t, pool, `SELECT count(*) FROM outbox_event WHERE event_type='LifecycleDispositionPlanPrepared'`) != 1 {
		t.Fatal("duplicate event under concurrency")
	}
}

func insertDraft(t *testing.T, pool *pgxpool.Pool, w uuid.UUID, c domain.Copy) domain.Plan {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	r := &infrastructure.PostgresRepository{}
	p := domain.Plan{ID: uuid.New(), WorkspaceID: w, Action: domain.DeleteExact, Status: domain.Draft, PreparedBy: uuid.New(), PrincipalRef: "synthetic", CreatedAt: time.Now().UTC()}
	if err := r.InsertDraft(ctx, tx, p); err != nil {
		t.Fatal(err)
	}
	if err := r.AddTarget(ctx, tx, w, p.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return p
}

// Wait for an observed PostgreSQL lock wait, never infer a race from sleep.
func waitForPlanLock(t *testing.T, pool *pgxpool.Pool, pid int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("writer did not reach parent lock barrier")
		case <-ticker.C:
		}
	}
}

func TestPlanFreezeSerializesLateMembershipBothOrders(t *testing.T) {
	pool := planDatabase(t)
	w, copies, _ := seedPlanFixture(t, pool)
	ctx := context.Background()
	r := &infrastructure.PostgresRepository{}
	for _, memberFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("member-first=%t", memberFirst), func(t *testing.T) {
			p := insertDraft(t, pool, w, copies[0])
			tx1, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx1.Rollback(ctx)
			tx2, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx2.Rollback(ctx)
			var pid int32
			if err := tx2.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			if memberFirst {
				if err := r.AddTarget(ctx, tx1, w, p.ID, copies[1].ID); err != nil {
					t.Fatal(err)
				}
				go func() { _, err := r.Finalize(ctx, tx2, w, p.ID, time.Now().UTC()); done <- err }()
			} else {
				if _, err := r.Finalize(ctx, tx1, w, p.ID, time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
				go func() {
					_, err := tx2.Exec(ctx, `INSERT INTO lifecycle_disposition_plan_target(plan_id,workspace_id,copy_id) VALUES($1,$2,$3)`, p.ID, w, copies[1].ID)
					done <- err
				}()
			}
			waitForPlanLock(t, pool, pid)
			if err := tx1.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			err = <-done
			if memberFirst {
				if err != nil {
					t.Fatal(err)
				}
				if err := tx2.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("late member crossed finalization")
				}
				_ = tx2.Rollback(ctx)
			}
			expected := 1
			if memberFirst {
				expected = 2
			}
			if n := count(t, pool, `SELECT count(*) FROM lifecycle_disposition_plan_target WHERE plan_id=$1`, p.ID); n != expected {
				t.Fatalf("membership=%d want=%d", n, expected)
			}
			checkTx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer checkTx.Rollback(ctx)
			frozen, err := r.GetPlan(ctx, checkTx, w, p.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			if err := frozen.VerifyFrozenScope(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlanFinalizeRejectsForgedHashEmptyAndDownDropsNoHistory(t *testing.T) {
	pool, dir := planDatabaseAtVersion(t, 60)
	w, copies, _ := seedPlanFixture(t, pool)
	ctx := context.Background()
	p := insertDraft(t, pool, w, copies[0])
	if _, err := pool.Exec(ctx, `UPDATE lifecycle_disposition_plan SET status='FINALIZED',finalized_at=now(),content_hash=repeat('b',64) WHERE id=$1`, p.ID); err == nil {
		t.Fatal("forged hash accepted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM lifecycle_disposition_plan_target WHERE plan_id=$1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE lifecycle_disposition_plan SET status='FINALIZED',finalized_at=now(),content_hash=lifecycle_plan_scope_hash(id) WHERE id=$1`, p.ID); err == nil {
		t.Fatal("empty scope finalized")
	}
	if _, err := pool.Exec(ctx, `UPDATE lifecycle_fixture_copy SET provider_version='replacement' WHERE id=$1`, copies[0].ID); err == nil {
		t.Fatal("historical provider identity overwritten")
	}
	snapshot := func() map[string]string {
		facts := map[string]string{}
		for _, table := range []string{"lifecycle_fixture_namespace", "lifecycle_fixture_copy", "lifecycle_disposition_plan", "lifecycle_disposition_plan_target", "schema_migration"} {
			var digest string
			query := `SELECT count(*)::text || '|' || md5(coalesce(string_agg(t::text,',' ORDER BY t::text),'')) FROM ` + pgx.Identifier{table}.Sanitize() + ` t`
			if err := pool.QueryRow(ctx, query).Scan(&digest); err != nil {
				t.Fatal(err)
			}
			facts[table] = digest
		}
		return facts
	}
	before := snapshot()
	err := migration.NewRunner(pool, dir).Down(ctx)
	if err == nil || !strings.Contains(err.Error(), "execute migration 000060_lifecycle_fixture_plan.down.sql") || !strings.Contains(err.Error(), "cannot remove lifecycle fixture or disposition history") {
		t.Fatalf("migration60 did not refuse destructive history removal: %v", err)
	}
	for table, digest := range snapshot() {
		if digest != before[table] {
			t.Fatalf("failed migration60 down changed %s history", table)
		}
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_disposition_plan WHERE id=$1`, p.ID) != 1 {
		t.Fatal("down lost historical plan")
	}
}
