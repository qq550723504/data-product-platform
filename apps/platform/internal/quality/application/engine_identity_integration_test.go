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
	qualityengine "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/engine"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

type neverCalledQualityEngine struct{}

func (neverCalledQualityEngine) Descriptor() qualityengine.Descriptor {
	return qualityengine.Descriptor{Name: "reference-engine", Version: "1"}
}
func (neverCalledQualityEngine) Evaluate(context.Context, qualityengine.Request) (qualityengine.Result, error) {
	return qualityengine.Result{}, errors.New("reference engine should not run for conflicting attempt identity")
}

func TestQualityAttemptIdentityIncludesEngine(t *testing.T) {
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
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENGINE-IDENTITY")
	version := uploadCSV(t, ctx, uploadDataset, dataset.ID, "engine-identity.csv", "company_id\nCOMPANY-001\n", nil)

	root := t.TempDir()
	content := []byte(`apiVersion: quality/v1
kind: QualityRuleSet
metadata:
  name: engine-identity
  version: 1.0.0
spec:
  rules:
    - id: QA-COMPANY-ID
      dimension: COMPLETENESS
      type: not_null
      target: company_id
      threshold: 1
      required: true
      severity: CRITICAL
  gate:
    criticalFailure: FAIL
    highFailure: REVIEW
    warningFailure: PASS_WITH_WARNING
`)
	if err := os.WriteFile(filepath.Join(root, "engine.yaml"), content, 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}

	service := qualityapp.NewService(root, txManager, datasetRepo, qualityinfra.NewPostgresRepository(pool), store)
	if err := service.RegisterEngine(neverCalledQualityEngine{}); err != nil {
		t.Fatalf("register reference engine: %v", err)
	}
	attemptID := uuid.New()
	if _, err := service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		AssessmentAttemptID: attemptID,
	}); err != nil {
		t.Fatalf("native assessment: %v", err)
	}

	_, err = service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		EngineName: "reference-engine", AssessmentAttemptID: attemptID,
	})
	if !errors.Is(err, qualityapp.ErrAssessmentAttemptConflict) {
		t.Fatalf("cross-engine attempt replay error = %v, want ErrAssessmentAttemptConflict", err)
	}

	var storedEngine string
	if err := pool.QueryRow(ctx, `
		SELECT engine_name FROM quality_assessment_attempt WHERE id=$1
	`, attemptID).Scan(&storedEngine); err != nil {
		t.Fatalf("read attempt engine: %v", err)
	}
	if storedEngine != "native-quality" {
		t.Fatalf("stored engine = %q, want native-quality", storedEngine)
	}
}
