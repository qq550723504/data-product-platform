package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	complianceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/queue"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
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
)

const liveAPI = "http://127.0.0.1:18080"

// This is opt-in and only accepts the isolated stack defined by tests/live-browser.
// Preparation uses actual application commands. Review and publish are performed
// exclusively by Chromium -> Next Server Actions -> the separately built Core API.
func TestBrowserLiveCorePOC(t *testing.T) {
	if os.Getenv("LIVE_BROWSER_ACCEPTANCE") != "1" {
		t.Skip("opt-in: run tests/live-browser against its disposable local stack")
	}
	t.Setenv("INDUSTRY_PACK_ROOT", repoPath(t, "industry-packs"))
	cfg, err := config.Load()
	liveOK(t, err, "load isolated config")
	liveOK(t, validateLiveConfig(cfg), "refuse non-isolated target")
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, cfg.PostgresDSN)
	liveOK(t, err, "open isolated PostgreSQL")
	defer pool.Close()
	var resourceCount int
	liveOK(t, pool.QueryRow(ctx, "SELECT count(*) FROM data_resource").Scan(&resourceCount), "verify clean database")
	if resourceCount != 0 {
		t.Fatal("live suite requires a fresh database; it never truncates an existing database")
	}
	store, err := storage.New(cfg.Storage.Endpoint, cfg.Storage.AccessKey, cfg.Storage.SecretKey, cfg.Storage.Bucket, cfg.Storage.UseSSL)
	liveOK(t, err, "create real S3 client")
	liveOK(t, store.EnsureBucket(ctx), "create isolated bucket")

	artifacts := repoPath(t, ".artifacts", "live-browser")
	liveOK(t, os.MkdirAll(artifacts, 0700), "create test artifacts")
	// Probe before spawning: never send commands to a pre-existing API process.
	listener, err := net.Listen("tcp", "127.0.0.1:18080")
	liveOK(t, err, "API port must be unused")
	liveOK(t, listener.Close(), "release API port")
	api := startLiveProcess(t, ctx, filepath.Join(artifacts, "platform-api"), repoPath(t, "apps", "platform"), filepath.Join(artifacts, "api.log"))
	startLiveProcess(t, ctx, filepath.Join(artifacts, "platform-worker"), repoPath(t, "apps", "platform"), filepath.Join(artifacts, "worker.log"))
	workspaceID, seedActor, reviewerID, publisherID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	liveWaitAPI(t, ctx, api, workspaceID)
	traceID, suffix := "browser-live-"+uuid.NewString(), uuid.NewString()
	reason := "LIVE_BROWSER_REVIEW_" + suffix
	tx := transaction.NewManager(pool)
	resourceRepo := resourceinfra.NewPostgresRepository()
	resourceService := resourceapp.NewCreateService(tx, resourceRepo)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	createDataset := datasetapp.NewCreateDatasetService(tx, datasetRepo, resourceRepo)
	upload := datasetapp.NewUploadVersionService(tx, datasetRepo, store)
	entityRepo := entityinfra.NewPostgresRepository(pool)
	entityService := entityapp.NewMatchService(cfg.IndustryPackRoot, tx, entityRepo, datasetRepo, upload, store)

	enterpriseResource := mustCreateResource(t, ctx, resourceService, workspaceID, "ENTERPRISE", "Live enterprise source", &seedActor, traceID)
	leaseResource := mustCreateResource(t, ctx, resourceService, workspaceID, "LEASE", "Live lease source", &seedActor, traceID)
	energyResource := mustCreateResource(t, ctx, resourceService, workspaceID, "ENERGY", "Live energy source", &seedActor, traceID)
	enterpriseDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "ENTERPRISE-RAW", "Live enterprise RAW", datasetdomain.DatasetTypeRaw, &enterpriseResource.ID, &seedActor, traceID)
	leaseDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "LEASE-RAW", "Live lease RAW", datasetdomain.DatasetTypeRaw, &leaseResource.ID, &seedActor, traceID)
	energyDataset := mustCreateDataset(t, ctx, createDataset, workspaceID, "ENERGY-RAW", "Live energy RAW", datasetdomain.DatasetTypeRaw, &energyResource.ID, &seedActor, traceID)
	standardized := mustCreateDataset(t, ctx, createDataset, workspaceID, "STANDARDIZED", "Live standardized", datasetdomain.DatasetTypeStandardized, nil, &seedActor, traceID)
	curated := mustCreateDataset(t, ctx, createDataset, workspaceID, "CURATED", "Live curated", datasetdomain.DatasetTypeCurated, nil, &seedActor, traceID)
	enterpriseName := "enterprise-" + suffix + ".csv"
	enterpriseVersion := mustUploadFixture(t, ctx, upload, enterpriseDataset.ID, "enterprise.csv", enterpriseName, &seedActor, traceID)
	leaseVersion := mustUploadFixture(t, ctx, upload, leaseDataset.ID, "lease.csv", "lease-"+suffix+".csv", &seedActor, traceID)
	energyVersion := mustUploadFixture(t, ctx, upload, energyDataset.ID, "energy.csv", "energy-"+suffix+".csv", &seedActor, traceID)
	job, err := entityService.Start(ctx, entityapp.StartJobCommand{
		WorkspaceID: workspaceID, InputDatasetVersionID: enterpriseVersion.ID, OutputDatasetID: standardized.ID,
		SourceType: "CSV", SourceRef: enterpriseName, SourceRole: entitydomain.SourceAnchor,
		PolicyRef: companyPolicyRef, ActorID: &seedActor, TraceID: traceID,
	})
	liveOK(t, err, "prepare pending real entity match job")
	if job.Status != entitydomain.JobWaitingReview || job.OutputDatasetVersionID != nil {
		t.Fatalf("reference job must await browser review, got %s / %v", job.Status, job.OutputDatasetVersionID)
	}
	candidates, err := entityRepo.ListCandidates(ctx, job.ID)
	liveOK(t, err, "read real candidates")
	pending := make([]uuid.UUID, 0)
	for _, candidate := range candidates {
		if candidate.Status == entitydomain.CandidatePending {
			pending = append(pending, candidate.ID)
		}
	}
	if len(pending) == 0 {
		t.Fatal("reference data failed to produce any manual review; refusing a vacuous browser test")
	}
	manifest := map[string]any{
		"workspaceId": workspaceID, "reviewerId": reviewerID, "publisherId": publisherID,
		"jobId": job.ID, "candidateIds": pending, "reason": reason,
	}
	livePost(t, ctx, "/api/v1/entity-match-reviews/"+pending[0].String()+"/confirm", reviewerID, "", map[string]any{"reason": "   "}, http.StatusBadRequest)
	assertLiveCount(t, ctx, pool, 0, "SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='ENTITY_MATCH_CONFIRMED'", pending[0])
	runLiveBrowser(t, ctx, "review", manifest, artifacts)

	job, err = entityRepo.GetJob(ctx, job.ID)
	liveOK(t, err, "read job after browser review")
	if job.Status != entitydomain.JobSucceeded || job.OutputDatasetVersionID == nil {
		t.Fatalf("browser did not finalize the real entity job: %s / %v", job.Status, job.OutputDatasetVersionID)
	}
	for _, id := range pending {
		assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM audit_event WHERE workspace_id=$1 AND object_id=$2 AND action='ENTITY_MATCH_CONFIRMED' AND actor_id=$3 AND reason=$4`, workspaceID, id, reviewerID, reason)
		items, err := evidence.NewQueryRepository(pool).ListForObject(ctx, "ENTITY_MATCH_CANDIDATE", id)
		liveOK(t, err, "read persisted review evidence")
		found := false
		for _, item := range items {
			if !item.IntegrityValid {
				t.Fatalf("review evidence %s failed integrity verification", item.ID)
			}
			if item.Metadata["decision"] == "CONFIRMED" && item.Metadata["reviewerReason"] == reason {
				found = true
			}
		}
		if !found {
			t.Fatalf("candidate %s has no persisted review evidence with the browser reason", id)
		}
	}
	mapping, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", enterpriseName, "ENT-005")
	liveOK(t, err, "read persisted alias mapping")
	if mapping.ReviewedBy == nil || *mapping.ReviewedBy != reviewerID || mapping.ReviewerReason != reason || mapping.EvidenceID == nil {
		t.Fatalf("persisted mapping lost browser reviewer/reason/evidence: %+v", mapping)
	}
	t.Logf("DATABASE_REVIEW_VERIFIED candidates=%d reviewer=%s standardized=%s", len(pending), reviewerID, *job.OutputDatasetVersionID)

	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	workflowService := workflowapp.NewWorkflowVersionService(tx, workflowRepo)
	workflow, err := workflowService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID: workspaceID, Code: "browser-activity-" + suffix, Name: "Live browser activity", Version: "1.0.0",
		DefinitionRef:  "examples/enterprise-activity/workflow/workflow-v1.yaml",
		DefinitionYAML: readRepoFile(t, "examples", "enterprise-activity", "workflow", "workflow-v1.yaml"), ActorID: &seedActor, TraceID: traceID,
	})
	liveOK(t, err, "create actual workflow version")
	queueClient := queue.NewClient(queue.Config{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	defer queueClient.Close()
	executionService := workflowapp.NewExecutionService(tx, workflowRepo)
	execution, err := executionService.Create(ctx, workflowapp.CreateExecutionCommand{
		WorkspaceID: workspaceID, WorkflowVersionID: workflow.ID, OutputDatasetID: curated.ID, TargetPeriod: "2025-03",
		Inputs: []workflowdomain.InputBinding{
			{Name: "enterprise_raw", DatasetVersionID: enterpriseVersion.ID},
			{Name: "enterprise_resolution", DatasetVersionID: *job.OutputDatasetVersionID},
			{Name: "lease_raw", DatasetVersionID: leaseVersion.ID},
			{Name: "energy_raw", DatasetVersionID: energyVersion.ID},
		}, IdempotencyKey: "browser-live-create-" + suffix, ActorID: &seedActor, TraceID: traceID,
	})
	liveOK(t, err, "enqueue production through real Redis")
	deadline := time.Now().Add(60 * time.Second)
	for {
		execution, err = workflowRepo.GetExecution(ctx, execution.ID)
		liveOK(t, err, "poll actual worker execution")
		if execution.Status == workflowdomain.ExecutionSucceeded && execution.OutputDatasetVersionID != nil {
			break
		}
		if execution.Status == workflowdomain.ExecutionFailed || time.Now().After(deadline) {
			t.Fatalf("real worker did not succeed: %+v (see worker.log)", execution)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
	output, err := datasetRepo.GetVersion(ctx, *execution.OutputDatasetVersionID)
	liveOK(t, err, "read real worker output version")
	csv := readLiveObject(t, ctx, store, output.StorageURI)
	if !bytes.Contains(csv, []byte("96.01")) {
		t.Fatal("real object storage output does not contain the deterministic reference score")
	}
	if bytes.Contains(csv, []byte("company_name")) || bytes.Contains(csv, []byte("深圳星云科技有限公司")) {
		t.Fatal("real object storage V1 output leaked mutable company display name")
	}
	if output.GeneratedByExecutionID == nil || *output.GeneratedByExecutionID != execution.ID {
		t.Fatal("output version lost its producing execution identity")
	}
	t.Logf("WORKER_STORAGE_VERIFIED execution=%s output=%s bytes=%d", execution.ID, output.ID, len(csv))

	qualityService := qualityapp.NewService(cfg.IndustryPackRoot, tx, datasetRepo, qualityinfra.NewPostgresRepository(pool), store)
	quality, err := qualityService.Run(ctx, qualityapp.RunCommand{WorkspaceID: workspaceID, DatasetVersionID: output.ID, RuleSetRef: qualityRuleSetRef, ActorID: &seedActor, TraceID: traceID, Now: output.ReadyAt.Add(30 * time.Minute)})
	liveOK(t, err, "evaluate actual quality rules")
	complianceService := complianceapp.NewService(cfg.IndustryPackRoot, tx, datasetRepo, complianceinfra.NewPostgresRepository(pool), store)
	compliance, err := complianceService.Run(ctx, complianceapp.RunCommand{WorkspaceID: workspaceID, DatasetVersionID: output.ID, PolicyRef: complianceRef, ActorID: &seedActor, TraceID: traceID})
	liveOK(t, err, "evaluate actual compliance rules")
	contractService := contractapp.NewService(tx, contractinfra.NewPostgresRepository(pool))
	contract, err := contractService.CreateVersionFromYAML(ctx, contractapp.CreateVersionFromYAMLCommand{
		WorkspaceID: workspaceID, SourceRef: "examples/enterprise-activity/contract/data-contract-v1.yaml",
		DocumentYAML: readRepoFile(t, "examples", "enterprise-activity", "contract", "data-contract-v1.yaml"), ActorID: &seedActor, TraceID: traceID,
	})
	liveOK(t, err, "prepare actual data contract")
	contract, err = contractService.PublishVersion(ctx, contractapp.PublishVersionCommand{VersionID: contract.ID, ActorID: &seedActor, TraceID: traceID})
	liveOK(t, err, "publish preparation contract")
	rightsService := rightsapp.NewService(tx, rightsinfra.NewPostgresRepository(pool))
	grants := []rightsdomain.ResourceGrantSpec{}
	for _, id := range []uuid.UUID{enterpriseResource.ID, leaseResource.ID, energyResource.ID} {
		grants = append(grants, rightsdomain.ResourceGrantSpec{DataResourceID: id, Actions: []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}, ScopeType: "ALL_RESOURCE", ScopeRef: id.String(), Scope: map[string]any{"useCase": purpose}})
	}
	authorization := activateAuthorization(t, ctx, rightsService, workspaceID, "AUTH-LIVE-"+suffix, time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(24*time.Hour), grants, &seedActor, traceID)
	productRepo := productinfra.NewPostgresRepository(pool)
	productService := productapp.NewService(tx, productRepo)
	product, err := productService.CreateProduct(ctx, productapp.CreateProductCommand{WorkspaceID: workspaceID, Code: "DP-LIVE-" + suffix, Name: "真实后端浏览器验收产品", Description: "Synthetic test inputs, real persisted Core facts", DomainCode: "PARK_ENTERPRISE_ACTIVITY", ActorID: &seedActor, TraceID: traceID})
	liveOK(t, err, "prepare data product")
	version, err := productService.CreateVersion(ctx, productapp.CreateVersionCommand{
		ProductID: product.ID, MajorVersion: 1, MinorVersion: 0, PatchVersion: 0, WorkflowVersionID: &workflow.ID, ContractVersionID: &contract.ID,
		EntityPolicyRef: companyPolicyRef, IndicatorSetRef: indicatorSetRef, Definition: map[string]any{"referenceImplementation": "enterprise-activity"},
		Assets:  []productdomain.AssetSpec{{AssetType: productdomain.AssetDataset, Name: "enterprise_activity_curated", DatasetID: &curated.ID, DeliveryConfig: map[string]any{"mode": "DATASET", "rawExport": false}}},
		ActorID: &seedActor, TraceID: traceID,
	})
	liveOK(t, err, "prepare immutable product version")
	createRelease := func(number string) productdomain.ProductRelease {
		r, err := productService.CreateRelease(ctx, productapp.CreateReleaseCommand{ProductID: product.ID, ProductVersionID: version.ID, ReleaseNo: number, Datasets: []productdomain.ReleaseDataset{{DatasetVersionID: output.ID, Role: productdomain.DatasetPrimary}}, ActorID: &seedActor, TraceID: traceID})
		liveOK(t, err, "prepare release")
		return r
	}
	release := createRelease("R-LIVE-READY")
	blocked := createRelease("R-LIVE-BLOCKED")
	snapshot, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{WorkspaceID: workspaceID, ProductReleaseID: &release.ID, Purpose: purpose, ConsumerRef: "LICENSED_BANK", AsOf: time.Now().UTC(), AuthorizationIDs: []uuid.UUID{authorization.ID}, ActorID: &seedActor, TraceID: traceID})
	liveOK(t, err, "create actual rights snapshot")
	readiness, err := productService.ValidateRelease(ctx, productapp.ValidateReleaseCommand{ReleaseID: release.ID, ContractVersionID: contract.ID, RightsSnapshotID: snapshot.ID, QualityResultID: quality.ID, ComplianceResultID: compliance.ID, ActorID: &seedActor, TraceID: traceID})
	liveOK(t, err, "prepare READY through Core validation")
	if readiness.Overall != "READY" || len(readiness.Blockers) != 0 {
		t.Fatalf("real Core not ready: %+v", readiness)
	}

	// Certified Dataset browser slice: reuse the exact real CURATED output and
	// governance facts already prepared for the live Core scenario. This keeps
	// the browser proof attached to the same immutable data bytes rather than a
	// parallel fixture-only dataset.
	effectiveRights, err := rightsService.ComputeEffectiveRights(ctx, rightsapp.ComputeEffectiveRightsCommand{
		WorkspaceID:            workspaceID,
		TargetDatasetVersionID: output.ID,
		ConsumerRef:            "LICENSED_BANK",
		Purpose:                purpose,
		ActorID:                &seedActor,
		TraceID:                traceID,
	})
	liveOK(t, err, "compute live Certified Dataset EffectiveRights")
	if effectiveRights.Status != "FINALIZED" {
		t.Fatalf("live Certified Dataset EffectiveRights status=%s, want FINALIZED", effectiveRights.Status)
	}

	var certificationEvidence evidence.Snapshot
	liveOK(t, tx.Do(ctx, func(ctx context.Context, dbtx pgx.Tx) error {
		record, err := evidence.Append(ctx, dbtx, evidence.Record{
			WorkspaceID:  workspaceID,
			EvidenceType: "LIVE_CERTIFIED_DATASET_TRACE",
			Title:        "Live browser Certified Dataset trace",
			SourceType:   "DATASET_VERSION",
			SourceID:     &output.ID,
			Metadata: map[string]any{
				"executionId":               execution.ID,
				"qualityAssessmentId":       quality.ID,
				"effectiveRightsSnapshotId": effectiveRights.ID,
				"productReleaseId":          release.ID,
			},
			CreatedBy: &seedActor,
		}, evidence.Relation{
			ObjectType:   "DATASET_VERSION",
			ObjectID:     output.ID,
			RelationType: "LIVE_CERTIFIED_DATASET_TRACE",
		})
		if err != nil {
			return err
		}
		snapshot, err := evidence.CreateSnapshot(
			ctx,
			dbtx,
			workspaceID,
			"DATASET_VERSION",
			output.ID,
			map[string]any{
				"acceptance":  "live-core-browser",
				"executionId": execution.ID,
			},
			[]evidence.SnapshotItem{{EvidenceID: record.ID, Category: "TRACEABILITY"}},
			&seedActor,
		)
		if err != nil {
			return err
		}
		certificationEvidence = snapshot
		return nil
	}), "create live Certified Dataset EvidenceSnapshot")

	profileRepo := certificationinfra.NewProfileRepository(pool)
	profileService := certificationapp.NewProfileService(tx, profileRepo)
	profile, err := profileService.Create(ctx, certificationapp.CreateProfileCommand{
		WorkspaceID: workspaceID,
		Profile: certificationdomain.CertificationProfile{
			ProfileRef:            "park/live-browser-certified-v1",
			Code:                  "LIVE-BROWSER-CERTIFIED",
			Name:                  "Live Browser Certified Dataset",
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
				Scopes: certificationdomain.ScopeApplicability{
					Mode: certificationdomain.ApplicabilityExplicit,
					Values: []certificationdomain.ScopeRef{
						{Type: "ALL_RESOURCE", Ref: enterpriseResource.ID.String()},
						{Type: "ALL_RESOURCE", Ref: leaseResource.ID.String()},
						{Type: "ALL_RESOURCE", Ref: energyResource.ID.String()},
					},
				},
			},
			ComplianceRequired:   true,
			ContractRequired:     true,
			ContractCode:         "DP-ENTERPRISE-ACTIVITY",
			TraceabilityRequired: true,
			EvidenceRequired:     true,
		},
		ActorID: &seedActor,
		TraceID: traceID,
	})
	liveOK(t, err, "create live Certified Dataset CertificationProfile")

	certificationRepo := certificationinfra.NewCertificationRepository(pool)
	certificationService := certificationapp.NewCertificationService(
		tx,
		profileRepo,
		certificationRepo,
		pilotCertificationResolver{input: certificationdomain.EvaluationInput{
			WorkspaceID:      workspaceID,
			DatasetVersionID: output.ID,
			Quality:          certificationdomain.QualityAssessmentEvidence{ID: quality.ID},
			Rights: &certificationdomain.RightsEvidence{
				RightsSnapshotID:          snapshot.ID,
				EffectiveRightsSnapshotID: effectiveRights.ID,
			},
			Compliance:   &certificationdomain.ComplianceEvidence{ID: compliance.ID},
			Contract:     &certificationdomain.ContractEvidence{ID: contract.ID},
			Traceability: &certificationdomain.TraceabilityEvidence{ID: certificationEvidence.ID},
			Evidence:     &certificationdomain.EvidenceSnapshot{ID: certificationEvidence.ID},
			ActorID:      &seedActor,
		}},
	)
	certification, err := certificationService.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
		WorkspaceID:      workspaceID,
		DatasetVersionID: output.ID,
		ProfileID:        profile.ID,
		IdempotencyKey:   "live-browser-certification-" + suffix,
		ActorID:          &seedActor,
		TraceID:          traceID,
	})
	liveOK(t, err, "certify live DatasetVersion")
	if certification.Decision != certificationdomain.DecisionCertified {
		t.Fatalf("live DatasetCertification=%s blockers=%+v, want CERTIFIED", certification.Decision, certification.Blockers)
	}

	eligibility := certificationapp.NewEligibilityService(certificationService, datasetRepo, rightsinfra.NewPostgresRepository(pool))
	eligibilityResult, err := eligibility.Check(ctx, certificationapp.DeliveryEligibilityQuery{
		WorkspaceID:      workspaceID,
		DatasetVersionID: output.ID,
		ProfileID:        profile.ID,
		Consumer:         "LICENSED_BANK",
		Purpose:          purpose,
		Action:           "READ",
		Delivery:         "DIRECT_DATA",
		ScopeType:        "ALL_RESOURCE",
		ScopeRef:         output.ID.String(),
		AsOf:             time.Now().UTC(),
	})
	liveOK(t, err, "check live Certified Dataset delivery eligibility")
	if !eligibilityResult.Allowed {
		t.Fatalf("live Current Delivery Eligibility blocked: %+v", eligibilityResult.Blockers)
	}

	directData := deliveryapp.NewDirectDataService(
		tx,
		deliveryinfra.NewPostgresRepository(pool),
		deliveryapp.NewCertificationDirectDataGate(eligibility),
		datasetRepo,
	)
	delivered, err := directData.Deliver(ctx, deliveryapp.DirectDataCommand{
		WorkspaceID:          workspaceID,
		DatasetVersionID:     output.ID,
		ProfileID:            profile.ID,
		PrincipalRef:         "live-browser-certified-principal",
		EffectiveConsumerRef: "LICENSED_BANK",
		Purpose:              purpose,
		Action:               "READ",
		ScopeType:            "ALL_RESOURCE",
		ScopeRef:             output.ID.String(),
		IdempotencyKey:       "live-browser-certified-delivery-" + suffix,
		TraceID:              traceID,
	})
	liveOK(t, err, "deliver live Certified Dataset")
	if delivered.Operation.Status != deliverydomain.StatusIssued || !delivered.PayloadReady || delivered.DatasetVersion.ID != output.ID {
		t.Fatalf("live Certified Dataset delivery=%+v", delivered)
	}
	deliveredBytes := readLiveObject(t, ctx, store, delivered.DatasetVersion.StorageURI)
	deliveredHash := sha256.Sum256(deliveredBytes)
	deliveredSHA256 := hex.EncodeToString(deliveredHash[:])
	if !strings.EqualFold(deliveredSHA256, output.ChecksumValue) {
		t.Fatalf("delivered Certified Dataset checksum=%s, persisted=%s", deliveredSHA256, output.ChecksumValue)
	}
	if delivered.Operation.CertificationRef == nil || *delivered.Operation.CertificationRef != certification.ID {
		t.Fatalf("live delivery certification=%v, want %s", delivered.Operation.CertificationRef, certification.ID)
	}

	manifest["productId"], manifest["releaseId"], manifest["blockedReleaseId"] = product.ID, release.ID, blocked.ID
	manifest["executionId"], manifest["outputVersionId"] = execution.ID, output.ID
	manifest["datasetId"] = curated.ID
	manifest["qualityAssessmentId"] = quality.ID
	manifest["certificationId"] = certification.ID
	manifest["certificationProfileId"] = profile.ID
	manifest["certificationProfileName"] = profile.Name
	manifest["effectiveRightsSnapshotId"] = effectiveRights.ID
	manifest["certificationEvidenceSnapshotId"] = certificationEvidence.ID
	manifest["rightsSnapshotId"] = snapshot.ID
	manifest["deliveryOperationId"] = delivered.Operation.ID
	manifest["deliveredSha256"] = deliveredSHA256
	manifest["datasetChecksum"] = output.ChecksumValue
	manifest["qualityDimensions"] = []string{"completeness", "accuracy", "consistency", "validity", "uniqueness", "timeliness"}

	runLiveBrowser(t, ctx, "certified", manifest, artifacts)

	frozenQuery := `SELECT jsonb_build_object('productVersion', (SELECT to_jsonb(p) FROM product_version p WHERE id=$1), 'datasets', (SELECT jsonb_agg(to_jsonb(v) ORDER BY id) FROM dataset_version v WHERE id=ANY($2::uuid[])))`
	frozenIDs := []uuid.UUID{enterpriseVersion.ID, leaseVersion.ID, energyVersion.ID, *job.OutputDatasetVersionID, output.ID}
	var frozenBefore []byte
	liveOK(t, pool.QueryRow(ctx, frozenQuery, version.ID, frozenIDs).Scan(&frozenBefore), "capture immutable rows before browser publishing")
	assertLiveCount(t, ctx, pool, 0, "SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ProductReleased'", release.ID)
	livePost(t, ctx, "/api/v1/product-releases/"+blocked.ID.String()+"/publish", publisherID, uuid.NewString(), map[string]any{}, http.StatusConflict)
	runLiveBrowser(t, ctx, "publish", manifest, artifacts)

	published, err := productRepo.GetRelease(ctx, release.ID)
	liveOK(t, err, "read release after browser publishing")
	if published.Status != productdomain.ReleasePublished || published.EvidenceSnapshotID == nil {
		t.Fatalf("browser did not persist PUBLISHED and an evidence snapshot: %+v", published)
	}
	trace, err := traceability.NewRepository(pool).ProductRelease(ctx, release.ID)
	liveOK(t, err, "independent PostgreSQL trace query")
	if trace.EvidenceSnapshot == nil || !trace.EvidenceSnapshot.IntegrityValid || trace.EvidenceSnapshot.ID != *published.EvidenceSnapshotID || len(trace.EvidenceSnapshot.Items) == 0 {
		t.Fatalf("published evidence snapshot is invalid/incomplete: %+v", trace.EvidenceSnapshot)
	}
	for _, id := range frozenIDs {
		if !containsDatasetVersion(trace.DatasetVersions, id) {
			t.Fatalf("release trace omitted exact immutable input/output %s", id)
		}
	}
	if len(trace.Executions) != 1 || trace.Executions[0].ID != execution.ID || trace.Executions[0].WorkflowVersionID != workflow.ID || len(trace.CostEvents) == 0 || len(trace.Evidence) < 3 {
		t.Fatal("persisted release trace lost execution/workflow/cost/evidence facts")
	}
	for _, item := range trace.Evidence {
		if !item.IntegrityValid {
			t.Fatalf("trace evidence %s is invalid", item.ID)
		}
	}
	for _, item := range trace.DatasetVersions {
		if item.StorageURI == "" || item.ChecksumAlgorithm != "SHA256" && item.ChecksumAlgorithm != "SHA-256" && item.ChecksumAlgorithm != "sha256" {
			t.Fatalf("dataset %s has no verifiable SHA-256 storage reference", item.ID)
		}
		hash := sha256.Sum256(readLiveObject(t, ctx, store, item.StorageURI))
		if !strings.EqualFold(hex.EncodeToString(hash[:]), item.ChecksumValue) {
			t.Fatalf("real S3 bytes do not match persisted checksum for %s", item.ID)
		}
	}
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM audit_event WHERE workspace_id=$1 AND object_id=$2 AND action='PRODUCT_RELEASE_PUBLISHED' AND actor_id=$3`, workspaceID, release.ID, publisherID)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM outbox_event WHERE aggregate_type='PRODUCT_RELEASE' AND aggregate_id=$1 AND event_type='ProductReleased'`, release.ID)
	assertLiveCount(t, ctx, pool, 0, "SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ProductReleased'", blocked.ID)
	manifest["snapshotId"], manifest["rootHash"] = trace.EvidenceSnapshot.ID, trace.EvidenceSnapshot.RootHash

	// B: a published Release must show the decision that produced it, not whatever
	// entity_mapping currently points at. A post-release manual correction moves the
	// current mapping to another entity through the same immutable decision path Core
	// uses; the Release must keep its original decision id, entity, reviewer reason and
	// frozen evidence.
	currentMapping, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", enterpriseName, "ENT-005")
	liveOK(t, err, "read release-time current mapping")
	if currentMapping.CurrentDecisionID == nil || currentMapping.EvidenceID == nil {
		t.Fatalf("release-time mapping lost its decision/evidence: %+v", currentMapping)
	}
	releaseDecisionID, releaseEntityID, releaseReason := *currentMapping.CurrentDecisionID, currentMapping.EntityID, currentMapping.ReviewerReason
	correctionReason := "LIVE_POST_RELEASE_" + suffix
	correctionEntity, err := entitydomain.NewEntity(workspaceID, job.EntityTypeID, "COMPANY-LIVE-B-"+suffix, "Post-release correction entity", map[string]any{}, &seedActor)
	liveOK(t, err, "build post-release correction entity")
	correctionTx, err := pool.Begin(ctx)
	liveOK(t, err, "begin post-release correction")
	liveOK(t, entityRepo.InsertEntity(ctx, correctionTx, correctionEntity), "insert post-release correction entity")
	correctionMapping := currentMapping
	correctionMapping.EntityID = correctionEntity.ID
	correctionMapping.Status = entitydomain.MappingConfirmed
	correctionMapping.MatchMethod = "MANUAL_REVIEW"
	correctionMapping.MatchRuleID = "LIVE-CORRECTION"
	correctionMapping.ReviewerReason = correctionReason
	correctionMapping.ReviewedBy = &reviewerID
	correctionTime := time.Now().UTC()
	correctionMapping.ReviewedAt = &correctionTime
	correctionMapping.CreatedAt = correctionTime
	correctionMapping.EvidenceID = nil
	correctionDecision, err := entityRepo.RecordMappingDecision(ctx, correctionTx, entitydomain.MappingDecisionCommand{
		Mapping:                   correctionMapping,
		SourceOrigin:              entitydomain.OriginManualReview,
		IdempotencyKey:            "live-post-release:" + suffix,
		DecidedBy:                 &reviewerID,
		ExpectCurrentDecision:     true,
		ExpectedCurrentDecisionID: currentMapping.CurrentDecisionID,
	})
	if err != nil {
		_ = correctionTx.Rollback(ctx)
		t.Fatalf("record post-release correction decision: %v", err)
	}
	liveOK(t, correctionTx.Commit(ctx), "commit post-release correction")
	moved, err := entityRepo.GetMappingBySource(ctx, workspaceID, "CSV", enterpriseName, "ENT-005")
	liveOK(t, err, "read corrected current mapping")
	if moved.EntityID != correctionEntity.ID || moved.CurrentDecisionID == nil || *moved.CurrentDecisionID != correctionDecision.ID {
		t.Fatalf("current mapping did not move to the correction: %+v", moved)
	}
	trace, err = traceability.NewRepository(pool).ProductRelease(ctx, release.ID)
	liveOK(t, err, "re-query real PostgreSQL trace after post-release correction")
	if len(trace.EntityMappings) == 0 {
		t.Fatal("release trace lost entity mappings after the current mapping changed")
	}
	foundReleaseDecision := false
	for _, item := range trace.EntityMappings {
		if item.DecisionID == correctionDecision.ID {
			t.Fatalf("release trace leaked the post-release current decision: %+v", item)
		}
		if item.DecisionID == releaseDecisionID && item.EntityID == releaseEntityID && item.ReviewerReason == releaseReason {
			foundReleaseDecision = true
		}
	}
	if !foundReleaseDecision {
		t.Fatalf("release trace no longer binds the release-time decision %s: %+v", releaseDecisionID, trace.EntityMappings)
	}
	manifest["decisionId"] = releaseDecisionID
	manifest["currentMappingDecisionId"] = correctionDecision.ID
	manifest["currentMappingReason"] = correctionReason
	liveJSONFile(t, filepath.Join(artifacts, "persisted-trace.json"), trace)
	// A second browser session must see the same published history, not in-memory UI state.
	publishedBefore, _ := json.Marshal(published)
	runLiveBrowser(t, ctx, "history", manifest, artifacts)
	livePost(t, ctx, "/api/v1/product-releases/"+release.ID.String()+"/publish", publisherID, uuid.NewString(), map[string]any{}, http.StatusConflict)
	published, err = productRepo.GetRelease(ctx, release.ID)
	liveOK(t, err, "read release after fresh-session history and rejected repeat")
	publishedAfter, _ := json.Marshal(published)
	var frozenAfter []byte
	liveOK(t, pool.QueryRow(ctx, frozenQuery, version.ID, frozenIDs).Scan(&frozenAfter), "recheck immutable rows")
	if !bytes.Equal(frozenBefore, frozenAfter) || !bytes.Equal(publishedBefore, publishedAfter) {
		t.Fatal("reading published history or rejected replay mutated frozen business records")
	}
	assertLiveCount(t, ctx, pool, 1, "SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ProductReleased'", release.ID)
	liveJSONFile(t, filepath.Join(artifacts, "verification.json"), map[string]any{
		"verified": true, "workspaceId": workspaceID, "releaseId": release.ID, "evidenceSnapshotId": trace.EvidenceSnapshot.ID,
		"rootHash": trace.EvidenceSnapshot.RootHash, "reviewedCandidates": len(pending), "checksummedDatasetVersions": len(trace.DatasetVersions),
		"evidenceRecords": len(trace.Evidence), "costEvents": len(trace.CostEvents), "auditEventsInTrace": len(trace.AuditEvents),
		"production": "real native Worker via Redis", "browserPhases": []string{"review", "publish", "history"},
		"boundaries": "Preparation via actual Core application commands; not all-UI onboarding, IAM, Hop or Splink acceptance",
	})
	t.Logf("LIVE_CORE_BROWSER_VERIFIED release=%s snapshot=%s rootHash=%s", release.ID, trace.EvidenceSnapshot.ID, trace.EvidenceSnapshot.RootHash)
}

func validateLiveConfig(cfg config.Config) error {
	u, err := url.Parse(cfg.PostgresDSN)
	if err != nil || u.Host != "127.0.0.1:15432" || u.Path != "/dpp_browser_live" || u.Scheme != "postgres" {
		return fmt.Errorf("only the dedicated loopback PostgreSQL database is allowed")
	}
	if cfg.Environment != "browser-acceptance" || cfg.HTTPAddr != "127.0.0.1:18080" || cfg.Redis.Addr != "127.0.0.1:16379" || cfg.Redis.DB != 0 || cfg.Storage.Endpoint != "127.0.0.1:19000" || cfg.Storage.Bucket != "dpp-browser-live" || cfg.Storage.UseSSL || cfg.OpenMetadata.Enabled || cfg.Hop.Enabled || cfg.Splink.Enabled {
		return fmt.Errorf("live test configuration differs from the disposable local stack")
	}
	return nil
}

func liveOK(t *testing.T, err error, operation string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", operation, err)
	}
}

func startLiveProcess(t *testing.T, ctx context.Context, binary, dir, logPath string) *exec.Cmd {
	t.Helper()
	logFile, err := os.Create(logPath)
	liveOK(t, err, "open process log")
	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir, cmd.Env, cmd.Stdout, cmd.Stderr = dir, os.Environ(), logFile, logFile
	liveOK(t, cmd.Start(), "start "+filepath.Base(binary))
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
	return cmd
}

func liveWaitAPI(t *testing.T, ctx context.Context, cmd *exec.Cmd, workspace uuid.UUID) {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
			t.Fatal("Core API exited before readiness; see api.log")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, liveAPI+"/api/v1/workspaces/"+workspace.String()+"/data-products?limit=1&offset=0", nil)
		liveOK(t, err, "create readiness request")
		res, err := client.Do(req)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	t.Fatal("Core API did not become ready; see api.log")
}

func livePost(t *testing.T, ctx context.Context, path string, actor uuid.UUID, key string, body any, wantStatus int) {
	t.Helper()
	payload, err := json.Marshal(body)
	liveOK(t, err, "marshal negative-control request")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, liveAPI+path, bytes.NewReader(payload))
	liveOK(t, err, "create negative-control request")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Actor-ID", actor.String())
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	liveOK(t, err, "call actual Core negative control")
	defer response.Body.Close()
	content, _ := io.ReadAll(io.LimitReader(response.Body, 16384))
	if response.StatusCode != wantStatus {
		t.Fatalf("Core %s returned %d, want %d: %s", path, response.StatusCode, wantStatus, content)
	}
}

func assertLiveCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int, query string, args ...any) {
	t.Helper()
	var got int
	liveOK(t, pool.QueryRow(ctx, query, args...).Scan(&got), "query persisted fact count")
	if got != want {
		t.Fatalf("persisted fact count=%d, want %d: %s", got, want, query)
	}
}

func readLiveObject(t *testing.T, ctx context.Context, store interface {
	Get(context.Context, string) (io.ReadCloser, error)
}, uri string) []byte {
	t.Helper()
	r, err := store.Get(ctx, uri)
	liveOK(t, err, "read real S3 object")
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, 8<<20))
	liveOK(t, err, "read object bytes")
	return b
}

func liveJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	b, err := json.MarshalIndent(value, "", "  ")
	liveOK(t, err, "encode test artifact")
	liveOK(t, os.WriteFile(path, b, 0600), "write test artifact")
}

func runLiveBrowser(t *testing.T, ctx context.Context, phase string, manifest map[string]any, artifacts string) {
	t.Helper()
	manifestPath := filepath.Join(artifacts, "input-"+phase+".json")
	liveJSONFile(t, manifestPath, manifest)
	cmd := exec.CommandContext(ctx, "node", "node_modules/@playwright/test/cli.js", "test", "--config=playwright.config.mjs")
	cmd.Dir = repoPath(t, "tests", "live-browser")
	cmd.Env = append(os.Environ(), "LIVE_BROWSER_PHASE="+phase, "LIVE_BROWSER_MANIFEST="+manifestPath)
	output, err := cmd.CombinedOutput()
	liveOK(t, os.WriteFile(filepath.Join(artifacts, "browser-"+phase+".log"), output, 0600), "record browser log")
	t.Logf("browser %s:\n%s", phase, output)
	liveOK(t, err, "Chromium "+phase)
}
