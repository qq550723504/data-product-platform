package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
)

var ErrNotFound = errors.New("delivery operation not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

type IdempotencyRecord struct {
	ObjectID           uuid.UUID
	RequestFingerprint string
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) TryInsertIdempotency(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, key, fingerprint string, operationID uuid.UUID) (bool, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO command_idempotency(workspace_id, command_type, idempotency_key, object_id, result_ref, request_fingerprint)
		VALUES ($1,'DELIVERY.ISSUE_CREDENTIAL',$2,$3,$3,$4)
		ON CONFLICT (workspace_id, command_type, idempotency_key) DO NOTHING
		RETURNING id
	`, workspaceID, key, operationID, fingerprint).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert delivery idempotency: %w", err)
	}
	return id != uuid.Nil, nil
}

func (r *PostgresRepository) FindIdempotency(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, key string) (IdempotencyRecord, bool, error) {
	var record IdempotencyRecord
	err := tx.QueryRow(ctx, `
		SELECT object_id, COALESCE(request_fingerprint,'')
		FROM command_idempotency
		WHERE workspace_id=$1 AND command_type='DELIVERY.ISSUE_CREDENTIAL' AND idempotency_key=$2
		FOR UPDATE
	`, workspaceID, key).Scan(&record.ObjectID, &record.RequestFingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("find delivery idempotency: %w", err)
	}
	return record, true, nil
}

func (r *PostgresRepository) InsertOperation(ctx context.Context, tx pgx.Tx, operation domain.Operation) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_operation(
			id, workspace_id, dataset_version_id, certification_ref, idempotency_key,
			provider_name, provider_request_key, status, current_gate_decision,
			dependency_revision, principal_ref, effective_consumer_ref, delegation_ref,
			purpose, action, scope_ref, delivery_channel, delivery_mode, requested_expires_at,
			fresh_cap_expires_at, credential_ref, credential_hash,
			provider_credential_expires_at, terminal_reason, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
	`, operation.ID, operation.WorkspaceID, operation.DatasetVersionID, operation.CertificationRef,
		operation.IdempotencyKey, operation.ProviderName, operation.ProviderRequestKey,
		operation.Status, operation.CurrentGateDecision, operation.DependencyRevision,
		operation.PrincipalRef, operation.EffectiveConsumerRef, nullable(operation.DelegationRef),
		operation.Purpose, operation.Action, operation.ScopeRef, operation.DeliveryChannel, operation.DeliveryMode,
		operation.RequestedExpiresAt, operation.FreshCapExpiresAt, nullable(operation.CredentialRef),
		nullable(operation.CredentialHash), operation.ProviderCredentialExpiresAt,
		nullable(operation.TerminalReason), operation.CreatedAt, operation.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert delivery operation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetOperation(ctx context.Context, tx pgx.Tx, id uuid.UUID, forUpdate bool) (domain.Operation, error) {
	lock := ""
	if forUpdate {
		lock = " FOR UPDATE"
	}
	var operation domain.Operation
	err := tx.QueryRow(ctx, `
		SELECT id, workspace_id, dataset_version_id, certification_ref, idempotency_key,
		       provider_name, provider_request_key, status, current_gate_decision,
		       dependency_revision, principal_ref, effective_consumer_ref, COALESCE(delegation_ref,''),
		       purpose, action, scope_ref, delivery_channel, delivery_mode, requested_expires_at,
		       fresh_cap_expires_at, COALESCE(credential_ref,''), COALESCE(credential_hash,''),
		       provider_credential_expires_at, COALESCE(terminal_reason,''), created_at, updated_at
		FROM delivery_operation WHERE id=$1`+lock, id).Scan(
		&operation.ID, &operation.WorkspaceID, &operation.DatasetVersionID, &operation.CertificationRef,
		&operation.IdempotencyKey, &operation.ProviderName, &operation.ProviderRequestKey,
		&operation.Status, &operation.CurrentGateDecision, &operation.DependencyRevision,
		&operation.PrincipalRef, &operation.EffectiveConsumerRef, &operation.DelegationRef,
		&operation.Purpose, &operation.Action, &operation.ScopeRef, &operation.DeliveryChannel, &operation.DeliveryMode,
		&operation.RequestedExpiresAt, &operation.FreshCapExpiresAt, &operation.CredentialRef,
		&operation.CredentialHash, &operation.ProviderCredentialExpiresAt, &operation.TerminalReason,
		&operation.CreatedAt, &operation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Operation{}, ErrNotFound
	}
	if err != nil {
		return domain.Operation{}, fmt.Errorf("get delivery operation: %w", err)
	}
	return operation, nil
}

func (r *PostgresRepository) LockFence(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID) (int64, error) {
	return deliveryfence.Lock(ctx, tx, workspaceID)
}

func (r *PostgresRepository) InsertGateEvaluation(ctx context.Context, tx pgx.Tx, operationID uuid.UUID, evaluation domain.GateEvaluation, createdAt time.Time) error {
	if evaluation.ID == uuid.Nil {
		evaluation.ID = uuid.New()
	}
	if evaluation.EvaluationKey == "" {
		evaluation.EvaluationKey = uuid.NewString()
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	blockers := evaluation.Blockers
	if blockers == nil {
		blockers = []string{}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_gate_evaluation(
			id, delivery_operation_id, evaluation_key, stage, decision, blockers,
			dependency_revision, principal_ref, effective_consumer_ref, delegation_ref,
			fresh_cap_expires_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, evaluation.ID, operationID, evaluation.EvaluationKey, evaluation.Stage, evaluation.Decision(), blockers,
		evaluation.DependencyRevision, evaluation.PrincipalRef, evaluation.EffectiveConsumerRef,
		nullable(evaluation.DelegationRef), evaluation.FreshCapExpiresAt, createdAt)
	if err != nil {
		return fmt.Errorf("insert delivery gate evaluation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertTransition(ctx context.Context, tx pgx.Tx, operationID uuid.UUID, transitionKey string, from, to domain.Status, evaluationID, attemptID *uuid.UUID, reason string, createdAt time.Time) error {
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_transition(
			id, delivery_operation_id, transition_key, from_status, to_status,
			gate_evaluation_id, provider_attempt_id, reason, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, uuid.New(), operationID, transitionKey, from, to, evaluationID, attemptID, nullable(reason), createdAt)
	if err != nil {
		return fmt.Errorf("insert delivery transition: %w", err)
	}
	return nil
}

func (r *PostgresRepository) UpdateProjection(ctx context.Context, tx pgx.Tx, operation domain.Operation) error {
	_, err := tx.Exec(ctx, `
		UPDATE delivery_operation SET
			status=$2, current_gate_decision=$3, dependency_revision=$4,
			fresh_cap_expires_at=$5, credential_ref=$6, credential_hash=$7,
			provider_credential_expires_at=$8, terminal_reason=$9, updated_at=$10
		WHERE id=$1
	`, operation.ID, operation.Status, operation.CurrentGateDecision, operation.DependencyRevision,
		operation.FreshCapExpiresAt, nullable(operation.CredentialRef), nullable(operation.CredentialHash),
		operation.ProviderCredentialExpiresAt, nullable(operation.TerminalReason), operation.UpdatedAt)
	if err != nil {
		return fmt.Errorf("update delivery projection: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetContainmentStatus(ctx context.Context, tx pgx.Tx, operationID uuid.UUID) (domain.ContainmentStatus, bool, error) {
	var status domain.ContainmentStatus
	err := tx.QueryRow(ctx, `
		SELECT status FROM delivery_containment WHERE delivery_operation_id=$1 FOR UPDATE
	`, operationID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("get delivery containment: %w", err)
	}
	return status, true, nil
}

func (r *PostgresRepository) UpsertContainmentProjection(ctx context.Context, tx pgx.Tx, operationID uuid.UUID, status domain.ContainmentStatus, attemptID *uuid.UUID, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_containment(delivery_operation_id, status, last_provider_attempt_id, reason, created_at, updated_at)
		VALUES ($1,$2,$3,$4,now(),now())
		ON CONFLICT (delivery_operation_id) DO UPDATE SET
			status=EXCLUDED.status, last_provider_attempt_id=EXCLUDED.last_provider_attempt_id,
			reason=EXCLUDED.reason, updated_at=EXCLUDED.updated_at
	`, operationID, status, attemptID, nullable(reason))
	if err != nil {
		return fmt.Errorf("upsert delivery containment: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertContainmentTransition(ctx context.Context, tx pgx.Tx, operationID uuid.UUID, from, to domain.ContainmentStatus, attemptID *uuid.UUID, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_containment_transition(
			id, delivery_operation_id, from_status, to_status, provider_attempt_id, reason
		) VALUES ($1,$2,$3,$4,$5,$6)
	`, uuid.New(), operationID, nullable(string(from)), to, attemptID, nullable(reason))
	if err != nil {
		return fmt.Errorf("insert delivery containment transition: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertProviderAttempt(ctx context.Context, tx pgx.Tx, operation domain.Operation, invocationKey string, kind domain.InvocationKind) (uuid.UUID, error) {
	id := uuid.New()
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_provider_attempt(
			id, delivery_operation_id, provider_request_key, invocation_key, invocation_kind
		) VALUES ($1,$2,$3,$4,$5)
	`, id, operation.ID, operation.ProviderRequestKey, invocationKey, kind)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert delivery provider attempt: %w", err)
	}
	return id, nil
}

func (r *PostgresRepository) InsertProviderObservation(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID, kind domain.ObservationKind, outcome domain.Outcome, capability domain.Capability, evidenceRef string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_provider_observation(
			id, provider_attempt_id, observation_kind, observed_outcome,
			capability_ref, capability_hash, provider_credential_expires_at, evidence_ref
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, uuid.New(), attemptID, kind, outcome, nullable(capability.CapabilityRef), nullable(capability.CapabilityHash),
		nullableTime(capability.ProviderCredentialExpiresAt), nullable(evidenceRef))
	if err != nil {
		return fmt.Errorf("insert delivery provider observation: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindUnobservedIssueAttempt(ctx context.Context, tx pgx.Tx, operationID, excludeAttemptID uuid.UUID) (uuid.UUID, bool, error) {
	var attemptID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT a.id
		FROM delivery_provider_attempt a
		WHERE a.delivery_operation_id=$1
		  AND a.id <> $2
		  AND a.invocation_kind='ISSUE'
		  AND NOT EXISTS (
			  SELECT 1 FROM delivery_provider_observation o WHERE o.provider_attempt_id=a.id
		  )
		ORDER BY a.started_at, a.id
		LIMIT 1
	`, operationID, excludeAttemptID).Scan(&attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("find unobserved delivery issue attempt: %w", err)
	}
	return attemptID, true, nil
}

func (r *PostgresRepository) FindActiveUnobservedIssueAttempt(ctx context.Context, tx pgx.Tx, operationID, excludeAttemptID uuid.UUID, startedAfter time.Time) (uuid.UUID, bool, error) {
	var attemptID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT a.id
		FROM delivery_provider_attempt a
		WHERE a.delivery_operation_id=$1
		  AND a.id <> $2
		  AND a.invocation_kind='ISSUE'
		  AND a.started_at >= $3
		  AND NOT EXISTS (
			  SELECT 1 FROM delivery_provider_observation o WHERE o.provider_attempt_id=a.id
		  )
		ORDER BY a.started_at, a.id
		LIMIT 1
	`, operationID, excludeAttemptID, startedAfter).Scan(&attemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("find active unobserved delivery issue attempt: %w", err)
	}
	return attemptID, true, nil
}

func (r *PostgresRepository) GetProviderAttemptKind(ctx context.Context, tx pgx.Tx, attemptID uuid.UUID) (domain.InvocationKind, error) {
	var kind domain.InvocationKind
	if err := tx.QueryRow(ctx, `SELECT invocation_kind FROM delivery_provider_attempt WHERE id=$1`, attemptID).Scan(&kind); err != nil {
		return "", fmt.Errorf("get delivery provider attempt kind: %w", err)
	}
	return kind, nil
}

func (r *PostgresRepository) InsertReplayDecision(ctx context.Context, tx pgx.Tx, operationID, replayAttemptID, gateEvaluationID uuid.UUID, decision string, evaluation domain.GateEvaluation, capability domain.Capability, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO delivery_credential_replay_decision(
			id, replay_attempt_id, delivery_operation_id, gate_evaluation_id, decision,
			principal_ref, effective_consumer_ref, dependency_revision, fresh_cap_expires_at,
			capability_ref, capability_hash, reason
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, uuid.New(), replayAttemptID, operationID, gateEvaluationID, decision,
		evaluation.PrincipalRef, evaluation.EffectiveConsumerRef, evaluation.DependencyRevision,
		evaluation.FreshCapExpiresAt, nullable(capability.CapabilityRef), nullable(capability.CapabilityHash), nullable(reason))
	if err != nil {
		return fmt.Errorf("insert delivery replay decision: %w", err)
	}
	return nil
}

func (r *PostgresRepository) InsertProviderCost(ctx context.Context, tx pgx.Tx, operation domain.Operation, attemptID uuid.UUID) error {
	costEventID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO cost_event(id, workspace_id, execution_id, activity_id, cost_type, quantity, unit, pricing_mode, metadata)
		VALUES ($1,$2,NULL,$3,'DELIVERY_PROVIDER_INVOCATION',1,'invocation','ACTUAL',jsonb_build_object('provider_attempt_id',$4::text))
	`, costEventID, operation.WorkspaceID, attemptID, attemptID); err != nil {
		return fmt.Errorf("insert delivery provider cost event: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cost_allocation(id, cost_event_id, delivery_operation_id)
		VALUES ($1,$2,$3)
	`, uuid.New(), costEventID, operation.ID); err != nil {
		return fmt.Errorf("insert delivery cost allocation: %w", err)
	}
	return nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
