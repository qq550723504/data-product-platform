package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	goldinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/gold/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type liveGoldFixture struct {
	workspaceID             uuid.UUID
	inputDatasetVersionID   uuid.UUID
	inputCertificationID    uuid.UUID
	contributionResourceID  uuid.UUID
	campaignID              uuid.UUID
	snapshotID              uuid.UUID
	outputDatasetID         uuid.UUID
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

	inputText := "Review the evidence and choose the supported label."
	inputCSV := []byte("id,text\n1," + inputText + "\n")
	inputObject := "gold-live/input-" + uuid.NewString() + ".csv"
	inputURI, err := store.Put(
		ctx,
		inputObject,
		bytes.NewReader(inputCSV),
		int64(len(inputCSV)),
		"text/csv",
	)
	if err != nil {
		t.Fatalf("put live Gold input: %v", err)
	}

	txManager := transaction.NewManager(pool)
	datasetRepo := datasetinfra.NewPostgresRepository(pool)
	annotationRepo := annotationinfra.NewRepository(pool)
	annotationService := annotationapp.NewService(txManager, annotationRepo, nil)
	fixture := seedLiveGoldSnapshotFixture(
		t,
		ctx,
		pool,
		annotationService,
		inputURI,
		inputCSV,
		inputText,
	)

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

	worker, workerLog := startLiveGoldWorker(t, ctx, workerBinary)
	outputVersionID := waitLiveGoldExecution(t, ctx, pool, worker, workerLog, execution.ID)

	outputVersion, err := datasetRepo.GetVersion(ctx, outputVersionID)
	if err != nil {
		t.Fatalf("read live Gold output version: %v", err)
	}
	if outputVersion.GeneratedByExecutionID == nil ||
		*outputVersion.GeneratedByExecutionID != execution.ID ||
		outputVersion.RowCount == nil ||
		*outputVersion.RowCount != 1 {
		t.Fatalf("Gold output producer/rows=%+v", outputVersion)
	}
	outputBytes := readLiveGoldObject(t, ctx, store, outputVersion.StorageURI)
	if got := goldTestSHA256(outputBytes); got != strings.ToLower(outputVersion.ChecksumValue) {
		t.Fatalf("Gold output checksum bytes=%s persisted=%s", got, outputVersion.ChecksumValue)
	}
	if !bytes.Contains(outputBytes, []byte("gold_label")) ||
		!bytes.Contains(outputBytes, []byte("EVIDENCE_SUFFICIENT")) {
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
}

func seedLiveGoldSnapshotFixture(
	t *testing.T,
	ctx context.Context,
	pool interface {
		Exec(context.Context, string, ...any) (interface{ RowsAffected() int64 }, error)
	},
	annotationService *annotationapp.Service,
	inputURI string,
	inputCSV []byte,
	inputText string,
) liveGoldFixture {
	t.Helper()
	workspaceID := uuid.New()
	contributionResourceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	certificationID := uuid.New()
	campaignID := uuid.New()
	taskID := uuid.New()
	resultID := uuid.New()
	outputDatasetID := uuid.New()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	inputChecksum := goldTestSHA256(inputCSV)
	schema := `{"kind":"single-label-v1","labels":["EVIDENCE_SUFFICIENT","EVIDENCE_INSUFFICIENT","EVIDENCE_CONFLICT"]}`
	schemaHash := goldTestSHA256([]byte(schema))
	taskTextHash := goldTestSHA256([]byte(inputText))
	sourcePayload, err := json.Marshal(map[string]string{"id": "1", "text": inputText})
	if err != nil {
		t.Fatalf("marshal live Gold source row: %v", err)
	}
	sourceHash := goldTestSHA256(sourcePayload)

	goldSQL(t, ctx, pool, `
		INSERT INTO data_resource(id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES ($1,$2,$3,'live Gold annotation contribution','OTHER','READY')
	`, contributionResourceID, workspaceID, "LIVE-GOLD-ANN-"+suffix)
	goldSQL(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES
			($1,$3,$4,'live Gold input','CURATED'),
			($2,$3,$5,'live Gold output','CURATED')
	`, datasetID, outputDatasetID, workspaceID, "LIVE-GOLD-IN-"+suffix, "LIVE-GOLD-OUT-"+suffix)
	goldSQL(t, ctx, pool, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, byte_size, checksum_algorithm, checksum_value, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT',$3,'text/csv',1,$4,'SHA256',$5,now())
	`, versionID, datasetID, inputURI, int64(len(inputCSV)), inputChecksum)

	ruleContent := "live-gold-input-quality"
	goldSQL(t, ctx, pool, `
		INSERT INTO quality_result(
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, rule_set_content_sha256, rule_set_content,
			evaluator_name, evaluator_version
		) VALUES ($1,$2,$3,'live-gold-input','1','PASS',$4,$5,$6,'fixture','1')
	`, qualityID, workspaceID, versionID, []byte(`{"dimensions":{}}`), goldTestSHA256([]byte(ruleContent)), ruleContent)

	profileContent := "live-gold-input-profile"
	profileHash := goldTestSHA256([]byte(profileContent))
	goldSQL(t, ctx, pool, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state
		) VALUES ($1,$2,'live-gold-input',$3,'live Gold input','1',$4,$5,
		          'ANY','ANY','ANY','ANY',true,false,false,false,false,false,'DRAFT')
	`, profileID, workspaceID, "LIVE-GOLD-PROFILE-"+suffix, profileHash, profileContent)
	goldSQL(t, ctx, pool, "UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1", profileID)
	goldSQL(t, ctx, pool, `
		INSERT INTO dataset_certification(
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			decision, blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,'live-gold-input','1',$6,$7,'CERTIFIED','[]'::jsonb,'fixture',now())
	`, certificationID, workspaceID, versionID, qualityID, profileID, profileHash, profileContent)

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
			$1,$2,$3,$4,$5,'GOLD-PILOT','PROCESS',
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7
		)
	`, campaignID, workspaceID, versionID, certificationID, contributionResourceID, schemaHash, schema)
	goldSQL(t, ctx, pool, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES ($1,$2,$3,'row:1',$4,$5,'annotator:live-gold')
	`, taskID, workspaceID, campaignID, sourceHash, taskTextHash)
	goldSQL(t, ctx, pool, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=1,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("c", 64), inputChecksum)

	payload := []byte(`{"label":"EVIDENCE_SUFFICIENT"}`)
	goldSQL(t, ctx, pool, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator:live-gold','live-provider','live-task','live-annotation','1',$5,$6,$7,'live-v1')
	`, resultID, workspaceID, campaignID, taskID, "live-gold:"+uuid.NewString(), payload, goldTestSHA256(payload))
	goldSQL(t, ctx, pool, "UPDATE annotation_task SET status='REVIEWABLE', revision=2 WHERE id=$1", taskID)

	reviewerID := uuid.New()
	if _, err := annotationService.ReviewAnnotation(ctx, annotationapp.ReviewAnnotationCommand{
		WorkspaceID:          workspaceID,
		CampaignID:           campaignID,
		TaskID:               taskID,
		ExpectedTaskRevision: 2,
		ReviewerRef:          reviewerID.String(),
		Action:               "ACCEPT",
		Reason:               "live Gold worker input accepted",
		IdempotencyKey:       "live-gold-review-" + uuid.NewString(),
		ReviewedResultID:     &resultID,
		ActorID:              &reviewerID,
		TraceID:              "live-gold-worker",
	}); err != nil {
		t.Fatalf("review live Gold input: %v", err)
	}
	snapshot, err := annotationService.FinalizeAnnotationSnapshot(ctx, annotationapp.FinalizeAnnotationSnapshotCommand{
		WorkspaceID: workspaceID,
		CampaignID:  campaignID,
		ActorID:     &reviewerID,
		TraceID:     "live-gold-worker",
	})
	if err != nil {
		t.Fatalf("finalize live Gold snapshot: %v", err)
	}

	return liveGoldFixture{
		workspaceID:            workspaceID,
		inputDatasetVersionID:  versionID,
		inputCertificationID:   certificationID,
		contributionResourceID: contributionResourceID,
		campaignID:             campaignID,
		snapshotID:             snapshot.ID,
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

func waitLiveGoldExecution(
	t *testing.T,
	ctx context.Context,
	pool interface {
		QueryRow(context.Context, string, ...any) interface {
			Scan(...any) error
		}
	},
	worker *exec.Cmd,
	workerLog string,
	executionID uuid.UUID,
) uuid.UUID {
	t.Helper()
	deadline := time.Now().Add(35 * time.Second)
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
	content, _ := os.ReadFile(workerLog)
	t.Fatalf("live Gold execution timeout\n%s", content)
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

var _ = fmt.Sprintf
