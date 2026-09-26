package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

// GetMappingBySource resolves the authoritative current mapping for one source
// triple inside a workspace.
//
// A decision produced by an Entity Match job is authoritative only after that
// job reaches SUCCEEDED. RUNNING / WAITING_REVIEW / FAILED jobs may leave
// immutable candidate and decision history behind, but those partial facts must
// never become current mapping authority. Decisions that are not job-scoped
// (for example workflow aliases) remain authoritative immediately.
func (r *PostgresRepository) GetMappingBySource(ctx context.Context, workspaceID uuid.UUID, sourceType, sourceRef, sourceKey string) (domain.EntityMapping, error) {
	return getMappingBySource(ctx, r.pool, workspaceID, sourceType, sourceRef, sourceKey)
}

// GetMappingBySourceTx applies the same authority contract inside the caller's
// transaction. Native execution freezes the returned immutable decision, so it
// must never capture a decision from an unfinished or failed Entity Match job.
func (r *PostgresRepository) GetMappingBySourceTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, sourceType, sourceRef, sourceKey string) (domain.EntityMapping, error) {
	return getMappingBySource(ctx, tx, workspaceID, sourceType, sourceRef, sourceKey)
}

type mappingQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// LockMappingSourceTx serializes all decisions for one source triple before
// reading or creating its mutable mapping projection. Callers that may create
// evidence for a new mapping must take this lock before the existence check so
// concurrent retries cannot attach evidence to a discarded mapping ID.
func (r *PostgresRepository) LockMappingSourceTx(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, sourceType, sourceRef, sourceKey string) error {
	sourceDigest := sourceType + "\x1f" + sourceRef + "\x1f" + sourceKey
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, workspaceID.String(), sourceDigest); err != nil {
		return fmt.Errorf("lock mapping source: %w", err)
	}
	return nil
}

func getMappingBySource(ctx context.Context, q mappingQuerier, workspaceID uuid.UUID, sourceType, sourceRef, sourceKey string) (domain.EntityMapping, error) {
	var mapping domain.EntityMapping
	err := q.QueryRow(ctx, `
		SELECT d.mapping_id, d.workspace_id, d.entity_id, d.source_type, d.source_ref, d.source_key,
		       COALESCE(d.source_name,''), d.match_method, COALESCE(d.match_rule_id,''), d.match_policy_version,
		       d.match_engine_name, d.match_engine_version, d.match_model_version,
		       COALESCE(d.confidence,0), d.status, d.reviewed_by, d.reviewed_at,
		       COALESCE(d.reviewer_reason,''), d.evidence_id, em.created_at, d.id
		FROM entity_mapping_decision d
		JOIN entity_mapping em
		  ON em.workspace_id=d.workspace_id AND em.id=d.mapping_id
		LEFT JOIN entity_match_job j ON j.id=d.source_job_id
		WHERE d.workspace_id=$1
		  AND d.source_type=$2
		  AND d.source_ref=$3
		  AND d.source_key=$4
		  AND (d.source_job_id IS NULL OR j.status='SUCCEEDED')
		ORDER BY d.decided_seq DESC
		LIMIT 1
	`, workspaceID, sourceType, sourceRef, sourceKey).Scan(
		&mapping.ID, &mapping.WorkspaceID, &mapping.EntityID, &mapping.SourceType, &mapping.SourceRef, &mapping.SourceKey,
		&mapping.SourceName, &mapping.MatchMethod, &mapping.MatchRuleID, &mapping.MatchPolicyVersion,
		&mapping.MatchEngineName, &mapping.MatchEngineVersion, &mapping.MatchModelVersion,
		&mapping.Confidence, &mapping.Status, &mapping.ReviewedBy, &mapping.ReviewedAt,
		&mapping.ReviewerReason, &mapping.EvidenceID, &mapping.CreatedAt, &mapping.CurrentDecisionID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EntityMapping{}, ErrNotFound
	}
	if err != nil {
		return domain.EntityMapping{}, fmt.Errorf("get authoritative entity mapping by source: %w", err)
	}
	return mapping, nil
}

// GetMappingDecisionByIDTx reads one immutable decision with an explicit
// workspace check. A consumer may not turn a decision id into a cross-workspace
// lookup by accident.
func (r *PostgresRepository) GetMappingDecisionByIDTx(ctx context.Context, tx pgx.Tx, workspaceID, decisionID uuid.UUID) (domain.MappingDecision, error) {
	return getMappingDecisionByID(ctx, tx, workspaceID, decisionID)
}

func (r *PostgresRepository) GetMappingDecisionByID(ctx context.Context, workspaceID, decisionID uuid.UUID) (domain.MappingDecision, error) {
	return getMappingDecisionByID(ctx, r.pool, workspaceID, decisionID)
}

func getMappingDecisionByID(ctx context.Context, q mappingQuerier, workspaceID, decisionID uuid.UUID) (domain.MappingDecision, error) {
	return scanMappingDecision(q.QueryRow(ctx, `
		SELECT id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
		       COALESCE(source_name,''), match_method, COALESCE(match_rule_id,''), match_policy_version,
		       match_engine_name, match_engine_version, match_model_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, decided_at, decided_by,
		       COALESCE(idempotency_key,''), source_origin, source_job_id, source_candidate_id
		FROM entity_mapping_decision
		WHERE workspace_id=$1 AND id=$2
	`, workspaceID, decisionID))
}

// ListMappingsByEntity lists authoritative current mappings of one entity. For
// each source triple it selects the latest decision that is either not job-scoped
// or belongs to a SUCCEEDED Entity Match job. Partial facts from failed or
// unfinished jobs therefore remain queryable as history without entering the
// current mapping view.
func (r *PostgresRepository) ListMappingsByEntity(ctx context.Context, workspaceID, entityID uuid.UUID) ([]domain.EntityMapping, error) {
	rows, err := r.pool.Query(ctx, `
		WITH authoritative AS (
			SELECT DISTINCT ON (d.workspace_id, d.source_type, d.source_ref, d.source_key)
			       d.id, d.mapping_id, d.workspace_id, d.entity_id, d.source_type, d.source_ref, d.source_key,
			       d.source_name, d.match_method, d.match_rule_id, d.match_policy_version,
			       d.match_engine_name, d.match_engine_version, d.match_model_version,
			       d.confidence, d.status, d.reviewed_by, d.reviewed_at,
			       d.reviewer_reason, d.evidence_id, d.decided_seq
			FROM entity_mapping_decision d
			LEFT JOIN entity_match_job j ON j.id=d.source_job_id
			WHERE d.workspace_id=$1
			  AND (d.source_job_id IS NULL OR j.status='SUCCEEDED')
			ORDER BY d.workspace_id, d.source_type, d.source_ref, d.source_key, d.decided_seq DESC
		)
		SELECT a.mapping_id, a.workspace_id, a.entity_id, a.source_type, a.source_ref, a.source_key,
		       COALESCE(a.source_name,''), a.match_method, COALESCE(a.match_rule_id,''), a.match_policy_version,
		       a.match_engine_name, a.match_engine_version, a.match_model_version,
		       COALESCE(a.confidence,0), a.status, a.reviewed_by, a.reviewed_at,
		       COALESCE(a.reviewer_reason,''), a.evidence_id, em.created_at, a.id
		FROM authoritative a
		JOIN entity_mapping em
		  ON em.workspace_id=a.workspace_id AND em.id=a.mapping_id
		WHERE a.entity_id=$2
		ORDER BY a.source_type, a.source_ref, a.source_key
	`, workspaceID, entityID)
	if err != nil {
		return nil, fmt.Errorf("list authoritative entity mappings: %w", err)
	}
	defer rows.Close()

	result := make([]domain.EntityMapping, 0)
	for rows.Next() {
		var mapping domain.EntityMapping
		if err := rows.Scan(
			&mapping.ID, &mapping.WorkspaceID, &mapping.EntityID, &mapping.SourceType, &mapping.SourceRef, &mapping.SourceKey,
			&mapping.SourceName, &mapping.MatchMethod, &mapping.MatchRuleID, &mapping.MatchPolicyVersion,
			&mapping.MatchEngineName, &mapping.MatchEngineVersion, &mapping.MatchModelVersion,
			&mapping.Confidence, &mapping.Status, &mapping.ReviewedBy, &mapping.ReviewedAt,
			&mapping.ReviewerReason, &mapping.EvidenceID, &mapping.CreatedAt, &mapping.CurrentDecisionID,
		); err != nil {
			return nil, fmt.Errorf("scan authoritative entity mapping: %w", err)
		}
		result = append(result, mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authoritative entity mappings: %w", err)
	}
	return result, nil
}

// ListMappingDecisions returns the immutable decision history for one source
// triple, oldest first. Prior decisions are never overwritten, so callers can
// reconstruct how the current mapping was reached.
func (r *PostgresRepository) ListMappingDecisions(ctx context.Context, workspaceID uuid.UUID, sourceType, sourceRef, sourceKey string) ([]domain.MappingDecision, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
		       COALESCE(source_name,''), match_method, COALESCE(match_rule_id,''), match_policy_version,
		       match_engine_name, match_engine_version, match_model_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, decided_at, decided_by,
		       COALESCE(idempotency_key,''), source_origin, source_job_id, source_candidate_id
		FROM entity_mapping_decision
		WHERE workspace_id=$1 AND source_type=$2 AND source_ref=$3 AND source_key=$4
		ORDER BY decided_seq
	`, workspaceID, sourceType, sourceRef, sourceKey)
	if err != nil {
		return nil, fmt.Errorf("list entity mapping decisions: %w", err)
	}
	defer rows.Close()

	result := make([]domain.MappingDecision, 0)
	for rows.Next() {
		decision, err := scanMappingDecision(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, decision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entity mapping decisions: %w", err)
	}
	return result, nil
}

// findMappingDecisionByKey returns the decision already recorded for an
// operation-level idempotency key, if any. It runs inside the caller's
// transaction so it observes the same snapshot as the append.
func (r *PostgresRepository) findMappingDecisionByKey(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, key string) (domain.MappingDecision, bool, error) {
	decision, err := scanMappingDecision(tx.QueryRow(ctx, `
		SELECT id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
		       COALESCE(source_name,''), match_method, COALESCE(match_rule_id,''), match_policy_version,
		       match_engine_name, match_engine_version, match_model_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, decided_at, decided_by,
		       COALESCE(idempotency_key,''), source_origin, source_job_id, source_candidate_id
		FROM entity_mapping_decision
		WHERE workspace_id=$1 AND idempotency_key=$2
	`, workspaceID, key))
	if errors.Is(err, ErrNotFound) {
		return domain.MappingDecision{}, false, nil
	}
	if err != nil {
		return domain.MappingDecision{}, false, err
	}
	return decision, true, nil
}

func scanMappingDecision(row pgx.Row) (domain.MappingDecision, error) {
	var decision domain.MappingDecision
	if err := row.Scan(
		&decision.ID, &decision.WorkspaceID, &decision.MappingID, &decision.EntityID,
		&decision.SourceType, &decision.SourceRef, &decision.SourceKey,
		&decision.SourceName, &decision.MatchMethod, &decision.MatchRuleID, &decision.MatchPolicyVersion,
		&decision.MatchEngineName, &decision.MatchEngineVersion, &decision.MatchModelVersion,
		&decision.Confidence, &decision.Status, &decision.ReviewedBy, &decision.ReviewedAt,
		&decision.ReviewerReason, &decision.EvidenceID, &decision.DecidedAt, &decision.DecidedBy,
		&decision.IdempotencyKey, &decision.SourceOrigin, &decision.SourceJobID, &decision.SourceCandidateID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.MappingDecision{}, ErrNotFound
		}
		return domain.MappingDecision{}, fmt.Errorf("scan entity mapping decision: %w", err)
	}
	return decision, nil
}
