package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	deliveryhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/transport/http"
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
