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
	complianceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	compliancedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
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
	// workspace's DatasetVersion must be rejected without writing result/evidence facts.
	foreignWorkspaceID := uuid.New()
	foreignDataset := createDatasetForTest(t, ctx, createDataset, foreignWorkspaceID, "GOV-FOREIGN")
	foreignVersion := uploadCSV(t, ctx, uploadDataset, foreignDataset.ID, "product-foreign.csv", `company_id,company_name,period,tenancy_stability,rent_performance,energy_stability,activity_score,activity_level,indicator_coverage,generated_at
COMPANY-004,外部科技有限公司,2026-09,90,95,80,88,HIGH,100,2026-09-16T10:00:00Z
`, map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0})
	if _, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: foreignVersion.ID,
		RuleSetRef:       "park/quality/enterprise-activity-quality-v1.yaml",
		TraceID:          "governance-e2e",
		Now:              foreignVersion.ReadyAt.Add(30 * 60 * 1e9),
	}); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("cross workspace quality error = %v, want ErrDatasetWorkspace", err)
	}
	var foreignQualityResults int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM quality_result WHERE workspace_id=$1`, foreignWorkspaceID).Scan(&foreignQualityResults); err != nil {
		t.Fatalf("count rejected foreign quality results: %v", err)
	}
	if foreignQualityResults != 0 {
		t.Fatal("rejected cross workspace quality check persisted a result")
	}
	if _, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: foreignVersion.ID,
		PolicyRef:        "park/compliance/enterprise-activity-compliance-v1.yaml",
		TraceID:          "governance-e2e",
	}); !errors.Is(err, datasetdomain.ErrDatasetWorkspace) {
		t.Fatalf("cross workspace compliance error = %v, want ErrDatasetWorkspace", err)
	}
	var foreignComplianceResults int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM compliance_result WHERE workspace_id=$1`, foreignWorkspaceID).Scan(&foreignComplianceResults); err != nil {
		t.Fatalf("count rejected foreign compliance results: %v", err)
	}
	if foreignComplianceResults != 0 {
		t.Fatal("rejected cross workspace compliance check persisted a result")
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE evidence_type IN ('QUALITY_RESULT','COMPLIANCE_RESULT') AND metadata->>'datasetVersionId'=$1`, passVersion.ID.String()).Scan(&evidenceCount); err != nil {
		t.Fatalf("count governance evidence: %v", err)
	}
	if evidenceCount != 2 {
		t.Fatalf("governance evidence = %d, want 2", evidenceCount)
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
