package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

func (r *PostgresRepository) GetEntity(ctx context.Context, entityID uuid.UUID) (domain.Entity, error) {
	return scanEntity(r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, entity_type_id, COALESCE(canonical_key,''), canonical_name,
		       attributes, status, created_at, created_by
		FROM entity
		WHERE id=$1
	`, entityID))
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

// RecordMappingDecision appends one immutable mapping decision and moves the
// mutable current-mapping projection to it inside the caller's transaction. It:
//
//   - is idempotent on (workspace, idempotency key): a retry returns the
//     decision already recorded instead of appending a duplicate;
//   - checks the caller's expected current decision under a row lock, so a stale
//     automatic decision cannot silently overwrite a newer human decision;
//   - refuses automatic matches over a human-confirmed mapping.
func (r *PostgresRepository) RecordMappingDecision(ctx context.Context, tx pgx.Tx, cmd domain.MappingDecisionCommand) (domain.MappingDecision, error) {
	mapping := cmd.Mapping
	if mapping.WorkspaceID == uuid.Nil {
		return domain.MappingDecision{}, domain.ErrMappingWorkspaceRequired
	}
	if strings.TrimSpace(mapping.MatchEngineName) == "" || strings.TrimSpace(mapping.MatchEngineVersion) == "" {
		return domain.MappingDecision{}, fmt.Errorf("mapping decision engine provenance is required")
	}

	origin := cmd.SourceOrigin
	if origin == "" {
		return domain.MappingDecision{}, domain.ErrMappingDecisionOriginRequired
	}
	if !origin.Valid() {
		return domain.MappingDecision{}, fmt.Errorf("unsupported mapping decision source origin %q", origin)
	}
	// Compare replay semantics against the normalized request, not the raw input.
	cmd.Mapping = mapping

	key, err := domain.NormalizeMappingDecisionKey(cmd.IdempotencyKey)
	if err != nil {
		return domain.MappingDecision{}, err
	}
	decidedAt := cmd.DecidedAt
	if decidedAt.IsZero() {
		decidedAt = mapping.CreatedAt
	}
	if decidedAt.IsZero() {
		decidedAt = time.Now().UTC()
	}

	// Serialize concurrent retries that reuse one operation key so the
	// existence check below cannot race with itself.
	if key != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, mapping.WorkspaceID.String(), key); err != nil {
			return domain.MappingDecision{}, fmt.Errorf("lock mapping decision key: %w", err)
		}
		existing, found, err := r.findMappingDecisionByKey(ctx, tx, mapping.WorkspaceID, key)
		if err != nil {
			return domain.MappingDecision{}, err
		}
		if found {
			// A key hit is only an idempotent replay when the request semantics
			// match. Reusing the key for another target, reason, actor or source
			// must fail instead of reporting a false success.
			if !domain.MappingDecisionMatchesRequest(existing, cmd) {
				return domain.MappingDecision{}, domain.ErrMappingDecisionKeyConflict
			}
			return existing, nil
		}
	}

	// Serialize every operation on the same source, keyed independently of the
	// operation idempotency key. A first creation cannot lock a row that does not
	// exist yet, so two different keys (for example, one human review and one
	// automatic match) could both observe "no current decision" and then race on
	// the same upsert. Holding the source lock before the read makes that
	// impossible: the second operation re-reads the committed current decision.
	// The key lock is always taken first so the two lock orders cannot deadlock.
	if err := r.LockMappingSourceTx(ctx, tx, mapping.WorkspaceID, mapping.SourceType, mapping.SourceRef, mapping.SourceKey); err != nil {
		return domain.MappingDecision{}, err
	}

	// Lock the physical projection row before updating it. The physical pointer
	// may name a decision from a RUNNING / WAITING_REVIEW / FAILED match job, so
	// it is not itself the authority boundary. Concurrency and human-priority
	// checks must use the latest authoritative decision selected by
	// getMappingBySource: job-scoped decisions become authoritative only after
	// their job reaches SUCCEEDED.
	mappingID := mapping.ID
	err = tx.QueryRow(ctx, `
		SELECT em.id
		FROM entity_mapping em
		WHERE em.workspace_id=$1 AND em.source_type=$2 AND em.source_ref=$3 AND em.source_key=$4
		FOR UPDATE OF em
	`, mapping.WorkspaceID, mapping.SourceType, mapping.SourceRef, mapping.SourceKey).Scan(&mappingID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.MappingDecision{}, fmt.Errorf("lock current entity mapping: %w", err)
	}

	var currentDecisionID *uuid.UUID
	var currentStatus *domain.MappingStatus
	current, currentErr := getMappingBySource(ctx, tx, mapping.WorkspaceID, mapping.SourceType, mapping.SourceRef, mapping.SourceKey)
	switch {
	case currentErr == nil:
		currentDecisionID = current.CurrentDecisionID
		status := current.Status
		currentStatus = &status
	case errors.Is(currentErr, ErrNotFound):
		// No authoritative decision exists yet. This is expected when the only
		// history belongs to unfinished or failed Entity Match jobs.
	default:
		return domain.MappingDecision{}, currentErr
	}

	if err := domain.EnsureMappingDecisionExpectation(currentDecisionID, cmd.ExpectCurrentDecision, cmd.ExpectedCurrentDecisionID, mapping.Status); err != nil {
		return domain.MappingDecision{}, err
	}
	if err := domain.EnsureMappingDecisionAllowed(currentStatus, mapping.Status); err != nil {
		return domain.MappingDecision{}, err
	}

	if mappingID == uuid.Nil {
		mappingID = uuid.New()
	}
	decisionID := uuid.New()

	// entity_mapping is the mutable projection. It points at the decision that
	// produced it. The current-pointer foreign key is deferred until commit, so
	// the decision row can be inserted just after this projection row.
	if err := tx.QueryRow(ctx, `
		INSERT INTO entity_mapping (
			id, workspace_id, entity_id, source_type, source_ref, source_key, source_name,
			match_method, match_rule_id, match_policy_version,
			match_engine_name, match_engine_version, match_model_version,
			confidence, status, reviewed_by, reviewed_at, reviewer_reason, evidence_id, created_at,
			current_decision_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
		ON CONFLICT (workspace_id, source_type, source_ref, source_key) DO UPDATE SET
			entity_id=EXCLUDED.entity_id, source_name=EXCLUDED.source_name,
			match_method=EXCLUDED.match_method, match_rule_id=EXCLUDED.match_rule_id,
			match_policy_version=EXCLUDED.match_policy_version,
			match_engine_name=EXCLUDED.match_engine_name,
			match_engine_version=EXCLUDED.match_engine_version,
			match_model_version=EXCLUDED.match_model_version,
			confidence=EXCLUDED.confidence,
			status=EXCLUDED.status, reviewed_by=EXCLUDED.reviewed_by,
			reviewed_at=EXCLUDED.reviewed_at, reviewer_reason=EXCLUDED.reviewer_reason,
			evidence_id=EXCLUDED.evidence_id,
			current_decision_id=EXCLUDED.current_decision_id
		RETURNING id
	`, mappingID, mapping.WorkspaceID, mapping.EntityID, mapping.SourceType, mapping.SourceRef, mapping.SourceKey,
		mapping.SourceName, mapping.MatchMethod, mapping.MatchRuleID, mapping.MatchPolicyVersion,
		mapping.MatchEngineName, mapping.MatchEngineVersion, mapping.MatchModelVersion,
		mapping.Confidence, mapping.Status, mapping.ReviewedBy, mapping.ReviewedAt,
		mapping.ReviewerReason, mapping.EvidenceID, mapping.CreatedAt, decisionID).Scan(&mappingID); err != nil {
		return domain.MappingDecision{}, fmt.Errorf("upsert entity mapping: %w", err)
	}

	var decidedSeq int64
	err = tx.QueryRow(ctx, `
		INSERT INTO entity_mapping_decision (
			id, workspace_id, mapping_id, entity_id, source_type, source_ref, source_key,
			source_name, match_method, match_rule_id, match_policy_version,
			match_engine_name, match_engine_version, match_model_version,
			confidence, status, reviewed_by, reviewed_at, reviewer_reason, evidence_id,
			idempotency_key, source_origin, source_job_id, source_candidate_id,
			decided_at, decided_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
		          $21, $22, $23, $24, $25, $26)
		RETURNING decided_seq
	`, decisionID, mapping.WorkspaceID, mappingID, mapping.EntityID, mapping.SourceType, mapping.SourceRef, mapping.SourceKey,
		mapping.SourceName, mapping.MatchMethod, mapping.MatchRuleID, mapping.MatchPolicyVersion,
		mapping.MatchEngineName, mapping.MatchEngineVersion, mapping.MatchModelVersion,
		mapping.Confidence, mapping.Status, mapping.ReviewedBy, mapping.ReviewedAt,
		mapping.ReviewerReason, mapping.EvidenceID,
		key, origin, cmd.SourceJobID, cmd.SourceCandidateID, decidedAt, cmd.DecidedBy).Scan(&decidedSeq)
	if err != nil {
		return domain.MappingDecision{}, fmt.Errorf("insert entity mapping decision: %w", err)
	}

	return domain.MappingDecision{
		ID:                 decisionID,
		WorkspaceID:        mapping.WorkspaceID,
		MappingID:          mappingID,
		EntityID:           mapping.EntityID,
		SourceType:         mapping.SourceType,
		SourceRef:          mapping.SourceRef,
		SourceKey:          mapping.SourceKey,
		SourceName:         mapping.SourceName,
		MatchMethod:        mapping.MatchMethod,
		MatchRuleID:        mapping.MatchRuleID,
		MatchPolicyVersion: mapping.MatchPolicyVersion,
		MatchEngineName:    mapping.MatchEngineName,
		MatchEngineVersion: mapping.MatchEngineVersion,
		MatchModelVersion:  mapping.MatchModelVersion,
		Confidence:         mapping.Confidence,
		Status:             mapping.Status,
		ReviewedBy:         mapping.ReviewedBy,
		ReviewedAt:         mapping.ReviewedAt,
		ReviewerReason:     mapping.ReviewerReason,
		EvidenceID:         mapping.EvidenceID,
		DecidedAt:          decidedAt,
		DecidedBy:          cmd.DecidedBy,
		IdempotencyKey:     key,
		SourceOrigin:       origin,
		SourceJobID:        cmd.SourceJobID,
		SourceCandidateID:  cmd.SourceCandidateID,
	}, nil
}
