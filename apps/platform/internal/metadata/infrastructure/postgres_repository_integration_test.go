package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

const testProvider domain.Provider = "TEST_METADATA"

func TestResourceBindingAndGovernanceProjectionPersistence(t *testing.T) {
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
		VALUES ($1,$2,$3,'Metadata projection resource','TABLE_LIKE')
	`, resourceID, workspaceID, "META-"+uuid.NewString()); err != nil {
		t.Fatalf("insert DataResource: %v", err)
	}

	repo := infrastructure.NewPostgresRepository(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin binding transaction: %v", err)
	}
	binding := domain.ResourceBinding{
		ID:              uuid.New(),
		ResourceID:      resourceID,
		Provider:        testProvider,
		EntityType:      "TABLE",
		ExternalID:      "om-table-001",
		ExternalFQN:     "sample_data.ecommerce.public.orders",
		BindingMetadata: map[string]any{"serviceType": "PostgreSQL"},
		IsPrimary:       true,
	}
	persistedBinding, err := repo.UpsertBinding(ctx, tx, binding)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("upsert resource binding: %v", err)
	}
	if persistedBinding.ID != binding.ID {
		_ = tx.Rollback(ctx)
		t.Fatalf("persisted binding ID = %s, want %s", persistedBinding.ID, binding.ID)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit resource binding: %v", err)
	}

	bindings, err := repo.ListBindings(ctx, resourceID)
	if err != nil {
		t.Fatalf("list resource bindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].ExternalID != "om-table-001" || !bindings[0].IsPrimary {
		t.Fatalf("resource bindings = %+v, want one primary OpenMetadata binding", bindings)
	}

	firstEventID := uuid.New()
	projection := domain.GovernanceProjection{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		Provider:      testProvider,
		ObjectType:    "PRODUCT_RELEASE",
		ObjectID:      uuid.New(),
		SourceEventID: &firstEventID,
		Metadata:      map[string]any{"releaseNo": "R-001"},
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin projection transaction: %v", err)
	}
	if err := repo.BeginProjection(ctx, tx, projection); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("begin governance projection: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit governance projection: %v", err)
	}

	stored, err := repo.GetProjection(ctx, projection.Provider, projection.ObjectType, projection.ObjectID)
	if err != nil {
		t.Fatalf("get governance projection: %v", err)
	}
	if stored.Status != domain.ProjectionPending || stored.Attempts != 1 || stored.SourceEventID == nil || *stored.SourceEventID != firstEventID {
		t.Fatalf("initial projection = %+v, want PENDING attempt 1 with source event", stored)
	}

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin failed projection transaction: %v", err)
	}
	if err := repo.MarkProjectionFailed(ctx, tx, projection.Provider, projection.ObjectType, projection.ObjectID, errors.New("openmetadata unavailable")); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("mark governance projection failed: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit failed projection: %v", err)
	}
	failed, err := repo.GetProjection(ctx, projection.Provider, projection.ObjectType, projection.ObjectID)
	if err != nil {
		t.Fatalf("get failed projection: %v", err)
	}
	if failed.Status != domain.ProjectionFailed || failed.LastError == "" {
		t.Fatalf("failed projection = %+v, want FAILED with last error", failed)
	}

	secondEventID := uuid.New()
	projection.SourceEventID = &secondEventID
	projection.Metadata = map[string]any{"releaseNo": "R-001", "retry": true}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin retry projection transaction: %v", err)
	}
	if err := repo.BeginProjection(ctx, tx, projection); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("retry governance projection: %v", err)
	}
	if err := repo.MarkProjectionSucceeded(ctx, tx, projection.Provider, projection.ObjectType, projection.ObjectID,
		"om-product-001", "Park.EnterpriseActivity", map[string]any{"version": 0.1}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("mark governance projection succeeded: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit successful projection: %v", err)
	}

	succeeded, err := repo.GetProjection(ctx, projection.Provider, projection.ObjectType, projection.ObjectID)
	if err != nil {
		t.Fatalf("get successful projection: %v", err)
	}
	if succeeded.Status != domain.ProjectionSucceeded || succeeded.Attempts != 2 || succeeded.ProjectedAt == nil {
		t.Fatalf("successful projection = %+v, want SUCCEEDED attempt 2", succeeded)
	}
	if succeeded.ExternalID != "om-product-001" || succeeded.ExternalFQN != "Park.EnterpriseActivity" {
		t.Fatalf("successful projection external reference = %q/%q", succeeded.ExternalID, succeeded.ExternalFQN)
	}
}
