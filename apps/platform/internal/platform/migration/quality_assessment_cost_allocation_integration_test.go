package migration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestQualityAssessmentCostAllocationIsTypedAndWorkspaceBound(t *testing.T) {
	pool := scratchDatabase(t, 22)
	ctx := context.Background()

	workspaceID := uuid.New()
	otherWorkspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	assessmentID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Cost allocation dataset','CURATED')
	`, datasetID, workspaceID, "COST-ALLOCATION-"+uuid.NewString()); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://cost-allocation/fixture.csv',
			'text/csv','SHA256',repeat('a',64),'{}'::jsonb,now())
	`, versionID, datasetID); err != nil {
		t.Fatalf("insert dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'quality/cost.yaml','1.0.0',
			'c4b903018effbb6d36545f03ec3a6513aa3d5b35f12f42a50c150dc4f1ea35dc',
			'legacy-quality-fixture',
			'native-quality','1','PASS','{"dimensions":{}}'::jsonb)
	`, assessmentID, workspaceID, versionID); err != nil {
		t.Fatalf("insert quality assessment: %v", err)
	}
	costEventID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO cost_event(id, workspace_id, activity_id, cost_type, quantity, unit, pricing_mode)
		VALUES ($1,$2,$3,'QUALITY_ENGINE_INVOCATION',1,'assessment','ACTUAL')
	`, costEventID, workspaceID, uuid.New()); err != nil {
		t.Fatalf("insert cost event: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cost_allocation(id, cost_event_id, quality_assessment_id)
		VALUES ($1,$2,$3)
	`, uuid.New(), costEventID, assessmentID); err != nil {
		t.Fatalf("insert typed quality allocation: %v", err)
	}
	var allocated int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM cost_allocation
		WHERE quality_assessment_id=$1 AND delivery_operation_id IS NULL
	`, assessmentID).Scan(&allocated); err != nil {
		t.Fatalf("query typed allocation: %v", err)
	}
	if allocated != 1 {
		t.Fatalf("quality allocations = %d, want 1", allocated)
	}

	otherDatasetID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'Other workspace dataset','CURATED')
	`, otherDatasetID, otherWorkspaceID, "COST-OTHER-"+uuid.NewString()); err != nil {
		t.Fatalf("insert other dataset: %v", err)
	}
	otherVersionID := uuid.New()
	otherAssessmentID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://cost-allocation/other.csv',
			'text/csv','SHA256',repeat('b',64),'{}'::jsonb,now())
	`, otherVersionID, otherDatasetID); err != nil {
		t.Fatalf("insert other dataset version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics
		) VALUES ($1,$2,$3,'quality/cost-other.yaml','1.0.0',
			'c4b903018effbb6d36545f03ec3a6513aa3d5b35f12f42a50c150dc4f1ea35dc',
			'legacy-quality-fixture',
			'native-quality','1','PASS','{"dimensions":{}}'::jsonb)
	`, otherAssessmentID, otherWorkspaceID, otherVersionID); err != nil {
		t.Fatalf("insert other quality assessment: %v", err)
	}
	otherCostEventID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO cost_event(id, workspace_id, activity_id, cost_type, quantity, unit, pricing_mode)
		VALUES ($1,$2,$3,'QUALITY_ENGINE_INVOCATION',1,'assessment','ACTUAL')
	`, otherCostEventID, workspaceID, uuid.New()); err != nil {
		t.Fatalf("insert cross-workspace cost event: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cost_allocation(id, cost_event_id, quality_assessment_id)
		VALUES ($1,$2,$3)
	`, uuid.New(), otherCostEventID, otherAssessmentID); err == nil || !strings.Contains(err.Error(), "workspace boundary") {
		t.Fatalf("cross-workspace allocation error = %v, want workspace-boundary failure", err)
	}

	err := tryApplyMigrationFile(t, pool, 22, "down")
	if err == nil || !strings.Contains(err.Error(), "refusing destructive rollback of quality assessment cost allocation history") {
		t.Fatalf("quality cost allocation down error = %v, want historical-fact refusal", err)
	}
}
