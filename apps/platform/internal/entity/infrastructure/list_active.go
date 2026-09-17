package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
)

// ListActive returns the canonical Core Entity records available to an optional
// candidate generator. Provider-specific blocking/indexing remains an adapter
// concern and can replace this POC-scale reference scan later.
func (r *PostgresRepository) ListActive(ctx context.Context, entityTypeID uuid.UUID) ([]domain.Entity, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, entity_type_id, COALESCE(canonical_key,''), canonical_name,
		       attributes, status, created_at, created_by
		FROM entity
		WHERE entity_type_id=$1 AND status='ACTIVE'
		ORDER BY id
	`, entityTypeID)
	if err != nil {
		return nil, fmt.Errorf("list active entities: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active entities: %w", err)
	}
	return entities, nil
}
