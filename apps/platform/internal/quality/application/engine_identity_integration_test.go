package application_test

import (
	"context"
	"errors"
	"io"
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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
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

type malformedQualityEngine struct {
	calls int
}

func (*malformedQualityEngine) Descriptor() qualityengine.Descriptor {
	return qualityengine.Descriptor{Name: "malformed-engine", Version: "1", Capabilities: []string{"not_null"}}
}
func (e *malformedQualityEngine) Evaluate(context.Context, qualityengine.Request) (qualityengine.Result, error) {
	e.calls++
	return qualityengine.Result{
		Findings: []domain.Finding{{
			RuleID: "https://provider.internal?token=secret",
			Status: domain.FindingPass,
		}},
		Metrics: map[string]any{"provider": "ignored"},
	}, nil
}

type objectReadFailStore struct{}

func (objectReadFailStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("object store offline before provider invocation")
}

func TestLocalPreparationFailureDoesNotCreateInvocationCost(t *testing.T) {
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
	uploadStore := newMemoryStore()
	createDataset := datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceinfra.NewPostgresRepository())
	uploadDataset := datasetapp.NewUploadVersionService(txManager, datasetRepo, uploadStore)
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENGINE-PREP")
	version := uploadCSV(t, ctx, uploadDataset, dataset.ID, "engine-prep.csv", "company_id\nCOMPANY-001\n", nil)

	root := t.TempDir()
	content := []byte(`apiVersion: quality/v1
kind: QualityRuleSet
metadata:
  name: engine-prep
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

	service := qualityapp.NewService(root, txManager, datasetRepo, qualityinfra.NewPostgresRepository(pool), objectReadFailStore{})
	attemptID := uuid.New()
	_, err = service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		AssessmentAttemptID: attemptID,
	})
	if err == nil || !strings.Contains(err.Error(), "object store offline") {
		t.Fatalf("local preparation error = %v", err)
	}

	var attemptCount, costCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quality_assessment_attempt WHERE id=$1`, attemptID).Scan(&attemptCount); err != nil {
		t.Fatalf("count preparation attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE a.quality_assessment_attempt_id=$1
	`, attemptID).Scan(&costCount); err != nil {
		t.Fatalf("count preparation costs: %v", err)
	}
	if attemptCount != 0 || costCount != 0 {
		t.Fatalf("local preparation created attempt/cost = %d/%d, want 0/0", attemptCount, costCount)
	}
}

func TestMalformedEngineResultIsSanitizedAndFailsAttempt(t *testing.T) {
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
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENGINE-MALFORMED")
	version := uploadCSV(t, ctx, uploadDataset, dataset.ID, "engine-malformed.csv", "company_id\nCOMPANY-001\n", nil)

	root := t.TempDir()
	content := []byte(`apiVersion: quality/v1
kind: QualityRuleSet
metadata:
  name: engine-malformed
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
	provider := &malformedQualityEngine{}
	if err := service.RegisterEngine(provider); err != nil {
		t.Fatalf("register malformed engine: %v", err)
	}
	attemptID := uuid.New()
	_, err = service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		EngineName: "malformed-engine", AssessmentAttemptID: attemptID,
	})
	if err == nil || !strings.Contains(err.Error(), "PROVIDER_INVALID_RESPONSE") ||
		strings.Contains(err.Error(), "provider.internal") {
		t.Fatalf("malformed provider error was not sanitized: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}

	var persistedError, outcome string
	if err := pool.QueryRow(ctx, `
		SELECT outcome, COALESCE(error_message,'')
		FROM quality_assessment_attempt_outcome
		WHERE attempt_id=$1
	`, attemptID).Scan(&outcome, &persistedError); err != nil {
		t.Fatalf("read malformed attempt outcome: %v", err)
	}
	if outcome != "FAILED" || !strings.Contains(persistedError, "PROVIDER_INVALID_RESPONSE") ||
		strings.Contains(persistedError, "provider.internal") {
		t.Fatalf("malformed persisted error = %q/%q", outcome, persistedError)
	}
	var costCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE a.quality_assessment_attempt_id=$1
	`, attemptID).Scan(&costCount); err != nil {
		t.Fatalf("count malformed invocation cost: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("malformed provider invocation cost = %d, want 1", costCount)
	}
}

func TestUnsupportedEngineCapabilityDoesNotCreateInvocationCost(t *testing.T) {
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
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENGINE-CAPABILITY")
	version := uploadCSV(t, ctx, uploadDataset, dataset.ID, "engine-capability.csv", "company_id\nCOMPANY-001\n", nil)

	root := t.TempDir()
	content := []byte(`apiVersion: quality/v1
kind: QualityRuleSet
metadata:
  name: engine-capability
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
	_, err = service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		EngineName: "reference-engine", AssessmentAttemptID: attemptID,
	})
	if err == nil || !strings.Contains(err.Error(), "does not support rule type not_null") {
		t.Fatalf("capability preflight error = %v", err)
	}

	var attemptCount, costCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quality_assessment_attempt WHERE id=$1`, attemptID).Scan(&attemptCount); err != nil {
		t.Fatalf("count preflight attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE a.quality_assessment_attempt_id=$1
	`, attemptID).Scan(&costCount); err != nil {
		t.Fatalf("count preflight costs: %v", err)
	}
	if attemptCount != 0 || costCount != 0 {
		t.Fatalf("capability preflight created attempt/cost = %d/%d, want 0/0", attemptCount, costCount)
	}
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

func TestInvalidUTF8ExternalEngineInputFailsBeforeAttemptClaim(t *testing.T) {
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
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENGINE-UTF8")

	version, err := uploadDataset.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:      dataset.ID,
		Filename:       "invalid-utf8.csv",
		ContentType:    "text/csv",
		Content:        []byte{'c', 'o', 'm', 'p', 'a', 'n', 'y', '_', 'i', 'd', '
', 0xff, '
'},
		IdempotencyKey: "engine-invalid-utf8-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("upload invalid UTF-8 fixture: %v", err)
	}

	root := t.TempDir()
	content := []byte(`apiVersion: quality/v1
kind: QualityRuleSet
metadata:
  name: engine-utf8
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
		t.Fatalf("register external engine: %v", err)
	}
	attemptID := uuid.New()
	_, err = service.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID, RuleSetRef: "engine.yaml",
		EngineName: "failing-engine", AssessmentAttemptID: attemptID,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("invalid UTF-8 preflight error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", provider.calls)
	}

	var attemptCount, costCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quality_assessment_attempt WHERE id=$1`, attemptID).Scan(&attemptCount); err != nil {
		t.Fatalf("count UTF-8 attempts: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE a.quality_assessment_attempt_id=$1
	`, attemptID).Scan(&costCount); err != nil {
		t.Fatalf("count UTF-8 costs: %v", err)
	}
	if attemptCount != 0 || costCount != 0 {
		t.Fatalf("invalid UTF-8 created attempt/cost = %d/%d, want 0/0", attemptCount, costCount)
	}
}
