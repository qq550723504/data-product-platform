package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type SnapshotView struct {
	Snapshot
	IntegrityValid bool `json:"integrityValid"`
}

func (r *QueryRepository) GetSnapshot(ctx context.Context, snapshotID uuid.UUID) (SnapshotView, error) {
	var snapshot Snapshot
	var manifest []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, object_type, object_id, manifest, root_hash, created_at, created_by
		FROM evidence_snapshot
		WHERE id=$1 AND status='FINALIZED'
	`, snapshotID).Scan(
		&snapshot.ID,
		&snapshot.WorkspaceID,
		&snapshot.ObjectType,
		&snapshot.ObjectID,
		&manifest,
		&snapshot.RootHash,
		&snapshot.CreatedAt,
		&snapshot.CreatedBy,
	)
	if err != nil {
		return SnapshotView{}, fmt.Errorf("query evidence snapshot %s: %w", snapshotID, err)
	}
	if err := json.Unmarshal(manifest, &snapshot.Manifest); err != nil {
		return SnapshotView{}, fmt.Errorf("decode evidence snapshot manifest: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT evidence_id, category
		FROM evidence_snapshot_item
		WHERE snapshot_id=$1
		ORDER BY category, evidence_id
	`, snapshotID)
	if err != nil {
		return SnapshotView{}, fmt.Errorf("query evidence snapshot items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item SnapshotItem
		if err := rows.Scan(&item.EvidenceID, &item.Category); err != nil {
			return SnapshotView{}, fmt.Errorf("scan evidence snapshot item: %w", err)
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if err := rows.Err(); err != nil {
		return SnapshotView{}, fmt.Errorf("iterate evidence snapshot items: %w", err)
	}

	// Historical snapshots were hashed while evidenceItems was a []SnapshotItem.
	// JSONB decodes nested objects into maps and changes their key order when
	// marshaled again, so reconstruct the original typed representation before
	// recomputing the digest.
	verificationManifest := make(map[string]any, len(snapshot.Manifest))
	for key, value := range snapshot.Manifest {
		verificationManifest[key] = value
	}
	verificationManifest["evidenceItems"] = snapshot.Items
	encoded, err := json.Marshal(verificationManifest)
	if err != nil {
		return SnapshotView{}, fmt.Errorf("marshal evidence snapshot for verification: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return SnapshotView{
		Snapshot:       snapshot,
		IntegrityValid: hex.EncodeToString(digest[:]) == snapshot.RootHash,
	}, nil
}
