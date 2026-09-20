package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

func TestQualityAttemptRecordsCostBeforeEvaluationFailure(t *testing.T) {
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
	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	store := newMemoryStore()
	createDataset := datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := datasetapp.NewUploadVersionService(txManager, datasetRepo, store)
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ATTEMPT-FAILURE")
	version := uploadCSV(t, ctx, uploadDataset, dataset.ID, "attempt-failure.csv", "company_id\nCOMPANY-001\n", nil)

	industryPackRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(industryPackRoot, "unknown-rule.yaml"), []byte(`apiVersion: quality/v1
kind: QualityPolicy
metadata:
  name: unknown-rule
  version: 1.0.0
spec:
  rules:
    - id: QA-UNKNOWN-RULE
      dimension: completeness
      severity: HIGH
`), 0o600); err != nil {
		t.Fatalf("write failing quality policy: %v", err)
	}

	qualityRepo := qualityinfra.NewPostgresRepository(pool)
	qualityService := qualityapp.NewService(industryPackRoot, txManager, datasetRepo, qualityRepo, store)
	attemptID := uuid.New()
	_, err = qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    version.ID,
		RuleSetRef:          "unknown-rule.yaml",
		AssessmentAttemptID: attemptID,
	})
	if err == nil {
		t.Fatal("quality evaluation unexpectedly succeeded")
	}

	var costCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE a.quality_assessment_attempt_id=$1
	`, attemptID).Scan(&costCount); err != nil {
		t.Fatalf("count failed-attempt costs: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("failed quality attempt costs = %d, want 1", costCount)
	}

	var outcome, errorMessage string
	if err := pool.QueryRow(ctx, `
		SELECT outcome, COALESCE(error_message,'')
		FROM quality_assessment_attempt_outcome
		WHERE attempt_id=$1
	`, attemptID).Scan(&outcome, &errorMessage); err != nil {
		t.Fatalf("read failed-attempt outcome: %v", err)
	}
	if outcome != "FAILED" || errorMessage == "" {
		t.Fatalf("failed-attempt outcome = %q/%q, want FAILED with error", outcome, errorMessage)
	}

	_, replayErr := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    version.ID,
		RuleSetRef:          "unknown-rule.yaml",
		AssessmentAttemptID: attemptID,
	})
	if !errors.Is(replayErr, qualityapp.ErrAssessmentAttemptFailed) {
		t.Fatalf("failed-attempt replay error = %v, want ErrAssessmentAttemptFailed", replayErr)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE a.quality_assessment_attempt_id=$1
	`, attemptID).Scan(&costCount); err != nil {
		t.Fatalf("recount failed-attempt costs: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("failed quality attempt costs after replay = %d, want 1", costCount)
	}
}
