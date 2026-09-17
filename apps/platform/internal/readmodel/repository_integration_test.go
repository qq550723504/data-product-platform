package readmodel_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel"
)

func TestWorkspaceReadModelsStayScoped(t *testing.T) {
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

	workspaceA := uuid.New()
	workspaceB := uuid.New()
	resourceA := uuid.New()
	resourceB := uuid.New()
	datasetA := uuid.New()
	datasetB := uuid.New()
	productA := uuid.New()
	codeSuffix := uuid.NewString()

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM data_product WHERE id=$1`, productA)
		_, _ = pool.Exec(ctx, `DELETE FROM dataset WHERE id=ANY($1)`, []uuid.UUID{datasetA, datasetB})
		_, _ = pool.Exec(ctx, `DELETE FROM data_resource WHERE id=ANY($1)`, []uuid.UUID{resourceA, resourceB})
	}
	defer cleanup()

	if _, err := pool.Exec(ctx, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES ($1,$2,$3,'Workspace A resource','TABLE_LIKE','READY'),
		       ($4,$5,$6,'Workspace B resource','TABLE_LIKE','READY')
	`, resourceA, workspaceA, "RM-A-"+codeSuffix, resourceB, workspaceB, "RM-B-"+codeSuffix); err != nil {
		t.Fatalf("insert resources: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, lifecycle_status)
		VALUES ($1,$2,$3,'Workspace A dataset','RAW','ACTIVE'),
		       ($4,$5,$6,'Workspace B dataset','CURATED','ACTIVE')
	`, datasetA, workspaceA, "DS-A-"+codeSuffix, datasetB, workspaceB, "DS-B-"+codeSuffix); err != nil {
		t.Fatalf("insert datasets: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_product (id, workspace_id, code, name, lifecycle_status, health_status)
		VALUES ($1,$2,$3,'Workspace A product','DRAFT','UNKNOWN')
	`, productA, workspaceA, "DP-A-"+codeSuffix); err != nil {
		t.Fatalf("insert product: %v", err)
	}

	repo := readmodel.NewRepository(pool)
	summary, err := repo.Workbench(ctx, workspaceA)
	if err != nil {
		t.Fatalf("workbench: %v", err)
	}
	if summary.Counts.DataResources != 1 || summary.Counts.Datasets != 1 || summary.Counts.DataProducts != 1 {
		t.Fatalf("unexpected workspace A counts: %#v", summary.Counts)
	}

	resources, err := repo.ListDataResources(ctx, workspaceA, 25, 0)
	if err != nil {
		t.Fatalf("list data resources: %v", err)
	}
	if resources.Page.Total != 1 || len(resources.Items) != 1 || resources.Items[0].ID != resourceA {
		t.Fatalf("workspace scope leaked resources: %#v", resources)
	}

	datasets, err := repo.ListDatasets(ctx, workspaceA, 25, 0)
	if err != nil {
		t.Fatalf("list datasets: %v", err)
	}
	if datasets.Page.Total != 1 || len(datasets.Items) != 1 || datasets.Items[0].ID != datasetA || datasets.Items[0].DatasetType != "RAW" {
		t.Fatalf("workspace scope leaked datasets: %#v", datasets)
	}

	products, err := repo.ListDataProducts(ctx, workspaceA, 25, 0)
	if err != nil {
		t.Fatalf("list products: %v", err)
	}
	if products.Page.Total != 1 || len(products.Items) != 1 || products.Items[0].ID != productA {
		t.Fatalf("workspace scope leaked products: %#v", products)
	}

	belongs, err := repo.DataProductBelongsToWorkspace(ctx, productA, workspaceB)
	if err != nil {
		t.Fatalf("check product workspace: %v", err)
	}
	if belongs {
		t.Fatal("product must not belong to workspace B")
	}
}
