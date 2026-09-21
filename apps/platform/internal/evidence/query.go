package evidence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID             uuid.UUID      `json:"id"`
	WorkspaceID    uuid.UUID      `json:"workspaceId"`
	EvidenceType   string         `json:"evidenceType"`
	Title          string         `json:"title,omitempty"`
	SourceType     string         `json:"sourceType,omitempty"`
	SourceID       *uuid.UUID     `json:"sourceId,omitempty"`
	StorageURI     string         `json:"storageUri,omitempty"`
	HashAlgorithm  string         `json:"hashAlgorithm,omitempty"`
	HashValue      string         `json:"hashValue,omitempty"`
	IntegrityValid bool           `json:"integrityValid"`
	Metadata       map[string]any `json:"metadata"`
	RelationType   string         `json:"relationType"`
	CreatedAt      time.Time      `json:"createdAt"`
	CreatedBy      *uuid.UUID     `json:"createdBy,omitempty"`
}

type QueryRepository struct {
	pool *pgxpool.Pool
}

func NewQueryRepository(pool *pgxpool.Pool) *QueryRepository {
	return &QueryRepository{pool: pool}
}

func (r *QueryRepository) ListForObject(ctx context.Context, objectType string, objectID uuid.UUID) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.workspace_id, e.evidence_type, COALESCE(e.title,''), COALESCE(e.source_type,''), e.source_id,
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
			&item.WorkspaceID,
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
			if err := decodeMetadataForHash(metadata, item.HashAlgorithm, &item.Metadata); err != nil {
				return nil, fmt.Errorf("decode evidence metadata: %w", err)
			}
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		item.IntegrityValid = VerifyHash(Record{
			ID:           item.ID,
			WorkspaceID:  item.WorkspaceID,
			EvidenceType: item.EvidenceType,
			Title:        item.Title,
			SourceType:   item.SourceType,
			SourceID:     item.SourceID,
			StorageURI:   item.StorageURI,
			Metadata:     item.Metadata,
			CreatedAt:    item.CreatedAt,
			CreatedBy:    item.CreatedBy,
		}, item.HashAlgorithm, item.HashValue)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate evidence query results: %w", err)
	}
	return items, nil
}

// HasRelationForObject is the small provenance port used by quality rules that
// need to verify evidence existence without loading unbounded evidence metadata.
func (r *QueryRepository) HasRelationForObject(ctx context.Context, objectType string, objectID uuid.UUID) (bool, error) {
	var present bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM evidence_relation
			WHERE object_type = $1 AND object_id = $2
		)
	`, objectType, objectID).Scan(&present); err != nil {
		return false, fmt.Errorf("check evidence for %s %s: %w", objectType, objectID, err)
	}
	return present, nil
}

// HasSupportingEvidenceForObject excludes governance results produced by the
// quality/compliance checks themselves, preventing a rerun from using its own
// previous assessment as production traceability proof.
func (r *QueryRepository) HasSupportingEvidenceForObject(ctx context.Context, objectType string, objectID uuid.UUID) (bool, error) {
	var present bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM evidence_relation er
			JOIN evidence e ON e.id = er.evidence_id
			WHERE er.object_type = $1
			  AND er.object_id = $2
			  AND e.evidence_type NOT IN (
				  'QUALITY_RESULT',
				  'COMPLIANCE_RESULT',
				  'QUALITY_ASSESSMENT_ATTEMPT_FAILED'
			  )
		)
	`, objectType, objectID).Scan(&present); err != nil {
		return false, fmt.Errorf("check supporting evidence for %s %s: %w", objectType, objectID, err)
	}
	return present, nil
}
