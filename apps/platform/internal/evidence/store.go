package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Record struct {
	ID           uuid.UUID
	WorkspaceID  uuid.UUID
	EvidenceType string
	Title        string
	SourceType   string
	SourceID     *uuid.UUID
	StorageURI   string
	Metadata     map[string]any
	CreatedAt    time.Time
	CreatedBy    *uuid.UUID
}

type Relation struct {
	ObjectType   string
	ObjectID     uuid.UUID
	RelationType string
}

func Append(ctx context.Context, tx pgx.Tx, record Record, relations ...Relation) (Record, error) {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}

	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return Record{}, fmt.Errorf("marshal evidence metadata: %w", err)
	}
	digest := sha256.Sum256(metadata)
	hashValue := hex.EncodeToString(digest[:])

	_, err = tx.Exec(ctx, `
		INSERT INTO evidence (
			id, workspace_id, evidence_type, title, source_type, source_id,
			storage_uri, hash_algorithm, hash_value, metadata, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'SHA256',$8,$9,$10,$11)
	`, record.ID, record.WorkspaceID, record.EvidenceType, record.Title, record.SourceType, record.SourceID,
		record.StorageURI, hashValue, metadata, record.CreatedAt, record.CreatedBy)
	if err != nil {
		return Record{}, fmt.Errorf("insert evidence: %w", err)
	}

	for _, relation := range relations {
		if relation.ObjectID == uuid.Nil || relation.ObjectType == "" || relation.RelationType == "" {
			return Record{}, fmt.Errorf("invalid evidence relation")
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO evidence_relation (evidence_id, object_type, object_id, relation_type)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT DO NOTHING
		`, record.ID, relation.ObjectType, relation.ObjectID, relation.RelationType); err != nil {
			return Record{}, fmt.Errorf("insert evidence relation: %w", err)
		}
	}
	return record, nil
}
