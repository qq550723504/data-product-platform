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

	externalFQN := "sample_data.ecommerce.public.orders." + uuid.NewString()

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
		ExternalFQN:     externalFQN,
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
	if len(bindings) != 1 || bindings[0].ID != persistedBinding.ID || bindings[0].ExternalID != "om-table-001" || bindings[0].ExternalFQN != externalFQN || !bindings[0].IsPrimary {
		t.Fatalf("resource bindings = %+v, want persisted primary metadata binding %+v", bindings, persistedBinding)
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
	firstAttempt, err := repo.BeginProjection(ctx, tx, projection)
	if err != nil {
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
	if err := repo.MarkProjectionFailed(ctx, tx, projection.Provider, projection.ObjectType, projection.ObjectID, firstEventID, firstAttempt, errors.New("openmetadata unavailable")); err != nil {
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
	secondAttempt, err := repo.BeginProjection(ctx, tx, projection)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("retry governance projection: %v", err)
	}
	if err := repo.MarkProjectionSucceeded(ctx, tx, projection.Provider, projection.ObjectType, projection.ObjectID,
		secondEventID, secondAttempt, "om-product-001", "Park.EnterpriseActivity", map[string]any{"version": 0.1}); err != nil {
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


func TestGovernanceProjectionRejectsStaleCompletion(t *testing.T) {
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
	repo := infrastructure.NewPostgresRepository(pool)
	objectID := uuid.New()
	firstEventID := uuid.New()
	secondEventID := uuid.New()

	first := domain.GovernanceProjection{
		ID:            uuid.New(),
		WorkspaceID:   workspaceID,
		Provider:      testProvider,
		ObjectType:    "PRODUCT_RELEASE",
		ObjectID:      objectID,
		SourceEventID: &firstEventID,
		Metadata:      map[string]any{"attempt": 1},
	}
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first projection: %v", err)
	}
	firstAttempt, err := repo.BeginProjection(ctx, tx1, first)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("begin first projection: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit first projection: %v", err)
	}

	second := first
	second.ID = uuid.New()
	second.SourceEventID = &secondEventID
	second.Metadata = map[string]any{"attempt": 2}
	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second projection: %v", err)
	}
	secondAttempt, err := repo.BeginProjection(ctx, tx2, second)
	if err != nil {
		_ = tx2.Rollback(ctx)
		t.Fatalf("begin second projection: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit second projection: %v", err)
	}
	if secondAttempt != firstAttempt+1 {
		t.Fatalf("second attempt=%d, want %d", secondAttempt, firstAttempt+1)
	}

	staleSuccessTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin stale success: %v", err)
	}
	err = repo.MarkProjectionSucceeded(ctx, staleSuccessTx, testProvider, "PRODUCT_RELEASE", objectID,
		firstEventID, firstAttempt, "stale-external", "stale.fqn", map[string]any{"winner": "stale"})
	if !errors.Is(err, infrastructure.ErrStaleProjectionAttempt) {
		_ = staleSuccessTx.Rollback(ctx)
		t.Fatalf("stale success error=%v, want ErrStaleProjectionAttempt", err)
	}
	_ = staleSuccessTx.Rollback(ctx)

	staleFailureTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin stale failure: %v", err)
	}
	err = repo.MarkProjectionFailed(ctx, staleFailureTx, testProvider, "PRODUCT_RELEASE", objectID,
		firstEventID, firstAttempt, errors.New("late stale failure"))
	if !errors.Is(err, infrastructure.ErrStaleProjectionAttempt) {
		_ = staleFailureTx.Rollback(ctx)
		t.Fatalf("stale failure error=%v, want ErrStaleProjectionAttempt", err)
	}
	_ = staleFailureTx.Rollback(ctx)

	current, err := repo.GetProjection(ctx, testProvider, "PRODUCT_RELEASE", objectID)
	if err != nil {
		t.Fatalf("get current projection after stale completions: %v", err)
	}
	if current.Status != domain.ProjectionPending || current.Attempts != secondAttempt ||
		current.SourceEventID == nil || *current.SourceEventID != secondEventID {
		t.Fatalf("current projection=%+v, want second attempt still PENDING", current)
	}

	winnerTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin winning completion: %v", err)
	}
	if err := repo.MarkProjectionSucceeded(ctx, winnerTx, testProvider, "PRODUCT_RELEASE", objectID,
		secondEventID, secondAttempt, "winner-external", "winner.fqn", map[string]any{"winner": true}); err != nil {
		_ = winnerTx.Rollback(ctx)
		t.Fatalf("complete current projection: %v", err)
	}
	if err := winnerTx.Commit(ctx); err != nil {
		t.Fatalf("commit winning completion: %v", err)
	}

	final, err := repo.GetProjection(ctx, testProvider, "PRODUCT_RELEASE", objectID)
	if err != nil {
		t.Fatalf("get final projection: %v", err)
	}
	if final.Status != domain.ProjectionSucceeded || final.ExternalID != "winner-external" ||
		final.ExternalFQN != "winner.fqn" || final.Attempts != secondAttempt {
		t.Fatalf("final projection=%+v, want second attempt winner", final)
	}
}
