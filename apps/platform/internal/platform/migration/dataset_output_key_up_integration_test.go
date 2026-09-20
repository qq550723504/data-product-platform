package migration_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// insertCuratedDataset writes a dataset row for the C2-a guard fixtures.
func insertCuratedDataset(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	datasetID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'C2-a migration guard dataset','CURATED')
	`, datasetID, uuid.New(), "C2A-MIGRATION-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	return datasetID
}

// insertDatasetVersion writes one version row bound to an optional producing
// execution. status selects whether the row is part of the C2-a live output set.
// Terminal statuses need a storage reference, exactly as the platform writes them.
func insertDatasetVersion(t *testing.T, pool *pgxpool.Pool, datasetID uuid.UUID, versionNo int64, executionID *uuid.UUID, status string) {
	t.Helper()
	var storageURI, checksum *string
	if status == "READY" || status == "INVALID" || status == "SUPERSEDED" {
		uri := "s3://c2a-guard/fixture.csv"
		sum := strings.Repeat("a", 64)
		storageURI, checksum = &uri, &sum
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, metadata, generated_by_execution_id,
			storage_type, storage_uri, checksum_algorithm, checksum_value
		) VALUES ($1,$2,$3,$4,'{}'::jsonb,$5,'OBJECT_STORAGE',$6,'SHA256',$7)
	`, uuid.New(), datasetID, versionNo, status, executionID, storageURI, checksum); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
}

// TestC2AOutputKeyMigrationRefusesExistingDuplicateOutputs pins the refusal
// branch: an installation that already has two live output versions for one
// Execution must be brought back to one live row deliberately, not upgraded by
// silently dropping a fact.
func TestC2AOutputKeyMigrationRefusesExistingDuplicateOutputs(t *testing.T) {
	pool := scratchDatabase(t, 19)

	// The duplicates must predate the constraint, so they are inserted while the
	// partial unique index does not exist yet.
	datasetID := insertCuratedDataset(t, pool)
	executionID := uuid.New()
	insertDatasetVersion(t, pool, datasetID, 1, &executionID, "CREATED")
	insertDatasetVersion(t, pool, datasetID, 2, &executionID, "PROCESSING")

	// Multiple NULL executions are not duplicates and must not block the upgrade.
	nullDatasetID := insertCuratedDataset(t, pool)
	insertDatasetVersion(t, pool, nullDatasetID, 1, nil, "CREATED")
	insertDatasetVersion(t, pool, nullDatasetID, 2, nil, "CREATED")

	err := tryApplyMigrationFile(t, pool, 20, "up")
	if err == nil {
		t.Fatal("C2-a migration accepted a pre-existing duplicate output pair")
	}
	if !strings.Contains(err.Error(), "already have more than one live output version") {
		t.Fatalf("C2-a migration error = %v, want the duplicate-output refusal", err)
	}
	if !strings.Contains(err.Error(), "InvalidateDatasetVersion") {
		t.Fatalf("C2-a migration error = %v, want a remediation path in the message", err)
	}

	var indexName string
	if err := pool.QueryRow(context.Background(), `SELECT COALESCE(to_regclass('uq_dataset_version_execution_output')::text, '')`).Scan(&indexName); err != nil {
		t.Fatalf("check C2-a index: %v", err)
	}
	if indexName != "" {
		t.Fatalf("refused migration left index %q behind; it must roll back", indexName)
	}
}

// TestC2AOutputKeyMigrationUpgradesInstallationsWithHistoricalDuplicates is the
// operational half of the guard: terminal rows (INVALID/SUPERSEDED) can never be
// deleted or rewritten, so an index that covered them would be impossible to
// install. Historical duplicates must therefore not block the upgrade.
func TestC2AOutputKeyMigrationUpgradesInstallationsWithHistoricalDuplicates(t *testing.T) {
	pool := scratchDatabase(t, 19)
	ctx := context.Background()

	datasetID := insertCuratedDataset(t, pool)
	executionID := uuid.New()
	// A real shape from the pre-C2-a bug: the Execution output was superseded by
	// later uploads, so the pair has several historical rows and exactly one live one.
	insertDatasetVersion(t, pool, datasetID, 1, &executionID, "CREATED")
	insertDatasetVersion(t, pool, datasetID, 2, &executionID, "SUPERSEDED")
	insertDatasetVersion(t, pool, datasetID, 3, &executionID, "INVALID")

	if err := tryApplyMigrationFile(t, pool, 20, "up"); err != nil {
		t.Fatalf("C2-a migration refused an installation with historical duplicates: %v", err)
	}

	// The one live row is now constrained: a second live output is rejected.
	insertDatasetVersion(t, pool, datasetID, 4, &executionID, "SUPERSEDED")
	_, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (id, dataset_id, version_no, status, metadata, generated_by_execution_id)
		VALUES ($1,$2,5,'CREATED','{}'::jsonb,$3)
	`, uuid.New(), datasetID, executionID)
	if err == nil {
		t.Fatal("C2-a index accepted a second live output version for one execution")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_dataset_version_execution_output" {
		t.Fatalf("live duplicate error = %v, want uq_dataset_version_execution_output 23505", err)
	}
}

// TestC2AOutputKeyMigrationIsPartialAndReversible covers the properties the
// writer relies on: NULL executions keep working, live duplicates are rejected,
// and the downgrade removes only the constraint, never the rows.
func TestC2AOutputKeyMigrationIsPartialAndReversible(t *testing.T) {
	pool := scratchDatabase(t, 20)
	ctx := context.Background()

	// Non-execution versions and multiple NULL executions remain legal.
	nullDatasetID := insertCuratedDataset(t, pool)
	insertDatasetVersion(t, pool, nullDatasetID, 1, nil, "CREATED")
	insertDatasetVersion(t, pool, nullDatasetID, 2, nil, "CREATED")

	datasetID := insertCuratedDataset(t, pool)
	executionID := uuid.New()
	insertDatasetVersion(t, pool, datasetID, 1, &executionID, "CREATED")

	_, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (id, dataset_id, version_no, status, metadata, generated_by_execution_id)
		VALUES ($1,$2,2,'CREATED','{}'::jsonb,$3)
	`, uuid.New(), datasetID, executionID)
	if err == nil {
		t.Fatal("C2-a index accepted a second live output version for one execution")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate output error = %v, want 23505 unique violation", err)
	}
	if pgErr.ConstraintName != "uq_dataset_version_execution_output" {
		t.Fatalf("violated constraint = %q, want uq_dataset_version_execution_output", pgErr.ConstraintName)
	}

	if err := tryApplyMigrationFile(t, pool, 20, "down"); err != nil {
		t.Fatalf("C2-a down migration: %v", err)
	}
	var regclass string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(to_regclass('uq_dataset_version_execution_output')::text, '')`).Scan(&regclass); err != nil {
		t.Fatalf("check index after down: %v", err)
	}
	if regclass != "" {
		t.Fatalf("index %q survived the C2-a down migration", regclass)
	}
	// Historical facts survive a downgrade.
	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, datasetID).Scan(&versions); err != nil {
		t.Fatalf("count dataset versions after down: %v", err)
	}
	if versions != 1 {
		t.Fatalf("dataset versions after down = %d, want 1", versions)
	}
}
