package migration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type File struct {
	Version int64
	Name    string
	Path    string
}

type Runner struct {
	pool *pgxpool.Pool
	dir  string
}

func NewRunner(pool *pgxpool.Pool, dir string) *Runner {
	return &Runner{pool: pool, dir: dir}
}

func (r *Runner) Up(ctx context.Context) error {
	if err := r.ensureTable(ctx); err != nil {
		return err
	}

	files, err := r.files(".up.sql")
	if err != nil {
		return err
	}

	for _, file := range files {
		applied, err := r.isApplied(ctx, file.Version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := r.apply(ctx, file, true); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) Down(ctx context.Context) error {
	if err := r.ensureTable(ctx); err != nil {
		return err
	}

	var version int64
	err := r.pool.QueryRow(ctx, `SELECT version FROM schema_migration ORDER BY version DESC LIMIT 1`).Scan(&version)
	if err == pgx.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read latest migration: %w", err)
	}

	files, err := r.files(".down.sql")
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.Version == version {
			return r.apply(ctx, file, false)
		}
	}
	return fmt.Errorf("down migration for version %d not found", version)
}

func (r *Runner) ensureTable(ctx context.Context) error {
	_, err := r.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migration (
			version bigint PRIMARY KEY,
			name text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("ensure schema_migration table: %w", err)
	}
	return nil
}

func (r *Runner) isApplied(ctx context.Context, version int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migration WHERE version = $1)`, version).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return exists, nil
}

func (r *Runner) apply(ctx context.Context, file File, up bool) error {
	sqlBytes, err := os.ReadFile(file.Path)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", file.Name, err)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	// Serialize migration writers within this database.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(810042001)`); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	// Migration files can contain multiple SQL statements, so force pgx simple protocol.
	if _, err := tx.Exec(ctx, string(sqlBytes), pgx.QueryExecModeSimpleProtocol); err != nil {
		return fmt.Errorf("execute migration %s: %w", file.Name, err)
	}

	if up {
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migration(version, name) VALUES ($1, $2)`, file.Version, file.Name); err != nil {
			return fmt.Errorf("record migration %s: %w", file.Name, err)
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migration WHERE version = $1`, file.Version); err != nil {
			return fmt.Errorf("remove migration %s: %w", file.Name, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", file.Name, err)
	}
	return nil
}

func (r *Runner) files(suffix string) ([]File, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory %q: %w", r.dir, err)
	}

	files := make([]File, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse migration version from %q: %w", entry.Name(), err)
		}
		files = append(files, File{
			Version: version,
			Name:    entry.Name(),
			Path:    filepath.Join(r.dir, entry.Name()),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	return files, nil
}
