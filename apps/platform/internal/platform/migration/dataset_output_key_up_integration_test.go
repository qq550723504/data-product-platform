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

func insertCuratedDataset(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	datasetID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'C2-a output-key dataset','CURATED')
	`, datasetID, uuid.New(), "C2A-MIGRATION-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	return datasetID
}

func insertDatasetVersion(t *testing.T, pool *pgxpool.Pool, datasetID uuid.UUID, versionNo int64, executionID *uuid.UUID, status string) {
	t.Helper()
	var storageURI, checksum *string
	if status == "READY" || status == "INVALID" || status == "SUPERSEDED" {
		uri := "s3://c2a-output-key/fixture.csv"
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

func TestC2AOutputKeyMigrationInstallsCurrentContract(t *testing.T) {
	pool := scratchDatabase(t, 19)
	ctx := context.Background()

	datasetID := insertCuratedDataset(t, pool)
	executionID := uuid.New()
	insertDatasetVersion(t, pool, datasetID, 1, &executionID, "CREATED")

	// Non-execution versions are outside the producer-key contract.
	nullDatasetID := insertCuratedDataset(t, pool)
	insertDatasetVersion(t, pool, nullDatasetID, 1, nil, "CREATED")
	insertDatasetVersion(t, pool, nullDatasetID, 2, nil, "CREATED")

	if err := tryApplyMigrationFile(t, pool, 20, "up"); err != nil {
		t.Fatalf("apply C2-a migration: %v", err)
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (id, dataset_id, version_no, status, metadata, generated_by_execution_id)
		VALUES ($1,$2,2,'CREATED','{}'::jsonb,$3)
	`, uuid.New(), datasetID, executionID)
	if err == nil {
		t.Fatal("C2-a index accepted a second live output version for one execution")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_dataset_version_execution_output" {
		t.Fatalf("live duplicate error = %v, want uq_dataset_version_execution_output 23505", err)
	}
}

func TestC2AOutputKeyConstraintScopesOnlyLiveExecutionOutputs(t *testing.T) {
	pool := scratchDatabase(t, 20)

	datasetID := insertCuratedDataset(t, pool)
	executionID := uuid.New()

	// Terminal history and failed half-products do not consume the live slot.
	insertDatasetVersion(t, pool, datasetID, 1, &executionID, "SUPERSEDED")
	insertDatasetVersion(t, pool, datasetID, 2, &executionID, "INVALID")
	insertDatasetVersion(t, pool, datasetID, 3, &executionID, "FAILED")
	insertDatasetVersion(t, pool, datasetID, 4, &executionID, "READY")
	insertDatasetVersion(t, pool, datasetID, 5, &executionID, "FAILED")

	_, err := pool.Exec(context.Background(), `
		INSERT INTO dataset_version (id, dataset_id, version_no, status, metadata, generated_by_execution_id)
		VALUES ($1,$2,6,'PROCESSING','{}'::jsonb,$3)
	`, uuid.New(), datasetID, executionID)
	if err == nil {
		t.Fatal("C2-a index accepted a second live output beside READY")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" || pgErr.ConstraintName != "uq_dataset_version_execution_output" {
		t.Fatalf("live duplicate error = %v, want uq_dataset_version_execution_output 23505", err)
	}
}

func TestC2AOutputKeyMigrationDownDropsOnlyConstraint(t *testing.T) {
	pool := scratchDatabase(t, 20)
	ctx := context.Background()

	datasetID := insertCuratedDataset(t, pool)
	executionID := uuid.New()
	insertDatasetVersion(t, pool, datasetID, 1, &executionID, "CREATED")

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

	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, datasetID).Scan(&versions); err != nil {
		t.Fatalf("count dataset versions after down: %v", err)
	}
	if versions != 1 {
		t.Fatalf("dataset versions after down = %d, want 1", versions)
	}
}
