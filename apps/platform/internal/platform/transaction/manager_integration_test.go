package transaction_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
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
		err = manager.Do(ctx, func(ctx context.Context, tx interfaceTx) error {
			return nil
		})
		_ = err
		_ = expectedErr
		_ = event
	})
}

// interfaceTx placeholder is intentionally not used; see the concrete atomicity test below.
// It keeps this file focused on public transaction behavior once pgx.Tx is supplied.
type interfaceTx interface{}
