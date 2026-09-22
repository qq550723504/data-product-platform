package evidence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/uuid"
)

type SnapshotView struct {
	Snapshot
	IntegrityValid bool `json:"integrityValid"`
}

func (r *QueryRepository) GetSnapshot(ctx context.Context, snapshotID uuid.UUID) (SnapshotView, error) {
	var snapshot Snapshot
	var manifest []byte
	var hashPayload []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, object_type, object_id, manifest, root_hash,
		       manifest_hash_payload, created_at, created_by
		FROM evidence_snapshot
		WHERE id=$1 AND status='FINALIZED'
	`, snapshotID).Scan(
		&snapshot.ID,
		&snapshot.WorkspaceID,
		&snapshot.ObjectType,
		&snapshot.ObjectID,
		&manifest,
		&snapshot.RootHash,
		&hashPayload,
		&snapshot.CreatedAt,
		&snapshot.CreatedBy,
	)
	if err != nil {
		return SnapshotView{}, fmt.Errorf("query evidence snapshot %s: %w", snapshotID, err)
	}
	if err := decodeSnapshotJSON(manifest, &snapshot.Manifest); err != nil {
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

	// The membership table is part of the frozen aggregate, so verification
	// always reconstructs evidenceItems from persisted child rows.
	verificationManifest := make(map[string]any, len(snapshot.Manifest))
	for key, value := range snapshot.Manifest {
		verificationManifest[key] = value
	}
	verificationManifest["evidenceItems"] = snapshot.Items

	integrityValid, err := verifySnapshotIntegrity(snapshot.RootHash, hashPayload, verificationManifest)
	if err != nil {
		return SnapshotView{}, err
	}
	return SnapshotView{
		Snapshot:       snapshot,
		IntegrityValid: integrityValid,
	}, nil
}

func verifySnapshotIntegrity(rootHash string, hashPayload []byte, verificationManifest map[string]any) (bool, error) {
	// Snapshots created after migration 000030 preserve the exact bytes used to
	// derive root_hash. Verify both the digest and semantic equality with the
	// manifest reconstructed from frozen membership.
	if len(hashPayload) > 0 {
		var payloadValue any
		if err := decodeSnapshotJSON(hashPayload, &payloadValue); err != nil {
			return false, fmt.Errorf("decode evidence snapshot hash payload: %w", err)
		}
		verificationJSON, err := json.Marshal(verificationManifest)
		if err != nil {
			return false, fmt.Errorf("marshal evidence snapshot for verification: %w", err)
		}
		var verificationValue any
		if err := decodeSnapshotJSON(verificationJSON, &verificationValue); err != nil {
			return false, fmt.Errorf("decode reconstructed evidence snapshot manifest: %w", err)
		}
		digest := sha256.Sum256(hashPayload)
		return hex.EncodeToString(digest[:]) == rootHash && reflect.DeepEqual(payloadValue, verificationValue), nil
	}

	// Historical snapshots predate manifest_hash_payload. Preserve their
	// established verification contract by rebuilding the typed evidenceItems
	// representation before hashing.
	encoded, err := json.Marshal(verificationManifest)
	if err != nil {
		return false, fmt.Errorf("marshal evidence snapshot for verification: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]) == rootHash, nil
}

func decodeSnapshotJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	return decoder.Decode(target)
}
