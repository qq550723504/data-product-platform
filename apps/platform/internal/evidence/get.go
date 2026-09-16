package evidence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

func (r *QueryRepository) Get(ctx context.Context, evidenceID uuid.UUID) (Item, error) {
	var item Item
	var metadata []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, evidence_type, COALESCE(title,''), COALESCE(source_type,''), source_id,
		       COALESCE(storage_uri,''), COALESCE(hash_algorithm,''), COALESCE(hash_value,''),
		       metadata, created_at, created_by
		FROM evidence
		WHERE id=$1
	`, evidenceID).Scan(
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
		&item.CreatedAt,
		&item.CreatedBy,
	)
	if err != nil {
		return Item{}, fmt.Errorf("query evidence %s: %w", evidenceID, err)
	}
	if len(metadata) > 0 {
		if err := json.Unmarshal(metadata, &item.Metadata); err != nil {
			return Item{}, fmt.Errorf("decode evidence metadata: %w", err)
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
	return item, nil
}
