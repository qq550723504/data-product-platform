package infrastructure_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestReadinessAcceptsPreparedHeaderOnlyInputs(t *testing.T) {
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
	workflowID := uuid.New()
	workflowVersionID := uuid.New()
	executionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, lifecycle_status, metadata)
		VALUES ($1,$2,$3,'Header-only curated dataset','CURATED','ACTIVE','{}'::jsonb)
	`, datasetID, workspaceID, "HEADER-ONLY-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, generated_by_execution_id,
			metadata, created_at, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://test-bucket/header-only.csv',
		          'text/csv','SHA256',$3,$4,'{}'::jsonb,now(),now())
	`, datasetVersionID, datasetID, repeatReadinessHex('a'), executionID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow (id, workspace_id, code, name)
		VALUES ($1,$2,$3,'Header-only readiness workflow')
	`, workflowID, workspaceID, "HEADER-ONLY-WORKFLOW-"+uuid.NewString()); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_version (id, workflow_id, version, definition_ref, definition_sha256, definition)
		VALUES ($1,$2,'1.0.0','header-only/readiness',$3,'{}'::jsonb)
	`, workflowVersionID, workflowID, repeatReadinessHex('b')); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO execution (
			id, workspace_id, workflow_version_id, output_dataset_id, output_dataset_version_id,
			target_period, status, attempt, engine_type, metrics, created_at, started_at, finished_at
		) VALUES ($1,$2,$3,$4,$5,'2026-09','SUCCEEDED',1,'NATIVE','{}'::jsonb,now(),now(),now())
	`, executionID, workspaceID, workflowVersionID, datasetID, datasetVersionID); err != nil {
		t.Fatalf("insert execution: %v", err)
	}
	for _, inputName := range []string{"enterprise_resolution", "enterprise_raw", "lease_raw", "energy_raw"} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO execution_input (execution_id, input_name, dataset_version_id)
			VALUES ($1,$2,$3)
		`, executionID, inputName, datasetVersionID); err != nil {
			t.Fatalf("insert execution input %s: %v", inputName, err)
		}
	}
	companyContent := []byte("company policy snapshot")
	indicatorContent := []byte("indicator policy snapshot")
	resolutionContent := []byte("resolution dependency snapshot")
	if _, err := pool.Exec(ctx, `
		INSERT INTO execution_dependency_preparation (
			execution_id, workspace_id, binding_fingerprint, mapping_usage_count, status
		) VALUES ($1,$2,$3,0,'PREPARED')
	`, executionID, workspaceID, repeatReadinessHex('c')); err != nil {
		t.Fatalf("insert zero-usage preparation: %v", err)
	}
	for _, binding := range []struct {
		name      string
		datasetID *uuid.UUID
		reference string
		version   string
		content   []byte
	}{
		{name: "enterprise_resolution", datasetID: &datasetVersionID, reference: "resolution", version: "1.0.0", content: resolutionContent},
		{name: "company_match_policy", reference: "company", version: "1.0.0", content: companyContent},
		{name: "indicator_policy", reference: "indicator", version: "1.0.0", content: indicatorContent},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO execution_dependency_binding (
				id, execution_id, workspace_id, dependency_name, dataset_version_id,
				reference, version, content_sha256, content
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, uuid.New(), executionID, workspaceID, binding.name, binding.datasetID, binding.reference,
			binding.version, hashReadinessBytes(binding.content), binding.content); err != nil {
			t.Fatalf("insert dependency binding %s: %v", binding.name, err)
		}
	}

	repo := infrastructure.NewPostgresRepository(pool)
	release := domain.ProductRelease{
		ID: uuid.New(), ProductID: uuid.New(), ProductVersionID: uuid.New(),
		Datasets: []domain.ReleaseDataset{{DatasetVersionID: datasetVersionID, Role: domain.DatasetPrimary}},
	}
	product := domain.DataProduct{ID: release.ProductID, WorkspaceID: workspaceID}
	version := domain.ProductVersion{ID: release.ProductVersionID, ProductID: release.ProductID}
	facts, err := repo.ReadinessFacts(ctx, release, product, version, time.Now().UTC())
	if err != nil {
		t.Fatalf("read header-only readiness: %v", err)
	}
	if !facts.ProductionDependencyBindingRequired || !facts.ProductionDependencyBindingComplete {
		t.Fatalf("header-only dependency readiness = required:%v complete:%v, want true/true", facts.ProductionDependencyBindingRequired, facts.ProductionDependencyBindingComplete)
	}
}

func hashReadinessBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func repeatReadinessHex(char byte) string {
	value := string(char)
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
