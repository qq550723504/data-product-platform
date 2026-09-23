package traceability_test

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
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

	mappingDecisionID := uuid.New()
	mappingTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin mapping fixture transaction: %v", err)
	}
	if _, err := mappingTx.Exec(ctx, `
		INSERT INTO entity_mapping (
			id, workspace_id, entity_id, source_type, source_ref, source_key, source_name, match_method,
			match_rule_id, match_policy_version, match_engine_name, match_engine_version,
			confidence, status, reviewer_reason, evidence_id, current_decision_id
		) VALUES ($1,$2,$3,'CSV','enterprise.csv','SRC-001','示例科技有限公司','MANUAL_REVIEW',
		          'REVIEW-001','1.0.0','RULES','1',1.0,'CONFIRMED','verified against source registry',$4,$5)
	`, mappingID, workspaceID, entityID, entityEvidence.ID, mappingDecisionID); err != nil {
		_ = mappingTx.Rollback(ctx)
		t.Fatalf("insert entity mapping fixture: %v", err)
	}
	// entity_mapping.current_decision_id is deferred, so the immutable decision
	// that produced the projection is appended in the same transaction. The
	// decision proves it was produced by the release-time match job, which is
	// what lets the release trace bind to it instead of the mutable projection.
	if _, err := mappingTx.Exec(ctx, `
		INSERT INTO entity_mapping_decision (
			id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version, match_engine_name, match_engine_version,
			match_model_version, confidence, status, reviewer_reason, evidence_id, idempotency_key, source_origin, source_job_id, decided_at
		) VALUES ($1,$2,$3,$4,'CSV','enterprise.csv','SRC-001','示例科技有限公司',
		          'MANUAL_REVIEW','REVIEW-001','1.0.0','RULES','1','',1.0,'CONFIRMED',
		          'verified against source registry',$5,$7,'MATCH_CANDIDATE',$6,now())
	`, mappingDecisionID, workspaceID, mappingID, entityID, entityEvidence.ID, matchJobID, "trace-release:"+mappingDecisionID.String()); err != nil {
		_ = mappingTx.Rollback(ctx)
		t.Fatalf("insert entity mapping decision fixture: %v", err)
	}
	if err := mappingTx.Commit(ctx); err != nil {
		t.Fatalf("commit entity mapping fixture: %v", err)
	}

	sourceMetadata := map[string]any{"source": "captured-import"}
	sourceRecord := evidence.Record{WorkspaceID: workspaceID, EvidenceType: "SOURCE_CAPTURE", Title: "Source evidence", Metadata: sourceMetadata, CreatedAt: evidence.NormalizeCreatedAt(time.Now().UTC())}
	sourceHash, err := evidence.ComputeHash(sourceRecord, evidence.HashAlgorithmEvidenceV2)
	if err != nil {
		t.Fatalf("compute source Evidence hash: %v", err)
	}
	sourceMetadataJSON, _ := json.Marshal(sourceMetadata)
	sourceEvidenceID := uuid.New()
	mustExec(t, ctx, pool, `
		INSERT INTO evidence (
			id, workspace_id, evidence_type, title, hash_algorithm, hash_value, metadata, created_at
		) VALUES ($1,$2,'SOURCE_CAPTURE','Source evidence','SHA256-EVIDENCE-V2',$3,$4,$5)
	`, sourceEvidenceID, workspaceID, sourceHash, sourceMetadataJSON, sourceRecord.CreatedAt)
	mustExec(t, ctx, pool, `
		INSERT INTO evidence_relation (evidence_id, object_type, object_id, relation_type)
		VALUES ($1,'DATASET_VERSION',$2,'SOURCE_EVIDENCE')
	`, sourceEvidenceID, rawVersionID)

	mustExec(t, ctx, pool, `
		INSERT INTO data_product (
			id, workspace_id, code, name, lifecycle_status, health_status, metadata, created_at, updated_at
		) VALUES ($1,$2,$3,'企业经营活跃度','PUBLISHED','HEALTHY','{}'::jsonb,now(),now())
	`, productID, workspaceID, "TRACE-PRODUCT-"+uuid.NewString())
	mustBuildProductVersion(t, ctx, pool, productVersionID, `
		INSERT INTO product_version (
			id, product_id, major_version, minor_version, patch_version, workflow_version_id,
			entity_policy_ref, indicator_set_ref, definition_snapshot, build_status, expected_asset_count, created_at
		) VALUES ($1,$2,1,0,0,$3,'park/matching/company-match-policy-v1.yaml',
		          'park/indicators/enterprise-activity-v1.yaml','{}'::jsonb,'BUILDING',0,now())
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
		) VALUES ($1,$2,$3,'R-TRACE-001','READY',$4,'{}'::jsonb,now(),NULL)
	`, releaseID, productID, productVersionID, snapshot.ID)
	mustExec(t, ctx, pool, `
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($1,$2,'PRIMARY')
	`, releaseID, curatedVersionID)
	mustExec(t, ctx, pool, `
		UPDATE product_release
		SET status='PUBLISHED', released_at=now()
		WHERE id=$1 AND status='READY'
	`, releaseID)
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
	if trace.Executions[0].DependencyPreparationStatus != "NOT_AVAILABLE" || len(trace.Executions[0].MappingUsages) != 0 {
		t.Fatalf("legacy execution dependency gap = status=%s usages=%d, want NOT_AVAILABLE/0", trace.Executions[0].DependencyPreparationStatus, len(trace.Executions[0].MappingUsages))
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
	if trace.EntityMappings[0].DecisionID != mappingDecisionID || trace.EntityMappings[0].EntityID != entityID || trace.EntityMappings[0].SourceJobID == nil || *trace.EntityMappings[0].SourceJobID != matchJobID {
		t.Fatalf("EntityMapping trace is not bound to the release-time decision: %+v", trace.EntityMappings[0])
	}
	if len(trace.AuditEvents) < 3 {
		t.Fatalf("AuditEvent trace size = %d, want at least 3", len(trace.AuditEvents))
	}

	integrityByID := map[uuid.UUID]bool{}
	for _, item := range trace.Evidence {
		integrityByID[item.ID] = item.IntegrityValid
	}
	for _, id := range []uuid.UUID{executionEvidence.ID, entityEvidence.ID, sourceEvidenceID} {
		if !integrityByID[id] {
			t.Fatalf("Evidence %s missing or failed integrity verification: %#v", id, integrityByID)
		}
	}

	assertMutationRejected(t, ctx, pool, `UPDATE evidence SET title='tampered' WHERE id=$1`, executionEvidence.ID)
	assertMutationRejected(t, ctx, pool, `DELETE FROM evidence_relation WHERE evidence_id=$1`, entityEvidence.ID)
	assertMutationRejected(t, ctx, pool, `UPDATE cost_event SET quantity=2 WHERE execution_id=$1`, executionID)
	assertMutationRejected(t, ctx, pool, `DELETE FROM audit_event WHERE object_id=$1`, releaseID)

	// Dependency trace queries must not run while the outer execution rows still
	// hold the pool connection. A one-connection pool makes that regression
	// deterministic instead of relying on production pool pressure.
	singleConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse single-connection postgres config: %v", err)
	}
	singleConfig.MaxConns = 1
	singlePool, err := pgxpool.NewWithConfig(ctx, singleConfig)
	if err != nil {
		t.Fatalf("open single-connection postgres pool: %v", err)
	}
	defer singlePool.Close()
	traceCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := traceability.NewRepository(singlePool).ProductRelease(traceCtx, releaseID); err != nil {
		t.Fatalf("ProductRelease trace with one pool connection: %v", err)
	}
}

// TestReleaseTraceBindsMappingsToDecisionNotCurrentProjection is the B regression:
// a release must report the immutable decision its match job actually applied,
// even though entity_mapping may have moved on to another entity afterwards.
//
//	(a) the current mapping moves to B before the release exists: R1 still binds A;
//	(b) a decision recorded after R1 is published leaves R1's entity, reason,
//	    decision id and frozen evidence untouched.
//
// Decisions that cannot prove a source job are never guessed into the release.
func TestReleaseTraceBindsMappingsToDecisionNotCurrentProjection(t *testing.T) {
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
	rawDatasetID := uuid.New()
	standardizedDatasetID := uuid.New()
	rawVersionID := uuid.New()
	standardizedVersionID := uuid.New()
	entityTypeID := uuid.New()
	entityAID := uuid.New()
	entityBID := uuid.New()
	matchJobID := uuid.New()
	mappingID := uuid.New()
	decisionAID := uuid.New()
	decisionBID := uuid.New()
	productID := uuid.New()
	productVersionID := uuid.New()
	releaseID := uuid.New()
	reasonA := "release-time confirmation " + uuid.NewString()
	reasonB := "post-release correction " + uuid.NewString()

	insertDataset(t, ctx, pool, rawDatasetID, workspaceID, "BIND-RAW-"+uuid.NewString(), "RAW", nil)
	insertDataset(t, ctx, pool, standardizedDatasetID, workspaceID, "BIND-STD-"+uuid.NewString(), "STANDARDIZED", nil)
	insertDatasetVersion(t, ctx, pool, rawVersionID, rawDatasetID, nil)
	insertDatasetVersion(t, ctx, pool, standardizedVersionID, standardizedDatasetID, nil)
	mustExec(t, ctx, pool, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type)
		VALUES ($1,$2,'ENTITY_RESOLUTION')
	`, standardizedVersionID, rawVersionID)

	mustExec(t, ctx, pool, `
		INSERT INTO entity_type (id, workspace_id, code, name, key_schema, attribute_schema)
		VALUES ($1,$2,$3,'Company','{}'::jsonb,'{}'::jsonb)
	`, entityTypeID, workspaceID, "BIND-COMPANY-"+uuid.NewString())
	mustExec(t, ctx, pool, `
		INSERT INTO entity (id, workspace_id, entity_type_id, canonical_key, canonical_name, attributes)
		VALUES ($1,$2,$3,'COMPANY-A','甲公司','{}'::jsonb), ($4,$2,$3,'COMPANY-B','乙公司','{}'::jsonb)
	`, entityAID, workspaceID, entityTypeID, entityBID)
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
		t.Fatalf("begin evidence fixture: %v", err)
	}
	evidenceA, err := evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "ENTITY_MATCH_REVIEW",
		Title:        "Release-time human confirmation",
		SourceType:   "ENTITY_MATCH_JOB",
		SourceID:     &matchJobID,
		Metadata:     map[string]any{"decision": "CONFIRMED", "reviewerReason": reasonA},
	}, evidence.Relation{ObjectType: "ENTITY_MATCH_JOB", ObjectID: matchJobID, RelationType: "SUPPORTS"})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append release-time Evidence: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit release-time Evidence: %v", err)
	}

	// The projection starts on A (the decision the job produced) and is then moved
	// to B, exactly like a later manual correction. The release is created after
	// that move, so a current-table read would return B.
	mappingTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin mapping fixture: %v", err)
	}
	if _, err := mappingTx.Exec(ctx, `
		INSERT INTO entity_mapping (
			id, workspace_id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version, match_engine_name, match_engine_version,
			confidence, status, reviewer_reason, evidence_id, current_decision_id
		) VALUES ($1,$2,$3,'CSV','enterprise.csv','SRC-001','甲公司','MANUAL_REVIEW',
		          'REVIEW-001','1.0.0','RULES','1',1.0,'CONFIRMED',$4,NULL,$5)
	`, mappingID, workspaceID, entityAID, reasonA, decisionAID); err != nil {
		_ = mappingTx.Rollback(ctx)
		t.Fatalf("insert entity mapping fixture: %v", err)
	}
	if _, err := mappingTx.Exec(ctx, `
		INSERT INTO entity_mapping_decision (
			id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version, match_engine_name, match_engine_version,
			match_model_version, confidence, status, reviewer_reason, evidence_id, idempotency_key, source_origin, source_job_id, decided_at
		) VALUES ($1,$2,$3,$4,'CSV','enterprise.csv','SRC-001','甲公司',
		          'MANUAL_REVIEW','REVIEW-001','1.0.0','RULES','1','',1.0,'CONFIRMED',$5,$6,$8,'MATCH_CANDIDATE',$7,now())
	`, decisionAID, workspaceID, mappingID, entityAID, reasonA, evidenceA.ID, matchJobID, "trace-bind-a:"+decisionAID.String()); err != nil {
		_ = mappingTx.Rollback(ctx)
		t.Fatalf("insert job-bound mapping decision: %v", err)
	}
	if _, err := mappingTx.Exec(ctx, `
		INSERT INTO entity_mapping_decision (
			id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version, match_engine_name, match_engine_version,
			match_model_version, confidence, status, reviewer_reason, evidence_id, idempotency_key, source_origin, source_job_id, decided_at
		) VALUES ($1,$2,$3,$4,'CSV','enterprise.csv','SRC-001','乙公司',
		          'MANUAL_REVIEW','REVIEW-002','1.0.0','RULES','1','',1.0,'CONFIRMED',$5,NULL,$6,'MANUAL_REVIEW',NULL,now())
	`, decisionBID, workspaceID, mappingID, entityBID, reasonB, "trace-bind-b:"+decisionBID.String()); err != nil {
		_ = mappingTx.Rollback(ctx)
		t.Fatalf("insert unbound correction decision: %v", err)
	}
	if _, err := mappingTx.Exec(ctx, `
		UPDATE entity_mapping
		SET entity_id=$2, reviewer_reason=$3, evidence_id=NULL, current_decision_id=$4
		WHERE id=$1
	`, mappingID, entityBID, reasonB, decisionBID); err != nil {
		_ = mappingTx.Rollback(ctx)
		t.Fatalf("move current mapping to B: %v", err)
	}
	if err := mappingTx.Commit(ctx); err != nil {
		t.Fatalf("commit mapping fixture: %v", err)
	}

	mustExec(t, ctx, pool, `
		INSERT INTO data_product (id, workspace_id, code, name, lifecycle_status, health_status, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,'Bound mapping product','PUBLISHED','HEALTHY','{}'::jsonb,now(),now())
	`, productID, workspaceID, "BIND-PRODUCT-"+uuid.NewString())
	mustBuildProductVersion(t, ctx, pool, productVersionID, `
		INSERT INTO product_version (id, product_id, major_version, minor_version, patch_version, definition_snapshot, build_status, expected_asset_count, created_at)
		VALUES ($1,$2,1,0,0,'{}'::jsonb,'BUILDING',0,now())
	`, productVersionID, productID)
	mustExec(t, ctx, pool, `
		INSERT INTO product_release (id, product_id, product_version_id, release_no, status, metadata, created_at, released_at)
		VALUES ($1,$2,$3,'R-BIND-001','READY','{}'::jsonb,now(),NULL)
	`, releaseID, productID, productVersionID)
	mustExec(t, ctx, pool, `
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($1,$2,'PRIMARY')
	`, releaseID, standardizedVersionID)
	mustExec(t, ctx, pool, `
		UPDATE product_release
		SET status='PUBLISHED', released_at=now()
		WHERE id=$1 AND status='READY'
	`, releaseID)

	repo := traceability.NewRepository(pool)
	trace, err := repo.ProductRelease(ctx, releaseID)
	if err != nil {
		t.Fatalf("query ProductRelease traceability: %v", err)
	}
	if len(trace.EntityMappings) != 1 {
		t.Fatalf("release trace entity mappings = %+v, want exactly the job-bound decision", trace.EntityMappings)
	}
	bound := trace.EntityMappings[0]
	if bound.DecisionID != decisionAID || bound.EntityID != entityAID || bound.ReviewerReason != reasonA || bound.EvidenceID == nil || *bound.EvidenceID != evidenceA.ID {
		t.Fatalf("release trace did not bind the job decision: %+v", bound)
	}
	if bound.SourceJobID == nil || *bound.SourceJobID != matchJobID || bound.SourceOrigin != "MATCH_CANDIDATE" {
		t.Fatalf("release trace lost the decision's source association: %+v", bound)
	}
	if bound.DecisionID == decisionBID {
		t.Fatalf("release trace leaked the later current mapping decision: %+v", bound)
	}

	var currentEntityID, currentDecisionID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT entity_id, current_decision_id FROM entity_mapping WHERE id=$1`, mappingID).Scan(&currentEntityID, &currentDecisionID); err != nil {
		t.Fatalf("read current mapping projection: %v", err)
	}
	if currentEntityID != entityBID || currentDecisionID != decisionBID {
		t.Fatalf("current mapping projection = %s/%s, want B/B", currentEntityID, currentDecisionID)
	}

	// (b) A decision appended after publication must not rewrite what R1 shows.
	decisionCID := uuid.New()
	mustExec(t, ctx, pool, `
		INSERT INTO entity_mapping_decision (
			id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version, match_engine_name, match_engine_version,
			match_model_version, confidence, status, reviewer_reason, evidence_id, idempotency_key, source_origin, source_job_id, decided_at
		) VALUES ($1,$2,$3,$4,'CSV','enterprise.csv','SRC-001','甲公司',
		          'MANUAL_REVIEW','REVIEW-003','1.0.0','RULES','1','',1.0,'CONFIRMED','post-release append',NULL,$5,'MANUAL_REVIEW',NULL,now())
	`, decisionCID, workspaceID, mappingID, entityAID, "trace-bind-c:"+decisionCID.String())
	after, err := repo.ProductRelease(ctx, releaseID)
	if err != nil {
		t.Fatalf("re-query ProductRelease traceability: %v", err)
	}
	if !reflect.DeepEqual(trace.EntityMappings, after.EntityMappings) {
		t.Fatalf("published release entity mappings changed after a later decision:\nbefore=%+v\nafter=%+v", trace.EntityMappings, after.EntityMappings)
	}
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

func mustBuildProductVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, versionID uuid.UUID, query string, args ...any) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin ProductVersion fixture: %v", err)
	}
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("insert ProductVersion fixture: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE product_version
		SET build_status='FINALIZED'
		WHERE id=$1 AND build_status='BUILDING'
	`, versionID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("finalize ProductVersion fixture: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit ProductVersion fixture: %v", err)
	}
}

func mustExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("execute fixture SQL: %v", err)
	}
}
