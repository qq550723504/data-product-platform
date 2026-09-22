package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	complianceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	compliancedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliverydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	deliveryhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/transport/http"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	qualityhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/transport/http"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflownative "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

type pilotCertificationResolver struct {
	input certificationdomain.EvaluationInput
}

func (r pilotCertificationResolver) Resolve(context.Context, certificationapp.EvaluateDatasetCertificationCommand, certificationdomain.ProfileSnapshot) (certificationdomain.EvaluationInput, error) {
	return r.input, nil
}

type pilotDeliveryStatusQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

type pilotCommitObservingStore struct {
	base           *memoryStore
	query          pilotDeliveryStatusQuery
	workspaceID    uuid.UUID
	idempotencyKey string

	calls          int
	observedStatus string
}

func (s *pilotCommitObservingStore) Get(ctx context.Context, storageURI string) (io.ReadCloser, error) {
	s.calls++
	if err := s.query.QueryRow(ctx, `
		SELECT status
		FROM delivery_operation
		WHERE workspace_id=$1 AND idempotency_key=$2
	`, s.workspaceID, s.idempotencyKey).Scan(&s.observedStatus); err != nil {
		return nil, err
	}
	return s.base.Get(ctx, storageURI)
}

type pilotBarrierDirectDataGate struct {
	base    deliveryapp.DirectDataGate
	entered chan struct{}
	release chan struct{}
}

func (g *pilotBarrierDirectDataGate) EvaluateDirectData(ctx context.Context, tx pgx.Tx, request deliveryapp.DirectDataGateRequest) (deliveryapp.DirectDataGateResult, error) {
	result, err := g.base.EvaluateDirectData(ctx, tx, request)
	if err != nil {
		return deliveryapp.DirectDataGateResult{}, err
	}
	select {
	case g.entered <- struct{}{}:
	default:
	}
	select {
	case <-g.release:
		return result, nil
	case <-ctx.Done():
		return deliveryapp.DirectDataGateResult{}, ctx.Err()
	}
}

func TestCertifiedDatasetEnterpriseActivityPilotHappyPath(t *testing.T) {
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
	actorID := uuid.New()
	reviewerID := uuid.New()
	traceID := "certified-dataset-pilot-" + uuid.NewString()
	suffix := uuid.NewString()
	store := newMemoryStore()
	txManager := transaction.NewManager(pool)
	industryPackRoot := repoPath(t, "industry-packs")

	resourceRepo := resourceinfra.NewPostgresRepository()
	resourceService := resourceapp.NewCreateService(txManager, resourceRepo)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	createDataset := datasetapp.NewCreateDatasetService(txManager, datasetRepo, resourceRepo)
	uploadDataset := datasetapp.NewUploadVersionService(txManager, datasetRepo, store)
	entityRepo := entityinfra.NewPostgresRepository(pool)
	entityService := entityapp.NewMatchService(industryPackRoot, txManager, entityRepo, datasetRepo, uploadDataset, store)
	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	workflowVersionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	executionService := workflowapp.NewExecutionService(txManager, workflowRepo)
	qualityRepo := qualityinfra.NewPostgresRepository(pool)
	qualityService := qualityapp.NewService(industryPackRoot, txManager, datasetRepo, qualityRepo, store, evidence.NewQueryRepository(pool))
	complianceRepo := complianceinfra.NewPostgresRepository(pool)
	complianceService := complianceapp.NewService(industryPackRoot, txManager, datasetRepo, complianceRepo, store)
	contractRepo := contractinfra.NewPostgresRepository(pool)
	contractService := contractapp.NewService(txManager, contractRepo)
	rightsRepo := rightsinfra.NewPostgresRepository(pool)
	rightsService := rightsapp.NewService(txManager, rightsRepo)

	enterpriseResource := mustCreateResource(t, ctx, resourceService, workspaceID, "PILOT-ENTERPRISE", "Pilot enterprise master data", &actorID, traceID)
	leaseResource := mustCreateResource(t, ctx, resourceService, workspaceID, "PILOT-LEASE", "Pilot enterprise lease data", &actorID, traceID)
	energyResource := mustCreateResource(t, ctx, resourceService, workspaceID, "PILOT-ENERGY", "Pilot enterprise energy data", &actorID, traceID)

	enterpriseDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "PENT-RAW", "Pilot Enterprise RAW", datasetdomain.DatasetTypeRaw, &enterpriseResource.ID, &actorID, traceID)
	leaseDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "PLEASE-RAW", "Pilot Lease RAW", datasetdomain.DatasetTypeRaw, &leaseResource.ID, &actorID, traceID)
	energyDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "PENERGY-RAW", "Pilot Energy RAW", datasetdomain.DatasetTypeRaw, &energyResource.ID, &actorID, traceID)
	standardizedDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "PENT-STD", "Pilot Enterprise standardized", datasetdomain.DatasetTypeStandardized, nil, &actorID, traceID)
	activityDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "PACT-CURATED", "Pilot Enterprise activity", datasetdomain.DatasetTypeCurated, nil, &actorID, traceID)

	enterpriseName := "pilot-enterprise-" + suffix + ".csv"
	leaseName := "pilot-lease-" + suffix + ".csv"
	energyName := "pilot-energy-" + suffix + ".csv"
	enterpriseVersion := mustUploadFixture(t, ctx, uploadDataset, enterpriseDataset.ID, "enterprise.csv", enterpriseName, &actorID, traceID)
	leaseVersion := mustUploadFixture(t, ctx, uploadDataset, leaseDataset.ID, "lease.csv", leaseName, &actorID, traceID)
	energyVersion := mustUploadFixture(t, ctx, uploadDataset, energyDataset.ID, "energy.csv", energyName, &actorID, traceID)

	matchJob, err := entityService.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID:           workspaceID,
		InputDatasetVersionID: enterpriseVersion.ID,
		OutputDatasetID:       standardizedDataset.ID,
		SourceType:            "CSV",
		SourceRef:             enterpriseName,
		SourceRole:            entitydomain.SourceAnchor,
		PolicyRef:             companyPolicyRef,
		ActorID:               &actorID,
		TraceID:               traceID,
	})
	if err != nil {
		t.Fatalf("start pilot Entity Resolution: %v", err)
	}
	for matchJob.Status == entitydomain.JobWaitingReview {
		candidates, err := entityRepo.ListCandidates(ctx, matchJob.ID)
		if err != nil {
			t.Fatalf("list pilot review candidates: %v", err)
		}
		progressed := false
		for _, candidate := range candidates {
			if candidate.Status != entitydomain.CandidatePending {
				continue
			}
			progressed = true
			matchJob, err = entityService.Confirm(ctx, entityapp.ReviewCommand{
				CandidateID: candidate.ID,
				ReviewerID:  reviewerID,
				Reason:      "confirmed during Certified Dataset Pilot acceptance",
				TraceID:     traceID,
			})
			if err != nil {
				t.Fatalf("confirm pilot Entity Resolution candidate: %v", err)
			}
		}
		if !progressed {
			t.Fatal("Entity Resolution is WAITING_REVIEW without pending candidates")
		}
	}
	if matchJob.Status != entitydomain.JobSucceeded || matchJob.OutputDatasetVersionID == nil {
		t.Fatalf("pilot Entity Resolution = %s/%v, want SUCCEEDED with STANDARDIZED output", matchJob.Status, matchJob.OutputDatasetVersionID)
	}
	standardizedVersionID := *matchJob.OutputDatasetVersionID
	standardizedVersion, err := datasetRepo.GetVersion(ctx, standardizedVersionID)
	if err != nil {
		t.Fatalf("load pilot STANDARDIZED DatasetVersion: %v", err)
	}
	if standardizedVersion.Status != datasetdomain.VersionReady ||
		standardizedVersion.GeneratedByEntityMatchJobID == nil ||
		*standardizedVersion.GeneratedByEntityMatchJobID != matchJob.ID ||
		standardizedVersion.GeneratedByExecutionID != nil {
		t.Fatalf("pilot STANDARDIZED output = status %s matchProducer %v executionProducer %v, want READY/%s/nil",
			standardizedVersion.Status, standardizedVersion.GeneratedByEntityMatchJobID,
			standardizedVersion.GeneratedByExecutionID, matchJob.ID)
	}
	provenJob, err := entityRepo.GetSuccessfulResolutionJobForOutput(
		ctx,
		workspaceID,
		enterpriseVersion.ID,
		standardizedVersionID,
		"CSV",
		enterpriseName,
	)
	if err != nil {
		t.Fatalf("prove pilot Entity Resolution output lineage: %v", err)
	}
	if provenJob.ID != matchJob.ID ||
		provenJob.OutputDatasetVersionID == nil ||
		*provenJob.OutputDatasetVersionID != standardizedVersionID {
		t.Fatalf("pilot proven Entity Resolution job = id %s output %v, want %s/%s",
			provenJob.ID, provenJob.OutputDatasetVersionID, matchJob.ID, standardizedVersionID)
	}

	candidates, err := entityRepo.ListCandidates(ctx, matchJob.ID)
	if err != nil {
		t.Fatalf("list finalized pilot Entity Resolution candidates: %v", err)
	}
	frozenDecisions, err := entityRepo.ListResolutionOutputDecisions(ctx, workspaceID, standardizedVersionID)
	if err != nil {
		t.Fatalf("list frozen pilot Entity Resolution decisions: %v", err)
	}
	if len(candidates) == 0 || len(frozenDecisions) != len(candidates) {
		t.Fatalf("pilot frozen resolution decisions = %d for %d candidates, want one per candidate",
			len(frozenDecisions), len(candidates))
	}
	for _, candidate := range candidates {
		decision, ok := frozenDecisions[candidate.SourceKey]
		if !ok {
			t.Fatalf("pilot STANDARDIZED output has no frozen decision for source key %s", candidate.SourceKey)
		}
		if decision.WorkspaceID != workspaceID ||
			decision.SourceType != matchJob.SourceType ||
			decision.SourceRef != matchJob.SourceRef ||
			decision.SourceKey != candidate.SourceKey ||
			decision.SourceJobID == nil || *decision.SourceJobID != matchJob.ID ||
			decision.SourceCandidateID == nil || *decision.SourceCandidateID != candidate.ID {
			t.Fatalf("pilot frozen resolution decision for %s is inconsistent: %+v", candidate.SourceKey, decision)
		}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE dataset_version
		SET generated_by_entity_match_job_id=NULL
		WHERE id=$1
	`, standardizedVersionID); err == nil || !strings.Contains(err.Error(), "entity-match producer identity is immutable") {
		t.Fatalf("pilot STANDARDIZED producer identity mutation error = %v, want immutable producer guard", err)
	}

	workflowVersion, err := workflowVersionService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           "pilot-enterprise-activity-" + suffix,
		Name:           "Certified Dataset Pilot Enterprise Activity",
		Version:        "1.0.0",
		DefinitionRef:  "examples/enterprise-activity/workflow/workflow-v1.yaml",
		DefinitionYAML: readRepoFile(t, "examples", "enterprise-activity", "workflow", "workflow-v1.yaml"),
		ActorID:        &actorID,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("create pilot WorkflowVersion: %v", err)
	}
	execution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID:       workspaceID,
		WorkflowVersionID: workflowVersion.ID,
		OutputDatasetID:   activityDataset.ID,
		TargetPeriod:      "2025-03",
		Inputs: []workflowdomain.InputBinding{
			{Name: "enterprise_raw", DatasetVersionID: enterpriseVersion.ID},
			{Name: "enterprise_resolution", DatasetVersionID: standardizedVersionID},
			{Name: "lease_raw", DatasetVersionID: leaseVersion.ID},
			{Name: "energy_raw", DatasetVersionID: energyVersion.ID},
		},
		IdempotencyKey: "pilot-execution-" + suffix,
		ActorID:        &actorID,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("create pilot Execution: %v", err)
	}
	engine := workflownative.NewEngine(industryPackRoot, txManager, datasetRepo, entityRepo, workflowRepo, uploadDataset, store, parkindicator.NewCalculator())
	queueHandler := workflowqueue.NewHandler(executionService, workflowRepo, engine)
	payload, _ := json.Marshal(map[string]any{"executionId": execution.ID})
	if err := queueHandler.Handle(ctx, asynq.NewTask(workflowqueue.TaskExecute, payload)); err != nil {
		t.Fatalf("execute pilot native workflow: %v", err)
	}
	storedExecution, err := workflowRepo.GetExecution(ctx, execution.ID)
	if err != nil {
		t.Fatalf("load pilot Execution: %v", err)
	}
	if storedExecution.Status != workflowdomain.ExecutionSucceeded || storedExecution.OutputDatasetVersionID == nil {
		t.Fatalf("pilot Execution = %s/%v, want SUCCEEDED with CURATED output", storedExecution.Status, storedExecution.OutputDatasetVersionID)
	}
	outputVersion, err := datasetRepo.GetVersion(ctx, *storedExecution.OutputDatasetVersionID)
	if err != nil {
		t.Fatalf("load pilot CURATED DatasetVersion: %v", err)
	}
	if outputVersion.Status != datasetdomain.VersionReady || outputVersion.GeneratedByExecutionID == nil || *outputVersion.GeneratedByExecutionID != execution.ID {
		t.Fatalf("pilot CURATED output = status %s producer %v, want READY/%s", outputVersion.Status, outputVersion.GeneratedByExecutionID, execution.ID)
	}

	qualityResult, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: outputVersion.ID, RuleSetRef: qualityRuleSetRef,
		ActorID: &actorID, TraceID: traceID, Now: outputVersion.ReadyAt.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("run pilot QualityAssessment: %v", err)
	}
	if qualityResult.GateDecision != qualitydomain.GatePass {
		t.Fatalf("pilot QualityAssessment gate = %s, want PASS; findings=%+v", qualityResult.GateDecision, qualityResult.Findings)
	}
	for _, dimension := range qualitydomain.QualityDimensions {
		if _, ok := qualityResult.DimensionSummaries[dimension]; !ok {
			t.Fatalf("pilot QualityAssessment missing dimension %s", dimension)
		}
	}

	qualityHandler := qualityhttp.NewHandler(
		qualityService,
		qualityRepo,
		evidence.NewQueryRepository(pool),
	)
	qualityMux := http.NewServeMux()
	qualityHandler.Register(qualityMux)
	reportRequest := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/quality-assessments/"+qualityResult.ID.String()+"/report?workspaceId="+workspaceID.String()+"&limit=100&offset=0",
		nil,
	)
	reportResponse := httptest.NewRecorder()
	qualityMux.ServeHTTP(reportResponse, reportRequest)
	if reportResponse.Code != http.StatusOK {
		t.Fatalf("pilot Quality Report = %d %s, want 200", reportResponse.Code, reportResponse.Body.String())
	}
	var report map[string]any
	if err := json.Unmarshal(reportResponse.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode pilot Quality Report: %v", err)
	}
	if report["id"] != qualityResult.ID.String() ||
		report["workspaceId"] != workspaceID.String() ||
		report["datasetVersionId"] != outputVersion.ID.String() ||
		report["gateDecision"] != string(qualitydomain.GatePass) ||
		report["ruleSetRef"] != qualityResult.RuleSetRef ||
		report["ruleSetVersion"] != qualityResult.RuleSetVersion {
		t.Fatalf("pilot Quality Report summary is inconsistent: %+v", report)
	}
	dimensionSummary, ok := report["dimensionSummary"].(map[string]any)
	if !ok {
		t.Fatalf("pilot Quality Report dimensionSummary has unexpected shape: %#v", report["dimensionSummary"])
	}
	for _, dimension := range qualitydomain.QualityDimensions {
		if _, ok := dimensionSummary[string(dimension)]; !ok {
			t.Fatalf("pilot Quality Report missing dimension %s: %+v", dimension, dimensionSummary)
		}
	}
	findingsSection, ok := report["findings"].(map[string]any)
	if !ok {
		t.Fatalf("pilot Quality Report findings has unexpected shape: %#v", report["findings"])
	}
	items, ok := findingsSection["items"].([]any)
	if !ok {
		t.Fatalf("pilot Quality Report findings.items has unexpected shape: %#v", findingsSection["items"])
	}
	page, ok := findingsSection["page"].(map[string]any)
	if !ok {
		t.Fatalf("pilot Quality Report findings.page has unexpected shape: %#v", findingsSection["page"])
	}
	total, totalOK := page["total"].(float64)
	offset, offsetOK := page["offset"].(float64)
	if !totalOK || !offsetOK || int(total) != len(qualityResult.Findings) || int(offset) != 0 || len(items) != len(qualityResult.Findings) {
		t.Fatalf("pilot Quality Report findings page = %#v items=%d, want offset=0 total/items=%d",
			page, len(items), len(qualityResult.Findings))
	}
	for _, raw := range items {
		finding, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("pilot Quality Report finding has unexpected shape: %#v", raw)
		}
		if strings.TrimSpace(asPilotString(finding["id"])) == "" ||
			strings.TrimSpace(asPilotString(finding["ruleId"])) == "" ||
			strings.TrimSpace(asPilotString(finding["dimension"])) == "" ||
			strings.TrimSpace(asPilotString(finding["severity"])) == "" ||
			strings.TrimSpace(asPilotString(finding["status"])) == "" ||
			finding["observed"] == nil {
			t.Fatalf("pilot Quality Report contains an unexplained finding: %+v", finding)
		}
	}
	evidenceRefs, ok := report["evidence"].([]any)
	if !ok || len(evidenceRefs) == 0 {
		t.Fatalf("pilot Quality Report Evidence references = %#v, want non-empty", report["evidence"])
	}
	auditRefs, ok := report["auditEvents"].([]any)
	if !ok || len(auditRefs) == 0 {
		t.Fatalf("pilot Quality Report Audit references = %#v, want non-empty", report["auditEvents"])
	}

	complianceResult, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: outputVersion.ID, PolicyRef: complianceRef,
		ActorID: &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("run pilot Compliance Gate: %v", err)
	}
	if complianceResult.GateDecision != compliancedomain.GatePass {
		t.Fatalf("pilot Compliance Gate = %s, want PASS", complianceResult.GateDecision)
	}

	contractVersion, err := contractService.CreateVersionFromYAML(ctx, contractapp.CreateVersionFromYAMLCommand{
		WorkspaceID:  workspaceID,
		SourceRef:    "examples/enterprise-activity/contract/data-contract-v1.yaml",
		DocumentYAML: readRepoFile(t, "examples", "enterprise-activity", "contract", "data-contract-v1.yaml"),
		ActorID:      &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("create pilot Data Contract: %v", err)
	}
	contractVersion, err = contractService.PublishVersion(ctx, contractapp.PublishVersionCommand{VersionID: contractVersion.ID, ActorID: &actorID, TraceID: traceID})
	if err != nil {
		t.Fatalf("publish pilot Data Contract: %v", err)
	}

	validFrom := time.Now().UTC().Add(-time.Hour)
	validTo := time.Now().UTC().Add(24 * time.Hour)
	grants := []rightsdomain.ResourceGrantSpec{
		{DataResourceID: enterpriseResource.ID, Actions: []string{"READ"}, ScopeType: "ALL_RESOURCE", ScopeRef: enterpriseResource.ID.String()},
		{DataResourceID: leaseResource.ID, Actions: []string{"READ"}, ScopeType: "ALL_RESOURCE", ScopeRef: leaseResource.ID.String()},
		{DataResourceID: energyResource.ID, Actions: []string{"READ"}, ScopeType: "ALL_RESOURCE", ScopeRef: energyResource.ID.String()},
	}
	authorization := activateAuthorization(t, ctx, rightsService, workspaceID, "AUTH-PILOT-"+suffix, validFrom, validTo, grants, &actorID, traceID)

	rightsSnapshot, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID: workspaceID, Purpose: purpose, ConsumerRef: "LICENSED_BANK", AsOf: time.Now().UTC(),
		AuthorizationIDs: []uuid.UUID{authorization.ID}, ActorID: &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("create pilot RightsSnapshot: %v", err)
	}

	effectiveRights, err := rightsService.ComputeEffectiveRights(ctx, rightsapp.ComputeEffectiveRightsCommand{
		WorkspaceID: workspaceID, TargetDatasetVersionID: outputVersion.ID,
		ConsumerRef: "LICENSED_BANK", Purpose: purpose, ActorID: &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("compute pilot Effective Rights: %v", err)
	}
	if effectiveRights.Status != "FINALIZED" {
		t.Fatalf("pilot Effective Rights status = %s, want FINALIZED", effectiveRights.Status)
	}
	readAllowed := false
	for _, action := range effectiveRights.Actions {
		if action.Action == "READ" {
			readAllowed = action.Decision == rightsdomain.DecisionAllowed
			break
		}
	}
	if !readAllowed {
		t.Fatalf("pilot Effective Rights READ is not ALLOWED: %+v", effectiveRights.Actions)
	}

	var supportingEvidence evidence.Snapshot
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		record, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  workspaceID,
			EvidenceType: "CERTIFIED_DATASET_PILOT_TRACE",
			Title:        "Certified Dataset Pilot traceability evidence",
			SourceType:   "DATASET_VERSION",
			SourceID:     &outputVersion.ID,
			Metadata: map[string]any{
				"executionId":               execution.ID,
				"entityMatchJobId":          matchJob.ID,
				"qualityAssessmentId":       qualityResult.ID,
				"effectiveRightsSnapshotId": effectiveRights.ID,
			},
			CreatedBy: &actorID,
		}, evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: outputVersion.ID, RelationType: "PILOT_TRACE"})
		if err != nil {
			return err
		}
		snapshot, err := evidence.CreateSnapshot(ctx, tx, workspaceID, "DATASET_VERSION", outputVersion.ID, map[string]any{
			"pilot":       "enterprise-activity",
			"executionId": execution.ID,
		}, []evidence.SnapshotItem{{EvidenceID: record.ID, Category: "TRACEABILITY"}}, &actorID)
		if err != nil {
			return err
		}
		supportingEvidence = snapshot
		return nil
	}); err != nil {
		t.Fatalf("create pilot EvidenceSnapshot: %v", err)
	}

	profileRepo := certificationinfra.NewProfileRepository(pool)
	profileService := certificationapp.NewProfileService(txManager, profileRepo)
	profile, err := profileService.Create(ctx, certificationapp.CreateProfileCommand{
		WorkspaceID: workspaceID,
		Profile: certificationdomain.CertificationProfile{
			ProfileRef:            "park/enterprise-activity-certified-v1",
			Code:                  "PILOT-ENTERPRISE-ACTIVITY-CERTIFIED",
			Name:                  "Enterprise Activity Certified Dataset Pilot",
			Version:               "1.0.0",
			Purpose:               certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{purpose}},
			Actions:               certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"READ"}},
			Consumers:             certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"LICENSED_BANK"}},
			Delivery:              certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"DIRECT_DATA"}},
			RequiredCriticalRules: []string{"QA-COMPANY-ID-COMPLETE"},
			QualityGateRequired:   true,
			Rights: certificationdomain.RightsRequirement{
				Required:  true,
				Purpose:   certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{purpose}},
				Actions:   certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"READ"}},
				Consumers: certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"LICENSED_BANK"}},
				Scopes: certificationdomain.ScopeApplicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []certificationdomain.ScopeRef{
					{Type: "ALL_RESOURCE", Ref: enterpriseResource.ID.String()},
					{Type: "ALL_RESOURCE", Ref: leaseResource.ID.String()},
					{Type: "ALL_RESOURCE", Ref: energyResource.ID.String()},
				}},
			},
			ComplianceRequired:   true,
			ContractRequired:     true,
			ContractCode:         "DP-ENTERPRISE-ACTIVITY",
			TraceabilityRequired: true,
			EvidenceRequired:     true,
		},
		ActorID: &actorID,
		TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("create pilot CertificationProfile: %v", err)
	}

	certificationRepo := certificationinfra.NewCertificationRepository(pool)
	resolver := pilotCertificationResolver{input: certificationdomain.EvaluationInput{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		Quality:          certificationdomain.QualityAssessmentEvidence{ID: qualityResult.ID},
		Rights: &certificationdomain.RightsEvidence{
			RightsSnapshotID:          rightsSnapshot.ID,
			EffectiveRightsSnapshotID: effectiveRights.ID,
		},
		Compliance:   &certificationdomain.ComplianceEvidence{ID: complianceResult.ID},
		Contract:     &certificationdomain.ContractEvidence{ID: contractVersion.ID},
		Traceability: &certificationdomain.TraceabilityEvidence{ID: supportingEvidence.ID},
		Evidence:     &certificationdomain.EvidenceSnapshot{ID: supportingEvidence.ID},
		ActorID:      &actorID,
	}}
	certificationService := certificationapp.NewCertificationService(txManager, profileRepo, certificationRepo, resolver)
	certificationCostActivityID := uuid.New()
	certificationCommand := certificationapp.EvaluateDatasetCertificationCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		ProfileID:        profile.ID,
		IdempotencyKey:   "pilot-certification-" + suffix,
		ActorID:          &actorID,
		TraceID:          traceID,
		CostActivity: &cost.CertificationActivity{
			ActivityID:  certificationCostActivityID,
			CostType:    cost.CertificationEvaluationActivity,
			Quantity:    1,
			Unit:        "certification",
			PricingMode: "ACTUAL",
		},
	}
	certification, err := certificationService.Evaluate(ctx, certificationCommand)
	if err != nil {
		t.Fatalf("evaluate pilot DatasetCertification: %v", err)
	}
	if certification.Decision != certificationdomain.DecisionCertified || len(certification.Blockers) != 0 {
		t.Fatalf("pilot DatasetCertification = %s blockers=%+v, want CERTIFIED", certification.Decision, certification.Blockers)
	}
	if certification.EffectiveRightsSnapshotID == nil || *certification.EffectiveRightsSnapshotID != effectiveRights.ID ||
		certification.RightsSnapshotID == nil || *certification.RightsSnapshotID != rightsSnapshot.ID ||
		certification.EvidenceSnapshotID == nil || *certification.EvidenceSnapshotID != supportingEvidence.ID {
		t.Fatalf("pilot certification did not freeze expected rights/evidence facts: %+v", certification)
	}

	certificationReplay, err := certificationService.Evaluate(ctx, certificationCommand)
	if err != nil {
		t.Fatalf("replay pilot DatasetCertification: %v", err)
	}
	if certificationReplay.ID != certification.ID {
		t.Fatalf("pilot certification replay = %s, want original %s", certificationReplay.ID, certification.ID)
	}

	t.Run("certification snapshot memberships reject tamper without changing history", func(t *testing.T) {
		evidenceRepo := evidence.NewQueryRepository(pool)
		evidenceBefore, err := evidenceRepo.GetSnapshot(ctx, supportingEvidence.ID)
		if err != nil {
			t.Fatalf("read frozen Pilot EvidenceSnapshot before tamper: %v", err)
		}
		if !evidenceBefore.IntegrityValid {
			t.Fatal("Pilot EvidenceSnapshot integrity is invalid before tamper")
		}
		rightsBefore, err := rightsRepo.GetSnapshot(ctx, rightsSnapshot.ID)
		if err != nil {
			t.Fatalf("read frozen Pilot RightsSnapshot before tamper: %v", err)
		}
		if rightsBefore.RootHash == "" || len(rightsBefore.DeclarationIDs) == 0 || len(rightsBefore.BindingIDs) == 0 {
			t.Fatalf("Pilot RightsSnapshot is incomplete before tamper: %+v", rightsBefore)
		}
		var rightsAuthorizationCountBefore int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM rights_snapshot_authorization
			WHERE rights_snapshot_id=$1
		`, rightsSnapshot.ID).Scan(&rightsAuthorizationCountBefore); err != nil {
			t.Fatalf("count frozen Pilot RightsSnapshot authorizations: %v", err)
		}
		if rightsAuthorizationCountBefore == 0 {
			t.Fatal("Pilot RightsSnapshot has no frozen Authorization membership")
		}

		var extraEvidence evidence.Record
		if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			record, err := evidence.Append(ctx, tx, evidence.Record{
				WorkspaceID:  workspaceID,
				EvidenceType: "SNAPSHOT_TAMPER_PROBE",
				Title:        "Certified Dataset Pilot snapshot tamper probe",
				SourceType:   "DATASET_VERSION",
				SourceID:     &outputVersion.ID,
				Metadata:     map[string]any{"pilot": "snapshot-tamper"},
				CreatedBy:    &actorID,
			})
			if err != nil {
				return err
			}
			extraEvidence = record
			return nil
		}); err != nil {
			t.Fatalf("create valid Evidence tamper fixture: %v", err)
		}

		tamperAuthorization := activatePilotAuthorizationForConsumer(
			t,
			ctx,
			rightsService,
			workspaceID,
			"AUTH-SNAPSHOT-TAMPER-"+suffix,
			"SNAPSHOT_TAMPER_CONSUMER",
			validFrom,
			validTo,
			grants,
			&actorID,
			traceID,
		)
		var extraBindingID, extraDeclarationID uuid.UUID
		if err := pool.QueryRow(ctx, `
			SELECT id, rights_declaration_id
			FROM authorization_provenance_binding
			WHERE authorization_id=$1
			ORDER BY created_at, id
			LIMIT 1
		`, tamperAuthorization.ID).Scan(&extraBindingID, &extraDeclarationID); err != nil {
			t.Fatalf("load valid RightsSnapshot tamper fixture: %v", err)
		}

		assertTamperRejected := func(label, expected string, exec func() error) {
			t.Helper()
			err := exec()
			if err == nil || !strings.Contains(err.Error(), expected) {
				t.Fatalf("%s error = %v, want %q", label, err, expected)
			}
		}

		assertTamperRejected("EvidenceSnapshot INSERT", "membership is finalized", func() error {
			_, err := pool.Exec(ctx, `
				INSERT INTO evidence_snapshot_item(snapshot_id, evidence_id, category)
				VALUES ($1,$2,'TAMPER')
			`, supportingEvidence.ID, extraEvidence.ID)
			return err
		})
		assertTamperRejected("EvidenceSnapshot UPDATE", "membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				UPDATE evidence_snapshot_item
				SET category='TAMPER'
				WHERE snapshot_id=$1 AND evidence_id=$2
			`, supportingEvidence.ID, supportingEvidence.Items[0].EvidenceID)
			return err
		})
		assertTamperRejected("EvidenceSnapshot DELETE", "membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				DELETE FROM evidence_snapshot_item
				WHERE snapshot_id=$1 AND evidence_id=$2
			`, supportingEvidence.ID, supportingEvidence.Items[0].EvidenceID)
			return err
		})

		assertTamperRejected("RightsSnapshot Authorization INSERT", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				INSERT INTO rights_snapshot_authorization(rights_snapshot_id, authorization_id)
				VALUES ($1,$2)
			`, rightsSnapshot.ID, tamperAuthorization.ID)
			return err
		})
		assertTamperRejected("RightsSnapshot Declaration INSERT", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				INSERT INTO rights_snapshot_declaration(rights_snapshot_id, declaration_id)
				VALUES ($1,$2)
			`, rightsSnapshot.ID, extraDeclarationID)
			return err
		})
		assertTamperRejected("RightsSnapshot Binding INSERT", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				INSERT INTO rights_snapshot_provenance_binding(rights_snapshot_id, binding_id)
				VALUES ($1,$2)
			`, rightsSnapshot.ID, extraBindingID)
			return err
		})
		assertTamperRejected("RightsSnapshot Authorization UPDATE", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				UPDATE rights_snapshot_authorization
				SET authorization_id=$3
				WHERE rights_snapshot_id=$1 AND authorization_id=$2
			`, rightsSnapshot.ID, authorization.ID, tamperAuthorization.ID)
			return err
		})
		assertTamperRejected("RightsSnapshot Declaration UPDATE", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				UPDATE rights_snapshot_declaration
				SET declaration_id=$3
				WHERE rights_snapshot_id=$1 AND declaration_id=$2
			`, rightsSnapshot.ID, rightsBefore.DeclarationIDs[0], extraDeclarationID)
			return err
		})
		assertTamperRejected("RightsSnapshot Binding UPDATE", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				UPDATE rights_snapshot_provenance_binding
				SET binding_id=$3
				WHERE rights_snapshot_id=$1 AND binding_id=$2
			`, rightsSnapshot.ID, rightsBefore.BindingIDs[0], extraBindingID)
			return err
		})
		assertTamperRejected("RightsSnapshot Authorization DELETE", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				DELETE FROM rights_snapshot_authorization
				WHERE rights_snapshot_id=$1 AND authorization_id=$2
			`, rightsSnapshot.ID, authorization.ID)
			return err
		})
		assertTamperRejected("RightsSnapshot Declaration DELETE", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				DELETE FROM rights_snapshot_declaration
				WHERE rights_snapshot_id=$1 AND declaration_id=$2
			`, rightsSnapshot.ID, rightsBefore.DeclarationIDs[0])
			return err
		})
		assertTamperRejected("RightsSnapshot Binding DELETE", "finalized rights_snapshot membership is immutable", func() error {
			_, err := pool.Exec(ctx, `
				DELETE FROM rights_snapshot_provenance_binding
				WHERE rights_snapshot_id=$1 AND binding_id=$2
			`, rightsSnapshot.ID, rightsBefore.BindingIDs[0])
			return err
		})

		evidenceAfter, err := evidenceRepo.GetSnapshot(ctx, supportingEvidence.ID)
		if err != nil {
			t.Fatalf("read frozen Pilot EvidenceSnapshot after tamper attempts: %v", err)
		}
		if !evidenceAfter.IntegrityValid ||
			evidenceAfter.RootHash != evidenceBefore.RootHash ||
			!reflect.DeepEqual(evidenceAfter.Items, evidenceBefore.Items) ||
			!reflect.DeepEqual(evidenceAfter.Manifest, evidenceBefore.Manifest) {
			t.Fatalf("Pilot EvidenceSnapshot changed after rejected tamper: before=%+v after=%+v",
				evidenceBefore, evidenceAfter)
		}

		rightsAfter, err := rightsRepo.GetSnapshot(ctx, rightsSnapshot.ID)
		if err != nil {
			t.Fatalf("read frozen Pilot RightsSnapshot after tamper attempts: %v", err)
		}
		var rightsAuthorizationCountAfter int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM rights_snapshot_authorization
			WHERE rights_snapshot_id=$1
		`, rightsSnapshot.ID).Scan(&rightsAuthorizationCountAfter); err != nil {
			t.Fatalf("count Pilot RightsSnapshot authorizations after tamper: %v", err)
		}
		if rightsAfter.RootHash != rightsBefore.RootHash ||
			!reflect.DeepEqual(rightsAfter.Manifest, rightsBefore.Manifest) ||
			!reflect.DeepEqual(rightsAfter.DeclarationIDs, rightsBefore.DeclarationIDs) ||
			!reflect.DeepEqual(rightsAfter.BindingIDs, rightsBefore.BindingIDs) ||
			rightsAuthorizationCountAfter != rightsAuthorizationCountBefore {
			t.Fatalf("Pilot RightsSnapshot changed after rejected tamper: before=%+v after=%+v authCounts=%d/%d",
				rightsBefore, rightsAfter, rightsAuthorizationCountBefore, rightsAuthorizationCountAfter)
		}

		history, err := certificationService.ListDatasetHistory(ctx, workspaceID, outputVersion.ID, time.Now().UTC())
		if err != nil {
			t.Fatalf("read certification history after snapshot tamper attempts: %v", err)
		}
		found := false
		for _, item := range history {
			if item.Certification.ID != certification.ID {
				continue
			}
			found = true
			if item.Certification.RightsSnapshotID == nil ||
				*item.Certification.RightsSnapshotID != rightsSnapshot.ID ||
				item.Certification.EvidenceSnapshotID == nil ||
				*item.Certification.EvidenceSnapshotID != supportingEvidence.ID {
				t.Fatalf("historical certification snapshot refs changed after rejected tamper: %+v", item.Certification)
			}
		}
		if !found {
			t.Fatalf("historical certification %s disappeared after rejected snapshot tamper", certification.ID)
		}
	})

	t.Run("rule and profile changes do not rewrite historical explanation", func(t *testing.T) {
		originalPolicy := readRepoFile(t, "industry-packs", "park", "quality", "enterprise-activity-quality-v1.yaml")
		modifiedPolicy := bytes.Replace(originalPolicy, []byte("version: 2.0.0"), []byte("version: 9.9.9"), 1)
		if bytes.Equal(modifiedPolicy, originalPolicy) {
			t.Fatal("pilot quality policy fixture version replacement did not change content")
		}
		mutatedIndustryPackRoot := t.TempDir()
		mutatedPolicyPath := filepath.Join(mutatedIndustryPackRoot, "park", "quality", "enterprise-activity-quality-v1.yaml")
		if err := os.MkdirAll(filepath.Dir(mutatedPolicyPath), 0o755); err != nil {
			t.Fatalf("create mutated quality policy directory: %v", err)
		}
		if err := os.WriteFile(mutatedPolicyPath, modifiedPolicy, 0o644); err != nil {
			t.Fatalf("write mutated quality policy: %v", err)
		}

		mutatedQualityService := qualityapp.NewService(
			mutatedIndustryPackRoot,
			txManager,
			datasetRepo,
			qualityRepo,
			store,
			evidence.NewQueryRepository(pool),
		)
		mutatedQuality, err := mutatedQualityService.Run(ctx, qualityapp.RunCommand{
			WorkspaceID:      workspaceID,
			DatasetVersionID: outputVersion.ID,
			RuleSetRef:       qualityRuleSetRef,
			ActorID:          &actorID,
			TraceID:          traceID,
			Now:              outputVersion.ReadyAt.Add(30 * time.Minute),
		})
		if err != nil {
			t.Fatalf("run mutated-rule QualityAssessment: %v", err)
		}
		if mutatedQuality.ID == qualityResult.ID ||
			mutatedQuality.RuleSetVersion != "9.9.9" ||
			mutatedQuality.RuleSetContentSHA256 == qualityResult.RuleSetContentSHA256 {
			t.Fatalf("mutated quality facts = id %s version %s hash %s; original id %s version %s hash %s",
				mutatedQuality.ID, mutatedQuality.RuleSetVersion, mutatedQuality.RuleSetContentSHA256,
				qualityResult.ID, qualityResult.RuleSetVersion, qualityResult.RuleSetContentSHA256)
		}

		storedOriginalQuality, err := qualityRepo.GetAssessment(ctx, qualityResult.ID)
		if err != nil {
			t.Fatalf("reload original QualityAssessment after rule change: %v", err)
		}
		if storedOriginalQuality.RuleSetVersion != qualityResult.RuleSetVersion ||
			storedOriginalQuality.RuleSetContentSHA256 != qualityResult.RuleSetContentSHA256 ||
			storedOriginalQuality.RuleSetContent != qualityResult.RuleSetContent {
			t.Fatalf("historical QualityAssessment drifted after source rule change: stored=%s/%s original=%s/%s",
				storedOriginalQuality.RuleSetVersion, storedOriginalQuality.RuleSetContentSHA256,
				qualityResult.RuleSetVersion, qualityResult.RuleSetContentSHA256)
		}

		profileV2Spec := profile.CertificationProfile
		profileV2Spec.Version = "2.0.0"
		profileV2Spec.RequiredCriticalRules = []string{"QA-COMPANY-ID-COMPLETE", "QA-FRESHNESS"}
		profileV2, err := profileService.Create(ctx, certificationapp.CreateProfileCommand{
			WorkspaceID: workspaceID,
			Profile:     profileV2Spec,
			ActorID:     &actorID,
			TraceID:     traceID,
		})
		if err != nil {
			t.Fatalf("create evolved CertificationProfile: %v", err)
		}
		if profileV2.ID == profile.ID || profileV2.ContentSHA256 == profile.ContentSHA256 {
			t.Fatalf("evolved CertificationProfile did not create a distinct frozen snapshot: v1=%s/%s v2=%s/%s",
				profile.ID, profile.ContentSHA256, profileV2.ID, profileV2.ContentSHA256)
		}

		storedOriginalProfile, err := profileRepo.GetProfile(ctx, profile.ID)
		if err != nil {
			t.Fatalf("reload original CertificationProfile after V2 creation: %v", err)
		}
		if storedOriginalProfile.Version != profile.Version ||
			storedOriginalProfile.ContentSHA256 != profile.ContentSHA256 ||
			!bytes.Equal(storedOriginalProfile.Content, profile.Content) {
			t.Fatalf("historical CertificationProfile drifted after V2 creation: stored=%s/%s original=%s/%s",
				storedOriginalProfile.Version, storedOriginalProfile.ContentSHA256,
				profile.Version, profile.ContentSHA256)
		}

		history, err := certificationService.ListDatasetHistory(ctx, workspaceID, outputVersion.ID, time.Now().UTC())
		if err != nil {
			t.Fatalf("read certification history after profile/rule changes: %v", err)
		}
		found := false
		for _, item := range history {
			if item.Certification.ID != certification.ID {
				continue
			}
			found = true
			if item.Certification.QualityAssessmentID != qualityResult.ID ||
				item.Certification.Profile.ID != profile.ID ||
				item.Certification.Profile.Version != "1.0.0" ||
				item.Certification.Profile.ContentSHA256 != profile.ContentSHA256 ||
				!bytes.Equal(item.Certification.Profile.Content, profile.Content) {
				t.Fatalf("historical DatasetCertification explanation drifted: %+v", item.Certification)
			}
		}
		if !found {
			t.Fatalf("historical DatasetCertification %s disappeared after rule/profile evolution", certification.ID)
		}

		current, err := certificationService.CheckCurrent(ctx, certificationapp.CurrentCertificationQuery{
			WorkspaceID:      workspaceID,
			DatasetVersionID: outputVersion.ID,
			ProfileID:        profile.ID,
			AsOf:             time.Now().UTC(),
			DeliveryContext: certificationdomain.DeliveryContext{
				Purpose: purpose, Action: "READ", Consumer: "LICENSED_BANK", Delivery: "DIRECT_DATA",
			},
		})
		if err != nil {
			t.Fatalf("check original current certification after rule/profile evolution: %v", err)
		}
		if current.Certification.ID != certification.ID || !current.Gate.Allowed {
			t.Fatalf("original current certification changed after rule/profile evolution: id=%s allowed=%v blockers=%+v",
				current.Certification.ID, current.Gate.Allowed, current.Gate.Blockers)
		}
	})

	baseCertificationInput := certificationdomain.EvaluationInput{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		Quality:          certificationdomain.QualityAssessmentEvidence{ID: qualityResult.ID},
		Rights: &certificationdomain.RightsEvidence{
			RightsSnapshotID:          rightsSnapshot.ID,
			EffectiveRightsSnapshotID: effectiveRights.ID,
		},
		Compliance:   &certificationdomain.ComplianceEvidence{ID: complianceResult.ID},
		Contract:     &certificationdomain.ContractEvidence{ID: contractVersion.ID},
		Traceability: &certificationdomain.TraceabilityEvidence{ID: supportingEvidence.ID},
		Evidence:     &certificationdomain.EvidenceSnapshot{ID: supportingEvidence.ID},
		ActorID:      &actorID,
	}
	evaluateRejected := func(label string, input certificationdomain.EvaluationInput) certificationdomain.DatasetCertification {
		t.Helper()
		service := certificationapp.NewCertificationService(
			txManager,
			profileRepo,
			certificationRepo,
			pilotCertificationResolver{input: input},
		)
		result, err := service.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
			WorkspaceID:      workspaceID,
			DatasetVersionID: outputVersion.ID,
			ProfileID:        profile.ID,
			IdempotencyKey:   "pilot-rejected-" + label + "-" + suffix,
			ActorID:          &actorID,
			TraceID:          traceID,
		})
		if err != nil {
			t.Fatalf("evaluate rejected pilot certification %s: %v", label, err)
		}
		if result.Decision != certificationdomain.DecisionRejected {
			t.Fatalf("pilot certification %s = %s blockers=%+v, want REJECTED", label, result.Decision, result.Blockers)
		}
		return result
	}

	t.Run("critical quality failure rejects certification", func(t *testing.T) {
		failedQuality, err := qualityService.Run(ctx, qualityapp.RunCommand{
			WorkspaceID:      workspaceID,
			DatasetVersionID: outputVersion.ID,
			RuleSetRef:       qualityRuleSetRef,
			ActorID:          &actorID,
			TraceID:          traceID,
			Now:              outputVersion.ReadyAt.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("run stale pilot QualityAssessment: %v", err)
		}
		if failedQuality.GateDecision != qualitydomain.GateFail {
			t.Fatalf("stale pilot QualityAssessment gate = %s, want FAIL; findings=%+v", failedQuality.GateDecision, failedQuality.Findings)
		}
		freshnessFailed := false
		for _, finding := range failedQuality.Findings {
			if finding.RuleID == "QA-FRESHNESS" && finding.Status == qualitydomain.FindingFail {
				freshnessFailed = true
				break
			}
		}
		if !freshnessFailed {
			t.Fatalf("stale pilot QualityAssessment did not fail CRITICAL QA-FRESHNESS: %+v", failedQuality.Findings)
		}

		input := baseCertificationInput
		input.Quality = certificationdomain.QualityAssessmentEvidence{ID: failedQuality.ID}
		rejected := evaluateRejected("critical-quality", input)
		if !hasPilotBlocker(rejected.Blockers, "QUALITY_GATE_NOT_PASSED") {
			t.Fatalf("critical-quality rejection blockers=%+v, want QUALITY_GATE_NOT_PASSED", rejected.Blockers)
		}
	})

	t.Run("required governance evidence missing rejects certification", func(t *testing.T) {
		missingRights := baseCertificationInput
		missingRights.Rights = nil
		rightsRejected := evaluateRejected("missing-rights", missingRights)
		if !hasPilotBlocker(rightsRejected.Blockers, "RIGHTS_EVIDENCE_MISSING") {
			t.Fatalf("missing-rights blockers=%+v, want RIGHTS_EVIDENCE_MISSING", rightsRejected.Blockers)
		}

		missingCompliance := baseCertificationInput
		missingCompliance.Compliance = nil
		complianceRejected := evaluateRejected("missing-compliance", missingCompliance)
		if !hasPilotBlocker(complianceRejected.Blockers, "COMPLIANCE_EVIDENCE_MISSING") {
			t.Fatalf("missing-compliance blockers=%+v, want COMPLIANCE_EVIDENCE_MISSING", complianceRejected.Blockers)
		}

		missingContract := baseCertificationInput
		missingContract.Contract = nil
		contractRejected := evaluateRejected("missing-contract", missingContract)
		if !hasPilotBlocker(contractRejected.Blockers, "CONTRACT_EVIDENCE_MISSING") {
			t.Fatalf("missing-contract blockers=%+v, want CONTRACT_EVIDENCE_MISSING", contractRejected.Blockers)
		}
	})

	eligibility := certificationapp.NewEligibilityService(certificationService, datasetRepo, rightsRepo)

	t.Run("profile and authorization context mismatches fail closed", func(t *testing.T) {
		profileMismatch, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
			WorkspaceID:      workspaceID,
			DatasetVersionID: outputVersion.ID,
			ProfileID:        profile.ID,
			Consumer:         "LICENSED_BANK",
			Purpose:          purpose,
			Action:           "RAW_EXPORT",
			Delivery:         "DIRECT_DATA",
			ScopeType:        "ALL_RESOURCE",
			ScopeRef:         outputVersion.ID.String(),
			AsOf:             time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("check profile-context mismatch: %v", err)
		}
		if profileMismatch.Allowed || profileMismatch.CertificationGate.Allowed ||
			!hasPilotBlocker(profileMismatch.CertificationGate.Blockers, "CERTIFICATION_ACTION_NOT_COVERED") {
			t.Fatalf("profile-context mismatch = allowed=%v certificationAllowed=%v blockers=%+v",
				profileMismatch.Allowed, profileMismatch.CertificationGate.Allowed, profileMismatch.Blockers)
		}

		entitlementProfile, err := profileService.Create(ctx, certificationapp.CreateProfileCommand{
			WorkspaceID: workspaceID,
			Profile: certificationdomain.CertificationProfile{
				ProfileRef:            "park/enterprise-activity-entitlement-probe-v1",
				Code:                  "PILOT-ENTITLEMENT-CONTEXT",
				Name:                  "Enterprise Activity Entitlement Context Probe",
				Version:               "1.0.0",
				Purpose:               certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{purpose}},
				Actions:               certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"READ"}},
				Consumers:             certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"GUARANTEE_INSTITUTION"}},
				Delivery:              certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"DIRECT_DATA"}},
				RequiredCriticalRules: []string{"QA-COMPANY-ID-COMPLETE"},
				QualityGateRequired:   true,
				Rights:                certificationdomain.RightsRequirement{Required: false},
				ComplianceRequired:    true,
				ContractRequired:      true,
				ContractCode:          "DP-ENTERPRISE-ACTIVITY",
				TraceabilityRequired:  true,
				EvidenceRequired:      true,
			},
			ActorID: &actorID,
			TraceID: traceID,
		})
		if err != nil {
			t.Fatalf("create entitlement-context CertificationProfile: %v", err)
		}

		entitlementCertificationService := certificationapp.NewCertificationService(
			txManager,
			profileRepo,
			certificationRepo,
			pilotCertificationResolver{input: certificationdomain.EvaluationInput{
				WorkspaceID:      workspaceID,
				DatasetVersionID: outputVersion.ID,
				Quality:          certificationdomain.QualityAssessmentEvidence{ID: qualityResult.ID},
				Compliance:       &certificationdomain.ComplianceEvidence{ID: complianceResult.ID},
				Contract:         &certificationdomain.ContractEvidence{ID: contractVersion.ID},
				Traceability:     &certificationdomain.TraceabilityEvidence{ID: supportingEvidence.ID},
				Evidence:         &certificationdomain.EvidenceSnapshot{ID: supportingEvidence.ID},
				ActorID:          &actorID,
			}},
		)
		entitlementCertification, err := entitlementCertificationService.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
			WorkspaceID:      workspaceID,
			DatasetVersionID: outputVersion.ID,
			ProfileID:        entitlementProfile.ID,
			IdempotencyKey:   "pilot-entitlement-context-certification-" + suffix,
			ActorID:          &actorID,
			TraceID:          traceID,
		})
		if err != nil {
			t.Fatalf("evaluate entitlement-context certification: %v", err)
		}
		if entitlementCertification.Decision != certificationdomain.DecisionCertified {
			t.Fatalf("entitlement-context certification = %s blockers=%+v, want CERTIFIED",
				entitlementCertification.Decision, entitlementCertification.Blockers)
		}

		entitlementEligibility := certificationapp.NewEligibilityService(
			entitlementCertificationService,
			datasetRepo,
			rightsRepo,
		)
		entitlementMismatch, err := entitlementEligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
			WorkspaceID:      workspaceID,
			DatasetVersionID: outputVersion.ID,
			ProfileID:        entitlementProfile.ID,
			Consumer:         "GUARANTEE_INSTITUTION",
			Purpose:          purpose,
			Action:           "READ",
			Delivery:         "DIRECT_DATA",
			ScopeType:        "ALL_RESOURCE",
			ScopeRef:         outputVersion.ID.String(),
			AsOf:             time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("check Authorization/provenance context mismatch: %v", err)
		}
		if entitlementMismatch.Allowed || !entitlementMismatch.CertificationGate.Allowed ||
			entitlementMismatch.EntitlementGate.Allowed ||
			!hasPilotBlocker(entitlementMismatch.EntitlementGate.Blockers, "CURRENT_ENTITLEMENT_BLOCKED") {
			t.Fatalf("Authorization/provenance mismatch = allowed=%v certificationAllowed=%v entitlementAllowed=%v blockers=%+v",
				entitlementMismatch.Allowed, entitlementMismatch.CertificationGate.Allowed,
				entitlementMismatch.EntitlementGate.Allowed, entitlementMismatch.Blockers)
		}
		if len(entitlementMismatch.EntitlementChecks) == 0 {
			t.Fatal("Authorization/provenance mismatch produced no entitlement checks")
		}
		for _, check := range entitlementMismatch.EntitlementChecks {
			if check.Decision.Decision != rightsdomain.DecisionNotAllowed {
				t.Fatalf("mismatched entitlement for resource %s = %s, want NOT_ALLOWED",
					check.DataResourceID, check.Decision.Decision)
			}
		}

		mismatchDelivery := deliveryapp.NewDirectDataService(
			txManager,
			deliveryinfra.NewPostgresRepository(pool),
			deliveryapp.NewCertificationDirectDataGate(entitlementEligibility),
			datasetRepo,
		)
		blockedDelivery, err := mismatchDelivery.Deliver(ctx, deliveryapp.DirectDataCommand{
			WorkspaceID:          workspaceID,
			DatasetVersionID:     outputVersion.ID,
			ProfileID:            entitlementProfile.ID,
			PrincipalRef:         "pilot-principal-guarantee",
			EffectiveConsumerRef: "GUARANTEE_INSTITUTION",
			Purpose:              purpose,
			Action:               "READ",
			ScopeType:            "ALL_RESOURCE",
			ScopeRef:             outputVersion.ID.String(),
			IdempotencyKey:       "pilot-entitlement-context-delivery-" + suffix,
			TraceID:              traceID,
		})
		if err != nil {
			t.Fatalf("deliver Authorization/provenance mismatch context: %v", err)
		}
		if blockedDelivery.PayloadReady || blockedDelivery.Operation.Status != deliverydomain.StatusBlocked ||
			!hasString(blockedDelivery.Blockers, "CURRENT_ENTITLEMENT_BLOCKED") {
			t.Fatalf("Authorization/provenance mismatch delivery = %#v", blockedDelivery)
		}
	})

	eligibilityResult, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		ProfileID:        profile.ID,
		Consumer:         "LICENSED_BANK",
		Purpose:          purpose,
		Action:           "READ",
		Delivery:         "DIRECT_DATA",
		ScopeType:        "ALL_RESOURCE",
		ScopeRef:         outputVersion.ID.String(),
		AsOf:             time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("check pilot Current Delivery Eligibility: %v", err)
	}
	if !eligibilityResult.Allowed || len(eligibilityResult.Blockers) != 0 {
		t.Fatalf("pilot Current Delivery Eligibility = allowed=%v blockers=%+v", eligibilityResult.Allowed, eligibilityResult.Blockers)
	}

	deliveryRepo := deliveryinfra.NewPostgresRepository(pool)
	directData := deliveryapp.NewDirectDataService(
		txManager,
		deliveryRepo,
		deliveryapp.NewCertificationDirectDataGate(eligibility),
		datasetRepo,
	)

	t.Run("delivery fence linearizes revoke and terminal finalize", func(t *testing.T) {
		makeConsumerDelivery := func(label, consumer string) (rightsdomain.Authorization, certificationdomain.ProfileSnapshot, *certificationapp.EligibilityService, *deliveryapp.DirectDataService) {
			t.Helper()
			auth := activatePilotAuthorizationForConsumer(
				t, ctx, rightsService, workspaceID, "AUTH-LINEAR-"+label+"-"+suffix,
				consumer, validFrom, validTo, grants, &actorID, traceID,
			)

			profileSnapshot, err := profileService.Create(ctx, certificationapp.CreateProfileCommand{
				WorkspaceID: workspaceID,
				Profile: certificationdomain.CertificationProfile{
					ProfileRef:            "park/enterprise-activity-linearization-" + label,
					Code:                  "PILOT-LINEARIZATION-" + strings.ToUpper(label),
					Name:                  "Pilot Delivery Fence " + label,
					Version:               "1.0.0",
					Purpose:               certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{purpose}},
					Actions:               certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"READ"}},
					Consumers:             certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{consumer}},
					Delivery:              certificationdomain.Applicability{Mode: certificationdomain.ApplicabilityExplicit, Values: []string{"DIRECT_DATA"}},
					RequiredCriticalRules: []string{"QA-COMPANY-ID-COMPLETE"},
					QualityGateRequired:   true,
					Rights:                certificationdomain.RightsRequirement{Required: false},
					ComplianceRequired:    true,
					ContractRequired:      true,
					ContractCode:          "DP-ENTERPRISE-ACTIVITY",
					TraceabilityRequired:  true,
					EvidenceRequired:      true,
				},
				ActorID: &actorID,
				TraceID: traceID,
			})
			if err != nil {
				t.Fatalf("create %s linearization profile: %v", label, err)
			}

			certService := certificationapp.NewCertificationService(
				txManager,
				profileRepo,
				certificationRepo,
				pilotCertificationResolver{input: certificationdomain.EvaluationInput{
					WorkspaceID:      workspaceID,
					DatasetVersionID: outputVersion.ID,
					Quality:          certificationdomain.QualityAssessmentEvidence{ID: qualityResult.ID},
					Compliance:       &certificationdomain.ComplianceEvidence{ID: complianceResult.ID},
					Contract:         &certificationdomain.ContractEvidence{ID: contractVersion.ID},
					Traceability:     &certificationdomain.TraceabilityEvidence{ID: supportingEvidence.ID},
					Evidence:         &certificationdomain.EvidenceSnapshot{ID: supportingEvidence.ID},
					ActorID:          &actorID,
				}},
			)
			cert, err := certService.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
				WorkspaceID:      workspaceID,
				DatasetVersionID: outputVersion.ID,
				ProfileID:        profileSnapshot.ID,
				IdempotencyKey:   "pilot-linear-cert-" + label + "-" + suffix,
				ActorID:          &actorID,
				TraceID:          traceID,
			})
			if err != nil {
				t.Fatalf("certify %s linearization context: %v", label, err)
			}
			if cert.Decision != certificationdomain.DecisionCertified {
				t.Fatalf("%s linearization certification = %s blockers=%+v", label, cert.Decision, cert.Blockers)
			}

			eligibilityService := certificationapp.NewEligibilityService(certService, datasetRepo, rightsRepo)
			directService := deliveryapp.NewDirectDataService(
				txManager,
				deliveryinfra.NewPostgresRepository(pool),
				deliveryapp.NewCertificationDirectDataGate(eligibilityService),
				datasetRepo,
			)
			return auth, profileSnapshot, eligibilityService, directService
		}

		t.Run("revoke commits before delivery fence", func(t *testing.T) {
			const consumer = "LINEARIZATION_REVOKE_FIRST"
			auth, linearProfile, _, linearDirect := makeConsumerDelivery("revoke-first", consumer)

			stored, err := rightsRepo.GetAuthorization(ctx, auth.ID)
			if err != nil {
				t.Fatalf("load revoke-first Authorization: %v", err)
			}
			expectedStatus := stored.Status
			if err := stored.Revoke(&actorID); err != nil {
				t.Fatalf("prepare revoke-first Authorization transition: %v", err)
			}

			revokeTx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin revoke-first transaction: %v", err)
			}
			defer func() { _ = revokeTx.Rollback(context.Background()) }()
			if err := rightsRepo.SaveAuthorizationState(ctx, revokeTx, stored, expectedStatus); err != nil {
				t.Fatalf("hold revoke-first delivery fence: %v", err)
			}

			probeTx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin revoke-first delivery probe: %v", err)
			}
			if _, err := probeTx.Exec(ctx, `SET LOCAL lock_timeout = '100ms'`); err != nil {
				_ = probeTx.Rollback(ctx)
				t.Fatalf("set revoke-first probe lock timeout: %v", err)
			}
			_, lockErr := deliveryRepo.LockFence(ctx, probeTx, workspaceID)
			var pgErr *pgconn.PgError
			if !errors.As(lockErr, &pgErr) || pgErr.Code != "55P03" {
				_ = probeTx.Rollback(ctx)
				t.Fatalf("delivery fence while revoke-first tx is open = %v, want PostgreSQL 55P03", lockErr)
			}
			_ = probeTx.Rollback(ctx)

			if err := revokeTx.Commit(ctx); err != nil {
				t.Fatalf("commit revoke-first transition: %v", err)
			}

			result, err := linearDirect.Deliver(ctx, deliveryapp.DirectDataCommand{
				WorkspaceID:          workspaceID,
				DatasetVersionID:     outputVersion.ID,
				ProfileID:            linearProfile.ID,
				PrincipalRef:         "pilot-revoke-first-principal",
				EffectiveConsumerRef: consumer,
				Purpose:              purpose,
				Action:               "READ",
				ScopeType:            "ALL_RESOURCE",
				ScopeRef:             outputVersion.ID.String(),
				IdempotencyKey:       "pilot-revoke-first-delivery-" + suffix,
				TraceID:              traceID,
			})
			if err != nil {
				t.Fatalf("deliver after revoke-first commit: %v", err)
			}
			if result.PayloadReady || result.Operation.Status != deliverydomain.StatusBlocked ||
				!hasString(result.Blockers, "CURRENT_ENTITLEMENT_BLOCKED") {
				t.Fatalf("revoke-first delivery = %#v", result)
			}
		})

		t.Run("delivery finalize commits before revoke", func(t *testing.T) {
			const consumer = "LINEARIZATION_FINALIZE_FIRST"
			auth, linearProfile, linearEligibility, _ := makeConsumerDelivery("finalize-first", consumer)

			barrierGate := &pilotBarrierDirectDataGate{
				base:    deliveryapp.NewCertificationDirectDataGate(linearEligibility),
				entered: make(chan struct{}, 1),
				release: make(chan struct{}),
			}
			linearDirect := deliveryapp.NewDirectDataService(
				txManager,
				deliveryinfra.NewPostgresRepository(pool),
				barrierGate,
				datasetRepo,
			)

			command := deliveryapp.DirectDataCommand{
				WorkspaceID:          workspaceID,
				DatasetVersionID:     outputVersion.ID,
				ProfileID:            linearProfile.ID,
				PrincipalRef:         "pilot-finalize-first-principal",
				EffectiveConsumerRef: consumer,
				Purpose:              purpose,
				Action:               "READ",
				ScopeType:            "ALL_RESOURCE",
				ScopeRef:             outputVersion.ID.String(),
				IdempotencyKey:       "pilot-finalize-first-delivery-" + suffix,
				TraceID:              traceID,
			}

			type deliveryOutcome struct {
				result deliveryapp.DirectDataResult
				err    error
			}
			outcomeCh := make(chan deliveryOutcome, 1)
			deliverCtx, cancelDeliver := context.WithTimeout(ctx, 5*time.Second)
			defer cancelDeliver()
			go func() {
				result, err := linearDirect.Deliver(deliverCtx, command)
				outcomeCh <- deliveryOutcome{result: result, err: err}
			}()

			select {
			case <-barrierGate.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("finalize-first delivery never reached in-fence gate barrier")
			}

			stored, err := rightsRepo.GetAuthorization(ctx, auth.ID)
			if err != nil {
				close(barrierGate.release)
				t.Fatalf("load finalize-first Authorization: %v", err)
			}
			expectedStatus := stored.Status
			if err := stored.Revoke(&actorID); err != nil {
				close(barrierGate.release)
				t.Fatalf("prepare finalize-first Authorization transition: %v", err)
			}

			revokeTx, err := pool.Begin(ctx)
			if err != nil {
				close(barrierGate.release)
				t.Fatalf("begin finalize-first revoke transaction: %v", err)
			}
			if _, err := revokeTx.Exec(ctx, `SET LOCAL lock_timeout = '100ms'`); err != nil {
				close(barrierGate.release)
				_ = revokeTx.Rollback(ctx)
				t.Fatalf("set finalize-first revoke lock timeout: %v", err)
			}
			revokeErr := rightsRepo.SaveAuthorizationState(ctx, revokeTx, stored, expectedStatus)
			var pgErr *pgconn.PgError
			if !errors.As(revokeErr, &pgErr) || pgErr.Code != "55P03" {
				close(barrierGate.release)
				_ = revokeTx.Rollback(ctx)
				t.Fatalf("revoke while delivery holds fence = %v, want PostgreSQL 55P03", revokeErr)
			}
			_ = revokeTx.Rollback(ctx)

			close(barrierGate.release)

			var outcome deliveryOutcome
			select {
			case outcome = <-outcomeCh:
			case <-time.After(5 * time.Second):
				t.Fatal("finalize-first delivery did not complete after releasing barrier")
			}
			if outcome.err != nil {
				t.Fatalf("finalize-first delivery: %v", outcome.err)
			}
			if !outcome.result.PayloadReady || outcome.result.Operation.Status != deliverydomain.StatusIssued {
				t.Fatalf("finalize-first delivery = %#v", outcome.result)
			}

			if _, err := rightsService.Revoke(ctx, rightsapp.TransitionCommand{
				AuthorizationID: auth.ID,
				ActorID:         &actorID,
				TraceID:         traceID,
			}); err != nil {
				t.Fatalf("revoke after finalize-first ISSUED commit: %v", err)
			}

			var storedStatus string
			if err := pool.QueryRow(ctx, `
				SELECT status
				FROM delivery_operation
				WHERE id=$1
			`, outcome.result.Operation.ID).Scan(&storedStatus); err != nil {
				t.Fatalf("read finalize-first DeliveryOperation after revoke: %v", err)
			}
			if storedStatus != string(deliverydomain.StatusIssued) {
				t.Fatalf("finalize-first historical operation status = %s, want ISSUED", storedStatus)
			}

			replacement := command
			replacement.IdempotencyKey = "pilot-finalize-first-replacement-" + suffix
			replacement.RetryOfDeliveryOperationID = &outcome.result.Operation.ID
			blocked, err := deliveryapp.NewDirectDataService(
				txManager,
				deliveryinfra.NewPostgresRepository(pool),
				deliveryapp.NewCertificationDirectDataGate(linearEligibility),
				datasetRepo,
			).Deliver(ctx, replacement)
			if err != nil {
				t.Fatalf("replacement after finalize-first revoke: %v", err)
			}
			if blocked.PayloadReady || blocked.Operation.Status != deliverydomain.StatusBlocked ||
				!hasString(blocked.Blockers, "CURRENT_ENTITLEMENT_BLOCKED") {
				t.Fatalf("finalize-first replacement after revoke = %#v", blocked)
			}
		})
	})

	t.Run("HTTP reads object only after ISSUED commit and never on BLOCKED", func(t *testing.T) {
		const (
			trustedToken     = "pilot-response-boundary-token"
			trustedPrincipal = "pilot-response-boundary-principal"
			trustedConsumer  = "LICENSED_BANK"
		)
		resolver, err := deliveryhttp.NewStaticPrincipalResolver(
			true,
			trustedToken,
			trustedPrincipal,
			trustedConsumer,
			[]string{workspaceID.String()},
		)
		if err != nil {
			t.Fatalf("configure response-boundary principal resolver: %v", err)
		}

		execute := func(handler *deliveryhttp.Handler, action, key string) *httptest.ResponseRecorder {
			t.Helper()
			body, err := json.Marshal(map[string]any{
				"profileId": profile.ID.String(),
				"consumer":  trustedConsumer,
				"purpose":   purpose,
				"action":    action,
				"scopeType": "ALL_RESOURCE",
				"scopeRef":  outputVersion.ID.String(),
			})
			if err != nil {
				t.Fatalf("marshal response-boundary delivery request: %v", err)
			}
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/workspaces/"+workspaceID.String()+"/dataset-versions/"+outputVersion.ID.String()+"/deliveries",
				bytes.NewReader(body),
			)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+trustedToken)
			request.Header.Set("Idempotency-Key", key)
			request.Header.Set("X-Trace-ID", traceID)
			response := httptest.NewRecorder()
			mux := http.NewServeMux()
			handler.Register(mux)
			mux.ServeHTTP(response, request)
			return response
		}

		successKey := "pilot-commit-before-read-" + suffix
		successStore := &pilotCommitObservingStore{
			base:           store,
			query:          pool,
			workspaceID:    workspaceID,
			idempotencyKey: successKey,
		}
		success := execute(deliveryhttp.NewHandler(directData, resolver, successStore), "READ", successKey)
		if success.Code != http.StatusOK {
			t.Fatalf("commit-before-read delivery = %d %s, want 200", success.Code, success.Body.String())
		}
		if successStore.calls != 1 || successStore.observedStatus != string(deliverydomain.StatusIssued) {
			t.Fatalf("object read boundary = calls %d observed status %q, want 1/ISSUED",
				successStore.calls, successStore.observedStatus)
		}
		if !bytes.Equal(success.Body.Bytes(), store.bytes(outputVersion.StorageURI)) {
			t.Fatal("commit-before-read response bytes do not match certified CURATED DatasetVersion")
		}

		blockedKey := "pilot-blocked-no-read-" + suffix
		blockedStore := &pilotCommitObservingStore{
			base:           store,
			query:          pool,
			workspaceID:    workspaceID,
			idempotencyKey: blockedKey,
		}
		blockedResponse := execute(deliveryhttp.NewHandler(directData, resolver, blockedStore), "RAW_EXPORT", blockedKey)
		if blockedResponse.Code != http.StatusForbidden {
			t.Fatalf("profile-blocked HTTP delivery = %d %s, want 403", blockedResponse.Code, blockedResponse.Body.String())
		}
		if blockedStore.calls != 0 {
			t.Fatalf("BLOCKED HTTP delivery opened object storage %d times, want 0", blockedStore.calls)
		}
		var blockedStatus string
		if err := pool.QueryRow(ctx, `
			SELECT status
			FROM delivery_operation
			WHERE workspace_id=$1 AND idempotency_key=$2
		`, workspaceID, blockedKey).Scan(&blockedStatus); err != nil {
			t.Fatalf("read BLOCKED response-boundary DeliveryOperation: %v", err)
		}
		if blockedStatus != string(deliverydomain.StatusBlocked) {
			t.Fatalf("BLOCKED response-boundary operation status = %s, want BLOCKED", blockedStatus)
		}
	})

	t.Run("response loss before first byte requires explicit fresh replacement", func(t *testing.T) {
		lostCommand := deliveryapp.DirectDataCommand{
			WorkspaceID:          workspaceID,
			DatasetVersionID:     outputVersion.ID,
			ProfileID:            profile.ID,
			PrincipalRef:         "pilot-response-loss-principal",
			EffectiveConsumerRef: "LICENSED_BANK",
			Purpose:              purpose,
			Action:               "READ",
			ScopeType:            "ALL_RESOURCE",
			ScopeRef:             outputVersion.ID.String(),
			IdempotencyKey:       "pilot-response-loss-" + suffix,
			TraceID:              traceID,
		}
		lost, err := directData.Deliver(ctx, lostCommand)
		if err != nil {
			t.Fatalf("linearize response-loss delivery: %v", err)
		}
		if !lost.PayloadReady || lost.ReplayRequired || lost.Operation.Status != deliverydomain.StatusIssued {
			t.Fatalf("response-loss initial result = %#v", lost)
		}

		// Intentionally do not open lost.DatasetVersion.StorageURI. This is the
		// fault window after the ISSUED transaction committed but before the
		// HTTP layer emitted the first dataset byte.
		replay, err := directData.Deliver(ctx, lostCommand)
		if !errors.Is(err, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) {
			t.Fatalf("response-loss same-key replay error = %v, want replay-required", err)
		}
		if replay.PayloadReady || !replay.ReplayRequired || replay.Operation.ID != lost.Operation.ID {
			t.Fatalf("response-loss same-key replay = %#v", replay)
		}

		replacementCommand := lostCommand
		replacementCommand.IdempotencyKey = "pilot-response-loss-replacement-" + suffix
		replacementCommand.RetryOfDeliveryOperationID = &lost.Operation.ID
		replacement, err := directData.Deliver(ctx, replacementCommand)
		if err != nil {
			t.Fatalf("response-loss replacement delivery: %v", err)
		}
		if !replacement.PayloadReady || replacement.ReplayRequired ||
			replacement.Operation.Status != deliverydomain.StatusIssued ||
			replacement.Operation.ID == lost.Operation.ID {
			t.Fatalf("response-loss replacement result = %#v", replacement)
		}

		var retryOf uuid.UUID
		if err := pool.QueryRow(ctx, `
			SELECT retry_of_delivery_operation_id
			FROM delivery_operation
			WHERE id=$1
		`, replacement.Operation.ID).Scan(&retryOf); err != nil {
			t.Fatalf("read response-loss replacement relation: %v", err)
		}
		if retryOf != lost.Operation.ID {
			t.Fatalf("response-loss replacement retry_of = %s, want %s", retryOf, lost.Operation.ID)
		}
		var terminalAllowed int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM delivery_gate_evaluation
			WHERE delivery_operation_id=$1
			  AND stage='TERMINAL_FINALIZE'
			  AND decision='ALLOWED'
		`, replacement.Operation.ID).Scan(&terminalAllowed); err != nil {
			t.Fatalf("count response-loss replacement terminal gate: %v", err)
		}
		if terminalAllowed != 1 {
			t.Fatalf("response-loss replacement terminal ALLOWED gates = %d, want 1", terminalAllowed)
		}

		object, err := store.Get(ctx, replacement.DatasetVersion.StorageURI)
		if err != nil {
			t.Fatalf("open replacement dataset only after fresh ISSUED commit: %v", err)
		}
		replacementBytes, err := io.ReadAll(object)
		_ = object.Close()
		if err != nil {
			t.Fatalf("read replacement dataset bytes: %v", err)
		}
		if !bytes.Equal(replacementBytes, store.bytes(outputVersion.StorageURI)) {
			t.Fatal("response-loss replacement bytes do not match certified CURATED DatasetVersion")
		}
	})

	t.Run("trusted HTTP principal cannot spoof effective consumer", func(t *testing.T) {
		const (
			trustedToken     = "pilot-trusted-delivery-token"
			trustedPrincipal = "pilot-principal-licensed-bank"
			trustedConsumer  = "LICENSED_BANK"
		)
		resolver, err := deliveryhttp.NewStaticPrincipalResolver(
			true,
			trustedToken,
			trustedPrincipal,
			trustedConsumer,
			[]string{workspaceID.String()},
		)
		if err != nil {
			t.Fatalf("configure Pilot trusted principal resolver: %v", err)
		}
		handler := deliveryhttp.NewHandler(directData, resolver, store)
		mux := http.NewServeMux()
		handler.Register(mux)

		deliverHTTP := func(consumer, idempotencyKey string) *httptest.ResponseRecorder {
			t.Helper()
			body, err := json.Marshal(map[string]any{
				"profileId": profile.ID.String(),
				"consumer":  consumer,
				"purpose":   purpose,
				"action":    "READ",
				"scopeType": "ALL_RESOURCE",
				"scopeRef":  outputVersion.ID.String(),
			})
			if err != nil {
				t.Fatalf("marshal Pilot HTTP delivery request: %v", err)
			}
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/workspaces/"+workspaceID.String()+"/dataset-versions/"+outputVersion.ID.String()+"/deliveries",
				bytes.NewReader(body),
			)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+trustedToken)
			request.Header.Set("Idempotency-Key", idempotencyKey)
			request.Header.Set("X-Trace-ID", traceID)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			return response
		}

		var operationsBefore int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_operation WHERE workspace_id=$1`, workspaceID).Scan(&operationsBefore); err != nil {
			t.Fatalf("count Pilot delivery operations before spoof: %v", err)
		}

		spoof := deliverHTTP("GUARANTEE_INSTITUTION", "pilot-http-spoof-"+suffix)
		if spoof.Code != http.StatusForbidden || !strings.Contains(spoof.Body.String(), "CONSUMER_PRINCIPAL_MISMATCH") {
			t.Fatalf("spoofed Pilot HTTP delivery = %d %s, want 403 CONSUMER_PRINCIPAL_MISMATCH", spoof.Code, spoof.Body.String())
		}
		var operationsAfterSpoof int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_operation WHERE workspace_id=$1`, workspaceID).Scan(&operationsAfterSpoof); err != nil {
			t.Fatalf("count Pilot delivery operations after spoof: %v", err)
		}
		if operationsAfterSpoof != operationsBefore {
			t.Fatalf("spoofed caller created DeliveryOperation: before=%d after=%d", operationsBefore, operationsAfterSpoof)
		}

		allowed := deliverHTTP(trustedConsumer, "pilot-http-allowed-"+suffix)
		if allowed.Code != http.StatusOK {
			t.Fatalf("trusted Pilot HTTP delivery = %d %s, want 200", allowed.Code, allowed.Body.String())
		}
		if !bytes.Equal(allowed.Body.Bytes(), store.bytes(outputVersion.StorageURI)) {
			t.Fatal("trusted Pilot HTTP delivery bytes do not match certified CURATED DatasetVersion")
		}
		operationID, err := uuid.Parse(allowed.Header().Get("X-Delivery-Operation-Id"))
		if err != nil || operationID == uuid.Nil {
			t.Fatalf("trusted Pilot HTTP delivery operation header = %q", allowed.Header().Get("X-Delivery-Operation-Id"))
		}
		var principalRef, consumerRef, status string
		var storedVersionID uuid.UUID
		if err := pool.QueryRow(ctx, `
			SELECT principal_ref, effective_consumer_ref, status, dataset_version_id
			FROM delivery_operation
			WHERE id=$1
		`, operationID).Scan(&principalRef, &consumerRef, &status, &storedVersionID); err != nil {
			t.Fatalf("read trusted Pilot HTTP DeliveryOperation: %v", err)
		}
		if principalRef != trustedPrincipal || consumerRef != trustedConsumer ||
			status != string(deliverydomain.StatusIssued) || storedVersionID != outputVersion.ID {
			t.Fatalf("trusted Pilot HTTP DeliveryOperation = principal=%q consumer=%q status=%q version=%s",
				principalRef, consumerRef, status, storedVersionID)
		}
	})

	deliveryCommand := deliveryapp.DirectDataCommand{
		WorkspaceID:          workspaceID,
		DatasetVersionID:     outputVersion.ID,
		ProfileID:            profile.ID,
		PrincipalRef:         "pilot-principal-licensed-bank",
		EffectiveConsumerRef: "LICENSED_BANK",
		Purpose:              purpose,
		Action:               "READ",
		ScopeType:            "ALL_RESOURCE",
		ScopeRef:             outputVersion.ID.String(),
		IdempotencyKey:       "pilot-direct-data-" + suffix,
		TraceID:              traceID,
	}
	delivered, err := directData.Deliver(ctx, deliveryCommand)
	if err != nil {
		t.Fatalf("deliver Certified Dataset DIRECT_DATA: %v", err)
	}
	if !delivered.PayloadReady || delivered.ReplayRequired || delivered.Operation.Status != deliverydomain.StatusIssued {
		t.Fatalf("pilot DIRECT_DATA result = %#v", delivered)
	}
	if delivered.Operation.CertificationRef == nil || *delivered.Operation.CertificationRef != certification.ID {
		t.Fatalf("pilot DeliveryOperation certification = %v, want %s", delivered.Operation.CertificationRef, certification.ID)
	}
	if delivered.DatasetVersion.ID != outputVersion.ID ||
		!bytes.Equal(store.bytes(delivered.DatasetVersion.StorageURI), store.bytes(outputVersion.StorageURI)) {
		t.Fatal("pilot DIRECT_DATA bytes do not resolve to the certified CURATED DatasetVersion")
	}

	replay, err := directData.Deliver(ctx, deliveryCommand)
	if !errors.Is(err, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) {
		t.Fatalf("same-key pilot DIRECT_DATA replay error = %v, want replay-required", err)
	}
	if replay.PayloadReady || !replay.ReplayRequired || replay.Operation.ID != delivered.Operation.ID {
		t.Fatalf("same-key pilot DIRECT_DATA replay = %#v", replay)
	}

	t.Run("cost audit and evidence remain traceable and replay-safe", func(t *testing.T) {
		var qualityCostCount int
		if err := pool.QueryRow(ctx, `
			SELECT count(DISTINCT e.id)
			FROM cost_event e
			JOIN cost_allocation ca ON ca.cost_event_id=e.id
			JOIN quality_assessment_attempt_outcome o
			  ON o.attempt_id=ca.quality_assessment_attempt_id
			WHERE o.assessment_id=$1
			  AND e.cost_type='QUALITY_ENGINE_INVOCATION'
		`, qualityResult.ID).Scan(&qualityCostCount); err != nil {
			t.Fatalf("count Pilot Quality cost events: %v", err)
		}
		if qualityCostCount != 1 {
			t.Fatalf("Pilot Quality cost events = %d, want 1", qualityCostCount)
		}

		var rightsVerificationCostCount int
		if err := pool.QueryRow(ctx, `
			SELECT count(DISTINCT e.id)
			FROM cost_event e
			JOIN cost_allocation ca ON ca.cost_event_id=e.id
			JOIN rights_declaration_verification v
			  ON v.id=ca.rights_declaration_verification_id
			JOIN authorization_provenance_binding b
			  ON b.rights_declaration_id=v.declaration_id
			WHERE b.authorization_id=$1
			  AND e.cost_type='RIGHTS_DECLARATION_VERIFICATION'
		`, authorization.ID).Scan(&rightsVerificationCostCount); err != nil {
			t.Fatalf("count Pilot Rights verification cost events: %v", err)
		}
		if rightsVerificationCostCount != 3 {
			t.Fatalf("Pilot Rights verification cost events = %d, want 3", rightsVerificationCostCount)
		}

		var effectiveRightsCostCount int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM cost_event e
			JOIN cost_allocation ca ON ca.cost_event_id=e.id
			WHERE ca.effective_rights_snapshot_id=$1
			  AND e.cost_type='EFFECTIVE_RIGHTS_COMPUTE'
		`, effectiveRights.ID).Scan(&effectiveRightsCostCount); err != nil {
			t.Fatalf("count Pilot EffectiveRights cost events: %v", err)
		}
		if effectiveRightsCostCount != 1 {
			t.Fatalf("Pilot EffectiveRights cost events = %d, want 1", effectiveRightsCostCount)
		}

		var certificationCostCount int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM cost_event e
			JOIN cost_allocation ca ON ca.cost_event_id=e.id
			WHERE ca.dataset_certification_id=$1
			  AND e.cost_type='CERTIFICATION_EVALUATION'
		`, certification.ID).Scan(&certificationCostCount); err != nil {
			t.Fatalf("count Pilot Certification cost events: %v", err)
		}
		if certificationCostCount != 1 {
			t.Fatalf("Pilot Certification cost events = %d, want 1", certificationCostCount)
		}

		var directDataCostCount int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM cost_event e
			JOIN cost_allocation ca ON ca.cost_event_id=e.id
			WHERE ca.delivery_operation_id=$1
			  AND e.cost_type='DIRECT_DATA_DELIVERY_ATTEMPT'
			  AND e.activity_id=$1
		`, delivered.Operation.ID).Scan(&directDataCostCount); err != nil {
			t.Fatalf("count Pilot DIRECT_DATA cost events: %v", err)
		}
		if directDataCostCount != 1 {
			t.Fatalf("Pilot DIRECT_DATA cost events = %d, want 1", directDataCostCount)
		}

		// Replays above must reuse the same Certification/DeliveryOperation and
		// therefore must not append another physical activity cost.
		var certificationCostAfterReplay, directDataCostAfterReplay int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM cost_event e
			JOIN cost_allocation ca ON ca.cost_event_id=e.id
			WHERE ca.dataset_certification_id=$1
			  AND e.cost_type='CERTIFICATION_EVALUATION'
		`, certification.ID).Scan(&certificationCostAfterReplay); err != nil {
			t.Fatalf("count replayed Pilot Certification cost events: %v", err)
		}
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM cost_event e
			JOIN cost_allocation ca ON ca.cost_event_id=e.id
			WHERE ca.delivery_operation_id=$1
			  AND e.cost_type='DIRECT_DATA_DELIVERY_ATTEMPT'
		`, delivered.Operation.ID).Scan(&directDataCostAfterReplay); err != nil {
			t.Fatalf("count replayed Pilot DIRECT_DATA cost events: %v", err)
		}
		if certificationCostAfterReplay != 1 || directDataCostAfterReplay != 1 {
			t.Fatalf("Pilot replay duplicated costs: certification=%d delivery=%d, want 1/1",
				certificationCostAfterReplay, directDataCostAfterReplay)
		}

		type traceExpectation struct {
			label        string
			objectType   string
			objectID     uuid.UUID
			evidenceType string
			sourceType   string
		}
		for _, expected := range []traceExpectation{
			{label: "Quality", objectType: "QUALITY_RESULT", objectID: qualityResult.ID, evidenceType: "QUALITY_RESULT", sourceType: "QUALITY_RESULT"},
			{label: "EffectiveRights", objectType: "EFFECTIVE_RIGHTS_SNAPSHOT", objectID: effectiveRights.ID, evidenceType: "EFFECTIVE_RIGHTS_SNAPSHOT", sourceType: "EFFECTIVE_RIGHTS_SNAPSHOT"},
			{label: "Certification", objectType: "DATASET_CERTIFICATION", objectID: certification.ID, evidenceType: "DATASET_CERTIFICATION_DECISION", sourceType: "DATASET_CERTIFICATION"},
			{label: "Delivery", objectType: "DELIVERY_OPERATION", objectID: delivered.Operation.ID, evidenceType: "", sourceType: "DELIVERY_OPERATION"},
		} {
			var auditCount int
			if err := pool.QueryRow(ctx, `
				SELECT count(*)
				FROM audit_event
				WHERE object_type=$1 AND object_id=$2
			`, expected.objectType, expected.objectID).Scan(&auditCount); err != nil {
				t.Fatalf("count Pilot %s Audit facts: %v", expected.label, err)
			}
			if auditCount == 0 {
				t.Fatalf("Pilot %s has no Audit facts", expected.label)
			}

			var evidenceCount int
			if expected.evidenceType != "" {
				if err := pool.QueryRow(ctx, `
					SELECT count(*)
					FROM evidence
					WHERE source_type=$1 AND source_id=$2 AND evidence_type=$3
				`, expected.sourceType, expected.objectID, expected.evidenceType).Scan(&evidenceCount); err != nil {
					t.Fatalf("count Pilot %s Evidence facts: %v", expected.label, err)
				}
			} else {
				if err := pool.QueryRow(ctx, `
					SELECT count(*)
					FROM evidence
					WHERE source_type=$1 AND source_id=$2
				`, expected.sourceType, expected.objectID).Scan(&evidenceCount); err != nil {
					t.Fatalf("count Pilot %s Evidence facts: %v", expected.label, err)
				}
			}
			if evidenceCount == 0 {
				t.Fatalf("Pilot %s has no Evidence facts", expected.label)
			}
		}
	})

	// A UI/preflight ALLOWED result is not an authorization token. Re-check the
	// exact current context, then revoke the underlying Authorization before a
	// replacement delivery attempt. The replacement must fresh re-gate and fail
	// closed while preserving the historical CERTIFIED fact.
	beforeRevoke, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		ProfileID:        profile.ID,
		Consumer:         "LICENSED_BANK",
		Purpose:          purpose,
		Action:           "READ",
		Delivery:         "DIRECT_DATA",
		ScopeType:        "ALL_RESOURCE",
		ScopeRef:         outputVersion.ID.String(),
		AsOf:             time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("pre-revocation delivery eligibility: %v", err)
	}
	if !beforeRevoke.Allowed || beforeRevoke.Certification.ID != certification.ID {
		t.Fatalf("pre-revocation eligibility = allowed=%v certification=%s, want ALLOWED/%s", beforeRevoke.Allowed, beforeRevoke.Certification.ID, certification.ID)
	}

	if _, err := rightsService.Revoke(ctx, rightsapp.TransitionCommand{
		AuthorizationID: authorization.ID,
		ActorID:         &actorID,
		TraceID:         traceID,
	}); err != nil {
		t.Fatalf("revoke pilot Authorization: %v", err)
	}

	afterRevoke, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		ProfileID:        profile.ID,
		Consumer:         "LICENSED_BANK",
		Purpose:          purpose,
		Action:           "READ",
		Delivery:         "DIRECT_DATA",
		ScopeType:        "ALL_RESOURCE",
		ScopeRef:         outputVersion.ID.String(),
		AsOf:             time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("post-revocation delivery eligibility: %v", err)
	}
	if afterRevoke.Allowed || !afterRevoke.CertificationGate.Allowed || afterRevoke.EntitlementGate.Allowed ||
		afterRevoke.Certification.ID != certification.ID || !hasPilotBlocker(afterRevoke.Blockers, "CURRENT_ENTITLEMENT_BLOCKED") {
		t.Fatalf("post-revocation eligibility = allowed=%v certificationAllowed=%v entitlementAllowed=%v certification=%s blockers=%+v",
			afterRevoke.Allowed, afterRevoke.CertificationGate.Allowed, afterRevoke.EntitlementGate.Allowed,
			afterRevoke.Certification.ID, afterRevoke.Blockers)
	}

	replacement := deliveryCommand
	replacement.IdempotencyKey = "pilot-direct-data-replacement-" + suffix
	replacement.RetryOfDeliveryOperationID = &delivered.Operation.ID
	blocked, err := directData.Deliver(ctx, replacement)
	if err != nil {
		t.Fatalf("post-revocation replacement delivery: %v", err)
	}
	if blocked.PayloadReady || blocked.ReplayRequired || blocked.Operation.Status != deliverydomain.StatusBlocked ||
		!hasString(blocked.Blockers, "CURRENT_ENTITLEMENT_BLOCKED") {
		t.Fatalf("post-revocation replacement result = %#v", blocked)
	}

	var blockedEvents, blockedGateFacts int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_event
		WHERE aggregate_type='DELIVERY_OPERATION' AND aggregate_id=$1 AND event_type='DatasetDeliveryBlocked'
	`, blocked.Operation.ID).Scan(&blockedEvents); err != nil {
		t.Fatalf("count post-revocation DatasetDeliveryBlocked events: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM delivery_gate_evaluation
		WHERE delivery_operation_id=$1 AND stage='TERMINAL_FINALIZE' AND decision='BLOCKED'
	`, blocked.Operation.ID).Scan(&blockedGateFacts); err != nil {
		t.Fatalf("count post-revocation blocked gate evaluations: %v", err)
	}
	if blockedEvents != 1 || blockedGateFacts != 1 {
		t.Fatalf("post-revocation delivery facts blockedEvents=%d blockedGateFacts=%d, want 1/1", blockedEvents, blockedGateFacts)
	}

	invalidateVersion := datasetapp.NewInvalidateVersionService(txManager, datasetRepo)
	invalidated, err := invalidateVersion.Handle(ctx, datasetapp.InvalidateVersionCommand{
		VersionID: outputVersion.ID,
		Reason:    "Certified Dataset Pilot current-facts invalidation",
		ActorID:   &actorID,
		TraceID:   traceID,
	})
	if err != nil {
		t.Fatalf("invalidate pilot DatasetVersion: %v", err)
	}
	if invalidated.Status != datasetdomain.VersionInvalid {
		t.Fatalf("pilot invalidated DatasetVersion status = %s, want INVALID", invalidated.Status)
	}

	afterInvalidation, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		ProfileID:        profile.ID,
		Consumer:         "LICENSED_BANK",
		Purpose:          purpose,
		Action:           "READ",
		Delivery:         "DIRECT_DATA",
		ScopeType:        "ALL_RESOURCE",
		ScopeRef:         outputVersion.ID.String(),
		AsOf:             time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("post-invalidation delivery eligibility: %v", err)
	}
	if afterInvalidation.Allowed || afterInvalidation.DatasetVersionGate.Allowed ||
		afterInvalidation.Certification.ID != certification.ID ||
		!hasPilotBlocker(afterInvalidation.Blockers, "DATASET_VERSION_INVALID") {
		t.Fatalf("post-invalidation eligibility = allowed=%v datasetAllowed=%v certification=%s blockers=%+v",
			afterInvalidation.Allowed, afterInvalidation.DatasetVersionGate.Allowed,
			afterInvalidation.Certification.ID, afterInvalidation.Blockers)
	}

	invalidReplacement := deliveryCommand
	invalidReplacement.IdempotencyKey = "pilot-direct-data-invalid-version-" + suffix
	invalidReplacement.RetryOfDeliveryOperationID = &delivered.Operation.ID
	invalidBlocked, err := directData.Deliver(ctx, invalidReplacement)
	if err != nil {
		t.Fatalf("post-invalidation replacement delivery: %v", err)
	}
	if invalidBlocked.PayloadReady || invalidBlocked.Operation.Status != deliverydomain.StatusBlocked ||
		!hasString(invalidBlocked.Blockers, "DATASET_VERSION_INVALID") {
		t.Fatalf("post-invalidation replacement result = %#v", invalidBlocked)
	}

	disposition, err := certificationService.ChangeDisposition(ctx, certificationapp.ChangeCertificationDispositionCommand{
		WorkspaceID:     workspaceID,
		CertificationID: certification.ID,
		Disposition:     certificationdomain.DispositionRevoked,
		Reason:          "Certified Dataset Pilot certification revocation",
		EffectiveAt:     time.Now().UTC(),
		IdempotencyKey:  "pilot-certification-revoke-" + suffix,
		ActorID:         &actorID,
		TraceID:         traceID,
	})
	if err != nil {
		t.Fatalf("revoke pilot DatasetCertification: %v", err)
	}
	if disposition.CertificationID != certification.ID || disposition.Disposition != certificationdomain.DispositionRevoked {
		t.Fatalf("pilot certification disposition = %#v", disposition)
	}

	afterCertificationRevoke, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		ProfileID:        profile.ID,
		Consumer:         "LICENSED_BANK",
		Purpose:          purpose,
		Action:           "READ",
		Delivery:         "DIRECT_DATA",
		ScopeType:        "ALL_RESOURCE",
		ScopeRef:         outputVersion.ID.String(),
		AsOf:             time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("post-certification-revoke delivery eligibility: %v", err)
	}
	if afterCertificationRevoke.Allowed || afterCertificationRevoke.CertificationGate.Allowed ||
		!hasPilotBlocker(afterCertificationRevoke.Blockers, "CERTIFICATION_NOT_CURRENT") {
		t.Fatalf("post-certification-revoke eligibility = allowed=%v certificationAllowed=%v blockers=%+v",
			afterCertificationRevoke.Allowed, afterCertificationRevoke.CertificationGate.Allowed,
			afterCertificationRevoke.Blockers)
	}

	history, err := certificationService.ListDatasetHistory(ctx, workspaceID, outputVersion.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("read pilot certification history after revocation: %v", err)
	}
	foundHistoricalCertification := false
	for _, item := range history {
		if item.Certification.ID == certification.ID {
			foundHistoricalCertification = true
			break
		}
	}
	if !foundHistoricalCertification {
		t.Fatalf("historical certification %s disappeared after disposition", certification.ID)
	}

	certificationReplacement := deliveryCommand
	certificationReplacement.IdempotencyKey = "pilot-direct-data-certification-revoked-" + suffix
	certificationReplacement.RetryOfDeliveryOperationID = &delivered.Operation.ID
	certificationBlocked, err := directData.Deliver(ctx, certificationReplacement)
	if err != nil {
		t.Fatalf("post-certification-revoke replacement delivery: %v", err)
	}
	if certificationBlocked.PayloadReady || certificationBlocked.Operation.Status != deliverydomain.StatusBlocked ||
		!hasString(certificationBlocked.Blockers, "CERTIFICATION_NOT_CURRENT") {
		t.Fatalf("post-certification-revoke replacement result = %#v", certificationBlocked)
	}

	var issuedEvents, gateFacts int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_event
		WHERE aggregate_type='DELIVERY_OPERATION' AND aggregate_id=$1 AND event_type='DatasetDeliveryIssued'
	`, delivered.Operation.ID).Scan(&issuedEvents); err != nil {
		t.Fatalf("count pilot DatasetDeliveryIssued events: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM delivery_gate_evaluation
		WHERE delivery_operation_id=$1 AND stage='TERMINAL_FINALIZE'
	`, delivered.Operation.ID).Scan(&gateFacts); err != nil {
		t.Fatalf("count pilot terminal gate evaluations: %v", err)
	}
	if issuedEvents != 1 || gateFacts != 1 {
		t.Fatalf("pilot delivery facts issuedEvents=%d terminalGateFacts=%d, want 1/1", issuedEvents, gateFacts)
	}

	t.Run("new DatasetVersion does not inherit quality or certification", func(t *testing.T) {
		newVersion := mustUploadCSV(
			t,
			ctx,
			uploadDataset,
			activityDataset.ID,
			"pilot-new-version-"+suffix+".csv",
			store.bytes(outputVersion.StorageURI),
			map[string]any{
				"unresolvedEntityRate":       0.0,
				"acceptedNegativeEnergyRate": 0.0,
			},
			nil,
			&actorID,
			traceID,
		)
		if newVersion.ID == outputVersion.ID || newVersion.VersionNo <= outputVersion.VersionNo {
			t.Fatalf("new pilot DatasetVersion = id %s version %d, previous %s/%d", newVersion.ID, newVersion.VersionNo, outputVersion.ID, outputVersion.VersionNo)
		}

		var qualityCount, certificationCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM quality_result WHERE dataset_version_id=$1`, newVersion.ID).Scan(&qualityCount); err != nil {
			t.Fatalf("count new-version QualityAssessments: %v", err)
		}
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM dataset_certification WHERE dataset_version_id=$1`, newVersion.ID).Scan(&certificationCount); err != nil {
			t.Fatalf("count new-version certifications: %v", err)
		}
		if qualityCount != 0 || certificationCount != 0 {
			t.Fatalf("new DatasetVersion inherited facts: quality=%d certification=%d, want 0/0", qualityCount, certificationCount)
		}

		history, err := certificationService.ListDatasetHistory(ctx, workspaceID, newVersion.ID, time.Now().UTC())
		if err != nil {
			t.Fatalf("read new-version certification history: %v", err)
		}
		if len(history) != 0 {
			t.Fatalf("new DatasetVersion certification history = %+v, want empty", history)
		}

		newEligibility, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
			WorkspaceID:      workspaceID,
			DatasetVersionID: newVersion.ID,
			ProfileID:        profile.ID,
			Consumer:         "LICENSED_BANK",
			Purpose:          purpose,
			Action:           "READ",
			Delivery:         "DIRECT_DATA",
			ScopeType:        "ALL_RESOURCE",
			ScopeRef:         newVersion.ID.String(),
			AsOf:             time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("check new-version delivery eligibility: %v", err)
		}
		if newEligibility.Allowed || !hasPilotBlocker(newEligibility.Blockers, "CERTIFICATION_NOT_CURRENT") {
			t.Fatalf("new-version eligibility = allowed=%v blockers=%+v, want CERTIFICATION_NOT_CURRENT", newEligibility.Allowed, newEligibility.Blockers)
		}
	})
}

func asPilotString(value any) string {
	text, _ := value.(string)
	return text
}

func activatePilotAuthorizationForConsumer(t *testing.T, ctx context.Context, service *rightsapp.Service, workspaceID uuid.UUID, code, consumer string, validFrom, validTo time.Time, grants []rightsdomain.ResourceGrantSpec, actorID *uuid.UUID, traceID string) rightsdomain.Authorization {
	t.Helper()
	normalizedGrants := append([]rightsdomain.ResourceGrantSpec(nil), grants...)
	for i := range normalizedGrants {
		if normalizedGrants[i].ScopeType == "" {
			normalizedGrants[i].ScopeType = "ALL_RESOURCE"
			normalizedGrants[i].ScopeRef = normalizedGrants[i].DataResourceID.String()
		}
	}
	authorization, err := service.Create(ctx, rightsapp.CreateAuthorizationCommand{
		WorkspaceID: workspaceID,
		Code:        code,
		GrantorRef:  "PARK-OPERATOR",
		GranteeRef:  consumer,
		Purpose:     purpose,
		ValidFrom:   &validFrom,
		ValidTo:     &validTo,
		Resources:   normalizedGrants,
		ActorID:     actorID,
		TraceID:     traceID,
	})
	if err != nil {
		t.Fatalf("create %s Authorization: %v", consumer, err)
	}
	authorization, err = service.Submit(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, ActorID: actorID, TraceID: traceID})
	if err != nil {
		t.Fatalf("submit %s Authorization: %v", consumer, err)
	}
	authorization, err = service.Approve(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, ActorID: actorID, TraceID: traceID})
	if err != nil {
		t.Fatalf("approve %s Authorization: %v", consumer, err)
	}
	authorization, err = service.Activate(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, ActorID: actorID, TraceID: traceID, At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("activate %s Authorization: %v", consumer, err)
	}

	for _, grant := range authorization.Resources {
		permissions := make([]rightsdomain.RightsPermission, 0, len(grant.Actions))
		for _, action := range grant.Actions {
			permissions = append(permissions, rightsdomain.RightsPermission{
				Kind:    rightsdomain.PermissionGrant,
				Action:  action,
				Purpose: purpose,
				Scope:   rightsdomain.NormalizedScope{Type: grant.ScopeType, Ref: grant.ScopeRef},
			})
		}
		declaration, err := service.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{
			Spec: rightsdomain.RightsDeclarationSpec{
				WorkspaceID:       workspaceID,
				DataResourceID:    grant.DataResourceID,
				ClaimantRef:       authorization.GrantorRef,
				BasisType:         "LICENSE",
				BasisRef:          "certified-dataset-pilot-linearization",
				ConsumerScopeType: "ANY",
				Parties: []rightsdomain.RightsParty{
					{PartyRef: authorization.GrantorRef, Role: "RIGHTS_HOLDER"},
				},
				Permissions: permissions,
				ActorID:     actorID,
			},
			TraceID: traceID,
		})
		if err != nil {
			t.Fatalf("create %s rights declaration: %v", consumer, err)
		}
		if _, err := service.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{
			DeclarationID: declaration.ID,
			Outcome:       rightsdomain.DeclarationVerified,
			ActorID:       actorID,
			TraceID:       traceID,
		}); err != nil {
			t.Fatalf("verify %s rights declaration: %v", consumer, err)
		}
		if _, err := service.BindAuthorizationProvenance(ctx, rightsapp.BindAuthorizationProvenanceCommand{
			WorkspaceID:     workspaceID,
			AuthorizationID: authorization.ID,
			DataResourceID:  grant.DataResourceID,
			DeclarationID:   declaration.ID,
			GrantorRef:      authorization.GrantorRef,
			AuthorityMode:   rightsdomain.AuthorityDirect,
			AsOf:            time.Now().UTC(),
			ActorID:         actorID,
			TraceID:         traceID,
		}); err != nil {
			t.Fatalf("bind %s authorization provenance: %v", consumer, err)
		}
	}
	return authorization
}

func hasPilotBlocker(blockers []certificationdomain.Blocker, code string) bool {
	for _, blocker := range blockers {
		if blocker.Code == code {
			return true
		}
	}
	return false
}

func hasString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
