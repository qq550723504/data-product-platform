package transaction

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrAdvisoryLockBusy = errors.New("advisory lock is already held")

type Manager struct {
	pool *pgxpool.Pool
}

type advisoryLockConnectionContextKey struct{}

func NewManager(pool *pgxpool.Pool) *Manager {
	return &Manager{pool: pool}
}

func (m *Manager) Do(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	var (
		tx  pgx.Tx
		err error
	)
	if conn, ok := ctx.Value(advisoryLockConnectionContextKey{}).(*pgxpool.Conn); ok {
		tx, err = conn.BeginTx(ctx, pgx.TxOptions{})
	} else {
		tx, err = m.pool.BeginTx(ctx, pgx.TxOptions{})
	}
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			return fmt.Errorf("transaction failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return nil
}

// WithAdvisoryLock serializes a short cross-transaction critical section on a
// stable text key. PostgreSQL session advisory locks are released automatically
// if the process/connection dies, which makes them suitable for remote-output
// finalization that spans object-store I/O plus several Core transactions.
func (m *Manager) WithAdvisoryLock(ctx context.Context, key string, fn func(context.Context) error) error {
	if conn, ok := ctx.Value(advisoryLockConnectionContextKey{}).(*pgxpool.Conn); ok {
		return withAdvisoryLockOnConn(ctx, conn, key, fn)
	}
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire advisory-lock connection: %w", err)
	}
	defer conn.Release()
	return withAdvisoryLockOnConn(ctx, conn, key, fn)
}

func withAdvisoryLockOnConn(ctx context.Context, conn *pgxpool.Conn, key string, fn func(context.Context) error) error {
	var locked bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 0))`, key).Scan(&locked); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !locked {
		return ErrAdvisoryLockBusy
	}
	defer func() {
		var unlocked bool
		_ = conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, key).Scan(&unlocked)
	}()

	return fn(context.WithValue(ctx, advisoryLockConnectionContextKey{}, conn))
}
