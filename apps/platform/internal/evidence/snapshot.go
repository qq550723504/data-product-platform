package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SnapshotItem struct {
	EvidenceID uuid.UUID `json:"evidenceId"`
	Category   string    `json:"category"`
}

type Snapshot struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	ObjectType  string
	ObjectID    uuid.UUID
	Manifest    map[string]any
	RootHash    string
	Items       []SnapshotItem
	CreatedAt   time.Time
	CreatedBy   *uuid.UUID
}

func CreateSnapshot(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, objectType string, objectID uuid.UUID, manifest map[string]any, items []SnapshotItem, actorID *uuid.UUID) (Snapshot, error) {
	if workspaceID == uuid.Nil || objectType == "" || objectID == uuid.Nil || manifest == nil {
		return Snapshot{}, fmt.Errorf("invalid evidence snapshot")
	}
	items = append([]SnapshotItem{}, items...)
	for _, item := range items {
		if item.EvidenceID == uuid.Nil || item.Category == "" {
			return Snapshot{}, fmt.Errorf("invalid evidence snapshot item")
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Category == items[j].Category {
			return items[i].EvidenceID.String() < items[j].EvidenceID.String()
		}
		return items[i].Category < items[j].Category
	})
	manifestCopy := make(map[string]any, len(manifest)+1)
	for key, value := range manifest {
		manifestCopy[key] = value
	}
	manifestCopy["evidenceItems"] = items
	encoded, err := json.Marshal(manifestCopy)
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal evidence snapshot manifest: %w", err)
	}
	digest := sha256.Sum256(encoded)
	snapshot := Snapshot{
		ID:          uuid.New(),
		WorkspaceID: workspaceID,
		ObjectType:  objectType,
		ObjectID:    objectID,
		Manifest:    manifestCopy,
		RootHash:    hex.EncodeToString(digest[:]),
		Items:       items,
		CreatedAt:   time.Now().UTC(),
		CreatedBy:   actorID,
	}

	manifestJSON, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal stored evidence snapshot manifest: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO evidence_snapshot (
			id, workspace_id, object_type, object_id, manifest, root_hash,
			manifest_hash_payload, status, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'BUILDING',$8,$9)
	`, snapshot.ID, snapshot.WorkspaceID, snapshot.ObjectType, snapshot.ObjectID,
		manifestJSON, snapshot.RootHash, encoded, snapshot.CreatedAt, snapshot.CreatedBy); err != nil {
		return Snapshot{}, fmt.Errorf("insert evidence snapshot: %w", err)
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO evidence_snapshot_item (snapshot_id, evidence_id, category)
			VALUES ($1,$2,$3)
		`, snapshot.ID, item.EvidenceID, item.Category); err != nil {
			return Snapshot{}, fmt.Errorf("insert evidence snapshot item: %w", err)
		}
	}
	tag, err := tx.Exec(ctx, `
		UPDATE evidence_snapshot
		SET status='FINALIZED'
		WHERE id=$1 AND status='BUILDING'
	`, snapshot.ID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("finalize evidence snapshot: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return Snapshot{}, fmt.Errorf("finalize evidence snapshot: expected one BUILDING row")
	}
	return snapshot, nil
}
