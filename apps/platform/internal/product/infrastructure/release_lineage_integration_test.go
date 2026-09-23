package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestPublishedReleaseLineagePublishFirstFreezesHistory(t *testing.T) {
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

	release, firstInputID := insertReleaseMembershipFixture(t, ctx, pool)
	secondInputID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_uri, checksum_algorithm, checksum_value, metadata, ready_at
		)
		SELECT $1, dataset_id, 3, 'READY', $2, 'SHA256', $3, '{}'::jsonb, now()
		FROM dataset_version WHERE id=$4
	`, secondInputID, "s3://release-lineage/"+secondInputID.String(), strings.Repeat("c", 64), firstInputID); err != nil {
		t.Fatalf("insert second lineage input: %v", err)
	}
	outputID := release.Datasets[0].DatasetVersionID
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'DERIVED_FROM')
	`, outputID, firstInputID); err != nil {
		t.Fatalf("insert initial lineage edge: %v", err)
	}

	repo := infrastructure.NewPostgresRepository(pool)
	workspaceID := releaseWorkspaceID(t, ctx, pool, release.ProductID)
	publishTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin publish tx: %v", err)
	}
	defer publishTx.Rollback(ctx)
	if _, err := deliveryfence.Lock(ctx, publishTx, workspaceID); err != nil {
		t.Fatalf("lock publish workspace fence: %v", err)
	}
	if err := repo.LockReleaseMembershipForPublish(ctx, publishTx, release); err != nil {
		t.Fatalf("lock release membership: %v", err)
	}
	if err := repo.LockReleaseLineageForPublish(ctx, publishTx, release.ID); err != nil {
		t.Fatalf("lock release lineage: %v", err)
	}

	mutationTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lineage mutation tx: %v", err)
	}
	defer mutationTx.Rollback(ctx)
	if _, err := mutationTx.Exec(ctx, "SET LOCAL lock_timeout = '100ms'"); err != nil {
		t.Fatalf("set mutation lock timeout: %v", err)
	}
	_, err = mutationTx.Exec(ctx, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'DERIVED_FROM')
	`, outputID, secondInputID)
	if !isLockTimeout(err) {
		t.Fatalf("concurrent lineage INSERT error = %v, want lock timeout while publish owns lineage closure", err)
	}
	if err := mutationTx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback lineage mutation tx: %v", err)
	}

	if _, err := publishTx.Exec(ctx, `
		UPDATE product_release SET status='PUBLISHED', released_at=now()
		WHERE id=$1 AND status='READY'
	`, release.ID); err != nil {
		t.Fatalf("publish release: %v", err)
	}
	if err := publishTx.Commit(ctx); err != nil {
		t.Fatalf("commit publish tx: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'DERIVED_FROM')
	`, outputID, secondInputID); err == nil || !strings.Contains(err.Error(), "published release history is frozen") {
		t.Fatalf("post-publish lineage INSERT error = %v, want frozen history", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE dataset_version_lineage SET relation_type='SOURCE_OF'
		WHERE output_version_id=$1 AND input_version_id=$2 AND relation_type='DERIVED_FROM'
	`, outputID, firstInputID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("post-publish lineage UPDATE error = %v, want append-only rejection", err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM dataset_version_lineage
		WHERE output_version_id=$1 AND input_version_id=$2 AND relation_type='DERIVED_FROM'
	`, outputID, firstInputID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("post-publish lineage DELETE error = %v, want append-only rejection", err)
	}

	replayTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lineage replay tx: %v", err)
	}
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	if err := datasetRepo.AddLineage(ctx, replayTx, outputID, firstInputID, "DERIVED_FROM", nil); err != nil {
		_ = replayTx.Rollback(ctx)
		t.Fatalf("idempotent published lineage replay: %v", err)
	}
	if err := replayTx.Commit(ctx); err != nil {
		t.Fatalf("commit idempotent lineage replay: %v", err)
	}

	var edges int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM dataset_version_lineage
		WHERE output_version_id=$1
	`, outputID).Scan(&edges); err != nil {
		t.Fatalf("count frozen lineage edges: %v", err)
	}
	if edges != 1 {
		t.Fatalf("frozen lineage edges = %d, want 1", edges)
	}
}

func TestPublishedReleaseLineageMutationFirstSerializesPublish(t *testing.T) {
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

	release, inputID := insertReleaseMembershipFixture(t, ctx, pool)
	outputID := release.Datasets[0].DatasetVersionID
	repo := infrastructure.NewPostgresRepository(pool)
	workspaceID := releaseWorkspaceID(t, ctx, pool, release.ProductID)

	mutationTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lineage mutation tx: %v", err)
	}
	defer mutationTx.Rollback(ctx)
	if _, err := mutationTx.Exec(ctx, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'DERIVED_FROM')
	`, outputID, inputID); err != nil {
		t.Fatalf("insert in-flight lineage edge: %v", err)
	}

	type publishOutcome struct {
		closureCount int
		err          error
	}
	started := make(chan struct{})
	done := make(chan publishOutcome, 1)
	go func() {
		publishTx, err := pool.Begin(ctx)
		if err != nil {
			done <- publishOutcome{err: err}
			return
		}
		defer publishTx.Rollback(ctx)
		close(started)

		// This blocks on the workspace delivery fence held by the lineage INSERT.
		// The release and lineage closure are read only after the shared fence is
		// acquired, so the resumed publisher must see the committed mutation.
		if _, err := deliveryfence.Lock(ctx, publishTx, workspaceID); err != nil {
			done <- publishOutcome{err: err}
			return
		}
		if err := repo.LockReleaseMembershipForPublish(ctx, publishTx, release); err != nil {
			done <- publishOutcome{err: err}
			return
		}
		if err := repo.LockReleaseLineageForPublish(ctx, publishTx, release.ID); err != nil {
			done <- publishOutcome{err: err}
			return
		}

		var closureCount int
		if err := publishTx.QueryRow(ctx, `
			WITH RECURSIVE lineage(version_id) AS (
				SELECT dataset_version_id FROM product_release_dataset WHERE release_id=$1
				UNION
				SELECT dvl.input_version_id
				FROM dataset_version_lineage dvl
				JOIN lineage l ON l.version_id=dvl.output_version_id
			)
			SELECT count(*) FROM lineage
		`, release.ID).Scan(&closureCount); err != nil {
			done <- publishOutcome{err: err}
			return
		}
		done <- publishOutcome{closureCount: closureCount}
	}()

	<-started
	select {
	case outcome := <-done:
		t.Fatalf("publish did not block behind lineage mutation: %+v", outcome)
	case <-time.After(150 * time.Millisecond):
		// Expected: publish is waiting on the shared workspace delivery fence.
	}

	if err := mutationTx.Commit(ctx); err != nil {
		t.Fatalf("commit lineage mutation: %v", err)
	}

	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("publish after lineage commit: %v", outcome.err)
		}
		if outcome.closureCount != 2 {
			t.Fatalf("lineage closure count after blocked publish resumed = %d, want 2", outcome.closureCount)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for blocked publish to resume")
	}
}


func TestDatasetVersionLineageRejectsCrossWorkspaceEdge(t *testing.T) {
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

	release, _ := insertReleaseMembershipFixture(t, ctx, pool)
	outputID := release.Datasets[0].DatasetVersionID
	foreignWorkspaceID := uuid.New()
	foreignDatasetID := uuid.New()
	foreignVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Foreign lineage dataset','RAW')
	`, foreignDatasetID, foreignWorkspaceID, "LINEAGE-FOREIGN-"+uuid.NewString()); err != nil {
		t.Fatalf("insert foreign dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_uri,
			checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY',$3,'SHA256',$4,'{}'::jsonb,now())
	`, foreignVersionID, foreignDatasetID, "s3://foreign-lineage/"+foreignVersionID.String(), strings.Repeat("f", 64)); err != nil {
		t.Fatalf("insert foreign dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'DERIVED_FROM')
	`, outputID, foreignVersionID); err == nil || !strings.Contains(err.Error(), "crosses workspace boundary") {
		t.Fatalf("cross-workspace lineage error = %v, want workspace-boundary rejection", err)
	}
}

func releaseWorkspaceID(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, productID uuid.UUID) uuid.UUID {
	t.Helper()
	var workspaceID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT workspace_id FROM data_product WHERE id=$1
	`, productID).Scan(&workspaceID); err != nil {
		t.Fatalf("resolve release workspace: %v", err)
	}
	return workspaceID
}
