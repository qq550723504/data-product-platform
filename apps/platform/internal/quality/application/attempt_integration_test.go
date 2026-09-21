package application_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	beforeInvocation := time.Now().UTC()
	_, err = qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    version.ID,
		RuleSetRef:          "unknown-rule.yaml",
		AssessmentAttemptID: attemptID,
		Now:                 beforeInvocation.Add(24 * time.Hour),
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
	var startedAt, costOccurredAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT started_at
		FROM quality_assessment_attempt
		WHERE id=$1
	`, attemptID).Scan(&startedAt); err != nil {
		t.Fatalf("read attempt start time: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT e.occurred_at
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		WHERE a.quality_assessment_attempt_id=$1
	`, attemptID).Scan(&costOccurredAt); err != nil {
		t.Fatalf("read attempt cost time: %v", err)
	}
	if startedAt.Before(beforeInvocation.Add(-time.Second)) || startedAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("attempt started_at = %s, want wall-clock invocation time near %s", startedAt, beforeInvocation)
	}
	if costOccurredAt.Before(beforeInvocation.Add(-time.Second)) || costOccurredAt.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("attempt cost occurred_at = %s, want wall-clock invocation time near %s", costOccurredAt, beforeInvocation)
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

func TestQualityAttemptReconcilesExpiredClaim(t *testing.T) {
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
	dataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ATTEMPT-EXPIRED")
	version := uploadCSV(t, ctx, uploadDataset, dataset.ID, "attempt-expired.csv", "company_id\nCOMPANY-001\n", nil)
	qualityRepo := qualityinfra.NewPostgresRepository(pool)
	qualityService := qualityapp.NewService(t.TempDir(), txManager, datasetRepo, qualityRepo, store)
	attemptID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_assessment_attempt (
			id, workspace_id, dataset_version_id, rule_set_ref,
			started_at, lease_expires_at
		) VALUES ($1,$2,$3,$4,now() - interval '2 hours',now() - interval '1 minute')
	`, attemptID, workspaceID, version.ID, "unknown-rule.yaml"); err != nil {
		t.Fatalf("insert expired quality attempt: %v", err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire attempt owner connection: %v", err)
	}
	lockKey := fmt.Sprintf("quality-assessment-attempt:%s", attemptID)
	var locked bool
	if err := conn.QueryRow(ctx, `
		SELECT pg_try_advisory_lock(hashtextextended($1, 0))
	`, lockKey).Scan(&locked); err != nil {
		conn.Release()
		t.Fatalf("acquire attempt owner lock: %v", err)
	}
	if !locked {
		conn.Release()
		t.Fatal("attempt owner lock was not acquired")
	}
	_, replayErr := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID,
		RuleSetRef: "unknown-rule.yaml", AssessmentAttemptID: attemptID,
	})
	if !errors.Is(replayErr, qualityapp.ErrAssessmentAttemptInProgress) {
		conn.Release()
		t.Fatalf("live expired-attempt replay error = %v, want ErrAssessmentAttemptInProgress", replayErr)
	}
	var unlocked bool
	if err := conn.QueryRow(ctx, `
		SELECT pg_advisory_unlock(hashtextextended($1, 0))
	`, lockKey).Scan(&unlocked); err != nil || !unlocked {
		conn.Release()
		t.Fatalf("release attempt owner lock: %v", err)
	}
	conn.Release()
	_, replayErr = qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: version.ID,
		RuleSetRef: "unknown-rule.yaml", AssessmentAttemptID: attemptID,
	})
	if !errors.Is(replayErr, qualityapp.ErrAssessmentAttemptFailed) {
		t.Fatalf("expired-attempt replay error = %v, want ErrAssessmentAttemptFailed", replayErr)
	}
	var outcome string
	if err := pool.QueryRow(ctx, `
		SELECT outcome
		FROM quality_assessment_attempt_outcome
		WHERE attempt_id=$1
	`, attemptID).Scan(&outcome); err != nil {
		t.Fatalf("read reconciled attempt outcome: %v", err)
	}
	if outcome != "FAILED" {
		t.Fatalf("reconciled attempt outcome = %q, want FAILED", outcome)
	}
}
