package acceptance_test

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	complianceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	compliancedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	workflownative "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	workflowqueue "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

const (
	qualityRuleSetRef = "park/quality/enterprise-activity-quality-v1.yaml"
	complianceRef     = "park/compliance/enterprise-activity-compliance-v1.yaml"
	companyPolicyRef  = "park/matching/company-match-policy-v1.yaml"
	indicatorSetRef   = "park-enterprise-activity@1.0.0"
	purpose           = "ENTERPRISE_CREDIT_RISK_SUPPORT"
)

func TestEnterpriseActivityCorePOCFullPath(t *testing.T) {
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
	traceID := "core-poc-" + uuid.NewString()
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
	qualityService := qualityapp.NewService(industryPackRoot, txManager, datasetRepo, qualityRepo, store)
	complianceRepo := complianceinfra.NewPostgresRepository(pool)
	complianceService := complianceapp.NewService(industryPackRoot, txManager, datasetRepo, complianceRepo, store)
	contractRepo := contractinfra.NewPostgresRepository(pool)
	contractService := contractapp.NewService(txManager, contractRepo)
	rightsRepo := rightsinfra.NewPostgresRepository(pool)
	rightsService := rightsapp.NewService(txManager, rightsRepo)
	productRepo := productinfra.NewPostgresRepository(pool)
	productService := productapp.NewService(txManager, productRepo)

	enterpriseResource := mustCreateResource(t, ctx, resourceService, workspaceID, "ENTERPRISE", "Enterprise master data", &actorID, traceID)
	leaseResource := mustCreateResource(t, ctx, resourceService, workspaceID, "LEASE", "Enterprise lease data", &actorID, traceID)
	energyResource := mustCreateResource(t, ctx, resourceService, workspaceID, "ENERGY", "Enterprise energy data", &actorID, traceID)

	enterpriseDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "ENTERPRISE-RAW", "Enterprise RAW", datasetdomain.DatasetTypeRaw, &enterpriseResource.ID, &actorID, traceID)
	leaseDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "LEASE-RAW", "Lease RAW", datasetdomain.DatasetTypeRaw, &leaseResource.ID, &actorID, traceID)
	energyDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "ENERGY-RAW", "Energy RAW", datasetdomain.DatasetTypeRaw, &energyResource.ID, &actorID, traceID)
	standardizedDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "ENTERPRISE-STANDARDIZED", "Enterprise standardized", datasetdomain.DatasetTypeStandardized, nil, &actorID, traceID)
	activityDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "ENTERPRISE-ACTIVITY", "Enterprise activity", datasetdomain.DatasetTypeCurated, nil, &actorID, traceID)

	enterpriseName := "enterprise-" + suffix + ".csv"
	leaseName := "lease-" + suffix + ".csv"
	energyName := "energy-" + suffix + ".csv"
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
		t.Fatalf("start Company Entity Resolution: %v", err)
	}
	manualReviews := 0
	if matchJob.Status == entitydomain.JobWaitingReview {
		candidates, err := entityRepo.ListCandidates(ctx, matchJob.ID)
		if err != nil {
			t.Fatalf("list entity review candidates: %v", err)
		}
		for _, candidate := range candidates {
			if candidate.Status != entitydomain.CandidatePending {
				continue
			}
			manualReviews++
			matchJob, err = entityService.Confirm(ctx, entityapp.ReviewCommand{
				CandidateID: candidate.ID,
				ReviewerID:  reviewerID,
				Reason:      "confirmed against reference fixture during Core POC acceptance",
				TraceID:     traceID,
			})
			if err != nil {
				t.Fatalf("confirm entity review candidate: %v", err)
			}
		}
	}
	if manualReviews == 0 {
		t.Fatal("reference Entity Resolution did not exercise a manual review")
	}
	if matchJob.Status != entitydomain.JobSucceeded || matchJob.OutputDatasetVersionID == nil {
		t.Fatalf("Entity Resolution status/output = %s/%v, want SUCCEEDED with STANDARDIZED DatasetVersion", matchJob.Status, matchJob.OutputDatasetVersionID)
	}
	standardizedVersionID := *matchJob.OutputDatasetVersionID

	aliasMapping, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", enterpriseName, "ENT-005")
	if err != nil {
		t.Fatalf("query reviewed EntityMapping: %v", err)
	}
	if aliasMapping.Status != entitydomain.MappingConfirmed || aliasMapping.EvidenceID == nil || aliasMapping.MatchRuleID == "" {
		t.Fatalf("reviewed EntityMapping is not explainable: %+v", aliasMapping)
	}
	matchEvidence, err := evidence.NewQueryRepository(pool).ListForObject(ctx, "ENTITY_MATCH_JOB", matchJob.ID)
	if err != nil {
		t.Fatalf("query Entity Resolution evidence: %v", err)
	}
	if len(matchEvidence) == 0 {
		t.Fatal("Entity Resolution produced no Evidence")
	}
	for _, item := range matchEvidence {
		if !item.IntegrityValid {
			t.Fatalf("Entity Resolution Evidence %s failed SHA-256 verification", item.ID)
		}
	}

	workflowDefinition := readRepoFile(t, "examples", "enterprise-activity", "workflow", "workflow-v1.yaml")
	workflowVersion, err := workflowVersionService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           "enterprise-activity-" + suffix,
		Name:           "Enterprise Activity Production",
		Version:        "1.0.0",
		DefinitionRef:  "examples/enterprise-activity/workflow/workflow-v1.yaml",
		DefinitionYAML: workflowDefinition,
		ActorID:        &actorID,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("create WorkflowVersion: %v", err)
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
		IdempotencyKey: "enterprise-activity-create-" + suffix,
		ActorID:        &actorID,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("create Workflow Execution: %v", err)
	}
	engine := workflownative.NewEngine(industryPackRoot, txManager, datasetRepo, entityRepo, workflowRepo, uploadDataset, store, parkindicator.NewCalculator())
	queueHandler := workflowqueue.NewHandler(executionService, workflowRepo, engine)
	payload, _ := json.Marshal(map[string]any{"executionId": execution.ID})
	if err := queueHandler.Handle(ctx, asynq.NewTask(workflowqueue.TaskExecute, payload)); err != nil {
		t.Fatalf("execute native workflow: %v", err)
	}
	storedExecution, err := workflowRepo.GetExecution(ctx, execution.ID)
	if err != nil {
		t.Fatalf("query completed Execution: %v", err)
	}
	if storedExecution.Status != workflowdomain.ExecutionSucceeded || storedExecution.OutputDatasetVersionID == nil {
		t.Fatalf("Execution status/output = %s/%v, want SUCCEEDED with output", storedExecution.Status, storedExecution.OutputDatasetVersionID)
	}
	outputVersion, err := datasetRepo.GetVersion(ctx, *storedExecution.OutputDatasetVersionID)
	if err != nil {
		t.Fatalf("query CURATED DatasetVersion: %v", err)
	}
	if outputVersion.GeneratedByExecutionID == nil || *outputVersion.GeneratedByExecutionID != execution.ID {
		t.Fatalf("CURATED DatasetVersion generatedByExecutionId = %v, want %s", outputVersion.GeneratedByExecutionID, execution.ID)
	}
	outputCSV := string(store.bytes(outputVersion.StorageURI))
	if strings.Contains(outputCSV, "company_name") || strings.Contains(outputCSV, "深圳星云科技有限公司") {
		t.Fatalf("CURATED V1 output leaked non-contract company name data: %s", outputCSV)
	}
	if !strings.Contains(outputCSV, ",96.01,HIGH,100.00,") {
		t.Fatalf("CURATED output does not contain deterministic reference score: %s", outputCSV)
	}

	qualityResult, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		RuleSetRef:       qualityRuleSetRef,
		ActorID:          &actorID,
		TraceID:          traceID,
		Now:              outputVersion.ReadyAt.Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("run Quality Gate: %v", err)
	}
	if qualityResult.GateDecision != qualitydomain.GatePass {
		t.Fatalf("Quality Gate = %s, want PASS; findings=%+v", qualityResult.GateDecision, qualityResult.Findings)
	}
	complianceResult, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		PolicyRef:        complianceRef,
		ActorID:          &actorID,
		TraceID:          traceID,
	})
	if err != nil {
		t.Fatalf("run Compliance Gate: %v", err)
	}
	if complianceResult.GateDecision != compliancedomain.GatePass {
		t.Fatalf("Compliance Gate = %s, want PASS; findings=%+v", complianceResult.GateDecision, complianceResult.Findings)
	}

	contractVersion, err := contractService.CreateVersionFromYAML(ctx, contractapp.CreateVersionFromYAMLCommand{
		WorkspaceID:  workspaceID,
		SourceRef:    "examples/enterprise-activity/contract/data-contract-v1.yaml",
		DocumentYAML: readRepoFile(t, "examples", "enterprise-activity", "contract", "data-contract-v1.yaml"),
		ActorID:      &actorID,
		TraceID:      traceID,
	})
	if err != nil {
		t.Fatalf("create Data Contract: %v", err)
	}
	contractVersion, err = contractService.PublishVersion(ctx, contractapp.PublishVersionCommand{VersionID: contractVersion.ID, ActorID: &actorID, TraceID: traceID})
	if err != nil {
		t.Fatalf("publish Data Contract: %v", err)
	}

	validFrom := time.Now().UTC().Add(-time.Hour)
	validTo := time.Now().UTC().Add(24 * time.Hour)
	grants := []rightsdomain.ResourceGrantSpec{
		{DataResourceID: enterpriseResource.ID, Actions: []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}, ScopeType: "ALL_RESOURCE", ScopeRef: enterpriseResource.ID.String(), Scope: map[string]any{"useCase": purpose}},
		{DataResourceID: leaseResource.ID, Actions: []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}, ScopeType: "ALL_RESOURCE", ScopeRef: leaseResource.ID.String(), Scope: map[string]any{"useCase": purpose}},
		{DataResourceID: energyResource.ID, Actions: []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}, ScopeType: "ALL_RESOURCE", ScopeRef: energyResource.ID.String(), Scope: map[string]any{"useCase": purpose}},
	}
	authorization := activateAuthorization(t, ctx, rightsService, workspaceID, "AUTH-CORE-"+suffix, validFrom, validTo, grants, &actorID, traceID)

	product, err := productService.CreateProduct(ctx, productapp.CreateProductCommand{
		WorkspaceID: workspaceID,
		Code:        "DP-ENTERPRISE-ACTIVITY",
		Name:        "企业经营活跃度",
		Description: "Core POC reference Data Product",
		DomainCode:  "PARK_ENTERPRISE_ACTIVITY",
		ActorID:     &actorID,
		TraceID:     traceID,
	})
	if err != nil {
		t.Fatalf("create DataProduct: %v", err)
	}
	productVersion, err := productService.CreateVersion(ctx, productapp.CreateVersionCommand{
		ProductID:         product.ID,
		MajorVersion:      1,
		MinorVersion:      0,
		PatchVersion:      0,
		WorkflowVersionID: &workflowVersion.ID,
		ContractVersionID: &contractVersion.ID,
		EntityPolicyRef:   companyPolicyRef,
		IndicatorSetRef:   indicatorSetRef,
		Definition: map[string]any{
			"referenceImplementation": "enterprise-activity",
			"workflowVersion":         workflowVersion.Version,
		},
		Assets: []productdomain.AssetSpec{{
			AssetType:      productdomain.AssetDataset,
			Name:           "enterprise_activity_curated",
			DatasetID:      &activityDataset.ID,
			DeliveryConfig: map[string]any{"mode": "DATASET", "rawExport": false},
		}},
		ActorID: &actorID,
		TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("create ProductVersion: %v", err)
	}
	release, err := productService.CreateRelease(ctx, productapp.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: productVersion.ID,
		ReleaseNo:        "R-CORE-" + suffix,
		Datasets:         []productdomain.ReleaseDataset{{DatasetVersionID: outputVersion.ID, Role: productdomain.DatasetPrimary}},
		ReleaseNotes:     "Core POC deterministic acceptance release",
		ActorID:          &actorID,
		TraceID:          traceID,
	})
	if err != nil {
		t.Fatalf("create ProductRelease: %v", err)
	}
	rightsSnapshot, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID:      workspaceID,
		ProductReleaseID: &release.ID,
		Purpose:          purpose,
		ConsumerRef:      "LICENSED_BANK",
		AsOf:             time.Now().UTC(),
		AuthorizationIDs: []uuid.UUID{authorization.ID},
		ActorID:          &actorID,
		TraceID:          traceID,
	})
	if err != nil {
		t.Fatalf("create RightsSnapshot: %v", err)
	}
	readiness, err := productService.ValidateRelease(ctx, productapp.ValidateReleaseCommand{
		ReleaseID:          release.ID,
		ContractVersionID:  contractVersion.ID,
		RightsSnapshotID:   rightsSnapshot.ID,
		QualityResultID:    qualityResult.ID,
		ComplianceResultID: complianceResult.ID,
		ActorID:            &actorID,
		TraceID:            traceID,
	})
	if err != nil {
		t.Fatalf("validate ProductRelease: %v", err)
	}
	if readiness.Overall != "READY" || len(readiness.Blockers) != 0 {
		t.Fatalf("ReleaseReadiness = %s blockers=%v details=%v, want READY", readiness.Overall, readiness.Blockers, readiness.Details)
	}

	// AC7: an output that claims a production Execution with the explicit
	// resolution input but has no committed T3/B2 preparation must not pass the
	// production gate merely because the execution row exists.
	incompleteExecution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflowVersion.ID, OutputDatasetID: activityDataset.ID,
		TargetPeriod: "2025-03", Inputs: execution.Inputs,
		IdempotencyKey: "enterprise-activity-incomplete-binding-" + suffix, ActorID: &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("create incomplete-binding execution: %v", err)
	}
	incompleteOutput := mustUploadCSV(t, ctx, uploadDataset, activityDataset.ID, "incomplete-binding-"+suffix+".csv", []byte(outputCSV), map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0}, &incompleteExecution.ID, &actorID, traceID)
	incompleteQuality, err := qualityService.Run(ctx, qualityapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: incompleteOutput.ID, RuleSetRef: qualityRuleSetRef,
		ActorID: &actorID, TraceID: traceID, Now: incompleteOutput.ReadyAt.Add(30 * time.Minute),
	})
	if err != nil || incompleteQuality.GateDecision != qualitydomain.GatePass {
		t.Fatalf("incomplete-binding Quality Gate = %s err=%v, want PASS", incompleteQuality.GateDecision, err)
	}
	incompleteCompliance, err := complianceService.Run(ctx, complianceapp.RunCommand{
		WorkspaceID: workspaceID, DatasetVersionID: incompleteOutput.ID, PolicyRef: complianceRef,
		ActorID: &actorID, TraceID: traceID,
	})
	if err != nil || incompleteCompliance.GateDecision != compliancedomain.GatePass {
		t.Fatalf("incomplete-binding Compliance Gate = %s err=%v, want PASS", incompleteCompliance.GateDecision, err)
	}
	incompleteRelease, err := productService.CreateRelease(ctx, productapp.CreateReleaseCommand{
		ProductID: product.ID, ProductVersionID: productVersion.ID, ReleaseNo: "R-INCOMPLETE-BINDING-" + suffix,
		Datasets: []productdomain.ReleaseDataset{{DatasetVersionID: incompleteOutput.ID, Role: productdomain.DatasetPrimary}},
		ActorID:  &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("create incomplete-binding release: %v", err)
	}
	incompleteRights, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID: workspaceID, ProductReleaseID: &incompleteRelease.ID, Purpose: purpose,
		ConsumerRef: "LICENSED_BANK", AsOf: time.Now().UTC(), AuthorizationIDs: []uuid.UUID{authorization.ID},
		ActorID: &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("create incomplete-binding RightsSnapshot: %v", err)
	}
	incompleteReadiness, err := productService.ValidateRelease(ctx, productapp.ValidateReleaseCommand{
		ReleaseID: incompleteRelease.ID, ContractVersionID: contractVersion.ID, RightsSnapshotID: incompleteRights.ID,
		QualityResultID: incompleteQuality.ID, ComplianceResultID: incompleteCompliance.ID, ActorID: &actorID, TraceID: traceID,
	})
	if err != nil {
		t.Fatalf("validate incomplete-binding release: %v", err)
	}
	if incompleteReadiness.Overall != "NOT_READY" || !slices.Contains(incompleteReadiness.Blockers, "PRODUCTION_DEPENDENCY_BINDING_INCOMPLETE") {
		t.Fatalf("incomplete-binding readiness = %s blockers=%v, want dependency-binding blocker", incompleteReadiness.Overall, incompleteReadiness.Blockers)
	}

	idempotencyKey := "publish-" + suffix
	published, err := productService.PublishRelease(ctx, productapp.PublishReleaseCommand{
		ReleaseID:      release.ID,
		IdempotencyKey: idempotencyKey,
		ActorID:        &actorID,
		TraceID:        traceID,
	})
	if err != nil {
		t.Fatalf("publish ProductRelease: %v", err)
	}
	secondPublish, err := productService.PublishRelease(ctx, productapp.PublishReleaseCommand{
		ReleaseID:      release.ID,
		IdempotencyKey: idempotencyKey,
		ActorID:        &actorID,
		TraceID:        traceID + "-retry",
	})
	if err != nil {
		t.Fatalf("retry idempotent ProductRelease publish: %v", err)
	}
	if published.Status != productdomain.ReleasePublished || published.EvidenceSnapshotID == nil || secondPublish.EvidenceSnapshotID == nil || *published.EvidenceSnapshotID != *secondPublish.EvidenceSnapshotID {
		t.Fatalf("idempotent publish changed release/snapshot: first=%+v second=%+v", published, secondPublish)
	}

	var releasedEventCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox_event
		WHERE aggregate_type='PRODUCT_RELEASE' AND aggregate_id=$1 AND event_type='ProductReleased'
	`, release.ID).Scan(&releasedEventCount); err != nil {
		t.Fatalf("count ProductReleased events: %v", err)
	}
	if releasedEventCount != 1 {
		t.Fatalf("ProductReleased event count = %d, want 1 after idempotent retry", releasedEventCount)
	}

	releaseTrace, err := traceability.NewRepository(pool).ProductRelease(ctx, release.ID)
	if err != nil {
		t.Fatalf("query ProductRelease traceability: %v", err)
	}
	if releaseTrace.EvidenceSnapshot == nil || !releaseTrace.EvidenceSnapshot.IntegrityValid || releaseTrace.EvidenceSnapshot.ID != *published.EvidenceSnapshotID {
		t.Fatalf("release EvidenceSnapshot trace is invalid: %+v", releaseTrace.EvidenceSnapshot)
	}
	if !containsDatasetVersion(releaseTrace.DatasetVersions, outputVersion.ID) || !containsDatasetVersion(releaseTrace.DatasetVersions, enterpriseVersion.ID) || !containsDatasetVersion(releaseTrace.DatasetVersions, leaseVersion.ID) || !containsDatasetVersion(releaseTrace.DatasetVersions, energyVersion.ID) {
		t.Fatalf("release DatasetVersion lineage is incomplete: %+v", releaseTrace.DatasetVersions)
	}
	if len(releaseTrace.Executions) != 1 || releaseTrace.Executions[0].ID != execution.ID || releaseTrace.Executions[0].WorkflowVersionID != workflowVersion.ID {
		t.Fatalf("release execution/workflow trace is incomplete: %+v", releaseTrace.Executions)
	}
	if len(releaseTrace.CostEvents) == 0 {
		t.Fatal("release trace contains no CostEvents")
	}
	if len(releaseTrace.Evidence) < 3 {
		t.Fatalf("release trace Evidence count = %d, want processing + Quality + Compliance Evidence", len(releaseTrace.Evidence))
	}
	for _, item := range releaseTrace.Evidence {
		if !item.IntegrityValid {
			t.Fatalf("release Evidence %s failed integrity verification", item.ID)
		}
	}
	// AC2: an alias decision used by production remains the trace decision even
	// after the mutable current mapping is corrected after publication.
	var leaseSourceRef string
	var leaseUsedDecisionID, leaseUsedEntityID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT source_ref, decision_id, entity_id
		FROM execution_mapping_usage
		WHERE execution_id=$1 AND input_name='lease_raw' AND source_key='LEASE-C001'
	`, execution.ID).Scan(&leaseSourceRef, &leaseUsedDecisionID, &leaseUsedEntityID); err != nil {
		t.Fatalf("read production lease decision usage: %v", err)
	}
	leaseMapping, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", leaseSourceRef, "LEASE-C001")
	if err != nil || leaseMapping.CurrentDecisionID == nil {
		t.Fatalf("read current lease mapping: %v / %+v", err, leaseMapping)
	}
	leaseAlternate, err := entitydomain.NewEntity(workspaceID, matchJob.EntityTypeID, "POST-RELEASE-LEASE", "Post-release lease correction", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("create post-release lease entity: %v", err)
	}
	if err := txManager.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := entityRepo.InsertEntity(ctx, tx, leaseAlternate); err != nil {
			return err
		}
		corrected := leaseMapping
		corrected.EntityID = leaseAlternate.ID
		corrected.Status = entitydomain.MappingConfirmed
		_, err := entityRepo.RecordMappingDecision(ctx, tx, entitydomain.MappingDecisionCommand{
			Mapping: corrected, SourceOrigin: entitydomain.OriginWorkflowAlias,
			IdempotencyKey:        "post-release-lease-correction-" + suffix,
			ExpectCurrentDecision: true, ExpectedCurrentDecisionID: leaseMapping.CurrentDecisionID,
		})
		return err
	}); err != nil {
		t.Fatalf("record post-release lease correction: %v", err)
	}
	releaseTraceAfterCorrection, err := traceability.NewRepository(pool).ProductRelease(ctx, release.ID)
	if err != nil {
		t.Fatalf("query trace after post-release correction: %v", err)
	}
	var leaseTraceFound bool
	for _, mapping := range releaseTraceAfterCorrection.EntityMappings {
		if mapping.DecisionID == leaseUsedDecisionID {
			leaseTraceFound = true
			if mapping.EntityID != leaseUsedEntityID {
				t.Fatalf("release trace changed production lease entity from %s to %s", leaseUsedEntityID, mapping.EntityID)
			}
		}
	}
	if !leaseTraceFound {
		t.Fatalf("release trace lost alias decision %s after current mapping correction", leaseUsedDecisionID)
	}

	// The STANDARDIZED DatasetVersion is deliberately checked through the entity-resolution
	// boundary as well as the release lineage. It proves the exact policy/review output used
	// to establish the canonical mappings consumed by the workflow.
	resolvedOutput, found, err := entityRepo.FindSucceededOutputVersionForInput(ctx, enterpriseVersion.ID, "CSV", enterpriseName)
	if err != nil {
		t.Fatalf("resolve Entity Resolution provenance: %v", err)
	}
	if !found || resolvedOutput != standardizedVersionID {
		t.Fatalf("Entity Resolution provenance output = %s found=%v, want %s", resolvedOutput, found, standardizedVersionID)
	}

	t.Run("critical quality failure blocks readiness", func(t *testing.T) {
		// These blocker fixtures are manually uploaded DatasetVersions, not outputs of
		// the production Execution above. They deliberately carry no
		// generatedByExecutionId: C2-a makes "one Execution produces at most one
		// DatasetVersion of a Dataset" a database constraint, so claiming execution.ID
		// here would be both false provenance and a violation of
		// uq_dataset_version_execution_output.
		badVersion := mustUploadCSV(t, ctx, uploadDataset, activityDataset.ID, "bad-quality-"+suffix+".csv", []byte("company_id,company_name,period,tenancy_stability,rent_performance,energy_stability,activity_score,activity_level,indicator_coverage,generated_at\nCOMPANY-BAD,异常科技有限公司,2025-03,90,95,80,120,HIGH,100,2025-03-31T00:00:00Z\n"), map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0}, nil, &actorID, traceID)
		badQuality, err := qualityService.Run(ctx, qualityapp.RunCommand{WorkspaceID: workspaceID, DatasetVersionID: badVersion.ID, RuleSetRef: qualityRuleSetRef, ActorID: &actorID, TraceID: traceID, Now: badVersion.ReadyAt.Add(30 * time.Minute)})
		if err != nil {
			t.Fatalf("run blocking Quality Gate: %v", err)
		}
		if badQuality.GateDecision != qualitydomain.GateFail {
			t.Fatalf("blocking Quality Gate = %s, want FAIL", badQuality.GateDecision)
		}
		passCompliance, err := complianceService.Run(ctx, complianceapp.RunCommand{WorkspaceID: workspaceID, DatasetVersionID: badVersion.ID, PolicyRef: complianceRef, ActorID: &actorID, TraceID: traceID})
		if err != nil || passCompliance.GateDecision != compliancedomain.GatePass {
			t.Fatalf("control Compliance Gate = %s err=%v, want PASS", passCompliance.GateDecision, err)
		}
		blocked := validateSecondaryRelease(t, ctx, productService, rightsService, product, productVersion, contractVersion.ID, authorization.ID, badVersion.ID, badQuality.ID, passCompliance.ID, "R-BAD-QUALITY-"+suffix, workspaceID, &actorID, traceID)
		if blocked.Overall != "NOT_READY" || !slices.Contains(blocked.Blockers, "QUALITY_GATE_BLOCKING") {
			t.Fatalf("quality-blocked readiness = %s blockers=%v", blocked.Overall, blocked.Blockers)
		}
	})

	t.Run("compliance block finding blocks readiness", func(t *testing.T) {
		badVersion := mustUploadCSV(t, ctx, uploadDataset, activityDataset.ID, "bad-compliance-"+suffix+".csv", []byte("company_id,company_name,period,tenancy_stability,rent_performance,energy_stability,activity_score,activity_level,indicator_coverage,generated_at,mobile\nCOMPANY-PII,敏感科技有限公司,2025-03,90,95,80,88,HIGH,100,2025-03-31T00:00:00Z,13800000000\n"), map[string]any{"unresolvedEntityRate": 0.0, "acceptedNegativeEnergyRate": 0.0}, nil, &actorID, traceID)
		passQuality, err := qualityService.Run(ctx, qualityapp.RunCommand{WorkspaceID: workspaceID, DatasetVersionID: badVersion.ID, RuleSetRef: qualityRuleSetRef, ActorID: &actorID, TraceID: traceID, Now: badVersion.ReadyAt.Add(30 * time.Minute)})
		if err != nil || passQuality.GateDecision != qualitydomain.GatePass {
			t.Fatalf("control Quality Gate = %s err=%v, want PASS", passQuality.GateDecision, err)
		}
		badCompliance, err := complianceService.Run(ctx, complianceapp.RunCommand{WorkspaceID: workspaceID, DatasetVersionID: badVersion.ID, PolicyRef: complianceRef, ActorID: &actorID, TraceID: traceID})
		if err != nil {
			t.Fatalf("run blocking Compliance Gate: %v", err)
		}
		if badCompliance.GateDecision != compliancedomain.GateFail {
			t.Fatalf("blocking Compliance Gate = %s, want FAIL", badCompliance.GateDecision)
		}
		blocked := validateSecondaryRelease(t, ctx, productService, rightsService, product, productVersion, contractVersion.ID, authorization.ID, badVersion.ID, passQuality.ID, badCompliance.ID, "R-BAD-COMPLIANCE-"+suffix, workspaceID, &actorID, traceID)
		if blocked.Overall != "NOT_READY" || !slices.Contains(blocked.Blockers, "COMPLIANCE_GATE_BLOCKING") {
			t.Fatalf("compliance-blocked readiness = %s blockers=%v", blocked.Overall, blocked.Blockers)
		}
	})

	t.Run("expired authorization blocks readiness", func(t *testing.T) {
		expiresAt := time.Now().UTC().Add(time.Minute)
		expiring := activateAuthorization(t, ctx, rightsService, workspaceID, "AUTH-EXPIRING-"+suffix, time.Now().UTC().Add(-time.Hour), expiresAt, grants, &actorID, traceID)
		release, err := productService.CreateRelease(ctx, productapp.CreateReleaseCommand{
			ProductID:        product.ID,
			ProductVersionID: productVersion.ID,
			ReleaseNo:        "R-EXPIRED-RIGHTS-" + suffix,
			Datasets:         []productdomain.ReleaseDataset{{DatasetVersionID: outputVersion.ID, Role: productdomain.DatasetPrimary}},
			ActorID:          &actorID,
			TraceID:          traceID,
		})
		if err != nil {
			t.Fatalf("create expired-rights release: %v", err)
		}
		snapshot, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{WorkspaceID: workspaceID, ProductReleaseID: &release.ID, Purpose: purpose, ConsumerRef: "LICENSED_BANK", AsOf: time.Now().UTC(), AuthorizationIDs: []uuid.UUID{expiring.ID}, ActorID: &actorID, TraceID: traceID})
		if err != nil {
			t.Fatalf("create expiring RightsSnapshot: %v", err)
		}
		if _, err := rightsService.Expire(ctx, expiring.ID, expiresAt, traceID); err != nil {
			t.Fatalf("expire Authorization: %v", err)
		}
		blocked, err := productService.ValidateRelease(ctx, productapp.ValidateReleaseCommand{ReleaseID: release.ID, ContractVersionID: contractVersion.ID, RightsSnapshotID: snapshot.ID, QualityResultID: qualityResult.ID, ComplianceResultID: complianceResult.ID, ActorID: &actorID, TraceID: traceID})
		if err != nil {
			t.Fatalf("validate expired-rights release: %v", err)
		}
		if blocked.Overall != "NOT_READY" || !slices.Contains(blocked.Blockers, "RIGHTS_INVALID") {
			t.Fatalf("expired-rights readiness = %s blockers=%v", blocked.Overall, blocked.Blockers)
		}
	})

	t.Run("historical release artifacts are immutable", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `UPDATE dataset_version SET checksum_value='tampered' WHERE id=$1`, outputVersion.ID); err == nil {
			t.Fatal("READY DatasetVersion mutation unexpectedly succeeded")
		}
		if _, err := pool.Exec(ctx, `UPDATE product_release SET release_notes='tampered' WHERE id=$1`, published.ID); err == nil {
			t.Fatal("PUBLISHED ProductRelease mutation unexpectedly succeeded")
		}
	})
}

func activateAuthorization(t *testing.T, ctx context.Context, service *rightsapp.Service, workspaceID uuid.UUID, code string, validFrom, validTo time.Time, grants []rightsdomain.ResourceGrantSpec, actorID *uuid.UUID, traceID string) rightsdomain.Authorization {
	t.Helper()
	for i := range grants {
		if grants[i].ScopeType == "" {
			grants[i].ScopeType = "ALL_RESOURCE"
			grants[i].ScopeRef = grants[i].DataResourceID.String()
		}
	}
	authorization, err := service.Create(ctx, rightsapp.CreateAuthorizationCommand{
		WorkspaceID: workspaceID,
		Code:        code,
		GrantorRef:  "PARK-OPERATOR",
		GranteeRef:  "LICENSED_BANK",
		Purpose:     purpose,
		ValidFrom:   &validFrom,
		ValidTo:     &validTo,
		Resources:   grants,
		ActorID:     actorID,
		TraceID:     traceID,
	})
	if err != nil {
		t.Fatalf("create Authorization: %v", err)
	}
	authorization, err = service.Submit(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, ActorID: actorID, TraceID: traceID})
	if err != nil {
		t.Fatalf("submit Authorization: %v", err)
	}
	authorization, err = service.Approve(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, ActorID: actorID, TraceID: traceID})
	if err != nil {
		t.Fatalf("approve Authorization: %v", err)
	}
	authorization, err = service.Activate(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, ActorID: actorID, TraceID: traceID, At: time.Now().UTC()})
	if err != nil {
		t.Fatalf("activate Authorization: %v", err)
	}
	for _, grant := range authorization.Resources {
		permissions := make([]rightsdomain.RightsPermission, 0, len(grant.Actions))
		for _, action := range grant.Actions {
			permissions = append(permissions, rightsdomain.RightsPermission{
				Kind: rightsdomain.PermissionGrant, Action: action, Purpose: purpose,
				Scope: rightsdomain.NormalizedScope{Type: grant.ScopeType, Ref: grant.ScopeRef},
			})
		}
		declaration, err := service.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{Spec: rightsdomain.RightsDeclarationSpec{
			WorkspaceID: workspaceID, DataResourceID: grant.DataResourceID, ClaimantRef: authorization.GrantorRef,
			BasisType: "LICENSE", BasisRef: "enterprise-activity-acceptance", Parties: []rightsdomain.RightsParty{{PartyRef: authorization.GrantorRef, Role: "RIGHTS_HOLDER"}}, Permissions: permissions, ActorID: actorID,
		}, TraceID: traceID})
		if err != nil {
			t.Fatalf("create rights declaration: %v", err)
		}
		if _, err := service.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{DeclarationID: declaration.ID, Outcome: rightsdomain.DeclarationVerified, ActorID: actorID, TraceID: traceID}); err != nil {
			t.Fatalf("verify rights declaration: %v", err)
		}
		if _, err := service.BindAuthorizationProvenance(ctx, rightsapp.BindAuthorizationProvenanceCommand{
			WorkspaceID: workspaceID, AuthorizationID: authorization.ID, DataResourceID: grant.DataResourceID, DeclarationID: declaration.ID,
			GrantorRef: authorization.GrantorRef, AuthorityMode: rightsdomain.AuthorityDirect, AsOf: time.Now().UTC(), ActorID: actorID, TraceID: traceID,
		}); err != nil {
			t.Fatalf("bind authorization provenance: %v", err)
		}
	}
	return authorization
}

func validateSecondaryRelease(t *testing.T, ctx context.Context, productService *productapp.Service, rightsService *rightsapp.Service, product productdomain.DataProduct, version productdomain.ProductVersion, contractVersionID, authorizationID, datasetVersionID, qualityResultID, complianceResultID uuid.UUID, releaseNo string, workspaceID uuid.UUID, actorID *uuid.UUID, traceID string) productapp.ReadinessResult {
	t.Helper()
	release, err := productService.CreateRelease(ctx, productapp.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: version.ID,
		ReleaseNo:        releaseNo,
		Datasets:         []productdomain.ReleaseDataset{{DatasetVersionID: datasetVersionID, Role: productdomain.DatasetPrimary}},
		ActorID:          actorID,
		TraceID:          traceID,
	})
	if err != nil {
		t.Fatalf("create secondary ProductRelease: %v", err)
	}
	snapshot, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID:      workspaceID,
		ProductReleaseID: &release.ID,
		Purpose:          purpose,
		ConsumerRef:      "LICENSED_BANK",
		AsOf:             time.Now().UTC(),
		AuthorizationIDs: []uuid.UUID{authorizationID},
		ActorID:          actorID,
		TraceID:          traceID,
	})
	if err != nil {
		t.Fatalf("create secondary RightsSnapshot: %v", err)
	}
	result, err := productService.ValidateRelease(ctx, productapp.ValidateReleaseCommand{
		ReleaseID:          release.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   snapshot.ID,
		QualityResultID:    qualityResultID,
		ComplianceResultID: complianceResultID,
		ActorID:            actorID,
		TraceID:            traceID,
	})
	if err != nil {
		t.Fatalf("validate secondary ProductRelease: %v", err)
	}
	return result
}

func containsDatasetVersion(items []traceability.DatasetVersionTrace, id uuid.UUID) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
