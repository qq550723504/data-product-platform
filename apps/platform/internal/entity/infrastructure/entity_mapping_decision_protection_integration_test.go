package infrastructure_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// TestEntityMappingDecisionProtection covers the guarantees added by
// 000014_entity_mapping_decision_protection:
//   - the current projection points at the immutable decision that produced it
//   - a retried operation key does not append a duplicate decision
//   - a stale expected decision is rejected instead of silently overwriting
//   - automatic matching can never replace a human confirmation
//   - legacy rows that cannot prove a source are explicitly UNKNOWN
func TestEntityMappingDecisionProtection(t *testing.T) {
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

	workspaceID := uuid.New()
	now := time.Now().UTC()
	entityType := domain.EntityType{ID: uuid.New(), WorkspaceID: workspaceID, Code: "COMPANY", Name: "Company", CreatedAt: now}
	entityA := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceID, EntityTypeID: entityType.ID, CanonicalName: "Alpha", Status: domain.EntityActive, CreatedAt: now}
	entityB := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceID, EntityTypeID: entityType.ID, CanonicalName: "Beta", Status: domain.EntityActive, CreatedAt: now}
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := repo.EnsureEntityType(ctx, tx, entityType); err != nil {
			return err
		}
		for _, entity := range []domain.Entity{entityA, entityB} {
			if err := repo.InsertEntity(ctx, tx, entity); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed entities: %v", err)
	}

	sourceRef := "decision-protection-" + uuid.NewString()
	sourceKey := "ENT-" + uuid.NewString()[:8]
	mapping := func(entityID uuid.UUID, status domain.MappingStatus) domain.EntityMapping {
		return domain.EntityMapping{
			ID:                 uuid.New(),
			WorkspaceID:        workspaceID,
			EntityID:           entityID,
			SourceType:         "CSV",
			SourceRef:          sourceRef,
			SourceKey:          sourceKey,
			SourceName:         "source " + sourceKey,
			MatchMethod:        "ALIAS",
			MatchRuleID:        "TEST-RULE",
			MatchPolicyVersion: "v1",
			MatchEngineName:    "RULES",
			MatchEngineVersion: "1",
			Confidence:         0.9,
			Status:             status,
			CreatedAt:          now,
		}
	}
	record := func(cmd domain.MappingDecisionCommand) (domain.MappingDecision, error) {
		var decision domain.MappingDecision
		err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			decision, err = repo.RecordMappingDecision(ctx, tx, cmd)
			return err
		})
		return decision, err
	}

	// First automatic decision moves the projection and records provenance.
	autoDecision, err := record(domain.MappingDecisionCommand{
		Mapping:        mapping(entityA.ID, domain.MappingAutoMatched),
		SourceOrigin:   domain.OriginMatchCandidate,
		IdempotencyKey: "auto:" + sourceKey,
	})
	if err != nil {
		t.Fatalf("record automatic decision: %v", err)
	}
	if autoDecision.SourceOrigin != domain.OriginMatchCandidate {
		t.Fatalf("source origin = %q, want MATCH_CANDIDATE", autoDecision.SourceOrigin)
	}

	current, err := repo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read current mapping: %v", err)
	}
	if current.CurrentDecisionID == nil || *current.CurrentDecisionID != autoDecision.ID {
		t.Fatalf("current decision = %v, want %s", current.CurrentDecisionID, autoDecision.ID)
	}
	if current.Status != domain.MappingAutoMatched || current.EntityID != entityA.ID {
		t.Fatalf("current mapping = %s/%s, want AUTO_MATCHED/%s", current.Status, current.EntityID, entityA.ID)
	}

	// A retry with the same operation key is idempotent: no duplicate decision.
	retry, err := record(domain.MappingDecisionCommand{
		Mapping:        mapping(entityA.ID, domain.MappingAutoMatched),
		SourceOrigin:   domain.OriginMatchCandidate,
		IdempotencyKey: "auto:" + sourceKey,
	})
	if err != nil {
		t.Fatalf("retry automatic decision: %v", err)
	}
	if retry.ID != autoDecision.ID {
		t.Fatalf("retry decision id = %s, want idempotent %s", retry.ID, autoDecision.ID)
	}
	if count := countDecisions(t, ctx, pool, workspaceID, sourceRef, sourceKey); count != 1 {
		t.Fatalf("decision count after retry = %d, want 1", count)
	}

	// A stale expected decision is rejected under the row lock.
	stale := uuid.New()
	if _, err := record(domain.MappingDecisionCommand{
		Mapping:                   mapping(entityB.ID, domain.MappingAutoMatched),
		SourceOrigin:              domain.OriginMatchCandidate,
		IdempotencyKey:            "auto:stale:" + sourceKey,
		ExpectCurrentDecision:     true,
		ExpectedCurrentDecisionID: &stale,
	}); !errors.Is(err, domain.ErrMappingDecisionConflict) {
		t.Fatalf("stale expected decision error = %v, want ErrMappingDecisionConflict", err)
	}

	// Human confirmation supersedes the automatic decision.
	confirmDecision, err := record(domain.MappingDecisionCommand{
		Mapping:                   mapping(entityB.ID, domain.MappingConfirmed),
		SourceOrigin:              domain.OriginMatchCandidate,
		IdempotencyKey:            "confirm:" + sourceKey,
		DecidedBy:                 &entityB.ID,
		ExpectCurrentDecision:     true,
		ExpectedCurrentDecisionID: &autoDecision.ID,
	})
	if err != nil {
		t.Fatalf("record human confirmation: %v", err)
	}
	current, err = repo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read confirmed mapping: %v", err)
	}
	if current.CurrentDecisionID == nil || *current.CurrentDecisionID != confirmDecision.ID {
		t.Fatalf("current decision = %v, want confirmation %s", current.CurrentDecisionID, confirmDecision.ID)
	}
	if current.Status != domain.MappingConfirmed || current.EntityID != entityB.ID {
		t.Fatalf("confirmed mapping = %s/%s, want CONFIRMED/%s", current.Status, current.EntityID, entityB.ID)
	}

	// Automatic matching must not replace the human confirmation, even with a
	// fresh expected decision.
	if _, err := record(domain.MappingDecisionCommand{
		Mapping:                   mapping(entityA.ID, domain.MappingAutoMatched),
		SourceOrigin:              domain.OriginMatchCandidate,
		IdempotencyKey:            "auto:after-confirm:" + sourceKey,
		ExpectCurrentDecision:     true,
		ExpectedCurrentDecisionID: &confirmDecision.ID,
	}); !errors.Is(err, domain.ErrMappingConfirmedImmutable) {
		t.Fatalf("auto over confirmation error = %v, want ErrMappingConfirmedImmutable", err)
	}
	current, err = repo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read mapping after rejected auto: %v", err)
	}
	if current.EntityID != entityB.ID || current.Status != domain.MappingConfirmed {
		t.Fatalf("rejected auto mutated mapping to %s/%s", current.Status, current.EntityID)
	}

	// History is append-only and ordered.
	decisions, err := repo.ListMappingDecisions(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("decision count = %d, want 2 (auto + confirmation)", len(decisions))
	}
	if decisions[0].ID != autoDecision.ID || decisions[1].ID != confirmDecision.ID {
		t.Fatalf("decision order = %s,%s want %s,%s", decisions[0].ID, decisions[1].ID, autoDecision.ID, confirmDecision.ID)
	}
	if decisions[0].IdempotencyKey != "auto:"+sourceKey || decisions[1].SourceOrigin != domain.OriginMatchCandidate {
		t.Fatalf("decision provenance not preserved: %+v", decisions)
	}

	// Legacy rows that predate source tracking are explicitly UNKNOWN. Insert a
	// decision the way the 000013 backfill did and read the default back.
	legacyRef := "legacy-" + uuid.NewString()
	legacyKey := "LEGACY-" + uuid.NewString()[:8]
	var legacyOrigin string
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		legacyMapping := mapping(entityA.ID, domain.MappingAutoMatched)
		legacyMapping.ID = uuid.New()
		legacyMapping.SourceRef = legacyRef
		legacyMapping.SourceKey = legacyKey
		if err := repo.InsertMapping(ctx, tx, legacyMapping); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			INSERT INTO entity_mapping_decision (
				id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
				match_method, match_policy_version, match_engine_name, match_engine_version,
				match_model_version, status
			)
			SELECT gen_random_uuid(), $1, id, entity_id, source_type, source_ref, source_key,
			       match_method, match_policy_version, match_engine_name, match_engine_version,
			       match_model_version, status
			FROM entity_mapping WHERE workspace_id=$1 AND source_ref=$2 AND source_key=$3
			RETURNING source_origin
		`, workspaceID, legacyRef, legacyKey).Scan(&legacyOrigin)
	}); err != nil {
		t.Fatalf("insert legacy decision: %v", err)
	}
	if legacyOrigin != string(domain.OriginUnknown) {
		t.Fatalf("legacy decision origin = %q, want UNKNOWN", legacyOrigin)
	}

	// The workflow alias path still records provenance instead of leaving it blank.
	aliasDecision, err := record(domain.MappingDecisionCommand{
		Mapping: func() domain.EntityMapping {
			m := mapping(entityA.ID, domain.MappingAutoMatched)
			m.SourceRef = "alias-" + uuid.NewString()
			m.SourceKey = "ALIAS-" + uuid.NewString()[:8]
			return m
		}(),
		SourceOrigin: domain.OriginWorkflowAlias,
	})
	if err != nil {
		t.Fatalf("record workflow alias decision: %v", err)
	}
	if aliasDecision.SourceOrigin != domain.OriginWorkflowAlias {
		t.Fatalf("alias origin = %q, want WORKFLOW_ALIAS", aliasDecision.SourceOrigin)
	}
}

// TestEntityMappingDecisionConcurrentRetry verifies that two concurrent retries
// of one operation key append exactly one decision.
func TestEntityMappingDecisionConcurrentRetry(t *testing.T) {
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

	workspaceID := uuid.New()
	now := time.Now().UTC()
	entityType := domain.EntityType{ID: uuid.New(), WorkspaceID: workspaceID, Code: "COMPANY", Name: "Company", CreatedAt: now}
	entity := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceID, EntityTypeID: entityType.ID, CanonicalName: "Gamma", Status: domain.EntityActive, CreatedAt: now}
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := repo.EnsureEntityType(ctx, tx, entityType); err != nil {
			return err
		}
		return repo.InsertEntity(ctx, tx, entity)
	}); err != nil {
		t.Fatalf("seed entity: %v", err)
	}

	sourceRef := "concurrent-" + uuid.NewString()
	sourceKey := "CON-" + uuid.NewString()[:8]
	key := "auto:" + sourceKey
	mapping := domain.EntityMapping{
		ID: uuid.New(), WorkspaceID: workspaceID, EntityID: entity.ID,
		SourceType: "CSV", SourceRef: sourceRef, SourceKey: sourceKey,
		MatchMethod: "ALIAS", MatchPolicyVersion: "v1",
		MatchEngineName: "RULES", MatchEngineVersion: "1",
		Confidence: 0.9, Status: domain.MappingAutoMatched, CreatedAt: now,
	}

	var wg sync.WaitGroup
	results := make(chan domain.MappingDecision, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var decision domain.MappingDecision
			err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
				var err error
				decision, err = repo.RecordMappingDecision(ctx, tx, domain.MappingDecisionCommand{
					Mapping:        mapping,
					SourceOrigin:   domain.OriginMatchCandidate,
					IdempotencyKey: key,
				})
				return err
			})
			if err != nil {
				errs <- err
				return
			}
			results <- decision
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent decision failed: %v", err)
	}
	seen := map[uuid.UUID]struct{}{}
	for decision := range results {
		seen[decision.ID] = struct{}{}
	}
	if len(seen) != 1 {
		t.Fatalf("concurrent retry produced %d distinct decisions, want 1", len(seen))
	}
	if count := countDecisions(t, ctx, pool, workspaceID, sourceRef, sourceKey); count != 1 {
		t.Fatalf("decision count = %d, want 1", count)
	}
}

func countDecisions(t *testing.T, ctx context.Context, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, workspaceID uuid.UUID, sourceRef, sourceKey string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM entity_mapping_decision
		WHERE workspace_id=$1 AND source_type='CSV' AND source_ref=$2 AND source_key=$3
	`, workspaceID, sourceRef, sourceKey).Scan(&count); err != nil {
		t.Fatalf("count decisions: %v", err)
	}
	return count
}

// seedDecisionWorkspace creates a workspace, one entity type and two entities so
// the idempotency and expectation tests below can record real decisions.
func seedDecisionWorkspace(t *testing.T, ctx context.Context, txManager *transaction.Manager, repo *entityinfra.PostgresRepository) (uuid.UUID, domain.Entity, domain.Entity) {
	t.Helper()
	workspaceID := uuid.New()
	now := time.Now().UTC()
	entityType := domain.EntityType{ID: uuid.New(), WorkspaceID: workspaceID, Code: "COMPANY", Name: "Company", CreatedAt: now}
	entityA := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceID, EntityTypeID: entityType.ID, CanonicalName: "Alpha", Status: domain.EntityActive, CreatedAt: now}
	entityB := domain.Entity{ID: uuid.New(), WorkspaceID: workspaceID, EntityTypeID: entityType.ID, CanonicalName: "Beta", Status: domain.EntityActive, CreatedAt: now}
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := repo.EnsureEntityType(ctx, tx, entityType); err != nil {
			return err
		}
		for _, entity := range []domain.Entity{entityA, entityB} {
			if err := repo.InsertEntity(ctx, tx, entity); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed decision workspace: %v", err)
	}
	return workspaceID, entityA, entityB
}

func decisionMapping(workspaceID, entityID uuid.UUID, sourceRef, sourceKey string, status domain.MappingStatus, reason string) domain.EntityMapping {
	return domain.EntityMapping{
		ID: uuid.New(), WorkspaceID: workspaceID, EntityID: entityID,
		SourceType: "CSV", SourceRef: sourceRef, SourceKey: sourceKey, SourceName: "source " + sourceKey,
		MatchMethod: "ALIAS", MatchRuleID: "TEST-RULE", MatchPolicyVersion: "v1",
		MatchEngineName: "RULES", MatchEngineVersion: "1",
		Confidence: 0.9, Status: status, ReviewerReason: reason,
		CreatedAt: time.Now().UTC(),
	}
}

// TestEntityMappingDecisionIdempotencySemantics verifies that one idempotency
// key is an exact-operation token: an identical retry is a replay, while the same
// key carrying a different target, reason or actor is a conflict that appends
// nothing.
func TestEntityMappingDecisionIdempotencySemantics(t *testing.T) {
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

	workspaceID, entityA, entityB := seedDecisionWorkspace(t, ctx, txManager, repo)
	sourceRef := "idempotency-" + uuid.NewString()
	sourceKey := "IDEM-" + uuid.NewString()[:8]
	const key = "confirm:semantics"
	record := func(mapping domain.EntityMapping) (domain.MappingDecision, error) {
		var decision domain.MappingDecision
		err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			decision, err = repo.RecordMappingDecision(ctx, tx, domain.MappingDecisionCommand{
				Mapping: mapping, SourceOrigin: domain.OriginMatchCandidate, IdempotencyKey: key,
			})
			return err
		})
		return decision, err
	}

	base := decisionMapping(workspaceID, entityA.ID, sourceRef, sourceKey, domain.MappingConfirmed, "first review")
	first, err := record(base)
	if err != nil {
		t.Fatalf("first decision: %v", err)
	}

	// An exact retry with fresh random ids/timestamps is a replay, not a new row.
	exact := base
	exact.ID = uuid.New()
	exact.CreatedAt = base.CreatedAt.Add(time.Minute)
	replay, err := record(exact)
	if err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	if replay.ID != first.ID {
		t.Fatalf("replay decision = %s, want %s", replay.ID, first.ID)
	}

	sameKeyDifferentEntity := base
	sameKeyDifferentEntity.EntityID = entityB.ID
	if _, err := record(sameKeyDifferentEntity); !errors.Is(err, domain.ErrMappingDecisionKeyConflict) {
		t.Fatalf("different entity error = %v, want ErrMappingDecisionKeyConflict", err)
	}

	sameKeyDifferentReason := base
	sameKeyDifferentReason.ReviewerReason = "changed my mind"
	if _, err := record(sameKeyDifferentReason); !errors.Is(err, domain.ErrMappingDecisionKeyConflict) {
		t.Fatalf("different reason error = %v, want ErrMappingDecisionKeyConflict", err)
	}

	reviewer := uuid.New()
	sameKeyDifferentActor := base
	sameKeyDifferentActor.ReviewedBy = &reviewer
	if _, err := record(sameKeyDifferentActor); !errors.Is(err, domain.ErrMappingDecisionKeyConflict) {
		t.Fatalf("different actor error = %v, want ErrMappingDecisionKeyConflict", err)
	}

	if count := countDecisions(t, ctx, pool, workspaceID, sourceRef, sourceKey); count != 1 {
		t.Fatalf("decision count = %d, want 1", count)
	}
	current, err := repo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read current mapping: %v", err)
	}
	if current.CurrentDecisionID == nil || *current.CurrentDecisionID != first.ID {
		t.Fatalf("current decision = %v, want %s", current.CurrentDecisionID, first.ID)
	}
}

// TestEntityMappingConfirmationRequiresExpectedDecision verifies the manual
// replacement contract: a first confirmation or an exact expected decision is
// accepted; replacing a current decision without stating what was observed is
// rejected explicitly instead of silently overwriting it.
func TestEntityMappingConfirmationRequiresExpectedDecision(t *testing.T) {
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

	workspaceID, entityA, entityB := seedDecisionWorkspace(t, ctx, txManager, repo)
	sourceRef := "expectation-" + uuid.NewString()
	sourceKey := "EXP-" + uuid.NewString()[:8]
	record := func(cmd domain.MappingDecisionCommand) (domain.MappingDecision, error) {
		var decision domain.MappingDecision
		err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var err error
			decision, err = repo.RecordMappingDecision(ctx, tx, cmd)
			return err
		})
		return decision, err
	}

	auto, err := record(domain.MappingDecisionCommand{
		Mapping:        decisionMapping(workspaceID, entityA.ID, sourceRef, sourceKey, domain.MappingAutoMatched, ""),
		SourceOrigin:   domain.OriginMatchCandidate,
		IdempotencyKey: "auto:" + sourceKey,
	})
	if err != nil {
		t.Fatalf("record automatic decision: %v", err)
	}

	// A confirmation that replaces the automatic decision without stating what
	// it observed is rejected explicitly.
	if _, err := record(domain.MappingDecisionCommand{
		Mapping:        decisionMapping(workspaceID, entityB.ID, sourceRef, sourceKey, domain.MappingConfirmed, "human review"),
		SourceOrigin:   domain.OriginMatchCandidate,
		IdempotencyKey: "confirm:no-token",
	}); !errors.Is(err, domain.ErrMappingDecisionExpectationRequired) {
		t.Fatalf("missing expectation error = %v, want ErrMappingDecisionExpectationRequired", err)
	}

	// A stale observed decision is a concurrency conflict.
	stale := uuid.New()
	if _, err := record(domain.MappingDecisionCommand{
		Mapping:                   decisionMapping(workspaceID, entityB.ID, sourceRef, sourceKey, domain.MappingConfirmed, "human review"),
		SourceOrigin:              domain.OriginMatchCandidate,
		IdempotencyKey:            "confirm:stale",
		ExpectCurrentDecision:     true,
		ExpectedCurrentDecisionID: &stale,
	}); !errors.Is(err, domain.ErrMappingDecisionConflict) {
		t.Fatalf("stale expectation error = %v, want ErrMappingDecisionConflict", err)
	}

	// Stating the observed automatic decision is accepted and moves the pointer.
	confirmed, err := record(domain.MappingDecisionCommand{
		Mapping:                   decisionMapping(workspaceID, entityB.ID, sourceRef, sourceKey, domain.MappingConfirmed, "human review"),
		SourceOrigin:              domain.OriginMatchCandidate,
		IdempotencyKey:            "confirm:expected",
		ExpectCurrentDecision:     true,
		ExpectedCurrentDecisionID: &auto.ID,
	})
	if err != nil {
		t.Fatalf("expected confirmation: %v", err)
	}

	current, err := repo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read current mapping: %v", err)
	}
	if current.CurrentDecisionID == nil || *current.CurrentDecisionID != confirmed.ID {
		t.Fatalf("current decision = %v, want %s", current.CurrentDecisionID, confirmed.ID)
	}
	if current.Status != domain.MappingConfirmed || current.EntityID != entityB.ID {
		t.Fatalf("current mapping = %s/%s, want CONFIRMED/%s", current.Status, current.EntityID, entityB.ID)
	}

	// A source with no current decision still allows a token-free first confirm.
	freshRef := "expectation-fresh-" + uuid.NewString()
	freshKey := "FRESH-" + uuid.NewString()[:8]
	if _, err := record(domain.MappingDecisionCommand{
		Mapping:        decisionMapping(workspaceID, entityA.ID, freshRef, freshKey, domain.MappingConfirmed, "first review"),
		SourceOrigin:   domain.OriginMatchCandidate,
		IdempotencyKey: "confirm:fresh",
	}); err != nil {
		t.Fatalf("first confirmation without a token: %v", err)
	}
}

// TestEntityMappingDecisionSourceSerialization is the deterministic regression
// for first-creation races. The human confirmation holds the source lock inside
// an open transaction; the automatic match, using a different operation key,
// must block, then observe the committed human decision and refuse to replace
// it. Before source-level serialization both writers could read "no current
// decision" and the automatic upsert overwrote the human one.
func TestEntityMappingDecisionSourceSerialization(t *testing.T) {
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

	workspaceID, entityA, entityB := seedDecisionWorkspace(t, ctx, txManager, repo)
	sourceRef := "serialization-" + uuid.NewString()
	sourceKey := "SER-" + uuid.NewString()[:8]

	manualCmd := domain.MappingDecisionCommand{
		Mapping:        decisionMapping(workspaceID, entityA.ID, sourceRef, sourceKey, domain.MappingConfirmed, "human wins"),
		SourceOrigin:   domain.OriginMatchCandidate,
		IdempotencyKey: "confirm:serialization",
		DecidedBy:      &entityA.ID,
	}
	autoCmd := domain.MappingDecisionCommand{
		Mapping:        decisionMapping(workspaceID, entityB.ID, sourceRef, sourceKey, domain.MappingAutoMatched, ""),
		SourceOrigin:   domain.OriginMatchCandidate,
		IdempotencyKey: "auto:serialization",
		DecidedBy:      &entityB.ID,
	}

	// The human confirmation acquires the source lock and stays uncommitted.
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin human transaction: %v", err)
	}
	committed := false
	defer func() {
		if !committed {
			// Never leave the human transaction open: a failed barrier would
			// otherwise block the automatic writer until the test timeout.
			_ = tx1.Rollback(context.Background())
		}
	}()
	humanDecision, err := repo.RecordMappingDecision(ctx, tx1, manualCmd)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("record human decision: %v", err)
	}

	const applicationName = "dpp-source-serialization-test"
	errCh := make(chan error, 1)
	go func() {
		tx2, err := pool.Begin(ctx)
		if err != nil {
			errCh <- err
			return
		}
		if _, err := tx2.Exec(ctx, "SET LOCAL application_name = '"+applicationName+"'"); err != nil {
			_ = tx2.Rollback(ctx)
			errCh <- err
			return
		}
		_, recordErr := repo.RecordMappingDecision(ctx, tx2, autoCmd)
		_ = tx2.Rollback(context.Background())
		errCh <- recordErr
	}()

	// Deterministic sync barrier: wait until the automatic writer is blocked on
	// the source-scoped advisory lock before releasing the human confirmation.
	// Waiting on any other lock (for example the row/unique-index lock of an
	// already-created mapping) is not enough: it would leave the first-creation
	// window open, which is exactly what source serialization closes.
	waitForAdvisoryLockWait(t, ctx, pool, applicationName)
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit human decision: %v", err)
	}
	committed = true

	select {
	case autoErr := <-errCh:
		if !errors.Is(autoErr, domain.ErrMappingConfirmedImmutable) {
			t.Fatalf("automatic decision error = %v, want ErrMappingConfirmedImmutable", autoErr)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("automatic decision did not finish after the human confirmation committed")
	}

	current, err := repo.GetMappingBySource(ctx, workspaceID, "CSV", sourceRef, sourceKey)
	if err != nil {
		t.Fatalf("read current mapping: %v", err)
	}
	if current.CurrentDecisionID == nil || *current.CurrentDecisionID != humanDecision.ID {
		t.Fatalf("current decision = %v, want human %s", current.CurrentDecisionID, humanDecision.ID)
	}
	if current.Status != domain.MappingConfirmed || current.EntityID != entityA.ID {
		t.Fatalf("current mapping = %s/%s, want CONFIRMED/%s", current.Status, current.EntityID, entityA.ID)
	}
	if count := countDecisions(t, ctx, pool, workspaceID, sourceRef, sourceKey); count != 1 {
		t.Fatalf("decision count = %d, want 1 (automatic match must not append)", count)
	}
}

func waitForAdvisoryLockWait(t *testing.T, ctx context.Context, pool *pgxpool.Pool, applicationName string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var waiting int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND wait_event='advisory'`, applicationName).Scan(&waiting); err != nil {
			t.Fatalf("inspect pg_stat_activity: %v", err)
		}
		if waiting > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("session %q never started waiting on the source advisory lock", applicationName)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
