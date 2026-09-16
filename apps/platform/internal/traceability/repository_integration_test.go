package traceability_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
)

func TestPublishedProductReleaseTraceability(t *testing.T) {
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
	rawDatasetID := uuid.New()
	standardizedDatasetID := uuid.New()
	curatedDatasetID := uuid.New()
	rawVersionID := uuid.New()
	standardizedVersionID := uuid.New()
	curatedVersionID := uuid.New()
	workflowID := uuid.New()
	workflowVersionID := uuid.New()
	executionID := uuid.New()
	entityTypeID := uuid.New()
	entityID := uuid.New()
	matchJobID := uuid.New()
	mappingID := uuid.New()
	productID := uuid.New()
	productVersionID := uuid.New()
	releaseID := uuid.New()

	mustExec(t, ctx, pool, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type)
		VALUES ($1,$2,$3,'Enterprise source','TABLE_LIKE')
	`, resourceID, workspaceID, "TRACE-RESOURCE-"+uuid.NewString())
	insertDataset(t, ctx, pool, rawDatasetID, workspaceID, "TRACE-RAW-"+uuid.NewString(), "RAW", &resourceID)
	insertDataset(t, ctx, pool, standardizedDatasetID, workspaceID, "TRACE-STD-"+uuid.NewString(), "STANDARDIZED", nil)
	insertDataset(t, ctx, pool, curatedDatasetID, workspaceID, "TRACE-CURATED-"+uuid.NewString(), "CURATED", nil)

	insertDatasetVersion(t, ctx, pool, rawVersionID, rawDatasetID, nil)
	insertDatasetVersion(t, ctx, pool, standardizedVersionID, standardizedDatasetID, nil)

	mustExec(t, ctx, pool, `
		INSERT INTO workflow (id, workspace_id, code, name, status, created_at, updated_at)
		VALUES ($1,$2,$3,'Enterprise Activity','ACTIVE',now(),now())
	`, workflowID, workspaceID, "TRACE-WORKFLOW-"+uuid.NewString())
	mustExec(t, ctx, pool, `
		INSERT INTO workflow_version (
			id, workflow_id, version, definition_ref, definition_sha256, definition, created_at
		) VALUES ($1,$2,'1.0.0','trace/workflow',
		          'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','{}'::jsonb,now())
	`, workflowVersionID, workflowID)
	mustExec(t, ctx, pool, `
		INSERT INTO execution (
			id, workspace_id, workflow_version_id, output_dataset_id, target_period,
			status, attempt, engine_type, metrics, created_at, started_at, finished_at
		) VALUES ($1,$2,$3,$4,'2026-09','SUCCEEDED',1,'NATIVE','{"outputRows":1}'::jsonb,now(),now(),now())
	`, executionID, workspaceID, workflowVersionID, curatedDatasetID)
	insertDatasetVersion(t, ctx, pool, curatedVersionID, curatedDatasetID, &executionID)
	mustExec(t, ctx, pool, `
		UPDATE execution SET output_dataset_version_id=$2 WHERE id=$1
	`, executionID, curatedVersionID)
	mustExec(t, ctx, pool, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'ENTITY_RESOLUTION'), ($3,$1,'DERIVED_FROM')
	`, standardizedVersionID, rawVersionID, curatedVersionID)

	mustExec(t, ctx, pool, `
		INSERT INTO entity_type (id, workspace_id, code, name, key_schema, attribute_schema)
		VALUES ($1,$2,$3,'Company','{}'::jsonb,'{}'::jsonb)
	`, entityTypeID, workspaceID, "TRACE-COMPANY-"+uuid.NewString())
	mustExec(t, ctx, pool, `
		INSERT INTO entity (id, workspace_id, entity_type_id, canonical_key, canonical_name, attributes)
		VALUES ($1,$2,$3,'COMPANY-001','示例科技有限公司','{}'::jsonb)
	`, entityID, workspaceID, entityTypeID)
	mustExec(t, ctx, pool, `
		INSERT INTO entity_match_job (
			id, workspace_id, entity_type_id, input_dataset_version_id, output_dataset_id,
			source_type, source_ref, source_role, policy_ref, policy_version, status,
			output_dataset_version_id, created_at, started_at, finished_at
		) VALUES ($1,$2,$3,$4,$5,'CSV','enterprise.csv','ANCHOR',
		          'park/matching/company-match-policy-v1.yaml','1.0.0','SUCCEEDED',$6,now(),now(),now())
	`, matchJobID, workspaceID, entityTypeID, rawVersionID, standardizedDatasetID, standardizedVersionID)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin evidence fixture transaction: %v", err)
	}
	executionEvidence, err := evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "PROCESSING_EXECUTION",
		Title:        "Enterprise Activity execution",
		SourceType:   "EXECUTION",
		SourceID:     &executionID,
		Metadata: map[string]any{
			"workflowVersionId":      workflowVersionID,
			"outputDatasetVersionId": curatedVersionID,
		},
	}, evidence.Relation{ObjectType: "EXECUTION", ObjectID: executionID, RelationType: "SUPPORTS"},
		evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: curatedVersionID, RelationType: "SUPPORTS"})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append execution Evidence: %v", err)
	}
	entityEvidence, err := evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "ENTITY_MATCH_REVIEW",
		Title:        "Manual entity match confirmed",
		SourceType:   "ENTITY_MATCH_JOB",
		SourceID:     &matchJobID,
		Metadata: map[string]any{
			"decision":       "CONFIRMED",
			"policyVersion":  "1.0.0",
			"reviewerReason": "verified against source registry",
		},
	}, evidence.Relation{ObjectType: "ENTITY_MATCH_JOB", ObjectID: matchJobID, RelationType: "SUPPORTS"},
		evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: standardizedVersionID, RelationType: "SUPPORTS"})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append entity Evidence: %v", err)
	}
	executionIDCopy := executionID
	if err := cost.Append(ctx, tx, cost.Event{
		WorkspaceID: workspaceID,
		ExecutionID: &executionIDCopy,
		CostType:    "PROCESSING_EXECUTION",
		Quantity:    1,
		Unit:        "execution",
		PricingMode: "POC_ESTIMATE",
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append CostEvent: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit evidence fixture: %v", err)
	}

	mustExec(t, ctx, pool, `
		INSERT INTO entity_mapping (
			id, entity_id, source_type, source_ref, source_key, source_name, match_method,
			match_rule_id, match_policy_version, confidence, status, reviewer_reason, evidence_id
		) VALUES ($1,$2,'CSV','enterprise.csv','SRC-001','示例科技有限公司','MANUAL_REVIEW',
		          'REVIEW-001','1.0.0',1.0,'CONFIRMED','verified against source registry',$3)
	`, mappingID, entityID, entityEvidence.ID)

	legacyMetadata := map[string]any{"source": "legacy-import"}
	legacyHash, err := evidence.ComputeHash(evidence.Record{Metadata: legacyMetadata}, evidence.HashAlgorithmLegacy)
	if err != nil {
		t.Fatalf("compute legacy Evidence hash: %v", err)
	}
	legacyMetadataJSON, _ := json.Marshal(legacyMetadata)
	legacyEvidenceID := uuid.New()
	mustExec(t, ctx, pool, `
		INSERT INTO evidence (
			id, workspace_id, evidence_type, title, hash_algorithm, hash_value, metadata, created_at
		) VALUES ($1,$2,'SOURCE_CAPTURE','Legacy source evidence','SHA256',$3,$4,now())
	`, legacyEvidenceID, workspaceID, legacyHash, legacyMetadataJSON)
	mustExec(t, ctx, pool, `
		INSERT INTO evidence_relation (evidence_id, object_type, object_id, relation_type)
		VALUES ($1,'DATASET_VERSION',$2,'SOURCE_EVIDENCE')
	`, legacyEvidenceID, rawVersionID)

	mustExec(t, ctx, pool, `
		INSERT INTO data_product (
			id, workspace_id, code, name, lifecycle_status, health_status, metadata, created_at, updated_at
		) VALUES ($1,$2,$3,'企业经营活跃度','PUBLISHED','HEALTHY','{}'::jsonb,now(),now())
	`, productID, workspaceID, "TRACE-PRODUCT-"+uuid.NewString())
	mustExec(t, ctx, pool, `
		INSERT INTO product_version (
			id, product_id, major_version, minor_version, patch_version, workflow_version_id,
			entity_policy_ref, indicator_set_ref, definition_snapshot, created_at
		) VALUES ($1,$2,1,0,0,$3,'park/matching/company-match-policy-v1.yaml',
		          'park/indicators/enterprise-activity-v1.yaml','{}'::jsonb,now())
	`, productVersionID, productID, workflowVersionID)

	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin EvidenceSnapshot transaction: %v", err)
	}
	snapshot, err := evidence.CreateSnapshot(ctx, tx, workspaceID, "PRODUCT_RELEASE", releaseID,
		map[string]any{
			"releaseId":        releaseID,
			"productVersionId": productVersionID,
			"datasets":         []string{curatedVersionID.String()},
		},
		[]evidence.SnapshotItem{
			{EvidenceID: executionEvidence.ID, Category: "PROCESSING"},
			{EvidenceID: entityEvidence.ID, Category: "ENTITY_REVIEW"},
		}, nil)
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("create EvidenceSnapshot: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit EvidenceSnapshot: %v", err)
	}

	mustExec(t, ctx, pool, `
		INSERT INTO product_release (
			id, product_id, product_version_id, release_no, status, evidence_snapshot_id,
			metadata, created_at, released_at
		) VALUES ($1,$2,$3,'R-TRACE-001','PUBLISHED',$4,'{}'::jsonb,now(),now())
	`, releaseID, productID, productVersionID, snapshot.ID)
	mustExec(t, ctx, pool, `
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($1,$2,'PRIMARY')
	`, releaseID, curatedVersionID)
	mustExec(t, ctx, pool, `
		INSERT INTO audit_event (workspace_id, actor_type, action, object_type, object_id, metadata)
		VALUES ($1,'SYSTEM','PRODUCT_RELEASE_PUBLISHED','PRODUCT_RELEASE',$2,'{}'::jsonb),
		       ($1,'SERVICE','EXECUTION_SUCCEEDED','EXECUTION',$3,'{}'::jsonb),
		       ($1,'USER','ENTITY_MATCH_CONFIRMED','ENTITY_MATCH_JOB',$4,'{}'::jsonb)
	`, workspaceID, releaseID, executionID, matchJobID)

	repo := traceability.NewRepository(pool)
	trace, err := repo.ProductRelease(ctx, releaseID)
	if err != nil {
		t.Fatalf("query ProductRelease traceability: %v", err)
	}
	if trace.Status != "PUBLISHED" || trace.EvidenceSnapshot == nil || !trace.EvidenceSnapshot.IntegrityValid {
		t.Fatalf("unexpected release/snapshot trace: status=%s snapshot=%+v", trace.Status, trace.EvidenceSnapshot)
	}
	if len(trace.DatasetVersions) != 3 {
		t.Fatalf("DatasetVersion lineage size = %d, want 3", len(trace.DatasetVersions))
	}
	if len(trace.Executions) != 1 || trace.Executions[0].ID != executionID {
		t.Fatalf("execution trace = %+v, want %s", trace.Executions, executionID)
	}
	if len(trace.CostEvents) != 1 || trace.CostEvents[0].ExecutionID == nil || *trace.CostEvents[0].ExecutionID != executionID {
		t.Fatalf("CostEvent trace = %+v", trace.CostEvents)
	}
	if len(trace.EntityMatchJobs) != 1 || trace.EntityMatchJobs[0].ID != matchJobID {
		t.Fatalf("EntityMatchJob trace = %+v", trace.EntityMatchJobs)
	}
	if len(trace.EntityMappings) != 1 || trace.EntityMappings[0].EvidenceID == nil || *trace.EntityMappings[0].EvidenceID != entityEvidence.ID {
		t.Fatalf("EntityMapping trace = %+v", trace.EntityMappings)
	}
	if len(trace.AuditEvents) < 3 {
		t.Fatalf("AuditEvent trace size = %d, want at least 3", len(trace.AuditEvents))
	}

	integrityByID := map[uuid.UUID]bool{}
	for _, item := range trace.Evidence {
		integrityByID[item.ID] = item.IntegrityValid
	}
	for _, id := range []uuid.UUID{executionEvidence.ID, entityEvidence.ID, legacyEvidenceID} {
		if !integrityByID[id] {
			t.Fatalf("Evidence %s missing or failed integrity verification: %#v", id, integrityByID)
		}
	}

	assertMutationRejected(t, ctx, pool, `UPDATE evidence SET title='tampered' WHERE id=$1`, executionEvidence.ID)
	assertMutationRejected(t, ctx, pool, `DELETE FROM evidence_relation WHERE evidence_id=$1`, entityEvidence.ID)
	assertMutationRejected(t, ctx, pool, `UPDATE cost_event SET quantity=2 WHERE execution_id=$1`, executionID)
	assertMutationRejected(t, ctx, pool, `DELETE FROM audit_event WHERE object_id=$1`, releaseID)
}

func insertDataset(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, workspaceID uuid.UUID, code, datasetType string, sourceResourceID *uuid.UUID) {
	t.Helper()
	mustExec(t, ctx, pool, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, source_resource_id, lifecycle_status, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,$3,$4,$5,'ACTIVE','{}'::jsonb,now(),now())
	`, id, workspaceID, code, datasetType, sourceResourceID)
}

func insertDatasetVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id, datasetID uuid.UUID, executionID *uuid.UUID) {
	t.Helper()
	mustExec(t, ctx, pool, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, generated_by_execution_id,
			metadata, created_at
		) VALUES ($1,$2,1,'CREATED','OBJECT_STORAGE',$3,'text/csv','SHA256',$4,$5,'{}'::jsonb,now())
	`, id, datasetID, "s3://test-bucket/"+id.String()+".csv", repeatHex(id.String()[0]), executionID)
}

func repeatHex(seed byte) string {
	alphabet := "0123456789abcdef"
	value := string(alphabet[int(seed)%len(alphabet)])
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}

func assertMutationRejected(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err == nil {
		t.Fatalf("historical mutation unexpectedly succeeded: %s", query)
	}
}

func mustExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("execute fixture SQL: %v", err)
	}
}
