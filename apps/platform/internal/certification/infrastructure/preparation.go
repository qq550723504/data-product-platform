package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
)

type PreparationFacts struct {
	ExecutionID  uuid.UUID
	ComplianceID uuid.UUID
	ContractID   uuid.UUID
	Items        []evidence.SnapshotItem
}

// Preparation only collects real persisted facts. Ambiguous prerequisites are
// rejected instead of choosing a contract or compliance outcome by timestamp.
func (r *CertificationRepository) PreparationFacts(ctx context.Context, tx pgx.Tx, ws, version, quality uuid.UUID, contractCode string) (PreparationFacts, error) {
	var f PreparationFacts
	err := tx.QueryRow(ctx, `SELECT e.id FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id JOIN execution e ON e.id=v.generated_by_execution_id JOIN quality_result q ON q.dataset_version_id=v.id
		WHERE v.id=$1 AND d.workspace_id=$2 AND q.id=$3 AND q.workspace_id=$2 AND e.workspace_id=$2 AND e.status='SUCCEEDED' AND e.output_dataset_version_id=v.id AND v.status IN ('READY','SUPERSEDED')`, version, ws, quality).Scan(&f.ExecutionID)
	if err != nil {
		return f, fmt.Errorf("real completed execution and quality required: %w", err)
	}
	one := func(query string, args ...any) (uuid.UUID, error) {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return uuid.Nil, err
		}
		defer rows.Close()
		ids := []uuid.UUID{}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return uuid.Nil, err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			return uuid.Nil, err
		}
		if len(ids) != 1 {
			return uuid.Nil, fmt.Errorf("exactly one explicit prerequisite required; found %d", len(ids))
		}
		return ids[0], nil
	}
	f.ComplianceID, err = one(`SELECT id FROM compliance_result WHERE workspace_id=$1 AND dataset_version_id=$2 ORDER BY id LIMIT 2`, ws, version)
	if err != nil {
		return f, err
	}
	f.ContractID, err = one(`SELECT v.id FROM contract_version v JOIN data_contract c ON c.id=v.contract_id WHERE c.workspace_id=$1 AND c.code=$2 AND v.status='PUBLISHED' ORDER BY v.id LIMIT 2`, ws, contractCode)
	if err != nil {
		return f, err
	}
	rows, err := tx.Query(ctx, `SELECT id,evidence_type FROM evidence WHERE workspace_id=$1 AND ((source_type='EXECUTION' AND source_id=$2) OR (source_type='QUALITY_RESULT' AND source_id=$3) OR (source_type='COMPLIANCE_RESULT' AND source_id=$4)) ORDER BY id LIMIT 101`, ws, f.ExecutionID, quality, f.ComplianceID)
	if err != nil {
		return f, err
	}
	defer rows.Close()
	for rows.Next() {
		var item evidence.SnapshotItem
		if err := rows.Scan(&item.EvidenceID, &item.Category); err != nil {
			return f, err
		}
		f.Items = append(f.Items, item)
	}
	if err := rows.Err(); err != nil {
		return f, err
	}
	if len(f.Items) == 0 || len(f.Items) > 100 {
		return f, fmt.Errorf("bounded execution evidence required")
	}
	return f, nil
}

func (r *CertificationRepository) PreparedInputs(ctx context.Context, ws, version, profile, quality uuid.UUID) (map[string]string, error) {
	var data []byte
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id,manifest->'references' FROM evidence_snapshot WHERE workspace_id=$1 AND object_type='DATASET_VERSION' AND object_id=$2 AND status='FINALIZED' AND manifest->>'profileId'=$3 AND manifest->>'qualityAssessmentId'=$4 AND manifest->>'preparationType'='CERTIFICATION_INPUTS' ORDER BY created_at DESC,id DESC LIMIT 1`, ws, version, profile.String(), quality.String()).Scan(&id, &data)
	if err == pgx.ErrNoRows {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var refs map[string]string
	err = json.Unmarshal(data, &refs)
	if err == nil {
		refs["traceabilityEvidenceId"] = id.String()
		refs["evidenceSnapshotId"] = id.String()
	}
	return refs, err
}
