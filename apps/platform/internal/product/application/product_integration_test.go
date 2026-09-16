package application_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestProductVersionAndDraftReleaseAreFrozenBeforeGovernanceGates(t *testing.T) {
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
	datasetID := uuid.New()
	datasetVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (
			id, workspace_id, code, name, dataset_type, lifecycle_status, metadata,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,'CURATED','ACTIVE','{}'::jsonb,now(),now())
	`, datasetID, workspaceID, "PRODUCT-CORE-DATASET-"+uuid.NewString(), "Product core curated dataset"); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, created_at, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://test-bucket/product-core.csv',
		          'text/csv','SHA256','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
		          '{}'::jsonb,now(),now())
	`, datasetVersionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}

	txManager := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	service := application.NewService(txManager, repo)

	product, err := service.CreateProduct(ctx, application.CreateProductCommand{
		WorkspaceID: workspaceID,
		Code:        "DP-ACTIVITY-" + uuid.NewString(),
		Name:        "企业经营活跃度",
		Description: "Reference Data Product",
		DomainCode:  "PARK",
		Metadata: map[string]any{
			"referenceImplementation": "enterprise-activity-v1",
		},
		TraceID: "product-core-integration",
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	version, err := service.CreateVersion(ctx, application.CreateVersionCommand{
		ProductID:       product.ID,
		MajorVersion:    1,
		MinorVersion:    0,
		PatchVersion:    0,
		EntityPolicyRef: "park-company-match@1.0.0",
		IndicatorSetRef: "park-enterprise-activity@1.0.0",
		Definition: map[string]any{
			"purpose": "ENTERPRISE_CREDIT_RISK_SUPPORT",
		},
		Assets: []domain.AssetSpec{
			{
				AssetType: domain.AssetDataset,
				Name:      "enterprise_activity_curated",
				DatasetID: &datasetID,
				DeliveryConfig: map[string]any{
					"mode": "DATASET",
				},
			},
		},
		TraceID: "product-core-integration",
	})
	if err != nil {
		t.Fatalf("create product version: %v", err)
	}
	if version.Semver() != "1.0.0" || len(version.Assets) != 1 {
		t.Fatalf("product version = %s assets=%d, want 1.0.0 with one asset", version.Semver(), len(version.Assets))
	}

	if _, err := pool.Exec(ctx, `UPDATE product_version SET indicator_set_ref='mutated' WHERE id=$1`, version.ID); err == nil {
		t.Fatal("expected ProductVersion mutation to be rejected")
	}

	release, err := service.CreateRelease(ctx, application.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: version.ID,
		ReleaseNo:        "R-POC-001",
		Datasets: []domain.ReleaseDataset{
			{DatasetVersionID: datasetVersionID, Role: domain.DatasetPrimary},
		},
		ReleaseNotes: "Sprint 2 draft release; governance gates intentionally pending",
		TraceID:      "product-core-integration",
	})
	if err != nil {
		t.Fatalf("create release draft: %v", err)
	}
	if release.Status != domain.ReleaseDraft {
		t.Fatalf("release status = %s, want DRAFT", release.Status)
	}

	readiness, err := service.Readiness(ctx, release.ID)
	if err != nil {
		t.Fatalf("read release readiness: %v", err)
	}
	if readiness.Overall != "NOT_READY" {
		t.Fatalf("readiness overall = %s, want NOT_READY", readiness.Overall)
	}
	if readiness.Checks["production"] != application.CheckPass || readiness.Checks["dataset"] != application.CheckPass {
		t.Fatalf("production/dataset readiness = %#v", readiness.Checks)
	}
	for _, gate := range []string{"rights", "quality", "compliance", "contract", "evidence", "delivery"} {
		if readiness.Checks[gate] != application.CheckPending {
			t.Fatalf("gate %s = %s, want PENDING", gate, readiness.Checks[gate])
		}
	}

	storedProduct, err := repo.GetProduct(ctx, product.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if storedProduct.CurrentVersionID == nil || *storedProduct.CurrentVersionID != version.ID {
		t.Fatalf("current version = %v, want %s", storedProduct.CurrentVersionID, version.ID)
	}

	var versionEvents int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ProductVersionCreated'`, version.ID).Scan(&versionEvents); err != nil {
		t.Fatalf("count ProductVersionCreated events: %v", err)
	}
	if versionEvents != 1 {
		t.Fatalf("ProductVersionCreated events = %d, want 1", versionEvents)
	}
	var releaseAudits int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='PRODUCT_RELEASE_DRAFT_CREATED'`, release.ID).Scan(&releaseAudits); err != nil {
		t.Fatalf("count release audit events: %v", err)
	}
	if releaseAudits != 1 {
		t.Fatalf("release audit events = %d, want 1", releaseAudits)
	}
}
