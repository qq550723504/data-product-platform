package transaction_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

func TestAuditAndOutboxCommitAtomically(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	manager := transaction.NewManager(pool)

	t.Run("rollback removes both records", func(t *testing.T) {
		objectID := uuid.New()
		event, err := outbox.NewEvent("TEST", objectID, "TestEventRolledBack", map[string]any{"ok": false})
		if err != nil {
			t.Fatalf("new outbox event: %v", err)
		}

		expectedErr := errors.New("force rollback")
		err = manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if err := audit.Append(ctx, tx, audit.Event{
				Action:     "TEST_ROLLBACK",
				ObjectType: "TEST",
				ObjectID:   objectID,
			}); err != nil {
				return err
			}
			if err := outbox.Append(ctx, tx, event); err != nil {
				return err
			}
			return expectedErr
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("transaction error = %v, want %v", err, expectedErr)
		}

		assertCount(t, ctx, pool, `SELECT count(*) FROM audit_event WHERE object_id = $1`, objectID, 0)
		assertCount(t, ctx, pool, `SELECT count(*) FROM outbox_event WHERE aggregate_id = $1`, objectID, 0)
	})

	t.Run("commit persists both records", func(t *testing.T) {
		objectID := uuid.New()
		event, err := outbox.NewEvent("TEST", objectID, "TestEventCommitted", map[string]any{"ok": true})
		if err != nil {
			t.Fatalf("new outbox event: %v", err)
		}

		if err := manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if err := audit.Append(ctx, tx, audit.Event{
				Action:     "TEST_COMMIT",
				ObjectType: "TEST",
				ObjectID:   objectID,
			}); err != nil {
				return err
			}
			return outbox.Append(ctx, tx, event)
		}); err != nil {
			t.Fatalf("commit transaction: %v", err)
		}

		assertCount(t, ctx, pool, `SELECT count(*) FROM audit_event WHERE object_id = $1`, objectID, 1)
		assertCount(t, ctx, pool, `SELECT count(*) FROM outbox_event WHERE aggregate_id = $1`, objectID, 1)

		_, _ = pool.Exec(ctx, `DELETE FROM audit_event WHERE object_id = $1`, objectID)
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_event WHERE aggregate_id = $1`, objectID)
	})
}

func TestAdvisoryLockSerializesCriticalSection(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()
	manager := transaction.NewManager(pool)

	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	key := "test-finalize-" + uuid.NewString()

	go func() {
		firstDone <- manager.WithAdvisoryLock(ctx, key, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()

	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first advisory lock was not acquired")
	}

	err = manager.WithAdvisoryLock(ctx, key, func(context.Context) error {
		t.Fatal("second critical section must not run while lock is held")
		return nil
	})
	if !errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		t.Fatalf("second lock error = %v, want ErrAdvisoryLockBusy", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first lock holder: %v", err)
	}
	if err := manager.WithAdvisoryLock(ctx, key, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("lock should be reusable after release: %v", err)
	}
}

func TestAdvisoryLockReusesConnectionForNestedTransaction(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()
	manager := transaction.NewManager(pool)

	key := "test-nested-transaction-" + uuid.NewString()
	if err := manager.WithAdvisoryLock(ctx, key, func(ctx context.Context) error {
		return manager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `SELECT 1`)
			return err
		})
	}); err != nil {
		t.Fatalf("nested transaction under advisory lock: %v", err)
	}
}

func assertCount(t *testing.T, ctx context.Context, pool queryRower, query string, objectID uuid.UUID, want int) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query, objectID).Scan(&got); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
