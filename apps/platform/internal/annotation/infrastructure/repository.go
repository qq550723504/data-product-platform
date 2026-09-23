package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
)

var (
	ErrCampaignNotFound = errors.New("annotation campaign not found")
	ErrTaskNotFound     = errors.New("annotation task not found")
	ErrStaleRevision    = errors.New("annotation revision conflict")
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) InsertCampaign(ctx context.Context, tx pgx.Tx, campaign annotationdomain.Campaign) error {
	if campaign.ID == uuid.Nil {
		return annotationdomain.ErrInvalidCampaign
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO annotation_campaign (
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action, consumer_ref, scope_type, scope_ref,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot,
			status, revision, created_at, created_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),
			$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,
			$31,$32,$33,$34
		)
	`,
		campaign.ID, campaign.WorkspaceID, campaign.InputDatasetVersionID, campaign.InputCertificationID,
		campaign.AnnotationContributionID, campaign.Purpose, campaign.Action, campaign.ConsumerRef,
		campaign.ScopeType, campaign.ScopeRef,
		campaign.Schema.Ref, campaign.Schema.Version, campaign.Schema.ContentSHA256, campaign.Schema.ContentSnapshot,
		campaign.Taxonomy.Ref, campaign.Taxonomy.Version, campaign.Taxonomy.ContentSHA256, campaign.Taxonomy.ContentSnapshot,
		campaign.Rubric.Ref, campaign.Rubric.Version, campaign.Rubric.ContentSHA256, campaign.Rubric.ContentSnapshot,
		campaign.Renderer.Ref, campaign.Renderer.Version, campaign.Renderer.ContentSHA256, campaign.Renderer.ContentSnapshot,
		campaign.ReviewPolicy.Ref, campaign.ReviewPolicy.Version, campaign.ReviewPolicy.ContentSHA256, campaign.ReviewPolicy.ContentSnapshot,
		campaign.Status, campaign.Revision, campaign.CreatedAt, campaign.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("insert annotation campaign: %w", err)
	}
	return nil
}

func (r *Repository) InsertTask(ctx context.Context, tx pgx.Tx, task annotationdomain.Task) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO annotation_task (
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref, status, revision, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10)
	`, task.ID, task.WorkspaceID, task.CampaignID, task.SourceItemRef, task.SourceContentSHA256,
		task.TaskTextSHA256, task.PrimaryAnnotatorRef, task.Status, task.Revision, task.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert annotation task: %w", err)
	}
	return nil
}

func (r *Repository) ActivateCampaign(
	ctx context.Context,
	tx pgx.Tx,
	campaignID uuid.UUID,
	expectedRevision int64,
	expectedTaskCount int,
	taskManifestHash string,
	activatedAt time.Time,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE annotation_campaign
		   SET status='ACTIVE',
		       revision=revision+1,
		       expected_task_count=$3,
		       task_manifest_hash=$4,
		       activated_at=$5
		 WHERE id=$1 AND revision=$2 AND status='DRAFT'
	`, campaignID, expectedRevision, expectedTaskCount, taskManifestHash, activatedAt)
	if err != nil {
		return fmt.Errorf("activate annotation campaign: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleRevision
	}
	return nil
}

func (r *Repository) InsertResult(ctx context.Context, tx pgx.Tx, result annotationdomain.Result) error {
	if err := result.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO annotation_result (
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version,
			corrected_from_result_id, created_at, created_by
		) VALUES (
			$1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),
			$10,$11,$12,$13,$14,$15,$16
		)
	`, result.ID, result.WorkspaceID, result.CampaignID, result.TaskID, result.AuthorRef,
		result.ProviderBindingRef, result.ExternalTaskID, result.ExternalAnnotationID, result.ExternalRevision,
		result.ObservationKey, result.CanonicalPayload, result.CanonicalPayloadSHA256, result.NormalizerVersion,
		result.CorrectedFromResultID, result.CreatedAt, result.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert annotation result: %w", err)
	}
	return nil
}

func (r *Repository) AdvanceTaskForResult(
	ctx context.Context,
	tx pgx.Tx,
	taskID uuid.UUID,
	expectedRevision int64,
) error {
	tag, err := tx.Exec(ctx, `
		UPDATE annotation_task
		   SET status='REVIEWABLE', revision=revision+1
		 WHERE id=$1
		   AND revision=$2
		   AND status IN ('PENDING','REVIEWABLE')
		   AND current_decision_id IS NULL
	`, taskID, expectedRevision)
	if err != nil {
		return fmt.Errorf("advance annotation task after result: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleRevision
	}
	return nil
}

func (r *Repository) InsertReviewAttempt(ctx context.Context, tx pgx.Tx, attempt annotationdomain.ReviewAttempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO annotation_review_attempt (
			id, workspace_id, campaign_id, task_id, reviewer_ref, expected_task_revision,
			action, reason, idempotency_key, request_fingerprint, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, attempt.ID, attempt.WorkspaceID, attempt.CampaignID, attempt.TaskID, attempt.ReviewerRef,
		attempt.ExpectedTaskRevision, attempt.Action, attempt.Reason, attempt.IdempotencyKey,
		attempt.RequestFingerprint, attempt.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert annotation review attempt: %w", err)
	}
	return nil
}

func (r *Repository) InsertReviewAttemptOutcome(
	ctx context.Context,
	tx pgx.Tx,
	outcome annotationdomain.ReviewAttemptOutcome,
) error {
	if err := outcome.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO annotation_review_attempt_outcome (
			id, attempt_id, outcome, error_code, occurred_at
		) VALUES ($1,$2,$3,NULLIF($4,''),$5)
	`, outcome.ID, outcome.AttemptID, outcome.Outcome, outcome.ErrorCode, outcome.OccurredAt)
	if err != nil {
		return fmt.Errorf("insert annotation review attempt outcome: %w", err)
	}
	return nil
}

func (r *Repository) InsertReviewDecision(
	ctx context.Context,
	tx pgx.Tx,
	decision annotationdomain.ReviewDecision,
) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO annotation_review_decision (
			id, workspace_id, campaign_id, task_id, review_attempt_id,
			reviewed_result_id, selected_result_id, reviewer_ref, outcome, reason,
			expected_task_revision, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, decision.ID, decision.WorkspaceID, decision.CampaignID, decision.TaskID, decision.ReviewAttemptID,
		decision.ReviewedResultID, decision.SelectedResultID, decision.ReviewerRef, decision.Outcome,
		decision.Reason, decision.ExpectedTaskRevision, decision.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "expected task/attempt CAS") ||
			strings.Contains(err.Error(), "lost task CAS") ||
			strings.Contains(err.Error(), "uq_annotation_review_decision_task") {
			return ErrStaleRevision
		}
		return fmt.Errorf("insert annotation review decision: %w", err)
	}
	return nil
}

func (r *Repository) GetDecisionByAttempt(
	ctx context.Context,
	attemptID uuid.UUID,
) (annotationdomain.ReviewDecision, error) {
	var decision annotationdomain.ReviewDecision
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, campaign_id, task_id, review_attempt_id,
		       reviewed_result_id, selected_result_id, reviewer_ref, outcome, reason,
		       expected_task_revision, created_at
		  FROM annotation_review_decision
		 WHERE review_attempt_id=$1
	`, attemptID).Scan(
		&decision.ID, &decision.WorkspaceID, &decision.CampaignID, &decision.TaskID,
		&decision.ReviewAttemptID, &decision.ReviewedResultID, &decision.SelectedResultID,
		&decision.ReviewerRef, &decision.Outcome, &decision.Reason,
		&decision.ExpectedTaskRevision, &decision.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.ReviewDecision{}, pgx.ErrNoRows
	}
	if err != nil {
		return annotationdomain.ReviewDecision{}, fmt.Errorf("get annotation review decision by attempt: %w", err)
	}
	return decision, nil
}

func (r *Repository) GetCampaign(ctx context.Context, campaignID uuid.UUID) (annotationdomain.Campaign, error) {
	return getCampaign(ctx, r.pool, campaignID)
}

func (r *Repository) GetCampaignTx(ctx context.Context, tx pgx.Tx, campaignID uuid.UUID) (annotationdomain.Campaign, error) {
	return getCampaign(ctx, tx, campaignID)
}

func (r *Repository) GetTask(ctx context.Context, taskID uuid.UUID) (annotationdomain.Task, error) {
	return getTask(ctx, r.pool, taskID)
}

func (r *Repository) GetTaskTx(ctx context.Context, tx pgx.Tx, taskID uuid.UUID) (annotationdomain.Task, error) {
	return getTask(ctx, tx, taskID)
}


func (r *Repository) ListResultsTx(
	ctx context.Context,
	tx pgx.Tx,
	campaignID uuid.UUID,
) ([]annotationdomain.Result, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, workspace_id, campaign_id, task_id, author_ref,
		       COALESCE(provider_binding_ref,''), COALESCE(external_task_id,''),
		       COALESCE(external_annotation_id,''), COALESCE(external_revision,''),
		       observation_key, canonical_payload, canonical_payload_sha256,
		       normalizer_version, corrected_from_result_id, created_at, created_by
		  FROM annotation_result
		 WHERE campaign_id=$1
		 ORDER BY id
	`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list annotation results: %w", err)
	}
	defer rows.Close()
	results := make([]annotationdomain.Result, 0)
	for rows.Next() {
		var result annotationdomain.Result
		var payload []byte
		if err := rows.Scan(
			&result.ID, &result.WorkspaceID, &result.CampaignID, &result.TaskID, &result.AuthorRef,
			&result.ProviderBindingRef, &result.ExternalTaskID, &result.ExternalAnnotationID,
			&result.ExternalRevision, &result.ObservationKey, &payload, &result.CanonicalPayloadSHA256,
			&result.NormalizerVersion, &result.CorrectedFromResultID, &result.CreatedAt, &result.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("scan annotation result: %w", err)
		}
		result.CanonicalPayload = payload
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotation results: %w", err)
	}
	return results, nil
}

func (r *Repository) ListDecisionsTx(
	ctx context.Context,
	tx pgx.Tx,
	campaignID uuid.UUID,
) ([]annotationdomain.ReviewDecision, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, workspace_id, campaign_id, task_id, review_attempt_id,
		       reviewed_result_id, selected_result_id, reviewer_ref, outcome, reason,
		       expected_task_revision, created_at
		  FROM annotation_review_decision
		 WHERE campaign_id=$1
		 ORDER BY id
	`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list annotation review decisions: %w", err)
	}
	defer rows.Close()
	decisions := make([]annotationdomain.ReviewDecision, 0)
	for rows.Next() {
		var decision annotationdomain.ReviewDecision
		if err := rows.Scan(
			&decision.ID, &decision.WorkspaceID, &decision.CampaignID, &decision.TaskID,
			&decision.ReviewAttemptID, &decision.ReviewedResultID, &decision.SelectedResultID,
			&decision.ReviewerRef, &decision.Outcome, &decision.Reason,
			&decision.ExpectedTaskRevision, &decision.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan annotation review decision: %w", err)
		}
		decisions = append(decisions, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotation review decisions: %w", err)
	}
	return decisions, nil
}

func (r *Repository) InsertAndFinalizeSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	snapshot annotationdomain.Snapshot,
	tasks []annotationdomain.Task,
	results []annotationdomain.Result,
	decisions []annotationdomain.ReviewDecision,
	finalizedAt time.Time,
) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if snapshot.Status != annotationdomain.SnapshotBuilding {
		return annotationdomain.ErrInvalidSnapshot
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO annotation_snapshot (
			id, workspace_id, campaign_id, status, manifest, manifest_hash_payload, root_hash,
			expected_task_count, expected_result_count, expected_decision_count, expected_output_count,
			created_at, created_by
		) VALUES ($1,$2,$3,'BUILDING',$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, snapshot.ID, snapshot.WorkspaceID, snapshot.CampaignID, snapshot.Manifest,
		snapshot.ManifestHashPayload, snapshot.RootHash, snapshot.ExpectedTaskCount,
		snapshot.ExpectedResultCount, snapshot.ExpectedDecisionCount, snapshot.ExpectedOutputCount,
		snapshot.CreatedAt, snapshot.CreatedBy); err != nil {
		return fmt.Errorf("insert annotation snapshot: %w", err)
	}

	for _, task := range tasks {
		if _, err := tx.Exec(ctx, `
			INSERT INTO annotation_snapshot_task (
				snapshot_id, task_id, source_item_ref, source_content_sha256, task_text_sha256
			) VALUES ($1,$2,$3,$4,$5)
		`, snapshot.ID, task.ID, task.SourceItemRef, task.SourceContentSHA256, task.TaskTextSHA256); err != nil {
			return fmt.Errorf("insert annotation snapshot task: %w", err)
		}
	}
	for _, result := range results {
		if _, err := tx.Exec(ctx, `
			INSERT INTO annotation_snapshot_result (
				snapshot_id, result_id, task_id, canonical_payload_sha256, author_ref
			) VALUES ($1,$2,$3,$4,$5)
		`, snapshot.ID, result.ID, result.TaskID, result.CanonicalPayloadSHA256, result.AuthorRef); err != nil {
			return fmt.Errorf("insert annotation snapshot result: %w", err)
		}
	}
	for _, decision := range decisions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO annotation_snapshot_decision (
				snapshot_id, decision_id, task_id, outcome, reviewed_result_id,
				selected_result_id, reviewer_ref, reason
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, snapshot.ID, decision.ID, decision.TaskID, decision.Outcome,
			decision.ReviewedResultID, decision.SelectedResultID, decision.ReviewerRef, decision.Reason); err != nil {
			return fmt.Errorf("insert annotation snapshot decision: %w", err)
		}
		if decision.SelectedResultID != nil &&
			(decision.Outcome == annotationdomain.ReviewAccept || decision.Outcome == annotationdomain.ReviewCorrect) {
			if _, err := tx.Exec(ctx, `
				INSERT INTO annotation_snapshot_output (snapshot_id, task_id, selected_result_id)
				VALUES ($1,$2,$3)
			`, snapshot.ID, decision.TaskID, decision.SelectedResultID); err != nil {
				return fmt.Errorf("insert annotation snapshot output: %w", err)
			}
		}
	}

	tag, err := tx.Exec(ctx, `
		UPDATE annotation_snapshot
		   SET status='FINALIZED', finalized_at=$2
		 WHERE id=$1 AND status='BUILDING'
	`, snapshot.ID, finalizedAt)
	if err != nil {
		return fmt.Errorf("finalize annotation snapshot: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleRevision
	}
	return nil
}

func (r *Repository) GetResultByObservation(
	ctx context.Context,
	workspaceID, campaignID uuid.UUID,
	observationKey string,
) (annotationdomain.Result, error) {
	var result annotationdomain.Result
	var payload []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, campaign_id, task_id, author_ref,
		       COALESCE(provider_binding_ref,''), COALESCE(external_task_id,''),
		       COALESCE(external_annotation_id,''), COALESCE(external_revision,''),
		       observation_key, canonical_payload, canonical_payload_sha256,
		       normalizer_version, corrected_from_result_id, created_at, created_by
		  FROM annotation_result
		 WHERE workspace_id=$1 AND campaign_id=$2 AND observation_key=$3
	`, workspaceID, campaignID, observationKey).Scan(
		&result.ID, &result.WorkspaceID, &result.CampaignID, &result.TaskID, &result.AuthorRef,
		&result.ProviderBindingRef, &result.ExternalTaskID, &result.ExternalAnnotationID,
		&result.ExternalRevision, &result.ObservationKey, &payload, &result.CanonicalPayloadSHA256,
		&result.NormalizerVersion, &result.CorrectedFromResultID, &result.CreatedAt, &result.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.Result{}, pgx.ErrNoRows
	}
	if err != nil {
		return annotationdomain.Result{}, fmt.Errorf("get annotation result by observation: %w", err)
	}
	result.CanonicalPayload = payload
	return result, nil
}

func (r *Repository) GetReviewAttemptByKey(
	ctx context.Context,
	workspaceID uuid.UUID,
	idempotencyKey string,
) (annotationdomain.ReviewAttempt, error) {
	var attempt annotationdomain.ReviewAttempt
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, campaign_id, task_id, reviewer_ref, expected_task_revision,
		       action, reason, idempotency_key, request_fingerprint, created_at
		  FROM annotation_review_attempt
		 WHERE workspace_id=$1 AND idempotency_key=$2
	`, workspaceID, idempotencyKey).Scan(
		&attempt.ID, &attempt.WorkspaceID, &attempt.CampaignID, &attempt.TaskID,
		&attempt.ReviewerRef, &attempt.ExpectedTaskRevision, &attempt.Action, &attempt.Reason,
		&attempt.IdempotencyKey, &attempt.RequestFingerprint, &attempt.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.ReviewAttempt{}, pgx.ErrNoRows
	}
	if err != nil {
		return annotationdomain.ReviewAttempt{}, fmt.Errorf("get annotation review attempt by key: %w", err)
	}
	return attempt, nil
}

func (r *Repository) GetReviewAttemptOutcome(
	ctx context.Context,
	attemptID uuid.UUID,
) (annotationdomain.ReviewAttemptOutcome, error) {
	var outcome annotationdomain.ReviewAttemptOutcome
	err := r.pool.QueryRow(ctx, `
		SELECT id, attempt_id, outcome, COALESCE(error_code,''), occurred_at
		  FROM annotation_review_attempt_outcome
		 WHERE attempt_id=$1
	`, attemptID).Scan(&outcome.ID, &outcome.AttemptID, &outcome.Outcome, &outcome.ErrorCode, &outcome.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.ReviewAttemptOutcome{}, pgx.ErrNoRows
	}
	if err != nil {
		return annotationdomain.ReviewAttemptOutcome{}, fmt.Errorf("get annotation review attempt outcome: %w", err)
	}
	return outcome, nil
}

func (r *Repository) ListTasksTx(
	ctx context.Context,
	tx pgx.Tx,
	campaignID uuid.UUID,
) ([]annotationdomain.Task, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
		       task_text_sha256, COALESCE(primary_annotator_ref,''), status, revision,
		       current_decision_id, created_at
		  FROM annotation_task
		 WHERE campaign_id=$1
		 ORDER BY id
	`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list annotation tasks: %w", err)
	}
	defer rows.Close()
	tasks := make([]annotationdomain.Task, 0)
	for rows.Next() {
		var task annotationdomain.Task
		if err := rows.Scan(
			&task.ID, &task.WorkspaceID, &task.CampaignID, &task.SourceItemRef,
			&task.SourceContentSHA256, &task.TaskTextSHA256, &task.PrimaryAnnotatorRef,
			&task.Status, &task.Revision, &task.CurrentDecisionID, &task.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan annotation task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotation tasks: %w", err)
	}
	return tasks, nil
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getCampaign(ctx context.Context, q queryer, campaignID uuid.UUID) (annotationdomain.Campaign, error) {
	var c annotationdomain.Campaign
	err := q.QueryRow(ctx, `
		SELECT id, workspace_id, input_dataset_version_id, input_certification_id,
		       annotation_contribution_resource_id, purpose, action, COALESCE(consumer_ref,''),
		       COALESCE(scope_type,''), COALESCE(scope_ref,''),
		       schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
		       taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
		       rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
		       renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
		       review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot,
		       status, revision, COALESCE(expected_task_count,0), COALESCE(task_manifest_hash,''),
		       created_at, created_by, activated_at, sealed_at, cancelled_at
		  FROM annotation_campaign
		 WHERE id=$1
	`, campaignID).Scan(
		&c.ID, &c.WorkspaceID, &c.InputDatasetVersionID, &c.InputCertificationID,
		&c.AnnotationContributionID, &c.Purpose, &c.Action, &c.ConsumerRef, &c.ScopeType, &c.ScopeRef,
		&c.Schema.Ref, &c.Schema.Version, &c.Schema.ContentSHA256, &c.Schema.ContentSnapshot,
		&c.Taxonomy.Ref, &c.Taxonomy.Version, &c.Taxonomy.ContentSHA256, &c.Taxonomy.ContentSnapshot,
		&c.Rubric.Ref, &c.Rubric.Version, &c.Rubric.ContentSHA256, &c.Rubric.ContentSnapshot,
		&c.Renderer.Ref, &c.Renderer.Version, &c.Renderer.ContentSHA256, &c.Renderer.ContentSnapshot,
		&c.ReviewPolicy.Ref, &c.ReviewPolicy.Version, &c.ReviewPolicy.ContentSHA256, &c.ReviewPolicy.ContentSnapshot,
		&c.Status, &c.Revision, &c.ExpectedTaskCount, &c.TaskManifestHash,
		&c.CreatedAt, &c.CreatedBy, &c.ActivatedAt, &c.SealedAt, &c.CancelledAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.Campaign{}, ErrCampaignNotFound
	}
	if err != nil {
		return annotationdomain.Campaign{}, fmt.Errorf("get annotation campaign: %w", err)
	}
	return c, nil
}

func getTask(ctx context.Context, q queryer, taskID uuid.UUID) (annotationdomain.Task, error) {
	var task annotationdomain.Task
	err := q.QueryRow(ctx, `
		SELECT id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
		       task_text_sha256, COALESCE(primary_annotator_ref,''), status, revision,
		       current_decision_id, created_at
		  FROM annotation_task
		 WHERE id=$1
	`, taskID).Scan(
		&task.ID, &task.WorkspaceID, &task.CampaignID, &task.SourceItemRef,
		&task.SourceContentSHA256, &task.TaskTextSHA256, &task.PrimaryAnnotatorRef,
		&task.Status, &task.Revision, &task.CurrentDecisionID, &task.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.Task{}, ErrTaskNotFound
	}
	if err != nil {
		return annotationdomain.Task{}, fmt.Errorf("get annotation task: %w", err)
	}
	return task, nil
}
