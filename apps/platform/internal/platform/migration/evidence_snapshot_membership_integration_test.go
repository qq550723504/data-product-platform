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

func TestEvidenceSnapshotBuildingCannotCommit(t *testing.T) {
	pool := scratchDatabase(t, 30)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin BUILDING snapshot transaction: %v", err)
	}
	snapshotID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO evidence_snapshot (
			id, workspace_id, object_type, object_id, manifest, root_hash, status
		) VALUES ($1,$2,'DATASET_VERSION',$3,'{}'::jsonb,$4,'BUILDING')
	`, snapshotID, uuid.New(), uuid.New(), strings.Repeat("a", 64)); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert BUILDING snapshot: %v", err)
	}
	err = tx.Commit(ctx)
	if err == nil {
		t.Fatal("BUILDING EvidenceSnapshot unexpectedly committed")
	}
	if !strings.Contains(err.Error(), "must be FINALIZED before commit") {
		t.Fatalf("BUILDING commit error = %v, want FINALIZED guard", err)
	}
}

func TestEvidenceSnapshotMembershipParentLockSerializesFinalize(t *testing.T) {
	pool := scratchDatabase(t, 30)
	ctx := context.Background()

	workspaceID := uuid.New()
	snapshotID := uuid.New()
	firstEvidenceID := insertSnapshotMigrationEvidence(t, pool, workspaceID, "first")
	secondEvidenceID := insertSnapshotMigrationEvidence(t, pool, workspaceID, "second")

	// BUILDING snapshots are intentionally uncommittable in production. Disable
	// only the deferred commit guard in this isolated scratch database so the
	// test can construct the otherwise-unobservable intermediate state and prove
	// the parent-row lock semantics directly.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE evidence_snapshot
		DISABLE TRIGGER trg_evidence_snapshot_finalized_on_commit
	`); err != nil {
		t.Fatalf("disable deferred snapshot guard: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO evidence_snapshot (
			id, workspace_id, object_type, object_id, manifest, root_hash, status
		) VALUES ($1,$2,'DATASET_VERSION',$3,'{}'::jsonb,$4,'BUILDING')
	`, snapshotID, workspaceID, uuid.New(), strings.Repeat("b", 64)); err != nil {
		t.Fatalf("insert committed BUILDING snapshot fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		ALTER TABLE evidence_snapshot
		ENABLE TRIGGER trg_evidence_snapshot_finalized_on_commit
	`); err != nil {
		t.Fatalf("re-enable deferred snapshot guard: %v", err)
	}

	itemTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin item transaction: %v", err)
	}
	if _, err := itemTx.Exec(ctx, `
		INSERT INTO evidence_snapshot_item (snapshot_id, evidence_id, category)
		VALUES ($1,$2,'QUALITY')
	`, snapshotID, firstEvidenceID); err != nil {
		_ = itemTx.Rollback(ctx)
		t.Fatalf("insert BUILDING snapshot item: %v", err)
	}

	finalizeTx, err := pool.Begin(ctx)
	if err != nil {
		_ = itemTx.Rollback(ctx)
		t.Fatalf("begin finalize transaction: %v", err)
	}
	if _, err := finalizeTx.Exec(ctx, `SET LOCAL lock_timeout = '100ms'`); err != nil {
		_ = itemTx.Rollback(ctx)
		_ = finalizeTx.Rollback(ctx)
		t.Fatalf("set finalize lock timeout: %v", err)
	}
	_, err = finalizeTx.Exec(ctx, `
		UPDATE evidence_snapshot
		SET status='FINALIZED'
		WHERE id=$1 AND status='BUILDING'
	`, snapshotID)
	if err == nil {
		_ = itemTx.Rollback(ctx)
		_ = finalizeTx.Rollback(ctx)
		t.Fatal("finalize did not wait for in-flight membership insert")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		_ = itemTx.Rollback(ctx)
		_ = finalizeTx.Rollback(ctx)
		t.Fatalf("finalize lock error = %v, want PostgreSQL lock timeout 55P03", err)
	}
	_ = finalizeTx.Rollback(ctx)

	if err := itemTx.Commit(ctx); err != nil {
		t.Fatalf("commit membership insert: %v", err)
	}
	tag, err := pool.Exec(ctx, `
		UPDATE evidence_snapshot
		SET status='FINALIZED'
		WHERE id=$1 AND status='BUILDING'
	`, snapshotID)
	if err != nil {
		t.Fatalf("finalize snapshot after item commit: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("finalize rows = %d, want 1", tag.RowsAffected())
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO evidence_snapshot_item (snapshot_id, evidence_id, category)
		VALUES ($1,$2,'RIGHTS')
	`, snapshotID, secondEvidenceID); err == nil || !strings.Contains(err.Error(), "membership is finalized") {
		t.Fatalf("post-finalize membership insert error = %v, want finalized refusal", err)
	}
}

func insertSnapshotMigrationEvidence(t *testing.T, pool *pgxpool.Pool, workspaceID uuid.UUID, label string) uuid.UUID {
	t.Helper()
	evidenceID := uuid.New()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO evidence (
			id, workspace_id, evidence_type, title, source_type,
			hash_algorithm, hash_value, metadata
		) VALUES ($1,$2,'SNAPSHOT_MEMBERSHIP_TEST',$3,'TEST','SHA256-EVIDENCE-V2',$4,'{}'::jsonb)
	`, evidenceID, workspaceID, label, strings.Repeat("c", 64)); err != nil {
		t.Fatalf("insert evidence %s: %v", label, err)
	}
	return evidenceID
}
