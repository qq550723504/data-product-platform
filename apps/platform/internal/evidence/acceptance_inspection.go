package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"
)

// IntegrityReference is a safe projection. Envelope integrity says nothing
// about the bytes at StorageURI, recipient authority, or receipt of a delivery.
type IntegrityReference struct {
	ID             uuid.UUID  `json:"id"`
	WorkspaceID    uuid.UUID  `json:"workspaceId"`
	EnvelopeStatus string     `json:"envelopeStatus"`
	EnvelopeSHA256 string     `json:"envelopeSha256"`
	RecordedAt     *time.Time `json:"recordedAt"`
	RecordedBy     *uuid.UUID `json:"recordedBy"`
	ClaimKind      string     `json:"claimKind"`
}

// InspectAcceptanceReferences reuses the evidence envelope hash implementation
// on the caller's connection. No metadata/title/URI is returned to callers.
func (r *QueryRepository) InspectAcceptanceReferences(ctx context.Context, ws uuid.UUID, ids []uuid.UUID) ([]IntegrityReference, error) {
	if ws == uuid.Nil || len(ids) > 100 {
		return nil, errors.New("bounded scoped evidence references required")
	}
	out := make([]IntegrityReference, 0, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `WITH scoped AS (
 SELECT *,octet_length(COALESCE(title,''))+octet_length(COALESCE(source_type,''))+octet_length(COALESCE(storage_uri,''))+octet_length(COALESCE(metadata,'{}'::jsonb)::text)+octet_length(evidence_type)+octet_length(COALESCE(hash_algorithm,''))+octet_length(COALESCE(hash_value,''))<=65536 AS bounded
 FROM evidence WHERE workspace_id=$1 AND id=ANY($2::uuid[]))
 SELECT id,workspace_id,created_at,created_by,bounded,
 CASE WHEN bounded THEN evidence_type ELSE '' END,CASE WHEN bounded THEN COALESCE(title,'') ELSE '' END,
 CASE WHEN bounded THEN COALESCE(source_type,'') ELSE '' END,source_id,
 CASE WHEN bounded THEN COALESCE(storage_uri,'') ELSE '' END,
 CASE WHEN bounded THEN COALESCE(hash_algorithm,'') ELSE '' END,CASE WHEN bounded THEN COALESCE(hash_value,'') ELSE '' END,
 CASE WHEN bounded THEN COALESCE(metadata,'{}'::jsonb) ELSE '{}'::jsonb END FROM scoped ORDER BY id`, ws, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	found := map[uuid.UUID]IntegrityReference{}
	for rows.Next() {
		var rec Record
		var bounded bool
		var algorithm, hash string
		var metadata []byte
		if err = rows.Scan(&rec.ID, &rec.WorkspaceID, &rec.CreatedAt, &rec.CreatedBy, &bounded, &rec.EvidenceType, &rec.Title, &rec.SourceType, &rec.SourceID, &rec.StorageURI, &algorithm, &hash, &metadata); err != nil {
			return nil, err
		}
		if err = decodeMetadata(metadata, &rec.Metadata); err != nil {
			return nil, err
		}
		v := inspectRecord(rec, bounded, algorithm, hash)
		found[v.ID] = v
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		v, ok := found[id]
		if !ok {
			v = IntegrityReference{ID: id, WorkspaceID: ws, EnvelopeStatus: "MISSING", ClaimKind: "UNCLASSIFIED"}
		}
		out = append(out, v)
	}
	return out, nil
}

func inspectRecord(rec Record, bounded bool, algorithm, hash string) IntegrityReference {
	v := IntegrityReference{ID: rec.ID, WorkspaceID: rec.WorkspaceID, RecordedAt: &rec.CreatedAt, RecordedBy: rec.CreatedBy, EnvelopeStatus: "NOT_CHECKED", ClaimKind: "UNCLASSIFIED"}
	if !bounded {
		return v
	}
	v.EnvelopeStatus = "INVALID"
	if algorithm == HashAlgorithmEvidenceV2 && VerifyHash(rec, algorithm, hash) {
		v.EnvelopeStatus = "VALID"
		v.EnvelopeSHA256 = strings.ToLower(strings.TrimSpace(hash))
	}
	switch rec.EvidenceType {
	case "RECEIPT_ATTESTATION", "BUSINESS_RECEIPT_MATERIAL":
		v.ClaimKind = "RECEIPT_CLAIM"
	case "QUALITY_RESULT", "COMPLIANCE_RESULT", "DATASET_CERTIFICATION", "RELEASE_PUBLISHED", "DELIVERY_ISSUED", "EVIDENCE_SNAPSHOT":
		v.ClaimKind = "GOVERNANCE_RESULT"
	}
	return v
}

type AcceptanceReferencePage struct {
	Items []IntegrityReference
	Total int
}

// An exact target pair is mandatory. A generic version relation cannot expose
// another delivery's evidence. Relation strings/claims never confer trust.
func (r *QueryRepository) AcceptanceMaterialReferences(ctx context.Context, ws, delivery, version uuid.UUID, offset int, selected *uuid.UUID) (AcceptanceReferencePage, error) {
	result := AcceptanceReferencePage{Items: []IntegrityReference{}}
	if ws == uuid.Nil || delivery == uuid.Nil || version == uuid.Nil || offset < 0 || offset > 2147483647 {
		return result, errors.New("explicit bounded target required")
	}
	var payload []byte
	err := r.pool.QueryRow(ctx, `WITH scoped AS MATERIALIZED (
 SELECT e.*,octet_length(COALESCE(e.title,''))+octet_length(COALESCE(e.source_type,''))+octet_length(COALESCE(e.storage_uri,''))+octet_length(COALESCE(e.metadata,'{}'::jsonb)::text)+octet_length(e.evidence_type)+octet_length(COALESCE(e.hash_algorithm,''))+octet_length(COALESCE(e.hash_value,''))<=65536 AS bounded FROM evidence e
 JOIN delivery_operation o ON o.id=$2 AND o.workspace_id=$1 AND o.dataset_version_id=$3
 JOIN dataset_version v ON v.id=o.dataset_version_id JOIN dataset d ON d.id=v.dataset_id AND d.workspace_id=$1
 WHERE e.workspace_id=$1 AND ($5::uuid IS NULL OR e.id=$5)
 AND EXISTS(SELECT 1 FROM evidence_relation er WHERE er.evidence_id=e.id AND er.object_type='DELIVERY_OPERATION' AND er.object_id=$2)
 AND EXISTS(SELECT 1 FROM evidence_relation er WHERE er.evidence_id=e.id AND er.object_type='DATASET_VERSION' AND er.object_id=$3)
 ), page AS(SELECT * FROM scoped ORDER BY created_at DESC,id LIMIT 25 OFFSET $4)
 SELECT (SELECT count(*) FROM scoped),COALESCE(jsonb_agg(jsonb_build_object(
 'record',jsonb_build_object('ID',id,'WorkspaceID',workspace_id,'CreatedAt',created_at,'CreatedBy',created_by,'SourceID',source_id,
 'EvidenceType',CASE WHEN bounded THEN evidence_type ELSE '' END,'Title',CASE WHEN bounded THEN COALESCE(title,'') ELSE '' END,
 'SourceType',CASE WHEN bounded THEN COALESCE(source_type,'') ELSE '' END,'StorageURI',CASE WHEN bounded THEN COALESCE(storage_uri,'') ELSE '' END,
 'Metadata',CASE WHEN bounded THEN COALESCE(metadata,'{}'::jsonb) ELSE '{}'::jsonb END),
 'bounded',bounded,'algorithm',CASE WHEN bounded THEN COALESCE(hash_algorithm,'') ELSE '' END,'hash',CASE WHEN bounded THEN COALESCE(hash_value,'') ELSE '' END)
 ORDER BY created_at DESC,id),'[]'::jsonb) FROM page`, ws, delivery, version, offset, selected).Scan(&result.Total, &payload)
	if err != nil {
		return result, err
	}
	if selected != nil && result.Total != 1 {
		return result, pgx.ErrNoRows
	}
	var records []struct {
		Record          Record
		Bounded         bool
		Algorithm, Hash string
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	if err = decoder.Decode(&records); err != nil {
		return result, err
	}
	for _, rec := range records {
		result.Items = append(result.Items, inspectRecord(rec.Record, rec.Bounded, rec.Algorithm, rec.Hash))
	}
	return result, nil
}
