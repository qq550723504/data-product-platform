package labelstudio_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	annotationapp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/application"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/labelstudio"
	annotationhttp "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	platformprincipal "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type allowLiveEngineSend struct{}

func (allowLiveEngineSend) ValidateEngineSendTx(
	context.Context,
	pgx.Tx,
	annotationdomain.Campaign,
) error {
	return nil
}

type liveCoreAnnotationFixture struct {
	workspaceID      uuid.UUID
	campaignID       uuid.UUID
	taskID           uuid.UUID
	sourceSHA256     string
	taskText         string
	taskTextSHA256   string
	primaryAnnotator string
}

func TestLabelStudioLiveCoreResultReviewAndSnapshot(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_URL"))
	token := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_TOKEN"))
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if baseURL == "" || token == "" || dsn == "" {
		t.Skip("TEST_LABEL_STUDIO_URL, TEST_LABEL_STUDIO_TOKEN, and TEST_POSTGRES_DSN are required")
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

	fixture := seedLiveCoreAnnotationFixture(t, ctx, pool)
	client, err := labelstudio.NewClient(
		baseURL,
		token,
		"live-core-labelstudio",
		&http.Client{Timeout: 30 * time.Second},
	)
	if err != nil {
		t.Fatalf("new Label Studio client: %v", err)
	}

	txManager := transaction.NewManager(pool)
	repo := annotationinfra.NewRepository(pool)
	annotationService := annotationapp.NewService(txManager, repo, nil)
	engineService := annotationapp.NewEngineService(
		txManager,
		repo,
		client,
		allowLiveEngineSend{},
	)
	reconciler := annotationapp.NewEngineResultReconciler(
		txManager,
		repo,
		client,
		annotationService,
	)
	runner := annotationapp.NewEngineRunner(
		repo,
		engineService,
		reconciler,
		"live-core-labelstudio-test",
		15*time.Second,
		10,
	)

	campaignOperation, err := engineService.PrepareCampaign(ctx, annotationapp.PrepareEngineCampaignCommand{
		WorkspaceID: fixture.workspaceID,
		CampaignID:  fixture.campaignID,
		RequestID:   "live-campaign-" + uuid.NewString(),
		Title:       "Live Core Gold campaign",
		TraceID:     "live-labelstudio-core",
	})
	if err != nil {
		t.Fatalf("prepare engine campaign: %v", err)
	}
	runEngineOperationToMatched(t, ctx, runner, repo, campaignOperation.ID)

	binding, err := repo.GetEngineCampaignBinding(ctx, fixture.campaignID)
	if err != nil {
		t.Fatalf("read engine campaign binding: %v", err)
	}
	defer deleteProject(t, &http.Client{Timeout: 30 * time.Second}, baseURL, token, binding.ExternalProjectID)

	engineTask := annotationapp.EngineTask{
		TaskID:         fixture.taskID,
		SourceItemRef:  "row:1",
		SourceSHA256:   fixture.sourceSHA256,
		TaskText:       fixture.taskText,
		TaskTextSHA256: fixture.taskTextSHA256,
		CorrelationKey: "core-task-" + fixture.taskID.String(),
	}
	taskOperation, err := engineService.PrepareTasks(ctx, annotationapp.PrepareEngineTasksCommand{
		WorkspaceID: fixture.workspaceID,
		CampaignID:  fixture.campaignID,
		RequestID:   "live-tasks-" + uuid.NewString(),
		Tasks:       []annotationapp.EngineTask{engineTask},
		TraceID:     "live-labelstudio-core",
	})
	if err != nil {
		t.Fatalf("prepare engine tasks: %v", err)
	}
	runEngineOperationToMatched(t, ctx, runner, repo, taskOperation.ID)

	taskBindings, err := repo.ListEngineTaskBindings(ctx, fixture.campaignID)
	if err != nil {
		t.Fatalf("read engine task bindings: %v", err)
	}
	if len(taskBindings) != 1 || taskBindings[0].TaskID != fixture.taskID {
		t.Fatalf("task bindings=%+v", taskBindings)
	}
	externalTaskID := taskBindings[0].ExternalTaskID
	createAnnotation(t, &http.Client{Timeout: 30 * time.Second}, baseURL, token, externalTaskID, "EVIDENCE_SUFFICIENT")

	lookupBinding := annotationapp.EngineCampaignBinding{
		Provider:          binding.Provider,
		ProviderInstance:  binding.ProviderInstance,
		ExternalProjectID: binding.ExternalProjectID,
		RequestID:         binding.RequestID,
		ConfigSHA256:      binding.ConfigSHA256,
	}
	page, err := client.FetchResults(ctx, annotationapp.EngineLookupRequest{
		WorkspaceID:        fixture.workspaceID,
		CampaignID:         fixture.campaignID,
		Binding:            lookupBinding,
		RequestID:          taskOperation.RequestID,
		RequestFingerprint: taskOperation.RequestFingerprint,
		Tasks:              []annotationapp.EngineTask{engineTask},
	}, annotationapp.EngineResultCursor{})
	if err != nil {
		t.Fatalf("discover provider actor for Core binding: %v", err)
	}
	if len(page.Results) != 1 || strings.TrimSpace(page.Results[0].ExternalAuthorRef) == "" {
		t.Fatalf("provider results=%+v", page.Results)
	}
	externalActorRef := page.Results[0].ExternalAuthorRef
	liveSQL(t, ctx, pool, `
		INSERT INTO annotation_engine_actor_binding(
			id, workspace_id, provider, provider_instance_ref,
			external_actor_ref, core_actor_ref, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`,
		uuid.New(),
		fixture.workspaceID,
		labelstudio.Provider,
		"live-core-labelstudio",
		externalActorRef,
		fixture.primaryAnnotator,
		uuid.New(),
	)

	if err := runner.RunOnce(ctx); err != nil {
		t.Fatalf("reconcile live Label Studio result through EngineRunner: %v", err)
	}

	var resultID uuid.UUID
	var authorRef, providerBindingRef, storedExternalTaskID, externalAnnotationID, payloadHash string
	if err := pool.QueryRow(ctx, `
		SELECT id, author_ref, provider_binding_ref, external_task_id,
		       external_annotation_id, canonical_payload_sha256
		FROM annotation_result
		WHERE task_id=$1
	`, fixture.taskID).Scan(
		&resultID,
		&authorRef,
		&providerBindingRef,
		&storedExternalTaskID,
		&externalAnnotationID,
		&payloadHash,
	); err != nil {
		t.Fatalf("read reconciled Core AnnotationResult: %v", err)
	}
	if authorRef != fixture.primaryAnnotator ||
		providerBindingRef != binding.ID.String() ||
		storedExternalTaskID != externalTaskID ||
		externalAnnotationID == "" ||
		payloadHash != sha256HexString(`{"label":"EVIDENCE_SUFFICIENT"}`) {
		t.Fatalf(
			"Core AnnotationResult provenance author=%s binding=%s task=%s annotation=%s payload=%s",
			authorRef,
			providerBindingRef,
			storedExternalTaskID,
			externalAnnotationID,
			payloadHash,
		)
	}

	var taskStatus string
	var taskRevision int64
	if err := pool.QueryRow(ctx, `
		SELECT status, revision FROM annotation_task WHERE id=$1
	`, fixture.taskID).Scan(&taskStatus, &taskRevision); err != nil {
		t.Fatalf("read reconciled task: %v", err)
	}
	if taskStatus != annotationdomain.TaskReviewable || taskRevision < 2 {
		t.Fatalf("task status/revision=%s/%d", taskStatus, taskRevision)
	}

	reviewerID := uuid.New()
	resolver, err := platformprincipal.NewStaticResolver(
		true,
		"live-core-review-secret",
		"reviewer:live-core",
		reviewerID.String(),
		[]string{fixture.workspaceID.String()},
		[]string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		t.Fatalf("review principal resolver: %v", err)
	}
	mux := http.NewServeMux()
	annotationhttp.NewHandler(annotationService, resolver).Register(mux)
	reviewBody := fmt.Sprintf(
		`{"expectedTaskRevision":%d,"action":"ACCEPT","reason":"live provider result verified","reviewedResultId":%q}`,
		taskRevision,
		resultID.String(),
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+fixture.workspaceID.String()+
			"/annotation-campaigns/"+fixture.campaignID.String()+
			"/tasks/"+fixture.taskID.String()+"/review",
		bytes.NewBufferString(reviewBody),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer live-core-review-secret")
	request.Header.Set("Idempotency-Key", "live-review-"+uuid.NewString())
	request.Header.Set("X-Actor-ID", uuid.NewString())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("trusted review status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	var reviewerRef string
	var selectedResultID uuid.UUID
	if err := pool.QueryRow(ctx, `
		SELECT reviewer_ref, selected_result_id
		FROM annotation_review_decision
		WHERE task_id=$1
	`, fixture.taskID).Scan(&reviewerRef, &selectedResultID); err != nil {
		t.Fatalf("read review decision: %v", err)
	}
	if reviewerRef != reviewerID.String() || selectedResultID != resultID {
		t.Fatalf("review decision reviewer/result=%s/%s want=%s/%s", reviewerRef, selectedResultID, reviewerID, resultID)
	}

	snapshot, err := annotationService.FinalizeAnnotationSnapshot(ctx, annotationapp.FinalizeAnnotationSnapshotCommand{
		WorkspaceID: fixture.workspaceID,
		CampaignID:  fixture.campaignID,
		ActorID:     &reviewerID,
		TraceID:     "live-labelstudio-core",
	})
	if err != nil {
		t.Fatalf("finalize live annotation snapshot: %v", err)
	}
	if snapshot.Status != annotationdomain.SnapshotFinalized ||
		snapshot.FinalizedAt == nil ||
		snapshot.ExpectedTaskCount != 1 ||
		snapshot.ExpectedResultCount != 1 ||
		snapshot.ExpectedDecisionCount != 1 ||
		snapshot.ExpectedOutputCount != 1 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	valid, err := repo.GetSnapshotIntegrity(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("verify snapshot integrity: %v", err)
	}
	if !valid {
		t.Fatal("FINALIZED live annotation snapshot failed integrity verification")
	}
}

func runEngineOperationToMatched(
	t *testing.T,
	ctx context.Context,
	runner *annotationapp.EngineRunner,
	repo *annotationinfra.Repository,
	operationID uuid.UUID,
) {
	t.Helper()
	for attempt := 0; attempt < 6; attempt++ {
		if err := runner.RunOnce(ctx); err != nil {
			t.Fatalf("run annotation EngineRunner: %v", err)
		}
		operation, err := repo.GetEngineOperation(ctx, operationID)
		if err != nil {
			t.Fatalf("read annotation engine operation: %v", err)
		}
		if operation.Status == annotationdomain.EngineOperationMatched {
			return
		}
		if operation.Status == annotationdomain.EngineOperationRejected ||
			operation.Status == annotationdomain.EngineOperationConflict ||
			operation.Status == annotationdomain.EngineOperationManualResolution {
			t.Fatalf("engine operation terminal status=%s", operation.Status)
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("engine operation %s did not converge to MATCHED", operationID)
}

func seedLiveCoreAnnotationFixture(
	t *testing.T,
	ctx context.Context,
	pool interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	},
) liveCoreAnnotationFixture {
	t.Helper()
	workspaceID := uuid.New()
	resourceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	certificationID := uuid.New()
	campaignID := uuid.New()
	taskID := uuid.New()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	inputChecksum := strings.Repeat("1", 64)
	schema := `{"kind":"single-label-v1","labels":["EVIDENCE_SUFFICIENT","EVIDENCE_INSUFFICIENT","EVIDENCE_CONFLICT"]}`
	schemaHash := sha256HexString(schema)
	taskText := "Review the evidence and choose the supported label."
	taskTextHash := sha256HexString(taskText)
	sourceHash := strings.Repeat("a", 64)
	primaryAnnotator := "annotator:live-core"

	liveSQL(t, ctx, pool, `
		INSERT INTO data_resource(id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES ($1,$2,$3,'live annotation contribution','OTHER','READY')
	`, resourceID, workspaceID, "LIVE-ANN-"+suffix)
	liveSQL(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'live annotation input','CURATED')
	`, datasetID, workspaceID, "LIVE-DS-"+suffix)
	liveSQL(t, ctx, pool, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, checksum_algorithm, checksum_value, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT','test://live-labelstudio','text/csv',1,'SHA256',$3,now())
	`, versionID, datasetID, inputChecksum)

	ruleContent := "live-labelstudio-input-quality"
	liveSQL(t, ctx, pool, `
		INSERT INTO quality_result(
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, rule_set_content_sha256, rule_set_content,
			evaluator_name, evaluator_version
		) VALUES ($1,$2,$3,'live-labelstudio-input','1','PASS',$4,$5,$6,'fixture','1')
	`, qualityID, workspaceID, versionID, []byte(`{"dimensions":{}}`), sha256HexString(ruleContent), ruleContent)

	profileContent := "live-labelstudio-input-profile"
	profileHash := sha256HexString(profileContent)
	liveSQL(t, ctx, pool, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state
		) VALUES ($1,$2,'live-labelstudio-input',$3,'live labelstudio input','1',$4,$5,
		          'ANY','ANY','ANY','ANY',true,false,false,false,false,false,'DRAFT')
	`, profileID, workspaceID, "LIVE-PROFILE-"+suffix, profileHash, profileContent)
	liveSQL(t, ctx, pool, "UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1", profileID)
	liveSQL(t, ctx, pool, `
		INSERT INTO dataset_certification(
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			decision, blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,'live-labelstudio-input','1',$6,$7,'CERTIFIED','[]'::jsonb,'fixture',now())
	`, certificationID, workspaceID, versionID, qualityID, profileID, profileHash, profileContent)

	liveSQL(t, ctx, pool, `
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
	`, campaignID, workspaceID, versionID, certificationID, resourceID, schemaHash, schema)
	liveSQL(t, ctx, pool, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES ($1,$2,$3,'row:1',$4,$5,$6)
	`, taskID, workspaceID, campaignID, sourceHash, taskTextHash, primaryAnnotator)
	liveSQL(t, ctx, pool, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=1,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("c", 64), inputChecksum)

	return liveCoreAnnotationFixture{
		workspaceID:      workspaceID,
		campaignID:       campaignID,
		taskID:           taskID,
		sourceSHA256:     sourceHash,
		taskText:         taskText,
		taskTextSHA256:   taskTextHash,
		primaryAnnotator: primaryAnnotator,
	}
}

type liveSQLExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func liveSQL(
	t *testing.T,
	ctx context.Context,
	execer liveSQLExecer,
	query string,
	args ...any,
) {
	t.Helper()
	if _, err := execer.Exec(ctx, query, args...); err != nil {
		t.Fatalf("live Core fixture SQL: %v", err)
	}
}
