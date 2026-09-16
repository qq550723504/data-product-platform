package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

func (r *PostgresRepository) GetMappingBySource(ctx context.Context, sourceType, sourceRef, sourceKey string) (domain.EntityMapping, error) {
	var mapping domain.EntityMapping
	err := r.pool.QueryRow(ctx, `
		SELECT id, entity_id, source_type, source_ref, source_key, COALESCE(source_name,''),
		       match_method, COALESCE(match_rule_id,''), match_policy_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, created_at
		FROM entity_mapping
		WHERE source_type=$1 AND source_ref=$2 AND source_key=$3
	`, sourceType, sourceRef, sourceKey).Scan(
		&mapping.ID, &mapping.EntityID, &mapping.SourceType, &mapping.SourceRef, &mapping.SourceKey,
		&mapping.SourceName, &mapping.MatchMethod, &mapping.MatchRuleID, &mapping.MatchPolicyVersion,
		&mapping.Confidence, &mapping.Status, &mapping.ReviewedBy, &mapping.ReviewedAt,
		&mapping.ReviewerReason, &mapping.EvidenceID, &mapping.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.EntityMapping{}, ErrNotFound
	}
	if err != nil {
		return domain.EntityMapping{}, fmt.Errorf("get entity mapping by source: %w", err)
	}
	return mapping, nil
}

func (r *PostgresRepository) ListMappingsByEntity(ctx context.Context, entityID uuid.UUID) ([]domain.EntityMapping, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, entity_id, source_type, source_ref, source_key, COALESCE(source_name,''),
		       match_method, COALESCE(match_rule_id,''), match_policy_version,
		       COALESCE(confidence,0), status, reviewed_by, reviewed_at,
		       COALESCE(reviewer_reason,''), evidence_id, created_at
		FROM entity_mapping
		WHERE entity_id=$1
		ORDER BY source_type, source_ref, source_key
	`, entityID)
	if err != nil {
		return nil, fmt.Errorf("list entity mappings: %w", err)
	}
	defer rows.Close()

	result := make([]domain.EntityMapping, 0)
	for rows.Next() {
		var mapping domain.EntityMapping
		if err := rows.Scan(
			&mapping.ID, &mapping.EntityID, &mapping.SourceType, &mapping.SourceRef, &mapping.SourceKey,
			&mapping.SourceName, &mapping.MatchMethod, &mapping.MatchRuleID, &mapping.MatchPolicyVersion,
			&mapping.Confidence, &mapping.Status, &mapping.ReviewedBy, &mapping.ReviewedAt,
			&mapping.ReviewerReason, &mapping.EvidenceID, &mapping.CreatedAt,
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
