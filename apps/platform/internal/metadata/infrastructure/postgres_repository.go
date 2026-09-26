package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
)

var (
	ErrNotFound               = errors.New("metadata projection record not found")
	ErrStaleProjectionAttempt = errors.New("stale metadata projection attempt")
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) UpsertBinding(ctx context.Context, tx pgx.Tx, binding metadatadomain.ResourceBinding) (metadatadomain.ResourceBinding, error) {
	metadata, err := json.Marshal(binding.BindingMetadata)
	if err != nil {
		return metadatadomain.ResourceBinding{}, fmt.Errorf("marshal resource binding metadata: %w", err)
	}
	if binding.ID == uuid.Nil {
		binding.ID = uuid.New()
	}
	var persisted metadatadomain.ResourceBinding
	var persistedMetadata []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO resource_binding (
			id, resource_id, provider, entity_type, external_id, external_fqn,
			binding_metadata, is_primary, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now(),now())
		ON CONFLICT (provider, entity_type, external_fqn)
		DO UPDATE SET
			resource_id = EXCLUDED.resource_id,
			external_id = EXCLUDED.external_id,
			binding_metadata = EXCLUDED.binding_metadata,
			is_primary = EXCLUDED.is_primary,
			updated_at = now()
		RETURNING id, resource_id, provider, entity_type, COALESCE(external_id,''), external_fqn,
		          binding_metadata, is_primary, created_at, updated_at
	`, binding.ID, binding.ResourceID, binding.Provider, binding.EntityType, nullableString(binding.ExternalID),
		binding.ExternalFQN, metadata, binding.IsPrimary).Scan(
		&persisted.ID, &persisted.ResourceID, &persisted.Provider, &persisted.EntityType,
		&persisted.ExternalID, &persisted.ExternalFQN, &persistedMetadata, &persisted.IsPrimary,
		&persisted.CreatedAt, &persisted.UpdatedAt,
	)
	if err != nil {
		return metadatadomain.ResourceBinding{}, fmt.Errorf("upsert resource binding: %w", err)
	}
	if err := json.Unmarshal(persistedMetadata, &persisted.BindingMetadata); err != nil {
		return metadatadomain.ResourceBinding{}, fmt.Errorf("decode persisted resource binding metadata: %w", err)
	}
	return persisted, nil
}

func (r *PostgresRepository) ListBindings(ctx context.Context, resourceID uuid.UUID) ([]metadatadomain.ResourceBinding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, resource_id, provider, entity_type, COALESCE(external_id,''), external_fqn,
		       binding_metadata, is_primary, created_at, updated_at
		FROM resource_binding
		WHERE resource_id=$1
		ORDER BY is_primary DESC, provider, entity_type, external_fqn
	`, resourceID)
	if err != nil {
		return nil, fmt.Errorf("list resource bindings: %w", err)
	}
	defer rows.Close()

	result := make([]metadatadomain.ResourceBinding, 0)
	for rows.Next() {
		var binding metadatadomain.ResourceBinding
		var metadata []byte
		if err := rows.Scan(
			&binding.ID, &binding.ResourceID, &binding.Provider, &binding.EntityType,
			&binding.ExternalID, &binding.ExternalFQN, &metadata, &binding.IsPrimary,
			&binding.CreatedAt, &binding.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan resource binding: %w", err)
		}
		if err := json.Unmarshal(metadata, &binding.BindingMetadata); err != nil {
			return nil, fmt.Errorf("decode resource binding metadata: %w", err)
		}
		result = append(result, binding)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resource bindings: %w", err)
	}
	return result, nil
}

func (r *PostgresRepository) BeginProjection(ctx context.Context, tx pgx.Tx, projection metadatadomain.GovernanceProjection) (int, error) {
	metadata, err := json.Marshal(projection.Metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal governance projection metadata: %w", err)
	}
	if projection.ID == uuid.Nil {
		projection.ID = uuid.New()
	}
	var attempt int
	err = tx.QueryRow(ctx, `
		INSERT INTO governance_projection (
			id, workspace_id, provider, object_type, object_id, source_event_id,
			status, attempts, last_error, metadata, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'PENDING',1,NULL,$7,now(),now())
		ON CONFLICT (provider, object_type, object_id)
		DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			source_event_id = EXCLUDED.source_event_id,
			status = 'PENDING',
			attempts = governance_projection.attempts + 1,
			last_error = NULL,
			metadata = EXCLUDED.metadata,
			updated_at = now()
		RETURNING attempts
	`, projection.ID, projection.WorkspaceID, projection.Provider, projection.ObjectType,
		projection.ObjectID, projection.SourceEventID, metadata).Scan(&attempt)
	if err != nil {
		return 0, fmt.Errorf("begin governance projection: %w", err)
	}
	return attempt, nil
}

func (r *PostgresRepository) MarkProjectionSucceeded(ctx context.Context, tx pgx.Tx, provider metadatadomain.Provider, objectType string, objectID, expectedSourceEventID uuid.UUID, expectedAttempt int, externalID, externalFQN string, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal governance projection success metadata: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE governance_projection
		SET status='SUCCEEDED', external_id=$6, external_fqn=$7, metadata=$8,
		    last_error=NULL, projected_at=now(), updated_at=now()
		WHERE provider=$1 AND object_type=$2 AND object_id=$3
		  AND source_event_id=$4 AND attempts=$5 AND status='PENDING'
	`, provider, objectType, objectID, expectedSourceEventID, expectedAttempt,
		nullableString(externalID), nullableString(externalFQN), encoded)
	if err != nil {
		return fmt.Errorf("mark governance projection succeeded: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleProjectionAttempt
	}
	return nil
}

func (r *PostgresRepository) MarkProjectionFailed(ctx context.Context, tx pgx.Tx, provider metadatadomain.Provider, objectType string, objectID, expectedSourceEventID uuid.UUID, expectedAttempt int, cause error) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	tag, err := tx.Exec(ctx, `
		UPDATE governance_projection
		SET status='FAILED', last_error=$6, updated_at=now()
		WHERE provider=$1 AND object_type=$2 AND object_id=$3
		  AND source_event_id=$4 AND attempts=$5 AND status='PENDING'
	`, provider, objectType, objectID, expectedSourceEventID, expectedAttempt, message)
	if err != nil {
		return fmt.Errorf("mark governance projection failed: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrStaleProjectionAttempt
	}
	return nil
}

func (r *PostgresRepository) GetProjection(ctx context.Context, provider metadatadomain.Provider, objectType string, objectID uuid.UUID) (metadatadomain.GovernanceProjection, error) {
	var projection metadatadomain.GovernanceProjection
	var metadata []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, provider, object_type, object_id, source_event_id,
		       COALESCE(external_id,''), COALESCE(external_fqn,''), status, attempts,
		       COALESCE(last_error,''), metadata, projected_at, created_at, updated_at
		FROM governance_projection
		WHERE provider=$1 AND object_type=$2 AND object_id=$3
	`, provider, objectType, objectID).Scan(
		&projection.ID, &projection.WorkspaceID, &projection.Provider, &projection.ObjectType,
		&projection.ObjectID, &projection.SourceEventID, &projection.ExternalID, &projection.ExternalFQN,
		&projection.Status, &projection.Attempts, &projection.LastError, &metadata,
		&projection.ProjectedAt, &projection.CreatedAt, &projection.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return metadatadomain.GovernanceProjection{}, ErrNotFound
	}
	if err != nil {
		return metadatadomain.GovernanceProjection{}, fmt.Errorf("get governance projection: %w", err)
	}
	if err := json.Unmarshal(metadata, &projection.Metadata); err != nil {
		return metadatadomain.GovernanceProjection{}, fmt.Errorf("decode governance projection metadata: %w", err)
	}
	return projection, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
