package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
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
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	parkindicator "github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
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
	certification, err := certificationService.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: outputVersion.ID,
		ProfileID:        profile.ID,
		IdempotencyKey:   "pilot-certification-" + suffix,
		ActorID:          &actorID,
		TraceID:          traceID,
	})
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

	eligibility := certificationapp.NewEligibilityService(certificationService, datasetRepo, rightsRepo)
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
}
