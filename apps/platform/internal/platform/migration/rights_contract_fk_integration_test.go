package migration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func insertLegacyProductGraph(
	t *testing.T,
	pool *pgxpool.Pool,
	versionContractID *uuid.UUID,
	releaseContractID *uuid.UUID,
	rightsSnapshotID *uuid.UUID,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	productID := uuid.New()
	versionID := uuid.New()
	releaseID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO data_product (id, workspace_id, code, name)
		VALUES ($1,$2,$3,'migration-281 product')
	`, productID, uuid.New(), "MIGRATION-281-"+uuid.NewString()); err != nil {
		t.Fatalf("insert legacy product: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_version (
			id, product_id, major_version, minor_version, patch_version, contract_version_id
		) VALUES ($1,$2,1,0,0,$3)
	`, versionID, productID, versionContractID); err != nil {
		t.Fatalf("insert legacy product version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_release (
			id, product_id, product_version_id, release_no, contract_version_id, rights_snapshot_id
		) VALUES ($1,$2,$3,'R1',$4,$5)
	`, releaseID, productID, versionID, releaseContractID, rightsSnapshotID); err != nil {
		t.Fatalf("insert legacy product release: %v", err)
	}
	return productID, versionID
}

func TestRightsContractMigrationStagesAndValidatesForeignKeysForExistingRows(t *testing.T) {
	pool := scratchDatabase(t, 5)
	ctx := context.Background()

	insertLegacyProductGraph(t, pool, nil, nil, nil)

	if err := tryApplyMigrationFile(t, pool, 6, "up"); err != nil {
		t.Fatalf("apply rights/contract migration with existing compatible rows: %v", err)
	}

	rows, err := pool.Query(ctx, `
		SELECT conname, convalidated
		FROM pg_constraint
		WHERE conname = ANY($1)
		ORDER BY conname
	`, []string{
		"fk_product_release_contract_version",
		"fk_product_release_rights_snapshot",
		"fk_product_version_contract_version",
	})
	if err != nil {
		t.Fatalf("query staged foreign keys: %v", err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var name string
		var validated bool
		if err := rows.Scan(&name, &validated); err != nil {
			t.Fatalf("scan staged foreign key: %v", err)
		}
		seen++
		if !validated {
			t.Fatalf("constraint %s remained NOT VALID after successful migration", name)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate staged foreign keys: %v", err)
	}
	if seen != 3 {
		t.Fatalf("validated foreign keys = %d, want 3", seen)
	}
}

func TestRightsContractMigrationEnforcesNewRowsBeforeHistoricalValidation(t *testing.T) {
	pool := scratchDatabase(t, 5)
	ctx := context.Background()

	historicalContractID := uuid.New()
	insertLegacyProductGraph(t, pool, &historicalContractID, nil, nil)

	migrationPath := filepath.Join(migrationsDir(t), "000006_rights_contract.up.sql")
	content, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read rights/contract migration: %v", err)
	}
	const validationMarker = "-- Validate staged foreign keys explicitly after installation."
	parts := strings.SplitN(string(content), validationMarker, 2)
	if len(parts) != 2 {
		t.Fatalf("migration is missing staged-validation marker %q", validationMarker)
	}
	if _, err := pool.Exec(ctx, parts[0]); err != nil {
		t.Fatalf("install NOT VALID foreign keys over historical rows: %v", err)
	}

	var validated bool
	if err := pool.QueryRow(ctx, `
		SELECT convalidated
		FROM pg_constraint
		WHERE conname='fk_product_version_contract_version'
	`).Scan(&validated); err != nil {
		t.Fatalf("read staged foreign key state: %v", err)
	}
	if validated {
		t.Fatal("staged foreign key was validated before the explicit validation boundary")
	}

	productID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_product (id, workspace_id, code, name)
		VALUES ($1,$2,$3,'migration-281 new product')
	`, productID, uuid.New(), "MIGRATION-281-NEW-"+uuid.NewString()); err != nil {
		t.Fatalf("insert product after staged constraint install: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO product_version (
			id, product_id, major_version, minor_version, patch_version, contract_version_id
		) VALUES ($1,$2,1,0,0,$3)
	`, uuid.New(), productID, uuid.New())
	if err == nil {
		t.Fatal("NOT VALID foreign key accepted a new invalid contract reference")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" || pgErr.ConstraintName != "fk_product_version_contract_version" {
		t.Fatalf("new invalid reference error = %v, want fk_product_version_contract_version 23503", err)
	}

	_, err = pool.Exec(ctx, `
		ALTER TABLE product_version
			VALIDATE CONSTRAINT fk_product_version_contract_version
	`)
	if err == nil {
		t.Fatal("historical invalid contract reference unexpectedly passed explicit validation")
	}
	pgErr = nil
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" || pgErr.ConstraintName != "fk_product_version_contract_version" {
		t.Fatalf("historical validation error = %v, want fk_product_version_contract_version 23503", err)
	}
}
