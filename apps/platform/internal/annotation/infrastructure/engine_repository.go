package infrastructure

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
)

var (
	ErrEngineOperationConflict    = errors.New("annotation engine operation conflict")
	ErrEngineClaimBusy            = errors.New("annotation engine operation claim busy")
	ErrEngineBindingConflict      = errors.New("annotation engine binding conflict")
	ErrEngineOperationsUnsettled  = errors.New("annotation engine operations are unsettled")
	ErrEngineActorBindingNotFound = errors.New("annotation engine actor binding not found")
)

func (r *Repository) InsertEngineOperation(
	ctx context.Context,
	tx pgx.Tx,
	operation annotationdomain.EngineOperation,
) (bool, error) {
	if err := operation.Validate(); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
        INSERT INTO annotation_engine_operation(
            id, workspace_id, campaign_id, provider, provider_instance_ref,
            operation_kind, request_id, request_fingerprint,
            payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256,
            status, revision, created_at, updated_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
        ON CONFLICT (workspace_id, provider, provider_instance_ref, request_id) DO NOTHING
    `, operation.ID, operation.WorkspaceID, operation.CampaignID, operation.Provider,
		operation.ProviderInstanceRef, operation.OperationKind, operation.RequestID,
		operation.RequestFingerprint, operation.PayloadManifest, operation.PayloadManifestHash,
		operation.PayloadManifestSHA256, operation.Status, operation.Revision,
		operation.CreatedAt, operation.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("insert annotation engine operation: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}

	existing, err := getEngineOperationByRequest(
		ctx, tx, operation.WorkspaceID, operation.Provider, operation.ProviderInstanceRef, operation.RequestID,
	)
	if err != nil {
		return false, err
	}
	if existing.ID != operation.ID || existing.CampaignID != operation.CampaignID ||
		existing.OperationKind != operation.OperationKind ||
		existing.RequestFingerprint != operation.RequestFingerprint ||
		existing.PayloadManifestSHA256 != operation.PayloadManifestSHA256 ||
		!bytes.Equal(existing.PayloadManifestHash, operation.PayloadManifestHash) {
		return false, ErrEngineOperationConflict
	}
	return false, nil
}

func (r *Repository) GetEngineOperation(ctx context.Context, operationID uuid.UUID) (annotationdomain.EngineOperation, error) {
	return getEngineOperationByID(ctx, r.pool, operationID)
}

func (r *Repository) GetEngineOperationTx(
	ctx context.Context,
	tx pgx.Tx,
	operationID uuid.UUID,
) (annotationdomain.EngineOperation, error) {
	return getEngineOperationByID(ctx, tx, operationID)
}

func (r *Repository) RecoverExpiredEngineSending(
	ctx context.Context,
	tx pgx.Tx,
	operationID uuid.UUID,
	now time.Time,
) (annotationdomain.EngineOperation, error) {
	if operationID == uuid.Nil {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	operation, err := getEngineOperationByIDForUpdate(ctx, tx, operationID)
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	if operation.Status != annotationdomain.EngineOperationSending ||
		operation.ClaimExpiresAt == nil ||
		operation.ClaimExpiresAt.After(now.UTC()) {
		return annotationdomain.EngineOperation{}, ErrEngineClaimBusy
	}

	var attemptID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT a.id
		  FROM annotation_engine_attempt a
		 WHERE a.operation_id=$1
		   AND a.attempt_kind='SUBMIT'
		   AND NOT EXISTS (
		       SELECT 1
		         FROM annotation_engine_attempt_outcome o
		        WHERE o.attempt_id=a.id
		   )
		 ORDER BY a.attempt_no DESC
		 LIMIT 1
		 FOR UPDATE
	`, operationID).Scan(&attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.EngineOperation{}, fmt.Errorf(
			"recover expired annotation engine send: missing unresolved SUBMIT attempt",
		)
	}
	if err != nil {
		return annotationdomain.EngineOperation{}, fmt.Errorf(
			"recover expired annotation engine send attempt: %w",
			err,
		)
	}

	if err := r.AppendEngineAttemptOutcome(
		ctx,
		tx,
		annotationdomain.EngineAttemptOutcome{
			ID:            uuid.New(),
			AttemptID:     attemptID,
			Outcome:       annotationdomain.EngineAttemptUnknown,
			DiagnosticRef: "worker lease expired after durable send claim",
			OccurredAt:    now.UTC(),
		},
	); err != nil {
		return annotationdomain.EngineOperation{}, err
	}

	tag, err := tx.Exec(ctx, `
		UPDATE annotation_engine_operation
		   SET status='UNKNOWN', claimed_by=NULL, claim_expires_at=NULL, revision=revision+1
		 WHERE id=$1
		   AND revision=$2
		   AND status='SENDING'
		   AND claim_expires_at IS NOT NULL
		   AND claim_expires_at <= $3
	`, operationID, operation.Revision, now.UTC())
	if err != nil {
		return annotationdomain.EngineOperation{}, fmt.Errorf("recover expired annotation engine send: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return annotationdomain.EngineOperation{}, ErrEngineClaimBusy
	}
	return getEngineOperationByID(ctx, tx, operationID)
}

func (r *Repository) ClaimEngineOperation(
	ctx context.Context,
	tx pgx.Tx,
	operationID uuid.UUID,
	expectedRevision int64,
	workerRef string,
	leaseUntil time.Time,
) (annotationdomain.EngineOperation, error) {
	workerRef = strings.TrimSpace(workerRef)
	if operationID == uuid.Nil || expectedRevision < 1 || workerRef == "" || leaseUntil.IsZero() {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}
	tag, err := tx.Exec(ctx, `
        UPDATE annotation_engine_operation
           SET claimed_by=$3, claim_expires_at=$4, revision=revision+1
         WHERE id=$1
           AND revision=$2
           AND status IN ('PENDING','UNKNOWN')
           AND (claimed_by IS NULL OR claim_expires_at <= now())
    `, operationID, expectedRevision, workerRef, leaseUntil.UTC())
	if err != nil {
		return annotationdomain.EngineOperation{}, fmt.Errorf("claim annotation engine operation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return annotationdomain.EngineOperation{}, ErrEngineClaimBusy
	}
	return getEngineOperationByID(ctx, tx, operationID)
}

func (r *Repository) TransitionEngineOperation(
	ctx context.Context,
	tx pgx.Tx,
	operationID uuid.UUID,
	expectedRevision int64,
	workerRef, fromStatus, toStatus string,
	releaseClaim bool,
) (annotationdomain.EngineOperation, error) {
	workerRef = strings.TrimSpace(workerRef)
	if operationID == uuid.Nil || expectedRevision < 1 || workerRef == "" ||
		strings.TrimSpace(fromStatus) == "" || strings.TrimSpace(toStatus) == "" {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}
	var rowsAffected int64
	if releaseClaim {
		tag, err := tx.Exec(ctx, `
            UPDATE annotation_engine_operation
               SET status=$5, claimed_by=NULL, claim_expires_at=NULL, revision=revision+1
             WHERE id=$1 AND revision=$2 AND claimed_by=$3 AND status=$4
        `, operationID, expectedRevision, workerRef, fromStatus, toStatus)
		if err != nil {
			return annotationdomain.EngineOperation{}, fmt.Errorf("transition annotation engine operation: %w", err)
		}
		rowsAffected = tag.RowsAffected()
	} else {
		tag, err := tx.Exec(ctx, `
            UPDATE annotation_engine_operation
               SET status=$5, revision=revision+1
             WHERE id=$1 AND revision=$2 AND claimed_by=$3 AND status=$4
        `, operationID, expectedRevision, workerRef, fromStatus, toStatus)
		if err != nil {
			return annotationdomain.EngineOperation{}, fmt.Errorf("transition annotation engine operation: %w", err)
		}
		rowsAffected = tag.RowsAffected()
	}
	if rowsAffected != 1 {
		return annotationdomain.EngineOperation{}, ErrEngineClaimBusy
	}
	return getEngineOperationByID(ctx, tx, operationID)
}

func (r *Repository) StartEngineAttempt(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, operationID uuid.UUID,
	attemptKind string,
	startedAt time.Time,
) (annotationdomain.EngineAttempt, error) {
	if _, err := getEngineOperationByIDForUpdate(ctx, tx, operationID); err != nil {
		return annotationdomain.EngineAttempt{}, err
	}
	var next int
	if err := tx.QueryRow(ctx, `
        SELECT COALESCE(MAX(attempt_no),0)+1
          FROM annotation_engine_attempt
         WHERE operation_id=$1
    `, operationID).Scan(&next); err != nil {
		return annotationdomain.EngineAttempt{}, fmt.Errorf("next annotation engine attempt: %w", err)
	}
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	attempt := annotationdomain.EngineAttempt{
		ID: uuid.New(), WorkspaceID: workspaceID, OperationID: operationID,
		AttemptNo: next, AttemptKind: attemptKind, StartedAt: startedAt.UTC(),
	}
	if err := attempt.Validate(); err != nil {
		return annotationdomain.EngineAttempt{}, err
	}
	if _, err := tx.Exec(ctx, `
        INSERT INTO annotation_engine_attempt(
            id, workspace_id, operation_id, attempt_no, attempt_kind, started_at
        ) VALUES ($1,$2,$3,$4,$5,$6)
    `, attempt.ID, attempt.WorkspaceID, attempt.OperationID, attempt.AttemptNo,
		attempt.AttemptKind, attempt.StartedAt); err != nil {
		return annotationdomain.EngineAttempt{}, fmt.Errorf("insert annotation engine attempt: %w", err)
	}
	return attempt, nil
}

func (r *Repository) AppendEngineAttemptOutcome(
	ctx context.Context,
	tx pgx.Tx,
	outcome annotationdomain.EngineAttemptOutcome,
) error {
	if err := outcome.Validate(); err != nil {
		return err
	}
	if outcome.OccurredAt.IsZero() {
		outcome.OccurredAt = time.Now().UTC()
	}
	_, err := tx.Exec(ctx, `
        INSERT INTO annotation_engine_attempt_outcome(
            id, attempt_id, outcome, provider_status_code, diagnostic_ref, occurred_at
        ) VALUES ($1,$2,$3,$4,NULLIF($5,''),$6)
    `, outcome.ID, outcome.AttemptID, outcome.Outcome, outcome.ProviderStatusCode,
		outcome.DiagnosticRef, outcome.OccurredAt.UTC())
	if err != nil {
		return fmt.Errorf("append annotation engine attempt outcome: %w", err)
	}
	return nil
}

func (r *Repository) InsertEngineCampaignBinding(
	ctx context.Context,
	tx pgx.Tx,
	binding annotationdomain.EngineCampaignBinding,
) (bool, error) {
	if err := binding.Validate(); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
        INSERT INTO annotation_engine_campaign_binding(
            id, workspace_id, campaign_id, provider, provider_instance_ref,
            external_project_id, request_id, config_sha256, created_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
        ON CONFLICT (campaign_id) DO NOTHING
    `, binding.ID, binding.WorkspaceID, binding.CampaignID, binding.Provider,
		binding.ProviderInstance, binding.ExternalProjectID, binding.RequestID,
		binding.ConfigSHA256, binding.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("insert annotation engine campaign binding: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	existing, err := r.GetEngineCampaignBindingTx(ctx, tx, binding.CampaignID)
	if err != nil {
		return false, err
	}
	if existing.Provider != binding.Provider || existing.ProviderInstance != binding.ProviderInstance ||
		existing.ExternalProjectID != binding.ExternalProjectID || existing.RequestID != binding.RequestID ||
		existing.ConfigSHA256 != binding.ConfigSHA256 || existing.WorkspaceID != binding.WorkspaceID {
		return false, ErrEngineBindingConflict
	}
	return false, nil
}

func (r *Repository) GetEngineCampaignBinding(ctx context.Context, campaignID uuid.UUID) (annotationdomain.EngineCampaignBinding, error) {
	return getEngineCampaignBinding(ctx, r.pool, campaignID)
}

func (r *Repository) GetEngineCampaignBindingTx(
	ctx context.Context,
	tx pgx.Tx,
	campaignID uuid.UUID,
) (annotationdomain.EngineCampaignBinding, error) {
	return getEngineCampaignBinding(ctx, tx, campaignID)
}

func (r *Repository) InsertEngineTaskBinding(
	ctx context.Context,
	tx pgx.Tx,
	binding annotationdomain.EngineTaskBinding,
) (bool, error) {
	if err := binding.Validate(); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
        INSERT INTO annotation_engine_task_binding(
            id, workspace_id, campaign_binding_id, campaign_id, task_id, external_task_id, created_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7)
        ON CONFLICT (task_id) DO NOTHING
    `, binding.ID, binding.WorkspaceID, binding.CampaignBindingID, binding.CampaignID,
		binding.TaskID, binding.ExternalTaskID, binding.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("insert annotation engine task binding: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	var externalTaskID string
	var campaignBindingID, workspaceID, campaignID uuid.UUID
	err = tx.QueryRow(ctx, `
        SELECT external_task_id, campaign_binding_id, workspace_id, campaign_id
          FROM annotation_engine_task_binding
         WHERE task_id=$1
    `, binding.TaskID).Scan(&externalTaskID, &campaignBindingID, &workspaceID, &campaignID)
	if err != nil {
		return false, fmt.Errorf("read replayed annotation engine task binding: %w", err)
	}
	if externalTaskID != binding.ExternalTaskID || campaignBindingID != binding.CampaignBindingID ||
		workspaceID != binding.WorkspaceID || campaignID != binding.CampaignID {
		return false, ErrEngineBindingConflict
	}
	return false, nil
}

func (r *Repository) InsertEngineActorBinding(
	ctx context.Context,
	tx pgx.Tx,
	binding annotationdomain.EngineActorBinding,
) (bool, error) {
	if err := binding.Validate(); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO annotation_engine_actor_binding(
			id, workspace_id, provider, provider_instance_ref,
			external_actor_ref, core_actor_ref, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT DO NOTHING
	`, binding.ID, binding.WorkspaceID, binding.Provider, binding.ProviderInstance,
		binding.ExternalActorRef, binding.CoreActorRef, binding.CreatedAt, binding.CreatedBy)
	if err != nil {
		return false, fmt.Errorf("insert annotation engine actor binding: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}

	existing, err := r.ResolveEngineActorBinding(
		ctx,
		binding.WorkspaceID,
		binding.Provider,
		binding.ProviderInstance,
		binding.ExternalActorRef,
	)
	if errors.Is(err, ErrEngineActorBindingNotFound) {
		return false, ErrEngineBindingConflict
	}
	if err != nil {
		return false, err
	}
	if existing.ID != binding.ID || existing.CoreActorRef != binding.CoreActorRef {
		return false, ErrEngineBindingConflict
	}
	return false, nil
}

func (r *Repository) ResolveEngineActorBinding(
	ctx context.Context,
	workspaceID uuid.UUID,
	provider, providerInstance, externalActorRef string,
) (annotationdomain.EngineActorBinding, error) {
	var binding annotationdomain.EngineActorBinding
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, provider, provider_instance_ref,
		       external_actor_ref, core_actor_ref, created_at, created_by
		  FROM annotation_engine_actor_binding
		 WHERE workspace_id=$1
		   AND provider=$2
		   AND provider_instance_ref=$3
		   AND external_actor_ref=$4
	`, workspaceID, provider, providerInstance, externalActorRef).Scan(
		&binding.ID,
		&binding.WorkspaceID,
		&binding.Provider,
		&binding.ProviderInstance,
		&binding.ExternalActorRef,
		&binding.CoreActorRef,
		&binding.CreatedAt,
		&binding.CreatedBy,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.EngineActorBinding{}, ErrEngineActorBindingNotFound
	}
	if err != nil {
		return annotationdomain.EngineActorBinding{}, fmt.Errorf("resolve annotation engine actor binding: %w", err)
	}
	return binding, nil
}

func (r *Repository) LockAndRequireEngineOperationsSettledTx(
	ctx context.Context,
	tx pgx.Tx,
	campaignID uuid.UUID,
) error {
	rows, err := tx.Query(ctx, `
		SELECT id, status
		  FROM annotation_engine_operation
		 WHERE campaign_id=$1
		 ORDER BY id
		 FOR UPDATE
	`, campaignID)
	if err != nil {
		return fmt.Errorf("lock annotation engine operations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var operationID uuid.UUID
		var status string
		if err := rows.Scan(&operationID, &status); err != nil {
			return fmt.Errorf("scan locked annotation engine operation: %w", err)
		}
		switch status {
		case annotationdomain.EngineOperationMatched,
			annotationdomain.EngineOperationRejected,
			annotationdomain.EngineOperationConflict:
		default:
			return fmt.Errorf("%w: operation %s is %s", ErrEngineOperationsUnsettled, operationID, status)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate locked annotation engine operations: %w", err)
	}
	return nil
}

func (r *Repository) GetMatchedTaskSubmissionOperation(
	ctx context.Context,
	campaignID uuid.UUID,
	provider, providerInstance string,
) (annotationdomain.EngineOperation, error) {
	return scanEngineOperation(r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, campaign_id, provider, provider_instance_ref,
		       operation_kind, request_id, request_fingerprint,
		       payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256,
		       status, revision, COALESCE(claimed_by,''), claim_expires_at, created_at, updated_at
		  FROM annotation_engine_operation
		 WHERE campaign_id=$1
		   AND provider=$2
		   AND provider_instance_ref=$3
		   AND operation_kind='SUBMIT_TASKS'
		   AND status='MATCHED'
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1
	`, campaignID, provider, providerInstance))
}
func (r *Repository) ListEngineTaskBindings(
	ctx context.Context,
	campaignID uuid.UUID,
) ([]annotationdomain.EngineTaskBinding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, campaign_binding_id, campaign_id, task_id, external_task_id, created_at
		  FROM annotation_engine_task_binding
		 WHERE campaign_id=$1
		 ORDER BY task_id
	`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list annotation engine task bindings: %w", err)
	}
	defer rows.Close()

	bindings := make([]annotationdomain.EngineTaskBinding, 0)
	for rows.Next() {
		var binding annotationdomain.EngineTaskBinding
		if err := rows.Scan(
			&binding.ID,
			&binding.WorkspaceID,
			&binding.CampaignBindingID,
			&binding.CampaignID,
			&binding.TaskID,
			&binding.ExternalTaskID,
			&binding.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan annotation engine task binding: %w", err)
		}
		bindings = append(bindings, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotation engine task bindings: %w", err)
	}
	return bindings, nil
}

func (r *Repository) ListDispatchableEngineOperationIDs(
	ctx context.Context,
	provider, providerInstance string,
	limit int,
) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id
		  FROM annotation_engine_operation
		 WHERE provider=$1
		   AND provider_instance_ref=$2
		   AND (
		       status IN ('PENDING','UNKNOWN')
		       OR (status='SENDING' AND claim_expires_at IS NOT NULL AND claim_expires_at <= now())
		   )
		 ORDER BY updated_at, id
		 LIMIT $3
	`, provider, providerInstance, limit)
	if err != nil {
		return nil, fmt.Errorf("list dispatchable annotation engine operations: %w", err)
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dispatchable annotation engine operation: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dispatchable annotation engine operations: %w", err)
	}
	return ids, nil
}

func (r *Repository) ListCampaignIDsNeedingEngineResults(
	ctx context.Context,
	provider, providerInstance string,
	limit int,
) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT c.id
		  FROM annotation_campaign c
		  JOIN annotation_engine_campaign_binding b ON b.campaign_id=c.id
		 WHERE c.status='ACTIVE'
		   AND b.provider=$1
		   AND b.provider_instance_ref=$2
		   AND EXISTS (
		       SELECT 1
		         FROM annotation_engine_operation o
		        WHERE o.campaign_id=c.id
		          AND o.provider=b.provider
		          AND o.provider_instance_ref=b.provider_instance_ref
		          AND o.operation_kind='SUBMIT_TASKS'
		          AND o.status='MATCHED'
		   )
		   AND EXISTS (
		       SELECT 1
		         FROM annotation_task t
		        WHERE t.campaign_id=c.id
		          AND t.status IN ('PENDING','REVIEWABLE')
		   )
		 ORDER BY c.id
		 LIMIT $3
	`, provider, providerInstance, limit)
	if err != nil {
		return nil, fmt.Errorf("list annotation campaigns needing engine results: %w", err)
	}
	defer rows.Close()
	ids := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan annotation campaign needing engine results: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate annotation campaigns needing engine results: %w", err)
	}
	return ids, nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getEngineOperationByRequest(
	ctx context.Context,
	q rowQuerier,
	workspaceID uuid.UUID,
	provider, providerInstance, requestID string,
) (annotationdomain.EngineOperation, error) {
	return scanEngineOperation(q.QueryRow(ctx, `
        SELECT id, workspace_id, campaign_id, provider, provider_instance_ref,
               operation_kind, request_id, request_fingerprint,
               payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256,
               status, revision, COALESCE(claimed_by,''), claim_expires_at, created_at, updated_at
          FROM annotation_engine_operation
         WHERE workspace_id=$1 AND provider=$2 AND provider_instance_ref=$3 AND request_id=$4
    `, workspaceID, provider, providerInstance, requestID))
}

func getEngineOperationByID(
	ctx context.Context,
	q rowQuerier,
	operationID uuid.UUID,
) (annotationdomain.EngineOperation, error) {
	return scanEngineOperation(q.QueryRow(ctx, `
        SELECT id, workspace_id, campaign_id, provider, provider_instance_ref,
               operation_kind, request_id, request_fingerprint,
               payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256,
               status, revision, COALESCE(claimed_by,''), claim_expires_at, created_at, updated_at
          FROM annotation_engine_operation
         WHERE id=$1
    `, operationID))
}

func getEngineOperationByIDForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	operationID uuid.UUID,
) (annotationdomain.EngineOperation, error) {
	return scanEngineOperation(tx.QueryRow(ctx, `
        SELECT id, workspace_id, campaign_id, provider, provider_instance_ref,
               operation_kind, request_id, request_fingerprint,
               payload_manifest, payload_manifest_hash_payload, payload_manifest_sha256,
               status, revision, COALESCE(claimed_by,''), claim_expires_at, created_at, updated_at
          FROM annotation_engine_operation
         WHERE id=$1
         FOR UPDATE
    `, operationID))
}

func scanEngineOperation(row pgx.Row) (annotationdomain.EngineOperation, error) {
	var operation annotationdomain.EngineOperation
	if err := row.Scan(
		&operation.ID, &operation.WorkspaceID, &operation.CampaignID, &operation.Provider,
		&operation.ProviderInstanceRef, &operation.OperationKind, &operation.RequestID,
		&operation.RequestFingerprint, &operation.PayloadManifest, &operation.PayloadManifestHash,
		&operation.PayloadManifestSHA256, &operation.Status, &operation.Revision,
		&operation.ClaimedBy, &operation.ClaimExpiresAt, &operation.CreatedAt, &operation.UpdatedAt,
	); err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	return operation, nil
}

func getEngineCampaignBinding(
	ctx context.Context,
	q rowQuerier,
	campaignID uuid.UUID,
) (annotationdomain.EngineCampaignBinding, error) {
	var binding annotationdomain.EngineCampaignBinding
	err := q.QueryRow(ctx, `
        SELECT id, workspace_id, campaign_id, provider, provider_instance_ref,
               external_project_id, request_id, config_sha256, created_at
          FROM annotation_engine_campaign_binding
         WHERE campaign_id=$1
    `, campaignID).Scan(
		&binding.ID, &binding.WorkspaceID, &binding.CampaignID, &binding.Provider,
		&binding.ProviderInstance, &binding.ExternalProjectID, &binding.RequestID,
		&binding.ConfigSHA256, &binding.CreatedAt,
	)
	if err != nil {
		return annotationdomain.EngineCampaignBinding{}, err
	}
	return binding, nil
}
