package infrastructure_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestEvaluateRightsCoverageAcrossDatasetLineage(t *testing.T) {
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
	resourceA := uuid.New()
	resourceB := uuid.New()
	insertResource(t, ctx, pool, resourceA, workspaceID, "RIGHTS-A-"+uuid.NewString())
	insertResource(t, ctx, pool, resourceB, workspaceID, "RIGHTS-B-"+uuid.NewString())

	rawDatasetA := uuid.New()
	rawDatasetB := uuid.New()
	standardizedDataset := uuid.New()
	curatedDataset := uuid.New()
	insertDataset(t, ctx, pool, rawDatasetA, workspaceID, "RAW-A-"+uuid.NewString(), "RAW", &resourceA)
	insertDataset(t, ctx, pool, rawDatasetB, workspaceID, "RAW-B-"+uuid.NewString(), "RAW", &resourceB)
	insertDataset(t, ctx, pool, standardizedDataset, workspaceID, "STD-"+uuid.NewString(), "STANDARDIZED", nil)
	insertDataset(t, ctx, pool, curatedDataset, workspaceID, "CURATED-"+uuid.NewString(), "CURATED", nil)

	rawVersionA := uuid.New()
	rawVersionB := uuid.New()
	standardizedVersion := uuid.New()
	curatedVersion := uuid.New()
	insertDatasetVersion(t, ctx, pool, rawVersionA, rawDatasetA)
	insertDatasetVersion(t, ctx, pool, rawVersionB, rawDatasetB)
	insertDatasetVersion(t, ctx, pool, standardizedVersion, standardizedDataset)
	insertDatasetVersion(t, ctx, pool, curatedVersion, curatedDataset)
	insertLineage(t, ctx, pool, standardizedVersion, rawVersionA)
	insertLineage(t, ctx, pool, standardizedVersion, rawVersionB)
	insertLineage(t, ctx, pool, curatedVersion, standardizedVersion)

	repo := infrastructure.NewPostgresRepository(pool)
	now := time.Now().UTC()
	allActions := []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}

	missingResourceSnapshot := createRightsFixture(t, ctx, pool, workspaceID, map[uuid.UUID][]string{
		resourceA: allActions,
	})
	coverage, err := repo.EvaluateRightsCoverage(ctx, missingResourceSnapshot, []uuid.UUID{curatedVersion}, now)
	if err != nil {
		t.Fatalf("evaluate missing-resource coverage: %v", err)
	}
	if !coverage.Known || coverage.Complete || len(coverage.RequiredResources) != 2 || len(coverage.MissingResources) != 1 || coverage.MissingResources[0] != resourceB {
		t.Fatalf("missing-resource coverage = %+v", coverage)
	}

	missingActionSnapshot := createRightsFixture(t, ctx, pool, workspaceID, map[uuid.UUID][]string{
		resourceA: allActions,
		resourceB: {"READ", "AGGREGATE", "DERIVE"},
	})
	coverage, err = repo.EvaluateRightsCoverage(ctx, missingActionSnapshot, []uuid.UUID{curatedVersion}, now)
	if err != nil {
		t.Fatalf("evaluate missing-action coverage: %v", err)
	}
	missingActions := coverage.MissingActions[resourceB.String()]
	if coverage.Complete || len(missingActions) != 1 || missingActions[0] != "PRODUCTIZE" {
		t.Fatalf("missing-action coverage = %+v", coverage)
	}

	completeSnapshot := createRightsFixture(t, ctx, pool, workspaceID, map[uuid.UUID][]string{
		resourceA: allActions,
		resourceB: allActions,
	})
	coverage, err = repo.EvaluateRightsCoverage(ctx, completeSnapshot, []uuid.UUID{curatedVersion}, now)
	if err != nil {
		t.Fatalf("evaluate complete coverage: %v", err)
	}
	if !coverage.Known || !coverage.Complete || len(coverage.MissingResources) != 0 || len(coverage.MissingActions) != 0 {
		t.Fatalf("complete coverage = %+v", coverage)
	}
}

func insertResource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, workspaceID uuid.UUID, code string) {
	t.Helper()
	mustExec(t, ctx, pool, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type)
		VALUES ($1,$2,$3,$3,'TABLE_LIKE')
	`, id, workspaceID, code)
}

func insertDataset(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, workspaceID uuid.UUID, code, datasetType string, sourceResourceID *uuid.UUID) {
	t.Helper()
	mustExec(t, ctx, pool, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, source_resource_id)
		VALUES ($1,$2,$3,$3,$4,$5)
	`, id, workspaceID, code, datasetType, sourceResourceID)
}

func insertDatasetVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, datasetID uuid.UUID) {
	t.Helper()
	mustExec(t, ctx, pool, `
		INSERT INTO dataset_version (id, dataset_id, version_no, status, metadata)
		VALUES ($1,$2,1,'CREATED','{}'::jsonb)
	`, id, datasetID)
}

func insertLineage(t *testing.T, ctx context.Context, pool *pgxpool.Pool, outputID, inputID uuid.UUID) {
	t.Helper()
	mustExec(t, ctx, pool, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'DERIVED_FROM')
	`, outputID, inputID)
}

func createRightsFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workspaceID uuid.UUID, grants map[uuid.UUID][]string) uuid.UUID {
	t.Helper()
	authorizationID := uuid.New()
	snapshotID := uuid.New()
	mustExec(t, ctx, pool, `
		INSERT INTO data_authorization (
			id, workspace_id, code, grantor_ref, grantee_ref, purpose, status,
			valid_from, valid_to, metadata, created_at, updated_at
		) VALUES ($1,$2,$3,'PARK-OPERATOR','DATA-PRODUCT-PLATFORM','ENTERPRISE_CREDIT_RISK_SUPPORT','ACTIVE',
		          now()-interval '1 hour',now()+interval '1 hour','{}'::jsonb,now(),now())
	`, authorizationID, workspaceID, "RIGHTS-AUTH-"+uuid.NewString())
	for resourceID, actions := range grants {
		mustExec(t, ctx, pool, `
			INSERT INTO authorization_resource (
				id, authorization_id, data_resource_id, actions, scope, raw_export_allowed, created_at
			) VALUES ($1,$2,$3,$4,'{}'::jsonb,false,now())
		`, uuid.New(), authorizationID, resourceID, actions)
	}
	mustExec(t, ctx, pool, `
		INSERT INTO rights_snapshot (
			id, workspace_id, purpose, consumer_ref, as_of, manifest, root_hash, created_at
		) VALUES ($1,$2,'ENTERPRISE_CREDIT_RISK_SUPPORT','LICENSED_BANK',now(),
		          '{"purpose":"ENTERPRISE_CREDIT_RISK_SUPPORT","authorizations":[]}'::jsonb,
		          'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',now())
	`, snapshotID, workspaceID)
	mustExec(t, ctx, pool, `
		INSERT INTO rights_snapshot_authorization (rights_snapshot_id, authorization_id)
		VALUES ($1,$2)
	`, snapshotID, authorizationID)
	return snapshotID
}

func mustExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("execute fixture SQL: %v", err)
	}
}
