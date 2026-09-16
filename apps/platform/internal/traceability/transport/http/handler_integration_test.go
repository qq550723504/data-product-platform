package traceabilityhttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
)

func TestExecutionTraceabilityReturnsEvidenceAndCost(t *testing.T) {
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
	workflowID := uuid.New()
	workflowVersionID := uuid.New()
	executionID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (
			id, workspace_id, code, name, dataset_type, lifecycle_status, metadata,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,'CURATED','ACTIVE','{}'::jsonb,now(),now())
	`, datasetID, workspaceID, "TRACE-DATASET-"+uuid.NewString(), "Traceability dataset"); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow (id, workspace_id, code, name, status, created_at, updated_at)
		VALUES ($1,$2,$3,$4,'ACTIVE',now(),now())
	`, workflowID, workspaceID, "TRACE-WORKFLOW-"+uuid.NewString(), "Traceability workflow"); err != nil {
		t.Fatalf("insert workflow: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO workflow_version (
			id, workflow_id, version, definition_ref, definition_sha256, definition, created_at
		) VALUES ($1,$2,'1.0.0','test/traceability','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa','{}'::jsonb,now())
	`, workflowVersionID, workflowID); err != nil {
		t.Fatalf("insert workflow version: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO execution (
			id, workspace_id, workflow_version_id, output_dataset_id, target_period,
			status, attempt, engine_type, metrics, created_at, started_at, finished_at
		) VALUES ($1,$2,$3,$4,'2025-03','SUCCEEDED',1,'NATIVE','{}'::jsonb,now(),now(),now())
	`, executionID, workspaceID, workflowVersionID, datasetID); err != nil {
		t.Fatalf("insert execution: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin traceability fixture transaction: %v", err)
	}
	executionIDCopy := executionID
	if err := cost.Append(ctx, tx, cost.Event{
		WorkspaceID: workspaceID,
		ExecutionID: &executionIDCopy,
		CostType:    "PROCESSING_EXECUTION",
		Quantity:    1,
		Unit:        "execution",
		PricingMode: "POC_ESTIMATE",
		Metadata: map[string]any{
			"engineType": "NATIVE",
		},
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append cost event: %v", err)
	}
	if _, err := evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "PROCESSING_EXECUTION",
		Title:        "Traceability integration evidence",
		SourceType:   "EXECUTION",
		SourceID:     &executionIDCopy,
		Metadata: map[string]any{
			"workflowVersionId": workflowVersionID,
		},
	}, evidence.Relation{
		ObjectType:   "EXECUTION",
		ObjectID:     executionID,
		RelationType: "SUPPORTS",
	}); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("append evidence: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit traceability fixture transaction: %v", err)
	}

	handler := NewHandler(evidence.NewQueryRepository(pool), cost.NewQueryRepository(pool))
	mux := http.NewServeMux()
	handler.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/traceability/EXECUTION/"+executionID.String(), nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("traceability status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var body response
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode traceability response: %v", err)
	}
	if body.ObjectType != "EXECUTION" || body.ObjectID != executionID {
		t.Fatalf("traceability identity = %s/%s, want EXECUTION/%s", body.ObjectType, body.ObjectID, executionID)
	}
	if len(body.Evidence) != 1 {
		t.Fatalf("evidence items = %d, want 1", len(body.Evidence))
	}
	if body.Evidence[0].EvidenceType != "PROCESSING_EXECUTION" || body.Evidence[0].RelationType != "SUPPORTS" {
		t.Fatalf("unexpected evidence item: %#v", body.Evidence[0])
	}
	if len(body.CostEvents) != 1 {
		t.Fatalf("cost events = %d, want 1", len(body.CostEvents))
	}
	if body.CostEvents[0].CostType != "PROCESSING_EXECUTION" || body.CostEvents[0].Quantity != 1 {
		t.Fatalf("unexpected cost event: %#v", body.CostEvents[0])
	}
}
