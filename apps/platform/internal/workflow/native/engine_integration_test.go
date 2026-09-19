package native_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflowNative "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
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

func (s *memoryStore) bytes(storageURI string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.objects[storageURI]...)
}

func TestEnterpriseActivityNativeWorkerProducesCuratedDataset(t *testing.T) {
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
	entityRepo := entityinfra.NewPostgresRepository(pool)
	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	store := newMemoryStore()
	createDataset := datasetapp.NewCreateDatasetService(txManager, datasetRepo)
	uploadDataset := datasetapp.NewUploadVersionService(txManager, datasetRepo, store)

	enterpriseDataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENTERPRISE-RAW", "Enterprise RAW", datasetdomain.DatasetTypeRaw)
	leaseDataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "LEASE-RAW", "Lease RAW", datasetdomain.DatasetTypeRaw)
	energyDataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENERGY-RAW", "Energy RAW", datasetdomain.DatasetTypeRaw)
	enterpriseStandardized := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENTERPRISE-STANDARDIZED", "Enterprise standardized", datasetdomain.DatasetTypeStandardized)
	activityDataset := createDatasetForTest(t, ctx, createDataset, workspaceID, "ENTERPRISE-ACTIVITY", "Enterprise activity", datasetdomain.DatasetTypeCurated)

	enterpriseVersion := uploadFixture(t, ctx, uploadDataset, enterpriseDataset.ID, "enterprise.csv")
	leaseVersion := uploadFixture(t, ctx, uploadDataset, leaseDataset.ID, "lease.csv")
	energyVersion := uploadFixture(t, ctx, uploadDataset, energyDataset.ID, "energy.csv")

	industryPackRoot := repoPath(t, "industry-packs")
	matchService := entityapp.NewMatchService(industryPackRoot, txManager, entityRepo, datasetRepo, uploadDataset, store)
	matchJob, err := matchService.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: enterpriseVersion.ID,
		OutputDatasetID:       enterpriseStandardized.ID,
		SourceType:            "CSV",
		SourceRef:             "enterprise.csv",
		SourceRole:            entitydomain.SourceAnchor,
		PolicyRef:             "park/matching/company-match-policy-v1.yaml",
		TraceID:               "native-worker-e2e",
	})
	if err != nil {
		t.Fatalf("start enterprise entity resolution: %v", err)
	}
	if matchJob.Status == entitydomain.JobWaitingReview {
		candidates, err := entityRepo.ListCandidates(ctx, matchJob.ID)
		if err != nil {
			t.Fatalf("list entity candidates: %v", err)
		}
		reviewerID := uuid.New()
		for _, candidate := range candidates {
			if candidate.Status != entitydomain.CandidatePending {
				continue
			}
			matchJob, err = matchService.Confirm(ctx, entityapp.ReviewCommand{
				CandidateID: candidate.ID,
				ReviewerID:  reviewerID,
				Reason:      "reference fixture alias confirmed for end-to-end workflow test",
				TraceID:     "native-worker-e2e",
			})
			if err != nil {
				t.Fatalf("confirm entity candidate: %v", err)
			}
		}
	}
	if matchJob.Status != entitydomain.JobSucceeded {
		t.Fatalf("entity match job status = %s, want SUCCEEDED", matchJob.Status)
	}
	originalMapping, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", "enterprise.csv", "ENT-001")
	if err != nil || originalMapping.CurrentDecisionID == nil {
		t.Fatalf("read original enterprise mapping decision: %v / %+v", err, originalMapping)
	}
	originalDecision, err := entityRepo.GetMappingDecisionByID(ctx, workspaceID, *originalMapping.CurrentDecisionID)
	if err != nil {
		t.Fatalf("read original immutable mapping decision: %v", err)
	}
	alternateEntity, err := entitydomain.NewEntity(workspaceID, matchJob.EntityTypeID, "T3-B2-ALTERNATE", "T3/B2 alternate company", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("create alternate entity: %v", err)
	}
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := entityRepo.InsertEntity(ctx, tx, alternateEntity); err != nil {
			return err
		}
		mapping := originalMapping
		mapping.ID = uuid.New()
		mapping.EntityID = alternateEntity.ID
		mapping.Status = entitydomain.MappingConfirmed
		mapping.CurrentDecisionID = nil
		_, err := entityRepo.RecordMappingDecision(ctx, tx, entitydomain.MappingDecisionCommand{
			Mapping: mapping, SourceOrigin: entitydomain.OriginWorkflowAlias,
			IdempotencyKey:        "native-current-mapping-change-" + workspaceID.String(),
			ExpectCurrentDecision: true, ExpectedCurrentDecisionID: originalMapping.CurrentDecisionID,
		})
		return err
	}); err != nil {
		t.Fatalf("change current mapping after resolution: %v", err)
	}

	workflowDefinition := readRepoFile(t, "examples", "enterprise-activity", "workflow", "workflow-v1.yaml")
	workflowVersionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	workflowVersion, err := workflowVersionService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           "enterprise-activity-production",
		Name:           "Enterprise Activity Production",
		Version:        "1.0.0",
		DefinitionRef:  "examples/enterprise-activity/workflow/workflow-v1.yaml",
		DefinitionYAML: workflowDefinition,
		TraceID:        "native-worker-e2e",
	})
	if err != nil {
		t.Fatalf("create workflow version: %v", err)
	}

	executionService := workflowapp.NewExecutionService(txManager, workflowRepo)
	execution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersion.ID,
		OutputDatasetID:   activityDataset.ID,
		TargetPeriod:      "2025-03",
		Inputs: []workflowdomain.InputBinding{
			{Name: "enterprise_raw", DatasetVersionID: enterpriseVersion.ID},
			{Name: "enterprise_resolution", DatasetVersionID: *matchJob.OutputDatasetVersionID},
			{Name: "lease_raw", DatasetVersionID: leaseVersion.ID},
			{Name: "energy_raw", DatasetVersionID: energyVersion.ID},
		},
		IdempotencyKey: "native-worker-create-" + workspaceID.String(),
		TraceID:        "native-worker-e2e",
	})
	if err != nil {
		t.Fatalf("create workflow execution: %v", err)
	}

	engine := workflowNative.NewEngine(industryPackRoot, txManager, datasetRepo, entityRepo, workflowRepo, uploadDataset, store)
	handler := workflowqueue.NewHandler(executionService, workflowRepo, engine)
	payload, err := json.Marshal(map[string]any{"executionId": execution.ID})
	if err != nil {
		t.Fatalf("marshal queue payload: %v", err)
	}
	if err := handler.Handle(ctx, asynq.NewTask(workflowqueue.TaskExecute, payload)); err != nil {
		t.Fatalf("handle workflow execution: %v", err)
	}

	storedExecution, err := workflowRepo.GetExecution(ctx, execution.ID)
	if err != nil {
		t.Fatalf("get completed execution: %v", err)
	}
	if storedExecution.Status != workflowdomain.ExecutionSucceeded || storedExecution.OutputDatasetVersionID == nil {
		t.Fatalf("execution = status %s output %v, want SUCCEEDED with output", storedExecution.Status, storedExecution.OutputDatasetVersionID)
	}

	outputVersion, err := datasetRepo.GetVersion(ctx, *storedExecution.OutputDatasetVersionID)
	if err != nil {
		t.Fatalf("get CURATED DatasetVersion: %v", err)
	}
	if outputVersion.Status != datasetdomain.VersionReady {
		t.Fatalf("output status = %s, want READY", outputVersion.Status)
	}
	if outputVersion.GeneratedByExecutionID == nil || *outputVersion.GeneratedByExecutionID != execution.ID {
		t.Fatalf("output generated_by_execution_id = %v, want %s", outputVersion.GeneratedByExecutionID, execution.ID)
	}

	rows := parseCSV(t, store.bytes(outputVersion.StorageURI))
	if len(rows) != 5 {
		t.Fatalf("CURATED rows = %d, want 5 canonical companies", len(rows))
	}
	byName := map[string]map[string]string{}
	for _, row := range rows {
		byName[row["company_name"]] = row
	}
	assertOutput(t, byName, "深圳星云科技有限公司", "96.01", "HIGH", "100.00")
	assertOutput(t, byName, "广州青禾智能科技有限公司", "81.97", "HIGH", "100.00")
	assertOutput(t, byName, "武汉蓝图装备制造有限公司", "99.13", "HIGH", "100.00")
	assertOutput(t, byName, "杭州云帆数据科技有限公司", "", "INSUFFICIENT_DATA", "33.33")
	assertOutput(t, byName, "上海海岳生物科技有限公司", "", "INSUFFICIENT_DATA", "33.33")
	if byName["深圳星云科技有限公司"]["company_id"] != originalDecision.EntityID.String() {
		t.Fatalf("native CURATED bytes used current mapping entity %s, want frozen resolution entity %s", byName["深圳星云科技有限公司"]["company_id"], originalDecision.EntityID)
	}
	var persistedDecisionID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT decision_id FROM execution_mapping_usage
		WHERE execution_id=$1 AND input_name='enterprise_raw' AND source_key='ENT-001'
	`, execution.ID).Scan(&persistedDecisionID); err != nil {
		t.Fatalf("query persisted enterprise mapping usage: %v", err)
	}
	if persistedDecisionID != originalDecision.ID {
		t.Fatalf("persisted enterprise decision = %s, want frozen decision %s", persistedDecisionID, originalDecision.ID)
	}
	var resolutionPolicyHash string
	var resolutionPolicyContent []byte
	if err := pool.QueryRow(ctx, `
		SELECT content_sha256, content
		FROM execution_dependency_binding
		WHERE execution_id=$1 AND dependency_name='enterprise_resolution'
	`, execution.ID).Scan(&resolutionPolicyHash, &resolutionPolicyContent); err != nil {
		t.Fatalf("query frozen resolution policy content: %v", err)
	}
	if resolutionPolicyHash != matchJob.PolicyContentSHA256 || !bytes.Equal(resolutionPolicyContent, matchJob.PolicyContent) {
		t.Fatal("execution did not persist the exact matching policy content used by resolution")
	}
	var preparationCount, bindingCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_preparation WHERE execution_id=$1`, execution.ID).Scan(&preparationCount); err != nil {
		t.Fatalf("count dependency preparations: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_binding WHERE execution_id=$1`, execution.ID).Scan(&bindingCount); err != nil {
		t.Fatalf("count dependency bindings: %v", err)
	}
	if preparationCount != 1 || bindingCount != 3 {
		t.Fatalf("dependency preparation/bindings = %d/%d, want 1/3", preparationCount, bindingCount)
	}
	var executionAsAliasSource int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM execution_mapping_usage u
		JOIN entity_mapping_decision d ON d.workspace_id=u.workspace_id AND d.id=u.decision_id
		WHERE u.execution_id=$1 AND d.source_origin='WORKFLOW_ALIAS' AND d.source_job_id=$1
	`, execution.ID).Scan(&executionAsAliasSource); err != nil {
		t.Fatalf("check alias source job provenance: %v", err)
	}
	if executionAsAliasSource != 0 {
		t.Fatalf("execution id was incorrectly recorded as alias source_job_id %d times", executionAsAliasSource)
	}

	var quarantineCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_quarantine_record WHERE execution_id=$1 AND reason_code='NEGATIVE_ENERGY_KWH'`, execution.ID).Scan(&quarantineCount); err != nil {
		t.Fatalf("count quarantine records: %v", err)
	}
	if quarantineCount != 1 {
		t.Fatalf("quarantine records = %d, want 1", quarantineCount)
	}

	var lineageCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version_lineage WHERE output_version_id=$1 AND execution_id=$2`, outputVersion.ID, execution.ID).Scan(&lineageCount); err != nil {
		t.Fatalf("count lineage: %v", err)
	}
	if lineageCount != 4 {
		t.Fatalf("lineage edges = %d, want 3 RAW dependencies plus entity-resolution output", lineageCount)
	}
	if matchJob.OutputDatasetVersionID == nil {
		t.Fatal("successful entity job must have a historical output version")
	}
	var resolutionEdges int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version_lineage WHERE output_version_id=$1 AND input_version_id=$2 AND execution_id=$3`, outputVersion.ID, *matchJob.OutputDatasetVersionID, execution.ID).Scan(&resolutionEdges); err != nil {
		t.Fatalf("query exact entity-resolution dependency: %v", err)
	}
	if resolutionEdges != 1 {
		t.Fatalf("entity-resolution lineage edges = %d, want exactly one", resolutionEdges)
	}

	var costCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_event WHERE execution_id=$1`, execution.ID).Scan(&costCount); err != nil {
		t.Fatalf("count cost events: %v", err)
	}
	if costCount != 1 {
		t.Fatalf("cost events = %d, want 1", costCount)
	}

	var evidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE source_type='EXECUTION' AND source_id=$1`, execution.ID).Scan(&evidenceCount); err != nil {
		t.Fatalf("count execution evidence: %v", err)
	}
	if evidenceCount != 2 {
		t.Fatalf("execution evidence = %d, want dependency preparation plus completion", evidenceCount)
	}
	var preparationEvidenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE source_type='EXECUTION' AND source_id=$1 AND evidence_type='EXECUTION_DEPENDENCY_PREPARED'`, execution.ID).Scan(&preparationEvidenceCount); err != nil {
		t.Fatalf("count dependency preparation evidence: %v", err)
	}
	if preparationEvidenceCount != 1 {
		t.Fatalf("dependency preparation evidence = %d, want 1", preparationEvidenceCount)
	}
}

func createDatasetForTest(t *testing.T, ctx context.Context, service *datasetapp.CreateDatasetService, workspaceID uuid.UUID, code, name string, datasetType datasetdomain.DatasetType) datasetdomain.Dataset {
	t.Helper()
	dataset, err := service.Handle(ctx, datasetapp.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        code + "-" + uuid.NewString(),
		Name:        name,
		DatasetType: datasetType,
		TraceID:     "native-worker-e2e",
	})
	if err != nil {
		t.Fatalf("create dataset %s: %v", code, err)
	}
	return dataset
}

func uploadFixture(t *testing.T, ctx context.Context, service *datasetapp.UploadVersionService, datasetID uuid.UUID, filename string) datasetdomain.DatasetVersion {
	t.Helper()
	version, err := service.Handle(ctx, datasetapp.UploadVersionCommand{
		DatasetID:   datasetID,
		Filename:    filename,
		ContentType: "text/csv",
		Content:     readRepoFile(t, "examples", "enterprise-activity", "data", filename),
		TraceID:     "native-worker-e2e",
	})
	if err != nil {
		t.Fatalf("upload fixture %s: %v", filename, err)
	}
	return version
}

func readRepoFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	content, err := os.ReadFile(repoPath(t, parts...))
	if err != nil {
		t.Fatalf("read repository file: %v", err)
	}
	return content
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

func parseCSV(t *testing.T, content []byte) []map[string]string {
	t.Helper()
	reader := csv.NewReader(bytes.NewReader(content))
	headers, err := reader.Read()
	if err != nil {
		t.Fatalf("read output headers: %v", err)
	}
	result := make([]map[string]string, 0)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read output row: %v", err)
		}
		row := make(map[string]string, len(headers))
		for i, header := range headers {
			row[header] = record[i]
		}
		result = append(result, row)
	}
	return result
}

func assertOutput(t *testing.T, rows map[string]map[string]string, companyName, activityScore, activityLevel, coverage string) {
	t.Helper()
	row := rows[companyName]
	if row == nil {
		t.Fatalf("missing output row for %s", companyName)
	}
	if row["activity_score"] != activityScore || row["activity_level"] != activityLevel || row["indicator_coverage"] != coverage {
		t.Fatalf("%s output = score %q level %q coverage %q, want %q %q %q", companyName, row["activity_score"], row["activity_level"], row["indicator_coverage"], activityScore, activityLevel, coverage)
	}
}
