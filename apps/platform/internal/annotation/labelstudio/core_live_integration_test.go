package labelstudio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
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

type liveCoreAnnotationTaskFixture struct {
	taskID         uuid.UUID
	sourceItemRef  string
	sourceSHA256   string
	taskText       string
	taskTextSHA256 string
	providerLabel  string
	finalLabel     string
}

type liveCoreAnnotationFixture struct {
	workspaceID            uuid.UUID
	inputResourceID        uuid.UUID
	inputDatasetVersionID  uuid.UUID
	inputCertificationID   uuid.UUID
	contributionResourceID uuid.UUID
	campaignID             uuid.UUID
	tasks                  []liveCoreAnnotationTaskFixture
	primaryAnnotator       string
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

func TestLabelStudioLiveCoreResultReviewAndSnapshot(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_URL"))
	token := strings.TrimSpace(os.Getenv("TEST_LABEL_STUDIO_TOKEN"))
	dsn := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	endpoint := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ENDPOINT"))
	bucket := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_BUCKET"))
	accessKey := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_ACCESS_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("OBJECT_STORAGE_SECRET_KEY"))
	if baseURL == "" || token == "" || dsn == "" || endpoint == "" || bucket == "" || accessKey == "" || secretKey == "" {
		t.Skip("Label Studio, PostgreSQL, and object storage live-test environment are required")
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

	fixture := seedLiveCoreAnnotationFixture(t, ctx, pool, store)
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
		Title:       "Enterprise activity Gold Reference Pilot",
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

	engineTasks := make([]annotationapp.EngineTask, 0, len(fixture.tasks))
	for _, task := range fixture.tasks {
		engineTasks = append(engineTasks, annotationapp.EngineTask{
			TaskID:         task.taskID,
			SourceItemRef:  task.sourceItemRef,
			SourceSHA256:   task.sourceSHA256,
			TaskText:       task.taskText,
			TaskTextSHA256: task.taskTextSHA256,
			CorrelationKey: "core-task-" + task.taskID.String(),
		})
	}
	taskOperation, err := engineService.PrepareTasks(ctx, annotationapp.PrepareEngineTasksCommand{
		WorkspaceID: fixture.workspaceID,
		CampaignID:  fixture.campaignID,
		RequestID:   "live-tasks-" + uuid.NewString(),
		Tasks:       engineTasks,
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
	if len(taskBindings) != len(fixture.tasks) {
		t.Fatalf("task bindings=%+v", taskBindings)
	}
	externalTaskByID := make(map[uuid.UUID]string, len(taskBindings))
	for _, taskBinding := range taskBindings {
		externalTaskByID[taskBinding.TaskID] = taskBinding.ExternalTaskID
	}
	for _, task := range fixture.tasks {
		externalTaskID := externalTaskByID[task.taskID]
		if externalTaskID == "" {
			t.Fatalf("missing provider binding for task %s", task.taskID)
		}
		createAnnotation(t, &http.Client{Timeout: 30 * time.Second}, baseURL, token, externalTaskID, task.providerLabel)
	}

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
		Tasks:              engineTasks,
	}, annotationapp.EngineResultCursor{})
	if err != nil {
		t.Fatalf("discover provider actor for Core binding: %v", err)
	}
	if len(page.Results) != len(fixture.tasks) {
		t.Fatalf("provider results=%+v", page.Results)
	}
	externalActorRef := strings.TrimSpace(page.Results[0].ExternalAuthorRef)
	if externalActorRef == "" {
		t.Fatalf("provider result has empty author: %+v", page.Results[0])
	}
	for _, result := range page.Results {
		if strings.TrimSpace(result.ExternalAuthorRef) != externalActorRef {
			t.Fatalf("Reference Pilot provider results use different authors: %+v", page.Results)
		}
	}
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
		t.Fatalf("reconcile live Label Studio results through EngineRunner: %v", err)
	}

	type taskResult struct {
		resultID uuid.UUID
		revision int64
	}
	taskResults := make(map[uuid.UUID]taskResult, len(fixture.tasks))
	for _, task := range fixture.tasks {
		var resultID uuid.UUID
		var authorRef, providerBindingRef, storedExternalTaskID, externalAnnotationID, payloadHash string
		if err := pool.QueryRow(ctx, `
			SELECT id, author_ref, provider_binding_ref, external_task_id,
			       external_annotation_id, canonical_payload_sha256
			FROM annotation_result
			WHERE task_id=$1
		`, task.taskID).Scan(
			&resultID,
			&authorRef,
			&providerBindingRef,
			&storedExternalTaskID,
			&externalAnnotationID,
			&payloadHash,
		); err != nil {
			t.Fatalf("read reconciled Core AnnotationResult for task %s: %v", task.taskID, err)
		}
		if authorRef != fixture.primaryAnnotator ||
			providerBindingRef != binding.ID.String() ||
			storedExternalTaskID != externalTaskByID[task.taskID] ||
			externalAnnotationID == "" ||
			payloadHash != sha256HexString(`{"label":"`+task.providerLabel+`"}`) {
			t.Fatalf(
				"Core AnnotationResult provenance task=%s author=%s binding=%s providerTask=%s annotation=%s payload=%s",
				task.taskID,
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
		`, task.taskID).Scan(&taskStatus, &taskRevision); err != nil {
			t.Fatalf("read reconciled task %s: %v", task.taskID, err)
		}
		if taskStatus != annotationdomain.TaskReviewable || taskRevision < 2 {
			t.Fatalf("task %s status/revision=%s/%d", task.taskID, taskStatus, taskRevision)
		}
		taskResults[task.taskID] = taskResult{resultID: resultID, revision: taskRevision}
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

	accepted := fixture.tasks[0]
	acceptedResult := taskResults[accepted.taskID]
	reviewLiveCoreTask(
		t, mux, fixture.workspaceID, fixture.campaignID, accepted.taskID,
		acceptedResult.revision, acceptedResult.resultID, "ACCEPT",
		"Reference Pilot provider label verified", nil,
	)

	corrected := fixture.tasks[1]
	correctedResult := taskResults[corrected.taskID]
	correctedPayload := []byte(`{"label":"` + corrected.finalLabel + `"}`)
	reviewLiveCoreTask(
		t, mux, fixture.workspaceID, fixture.campaignID, corrected.taskID,
		correctedResult.revision, correctedResult.resultID, "CORRECT",
		"Reference Pilot provider label corrected", correctedPayload,
	)

	for index, task := range fixture.tasks {
		var reviewerRef, outcome string
		var selectedResultID uuid.UUID
		if err := pool.QueryRow(ctx, `
			SELECT reviewer_ref, selected_result_id, outcome
			FROM annotation_review_decision
			WHERE task_id=$1
		`, task.taskID).Scan(&reviewerRef, &selectedResultID, &outcome); err != nil {
			t.Fatalf("read review decision for task %s: %v", task.taskID, err)
		}
		if reviewerRef != reviewerID.String() {
			t.Fatalf("reviewer for task %s=%s want=%s", task.taskID, reviewerRef, reviewerID)
		}
		if index == 0 {
			if outcome != annotationdomain.ReviewAccept || selectedResultID != taskResults[task.taskID].resultID {
				t.Fatalf("ACCEPT decision task=%s outcome=%s selected=%s", task.taskID, outcome, selectedResultID)
			}
			continue
		}
		if outcome != annotationdomain.ReviewCorrect || selectedResultID == taskResults[task.taskID].resultID {
			t.Fatalf("CORRECT decision task=%s outcome=%s selected=%s provider=%s", task.taskID, outcome, selectedResultID, taskResults[task.taskID].resultID)
		}
		var selectedPayloadHash string
		if err := pool.QueryRow(ctx, `
			SELECT canonical_payload_sha256
			FROM annotation_result
			WHERE id=$1
		`, selectedResultID).Scan(&selectedPayloadHash); err != nil {
			t.Fatalf("read corrected selected result: %v", err)
		}
		if selectedPayloadHash != sha256HexString(string(correctedPayload)) {
			t.Fatalf("corrected result payload hash=%s", selectedPayloadHash)
		}
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
		snapshot.ExpectedTaskCount != 2 ||
		snapshot.ExpectedResultCount != 3 ||
		snapshot.ExpectedDecisionCount != 2 ||
		snapshot.ExpectedOutputCount != 2 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	valid, err := repo.GetSnapshotIntegrity(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("verify snapshot integrity: %v", err)
	}
	if !valid {
		t.Fatal("FINALIZED live annotation snapshot failed integrity verification")
	}
	writeLiveGoldSharedManifest(t, fixture, snapshot.ID)
}

func reviewLiveCoreTask(
	t *testing.T,
	mux *http.ServeMux,
	workspaceID, campaignID, taskID uuid.UUID,
	expectedRevision int64,
	reviewedResultID uuid.UUID,
	action, reason string,
	correctedPayload []byte,
) {
	t.Helper()
	body := fmt.Sprintf(
		`{"expectedTaskRevision":%d,"action":%q,"reason":%q,"reviewedResultId":%q`,
		expectedRevision,
		action,
		reason,
		reviewedResultID.String(),
	)
	if len(correctedPayload) > 0 {
		body += `,"correctedPayload":` + string(correctedPayload)
	}
	body += "}"
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/workspaces/"+workspaceID.String()+
			"/annotation-campaigns/"+campaignID.String()+
			"/tasks/"+taskID.String()+"/review",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer live-core-review-secret")
	request.Header.Set("Idempotency-Key", "live-review-"+uuid.NewString())
	request.Header.Set("X-Actor-ID", uuid.NewString())
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("trusted %s review status=%d body=%s", action, recorder.Code, recorder.Body.String())
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
	store *storage.Store,
) liveCoreAnnotationFixture {
	t.Helper()
	workspaceID := uuid.New()
	contributionResourceID := uuid.New()
	inputResourceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	certificationID := uuid.New()
	campaignID := uuid.New()
	task1ID := uuid.New()
	task2ID := uuid.New()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]

	headers := []string{
		"company_id", "period", "lease_activity", "energy_activity", "visit_activity",
		"activity_score", "activity_level", "indicator_coverage",
	}
	row1 := map[string]string{
		"company_id": "C001", "period": "2026-09",
		"lease_activity": "1", "energy_activity": "1", "visit_activity": "1",
		"activity_score": "100", "activity_level": "HIGH", "indicator_coverage": "100",
	}
	row2 := map[string]string{
		"company_id": "C002", "period": "2026-09",
		"lease_activity": "1", "energy_activity": "1", "visit_activity": "1",
		"activity_score": "90", "activity_level": "MEDIUM", "indicator_coverage": "100",
	}
	csvLine := func(row map[string]string) string {
		values := make([]string, 0, len(headers))
		for _, header := range headers {
			values = append(values, row[header])
		}
		return strings.Join(values, ",")
	}
	inputCSV := []byte(strings.Join(headers, ",") + "\n" + csvLine(row1) + "\n" + csvLine(row2) + "\n")
	inputObject := "gold-shared/input-" + uuid.NewString() + ".csv"
	inputURI, err := store.Put(ctx, inputObject, bytes.NewReader(inputCSV), int64(len(inputCSV)), "text/csv")
	if err != nil {
		t.Fatalf("put live shared Gold input: %v", err)
	}
	inputChecksum := sha256HexString(string(inputCSV))
	schema := `{"kind":"single-label-v1","labels":["INCONSISTENT_OUTPUT","INSUFFICIENT_INPUT","SUFFICIENT_INPUT"]}`
	schemaHash := sha256HexString(schema)
	primaryAnnotator := "annotator:live-core"

	makeTask := func(taskID uuid.UUID, sourceItemRef string, row map[string]string, providerLabel string) liveCoreAnnotationTaskFixture {
		sourcePayload, marshalErr := json.Marshal(row)
		if marshalErr != nil {
			t.Fatalf("marshal Reference Pilot source row: %v", marshalErr)
		}
		taskText := fmt.Sprintf(
			"company_id=%s period=%s lease_activity=%s energy_activity=%s visit_activity=%s activity_score=%s activity_level=%s indicator_coverage=%s",
			row["company_id"], row["period"], row["lease_activity"], row["energy_activity"], row["visit_activity"],
			row["activity_score"], row["activity_level"], row["indicator_coverage"],
		)
		return liveCoreAnnotationTaskFixture{
			taskID:         taskID,
			sourceItemRef:  sourceItemRef,
			sourceSHA256:   sha256HexString(string(sourcePayload)),
			taskText:       taskText,
			taskTextSHA256: sha256HexString(taskText),
			providerLabel:  providerLabel,
			finalLabel:     "SUFFICIENT_INPUT",
		}
	}
	tasks := []liveCoreAnnotationTaskFixture{
		makeTask(task1ID, "row:1", row1, "SUFFICIENT_INPUT"),
		makeTask(task2ID, "row:2", row2, "INCONSISTENT_OUTPUT"),
	}

	liveSQL(t, ctx, pool, `
		INSERT INTO data_resource(id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES
			($1,$3,$4,'live annotation contribution','OTHER','READY'),
			($2,$3,$5,'enterprise activity Gold Reference input','TABLE_LIKE','READY')
	`, contributionResourceID, inputResourceID, workspaceID, "LIVE-ANN-"+suffix, "LIVE-SRC-"+suffix)
	liveSQL(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type, source_resource_id)
		VALUES ($1,$2,$3,'enterprise activity Gold Reference input','CURATED',$4)
	`, datasetID, workspaceID, "LIVE-DS-"+suffix, inputResourceID)
	liveSQL(t, ctx, pool, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, byte_size, checksum_algorithm, checksum_value, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT',$3,'text/csv',2,$4,'SHA256',$5,now())
	`, versionID, datasetID, inputURI, int64(len(inputCSV)), inputChecksum)

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
			'activity-record-review','1.0.0',$6,$7,
			'activity-record-review','1.0.0',$6,$7,
			'activity-record-review','1.0.0',$6,$7,
			'enterprise-activity-record','1.0.0',$6,$7,
			'independent-review','1.0.0',$6,$7
		)
	`, campaignID, workspaceID, versionID, certificationID, contributionResourceID, schemaHash, schema)
	for _, task := range tasks {
		liveSQL(t, ctx, pool, `
			INSERT INTO annotation_task(
				id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
				task_text_sha256, primary_annotator_ref
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, task.taskID, workspaceID, campaignID, task.sourceItemRef, task.sourceSHA256, task.taskTextSHA256, primaryAnnotator)
	}
	liveSQL(t, ctx, pool, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=2,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("c", 64), inputChecksum)

	return liveCoreAnnotationFixture{
		workspaceID:            workspaceID,
		inputResourceID:        inputResourceID,
		inputDatasetVersionID:  versionID,
		inputCertificationID:   certificationID,
		contributionResourceID: contributionResourceID,
		campaignID:             campaignID,
		tasks:                  tasks,
		primaryAnnotator:       primaryAnnotator,
	}
}

func writeLiveGoldSharedManifest(t *testing.T, fixture liveCoreAnnotationFixture, snapshotID uuid.UUID) {
	t.Helper()
	artifacts := strings.TrimSpace(os.Getenv("LIVE_BROWSER_ARTIFACTS"))
	if artifacts == "" {
		t.Fatal("LIVE_BROWSER_ARTIFACTS is required for shared-facts Gold acceptance")
	}
	manifest := liveGoldSharedManifest{
		WorkspaceID:            fixture.workspaceID,
		InputResourceID:        fixture.inputResourceID,
		InputDatasetVersionID:  fixture.inputDatasetVersionID,
		InputCertificationID:   fixture.inputCertificationID,
		ContributionResourceID: fixture.contributionResourceID,
		CampaignID:             fixture.campaignID,
		SnapshotID:             snapshotID,
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("marshal shared Gold manifest: %v", err)
	}
	path := filepath.Join(artifacts, "gold-shared-facts.json")
	if err := os.WriteFile(path, append(content, '\n'), 0o600); err != nil {
		t.Fatalf("write shared Gold manifest: %v", err)
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
