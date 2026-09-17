package application_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	metadataapp "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

type fakeMetadataEngine struct {
	upsertCalls int
	failUpsert  bool
}

func (f *fakeMetadataEngine) GetAsset(_ context.Context, entityType, fullyQualifiedName string) (metadataengine.Asset, error) {
	return metadataengine.Asset{
		ID:                 "table-001",
		EntityType:         entityType,
		Name:               "orders",
		FullyQualifiedName: fullyQualifiedName,
		Metadata:           map[string]any{"serviceType": "PostgreSQL"},
	}, nil
}

func (f *fakeMetadataEngine) UpsertDataProduct(_ context.Context, product metadataengine.GovernanceProduct) (metadataengine.ExternalEntity, error) {
	f.upsertCalls++
	if f.failUpsert {
		return metadataengine.ExternalEntity{}, errors.New("OpenMetadata unavailable")
	}
	return metadataengine.ExternalEntity{
		ID:                 "om-product-001",
		EntityType:         "DATA_PRODUCT",
		FullyQualifiedName: product.Domain + "." + product.Name,
		Metadata:           map[string]any{"version": 0.1},
	}, nil
}

func TestBindingAndProductReleasedProjectionAreRetrySafe(t *testing.T) {
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
	resourceID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type)
		VALUES ($1,$2,$3,'Orders','TABLE_LIKE')
	`, resourceID, workspaceID, "RESOURCE-"+uuid.NewString()); err != nil {
		t.Fatalf("insert DataResource: %v", err)
	}

	productID := uuid.New()
	versionID := uuid.New()
	releaseID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_product (id, workspace_id, code, name, description)
		VALUES ($1,$2,$3,'企业经营活跃度','Reference product')
	`, productID, workspaceID, "DP-"+uuid.NewString()); err != nil {
		t.Fatalf("insert DataProduct: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_version (id, product_id, major_version, minor_version, patch_version, definition_snapshot)
		VALUES ($1,$2,1,0,0,'{}'::jsonb)
	`, versionID, productID); err != nil {
		t.Fatalf("insert ProductVersion: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_release (id, product_id, product_version_id, release_no, status, metadata, released_at)
		VALUES ($1,$2,$3,'R-OM-001','PUBLISHED','{}'::jsonb,now())
	`, releaseID, productID, versionID); err != nil {
		t.Fatalf("insert ProductRelease: %v", err)
	}

	txManager := transaction.NewManager(pool)
	metadataRepo := metadatainfra.NewPostgresRepository(pool)
	engine := &fakeMetadataEngine{}
	service := metadataapp.NewService(
		metadatadomain.ProviderOpenMetadata,
		"Park",
		txManager,
		metadataRepo,
		productinfra.NewPostgresRepository(pool),
		engine,
	)

	binding, err := service.BindResource(ctx, metadataapp.BindResourceCommand{
		ResourceID:         resourceID,
		EntityType:         "TABLE",
		FullyQualifiedName: "sample_data.ecommerce.public.orders",
		Primary:            true,
	})
	if err != nil {
		t.Fatalf("bind resource: %v", err)
	}
	if binding.ExternalID != "table-001" || binding.Provider != metadatadomain.ProviderOpenMetadata {
		t.Fatalf("binding = %+v", binding)
	}

	eventID := uuid.New()
	if err := service.ProjectProductRelease(ctx, eventID, releaseID); err != nil {
		t.Fatalf("project ProductRelease: %v", err)
	}
	if engine.upsertCalls != 1 {
		t.Fatalf("upsert calls = %d, want 1", engine.upsertCalls)
	}
	projection, err := metadataRepo.GetProjection(ctx, metadatadomain.ProviderOpenMetadata, "PRODUCT_RELEASE", releaseID)
	if err != nil {
		t.Fatalf("get projection: %v", err)
	}
	if projection.Status != metadatadomain.ProjectionSucceeded || projection.Attempts != 1 || projection.ExternalFQN == "" {
		t.Fatalf("projection = %+v, want SUCCEEDED attempt 1", projection)
	}

	if err := service.ProjectProductRelease(ctx, eventID, releaseID); err != nil {
		t.Fatalf("repeat successful projection: %v", err)
	}
	if engine.upsertCalls != 1 {
		t.Fatalf("repeat projection performed another external upsert: calls=%d", engine.upsertCalls)
	}

	failedReleaseID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO product_release (id, product_id, product_version_id, release_no, status, metadata, released_at)
		VALUES ($1,$2,$3,'R-OM-002','PUBLISHED','{}'::jsonb,now())
	`, failedReleaseID, productID, versionID); err != nil {
		t.Fatalf("insert retry ProductRelease: %v", err)
	}
	engine.failUpsert = true
	if err := service.ProjectProductRelease(ctx, uuid.New(), failedReleaseID); err == nil {
		t.Fatal("expected OpenMetadata projection failure")
	}
	failed, err := metadataRepo.GetProjection(ctx, metadatadomain.ProviderOpenMetadata, "PRODUCT_RELEASE", failedReleaseID)
	if err != nil {
		t.Fatalf("get failed projection: %v", err)
	}
	if failed.Status != metadatadomain.ProjectionFailed || failed.Attempts != 1 || failed.LastError == "" {
		t.Fatalf("failed projection = %+v", failed)
	}

	engine.failUpsert = false
	if err := service.ProjectProductRelease(ctx, uuid.New(), failedReleaseID); err != nil {
		t.Fatalf("retry projection: %v", err)
	}
	retried, err := metadataRepo.GetProjection(ctx, metadatadomain.ProviderOpenMetadata, "PRODUCT_RELEASE", failedReleaseID)
	if err != nil {
		t.Fatalf("get retried projection: %v", err)
	}
	if retried.Status != metadatadomain.ProjectionSucceeded || retried.Attempts != 2 {
		t.Fatalf("retried projection = %+v, want SUCCEEDED attempt 2", retried)
	}
}

var _ metadataengine.Engine = (*fakeMetadataEngine)(nil)
