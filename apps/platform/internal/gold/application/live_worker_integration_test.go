package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	deliverydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	deliveryinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	goldinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	readmodel "github.com/qq550723504/data-product-platform/apps/platform/internal/readmodel"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type liveGoldFixture struct {
	workspaceID            uuid.UUID
	inputResourceID        uuid.UUID
	inputDatasetVersionID  uuid.UUID
	inputCertificationID   uuid.UUID
	contributionResourceID uuid.UUID
	campaignID             uuid.UUID
	snapshotID             uuid.UUID
	outputDatasetID        uuid.UUID
}

type liveGoldSharedManifest struct {
	WorkspaceID            uuid.UUID `json:"workspaceId"`
	InputResourceID        uuid.UUID `json:"inputResourceId"`
	InputDatasetVersionID  uuid.UUID `json:"inputDatasetVersionId"`
	InputCertificationID   uuid.UUID `json:"inputCertificationId"`
	ContributionResourceID uuid.UUID `json:"contributionResourceId"`
	CampaignID             uuid.UUID `json:"campaignId"`
	SnapshotID             uuid.UUID `json:"snapshotId"`
}

type liveGoldUIManifest struct {
	WorkspaceID         uuid.UUID `json:"workspaceId"`
	GoldDatasetID       uuid.UUID `json:"goldDatasetId"`
	GoldVersionID       uuid.UUID `json:"goldVersionId"`
	GoldBindingID       uuid.UUID `json:"goldBindingId"`
	GoldSnapshotID      uuid.UUID `json:"goldSnapshotId"`
	GoldAssessmentID    uuid.UUID `json:"goldAssessmentId"`
	GoldCertificationID uuid.UUID `json:"goldCertificationId"`
	OutputChecksum      string    `json:"outputChecksum"`
	ExpectedDelivery    string    `json:"expectedDelivery"`
	ExpectedBlocker     string    `json:"expectedBlocker"`
}

func TestLiveGoldWorkerBuildAndFormalQuality(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	workerBinary := strings.TrimSpace(os.Getenv("LIVE_PLATFORM_WORKER"))
	endpoint := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ENDPOINT"))
	bucket := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_BUCKET"))
	accessKey := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_SECRET_KEY"))
	if dsn == "" || workerBinary == "" || endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		t.Skip("live PostgreSQL, worker, and object storage environment are required")
	}

	ctx := context.Background()
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("routing: %v", err)
	}
	outbox.ConfigureAppendObligation(router)

	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	store, err := storage.New(endpoint, accessKey, secretKey, bucket, false)
	if err != nil {
		t.Fatalf("create live object store: %v", err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("ensure live object bucket: %v", err)
	}

	worker, workerLog := startLiveGoldWorker(t, ctx, workerBinary)

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	annotationRepo := annotationinfra.NewRepository(pool)
	annotationService := annotationapp.NewService(txManager, annotationRepo, nil)
	fixture := loadLiveGoldSharedFixture(t, ctx, pool)

	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	workflowVersionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	workflowVersion, err := workflowVersionService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:   fixture.workspaceID,
		Code:          "GOLD-LIVE-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8],
		Name:          "Gold live builder",
		Version:       "1.0.0",
		DefinitionRef: "live/gold-builder.yaml",
		DefinitionYAML: []byte(
			"spec:\n" +
				"  processor: GOLD_DATASET_BUILDER_V1\n" +
				"  execution:\n" +
				"    requiresTargetPeriod: false\n",
		),
		TraceID: "live-gold-worker",
	})
	if err != nil {
		t.Fatalf("create Gold workflow version: %v", err)
	}

	datasetWriter := datasetapp.NewUploadVersionService(txManager, datasetRepo, store)
	goldRepo := goldinfra.NewPostgresRepository(pool)
	goldService := NewService(
		txManager,
		goldRepo,
		workflowRepo,
		annotationRepo,
		datasetRepo,
		datasetWriter,
		store,
	)
	waitLiveOutboxIdle(t, ctx, pool, worker, workerLog)

	buildKey := "live-gold-build-" + uuid.NewString()
	buildCommand := CreateBuildCommand{
		WorkspaceID:                      fixture.workspaceID,
		WorkflowVersionID:                workflowVersion.ID,
		OutputDatasetID:                  fixture.outputDatasetID,
		InputDatasetVersionID:            fixture.inputDatasetVersionID,
		InputCertificationID:             fixture.inputCertificationID,
		AnnotationCampaignID:             fixture.campaignID,
		AnnotationSnapshotID:             fixture.snapshotID,
		AnnotationContributionResourceID: fixture.contributionResourceID,
		IdempotencyKey:                   buildKey,
		TraceID:                          "live-gold-worker",
	}
	execution, err := goldService.CreateBuild(ctx, buildCommand)
	if err != nil {
		t.Fatalf("queue Gold build: %v", err)
	}
	replayExecution, err := goldService.CreateBuild(ctx, buildCommand)
	if err != nil {
		t.Fatalf("replay Gold build command: %v", err)
	}
	if replayExecution.ID != execution.ID {
		t.Fatalf("build replay execution=%s want=%s", replayExecution.ID, execution.ID)
	}

	outputVersionID := waitLiveGoldExecution(t, ctx, pool, worker, workerLog, execution.ID)

	outputVersion, err := datasetRepo.GetVersion(ctx, outputVersionID)
	if err != nil {
		t.Fatalf("read live Gold output version: %v", err)
	}
	if outputVersion.GeneratedByExecutionID == nil ||
		*outputVersion.GeneratedByExecutionID != execution.ID ||
		outputVersion.RowCount == nil ||
		*outputVersion.RowCount != 2 {
		t.Fatalf("Gold output producer/rows=%+v", outputVersion)
	}
	outputBytes := readLiveGoldObject(t, ctx, store, outputVersion.StorageURI)
	if got := goldTestSHA256(outputBytes); got != strings.ToLower(outputVersion.ChecksumValue) {
		t.Fatalf("Gold output checksum bytes=%s persisted=%s", got, outputVersion.ChecksumValue)
	}
	if !bytes.Contains(outputBytes, []byte("gold_label")) ||
		!bytes.Contains(outputBytes, []byte("SUFFICIENT_INPUT")) {
		t.Fatalf("unexpected Gold output bytes: %s", outputBytes)
	}

	binding, err := goldRepo.GetBindingByExecution(ctx, execution.ID)
	if err != nil {
		t.Fatalf("read live Gold production binding: %v", err)
	}
	if binding.Status != "FINALIZED" ||
		binding.OutputDatasetVersionID != outputVersion.ID ||
		binding.AnnotationSnapshotID != fixture.snapshotID ||
		binding.OutputChecksumSHA256 != strings.ToLower(outputVersion.ChecksumValue) {
		t.Fatalf("live Gold binding=%+v", binding)
	}
	goldCount(
		t,
		ctx,
		pool,
		1,
		"SELECT count(*) FROM dataset_version_lineage WHERE output_version_id=$1 AND input_version_id=$2 AND relation_type='DERIVED_FROM'",
		outputVersion.ID,
		fixture.inputDatasetVersionID,
	)

	qualityRepo := qualityinfra.NewPostgresRepository(pool)
	qualityService := qualityapp.NewService(
		"",
		txManager,
		datasetRepo,
		qualityRepo,
		store,
		evidence.NewQueryRepository(pool),
	).ConfigureGold(goldRepo, annotationService)
	attemptID := uuid.New()
	assessment, err := qualityService.RunGold(ctx, qualityapp.GoldRunCommand{
		WorkspaceID:         fixture.workspaceID,
		DatasetVersionID:    outputVersion.ID,
		AssessmentAttemptID: attemptID,
		TraceID:             "live-gold-quality",
	})
	if err != nil {
		t.Fatalf("run formal live Gold quality: %v", err)
	}
	if assessment.DatasetVersionID != outputVersion.ID ||
		assessment.GateDecision != "PASS" ||
		assessment.RuleSetRef != qualityapp.GoldRuleSetRef ||
		assessment.EvaluatorName != qualityapp.GoldEvaluatorName {
		t.Fatalf("live Gold assessment=%+v", assessment)
	}
	if assessment.Metrics["productionBindingId"] != binding.ID.String() ||
		assessment.Metrics["annotationSnapshotId"] != fixture.snapshotID.String() ||
		assessment.Metrics["formalAssessment"] != true {
		t.Fatalf("live Gold assessment metrics=%+v", assessment.Metrics)
	}
	replayedAssessment, err := qualityService.RunGold(ctx, qualityapp.GoldRunCommand{
		WorkspaceID:         fixture.workspaceID,
		DatasetVersionID:    outputVersion.ID,
		AssessmentAttemptID: attemptID,
		TraceID:             "live-gold-quality-replay",
	})
	if err != nil {
		t.Fatalf("replay formal live Gold quality: %v", err)
	}
	if replayedAssessment.ID != assessment.ID {
		t.Fatalf("quality replay assessment=%s want=%s", replayedAssessment.ID, assessment.ID)
	}

	actorID := uuid.New()
	rightsRepo := rightsinfra.NewPostgresRepository(pool)
	requiredResources, err := rightsRepo.RequiredLineageInputs(ctx, outputVersion.ID)
	if err != nil {
		t.Fatalf("resolve live Gold required rights resources: %v", err)
	}
	if len(requiredResources) != 2 {
		t.Fatalf("live Gold required rights resources=%+v want 2", requiredResources)
	}
	var sawDataset, sawAnnotationContribution bool
	for _, required := range requiredResources {
		switch required.DependencyKind {
		case rightsdomain.EffectiveDependencyDatasetVersion:
			if required.DatasetVersionID != fixture.inputDatasetVersionID ||
				required.DataResourceID != fixture.inputResourceID ||
				!required.ResourceMapped {
				t.Fatalf("live Gold dataset rights dependency=%+v", required)
			}
			sawDataset = true
		case rightsdomain.EffectiveDependencyAnnotationContribution:
			if required.DatasetVersionID != uuid.Nil ||
				required.DataResourceID != fixture.contributionResourceID ||
				!required.ResourceMapped {
				t.Fatalf("live Gold annotation rights dependency=%+v", required)
			}
			sawAnnotationContribution = true
		default:
			t.Fatalf("unexpected live Gold rights dependency kind %q", required.DependencyKind)
		}
	}
	if !sawDataset || !sawAnnotationContribution {
		t.Fatalf("live Gold typed rights dependencies dataset=%v annotation=%v", sawDataset, sawAnnotationContribution)
	}

	rightsService := rightsapp.NewService(txManager, rightsRepo)
	var annotationRightsDeclarationID uuid.UUID
	for _, rightsResourceID := range []uuid.UUID{fixture.inputResourceID, fixture.contributionResourceID} {
		declaration, err := rightsService.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{
			Spec: rightsdomain.RightsDeclarationSpec{
				WorkspaceID:       fixture.workspaceID,
				DataResourceID:    rightsResourceID,
				ClaimantRef:       "LIVE-GOLD-RIGHTS-HOLDER",
				BasisType:         "LICENSE",
				BasisRef:          "live-gold-delivery",
				ConsumerScopeType: "EXPLICIT",
				ConsumerRef:       "GOLD-PILOT-CONSUMER",
				Parties: []rightsdomain.RightsParty{{
					PartyRef: "LIVE-GOLD-RIGHTS-HOLDER",
					Role:     "RIGHTS_HOLDER",
				}},
				Permissions: []rightsdomain.RightsPermission{{
					Kind:    rightsdomain.PermissionUse,
					Action:  "USE",
					Purpose: "GOLD-PILOT",
					Scope: rightsdomain.NormalizedScope{
						Type: "ALL_RESOURCE",
						Ref:  rightsResourceID.String(),
					},
				}},
				ActorID: &actorID,
			},
			TraceID: "live-gold-delivery-rights",
		})
		if err != nil {
			t.Fatalf("create live Gold rights declaration for %s: %v", rightsResourceID, err)
		}
		if _, err := rightsService.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{
			DeclarationID: declaration.ID,
			Outcome:       rightsdomain.DeclarationVerified,
			ActorID:       &actorID,
			TraceID:       "live-gold-delivery-rights",
		}); err != nil {
			t.Fatalf("verify live Gold rights declaration for %s: %v", rightsResourceID, err)
		}
		if rightsResourceID == fixture.contributionResourceID {
			annotationRightsDeclarationID = declaration.ID
		}
	}
	if annotationRightsDeclarationID == uuid.Nil {
		t.Fatal("live Gold annotation contribution rights declaration was not captured")
	}

	allowedRights, err := rightsService.ComputeEffectiveRights(ctx, rightsapp.ComputeEffectiveRightsCommand{
		WorkspaceID:            fixture.workspaceID,
		TargetDatasetVersionID: outputVersion.ID,
		ConsumerRef:            "GOLD-PILOT-CONSUMER",
		Purpose:                "GOLD-PILOT",
		AsOf:                   time.Now().UTC(),
		AsOfProvided:           true,
		ActivityID:             ptrUUID(uuid.New()),
		ActorID:                &actorID,
		TraceID:                "live-gold-delivery-rights-allowed",
	})
	if err != nil {
		t.Fatalf("compute live Gold EffectiveRights: %v", err)
	}
	var useAllowed bool
	for _, action := range allowedRights.Actions {
		if action.Action == "USE" {
			useAllowed = action.Decision == rightsdomain.DecisionAllowed
		}
	}
	if !useAllowed {
		t.Fatalf("live Gold EffectiveRights USE must be ALLOWED: %+v", allowedRights.Actions)
	}

	evidenceTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin live Gold certification evidence tx: %v", err)
	}
	evidenceRecord, err := evidence.Append(ctx, evidenceTx, evidence.Record{
		WorkspaceID:  fixture.workspaceID,
		EvidenceType: "GOLD_CERTIFICATION_INPUT",
		Title:        "Live Gold certification frozen input",
		SourceType:   "DATASET_VERSION",
		SourceID:     &outputVersion.ID,
		Metadata: map[string]any{
			"goldProductionBindingId":   binding.ID,
			"annotationSnapshotId":      fixture.snapshotID,
			"qualityAssessmentId":       assessment.ID,
			"effectiveRightsSnapshotId": allowedRights.ID,
		},
		CreatedBy: &actorID,
	}, evidence.Relation{
		ObjectType:   "DATASET_VERSION",
		ObjectID:     outputVersion.ID,
		RelationType: "CERTIFICATION_INPUT",
	})
	if err != nil {
		_ = evidenceTx.Rollback(ctx)
		t.Fatalf("append live Gold certification evidence: %v", err)
	}
	certificationEvidence, err := evidence.CreateSnapshot(
		ctx,
		evidenceTx,
		fixture.workspaceID,
		"DATASET_VERSION",
		outputVersion.ID,
		map[string]any{
			"goldProductionBindingId": binding.ID,
			"annotationSnapshotId":    fixture.snapshotID,
		},
		[]evidence.SnapshotItem{{
			EvidenceID: evidenceRecord.ID,
			Category:   "GOLD_CERTIFICATION_INPUT",
		}},
		&actorID,
	)
	if err != nil {
		_ = evidenceTx.Rollback(ctx)
		t.Fatalf("create live Gold EvidenceSnapshot: %v", err)
	}
	if err := evidenceTx.Commit(ctx); err != nil {
		t.Fatalf("commit live Gold certification evidence: %v", err)
	}

	goldProfileSpec, err := certificationdomain.NewGoldCertificationProfile(
		"GOLD-PILOT",
		"USE",
		"GOLD-PILOT-CONSUMER",
		[]certificationdomain.ScopeRef{
			{Type: "ALL_RESOURCE", Ref: fixture.inputResourceID.String()},
			{Type: "ALL_RESOURCE", Ref: fixture.contributionResourceID.String()},
		},
	)
	if err != nil {
		t.Fatalf("build live Gold CertificationProfile: %v", err)
	}
	profileRepo := certificationinfra.NewProfileRepository(pool)
	profileService := certificationapp.NewProfileService(txManager, profileRepo)
	goldProfile, err := profileService.Create(ctx, certificationapp.CreateProfileCommand{
		WorkspaceID: fixture.workspaceID,
		Profile:     goldProfileSpec,
		ActorID:     &actorID,
		TraceID:     "live-gold-certification-profile",
	})
	if err != nil {
		t.Fatalf("create live Gold CertificationProfile: %v", err)
	}

	certificationRepo := certificationinfra.NewCertificationRepository(pool)
	certificationService := certificationapp.NewCertificationService(
		txManager,
		profileRepo,
		certificationRepo,
		certificationapp.NewReferenceEvidenceResolver(),
	)
	certificationKey := "live-gold-certification-" + uuid.NewString()
	certification, err := certificationService.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
		WorkspaceID:               fixture.workspaceID,
		DatasetVersionID:          outputVersion.ID,
		ProfileID:                 goldProfile.ID,
		QualityAssessmentID:       assessment.ID,
		EffectiveRightsSnapshotID: &allowedRights.ID,
		EvidenceSnapshotID:        &certificationEvidence.ID,
		IdempotencyKey:            certificationKey,
		ActorID:                   &actorID,
		TraceID:                   "live-gold-certification",
		Now:                       time.Now().UTC(),
		CostActivity: &cost.CertificationActivity{
			ActivityID:  uuid.New(),
			Quantity:    1,
			Unit:        "certification",
			PricingMode: "ACTUAL",
			Metadata:    map[string]any{"phase": "live-gold-certification"},
		},
	})
	if err != nil {
		t.Fatalf("evaluate live Gold certification: %v", err)
	}
	if certification.Decision != certificationdomain.DecisionCertified ||
		certification.GoldProductionBindingID == nil ||
		*certification.GoldProductionBindingID != binding.ID ||
		certification.AnnotationSnapshotID == nil ||
		*certification.AnnotationSnapshotID != fixture.snapshotID {
		t.Fatalf("live Gold certification=%+v", certification)
	}

	eligibility := certificationapp.NewEligibilityService(certificationService, datasetRepo, rightsRepo)
	directGate := deliveryapp.NewCertificationDirectDataGate(eligibility)
	deliveryRepo := deliveryinfra.NewPostgresRepository(pool)
	directService := deliveryapp.NewDirectDataService(txManager, deliveryRepo, directGate, datasetRepo)
	deliveryKey := "live-gold-direct-" + uuid.NewString()
	deliveryCommand := deliveryapp.DirectDataCommand{
		WorkspaceID:          fixture.workspaceID,
		DatasetVersionID:     outputVersion.ID,
		ProfileID:            goldProfile.ID,
		PrincipalRef:         "live-gold-principal",
		EffectiveConsumerRef: "GOLD-PILOT-CONSUMER",
		Purpose:              "GOLD-PILOT",
		Action:               "USE",
		ScopeType:            "ALL_RESOURCE",
		ScopeRef:             outputVersion.ID.String(),
		IdempotencyKey:       deliveryKey,
		TraceID:              "live-gold-direct-data",
	}
	delivered, err := directService.Deliver(ctx, deliveryCommand)
	if err != nil {
		t.Fatalf("issue live Gold DIRECT_DATA: %v", err)
	}
	if !delivered.PayloadReady ||
		delivered.Operation.Status != deliverydomain.StatusIssued ||
		delivered.DatasetVersion.ID != outputVersion.ID ||
		delivered.Operation.CertificationRef == nil ||
		*delivered.Operation.CertificationRef != certification.ID {
		t.Fatalf("live Gold DIRECT_DATA result=%+v", delivered)
	}

	replayedDelivery, err := directService.Deliver(ctx, deliveryCommand)
	if !errors.Is(err, deliveryapp.ErrDirectDataReplayRequiresNewAttempt) ||
		!replayedDelivery.ReplayRequired ||
		replayedDelivery.Operation.ID != delivered.Operation.ID {
		t.Fatalf("live Gold delivery replay=%+v err=%v", replayedDelivery, err)
	}
	goldCount(t, ctx, pool, 1, `
		SELECT count(*) FROM delivery_operation
		WHERE workspace_id=$1 AND idempotency_key=$2
	`, fixture.workspaceID, deliveryKey)
	goldCount(t, ctx, pool, 1, `
		SELECT count(*)
		FROM cost_allocation
		WHERE delivery_operation_id=$1
	`, delivered.Operation.ID)

	liveExplanation, err := readmodel.NewRepository(pool).GoldExplanation(
		ctx,
		fixture.workspaceID,
		outputVersion.ID,
	)
	if err != nil {
		t.Fatalf("read live Gold explanation trace: %v", err)
	}
	for _, phase := range []string{"HUMAN_REVIEW", "GOLD_BUILD", "GOLD_QUALITY", "GOLD_CERTIFICATION", "DIRECT_DATA"} {
		if !goldTraceHasCostPhase(liveExplanation.Trace.Costs, phase) {
			t.Fatalf("live Gold trace missing cost phase %s: %+v", phase, liveExplanation.Trace.Costs)
		}
	}
	if !goldTraceHasEvidencePhase(liveExplanation.Trace.Evidence, "DIRECT_DATA") {
		t.Fatalf("live Gold trace missing DIRECT_DATA evidence: %+v", liveExplanation.Trace.Evidence)
	}
	if !goldTraceHasAuditAction(liveExplanation.Trace.Audit, "DATASETDELIVERYISSUED") {
		t.Fatalf("live Gold trace missing delivery audit: %+v", liveExplanation.Trace.Audit)
	}

	if _, err := rightsService.DisposeRightsDeclaration(ctx, rightsapp.DisposeRightsDeclarationCommand{
		DeclarationID: annotationRightsDeclarationID,
		Disposition:   rightsdomain.DispositionInvalidated,
		EffectiveAt:   time.Now().UTC(),
		Reason:        "live Gold annotation contribution rights withdrawn",
		ActorID:       &actorID,
		TraceID:       "live-gold-rights-revoked",
	}); err != nil {
		t.Fatalf("revoke live Gold annotation rights: %v", err)
	}
	blockedRightsCommand := deliveryCommand
	blockedRightsCommand.IdempotencyKey = "live-gold-rights-blocked-" + uuid.NewString()
	blockedRights, err := directService.Deliver(ctx, blockedRightsCommand)
	if err != nil {
		t.Fatalf("deliver after live Gold rights revoke: %v", err)
	}
	if blockedRights.PayloadReady || blockedRights.Operation.Status != deliverydomain.StatusBlocked ||
		!containsLiveGoldBlocker(blockedRights.Blockers, "CURRENT_ENTITLEMENT_BLOCKED") {
		t.Fatalf("live Gold rights-revoked delivery=%+v", blockedRights)
	}

	replacementRights, err := rightsService.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{
		Spec: rightsdomain.RightsDeclarationSpec{
			WorkspaceID:       fixture.workspaceID,
			DataResourceID:    fixture.contributionResourceID,
			ClaimantRef:       "LIVE-GOLD-RIGHTS-HOLDER",
			BasisType:         "LICENSE",
			BasisRef:          "live-gold-delivery-restored",
			ConsumerScopeType: "EXPLICIT",
			ConsumerRef:       "GOLD-PILOT-CONSUMER",
			Parties: []rightsdomain.RightsParty{{
				PartyRef: "LIVE-GOLD-RIGHTS-HOLDER",
				Role:     "RIGHTS_HOLDER",
			}},
			Permissions: []rightsdomain.RightsPermission{{
				Kind:    rightsdomain.PermissionUse,
				Action:  "USE",
				Purpose: "GOLD-PILOT",
				Scope: rightsdomain.NormalizedScope{
					Type: "ALL_RESOURCE",
					Ref:  fixture.contributionResourceID.String(),
				},
			}},
			ActorID: &actorID,
		},
		TraceID: "live-gold-rights-restored",
	})
	if err != nil {
		t.Fatalf("replace live Gold annotation rights: %v", err)
	}
	if _, err := rightsService.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{
		DeclarationID: replacementRights.ID,
		Outcome:       rightsdomain.DeclarationVerified,
		ActorID:       &actorID,
		TraceID:       "live-gold-rights-restored",
	}); err != nil {
		t.Fatalf("verify replacement live Gold annotation rights: %v", err)
	}
	restoredCommand := deliveryCommand
	restoredCommand.IdempotencyKey = "live-gold-rights-restored-" + uuid.NewString()
	restoredDelivery, err := directService.Deliver(ctx, restoredCommand)
	if err != nil {
		t.Fatalf("deliver after live Gold rights restoration: %v", err)
	}
	if !restoredDelivery.PayloadReady || restoredDelivery.Operation.Status != deliverydomain.StatusIssued {
		t.Fatalf("live Gold restored-rights delivery=%+v", restoredDelivery)
	}

	invalidateService := datasetapp.NewInvalidateVersionService(txManager, datasetRepo)
	invalidated, err := invalidateService.Handle(ctx, datasetapp.InvalidateVersionCommand{
		VersionID: outputVersion.ID,
		Reason:    "live Gold invalidation negative control",
		ActorID:   &actorID,
		TraceID:   "live-gold-invalidated",
	})
	if err != nil {
		t.Fatalf("invalidate live Gold DatasetVersion: %v", err)
	}
	if invalidated.Status != "INVALID" {
		t.Fatalf("invalidated live Gold DatasetVersion status=%s", invalidated.Status)
	}
	blockedInvalidCommand := deliveryCommand
	blockedInvalidCommand.IdempotencyKey = "live-gold-invalid-blocked-" + uuid.NewString()
	blockedInvalid, err := directService.Deliver(ctx, blockedInvalidCommand)
	if err != nil {
		t.Fatalf("deliver invalid live Gold DatasetVersion: %v", err)
	}
	if blockedInvalid.PayloadReady ||
		blockedInvalid.Operation.Status != deliverydomain.StatusBlocked ||
		!containsLiveGoldBlocker(blockedInvalid.Blockers, "DATASET_VERSION_INVALID") {
		t.Fatalf("live Gold invalid-version delivery=%+v", blockedInvalid)
	}

	goldCount(t, ctx, pool, 1, `
		SELECT count(*) FROM dataset_certification
		WHERE id=$1 AND decision='CERTIFIED'
	`, certification.ID)

	writeLiveGoldUIManifest(t, liveGoldUIManifest{
		WorkspaceID:         fixture.workspaceID,
		GoldDatasetID:       fixture.outputDatasetID,
		GoldVersionID:       outputVersion.ID,
		GoldBindingID:       binding.ID,
		GoldSnapshotID:      fixture.snapshotID,
		GoldAssessmentID:    assessment.ID,
		GoldCertificationID: certification.ID,
		OutputChecksum:      outputVersion.ChecksumValue,
		ExpectedDelivery:    "BLOCKED",
		ExpectedBlocker:     "DATASET_VERSION_INVALID",
	})
}

func writeLiveGoldUIManifest(t *testing.T, manifest liveGoldUIManifest) {
	t.Helper()
	artifacts := strings.TrimSpace(os.Getenv("LIVE_BROWSER_ARTIFACTS"))
	if artifacts == "" {
		t.Fatal("LIVE_BROWSER_ARTIFACTS is required for live Gold UI acceptance")
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal live Gold UI manifest: %v", err)
	}
	path := filepath.Join(artifacts, "gold-live-ui.json")
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		t.Fatalf("write live Gold UI manifest: %v", err)
	}
}

func loadLiveGoldSharedFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) liveGoldFixture {
	t.Helper()
	artifacts := strings.TrimSpace(os.Getenv("LIVE_BROWSER_ARTIFACTS"))
	if artifacts == "" {
		t.Fatal("LIVE_BROWSER_ARTIFACTS is required for shared-facts Gold acceptance")
	}
	path := filepath.Join(artifacts, "gold-shared-facts.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shared Gold facts manifest: %v", err)
	}
	var manifest liveGoldSharedManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("decode shared Gold facts manifest: %v", err)
	}
	for name, id := range map[string]uuid.UUID{
		"workspace":                        manifest.WorkspaceID,
		"input resource":                   manifest.InputResourceID,
		"input DatasetVersion":             manifest.InputDatasetVersionID,
		"input certification":              manifest.InputCertificationID,
		"annotation contribution resource": manifest.ContributionResourceID,
		"campaign":                         manifest.CampaignID,
		"snapshot":                         manifest.SnapshotID,
	} {
		if id == uuid.Nil {
			t.Fatalf("shared Gold manifest has empty %s id", name)
		}
	}

	var snapshotStatus string
	if err := pool.QueryRow(ctx, `
		SELECT status
		FROM annotation_snapshot
		WHERE id=$1 AND workspace_id=$2 AND campaign_id=$3
	`, manifest.SnapshotID, manifest.WorkspaceID, manifest.CampaignID).Scan(&snapshotStatus); err != nil {
		t.Fatalf("verify shared FINALIZED AnnotationSnapshot: %v", err)
	}
	if snapshotStatus != "FINALIZED" {
		t.Fatalf("shared AnnotationSnapshot status=%s want=FINALIZED", snapshotStatus)
	}

	outputDatasetID := uuid.New()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	goldSQL(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'live Gold output from real Label Studio facts','CURATED')
	`, outputDatasetID, manifest.WorkspaceID, "LIVE-GOLD-SHARED-OUT-"+suffix)

	return liveGoldFixture{
		workspaceID:            manifest.WorkspaceID,
		inputResourceID:        manifest.InputResourceID,
		inputDatasetVersionID:  manifest.InputDatasetVersionID,
		inputCertificationID:   manifest.InputCertificationID,
		contributionResourceID: manifest.ContributionResourceID,
		campaignID:             manifest.CampaignID,
		snapshotID:             manifest.SnapshotID,
		outputDatasetID:        outputDatasetID,
	}
}

func startLiveGoldWorker(
	t *testing.T,
	ctx context.Context,
	workerBinary string,
) (*exec.Cmd, string) {
	t.Helper()
	artifacts := strings.TrimSpace(os.Getenv("LIVE_BROWSER_ARTIFACTS"))
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		t.Fatalf("create live worker artifact directory: %v", err)
	}
	logPath := filepath.Join(artifacts, "gold-worker-live.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("open live Gold worker log: %v", err)
	}
	cmd := exec.CommandContext(ctx, workerBinary)
	cmd.Env = os.Environ()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start live Gold worker: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(4 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		_ = logFile.Close()
	})
	return cmd, logPath
}

func waitLiveOutboxIdle(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	worker *exec.Cmd,
	workerLog string,
) {
	t.Helper()
	var initial int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_event
		WHERE status IN ('PENDING','PROCESSING','FAILED')
	`).Scan(&initial); err != nil {
		t.Fatalf("count initial live outbox backlog: %v", err)
	}
	budget := 75 * time.Second
	if scaled := time.Duration(initial)*1500*time.Millisecond + 10*time.Second; scaled > budget {
		budget = scaled
	}
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if err := worker.Process.Signal(syscall.Signal(0)); err != nil {
			content, _ := os.ReadFile(workerLog)
			t.Fatalf("live worker exited while draining outbox backlog: %v\n%s", err, content)
		}
		var pending int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM outbox_event
			WHERE status IN ('PENDING','PROCESSING','FAILED')
		`).Scan(&pending); err != nil {
			t.Fatalf("count live outbox backlog: %v", err)
		}
		if pending == 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	var pending int
	_ = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_event
		WHERE status IN ('PENDING','PROCESSING','FAILED')
	`).Scan(&pending)
	content, _ := os.ReadFile(workerLog)
	t.Fatalf("live outbox backlog did not drain initial=%d pending=%d budget=%s\n%s", initial, pending, budget, content)
}

func waitLiveGoldExecution(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	worker *exec.Cmd,
	workerLog string,
	executionID uuid.UUID,
) uuid.UUID {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if err := worker.Process.Signal(syscall.Signal(0)); err != nil {
			content, _ := os.ReadFile(workerLog)
			t.Fatalf("live Gold worker exited early: %v\n%s", err, content)
		}
		var status string
		var outputID *uuid.UUID
		if err := pool.QueryRow(ctx, `
			SELECT status, output_dataset_version_id
			FROM execution
			WHERE id=$1
		`, executionID).Scan(&status, &outputID); err != nil {
			t.Fatalf("read live Gold execution: %v", err)
		}
		switch status {
		case "SUCCEEDED":
			if outputID == nil || *outputID == uuid.Nil {
				t.Fatal("SUCCEEDED Gold execution has no output DatasetVersion")
			}
			return *outputID
		case "FAILED", "CANCELLED":
			content, _ := os.ReadFile(workerLog)
			t.Fatalf("live Gold execution ended %s\n%s", status, content)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	var status string
	var outputID *uuid.UUID
	_ = pool.QueryRow(ctx, `
		SELECT status, output_dataset_version_id
		FROM execution
		WHERE id=$1
	`, executionID).Scan(&status, &outputID)
	var queuedEvents, confirmedDispatches int
	_ = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_event
		WHERE aggregate_type='EXECUTION'
		  AND aggregate_id=$1
		  AND event_type='ExecutionQueued'
	`, executionID).Scan(&queuedEvents)
	_ = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM outbox_event_consumption c
		JOIN outbox_event e ON e.id=c.event_id
		WHERE e.aggregate_type='EXECUTION'
		  AND e.aggregate_id=$1
		  AND e.event_type='ExecutionQueued'
	`, executionID).Scan(&confirmedDispatches)
	content, _ := os.ReadFile(workerLog)
	t.Fatalf(
		"live Gold execution timeout status=%s output=%v queuedEvents=%d dispatchConfirmations=%d\n%s",
		status,
		outputID,
		queuedEvents,
		confirmedDispatches,
		content,
	)
	return uuid.Nil
}

func readLiveGoldObject(
	t *testing.T,
	ctx context.Context,
	store interface {
		Get(context.Context, string) (io.ReadCloser, error)
	},
	uri string,
) []byte {
	t.Helper()
	reader, err := store.Get(ctx, uri)
	if err != nil {
		t.Fatalf("read live Gold object: %v", err)
	}
	defer reader.Close()
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read live Gold object bytes: %v", err)
	}
	return content
}

func containsLiveGoldBlocker(blockers []string, want string) bool {
	for _, blocker := range blockers {
		if blocker == want {
			return true
		}
	}
	return false
}
