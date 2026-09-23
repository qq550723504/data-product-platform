package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestProductReleaseDatasetMembershipPublishFirstFreezesMutations(t *testing.T) {
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

	release, secondVersionID := insertReleaseMembershipFixture(t, ctx, pool)
	repo := infrastructure.NewPostgresRepository(pool)

	publishTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin publish tx: %v", err)
	}
	defer publishTx.Rollback(ctx)
	if err := repo.LockReleaseMembershipForPublish(ctx, publishTx, release); err != nil {
		t.Fatalf("lock release membership: %v", err)
	}

	mutationTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin mutation tx: %v", err)
	}
	defer mutationTx.Rollback(ctx)
	if _, err := mutationTx.Exec(ctx, "SET LOCAL lock_timeout = '100ms'"); err != nil {
		t.Fatalf("set mutation lock timeout: %v", err)
	}
	_, err = mutationTx.Exec(ctx, `
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($1,$2,'SUPPORTING')
	`, release.ID, secondVersionID)
	if !isLockTimeout(err) {
		t.Fatalf("concurrent membership INSERT error = %v, want lock timeout while publish owns parent row", err)
	}
	if err := mutationTx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback mutation tx: %v", err)
	}

	if err := repo.PermitReleasePublish(ctx, publishTx, release.ID); err != nil {
		t.Fatalf("permit release publish: %v", err)
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
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($1,$2,'SUPPORTING')
	`, release.ID, secondVersionID); err == nil || !strings.Contains(err.Error(), "dataset membership is frozen") {
		t.Fatalf("post-publish INSERT error = %v, want frozen membership", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE product_release_dataset SET role='OUTPUT'
		WHERE release_id=$1 AND dataset_version_id=$2 AND role='PRIMARY'
	`, release.ID, release.Datasets[0].DatasetVersionID); err == nil || !strings.Contains(err.Error(), "dataset membership is frozen") {
		t.Fatalf("post-publish UPDATE error = %v, want frozen membership", err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM product_release_dataset
		WHERE release_id=$1 AND dataset_version_id=$2 AND role='PRIMARY'
	`, release.ID, release.Datasets[0].DatasetVersionID); err == nil || !strings.Contains(err.Error(), "dataset membership is frozen") {
		t.Fatalf("post-publish DELETE error = %v, want frozen membership", err)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM product_release_dataset WHERE release_id=$1`, release.ID).Scan(&count); err != nil {
		t.Fatalf("count frozen membership: %v", err)
	}
	if count != 1 {
		t.Fatalf("frozen membership count = %d, want 1", count)
	}
}

func TestProductReleaseDatasetMembershipMutationFirstInvalidatesStalePublishView(t *testing.T) {
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

	release, secondVersionID := insertReleaseMembershipFixture(t, ctx, pool)
	repo := infrastructure.NewPostgresRepository(pool)

	mutationTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin mutation tx: %v", err)
	}
	defer mutationTx.Rollback(ctx)
	if _, err := mutationTx.Exec(ctx, `
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($1,$2,'SUPPORTING')
	`, release.ID, secondVersionID); err != nil {
		t.Fatalf("insert in-flight membership: %v", err)
	}

	publishTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin publish tx: %v", err)
	}
	defer publishTx.Rollback(ctx)
	if _, err := publishTx.Exec(ctx, "SET LOCAL lock_timeout = '100ms'"); err != nil {
		t.Fatalf("set publish lock timeout: %v", err)
	}
	if err := repo.LockReleaseMembershipForPublish(ctx, publishTx, release); !isLockTimeout(err) {
		t.Fatalf("publish lock error = %v, want lock timeout behind membership mutation", err)
	}
	if err := publishTx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("rollback publish tx: %v", err)
	}

	if err := mutationTx.Commit(ctx); err != nil {
		t.Fatalf("commit membership mutation: %v", err)
	}

	checkTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin membership check tx: %v", err)
	}
	defer checkTx.Rollback(ctx)
	err = repo.LockReleaseMembershipForPublish(ctx, checkTx, release)
	if err == nil || !strings.Contains(err.Error(), "membership changed before publish") {
		t.Fatalf("stale publish membership error = %v, want membership changed rejection", err)
	}
}

func insertReleaseMembershipFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (domain.ProductRelease, uuid.UUID) {
	t.Helper()
	workspaceID := uuid.New()
	productID := uuid.New()
	productVersionID := uuid.New()
	datasetID := uuid.New()
	firstVersionID := uuid.New()
	secondVersionID := uuid.New()
	releaseID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO data_product (id, workspace_id, code, name)
		VALUES ($1,$2,$3,'Release membership test product')
	`, productID, workspaceID, "REL-MEM-"+uuid.NewString()); err != nil {
		t.Fatalf("insert product: %v", err)
	}
	versionTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin product version fixture: %v", err)
	}
	if _, err := versionTx.Exec(ctx, `
		INSERT INTO product_version (
			id, product_id, major_version, minor_version, patch_version,
			build_status, expected_asset_count
		) VALUES ($1,$2,1,0,0,'BUILDING',0)
	`, productVersionID, productID); err != nil {
		_ = versionTx.Rollback(ctx)
		t.Fatalf("insert product version: %v", err)
	}
	if _, err := versionTx.Exec(ctx, `
		UPDATE product_version
		SET build_status='FINALIZED'
		WHERE id=$1 AND build_status='BUILDING'
	`, productVersionID); err != nil {
		_ = versionTx.Rollback(ctx)
		t.Fatalf("finalize product version: %v", err)
	}
	if err := versionTx.Commit(ctx); err != nil {
		t.Fatalf("commit product version fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Release membership dataset','CURATED')
	`, datasetID, workspaceID, "REL-MEM-DATASET-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	for index, versionID := range []uuid.UUID{firstVersionID, secondVersionID} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO dataset_version (
				id, dataset_id, version_no, status, storage_uri, checksum_algorithm, checksum_value, metadata, ready_at
			) VALUES ($1,$2,$3,'READY',$4,'SHA256',$5,'{}'::jsonb,now())
		`, versionID, datasetID, index+1, "s3://release-membership/"+versionID.String(), strings.Repeat(string(rune('a'+index)), 64)); err != nil {
			t.Fatalf("insert dataset version %d: %v", index+1, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_release (
			id, product_id, product_version_id, release_no, status, metadata
		) VALUES ($1,$2,$3,$4,'READY','{}'::jsonb)
	`, releaseID, productID, productVersionID, "R-"+uuid.NewString()); err != nil {
		t.Fatalf("insert release: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($1,$2,'PRIMARY')
	`, releaseID, firstVersionID); err != nil {
		t.Fatalf("insert release membership: %v", err)
	}

	return domain.ProductRelease{
		ID:               releaseID,
		ProductID:        productID,
		ProductVersionID: productVersionID,
		Status:           domain.ReleaseReady,
		Datasets: []domain.ReleaseDataset{
			{DatasetVersionID: firstVersionID, Role: domain.DatasetPrimary},
		},
	}, secondVersionID
}

func isLockTimeout(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "55P03"
}
