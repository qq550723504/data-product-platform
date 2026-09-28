package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type productionExecutionFixture struct {
	ExecutionID    uuid.UUID
	InputVersionID uuid.UUID
}

func createProductionExecutionFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workspaceID, outputDatasetID uuid.UUID) productionExecutionFixture {
	t.Helper()
	inputDatasetID := uuid.New()
	inputVersionID := uuid.New()
	workflowID := uuid.New()
	workflowVersionID := uuid.New()
	executionID := uuid.New()

	mustExec(t, ctx, pool, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, lifecycle_status, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,'Production provenance input','RAW','ACTIVE','{}'::jsonb,now(),now())
	`, inputDatasetID, workspaceID, "PROVENANCE-INPUT-"+uuid.NewString())
	mustExec(t, ctx, pool, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri, content_type,
			checksum_algorithm, checksum_value, metadata, created_at, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE',$3,'text/csv','SHA256',$4,'{}'::jsonb,now(),now())
	`, inputVersionID, inputDatasetID, "s3://test-bucket/provenance-"+inputVersionID.String()+".csv", strings.Repeat("d", 64))
	mustExec(t, ctx, pool, `
		INSERT INTO workflow (id, workspace_id, code, name, status, created_at, updated_at)
		VALUES ($1,$2,$3,'Release production fixture','ACTIVE',now(),now())
	`, workflowID, workspaceID, "WF-PROVENANCE-"+uuid.NewString())
	mustExec(t, ctx, pool, `
		INSERT INTO workflow_version (id, workflow_id, version, definition_ref, definition_sha256, definition, created_at)
		VALUES ($1,$2,'1.0.0','test/release-production-fixture', $3, '{"processor":"native"}'::jsonb, now())
	`, workflowVersionID, workflowID, strings.Repeat("a", 64))
	mustExec(t, ctx, pool, `
		INSERT INTO execution (
			id, workspace_id, workflow_version_id, output_dataset_id, target_period,
			status, attempt, engine_type, metrics, created_at, started_at
		) VALUES ($1,$2,$3,$4,'2026-09','RUNNING',1,'NATIVE','{}'::jsonb,now(),now())
	`, executionID, workspaceID, workflowVersionID, outputDatasetID)
	for _, inputName := range []string{"enterprise_raw", "lease_raw", "energy_raw", "enterprise_resolution"} {
		mustExec(t, ctx, pool, `INSERT INTO execution_input (execution_id, input_name, dataset_version_id) VALUES ($1,$2,$3)`,
			executionID, inputName, inputVersionID)
	}
	for _, dep := range []struct{name string; dataset bool}{
		{"enterprise_resolution", true}, {"company_match_policy", false}, {"indicator_policy", false},
	} {
		var datasetVersion any
		if dep.dataset { datasetVersion = inputVersionID }
		mustExec(t, ctx, pool, `
			INSERT INTO execution_dependency_binding (
				execution_id, workspace_id, dependency_name, dataset_version_id, reference, version, content_sha256, content
			) VALUES ($1,$2,$3,$4,$5,'1.0.0',encode(digest(convert_to($6,'UTF8'),'sha256'),'hex'),convert_to($6,'UTF8'))
		`, executionID, workspaceID, dep.name, datasetVersion, "test/"+dep.name, "fixture-"+dep.name)
	}
	mustExec(t, ctx, pool, `
		INSERT INTO execution_dependency_preparation (execution_id, workspace_id, binding_fingerprint, mapping_usage_count, status)
		VALUES ($1,$2,$3,0,'PREPARED')
	`, executionID, workspaceID, strings.Repeat("b", 64))
	return productionExecutionFixture{ExecutionID: executionID, InputVersionID: inputVersionID}
}

func bindProductionOutputFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, fixture productionExecutionFixture, outputVersionID uuid.UUID) {
	t.Helper()
	mustExec(t, ctx, pool, `
		UPDATE execution
		SET output_dataset_version_id=$2, status='SUCCEEDED', finished_at=now()
		WHERE id=$1
	`, fixture.ExecutionID, outputVersionID)
	mustExec(t, ctx, pool, `
		INSERT INTO dataset_version_lineage (output_version_id, input_version_id, relation_type, execution_id)
		VALUES ($1,$2,'DERIVED_FROM',$3)
	`, outputVersionID, fixture.InputVersionID, fixture.ExecutionID)
}
