package labelstudio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestLabelStudioLiveResultBecomesReviewedFinalizedCoreSnapshot(t *testing.T) {
	if strings.TrimSpace(os.Getenv("LIVE_BROWSER_ACCEPTANCE")) != "1" {
		t.Skip("LIVE_BROWSER_ACCEPTANCE=1 is required")
	}
	baseURL := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_URL"))
	token := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_TOKEN"))
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if baseURL == "" || token == "" || dsn == "" {
		t.Skip("TEST_LABEL_STUDIO_URL, TEST_LABEL_STUDIO_TOKEN and TEST_POSTGRES_DSN are required")
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

	httpClient := &http.Client{Timeout: 30 * time.Second}
	client, err := labelstudio.NewClient(baseURL, token, "live-core-ls", httpClient)
	if err != nil {
		t.Fatalf("new Label Studio client: %v", err)
	}

	fx := seedLiveCoreAnnotationFixture(t, ctx, pool)
	repo := annotationinfra.NewRepository(pool)
	txManager := transaction.NewManager(pool)
	service := annotationapp.NewService(txManager, repo, nil)
	engineService := annotationapp.NewEngineService(txManager, repo, client, allowLiveEngineSend{})
	reconciler := annotationapp.NewEngineResultReconciler(txManager, repo, client, service)

	campaignOperation, err := engineService.PrepareCampaign(ctx, annotationapp.PrepareEngineCampaignCommand{
		WorkspaceID: fx.workspaceID,
		CampaignID:  fx.campaignID,
		RequestID:   "live-campaign-" + uuid.NewString(),
		Title:       "Gold live Core " + fx.campaignID.String()[:8],
		ActorID:     &fx.actorID,
		TraceID:     "live-labelstudio-core",
	})
	if err != nil {
		t.Fatalf("prepare Label Studio campaign: %v", err)
	}
	campaignOperation = dispatchLiveEngineUntilMatched(t, ctx, engineService, campaignOperation.ID)
	campaignBinding, err := repo.GetEngineCampaignBinding(ctx, fx.campaignID)
	if err != nil {
		t.Fatalf("read Label Studio campaign binding: %v", err)
	}
	defer deleteProject(t, httpClient, baseURL, token, campaignBinding.ExternalProjectID)

	engineTask := annotationapp.EngineTask{
		TaskID:         fx.taskID,
		SourceItemRef:  "row:1",
		SourceSHA256:   fx.sourceSHA,
		TaskText:       fx.taskText,
		TaskTextSHA256: sha256HexString(fx.taskText),
		CorrelationKey: "live-core-" + fx.taskID.String(),
	}
	taskOperation, err := engineService.PrepareTasks(ctx, annotationapp.PrepareEngineTasksCommand{
		WorkspaceID: fx.workspaceID,
		CampaignID:  fx.campaignID,
		RequestID:   "live-tasks-" + uuid.NewString(),
		Tasks:       []annotationapp.EngineTask{engineTask},
		ActorID:     &fx.actorID,
		TraceID:     "live-labelstudio-core",
	})
	if err != nil {
		t.Fatalf("prepare Label Studio tasks: %v", err)
	}
	taskOperation = dispatchLiveEngineUntilMatched(t, ctx, engineService, taskOperation.ID)
	if taskOperation.Status != annotationdomain.EngineOperationMatched {
		t.Fatalf("task operation status=%s", taskOperation.Status)
	}

	taskBindings, err := repo.ListEngineTaskBindings(ctx, fx.campaignID)
	if err != nil || len(taskBindings) != 1 {
		t.Fatalf("task bindings=%+v err=%v", taskBindings, err)
	}
	externalTaskID := taskBindings[0].ExternalTaskID
	createAnnotation(t, httpClient, baseURL, token, externalTaskID, "EVIDENCE_SUFFICIENT")

	page, err := client.FetchResults(ctx, annotationapp.EngineLookupRequest{
		WorkspaceID: fx.workspaceID,
		CampaignID:  fx.campaignID,
		Binding: annotationapp.EngineCampaignBinding{
			Provider:          campaignBinding.Provider,
			ProviderInstance:  campaignBinding.ProviderInstance,
			ExternalProjectID: campaignBinding.ExternalProjectID,
			RequestID:         campaignBinding.RequestID,
			ConfigSHA256:      campaignBinding.ConfigSHA256,
		},
		RequestID:          taskOperation.RequestID,
		RequestFingerprint: taskOperation.RequestFingerprint,
		Tasks:              []annotationapp.EngineTask{engineTask},
	}, annotationapp.EngineResultCursor{})
	if err != nil {
		t.Fatalf("prefetch Label Studio result identity: %v", err)
	}
	if len(page.Results) != 1 || strings.TrimSpace(page.Results[0].ExternalAuthorRef) == "" {
		t.Fatalf("Label Studio result identity=%+v", page.Results)
	}
	externalAuthorRef := page.Results[0].ExternalAuthorRef

	actorBinding := annotationdomain.EngineActorBinding{
		ID:               uuid.New(),
		WorkspaceID:      fx.workspaceID,
		Provider:         labelstudio.Provider,
		ProviderInstance: "live-core-ls",
		ExternalActorRef: externalAuthorRef,
		CoreActorRef:     fx.annotatorRef,
		CreatedAt:        time.Now().UTC(),
		CreatedBy:        &fx.actorID,
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin actor binding: %v", err)
	}
	if _, err := repo.InsertEngineActorBinding(ctx, tx, actorBinding); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("persist Label Studio actor binding: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit actor binding: %v", err)
	}

	if err := reconciler.ReconcileCampaign(ctx, fx.campaignID); err != nil {
		t.Fatalf("reconcile real Label Studio result into Core: %v", err)
	}
	var result annotationdomain.Result
	var payload []byte
	if err := pool.QueryRow(ctx, `
		SELECT id, task_id, author_ref, provider_binding_ref,
		       external_task_id, external_annotation_id, external_revision,
		       canonical_payload, canonical_payload_sha256, normalizer_version
		FROM annotation_result
		WHERE campaign_id=$1
	`, fx.campaignID).Scan(
		&result.ID,
		&result.TaskID,
		&result.AuthorRef,
		&result.ProviderBindingRef,
		&result.ExternalTaskID,
		&result.ExternalAnnotationID,
		&result.ExternalRevision,
		&payload,
		&result.CanonicalPayloadSHA256,
		&result.NormalizerVersion,
	); err != nil {
		t.Fatalf("read reconciled Core result: %v", err)
	}
	result.CanonicalPayload = payload
	if result.TaskID != fx.taskID ||
		result.AuthorRef != fx.annotatorRef ||
		result.ProviderBindingRef != campaignBinding.ID.String() ||
		result.ExternalTaskID != externalTaskID ||
		result.ExternalAnnotationID == "" ||
		string(result.CanonicalPayload) != `{"label":"EVIDENCE_SUFFICIENT"}` {
		t.Fatalf("reconciled Core result=%+v payload=%s", result, string(result.CanonicalPayload))
	}

	resolver, err := platformprincipal.NewStaticResolver(
		true,
		"live-review-secret",
		"live-reviewer",
		fx.reviewerID.String(),
		[]string{fx.workspaceID.String()},
		[]string{platformprincipal.CapabilityHumanDecision},
	)
	if err != nil {
		t.Fatalf("review principal resolver: %v", err)
	}
	mux := http.NewServeMux()
	annotationhttp.NewHandler(service, resolver).Register(mux)
	body, _ := json.Marshal(map[string]any{
		"expectedTaskRevision": 2,
		"action":               "ACCEPT",
		"reason":               "live Label Studio annotation verified",
		"reviewedResultId":     result.ID.String(),
	})
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+fx.workspaceID.String()+
			"/annotation-campaigns/"+fx.campaignID.String()+
			"/tasks/"+fx.taskID.String()+"/review",
		bytes.NewReader(body),
	)
	req.Header.Set("Authorization", "Bearer live-review-secret")
	req.Header.Set("Idempotency-Key", "live-review-"+uuid.NewString())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("trusted review status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	snapshot, err := service.FinalizeAnnotationSnapshot(ctx, annotationapp.FinalizeAnnotationSnapshotCommand{
		WorkspaceID: fx.workspaceID,
		CampaignID:  fx.campaignID,
		ActorID:     &fx.reviewerID,
		TraceID:     "live-labelstudio-core",
	})
	if err != nil {
		t.Fatalf("finalize live annotation snapshot: %v", err)
	}
	if snapshot.Status != annotationdomain.SnapshotFinalized || snapshot.FinalizedAt == nil {
		t.Fatalf("snapshot=%+v", snapshot)
	}

	var frozenProviderBinding, frozenExternalTask, frozenExternalAnnotation string
	if err := pool.QueryRow(ctx, `
		SELECT r.provider_binding_ref, r.external_task_id, r.external_annotation_id
		  FROM annotation_snapshot_output so
		  JOIN annotation_result r ON r.id=so.selected_result_id
		 WHERE so.snapshot_id=$1 AND so.task_id=$2
	`, snapshot.ID, fx.taskID).Scan(
		&frozenProviderBinding, &frozenExternalTask, &frozenExternalAnnotation,
	); err != nil {
		t.Fatalf("read frozen provider provenance: %v", err)
	}
	if frozenProviderBinding != campaignBinding.ID.String() ||
		frozenExternalTask != externalTaskID ||
		frozenExternalAnnotation != result.ExternalAnnotationID {
		t.Fatalf(
			"frozen provider provenance binding/task/annotation=%s/%s/%s",
			frozenProviderBinding, frozenExternalTask, frozenExternalAnnotation,
		)
	}

	t.Logf(
		"LIVE_LABEL_STUDIO_CORE_SNAPSHOT_VERIFIED campaign=%s task=%s result=%s snapshot=%s root=%s",
		fx.campaignID, fx.taskID, result.ID, snapshot.ID, snapshot.RootHash,
	)
}

func dispatchLiveEngineUntilMatched(
	t *testing.T,
	ctx context.Context,
	service *annotationapp.EngineService,
	operationID uuid.UUID,
) annotationdomain.EngineOperation {
	t.Helper()
	var operation annotationdomain.EngineOperation
	for attempt := 0; attempt < 4; attempt++ {
		result, err := service.Dispatch(ctx, operationID, "live-test-worker", 30*time.Second)
		if err != nil {
			t.Fatalf("dispatch engine operation %s: %v", operationID, err)
		}
		operation = result.Operation
		if operation.Status == annotationdomain.EngineOperationMatched {
			return operation
		}
	}
	t.Fatalf("engine operation %s did not converge to MATCHED: %+v", operationID, operation)
	return annotationdomain.EngineOperation{}
}

type liveCoreAnnotationFixture struct {
	workspaceID  uuid.UUID
	campaignID   uuid.UUID
	taskID       uuid.UUID
	actorID      uuid.UUID
	reviewerID   uuid.UUID
	annotatorRef string
	sourceSHA    string
	taskText     string
}

func seedLiveCoreAnnotationFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
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
	taskID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("live-ls-task:"+campaignID.String()))
	actorID := uuid.New()
	reviewerID := uuid.New()
	annotatorRef := "annotator:live-labelstudio"
	sourceSHA := strings.Repeat("a", 64)
	taskText := "live evidence row"
	inputChecksum := strings.Repeat("1", 64)
	schema := `{"kind":"single-label-v1","labels":["EVIDENCE_SUFFICIENT","EVIDENCE_INSUFFICIENT","EVIDENCE_CONFLICT"]}`
	specHash := sha256HexString(schema)
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]

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
	liveSQL(t, ctx, pool, `
		INSERT INTO quality_result(
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, rule_set_content_sha256, rule_set_content,
			evaluator_name, evaluator_version
		) VALUES ($1,$2,$3,'live-input','1','PASS','{}'::jsonb,$4,'live-input','fixture','1')
	`, qualityID, workspaceID, versionID, sha256HexString("live-input"))
	profileContent := "live-labelstudio-input-profile"
	profileHash := sha256HexString(profileContent)
	liveSQL(t, ctx, pool, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state
		) VALUES (
			$1,$2,'live-input',$3,'live input','1',$4,$5,
			'ANY','ANY','ANY','ANY',true,false,false,false,false,false,'DRAFT'
		)
	`, profileID, workspaceID, "LIVE-PROFILE-"+suffix, profileHash, profileContent)
	liveSQL(t, ctx, pool, "UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1", profileID)
	liveSQL(t, ctx, pool, `
		INSERT INTO dataset_certification(
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			decision, blockers, reason, issued_at
		) VALUES (
			$1,$2,$3,$4,$5,'live-input','1',$6,$7,'CERTIFIED','[]'::jsonb,'fixture',now()
		)
	`, certificationID, workspaceID, versionID, qualityID, profileID, profileHash, profileContent)
	liveSQL(t, ctx, pool, `
		INSERT INTO annotation_campaign(
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action, consumer_ref, scope_type, scope_ref,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot,
			status, revision, expected_task_count, task_manifest_hash, input_checksum_sha256, activated_at
		) VALUES (
			$1,$2,$3,$4,$5,'GOLD-PILOT','PROCESS','live-consumer','ALL_RESOURCE',$5::text,
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7,
			'ACTIVE',2,1,$8,$9,now()
		)
	`, campaignID, workspaceID, versionID, certificationID, resourceID, specHash, schema,
		strings.Repeat("c", 64), inputChecksum)
	liveSQL(t, ctx, pool, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES ($1,$2,$3,'row:1',$4,$5,$6)
	`, taskID, workspaceID, campaignID, sourceSHA, sha256HexString(taskText), annotatorRef)

	return liveCoreAnnotationFixture{
		workspaceID:  workspaceID,
		campaignID:   campaignID,
		taskID:       taskID,
		actorID:      actorID,
		reviewerID:   reviewerID,
		annotatorRef: annotatorRef,
		sourceSHA:    sourceSHA,
		taskText:     taskText,
	}
}

func liveSQL(t *testing.T, ctx context.Context, execer *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := execer.Exec(ctx, query, args...); err != nil {
		t.Fatalf("live fixture SQL: %v", err)
	}
}
