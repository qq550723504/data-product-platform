package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

type failingQualityEngine struct {
	calls int
}

func (*failingQualityEngine) Descriptor() qualityengine.Descriptor {
	return qualityengine.Descriptor{Name: "failing-engine", Version: "1", Capabilities: []string{"not_null"}}
}
func (e *failingQualityEngine) Evaluate(context.Context, qualityengine.Request) (qualityengine.Result, error) {
	e.calls++
	return qualityengine.Result{}, errors.New("provider execution unavailable")
}

func TestQualityEngineFailureDoesNotBecomeRuleFailure(t *testing.T) {
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
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENGINE-FAILURE")
	version := uploadCSV(t, ctx, uploadDataset, dataset.ID, "engine-failure.csv", "company_id\nCOMPANY-001\n", nil)

	root := t.TempDir()
	content := []byte(`apiVersion: quality/v1
kind: QualityRuleSet
metadata:
  name: engine-failure
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
	provider := &failingQualityEngine{}
	if err := service.RegisterEngine(provider); err != nil {
		t.Fatalf("register failing engine: %v", err)
	}
	attemptID := uuid.New()
	_, err = service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		EngineName: "failing-engine", AssessmentAttemptID: attemptID,
	})
	if err == nil || !strings.Contains(err.Error(), "PROVIDER_EXECUTION_FAILED") ||
		strings.Contains(err.Error(), "provider execution unavailable") {
		t.Fatalf("engine failure error was not sanitized: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}

	var assessmentCount, findingCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM quality_result
		WHERE workspace_id=$1 AND dataset_version_id=$2
	`, workspaceID, version.ID).Scan(&assessmentCount); err != nil {
		t.Fatalf("count quality assessments: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM quality_finding f
		JOIN quality_result r ON r.id=f.result_id
		WHERE r.workspace_id=$1 AND r.dataset_version_id=$2
	`, workspaceID, version.ID).Scan(&findingCount); err != nil {
		t.Fatalf("count quality findings: %v", err)
	}
	if assessmentCount != 0 || findingCount != 0 {
		t.Fatalf("provider failure created assessment/findings = %d/%d, want 0/0", assessmentCount, findingCount)
	}

	var outcome, storedEngine, storedVersion string
	if err := pool.QueryRow(ctx, `
		SELECT a.engine_name, a.engine_version, o.outcome
		FROM quality_assessment_attempt a
		JOIN quality_assessment_attempt_outcome o ON o.attempt_id=a.id
		WHERE a.id=$1
	`, attemptID).Scan(&storedEngine, &storedVersion, &outcome); err != nil {
		t.Fatalf("read failed attempt: %v", err)
	}
	if storedEngine != "failing-engine" || storedVersion != "1" || outcome != "FAILED" {
		t.Fatalf("failed attempt = engine %q/%q outcome %q", storedEngine, storedVersion, outcome)
	}
	var persistedError string
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(error_message,'')
		FROM quality_assessment_attempt_outcome
		WHERE attempt_id=$1
	`, attemptID).Scan(&persistedError); err != nil {
		t.Fatalf("read failed attempt error: %v", err)
	}
	if !strings.Contains(persistedError, "PROVIDER_EXECUTION_FAILED") ||
		strings.Contains(persistedError, "provider execution unavailable") {
		t.Fatalf("persisted provider error was not sanitized: %q", persistedError)
	}

	_, replayErr := service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		EngineName: "failing-engine", AssessmentAttemptID: attemptID,
	})
	if !errors.Is(replayErr, qualityapp.ErrAssessmentAttemptFailed) {
		t.Fatalf("failed engine replay error = %v, want ErrAssessmentAttemptFailed", replayErr)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls after replay = %d, want 1", provider.calls)
	}
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

	var storedEngine, storedVersion string
	if err := pool.QueryRow(ctx, `
		SELECT engine_name, engine_version FROM quality_assessment_attempt WHERE id=$1
	`, attemptID).Scan(&storedEngine, &storedVersion); err != nil {
		t.Fatalf("read attempt engine: %v", err)
	}
	if storedEngine != "native-quality" || storedVersion != "2" {
		t.Fatalf("stored engine = %q/%q, want native-quality/2", storedEngine, storedVersion)
	}
}
