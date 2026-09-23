package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
)

type IdempotencyRecord struct {
	ObjectID  uuid.UUID
	ResultRef *uuid.UUID
}

func (r *PostgresRepository) FindIdempotency(ctx context.Context, workspaceID uuid.UUID, commandType, key string) (IdempotencyRecord, bool, error) {
	var record IdempotencyRecord
	err := r.pool.QueryRow(ctx, `
		SELECT object_id, result_ref
		FROM command_idempotency
		WHERE workspace_id=$1 AND command_type=$2 AND idempotency_key=$3
	`, workspaceID, commandType, key).Scan(&record.ObjectID, &record.ResultRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return IdempotencyRecord{}, false, nil
	}
	if err != nil {
		return IdempotencyRecord{}, false, fmt.Errorf("find idempotency record: %w", err)
	}
	return record, true, nil
}

func (r *PostgresRepository) InsertIdempotency(ctx context.Context, tx pgx.Tx, workspaceID uuid.UUID, commandType, key string, objectID uuid.UUID, resultRef *uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO command_idempotency (
			workspace_id, command_type, idempotency_key, object_id, result_ref
		) VALUES ($1,$2,$3,$4,$5)
	`, workspaceID, commandType, key, objectID, resultRef)
	if err != nil {
		return fmt.Errorf("insert idempotency record: %w", err)
	}
	return nil
}

func (r *PostgresRepository) LockReleaseMembershipForPublish(ctx context.Context, tx pgx.Tx, expected domain.ProductRelease) error {
	var status domain.ReleaseStatus
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM product_release
		WHERE id=$1
		FOR UPDATE
	`, expected.ID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lock ProductRelease for publish: %w", err)
	}
	if status != domain.ReleaseReady {
		return domain.ErrReleaseNotReady
	}

	rows, err := tx.Query(ctx, `
		SELECT dataset_version_id, role
		FROM product_release_dataset
		WHERE release_id=$1
		ORDER BY role, dataset_version_id
	`, expected.ID)
	if err != nil {
		return fmt.Errorf("read locked release dataset membership: %w", err)
	}
	defer rows.Close()

	actual := make([]domain.ReleaseDataset, 0, len(expected.Datasets))
	for rows.Next() {
		var binding domain.ReleaseDataset
		if err := rows.Scan(&binding.DatasetVersionID, &binding.Role); err != nil {
			return fmt.Errorf("scan locked release dataset membership: %w", err)
		}
		actual = append(actual, binding)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate locked release dataset membership: %w", err)
	}

	wanted := append([]domain.ReleaseDataset(nil), expected.Datasets...)
	sort.Slice(wanted, func(i, j int) bool {
		if wanted[i].Role == wanted[j].Role {
			return wanted[i].DatasetVersionID.String() < wanted[j].DatasetVersionID.String()
		}
		return wanted[i].Role < wanted[j].Role
	})
	if len(actual) != len(wanted) {
		return fmt.Errorf("release dataset membership changed before publish")
	}
	for i := range wanted {
		if actual[i] != wanted[i] {
			return fmt.Errorf("release dataset membership changed before publish")
		}
	}
	return nil
}

func (r *PostgresRepository) LockReleaseLineageForPublish(ctx context.Context, tx pgx.Tx, releaseID uuid.UUID) error {
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE lineage(version_id) AS (
			SELECT dataset_version_id
			FROM product_release_dataset
			WHERE release_id=$1
			UNION
			SELECT dvl.input_version_id
			FROM dataset_version_lineage dvl
			JOIN lineage l ON l.version_id=dvl.output_version_id
		)
		SELECT dv.id
		FROM dataset_version dv
		JOIN lineage l ON l.version_id=dv.id
		ORDER BY dv.id
		FOR UPDATE
	`, releaseID)
	if err != nil {
		return fmt.Errorf("lock ProductRelease lineage closure: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate ProductRelease lineage closure locks: %w", err)
	}
	return nil
}

func (r *PostgresRepository) PermitReleasePublish(ctx context.Context, tx pgx.Tx, releaseID uuid.UUID) error {
	if releaseID == uuid.Nil {
		return fmt.Errorf("release publish permit requires release id")
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('app.product_release_publish_id', $1, true)`, releaseID.String()); err != nil {
		return fmt.Errorf("set fenced ProductRelease publish permit: %w", err)
	}
	return nil
}

func (r *PostgresRepository) PublishRelease(ctx context.Context, tx pgx.Tx, release domain.ProductRelease, evidenceSnapshotID uuid.UUID, actorID *uuid.UUID) error {
	now := time.Now().UTC()
	commandTag, err := tx.Exec(ctx, `
		UPDATE product_release
		SET status='PUBLISHED', evidence_snapshot_id=$2, released_at=$3, released_by=$4
		WHERE id=$1 AND status='READY'
	`, release.ID, evidenceSnapshotID, now, actorID)
	if err != nil {
		return fmt.Errorf("publish ProductRelease: %w", err)
	}
	if commandTag.RowsAffected() != 1 {
		return domain.ErrInvalidReleaseTransition
	}
	if _, err := tx.Exec(ctx, `
		UPDATE data_product
		SET current_version_id=$2,
		    latest_release_id=$3,
		    lifecycle_status=CASE WHEN lifecycle_status IN ('DRAFT','DESIGNING','DEVELOPING','TESTING','READY') THEN 'PUBLISHED' ELSE lifecycle_status END,
		    updated_at=$4,
		    updated_by=$5
		WHERE id=$1
	`, release.ProductID, release.ProductVersionID, release.ID, now, actorID); err != nil {
		return fmt.Errorf("update DataProduct after release publish: %w", err)
	}
	return nil
}

func (r *PostgresRepository) BuildReleaseEvidenceManifest(ctx context.Context, release domain.ProductRelease, product domain.DataProduct, version domain.ProductVersion) (map[string]any, []evidence.SnapshotItem, error) {
	datasets := make([]map[string]any, 0, len(release.Datasets))
	datasetVersionIDs := make([]uuid.UUID, 0, len(release.Datasets))
	for _, binding := range release.Datasets {
		var datasetID uuid.UUID
		var versionNo int64
		var status string
		var storageURI, checksumAlgorithm, checksumValue string
		var generatedBy *uuid.UUID
		var metadata map[string]any
		err := r.pool.QueryRow(ctx, `
			SELECT dataset_id, version_no, status,
			       COALESCE(storage_uri,''), COALESCE(checksum_algorithm,''), COALESCE(checksum_value,''),
			       generated_by_execution_id, metadata
			FROM dataset_version WHERE id=$1
		`, binding.DatasetVersionID).Scan(&datasetID, &versionNo, &status, &storageURI,
			&checksumAlgorithm, &checksumValue, &generatedBy, &metadata)
		if err != nil {
			return nil, nil, fmt.Errorf("read DatasetVersion for EvidenceSnapshot: %w", err)
		}
		datasetVersionIDs = append(datasetVersionIDs, binding.DatasetVersionID)
		datasets = append(datasets, map[string]any{
			"role":                   binding.Role,
			"datasetVersionId":       binding.DatasetVersionID,
			"datasetId":              datasetID,
			"versionNo":              versionNo,
			"status":                 status,
			"storageUri":             storageURI,
			"checksumAlgorithm":      checksumAlgorithm,
			"checksumValue":          checksumValue,
			"generatedByExecutionId": generatedBy,
			"metadata":               metadata,
		})
	}
	sort.Slice(datasets, func(i, j int) bool {
		left := fmt.Sprint(datasets[i]["role"], datasets[i]["datasetVersionId"])
		right := fmt.Sprint(datasets[j]["role"], datasets[j]["datasetVersionId"])
		return left < right
	})

	manifest := map[string]any{
		"product": map[string]any{
			"id":   product.ID,
			"code": product.Code,
		},
		"productVersion": map[string]any{
			"id":      version.ID,
			"version": version.Semver(),
		},
		"release": map[string]any{
			"id":        release.ID,
			"releaseNo": release.ReleaseNo,
		},
		"datasets": datasets,
	}

	if release.ContractVersionID != nil {
		var status, sourceSHA string
		if err := r.pool.QueryRow(ctx, `SELECT status, source_sha256 FROM contract_version WHERE id=$1`, *release.ContractVersionID).Scan(&status, &sourceSHA); err != nil {
			return nil, nil, fmt.Errorf("read ContractVersion for EvidenceSnapshot: %w", err)
		}
		manifest["contract"] = map[string]any{"id": *release.ContractVersionID, "status": status, "sourceSha256": sourceSHA}
	}
	if release.RightsSnapshotID != nil {
		var rootHash, purpose string
		var asOf time.Time
		if err := r.pool.QueryRow(ctx, `SELECT root_hash, purpose, as_of FROM rights_snapshot WHERE id=$1`, *release.RightsSnapshotID).Scan(&rootHash, &purpose, &asOf); err != nil {
			return nil, nil, fmt.Errorf("read RightsSnapshot for EvidenceSnapshot: %w", err)
		}
		manifest["rights"] = map[string]any{"id": *release.RightsSnapshotID, "rootHash": rootHash, "purpose": purpose, "asOf": asOf}
	}
	if release.QualityResultID != nil {
		var datasetVersionID uuid.UUID
		var decision, ruleSetRef, ruleSetVersion string
		if err := r.pool.QueryRow(ctx, `
			SELECT dataset_version_id, gate_decision, rule_set_ref, rule_set_version
			FROM quality_result WHERE id=$1
		`, *release.QualityResultID).Scan(&datasetVersionID, &decision, &ruleSetRef, &ruleSetVersion); err != nil {
			return nil, nil, fmt.Errorf("read QualityResult for EvidenceSnapshot: %w", err)
		}
		manifest["quality"] = map[string]any{"id": *release.QualityResultID, "datasetVersionId": datasetVersionID, "decision": decision, "ruleSetRef": ruleSetRef, "ruleSetVersion": ruleSetVersion}
	}
	if release.ComplianceResultID != nil {
		var datasetVersionID uuid.UUID
		var decision, policyRef, policyVersion string
		if err := r.pool.QueryRow(ctx, `
			SELECT dataset_version_id, gate_decision, policy_ref, policy_version
			FROM compliance_result WHERE id=$1
		`, *release.ComplianceResultID).Scan(&datasetVersionID, &decision, &policyRef, &policyVersion); err != nil {
			return nil, nil, fmt.Errorf("read ComplianceResult for EvidenceSnapshot: %w", err)
		}
		manifest["compliance"] = map[string]any{"id": *release.ComplianceResultID, "datasetVersionId": datasetVersionID, "decision": decision, "policyRef": policyRef, "policyVersion": policyVersion}
	}

	items := make([]evidence.SnapshotItem, 0)
	if len(datasetVersionIDs) > 0 {
		rows, err := r.pool.Query(ctx, `
			SELECT DISTINCT er.evidence_id, er.relation_type
			FROM evidence_relation er
			WHERE er.object_type='DATASET_VERSION' AND er.object_id = ANY($1::uuid[])
			ORDER BY er.relation_type, er.evidence_id
		`, datasetVersionIDs)
		if err != nil {
			return nil, nil, fmt.Errorf("list release Evidence for snapshot: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var item evidence.SnapshotItem
			if err := rows.Scan(&item.EvidenceID, &item.Category); err != nil {
				return nil, nil, fmt.Errorf("scan release Evidence item: %w", err)
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return nil, nil, fmt.Errorf("iterate release Evidence items: %w", err)
		}
	}
	return manifest, items, nil
}
