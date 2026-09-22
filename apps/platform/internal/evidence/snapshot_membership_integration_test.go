package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

func TestCreateSnapshotFinalizesAndFreezesMembership(t *testing.T) {
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

	workspaceID := uuid.New()
	objectID := uuid.New()
	first := appendSnapshotTestEvidence(t, ctx, pool, workspaceID, "FIRST")
	second := appendSnapshotTestEvidence(t, ctx, pool, workspaceID, "SECOND")
	third := appendSnapshotTestEvidence(t, ctx, pool, workspaceID, "THIRD")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin snapshot transaction: %v", err)
	}
	snapshot, err := CreateSnapshot(ctx, tx, workspaceID, "DATASET_VERSION", objectID, map[string]any{
		"purpose": "membership-freeze-test",
	}, []SnapshotItem{
		{EvidenceID: first.ID, Category: "QUALITY"},
		{EvidenceID: second.ID, Category: "RIGHTS"},
	}, nil)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create evidence snapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit evidence snapshot: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM evidence_snapshot WHERE id=$1`, snapshot.ID).Scan(&status); err != nil {
		t.Fatalf("read snapshot status: %v", err)
	}
	if status != "FINALIZED" {
		t.Fatalf("snapshot status = %s, want FINALIZED", status)
	}

	view, err := NewQueryRepository(pool).GetSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("query finalized snapshot: %v", err)
	}
	if !view.IntegrityValid || len(view.Items) != 2 {
		t.Fatalf("snapshot integrity/items = %v/%d, want true/2", view.IntegrityValid, len(view.Items))
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO evidence_snapshot_item (snapshot_id, evidence_id, category)
		VALUES ($1,$2,'TRACE')
	`, snapshot.ID, third.ID); err == nil || !strings.Contains(err.Error(), "membership is finalized") {
		t.Fatalf("post-finalize INSERT error = %v, want membership finalized", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE evidence_snapshot_item
		SET category='TRACE'
		WHERE snapshot_id=$1 AND evidence_id=$2
	`, snapshot.ID, first.ID); err == nil || !strings.Contains(err.Error(), "membership is immutable") {
		t.Fatalf("post-finalize UPDATE error = %v, want membership immutable", err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM evidence_snapshot_item
		WHERE snapshot_id=$1 AND evidence_id=$2
	`, snapshot.ID, first.ID); err == nil || !strings.Contains(err.Error(), "membership is immutable") {
		t.Fatalf("post-finalize DELETE error = %v, want membership immutable", err)
	}

	var itemCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_snapshot_item WHERE snapshot_id=$1`, snapshot.ID).Scan(&itemCount); err != nil {
		t.Fatalf("count frozen membership: %v", err)
	}
	if itemCount != 2 {
		t.Fatalf("frozen membership count = %d, want 2", itemCount)
	}
}

func TestCreateSnapshotAllowsEmptyMembership(t *testing.T) {
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

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin snapshot transaction: %v", err)
	}
	snapshot, err := CreateSnapshot(ctx, tx, uuid.New(), "DATASET_VERSION", uuid.New(), map[string]any{
		"purpose": "empty-membership-test",
	}, nil, nil)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create empty evidence snapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit empty evidence snapshot: %v", err)
	}

	view, err := NewQueryRepository(pool).GetSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("query empty finalized snapshot: %v", err)
	}
	if !view.IntegrityValid {
		t.Fatal("empty EvidenceSnapshot integrity is invalid")
	}
	if view.Items == nil || len(view.Items) != 0 {
		t.Fatalf("empty EvidenceSnapshot items = %#v, want non-nil empty slice", view.Items)
	}
	manifestItems, ok := view.Manifest["evidenceItems"].([]any)
	if !ok || len(manifestItems) != 0 {
		t.Fatalf("empty EvidenceSnapshot manifest membership = %#v, want []", view.Manifest["evidenceItems"])
	}
}

func TestVerifySnapshotIntegrityNormalizesEquivalentJSONNumbers(t *testing.T) {
	hashPayload := []byte(`{"evidenceItems":[],"threshold":1e-7}`)
	digest := sha256.Sum256(hashPayload)
	rootHash := hex.EncodeToString(digest[:])

	valid, err := verifySnapshotIntegrity(rootHash, hashPayload, map[string]any{
		"evidenceItems": []SnapshotItem{},
		"threshold":     json.Number("0.0000001"),
	})
	if err != nil {
		t.Fatalf("verify equivalent numeric snapshot JSON: %v", err)
	}
	if !valid {
		t.Fatal("equivalent JSON number spellings invalidated snapshot integrity")
	}
}

func appendSnapshotTestEvidence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workspaceID uuid.UUID, label string) Record {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin evidence transaction: %v", err)
	}
	record, err := Append(ctx, tx, Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "SNAPSHOT_MEMBERSHIP_TEST",
		Title:        "Snapshot membership " + label,
		SourceType:   "TEST",
		Metadata:     map[string]any{"label": label},
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append evidence %s: %v", label, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit evidence %s: %v", label, err)
	}
	return record
}
