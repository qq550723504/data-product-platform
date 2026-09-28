package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/certification/domain"
)

func (r *CertificationRepository) ListProfileIDsForDatasetVersion(ctx context.Context, workspaceID, datasetVersionID uuid.UUID, asOf time.Time) ([]uuid.UUID, error) {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT certification_profile_id
		FROM dataset_certification
		WHERE workspace_id=$1 AND dataset_version_id=$2 AND issued_at <= $3
		ORDER BY certification_profile_id
	`, workspaceID, datasetVersionID, asOf.UTC())
	if err != nil {
		return nil, fmt.Errorf("list certification profiles for DatasetVersion: %w", err)
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan certification profile id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate certification profile ids: %w", err)
	}
	return ids, nil
}

type DatasetCertificationHistoryPageRow struct {
	ProfileID     uuid.UUID
	Certification domain.DatasetCertification
}

type DatasetCertificationHistoryPage struct {
	Rows           []DatasetCertificationHistoryPageRow
	Dispositions   []domain.CertificationDisposition
	Total          int
	AnchorRevision int64
}

func (r *CertificationRepository) ListDatasetCertificationHistoryPage(
	ctx context.Context,
	workspaceID, datasetVersionID uuid.UUID,
	asOf time.Time,
	limit, offset int,
	anchorRevision *int64,
) (DatasetCertificationHistoryPage, error) {
	if asOf.IsZero() {
		asOf = time.Now().UTC()
	}
	if limit <= 0 || offset < 0 {
		return DatasetCertificationHistoryPage{}, fmt.Errorf("certification history limit must be positive and offset must be non-negative")
	}

	var currentRevision int64
	if err := r.pool.QueryRow(ctx, `
		SELECT COALESCE((
			SELECT revision
			FROM delivery_authorization_fence
			WHERE workspace_id=$1
		), 0)
	`, workspaceID).Scan(&currentRevision); err != nil {
		return DatasetCertificationHistoryPage{}, fmt.Errorf("read certification history anchor revision: %w", err)
	}
	resolvedAnchor := currentRevision
	if anchorRevision != nil {
		if *anchorRevision < 0 {
			return DatasetCertificationHistoryPage{}, fmt.Errorf("certification history anchor revision must be non-negative")
		}
		if *anchorRevision > currentRevision {
			return DatasetCertificationHistoryPage{}, fmt.Errorf("certification history anchor revision is ahead of committed history")
		}
		resolvedAnchor = *anchorRevision
	}

	page := DatasetCertificationHistoryPage{
		Rows:           make([]DatasetCertificationHistoryPageRow, 0, limit),
		AnchorRevision: resolvedAnchor,
	}
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM dataset_certification
		WHERE workspace_id=$1
		  AND dataset_version_id=$2
		  AND issued_at <= $3
		  AND history_revision <= $4
	`, workspaceID, datasetVersionID, asOf.UTC(), resolvedAnchor).Scan(&page.Total); err != nil {
		return DatasetCertificationHistoryPage{}, fmt.Errorf("count dataset certification history: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT certification_profile_id,
		       id, workspace_id, dataset_version_id, quality_assessment_id,
		       rights_snapshot_id, effective_rights_snapshot_id, effective_rights_snapshot_hash,
		       frozen_rights_context_hash, compliance_result_id, contract_version_id,
		       traceability_evidence_id, evidence_snapshot_id,
		       gold_production_binding_id, annotation_snapshot_id, annotation_snapshot_root_hash,
		       annotation_schema_content_sha256, annotation_taxonomy_content_sha256,
		       gold_production_binding_root_hash,
		       decision, blockers, reason, issued_at, created_by
		FROM dataset_certification
		WHERE workspace_id=$1
		  AND dataset_version_id=$2
		  AND issued_at <= $3
		  AND history_revision <= $4
		ORDER BY issued_at DESC, id DESC
		LIMIT $5 OFFSET $6
	`, workspaceID, datasetVersionID, asOf.UTC(), resolvedAnchor, limit, offset)
	if err != nil {
		return DatasetCertificationHistoryPage{}, fmt.Errorf("list dataset certification history page: %w", err)
	}
	defer rows.Close()

	certificationIDs := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var row DatasetCertificationHistoryPageRow
		var blockers []byte
		var decision string
		var effectiveHash, contextHash *string
		var annotationRootHash, annotationSchemaHash, annotationTaxonomyHash, goldBindingRootHash *string
		certification := &row.Certification
		if err := rows.Scan(
			&row.ProfileID,
			&certification.ID, &certification.WorkspaceID, &certification.DatasetVersionID,
			&certification.QualityAssessmentID, &certification.RightsSnapshotID,
			&certification.EffectiveRightsSnapshotID, &effectiveHash, &contextHash,
			&certification.ComplianceResultID, &certification.ContractVersionID,
			&certification.TraceabilityEvidenceID, &certification.EvidenceSnapshotID,
			&certification.GoldProductionBindingID, &certification.AnnotationSnapshotID,
			&annotationRootHash, &annotationSchemaHash, &annotationTaxonomyHash, &goldBindingRootHash,
			&decision, &blockers, &certification.Reason, &certification.IssuedAt, &certification.ActorID,
		); err != nil {
			return DatasetCertificationHistoryPage{}, fmt.Errorf("scan dataset certification history page: %w", err)
		}
		if err := json.Unmarshal(blockers, &certification.Blockers); err != nil {
			return DatasetCertificationHistoryPage{}, fmt.Errorf("decode dataset certification blockers: %w", err)
		}
		certification.Decision = domain.Decision(decision)
		certification.EffectiveRightsSnapshotHash = dereferenceString(effectiveHash)
		certification.FrozenRightsContextHash = dereferenceString(contextHash)
		certification.AnnotationSnapshotRootHash = dereferenceString(annotationRootHash)
		certification.AnnotationSchemaSHA256 = dereferenceString(annotationSchemaHash)
		certification.AnnotationTaxonomySHA256 = dereferenceString(annotationTaxonomyHash)
		certification.GoldProductionBindingRootHash = dereferenceString(goldBindingRootHash)
		page.Rows = append(page.Rows, row)
		certificationIDs = append(certificationIDs, certification.ID)
	}
	if err := rows.Err(); err != nil {
		return DatasetCertificationHistoryPage{}, fmt.Errorf("iterate dataset certification history page: %w", err)
	}

	if len(certificationIDs) == 0 {
		return page, nil
	}
	dispositionRows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, certification_id, disposition, effective_at,
		       reason, superseded_by_certification_id, evidence_snapshot_id, created_by
		FROM certification_disposition
		WHERE certification_id = ANY($1::uuid[])
		  AND effective_at <= $2
		  AND history_revision <= $3
		ORDER BY certification_id, effective_at, id
	`, certificationIDs, asOf.UTC(), resolvedAnchor)
	if err != nil {
		return DatasetCertificationHistoryPage{}, fmt.Errorf("list certification dispositions for history page: %w", err)
	}
	defer dispositionRows.Close()
	page.Dispositions = make([]domain.CertificationDisposition, 0)
	for dispositionRows.Next() {
		var disposition domain.CertificationDisposition
		var kind string
		if err := dispositionRows.Scan(
			&disposition.ID, &disposition.WorkspaceID, &disposition.CertificationID, &kind,
			&disposition.EffectiveAt, &disposition.Reason, &disposition.SupersededByCertificationID,
			&disposition.EvidenceSnapshotID, &disposition.ActorID,
		); err != nil {
			return DatasetCertificationHistoryPage{}, fmt.Errorf("scan certification disposition for history page: %w", err)
		}
		disposition.Disposition = domain.Disposition(kind)
		page.Dispositions = append(page.Dispositions, disposition)
	}
	if err := dispositionRows.Err(); err != nil {
		return DatasetCertificationHistoryPage{}, fmt.Errorf("iterate certification dispositions for history page: %w", err)
	}
	return page, nil
}
