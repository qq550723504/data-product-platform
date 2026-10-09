package cost

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateAmount(t *testing.T) {
	for _, value := range []string{"0", "0.000001", "12345678901234.123456", "-9.5"} {
		v := value
		if err := validateAmount(&v); err != nil {
			t.Errorf("valid amount %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"", "1.0000001", "123456789012345", "1e2", "1,000", "NaN", "+2"} {
		v := value
		if err := validateAmount(&v); err == nil {
			t.Errorf("invalid amount %q accepted", value)
		}
	}
	if err := validateAmount(nil); err != nil {
		t.Fatalf("nil amount rejected: %v", err)
	}
}

func TestAppendExactDecimalAmount(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse postgres DSN: %v", err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `CREATE TEMP TABLE cost_event (
		id uuid, workspace_id uuid, execution_id uuid, activity_id uuid,
		cost_type text, quantity numeric(20,6), unit text, amount numeric(20,6),
		currency text, pricing_mode text, metadata jsonb, occurred_at timestamptz
	)`); err != nil {
		t.Fatalf("create temporary cost table: %v", err)
	}
	for _, value := range []string{"12345678901234.123456", "0.000001"} {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatalf("begin transaction: %v", err)
		}
		id := uuid.New()
		err = Append(ctx, tx, Event{
			ID: id, WorkspaceID: uuid.New(), CostType: "TEST", Quantity: 1,
			Unit: "unit", Amount: &value, Currency: "USD", PricingMode: "ACTUAL",
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("append amount %q: %v", value, err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		var actual string
		if err := conn.QueryRow(ctx, `SELECT amount::text FROM cost_event WHERE id=$1`, id).Scan(&actual); err != nil {
			t.Fatalf("read amount: %v", err)
		}
		// PostgreSQL numeric(20,6) intentionally normalizes the scale to six places.
		if actual != value {
			t.Fatalf("stored amount = %q, want %q", actual, value)
		}
	}
}
