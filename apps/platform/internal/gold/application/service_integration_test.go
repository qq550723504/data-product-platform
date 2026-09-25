package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	certificationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/application"
	certificationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
	certificationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/certification/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	deliveryapp "github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	goldinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
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

type goldMemoryStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newGoldMemoryStore() *goldMemoryStore {
	return &goldMemoryStore{objects: map[string][]byte{}}
}

func (s *goldMemoryStore) Put(_ context.Context, objectName string, reader io.Reader, _ int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	uri := "mem://" + objectName
	s.mu.Lock()
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
	return uri, nil
}

func (s *goldMemoryStore) Get(_ context.Context, storageURI string) (io.ReadCloser, error) {
	s.mu.Lock()
	content := append([]byte(nil), s.objects[storageURI]...)
	s.mu.Unlock()
	return io.NopCloser(bytes.NewReader(content)), nil
}

func (s *goldMemoryStore) seed(uri string, content []byte) {
	s.mu.Lock()
	s.objects[uri] = append([]byte(nil), content...)
	s.mu.Unlock()
}

func TestGoldCandidateBuilderCreatesOneOutputBindingAndLineageOnReplay(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
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

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	annotationRepo := annotationinfra.NewRepository(pool)
	workflowRepo := workflowinfra.NewPostgresRepository(pool)
	store := newGoldMemoryStore()
	datasetWriter := datasetapp.NewUploadVersionService(txManager, datasetRepo, store)
	annotationService := annotationapp.NewService(txManager, annotationRepo, nil)
	workflowVersionService := workflowapp.NewWorkflowVersionService(txManager, workflowRepo)
	executionService := workflowapp.NewExecutionService(txManager, workflowRepo)
	goldRepo := goldinfra.NewPostgresRepository(pool)
	goldService := NewService(txManager, goldRepo, workflowRepo, annotationRepo, datasetRepo, datasetWriter, store)

	workspaceID := uuid.New()
	actorID := uuid.New()
	inputCSV := []byte("company_id,activity_level\nc1,HIGH\nc2,LOW\n")
	inputURI := "mem://gold-input.csv"
	store.seed(inputURI, inputCSV)
	inputChecksum := goldTestSHA256(inputCSV)

	resourceID := uuid.New()
	inputResourceID := uuid.New()
	inputDatasetID := uuid.New()
	inputVersionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	certificationID := uuid.New()
	campaignID := uuid.New()
	task1 := uuid.New()
	task2 := uuid.New()
	result1 := uuid.New()
	result2 := uuid.New()
	outputDatasetID := uuid.New()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	specContent := `{"kind":"single-label-v1","labels":["A","B"]}`
	specHash := goldTestSHA256([]byte(specContent))
	row1Hash, err := goldSourceRowHash(map[string]string{"company_id": "c1", "activity_level": "HIGH"})
	if err != nil {
		t.Fatalf("row1 hash: %v", err)
	}
	row2Hash, err := goldSourceRowHash(map[string]string{"company_id": "c2", "activity_level": "LOW"})
	if err != nil {
		t.Fatalf("row2 hash: %v", err)
	}

	goldSQL(t, ctx, pool, `
		INSERT INTO data_resource(id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES
			($1,$3,$4,'gold annotation contribution','OTHER','READY'),
			($2,$3,$5,'gold input source','TABLE_LIKE','READY')
	`, resourceID, inputResourceID, workspaceID, "GOLD-ANN-"+suffix, "GOLD-SRC-"+suffix)
	goldSQL(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type, source_resource_id)
		VALUES ($1,$2,$3,'gold input','CURATED',$4)
	`, inputDatasetID, workspaceID, "GOLD-IN-"+suffix, inputResourceID)
	goldSQL(t, ctx, pool, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, byte_size, checksum_algorithm, checksum_value, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT',$3,'text/csv',2,$4,'SHA256',$5,now())
	`, inputVersionID, inputDatasetID, inputURI, int64(len(inputCSV)), inputChecksum)

	ruleContent := "gold-builder-input-quality"
	goldSQL(t, ctx, pool, `
		INSERT INTO quality_result(
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, rule_set_content_sha256, rule_set_content,
			evaluator_name, evaluator_version
		) VALUES ($1,$2,$3,'gold-builder-input','1','PASS',$4,$5,$6,'fixture','1')
	`, qualityID, workspaceID, inputVersionID, []byte(`{"dimensions":{}}`), goldTestSHA256([]byte(ruleContent)), ruleContent)

	profileContent := "gold-builder-input-profile"
	profileHash := goldTestSHA256([]byte(profileContent))
	goldSQL(t, ctx, pool, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state
		) VALUES ($1,$2,'gold-builder-input',$3,'gold builder input','1',$4,$5,
		          'ANY','ANY','ANY','ANY',true,false,false,false,false,false,'DRAFT')
	`, profileID, workspaceID, "GBP-"+suffix, profileHash, profileContent)
	goldSQL(t, ctx, pool, "UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1", profileID)
	goldSQL(t, ctx, pool, `
		INSERT INTO dataset_certification(
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			decision, blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,'gold-builder-input','1',$6,$7,'CERTIFIED','[]'::jsonb,'fixture',now())
	`, certificationID, workspaceID, inputVersionID, qualityID, profileID, profileHash, profileContent)

	goldSQL(t, ctx, pool, `
		INSERT INTO annotation_campaign(
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot
		) VALUES (
			$1,$2,$3,$4,$5,'gold-pilot','PROCESS',
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7
		)
	`, campaignID, workspaceID, inputVersionID, certificationID, resourceID, specHash, specContent)
	goldSQL(t, ctx, pool, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES
			($1,$3,$4,'row:1',$5,$6,'annotator'),
			($2,$3,$4,'row:2',$7,$8,'annotator')
	`, task1, task2, workspaceID, campaignID, row1Hash, strings.Repeat("b", 64), row2Hash, strings.Repeat("d", 64))
	goldSQL(t, ctx, pool, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=2,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("e", 64), inputChecksum)

	payloadA := []byte(`{"label":"A"}`)
	goldSQL(t, ctx, pool, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES
			($1,$3,$4,$5,'annotator','fixture-provider','task-1','ann-1','1',$7,$8,$9,'fixture-v1'),
			($2,$3,$4,$6,'annotator','fixture-provider','task-2','ann-2','1',$10,$8,$9,'fixture-v1')
	`, result1, result2, workspaceID, campaignID, task1, task2,
		"gold-result:"+uuid.NewString(), payloadA, goldTestSHA256(payloadA), "gold-result:"+uuid.NewString())
	goldSQL(t, ctx, pool, "UPDATE annotation_task SET status='REVIEWABLE', revision=2 WHERE campaign_id=$1", campaignID)

	if _, err := annotationService.ReviewAnnotation(ctx, annotationapp.ReviewAnnotationCommand{
		WorkspaceID: workspaceID, CampaignID: campaignID, TaskID: task1,
		ExpectedTaskRevision: 2, ReviewerRef: actorID.String(), Action: annotationdomain.ReviewAccept,
		Reason: "accept", IdempotencyKey: "accept-" + uuid.NewString(), ReviewedResultID: &result1,
		ActorID: &actorID, TraceID: "gold-builder-test",
	}); err != nil {
		t.Fatalf("accept task: %v", err)
	}
	corrected := []byte(`{"label":"B"}`)
	if _, err := annotationService.ReviewAnnotation(ctx, annotationapp.ReviewAnnotationCommand{
		WorkspaceID: workspaceID, CampaignID: campaignID, TaskID: task2,
		ExpectedTaskRevision: 2, ReviewerRef: actorID.String(), Action: annotationdomain.ReviewCorrect,
		Reason: "correct", IdempotencyKey: "correct-" + uuid.NewString(), ReviewedResultID: &result2,
		CorrectedPayload: corrected, CorrectedPayloadHash: goldTestSHA256(corrected),
		ActorID: &actorID, TraceID: "gold-builder-test",
	}); err != nil {
		t.Fatalf("correct task: %v", err)
	}
	snapshot, err := annotationService.FinalizeAnnotationSnapshot(ctx, annotationapp.FinalizeAnnotationSnapshotCommand{
		WorkspaceID: workspaceID, CampaignID: campaignID, ActorID: &actorID, TraceID: "gold-builder-test",
	})
	if err != nil {
		t.Fatalf("finalize annotation snapshot: %v", err)
	}

	goldSQL(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'gold candidate output','CURATED')
	`, outputDatasetID, workspaceID, "GOLD-OUT-"+suffix)

	workflowVersion, err := workflowVersionService.Create(ctx, workflowapp.CreateWorkflowVersionCommand{
		WorkspaceID:    workspaceID,
		Code:           "GOLD-BUILDER-" + suffix,
		Name:           "Gold builder",
		Version:        "1.0.0",
		DefinitionRef:  "test/gold-builder.yaml",
		DefinitionYAML: []byte("spec:\n  processor: GOLD_DATASET_BUILDER_V1\n  execution:\n    requiresTargetPeriod: false\n"),
		ActorID:        &actorID,
		TraceID:        "gold-builder-test",
	})
	if err != nil {
		t.Fatalf("create workflow version: %v", err)
	}

	command := CreateBuildCommand{
		WorkspaceID:                      workspaceID,
		WorkflowVersionID:                workflowVersion.ID,
		OutputDatasetID:                  outputDatasetID,
		InputDatasetVersionID:            inputVersionID,
		InputCertificationID:             certificationID,
		AnnotationCampaignID:             campaignID,
		AnnotationSnapshotID:             snapshot.ID,
		AnnotationContributionResourceID: resourceID,
		IdempotencyKey:                   "gold-build-" + uuid.NewString(),
		ActorID:                          &actorID,
		TraceID:                          "gold-builder-test",
	}
	execution, err := goldService.CreateBuild(ctx, command)
	if err != nil {
		t.Fatalf("create build: %v", err)
	}
	replay, err := goldService.CreateBuild(ctx, command)
	if err != nil {
		t.Fatalf("replay build: %v", err)
	}
	if replay.ID != execution.ID {
		t.Fatalf("replay execution=%s want=%s", replay.ID, execution.ID)
	}
	if execution.TargetPeriod != "" {
		t.Fatalf("target period=%q want empty", execution.TargetPeriod)
	}

	started, err := executionService.StartWithReferenceCheck(ctx, execution.ID, "native:"+execution.ID.String(), "gold-builder-test")
	if err != nil {
		t.Fatalf("start execution: %v", err)
	}
	request := workflowapp.ProcessingRequestFromExecution(started, workflowVersion)
	result, err := goldService.Execute(ctx, request)
	if err != nil {
		t.Fatalf("execute Gold build: %v", err)
	}
	if _, err := executionService.Succeed(ctx, execution.ID, result.OutputDatasetVersionID, result.Metrics, "gold-builder-test"); err != nil {
		t.Fatalf("succeed execution: %v", err)
	}

	output, err := datasetRepo.GetVersion(ctx, result.OutputDatasetVersionID)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if output.Status != datasetdomain.VersionReady || output.RowCount == nil || *output.RowCount != 2 {
		t.Fatalf("output status/rows=%s/%v", output.Status, output.RowCount)
	}
	reader, err := store.Get(ctx, output.StorageURI)
	if err != nil {
		t.Fatalf("read output object: %v", err)
	}
	outputBytes, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read output bytes: %v", err)
	}
	text := string(outputBytes)
	if !strings.Contains(text, "gold_label,annotation_result_id,review_decision_id") ||
		!strings.Contains(text, "c1,HIGH,A,") || !strings.Contains(text, "c2,LOW,B,") {
		t.Fatalf("unexpected Gold output:\n%s", text)
	}

	binding, err := goldRepo.GetBindingByExecution(ctx, execution.ID)
	if err != nil {
		t.Fatalf("read binding: %v", err)
	}
	if binding.Status != "FINALIZED" || binding.OutputDatasetVersionID != output.ID ||
		binding.AnnotationSnapshotID != snapshot.ID || binding.OutputRowCount != 2 {
		t.Fatalf("binding=%+v", binding)
	}

	explanation, err := readmodel.NewRepository(pool).GoldExplanation(ctx, workspaceID, output.ID)
	if err != nil {
		t.Fatalf("read Gold explanation projection: %v", err)
	}
	if explanation.InputDatasetVersionID != inputVersionID ||
		explanation.InputCertificationID != certificationID ||
		explanation.ExecutionID != execution.ID ||
		explanation.GoldProductionBindingID != binding.ID ||
		explanation.Campaign.ID != campaignID ||
		explanation.Snapshot.ID != snapshot.ID {
		t.Fatalf("Gold explanation identity mismatch: %+v", explanation)
	}
	if explanation.Campaign.TaskCount != 2 ||
		explanation.Campaign.ReviewDecisionCount != 2 ||
		explanation.Campaign.SelectedOutputCount != 2 ||
		len(explanation.Reviews) != 2 {
		t.Fatalf("Gold explanation counts/reviews=%+v", explanation)
	}
	for _, review := range explanation.Reviews {
		if review.ReviewerRef != actorID.String() ||
			strings.TrimSpace(review.Reason) == "" ||
			review.SelectedResultID == nil ||
			strings.TrimSpace(review.SelectedResultSHA256) == "" {
			t.Fatalf("Gold explanation review provenance=%+v", review)
		}
	}
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM gold_build_request WHERE execution_id=$1", execution.ID)
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM gold_production_binding WHERE execution_id=$1", execution.ID)
	goldCount(t, ctx, pool, 2, "SELECT count(*) FROM gold_production_member WHERE binding_id=$1", binding.ID)
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM dataset_version WHERE dataset_id=$1 AND generated_by_execution_id=$2", outputDatasetID, execution.ID)
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM dataset_version_lineage WHERE output_version_id=$1 AND input_version_id=$2 AND relation_type='DERIVED_FROM'", output.ID, inputVersionID)

	replayedResult, err := goldService.Execute(ctx, request)
	if err != nil {
		t.Fatalf("replay execute: %v", err)
	}
	if replayedResult.OutputDatasetVersionID != output.ID {
		t.Fatalf("replayed output=%s want=%s", replayedResult.OutputDatasetVersionID, output.ID)
	}
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM gold_production_binding WHERE execution_id=$1", execution.ID)
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM dataset_version WHERE dataset_id=$1 AND generated_by_execution_id=$2", outputDatasetID, execution.ID)

	qualityRepo := qualityinfra.NewPostgresRepository(pool)
	qualityService := qualityapp.NewService(
		"", txManager, datasetRepo, qualityRepo, store, evidence.NewQueryRepository(pool),
	).ConfigureGold(goldRepo, annotationService)
	assessmentAttemptID := uuid.New()
	assessment, err := qualityService.RunGold(ctx, qualityapp.GoldRunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    output.ID,
		AssessmentAttemptID: assessmentAttemptID,
		ActorID:             &actorID,
		TraceID:             "gold-builder-formal-quality",
	})
	if err != nil {
		t.Fatalf("run formal Gold quality: %v", err)
	}
	if assessment.DatasetVersionID != output.ID || assessment.GateDecision != "PASS" {
		t.Fatalf("formal Gold assessment target/gate=%s/%s want=%s/PASS", assessment.DatasetVersionID, assessment.GateDecision, output.ID)
	}
	if assessment.RuleSetRef != qualityapp.GoldRuleSetRef ||
		assessment.EvaluatorName != qualityapp.GoldEvaluatorName {
		t.Fatalf("formal Gold rule/evaluator=%s/%s", assessment.RuleSetRef, assessment.EvaluatorName)
	}
	if assessment.Metrics["productionBindingId"] != binding.ID.String() ||
		assessment.Metrics["annotationSnapshotId"] != snapshot.ID.String() ||
		assessment.Metrics["formalAssessment"] != true {
		t.Fatalf("formal Gold assessment metrics=%+v", assessment.Metrics)
	}
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM quality_result WHERE id=$1 AND dataset_version_id=$2", assessment.ID, output.ID)
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM quality_finding WHERE result_id=$1 AND rule_id='GOLD-OUTPUT-COUNT'", assessment.ID)

	replayedAssessment, err := qualityService.RunGold(ctx, qualityapp.GoldRunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    output.ID,
		AssessmentAttemptID: assessmentAttemptID,
		ActorID:             &actorID,
		TraceID:             "gold-builder-formal-quality-replay",
	})
	if err != nil {
		t.Fatalf("replay formal Gold quality: %v", err)
	}
	if replayedAssessment.ID != assessment.ID {
		t.Fatalf("replayed assessment=%s want=%s", replayedAssessment.ID, assessment.ID)
	}
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM quality_result WHERE id=$1", assessment.ID)

	_, err = qualityService.RunGold(ctx, qualityapp.GoldRunCommand{
		WorkspaceID:         workspaceID,
		DatasetVersionID:    inputVersionID,
		AssessmentAttemptID: uuid.New(),
		ActorID:             &actorID,
		TraceID:             "non-gold-formal-quality-must-fail",
	})
	if !errors.Is(err, qualityapp.ErrGoldProductionProof) {
		t.Fatalf("non-Gold DatasetVersion formal assessment error=%v, want Gold production proof failure", err)
	}
	goldCount(t, ctx, pool, 0, `
		SELECT count(*) FROM quality_result
		WHERE dataset_version_id=$1 AND rule_set_ref=$2
	`, inputVersionID, qualityapp.GoldRuleSetRef)

	rightsRepo := rightsinfra.NewPostgresRepository(pool)
	requiredResources, err := rightsRepo.RequiredLineageInputs(ctx, output.ID)
	if err != nil {
		t.Fatalf("resolve Gold required rights resources: %v", err)
	}
	if len(requiredResources) != 2 {
		t.Fatalf("required Gold resources=%+v want 2", requiredResources)
	}
	var sawDataset, sawAnnotationContribution bool
	for _, required := range requiredResources {
		switch required.DependencyKind {
		case rightsdomain.EffectiveDependencyDatasetVersion:
			if required.DatasetVersionID != inputVersionID || required.DataResourceID != inputResourceID || !required.ResourceMapped {
				t.Fatalf("dataset required resource=%+v", required)
			}
			sawDataset = true
		case rightsdomain.EffectiveDependencyAnnotationContribution:
			if required.DatasetVersionID != uuid.Nil || required.DataResourceID != resourceID || !required.ResourceMapped {
				t.Fatalf("annotation contribution required resource=%+v", required)
			}
			sawAnnotationContribution = true
		default:
			t.Fatalf("unexpected Gold dependency kind %q", required.DependencyKind)
		}
	}
	if !sawDataset || !sawAnnotationContribution {
		t.Fatalf("typed Gold required resources missing dataset=%v annotation=%v", sawDataset, sawAnnotationContribution)
	}

	rightsService := rightsapp.NewService(txManager, rightsRepo)
	effectiveRights, err := rightsService.ComputeEffectiveRights(ctx, rightsapp.ComputeEffectiveRightsCommand{
		WorkspaceID:            workspaceID,
		TargetDatasetVersionID: output.ID,
		ConsumerRef:            "GOLD-PILOT-CONSUMER",
		Purpose:                "GOLD-PILOT",
		AsOf:                   time.Now().UTC(),
		AsOfProvided:           true,
		ActivityID:             ptrUUID(uuid.New()),
		ActorID:                &actorID,
		TraceID:                "gold-required-resources",
	})
	if err != nil {
		t.Fatalf("compute Gold effective rights: %v", err)
	}
	if effectiveRights.Status != "FINALIZED" || len(effectiveRights.Inputs) != 2 {
		t.Fatalf("effective Gold rights status/inputs=%s/%+v", effectiveRights.Status, effectiveRights.Inputs)
	}
	for _, action := range effectiveRights.Actions {
		if action.Decision != rightsdomain.DecisionNotAllowed || action.BlockingInputID == nil {
			t.Fatalf("Gold effective rights action should fail closed without provenance: %+v", action)
		}
	}
	goldCount(t, ctx, pool, 1, `
		SELECT count(*)
		FROM effective_rights_input
		WHERE snapshot_id=$1
		  AND dependency_kind='ANNOTATION_CONTRIBUTION_RESOURCE'
		  AND input_dataset_version_id IS NULL
		  AND data_resource_id=$2
	`, effectiveRights.ID, resourceID)

	var sourceRightsDeclarationID, annotationRightsDeclarationID uuid.UUID
	for _, rightsResourceID := range []uuid.UUID{inputResourceID, resourceID} {
		declaration, err := rightsService.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{
			Spec: rightsdomain.RightsDeclarationSpec{
				WorkspaceID:       workspaceID,
				DataResourceID:    rightsResourceID,
				ClaimantRef:       "GOLD-RIGHTS-HOLDER",
				BasisType:         "LICENSE",
				BasisRef:          "gold-certification-test",
				ConsumerScopeType: "EXPLICIT",
				ConsumerRef:       "GOLD-PILOT-CONSUMER",
				Parties: []rightsdomain.RightsParty{{
					PartyRef: "GOLD-RIGHTS-HOLDER",
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
			TraceID: "gold-certification-rights",
		})
		if err != nil {
			t.Fatalf("create Gold rights declaration for %s: %v", rightsResourceID, err)
		}
		if _, err := rightsService.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{
			DeclarationID: declaration.ID,
			Outcome:       rightsdomain.DeclarationVerified,
			ActorID:       &actorID,
			TraceID:       "gold-certification-rights",
		}); err != nil {
			t.Fatalf("verify Gold rights declaration for %s: %v", rightsResourceID, err)
		}
		if rightsResourceID == inputResourceID {
			sourceRightsDeclarationID = declaration.ID
		} else if rightsResourceID == resourceID {
			annotationRightsDeclarationID = declaration.ID
		}
	}
	if sourceRightsDeclarationID == uuid.Nil || annotationRightsDeclarationID == uuid.Nil {
		t.Fatalf("Gold rights declaration identities source=%s annotation=%s", sourceRightsDeclarationID, annotationRightsDeclarationID)
	}

	allowedRights, err := rightsService.ComputeEffectiveRights(ctx, rightsapp.ComputeEffectiveRightsCommand{
		WorkspaceID:            workspaceID,
		TargetDatasetVersionID: output.ID,
		ConsumerRef:            "GOLD-PILOT-CONSUMER",
		Purpose:                "GOLD-PILOT",
		AsOf:                   time.Now().UTC(),
		AsOfProvided:           true,
		ActivityID:             ptrUUID(uuid.New()),
		ActorID:                &actorID,
		TraceID:                "gold-certification-rights-allowed",
	})
	if err != nil {
		t.Fatalf("compute allowed Gold effective rights: %v", err)
	}
	var useAllowed bool
	for _, action := range allowedRights.Actions {
		if action.Action == "USE" {
			useAllowed = action.Decision == rightsdomain.DecisionAllowed
		}
	}
	if !useAllowed {
		t.Fatalf("Gold EffectiveRights USE must be ALLOWED: %+v", allowedRights.Actions)
	}

	var certificationEvidence evidence.Snapshot
	evidenceTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin Gold certification evidence tx: %v", err)
	}
	evidenceRecord, err := evidence.Append(ctx, evidenceTx, evidence.Record{
		WorkspaceID:  workspaceID,
		EvidenceType: "GOLD_CERTIFICATION_INPUT",
		Title:        "Gold certification frozen input",
		SourceType:   "DATASET_VERSION",
		SourceID:     &output.ID,
		Metadata: map[string]any{
			"goldProductionBindingId":   binding.ID,
			"annotationSnapshotId":      snapshot.ID,
			"qualityAssessmentId":       assessment.ID,
			"effectiveRightsSnapshotId": allowedRights.ID,
		},
		CreatedBy: &actorID,
	}, evidence.Relation{
		ObjectType:   "DATASET_VERSION",
		ObjectID:     output.ID,
		RelationType: "CERTIFICATION_INPUT",
	})
	if err != nil {
		_ = evidenceTx.Rollback(ctx)
		t.Fatalf("append Gold certification evidence: %v", err)
	}
	certificationEvidence, err = evidence.CreateSnapshot(
		ctx,
		evidenceTx,
		workspaceID,
		"DATASET_VERSION",
		output.ID,
		map[string]any{
			"goldProductionBindingId": binding.ID,
			"annotationSnapshotId":    snapshot.ID,
		},
		[]evidence.SnapshotItem{{
			EvidenceID: evidenceRecord.ID,
			Category:   "GOLD_CERTIFICATION_INPUT",
		}},
		&actorID,
	)
	if err != nil {
		_ = evidenceTx.Rollback(ctx)
		t.Fatalf("create Gold certification EvidenceSnapshot: %v", err)
	}
	if err := evidenceTx.Commit(ctx); err != nil {
		t.Fatalf("commit Gold certification evidence: %v", err)
	}

	goldProfileSpec, err := certificationdomain.NewGoldCertificationProfile(
		"GOLD-PILOT",
		"USE",
		"GOLD-PILOT-CONSUMER",
		[]certificationdomain.ScopeRef{
			{Type: "ALL_RESOURCE", Ref: inputResourceID.String()},
			{Type: "ALL_RESOURCE", Ref: resourceID.String()},
		},
	)
	if err != nil {
		t.Fatalf("build Gold CertificationProfile: %v", err)
	}
	profileRepo := certificationinfra.NewProfileRepository(pool)
	profileService := certificationapp.NewProfileService(txManager, profileRepo)
	goldProfile, err := profileService.Create(ctx, certificationapp.CreateProfileCommand{
		WorkspaceID: workspaceID,
		Profile:     goldProfileSpec,
		ActorID:     &actorID,
		TraceID:     "gold-certification-profile",
	})
	if err != nil {
		t.Fatalf("create Gold CertificationProfile: %v", err)
	}

	certificationRepo := certificationinfra.NewCertificationRepository(pool)
	certificationService := certificationapp.NewCertificationService(
		txManager,
		profileRepo,
		certificationRepo,
		certificationapp.NewReferenceEvidenceResolver(),
	)
	certificationKey := "gold-certification-" + uuid.NewString()
	certification, err := certificationService.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
		WorkspaceID:               workspaceID,
		DatasetVersionID:          output.ID,
		ProfileID:                 goldProfile.ID,
		QualityAssessmentID:       assessment.ID,
		EffectiveRightsSnapshotID: &allowedRights.ID,
		EvidenceSnapshotID:        &certificationEvidence.ID,
		IdempotencyKey:            certificationKey,
		ActorID:                   &actorID,
		TraceID:                   "gold-certification",
		Now:                       time.Now().UTC(),
		CostActivity: &cost.CertificationActivity{
			ActivityID:  uuid.New(),
			Quantity:    1,
			Unit:        "certification",
			PricingMode: "ACTUAL",
			Metadata:    map[string]any{"phase": "gold-certification"},
		},
	})
	if err != nil {
		t.Fatalf("evaluate Gold certification: %v", err)
	}
	if certification.Decision != certificationdomain.DecisionCertified {
		t.Fatalf("Gold certification decision=%s blockers=%+v", certification.Decision, certification.Blockers)
	}
	if certification.GoldProductionBindingID == nil || *certification.GoldProductionBindingID != binding.ID ||
		certification.AnnotationSnapshotID == nil || *certification.AnnotationSnapshotID != snapshot.ID ||
		certification.AnnotationSnapshotRootHash != snapshot.RootHash ||
		certification.AnnotationSchemaSHA256 != binding.SchemaContentSHA256 ||
		certification.AnnotationTaxonomySHA256 != binding.TaxonomyContentSHA256 ||
		certification.GoldProductionBindingRootHash != binding.RootHash {
		t.Fatalf("Gold certification proof mismatch: %+v", certification)
	}
	goldCount(t, ctx, pool, 1, `
		SELECT count(*)
		FROM dataset_certification
		WHERE id=$1
		  AND dataset_version_id=$2
		  AND gold_production_binding_id=$3
		  AND annotation_snapshot_id=$4
	`, certification.ID, output.ID, binding.ID, snapshot.ID)

	replayedCertification, err := certificationService.Evaluate(ctx, certificationapp.EvaluateDatasetCertificationCommand{
		WorkspaceID:               workspaceID,
		DatasetVersionID:          output.ID,
		ProfileID:                 goldProfile.ID,
		QualityAssessmentID:       assessment.ID,
		EffectiveRightsSnapshotID: &allowedRights.ID,
		EvidenceSnapshotID:        &certificationEvidence.ID,
		IdempotencyKey:            certificationKey,
		ActorID:                   &actorID,
		TraceID:                   "gold-certification-replay",
		Now:                       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("replay Gold certification: %v", err)
	}
	if replayedCertification.ID != certification.ID {
		t.Fatalf("replayed certification=%s want=%s", replayedCertification.ID, certification.ID)
	}
	goldCount(t, ctx, pool, 1, "SELECT count(*) FROM dataset_certification WHERE id=$1", certification.ID)

	eligibility := certificationapp.NewEligibilityService(certificationService, datasetRepo, rightsRepo)
	directGate := deliveryapp.NewCertificationDirectDataGate(eligibility)
	gateTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin Gold DIRECT_DATA gate tx: %v", err)
	}
	gateResult, err := directGate.EvaluateDirectData(ctx, gateTx, deliveryapp.DirectDataGateRequest{
		OperationID:          uuid.New(),
		WorkspaceID:          workspaceID,
		DatasetVersionID:     output.ID,
		ProfileID:            goldProfile.ID,
		PrincipalRef:         "gold-principal",
		EffectiveConsumerRef: "GOLD-PILOT-CONSUMER",
		Purpose:              "GOLD-PILOT",
		Action:               "USE",
		ScopeType:            "ALL_RESOURCE",
		ScopeRef:             output.ID.String(),
		DeliveryChannel:      "DIRECT_DATA",
		DeliveryMode:         "DIRECT_DATA",
		RequestedExpiresAt:   time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		_ = gateTx.Rollback(ctx)
		t.Fatalf("evaluate Gold DIRECT_DATA gate: %v", err)
	}
	if !gateResult.Evaluation.Allowed ||
		gateResult.CertificationRef == nil ||
		*gateResult.CertificationRef != certification.ID {
		_ = gateTx.Rollback(ctx)
		t.Fatalf("Gold DIRECT_DATA gate=%+v certification=%v", gateResult.Evaluation, gateResult.CertificationRef)
	}
	if err := gateTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback read-only Gold gate tx: %v", err)
	}

	if _, err := rightsService.DisposeRightsDeclaration(ctx, rightsapp.DisposeRightsDeclarationCommand{
		DeclarationID: annotationRightsDeclarationID,
		Disposition:   rightsdomain.DispositionInvalidated,
		EffectiveAt:   time.Now().UTC(),
		Reason:        "annotation contribution rights withdrawn",
		ActorID:       &actorID,
		TraceID:       "gold-delivery-rights-revoked",
	}); err != nil {
		t.Fatalf("invalidate annotation contribution rights: %v", err)
	}

	blockedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocked Gold DIRECT_DATA gate tx: %v", err)
	}
	blockedGate, err := directGate.EvaluateDirectData(ctx, blockedTx, deliveryapp.DirectDataGateRequest{
		OperationID:          uuid.New(),
		WorkspaceID:          workspaceID,
		DatasetVersionID:     output.ID,
		ProfileID:            goldProfile.ID,
		PrincipalRef:         "gold-principal",
		EffectiveConsumerRef: "GOLD-PILOT-CONSUMER",
		Purpose:              "GOLD-PILOT",
		Action:               "USE",
		ScopeType:            "ALL_RESOURCE",
		ScopeRef:             output.ID.String(),
		DeliveryChannel:      "DIRECT_DATA",
		DeliveryMode:         "DIRECT_DATA",
		RequestedExpiresAt:   time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		_ = blockedTx.Rollback(ctx)
		t.Fatalf("reevaluate Gold DIRECT_DATA gate after rights revocation: %v", err)
	}
	if blockedGate.Evaluation.Allowed {
		_ = blockedTx.Rollback(ctx)
		t.Fatalf("Gold DIRECT_DATA remained allowed after annotation rights revocation: %+v", blockedGate.Evaluation)
	}
	var entitlementBlocked bool
	for _, blocker := range blockedGate.Evaluation.Blockers {
		if blocker == "CURRENT_ENTITLEMENT_BLOCKED" {
			entitlementBlocked = true
		}
	}
	if !entitlementBlocked {
		_ = blockedTx.Rollback(ctx)
		t.Fatalf("Gold DIRECT_DATA blockers=%v want CURRENT_ENTITLEMENT_BLOCKED", blockedGate.Evaluation.Blockers)
	}
	if err := blockedTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback blocked Gold gate tx: %v", err)
	}

	finalExplanation, err := readmodel.NewRepository(pool).GoldExplanation(ctx, workspaceID, output.ID)
	if err != nil {
		t.Fatalf("read completed Gold trace explanation: %v", err)
	}
	for _, phase := range []string{"HUMAN_REVIEW", "GOLD_BUILD", "GOLD_QUALITY", "GOLD_CERTIFICATION"} {
		if !goldTraceHasCostPhase(finalExplanation.Trace.Costs, phase) {
			t.Fatalf("Gold trace missing cost phase %s: %+v", phase, finalExplanation.Trace.Costs)
		}
	}
	for _, phase := range []string{"HUMAN_REVIEW", "SNAPSHOT", "GOLD_BUILD", "GOLD_QUALITY", "GOLD_CERTIFICATION"} {
		if !goldTraceHasEvidencePhase(finalExplanation.Trace.Evidence, phase) {
			t.Fatalf("Gold trace missing evidence phase %s: %+v", phase, finalExplanation.Trace.Evidence)
		}
	}
	for _, action := range []string{
		"ANNOTATION_REVIEWED",
		"ANNOTATION_SNAPSHOT_FINALIZED",
		"GOLD_PRODUCTION_BINDING_FINALIZED",
		"GOLD_QUALITY_ASSESSMENT_COMPLETED",
		"DATASET_CERTIFIED",
	} {
		if !goldTraceHasAuditAction(finalExplanation.Trace.Audit, action) {
			t.Fatalf("Gold trace missing audit action %s: %+v", action, finalExplanation.Trace.Audit)
		}
	}
}

func goldSQL(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("fixture SQL: %v", err)
	}
}

func goldCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, want int, query string, args ...any) {
	t.Helper()
	var got int
	if err := pool.QueryRow(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != want {
		t.Fatalf("count=%d want=%d query=%s", got, want, query)
	}
}

func goldTestSHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func ptrUUID(value uuid.UUID) *uuid.UUID {
	return &value
}

func goldTraceHasCostPhase(items []readmodel.GoldCostReference, phase string) bool {
	for _, item := range items {
		if item.Phase == phase {
			return true
		}
	}
	return false
}

func goldTraceHasEvidencePhase(items []readmodel.GoldEvidenceReference, phase string) bool {
	for _, item := range items {
		if item.Phase == phase {
			return true
		}
	}
	return false
}

func goldTraceHasAuditAction(items []readmodel.GoldAuditReference, action string) bool {
	for _, item := range items {
		if item.Action == action {
			return true
		}
	}
	return false
}
