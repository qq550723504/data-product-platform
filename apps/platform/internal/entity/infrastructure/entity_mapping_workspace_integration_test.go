package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// TestEntityMappingWorkspaceScopingAndDecisionHistory covers the guarantees added
// by 000013_entity_mapping_workspace:
//   - the same external source triple can be mapped independently per workspace
//   - a lookup scoped to one workspace never returns another workspace's mapping
//   - an upsert preserves the previous decision instead of overwriting it
//   - the database rejects a mapping whose workspace differs from its entity's
func TestEntityMappingWorkspaceScopingAndDecisionHistory(t *testing.T) {
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

	txManager := transaction.NewManager(pool)
	repo := entityinfra.NewPostgresRepository(pool)

	workspaceA, workspaceB := uuid.New(), uuid.New()
	now := time.Now().UTC()

	typeA := domain.EntityType{ID: uuid.New(), WorkspaceID: workspaceA, Code: "COMPANY", Name: "Company", CreatedAt: now}
	typeB := domain.EntityType{ID: uuid.New(), WorkspaceID: workspaceB, Code: "COMPANY", Name: "Company", CreatedAt: now}
	entityA1 := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceA, EntityTypeID: typeA.ID, CanonicalName: "Alpha One", Status: domain.EntityActive, CreatedAt: now}
	entityA2 := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceA, EntityTypeID: typeA.ID, CanonicalName: "Alpha Two", Status: domain.EntityActive, CreatedAt: now}
	entityB := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceB, EntityTypeID: typeB.ID, CanonicalName: "Beta", Status: domain.EntityActive, CreatedAt: now}

	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := repo.EnsureEntityType(ctx, tx, typeA); err != nil {
			return err
		}
		if _, err := repo.EnsureEntityType(ctx, tx, typeB); err != nil {
			return err
		}
		for _, entity := range []domain.Entity{entityA1, entityA2, entityB} {
			if err := repo.InsertEntity(ctx, tx, entity); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed entities: %v", err)
	}

	insertMapping := func(mapping domain.EntityMapping) error {
		return txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			return repo.InsertMapping(ctx, tx, mapping)
		})
	}
	newMapping := func(workspaceID, entityID uuid.UUID, sourceKey string) domain.EntityMapping {
		return domain.EntityMapping{
			ID:                 uuid.New(),
			WorkspaceID:        workspaceID,
			EntityID:           entityID,
			SourceType:         "CSV",
			SourceRef:          "companies.csv",
			SourceKey:          sourceKey,
			SourceName:         "source " + sourceKey,
			MatchMethod:        "ALIAS",
			MatchRuleID:        "TEST-RULE",
			MatchPolicyVersion: "v1",
			MatchEngineName:    "RULES",
			MatchEngineVersion: "1",
			Confidence:         0.9,
			Status:             domain.MappingAutoMatched,
			CreatedAt:          now,
		}
	}

	// Both workspaces map the same external source triple.
	if err := insertMapping(newMapping(workspaceA, entityA1.ID, "ENT-1")); err != nil {
		t.Fatalf("insert workspace A mapping: %v", err)
	}
	if err := insertMapping(newMapping(workspaceB, entityB.ID, "ENT-1")); err != nil {
		t.Fatalf("insert workspace B mapping for the same source triple: %v", err)
	}

	// A re-match in workspace A must move the current projection and keep history.
	if err := insertMapping(newMapping(workspaceA, entityA2.ID, "ENT-1")); err != nil {
		t.Fatalf("re-match workspace A mapping: %v", err)
	}

	currentA, err := repo.GetMappingBySource(ctx, workspaceA, "CSV", "companies.csv", "ENT-1")
	if err != nil {
		t.Fatalf("read workspace A mapping: %v", err)
	}
	if currentA.EntityID != entityA2.ID {
		t.Fatalf("workspace A current mapping entity = %s, want %s", currentA.EntityID, entityA2.ID)
	}
	if currentA.WorkspaceID != workspaceA {
		t.Fatalf("workspace A mapping workspace = %s, want %s", currentA.WorkspaceID, workspaceA)
	}

	currentB, err := repo.GetMappingBySource(ctx, workspaceB, "CSV", "companies.csv", "ENT-1")
	if err != nil {
		t.Fatalf("read workspace B mapping: %v", err)
	}
	if currentB.EntityID != entityB.ID {
		t.Fatalf("workspace B mapping entity = %s, want %s", currentB.EntityID, entityB.ID)
	}

	decisions, err := repo.ListMappingDecisions(ctx, workspaceA, "CSV", "companies.csv", "ENT-1")
	if err != nil {
		t.Fatalf("list workspace A decisions: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("workspace A decision count = %d, want 2 (prior decision must be preserved)", len(decisions))
	}
	if decisions[0].EntityID != entityA1.ID || decisions[1].EntityID != entityA2.ID {
		t.Fatalf("decision history entity order = %s,%s want %s,%s", decisions[0].EntityID, decisions[1].EntityID, entityA1.ID, entityA2.ID)
	}

	// Decision history is append-only: the database rejects mutation.
	if _, err := pool.Exec(ctx, `UPDATE entity_mapping_decision SET status='REJECTED' WHERE id=$1`, decisions[0].ID); err == nil {
		t.Fatal("immutable entity_mapping_decision accepted an UPDATE")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM entity_mapping_decision WHERE id=$1`, decisions[0].ID); err == nil {
		t.Fatal("immutable entity_mapping_decision accepted a DELETE")
	}

	// Listing mappings is workspace-scoped too.
	foreignList, err := repo.ListMappingsByEntity(ctx, workspaceB, entityA1.ID)
	if err != nil {
		t.Fatalf("list workspace B mappings for workspace A entity: %v", err)
	}
	if len(foreignList) != 0 {
		t.Fatalf("workspace B listed %d mappings of a workspace A entity", len(foreignList))
	}

	// A workspace-scoped read must not fall back to another workspace's mapping.
	if err := insertMapping(newMapping(workspaceA, entityA1.ID, "ENT-ONLY-A")); err != nil {
		t.Fatalf("insert workspace A only mapping: %v", err)
	}
	if _, err := repo.GetMappingBySource(ctx, workspaceB, "CSV", "companies.csv", "ENT-ONLY-A"); err != nil {
		if !errors.Is(err, entityinfra.ErrNotFound) {
			t.Fatalf("read workspace A only triple as workspace B = %v, want ErrNotFound", err)
		}
	} else {
		t.Fatal("workspace B resolved a source triple it never mapped")
	}

	// Database-level workspace consistency: a mapping cannot point at an entity
	// that lives in a different workspace.
	foreign := newMapping(workspaceA, entityB.ID, "ENT-FOREIGN")
	if err := insertMapping(foreign); err == nil {
		t.Fatal("inserted a mapping whose workspace differs from its entity's workspace")
	}
}
