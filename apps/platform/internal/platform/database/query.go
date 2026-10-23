package database

import (
	"context"
	"github.com/jackc/pgx/v5"
)

// Queryer lets read repositories share one read-only PostgreSQL snapshot.
// Both pgx.Tx and pgxpool.Pool implement this existing driver contract.
type Queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
