package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID            uuid.UUID      `json:"id"`
	EvidenceType  string         `json:"evidenceType"`
	Title         string         `json:"title,omitempty"`
	SourceType    string         `json:"sourceType,omitempty"`
	SourceID      *uuid.UUID     `json:"sourceId,omitempty"`
	StorageURI    string         `json:"storageUri,omitempty"`
	HashAlgorithm string         `json:"hashAlgorithm,omitempty"`
	HashValue     string         `json:"hashValue,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	RelationType  string         `json:"relationType"`
	CreatedAt     time.Time      `json:"createdAt"`
	CreatedBy     *uuid.UUID     `json:"createdBy,omitempty"`
}

type QueryRepository struct {
	pool *pgxpool.Pool
}

func NewQueryRepository(pool *pgxpool.Pool) *QueryRepository {
	return &QueryRepository{pool: pool}
}

func (r *QueryRepository) ListForObject(ctx context.Context, objectType string, objectID uuid.UUID) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.evidence_type, COALESCE(e.title,''), COALESCE(e.source_type,''), e.source_id,
		       COALESCE(e.storage_uri,''), COALESCE(e.hash_algorithm,''), COALESCE(e.hash_value,''),
		       e.metadata, er.relation_type, e.created_at, e.created_by
		FROM evidence_relation er
		JOIN evidence e ON e.id = er.evidence_id
		WHERE er.object_type = $1 AND er.object_id = $2
		ORDER BY e.created_at, e.id
	`, objectType, objectID)
	if err != nil {
		return nil, fmt.Errorf("query evidence for %s %s: %w", objectType, objectID, err)
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		var item Item
		var metadata []byte
		if err := rows.Scan(
			&item.ID,
			&item.EvidenceType,
			&item.Title,
			&item.SourceType,
			&item.SourceID,
			&item.StorageURI,
			&item.HashAlgorithm,
			&item.HashValue,
			&metadata,
			&item.RelationType,
			&item.CreatedAt,
			&item.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("scan evidence query result: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &item.Metadata); err != nil {
				return nil, fmt.Errorf("decode evidence metadata: %w", err)
			}
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evidence query results: %w", err)
	}
	return items, nil
}
