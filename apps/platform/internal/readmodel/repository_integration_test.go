package readmodel_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
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

// TestListEntityReviewsExposesCurrentMappingDecision proves the review queue
// carries the current mapping decision as an optimistic-concurrency token. The
// token must come from the read model, not from a value read at submit time.
func TestListEntityReviewsExposesCurrentMappingDecision(t *testing.T) {
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
	entityTypeID := uuid.New()
	entityID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	jobID := uuid.New()
	candidateID := uuid.New()
	suffix := uuid.NewString()
	sourceRef := "review-source-" + suffix
	sourceKey := "review-key-" + suffix

	if _, err := pool.Exec(ctx, `
		INSERT INTO entity_type (id, workspace_id, code, name)
		VALUES ($1,$2,$3,'Review entity type')
	`, entityTypeID, workspaceID, "RT-"+suffix); err != nil {
		t.Fatalf("insert entity type: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entity (id, workspace_id, entity_type_id, canonical_key, canonical_name)
		VALUES ($1,$2,$3,$4,'Review canonical')
	`, entityID, workspaceID, entityTypeID, "RK-"+suffix); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Review dataset','STANDARDIZED')
	`, datasetID, workspaceID, "RDS-"+suffix); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (id, dataset_id, version_no, status)
		VALUES ($1,$2,1,'CREATED')
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entity_match_job (
			id, workspace_id, entity_type_id, input_dataset_version_id, output_dataset_id,
			source_type, source_ref, source_role, policy_ref, policy_version, status
		) VALUES ($1,$2,$3,$4,$5,'CSV',$6,'ANCHOR','park/matching/policy.yaml','1','WAITING_REVIEW')
	`, jobID, workspaceID, entityTypeID, versionID, datasetID, sourceRef); err != nil {
		t.Fatalf("insert match job: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entity_match_candidate (
			id, job_id, source_key, source_name, source_payload, normalized_payload,
			candidate_entity_id, decision, status, match_method,
			match_engine_name, match_engine_version, match_model_version
		) VALUES ($1,$2,$3,'Review source','{}','{}',$4,'REVIEW','PENDING','RULES','RULES','1','1')
	`, candidateID, jobID, sourceKey, entityID); err != nil {
		t.Fatalf("insert candidate: %v", err)
	}

	repo := readmodel.NewRepository(pool)

	// Before any decision, the queue must say "no mapping observed" rather than
	// silently omitting the field.
	before, err := repo.ListEntityReviews(ctx, workspaceID, "PENDING", 10, 0)
	if err != nil {
		t.Fatalf("list reviews before decision: %v", err)
	}
	if len(before.Items) != 1 {
		t.Fatalf("review count before decision = %d, want 1", len(before.Items))
	}
	if before.Items[0].CurrentMappingDecisionID != nil {
		t.Fatalf("current decision before any mapping = %v, want nil", before.Items[0].CurrentMappingDecisionID)
	}

	entityRepo := entityinfra.NewPostgresRepository(pool)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin decision transaction: %v", err)
	}
	decision, err := entityRepo.RecordMappingDecision(ctx, tx, entitydomain.MappingDecisionCommand{
		Mapping: entitydomain.EntityMapping{
			ID:                 uuid.New(),
			WorkspaceID:        workspaceID,
			EntityID:           entityID,
			SourceType:         "CSV",
			SourceRef:          sourceRef,
			SourceKey:          sourceKey,
			SourceName:         "Review source",
			MatchMethod:        "RULES",
			MatchPolicyVersion: "1",
			MatchEngineName:    "RULES",
			MatchEngineVersion: "1",
			Status:             entitydomain.MappingConfirmed,
			ReviewedBy:         &entityID,
			ReviewerReason:     "reviewed",
		},
		SourceOrigin:          entitydomain.OriginMatchCandidate,
		SourceJobID:           &jobID,
		SourceCandidateID:     &candidateID,
		IdempotencyKey:        "confirm:" + candidateID.String(),
		ExpectCurrentDecision: true,
	})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("record mapping decision: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit decision: %v", err)
	}

	after, err := repo.ListEntityReviews(ctx, workspaceID, "PENDING", 10, 0)
	if err != nil {
		t.Fatalf("list reviews after decision: %v", err)
	}
	if len(after.Items) != 1 {
		t.Fatalf("review count after decision = %d, want 1", len(after.Items))
	}
	got := after.Items[0].CurrentMappingDecisionID
	if got == nil || *got != decision.ID {
		t.Fatalf("current mapping decision = %v, want %s", got, decision.ID)
	}
}
