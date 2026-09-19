package application_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	complianceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	compliancedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	qualitynative "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/native"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

type memoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemoryStore() *memoryStore {
	return &memoryStore{objects: map[string][]byte{}}
}

func (s *memoryStore) Put(_ context.Context, objectName string, reader io.Reader, _ int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	uri := "s3://test-bucket/" + objectName
	s.mu.Lock()
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
	return uri, nil
}

func (s *memoryStore) Get(_ context.Context, storageURI string) (io.ReadCloser, error) {
	s.mu.Lock()
	content := append([]byte(nil), s.objects[storageURI]...)
	s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(content)), nil
}

func TestNativeQualityAndComplianceGates(t *testing.T) {
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
	industryPackRoot := repoPath(t, "industry-packs")

	qualityRepo := qualityinfra.NewPostgresRepository(pool)
	qualityService := qualityapp.NewService(industryPackRoot, txManager, datasetRepo, qualityRepo, store)
	complianceRepo := complianceinfra.NewPostgresRepository(pool)
	complianceService := complianceapp.NewService(industryPackRoot, txManager, datasetRepo, complianceRepo, store)

	passDataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "GOV-PASS")
	passVersion := uploadCSV(t, ctx, uploadDataset, passDataset.ID, "product-pass.csv", `company_id,company_name,period,tenancy_stability,rent_performance,energy_stability,activity_score,activity_level,indicator_coverage,generated_at
COMPANY-001,示例科技有限公司,2026-09,90,95,80,88,HIGH,100,2026-09-16T10:00:00Z
`, map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0})

	qualityResult, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: passVersion.ID,
		RuleSetRef:       "park/quality/enterprise-activity-quality-v1.yaml",
		TraceID:          "governance-e2e",
		Now:              passVersion.ReadyAt.Add(30 * 60 * 1e9),
	})
	if err != nil {
		t.Fatalf("run quality gate: %v", err)
	}
	if qualityResult.GateDecision != qualitydomain.GatePass {
		t.Fatalf("quality gate = %s, want PASS; findings=%+v", qualityResult.GateDecision, qualityResult.Findings)
	}
	policyPath := filepath.Join(industryPackRoot, "park/quality/enterprise-activity-quality-v1.yaml")
	policy, err := qualitynative.LoadPolicy(policyPath)
	if err != nil {
		t.Fatalf("load quality policy snapshot expectation: %v", err)
	}
	if qualityResult.RuleSetContentSHA256 != policy.SourceContentSHA256 || qualityResult.RuleSetContent != policy.SourceContent {
		t.Fatalf("assessment did not capture exact rule snapshot")
	}
	if qualityResult.EvaluatorName != qualitynative.EvaluatorName || qualityResult.EvaluatorVersion != qualitynative.EvaluatorVersion {
		t.Fatalf("assessment evaluator = %s/%s, want %s/%s", qualityResult.EvaluatorName, qualityResult.EvaluatorVersion, qualitynative.EvaluatorName, qualitynative.EvaluatorVersion)
	}
	assessment, err := qualityRepo.GetAssessment(ctx, qualityResult.ID)
	if err != nil {
		t.Fatalf("query assessment by id: %v", err)
	}
	if assessment.RuleSetContent != policy.SourceContent || assessment.RuleSetContentSHA256 != policy.SourceContentSHA256 {
		t.Fatalf("queried assessment lost its rule snapshot")
	}
	evidenceItems, err := evidence.NewQueryRepository(pool).ListForObject(ctx, "QUALITY_RESULT", qualityResult.ID)
	if err != nil || len(evidenceItems) != 1 {
		t.Fatalf("assessment evidence = %d, err=%v; want one", len(evidenceItems), err)
	}
	auditEvents, err := qualityRepo.ListAuditEvents(ctx, qualityResult.ID)
	if err != nil || len(auditEvents) != 1 {
		t.Fatalf("assessment audit events = %d, err=%v; want one", len(auditEvents), err)
	}
	assessments, err := qualityRepo.ListAssessments(ctx, passVersion.ID)
	if err != nil || len(assessments) != 1 {
		t.Fatalf("list assessments = %d, err=%v; want one", len(assessments), err)
	}
	latest, err := qualityRepo.LatestAssessment(ctx, passVersion.ID)
	if err != nil || latest.ID != qualityResult.ID {
		t.Fatalf("latest assessment = %s, err=%v; want %s", latest.ID, err, qualityResult.ID)
	}
	if _, err := pool.Exec(ctx, `UPDATE quality_result SET rule_set_content='tampered' WHERE id=$1`, qualityResult.ID); err == nil {
		t.Fatal("direct quality assessment update was accepted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM quality_result WHERE id=$1`, qualityResult.ID); err == nil {
		t.Fatal("direct quality assessment delete was accepted")
	}
	var findingID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM quality_finding WHERE result_id=$1 ORDER BY id LIMIT 1`, qualityResult.ID).Scan(&findingID); err != nil {
		t.Fatalf("find assessment finding: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE quality_finding SET message='tampered' WHERE id=$1`, findingID); err == nil {
		t.Fatal("direct quality finding update was accepted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM quality_finding WHERE id=$1`, findingID); err == nil {
		t.Fatal("direct quality finding delete was accepted")
	}
	secondAssessment, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: passVersion.ID,
		RuleSetRef: "park/quality/enterprise-activity-quality-v1.yaml",
		TraceID:    "governance-e2e-second-assessment", Now: passVersion.ReadyAt.Add(31 * 60 * 1e9),
	})
	if err != nil {
		t.Fatalf("run second assessment: %v", err)
	}
	assessments, err = qualityRepo.ListAssessments(ctx, passVersion.ID)
	if err != nil || len(assessments) != 2 {
		t.Fatalf("assessment history = %d, err=%v; want two", len(assessments), err)
	}
	latest, err = qualityRepo.LatestAssessment(ctx, passVersion.ID)
	if err != nil || latest.ID != secondAssessment.ID {
		t.Fatalf("latest second assessment = %s, err=%v; want %s", latest.ID, err, secondAssessment.ID)
	}

	complianceResult, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: passVersion.ID,
		PolicyRef:        "park/compliance/enterprise-activity-compliance-v1.yaml",
		TraceID:          "governance-e2e",
	})
	if err != nil {
		t.Fatalf("run compliance gate: %v", err)
	}
	if complianceResult.GateDecision != compliancedomain.GatePass {
		t.Fatalf("compliance gate = %s, want PASS; findings=%+v", complianceResult.GateDecision, complianceResult.Findings)
	}

	badQualityDataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "GOV-BAD-QUALITY")
	badQualityVersion := uploadCSV(t, ctx, uploadDataset, badQualityDataset.ID, "product-bad-quality.csv", `company_id,company_name,period,tenancy_stability,rent_performance,energy_stability,activity_score,activity_level,indicator_coverage,generated_at
COMPANY-002,异常科技有限公司,2026-09,90,95,80,120,HIGH,100,2026-09-16T10:00:00Z
`, map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0})
	badQualityResult, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: badQualityVersion.ID,
		RuleSetRef:       "park/quality/enterprise-activity-quality-v1.yaml",
		TraceID:          "governance-e2e",
		Now:              badQualityVersion.ReadyAt.Add(30 * 60 * 1e9),
	})
	if err != nil {
		t.Fatalf("run bad quality gate: %v", err)
	}
	if badQualityResult.GateDecision != qualitydomain.GateFail {
		t.Fatalf("bad quality gate = %s, want FAIL", badQualityResult.GateDecision)
	}

	badComplianceDataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "GOV-BAD-COMPLIANCE")
	badComplianceVersion := uploadCSV(t, ctx, uploadDataset, badComplianceDataset.ID, "product-bad-compliance.csv", `company_id,company_name,period,tenancy_stability,rent_performance,energy_stability,activity_score,activity_level,indicator_coverage,generated_at,mobile
COMPANY-003,敏感科技有限公司,2026-09,90,95,80,88,HIGH,100,2026-09-16T10:00:00Z,13800000000
`, map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0})
	badComplianceResult, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: badComplianceVersion.ID,
		PolicyRef:        "park/compliance/enterprise-activity-compliance-v1.yaml",
		TraceID:          "governance-e2e",
	})
	if err != nil {
		t.Fatalf("run bad compliance gate: %v", err)
	}
	if badComplianceResult.GateDecision != compliancedomain.GateFail {
		t.Fatalf("bad compliance gate = %s, want FAIL", badComplianceResult.GateDecision)
	}

	// A QualityResult workspace is derived from the Dataset. A caller naming another
	// workspace's DatasetVersion must be rejected without writing result/evidence/audit facts.
	foreignWorkspaceID := uuid.New()
	foreignDataset := createDatasetForTest(t, ctx, createDataset, foreignWorkspaceID, "GOV-FOREIGN")
	foreignVersion := uploadCSV(t, ctx, uploadDataset, foreignDataset.ID, "product-foreign.csv", `company_id,company_name,period,tenancy_stability,rent_performance,energy_stability,activity_score,activity_level,indicator_coverage,generated_at
COMPANY-004,外部科技有限公司,2026-09,90,95,80,88,HIGH,100,2026-09-16T10:00:00Z
`, map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0})
	if _, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: foreignVersion.ID,
		RuleSetRef:       "park/quality/enterprise-activity-quality-v1.yaml",
		TraceID:          "governance-rejected",
		Now:              foreignVersion.ReadyAt.Add(30 * 60 * 1e9),
	}); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("cross workspace quality error = %v, want ErrDatasetWorkspace", err)
	}
	if _, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: foreignVersion.ID,
		PolicyRef:        "park/compliance/enterprise-activity-compliance-v1.yaml",
		TraceID:          "governance-rejected",
	}); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("cross workspace compliance error = %v, want ErrDatasetWorkspace", err)
	}
	// Absence must be scoped by the rejected DatasetVersion, not by the workspace the caller
	// happened to name: a faulty path could persist the result under the request workspace and
	// still leave a foreign-workspace count at zero.
	assertNoGovernanceFactsForVersion(t, ctx, pool, foreignVersion.ID)

	// Ownership must be resolved before the status check: a foreign version that is no
	// longer READY must still be rejected as a workspace mismatch, not surface its status
	// through the generic QUALITY_CHECK_FAILED / COMPLIANCE_CHECK_FAILED path.
	invalidateVersion := datasetapp.NewInvalidateVersionService(txManager, datasetRepo)
	if _, err := invalidateVersion.Handle(ctx, datasetapp.InvalidateVersionCommand{VersionID: foreignVersion.ID, Reason: "governance-ordering", TraceID: "governance-rejected"}); err != nil {
		t.Fatalf("invalidate foreign version: %v", err)
	}
	if _, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: foreignVersion.ID,
		RuleSetRef:       "park/quality/enterprise-activity-quality-v1.yaml",
		TraceID:          "governance-rejected",
		Now:              foreignVersion.ReadyAt.Add(30 * 60 * 1e9),
	}); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("foreign non-READY quality error = %v, want ErrDatasetWorkspace", err)
	}
	if _, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: foreignVersion.ID,
		PolicyRef:        "park/compliance/enterprise-activity-compliance-v1.yaml",
		TraceID:          "governance-rejected",
	}); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("foreign non-READY compliance error = %v, want ErrDatasetWorkspace", err)
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE evidence_type IN ('QUALITY_RESULT','COMPLIANCE_RESULT') AND metadata->>'datasetVersionId'=$1`, passVersion.ID.String()).Scan(&evidenceCount); err != nil {
		t.Fatalf("count governance evidence: %v", err)
	}
	if evidenceCount != 3 {
		t.Fatalf("governance evidence = %d, want 3", evidenceCount)
	}
}

func assertNoGovernanceFactsForVersion(t *testing.T, ctx context.Context, pool *pgxpool.Pool, versionID uuid.UUID) {
	t.Helper()
	for _, query := range []string{
		`SELECT count(*) FROM quality_result WHERE dataset_version_id=$1`,
		`SELECT count(*) FROM compliance_result WHERE dataset_version_id=$1`,
	} {
		var count int
		if err := pool.QueryRow(ctx, query, versionID).Scan(&count); err != nil {
			t.Fatalf("count rejected governance rows (%s): %v", query, err)
		}
		if count != 0 {
			t.Fatalf("rejected cross workspace run persisted %d row(s) for %s", count, query)
		}
	}
	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE metadata->>'datasetVersionId'=$1`, versionID.String()).Scan(&evidenceCount); err != nil {
		t.Fatalf("count rejected evidence: %v", err)
	}
	if evidenceCount != 0 {
		t.Fatalf("rejected cross workspace run persisted %d evidence row(s)", evidenceCount)
	}
	var auditCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_event WHERE after_state->>'datasetVersionId'=$1`, versionID.String()).Scan(&auditCount); err != nil {
		t.Fatalf("count rejected audit events: %v", err)
	}
	if auditCount != 0 {
		t.Fatalf("rejected cross workspace run persisted %d audit event(s)", auditCount)
	}
}

func createDatasetForTest(t *testing.T, ctx context.Context, service *datasetapp.CreateDatasetService, workspaceID uuid.UUID, code string) datasetdomain.Dataset {
	t.Helper()
	dataset, err := service.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        code + "-" + uuid.NewString(),
		Name:        code,
		DatasetType: datasetdomain.DatasetTypeCurated,
		TraceID:     "governance-e2e",
	})
	if err != nil {
		t.Fatalf("create dataset: %v", err)
	}
	return dataset
}

func uploadCSV(t *testing.T, ctx context.Context, service *datasetapp.UploadVersionService, datasetID uuid.UUID, filename, content string, metadata map[string]any) datasetdomain.DatasetVersion {
	t.Helper()
	version, err := service.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   datasetID,
		Filename:    filename,
		ContentType: "text/csv",
		Content:     []byte(content),
		Metadata:    metadata,
		TraceID:     "governance-e2e",
	})
	if err != nil {
		t.Fatalf("upload CSV: %v", err)
	}
	return version
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../"))
	return filepath.Join(append([]string{root}, parts...)...)
}
