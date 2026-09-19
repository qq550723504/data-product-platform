package native

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
	"time"

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

	engine := NewEngine(industryPackRoot, txManager, datasetRepo, entityRepo, workflowRepo, uploadDataset, store)
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

	// AC3: restore must use the persisted policy bytes even when the files on
	// disk have changed after preparation.
	enterpriseVersionStored, enterpriseRows, enterpriseRef, err := engine.readInput(ctx, enterpriseVersion.ID)
	if err != nil {
		t.Fatalf("reload enterprise input for dependency restore: %v", err)
	}
	leaseVersionStored, leaseRows, leaseRef, err := engine.readInput(ctx, leaseVersion.ID)
	if err != nil {
		t.Fatalf("reload lease input for dependency restore: %v", err)
	}
	energyVersionStored, energyRows, energyRef, err := engine.readInput(ctx, energyVersion.ID)
	if err != nil {
		t.Fatalf("reload energy input for dependency restore: %v", err)
	}
	restoreRequest := workflowapp.ProcessingRequest{
		ExecutionID: execution.ID, WorkspaceID: workspaceID, WorkflowVersion: workflowVersion,
		Inputs: execution.Inputs, OutputDatasetID: activityDataset.ID, TargetPeriod: "2025-03",
	}
	restoreBindings := map[string]uuid.UUID{
		"enterprise_raw": enterpriseVersionStored.ID, "enterprise_resolution": *matchJob.OutputDatasetVersionID,
		"lease_raw": leaseVersionStored.ID, "energy_raw": energyVersionStored.ID,
	}
	companyPolicyPath := filepath.Join(industryPackRoot, "park", "matching", "company-match-policy-v1.yaml")
	indicatorPolicyPath := filepath.Join(industryPackRoot, "park", "indicators", "enterprise-activity-v1.yaml")
	originalCompanyPolicy, err := os.ReadFile(companyPolicyPath)
	if err != nil {
		t.Fatalf("read company policy for restore test: %v", err)
	}
	originalIndicatorPolicy, err := os.ReadFile(indicatorPolicyPath)
	if err != nil {
		t.Fatalf("read indicator policy for restore test: %v", err)
	}
	restoredDependencies := func() preparedNativeDependencies {
		t.Helper()
		if err := os.WriteFile(companyPolicyPath, bytes.Replace(originalCompanyPolicy, []byte("version: 1.0.0"), []byte("version: 9.9.9"), 1), 0600); err != nil {
			t.Fatalf("change company policy for restore test: %v", err)
		}
		if err := os.WriteFile(indicatorPolicyPath, bytes.Replace(originalIndicatorPolicy, []byte("version: 1.0.0"), []byte("version: 9.9.9"), 1), 0600); err != nil {
			t.Fatalf("change indicator policy for restore test: %v", err)
		}
		defer func() {
			if err := os.WriteFile(companyPolicyPath, originalCompanyPolicy, 0600); err != nil {
				t.Fatalf("restore company policy after restore test: %v", err)
			}
			if err := os.WriteFile(indicatorPolicyPath, originalIndicatorPolicy, 0600); err != nil {
				t.Fatalf("restore indicator policy after restore test: %v", err)
			}
		}()
		prepared, err := engine.prepareDependencies(ctx, restoreRequest, restoreBindings, enterpriseRows, leaseRows, energyRows, enterpriseRef, leaseRef, energyRef)
		if err != nil {
			t.Fatalf("restore frozen dependencies: %v", err)
		}
		return prepared
	}()
	if restoredDependencies.CompanyPolicy.Metadata.Version != matchJob.PolicyVersion || restoredDependencies.IndicatorPolicy.Metadata.Version != "1.0.0" {
		t.Fatalf("restore used changed policy content: company=%s indicator=%s", restoredDependencies.CompanyPolicy.Metadata.Version, restoredDependencies.IndicatorPolicy.Metadata.Version)
	}

	// AC5/AC6: replaying preparation reuses the alias facts, and two initial
	// preparations for a new Execution converge on one committed set.
	var aliasDecisionCountBefore int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM entity_mapping_decision WHERE workspace_id=$1 AND source_origin='WORKFLOW_ALIAS'`, workspaceID).Scan(&aliasDecisionCountBefore); err != nil {
		t.Fatalf("count alias decisions before replay: %v", err)
	}
	if _, err := engine.prepareDependencies(ctx, restoreRequest, restoreBindings, enterpriseRows, leaseRows, energyRows, enterpriseRef, leaseRef, energyRef); err != nil {
		t.Fatalf("repeat dependency preparation: %v", err)
	}
	var aliasDecisionCountAfter int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM entity_mapping_decision WHERE workspace_id=$1 AND source_origin='WORKFLOW_ALIAS'`, workspaceID).Scan(&aliasDecisionCountAfter); err != nil {
		t.Fatalf("count alias decisions after replay: %v", err)
	}
	if aliasDecisionCountAfter != aliasDecisionCountBefore {
		t.Fatalf("alias decisions changed on replay: before=%d after=%d", aliasDecisionCountBefore, aliasDecisionCountAfter)
	}

	concurrentExecution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflowVersion.ID, OutputDatasetID: activityDataset.ID,
		TargetPeriod: "2025-03", Inputs: execution.Inputs, IdempotencyKey: "native-worker-concurrent-" + workspaceID.String(),
	})
	if err != nil {
		t.Fatalf("create concurrent preparation execution: %v", err)
	}
	concurrentRequest := restoreRequest
	concurrentRequest.ExecutionID = concurrentExecution.ID
	concurrentRequest.Inputs = concurrentExecution.Inputs
	concurrentResults := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := engine.prepareDependencies(ctx, concurrentRequest, restoreBindings, enterpriseRows, leaseRows, energyRows, enterpriseRef, leaseRef, energyRef)
			concurrentResults <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-concurrentResults; err != nil {
			t.Fatalf("concurrent dependency preparation: %v", err)
		}
	}
	var concurrentPreparationCount, concurrentBindingCount, concurrentUsageCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_preparation WHERE execution_id=$1`, concurrentExecution.ID).Scan(&concurrentPreparationCount); err != nil {
		t.Fatalf("count concurrent preparations: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_binding WHERE execution_id=$1`, concurrentExecution.ID).Scan(&concurrentBindingCount); err != nil {
		t.Fatalf("count concurrent bindings: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_mapping_usage WHERE execution_id=$1`, concurrentExecution.ID).Scan(&concurrentUsageCount); err != nil {
		t.Fatalf("count concurrent mapping usages: %v", err)
	}
	if concurrentPreparationCount != 1 || concurrentBindingCount != 3 || concurrentUsageCount == 0 {
		t.Fatalf("concurrent dependency shape = prep=%d bindings=%d usages=%d, want 1/3/nonzero", concurrentPreparationCount, concurrentBindingCount, concurrentUsageCount)
	}

	// Two executions may discover the same new aliases in opposite CSV row
	// orders. Preparation must acquire the source-scoped advisory locks in a
	// shared order so this does not become an A/B versus B/A deadlock.
	orderedAliasRows := []map[string]string{
		{"source_company_id": "LEASE-ORDER-A", "company_name": "深圳星云科技有限公司"},
		{"source_company_id": "LEASE-ORDER-B", "company_name": "广州青禾智能科技有限公司"},
	}
	reversedAliasRows := []map[string]string{orderedAliasRows[1], orderedAliasRows[0]}
	orderedExecution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflowVersion.ID, OutputDatasetID: activityDataset.ID,
		TargetPeriod: "2025-03", Inputs: execution.Inputs, IdempotencyKey: "native-worker-ordered-aliases-" + workspaceID.String(),
	})
	if err != nil {
		t.Fatalf("create ordered-alias execution: %v", err)
	}
	reversedExecution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflowVersion.ID, OutputDatasetID: activityDataset.ID,
		TargetPeriod: "2025-03", Inputs: execution.Inputs, IdempotencyKey: "native-worker-reversed-aliases-" + workspaceID.String(),
	})
	if err != nil {
		t.Fatalf("create reversed-alias execution: %v", err)
	}
	orderedRequest := restoreRequest
	orderedRequest.ExecutionID = orderedExecution.ID
	orderedRequest.Inputs = orderedExecution.Inputs
	reversedRequest := restoreRequest
	reversedRequest.ExecutionID = reversedExecution.ID
	reversedRequest.Inputs = reversedExecution.Inputs
	aliasOrderCtx, cancelAliasOrder := context.WithTimeout(ctx, 3*time.Second)
	defer cancelAliasOrder()
	aliasOrderResults := make(chan error, 2)
	go func() {
		_, err := engine.prepareDependencies(aliasOrderCtx, orderedRequest, restoreBindings, enterpriseRows, orderedAliasRows, energyRows, enterpriseRef, leaseRef, energyRef)
		aliasOrderResults <- err
	}()
	go func() {
		_, err := engine.prepareDependencies(aliasOrderCtx, reversedRequest, restoreBindings, enterpriseRows, reversedAliasRows, energyRows, enterpriseRef, leaseRef, energyRef)
		aliasOrderResults <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-aliasOrderResults; err != nil {
			t.Fatalf("opposite alias-order preparation: %v", err)
		}
	}

	// AC6: a failure after alias preparation starts rolls back the whole
	// dependency set and does not leave a newly-created alias fact behind.
	failedExecution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflowVersion.ID, OutputDatasetID: activityDataset.ID,
		TargetPeriod: "2025-03", Inputs: execution.Inputs, IdempotencyKey: "native-worker-failed-preparation-" + workspaceID.String(),
	})
	if err != nil {
		t.Fatalf("create failed-preparation execution: %v", err)
	}
	failedLeaseRows := append([]map[string]string(nil), leaseRows...)
	failedLeaseRows[0] = map[string]string{}
	for key, value := range leaseRows[0] {
		failedLeaseRows[0][key] = value
	}
	failedLeaseRows[0]["source_company_id"] = "LEASE-FAIL-T3-B2"
	failedLeaseRows[0]["company_name"] = "No matching company for rollback test"
	failedRequest := restoreRequest
	failedRequest.ExecutionID = failedExecution.ID
	failedRequest.Inputs = failedExecution.Inputs
	if _, err := engine.prepareDependencies(ctx, failedRequest, restoreBindings, enterpriseRows, failedLeaseRows, energyRows, enterpriseRef, leaseRef, energyRef); err == nil {
		t.Fatal("failed dependency preparation unexpectedly succeeded")
	}
	var failedPreparationCount, failedBindingCount, failedUsageCount, failedAliasCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_preparation WHERE execution_id=$1`, failedExecution.ID).Scan(&failedPreparationCount); err != nil {
		t.Fatalf("count failed preparations: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_binding WHERE execution_id=$1`, failedExecution.ID).Scan(&failedBindingCount); err != nil {
		t.Fatalf("count failed bindings: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_mapping_usage WHERE execution_id=$1`, failedExecution.ID).Scan(&failedUsageCount); err != nil {
		t.Fatalf("count failed mapping usages: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM entity_mapping_decision WHERE workspace_id=$1 AND source_origin='WORKFLOW_ALIAS' AND source_key=$2`, workspaceID, "LEASE-FAIL-T3-B2").Scan(&failedAliasCount); err != nil {
		t.Fatalf("count rolled-back alias decisions: %v", err)
	}
	if failedPreparationCount != 0 || failedBindingCount != 0 || failedUsageCount != 0 || failedAliasCount != 0 {
		t.Fatalf("failed preparation left partial facts: prep=%d bindings=%d usages=%d alias=%d", failedPreparationCount, failedBindingCount, failedUsageCount, failedAliasCount)
	}

	// AC4: a RAW DatasetVersion cannot be supplied as the explicit resolution
	// output; the worker fails before creating a prepared binding or output.
	wrongResolutionExecution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflowVersion.ID, OutputDatasetID: activityDataset.ID,
		TargetPeriod: "2025-03", Inputs: []workflowdomain.InputBinding{
			{Name: "enterprise_raw", DatasetVersionID: enterpriseVersion.ID},
			{Name: "enterprise_resolution", DatasetVersionID: enterpriseVersion.ID},
			{Name: "lease_raw", DatasetVersionID: leaseVersion.ID}, {Name: "energy_raw", DatasetVersionID: energyVersion.ID},
		}, IdempotencyKey: "native-worker-wrong-resolution-" + workspaceID.String(),
	})
	if err != nil {
		t.Fatalf("create wrong-resolution execution: %v", err)
	}
	wrongPayload, _ := json.Marshal(map[string]any{"executionId": wrongResolutionExecution.ID})
	if err := handler.Handle(ctx, asynq.NewTask(workflowqueue.TaskExecute, wrongPayload)); err != nil {
		t.Fatalf("handle wrong-resolution execution: %v", err)
	}
	wrongStored, err := workflowRepo.GetExecution(ctx, wrongResolutionExecution.ID)
	if err != nil {
		t.Fatalf("read wrong-resolution execution: %v", err)
	}
	if wrongStored.Status != workflowdomain.ExecutionFailed || wrongStored.OutputDatasetVersionID != nil {
		t.Fatalf("wrong-resolution execution = status %s output %v, want FAILED without output", wrongStored.Status, wrongStored.OutputDatasetVersionID)
	}
	var wrongPreparationCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_preparation WHERE execution_id=$1`, wrongResolutionExecution.ID).Scan(&wrongPreparationCount); err != nil {
		t.Fatalf("count wrong-resolution preparations: %v", err)
	}
	if wrongPreparationCount != 0 {
		t.Fatalf("wrong-resolution preparation count = %d, want 0", wrongPreparationCount)
	}

	foreignWorkspaceID := uuid.New()
	foreignDataset := createDatasetForTest(t, ctx, createDataset, foreignWorkspaceID, "FOREIGN-RAW", "Foreign RAW", datasetdomain.DatasetTypeRaw)
	foreignVersion := uploadFixture(t, ctx, uploadDataset, foreignDataset.ID, "enterprise.csv")
	if _, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflowVersion.ID, OutputDatasetID: activityDataset.ID,
		TargetPeriod: "2025-03", Inputs: []workflowdomain.InputBinding{
			{Name: "enterprise_raw", DatasetVersionID: foreignVersion.ID},
			{Name: "enterprise_resolution", DatasetVersionID: *matchJob.OutputDatasetVersionID},
			{Name: "lease_raw", DatasetVersionID: leaseVersion.ID}, {Name: "energy_raw", DatasetVersionID: energyVersion.ID},
		}, IdempotencyKey: "native-worker-cross-workspace-" + workspaceID.String(),
	}); err == nil {
		t.Fatal("cross-workspace input unexpectedly created an execution")
	}

	// All T3/B2 historical facts are append-only at the database boundary.
	mutations := []struct {
		name  string
		query string
	}{
		{"preparation", `UPDATE execution_dependency_preparation SET status='PREPARED' WHERE execution_id=$1`},
		{"binding", `UPDATE execution_dependency_binding SET reference='tampered' WHERE execution_id=$1 AND dependency_name='company_match_policy'`},
		{"usage", `UPDATE execution_mapping_usage SET entity_id=entity_id WHERE id=(SELECT id FROM execution_mapping_usage WHERE execution_id=$1 LIMIT 1)`},
		{"resolution output decision", `UPDATE entity_resolution_output_decision SET source_key=source_key WHERE output_dataset_version_id=$1`},
	}
	for _, mutation := range mutations {
		args := []any{execution.ID}
		if mutation.name == "resolution output decision" {
			args = []any{*matchJob.OutputDatasetVersionID}
		}
		if _, err := pool.Exec(ctx, mutation.query, args...); err == nil {
			t.Fatalf("%s historical fact mutation unexpectedly succeeded", mutation.name)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE entity_match_job SET policy_content='tampered' WHERE id=$1`, matchJob.ID); err == nil {
		t.Fatal("entity match policy snapshot mutation unexpectedly succeeded")
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
