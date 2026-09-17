package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

var ErrNotFound = errors.New("entity object not found")

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) EnsureEntityType(ctx context.Context, tx pgx.Tx, entityType domain.EntityType) (domain.EntityType, error) {
	keySchema, _ := json.Marshal(entityType.KeySchema)
	attributeSchema, _ := json.Marshal(entityType.AttributeSchema)
	var stored domain.EntityType
	var storedKeySchema, storedAttributeSchema []byte
	err := tx.QueryRow(ctx, `
		INSERT INTO entity_type (
			id, workspace_id, code, name, key_schema, attribute_schema,
			matching_policy_ref, matching_policy_version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$9)
		ON CONFLICT (workspace_id, code) DO UPDATE SET
			name = EXCLUDED.name,
			matching_policy_ref = EXCLUDED.matching_policy_ref,
			matching_policy_version = EXCLUDED.matching_policy_version,
			updated_at = now()
		RETURNING id, workspace_id, code, name, key_schema, attribute_schema,
		          COALESCE(matching_policy_ref,''), COALESCE(matching_policy_version,''), created_at
	`, entityType.ID, entityType.WorkspaceID, entityType.Code, entityType.Name, keySchema, attributeSchema,
		entityType.MatchingPolicyRef, entityType.MatchingPolicyVersion, entityType.CreatedAt).Scan(
		&stored.ID, &stored.WorkspaceID, &stored.Code, &stored.Name, &storedKeySchema, &storedAttributeSchema,
		&stored.MatchingPolicyRef, &stored.MatchingPolicyVersion, &stored.CreatedAt,
	)
	if err != nil {
		return domain.EntityType{}, fmt.Errorf("ensure entity type: %w", err)
	}
	_ = json.Unmarshal(storedKeySchema, &stored.KeySchema)
	_ = json.Unmarshal(storedAttributeSchema, &stored.AttributeSchema)
	return stored, nil
}

func (r *PostgresRepository) InsertEntity(ctx context.Context, tx pgx.Tx, entity domain.Entity) error {
	attributes, err := json.Marshal(entity.Attributes)
	if err != nil {
		return fmt.Errorf("marshal entity attributes: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO entity (
			id, workspace_id, entity_type_id, canonical_key, canonical_name,
			attributes, status, created_at, created_by, updated_at, updated_by
		) VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$8,$9)
	`, entity.ID, entity.WorkspaceID, entity.EntityTypeID, entity.CanonicalKey, entity.CanonicalName,
		attributes, entity.Status, entity.CreatedAt, entity.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert entity: %w", err)
	}
	return nil
}

func (r *PostgresRepository) FindByCanonicalKey(ctx context.Context, entityTypeID uuid.UUID, canonicalKey string) (*domain.Entity, error) {
	if canonicalKey == "" {
		return nil, nil
	}
	return r.findOne(ctx, `
		SELECT id, workspace_id, entity_type_id, COALESCE(canonical_key,''), canonical_name,
		       attributes, status, created_at, created_by
		FROM entity
		WHERE entity_type_id=$1 AND canonical_key=$2 AND status='ACTIVE'
		LIMIT 1
	`, entityTypeID, canonicalKey)
}

// FindByNameAddress returns the ACTIVE entities that share the same normalized
// name and address, capped at two rows because callers only need to distinguish
// "exactly one" from "ambiguous". The schema permits duplicates, and picking one
// arbitrarily would attach a source row to the wrong canonical entity.
func (r *PostgresRepository) FindByNameAddress(ctx context.Context, entityTypeID uuid.UUID, normalizedName, normalizedAddress string) ([]domain.Entity, error) {
	if normalizedName == "" || normalizedAddress == "" {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, entity_type_id, COALESCE(canonical_key,''), canonical_name,
		       attributes, status, created_at, created_by
		FROM entity
		WHERE entity_type_id=$1 AND status='ACTIVE'
		  AND attributes->>'normalized_company_name'=$2
		  AND attributes->>'normalized_registered_address'=$3
		ORDER BY id
		LIMIT 2
	`, entityTypeID, normalizedName, normalizedAddress)
	if err != nil {
		return nil, fmt.Errorf("find entities by name and address: %w", err)
	}
	defer rows.Close()
	entities := make([]domain.Entity, 0, 2)
	for rows.Next() {
		entity, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	return entities, rows.Err()
}

func (r *PostgresRepository) ListByLegalRepresentative(ctx context.Context, entityTypeID uuid.UUID, legalRepresentative string) ([]domain.Entity, error) {
	if legalRepresentative == "" {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, entity_type_id, COALESCE(canonical_key,''), canonical_name,
		       attributes, status, created_at, created_by
		FROM entity
		WHERE entity_type_id=$1 AND status='ACTIVE'
		  AND attributes->>'legal_representative'=$2
		ORDER BY id
	`, entityTypeID, legalRepresentative)
	if err != nil {
		return nil, fmt.Errorf("list entities by legal representative: %w", err)
	}
	defer rows.Close()
	entities := make([]domain.Entity, 0)
	for rows.Next() {
		entity, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	return entities, rows.Err()
}

func (r *PostgresRepository) findOne(ctx context.Context, query string, args ...any) (*domain.Entity, error) {
	entity, err := scanEntity(r.pool.QueryRow(ctx, query, args...))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

func scanEntity(row pgx.Row) (domain.Entity, error) {
	var entity domain.Entity
	var attributes []byte
	if err := row.Scan(&entity.ID, &entity.WorkspaceID, &entity.EntityTypeID, &entity.CanonicalKey,
		&entity.CanonicalName, &attributes, &entity.Status, &entity.CreatedAt, &entity.CreatedBy); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Entity{}, ErrNotFound
		}
		return domain.Entity{}, fmt.Errorf("scan entity: %w", err)
	}
	if err := json.Unmarshal(attributes, &entity.Attributes); err != nil {
		return domain.Entity{}, fmt.Errorf("decode entity attributes: %w", err)
	}
	return entity, nil
}

func (r *PostgresRepository) InsertMapping(ctx context.Context, tx pgx.Tx, mapping domain.EntityMapping) error {
	// Older Core paths (for example workflow alias resolution) predate external
	// candidate engines. Normalize missing provenance here so every accepted
	// mapping remains auditable even when it was produced by deterministic rules.
	if strings.TrimSpace(mapping.MatchEngineName) == "" {
		mapping.MatchEngineName = "RULES"
	}
	if strings.TrimSpace(mapping.MatchEngineVersion) == "" {
		mapping.MatchEngineVersion = "1"
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO entity_mapping (
			id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version,
			match_engine_name, match_engine_version, match_model_version,
			confidence, status, reviewed_by, reviewed_at, reviewer_reason, evidence_id, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		ON CONFLICT (source_type, source_ref, source_key) DO UPDATE SET
			entity_id=EXCLUDED.entity_id, source_name=EXCLUDED.source_name,
			match_method=EXCLUDED.match_method, match_rule_id=EXCLUDED.match_rule_id,
			match_policy_version=EXCLUDED.match_policy_version,
			match_engine_name=EXCLUDED.match_engine_name,
			match_engine_version=EXCLUDED.match_engine_version,
			match_model_version=EXCLUDED.match_model_version,
			confidence=EXCLUDED.confidence,
			status=EXCLUDED.status, reviewed_by=EXCLUDED.reviewed_by,
			reviewed_at=EXCLUDED.reviewed_at, reviewer_reason=EXCLUDED.reviewer_reason,
			evidence_id=EXCLUDED.evidence_id
	`, mapping.ID, mapping.EntityID, mapping.SourceType, mapping.SourceRef, mapping.SourceKey,
		mapping.SourceName, mapping.MatchMethod, mapping.MatchRuleID, mapping.MatchPolicyVersion,
		mapping.MatchEngineName, mapping.MatchEngineVersion, mapping.MatchModelVersion,
		mapping.Confidence, mapping.Status, mapping.ReviewedBy, mapping.ReviewedAt,
		mapping.ReviewerReason, mapping.EvidenceID, mapping.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert entity mapping: %w", err)
	}
	return nil
}
