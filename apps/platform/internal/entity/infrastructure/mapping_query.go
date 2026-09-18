package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

// GetMappingBySource resolves the current mapping for one source triple inside a
// workspace. The workspace is mandatory: the same external source key can be
// mapped independently by different workspaces, so a workspace-less lookup would
// be able to return another workspace's mapping.
func (r *PostgresRepository) GetMappingBySource(ctx context.Context, workspaceID uuid.UUID, sourceType, sourceRef, sourceKey string) (domain.EntityMapping, error) {
	var mapping domain.EntityMapping
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, entity_id, source_type, source_ref, source_key, COALESCE(source_name,''),
		       match_method, COALESCE(match_rule_id,''), match_policy_version,
		       match_engine_name, match_engine_version, match_model_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, created_at, current_decision_id
		FROM entity_mapping
		WHERE workspace_id=$1 AND source_type=$2 AND source_ref=$3 AND source_key=$4
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
		return domain.EntityMapping{}, fmt.Errorf("get entity mapping by source: %w", err)
	}
	return mapping, nil
}

// ListMappingsByEntity lists the current mappings of one entity. The workspace
// filter keeps a cross-workspace entity id from returning mappings that do not
// belong to the caller's workspace.
func (r *PostgresRepository) ListMappingsByEntity(ctx context.Context, workspaceID, entityID uuid.UUID) ([]domain.EntityMapping, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, entity_id, source_type, source_ref, source_key, COALESCE(source_name,''),
		       match_method, COALESCE(match_rule_id,''), match_policy_version,
		       match_engine_name, match_engine_version, match_model_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, created_at, current_decision_id
		FROM entity_mapping
		WHERE workspace_id=$1 AND entity_id=$2
		ORDER BY source_type, source_ref, source_key
	`, workspaceID, entityID)
	if err != nil {
		return nil, fmt.Errorf("list entity mappings: %w", err)
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
			return nil, fmt.Errorf("scan entity mapping: %w", err)
		}
		result = append(result, mapping)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entity mappings: %w", err)
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
