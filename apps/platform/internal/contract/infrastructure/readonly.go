package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
)

func (r *PostgresRepository) ReadAcceptanceSources(ctx context.Context, query q.Query) (q.Snapshot, error) {
	result := q.Snapshot{Versions: []q.StoredVersion{}}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&result.EvaluatedAt); err != nil {
		return result, err
	}
	if query.DeliveryID != nil {
		d := &q.Delivery{}
		err = tx.QueryRow(ctx, `SELECT o.id,o.dataset_version_id,o.status,o.effective_consumer_ref FROM delivery_operation o JOIN dataset_version v ON v.id=o.dataset_version_id JOIN dataset d ON d.id=v.dataset_id WHERE o.workspace_id=$1 AND d.workspace_id=$1 AND o.id=$2 AND o.dataset_version_id=$3`, query.WorkspaceID, query.DeliveryID, query.DatasetVersionID).Scan(&d.ID, &d.DatasetVersionID, &d.Status, &d.ConsumerRef)
		if errors.Is(err, pgx.ErrNoRows) {
			return result, q.ErrNotFound
		}
		if err != nil {
			return result, err
		}
		result.Delivery = d
		result.Bindings, err = readBindingFacts(ctx, tx, query)
		if err != nil {
			return result, err
		}
	}
	const from = ` FROM contract_version v JOIN data_contract c ON c.id=v.contract_id WHERE c.workspace_id=$1 AND ($2::uuid IS NULL OR v.id=$2)`
	if err = tx.QueryRow(ctx, `SELECT count(*)`+from, query.WorkspaceID, query.VersionID).Scan(&result.Total); err != nil {
		return result, err
	}
	if query.VersionID != nil && result.Total == 0 {
		return result, q.ErrNotFound
	}
	rows, err := tx.Query(ctx, `SELECT c.workspace_id,c.name,v.id,v.contract_id,v.major_version,v.minor_version,v.patch_version,v.status,v.source_sha256,v.document,v.created_at,v.published_at`+from+` ORDER BY v.created_at DESC,v.id LIMIT $3 OFFSET $4`, query.WorkspaceID, query.VersionID, query.Limit, query.Offset)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var v q.StoredVersion
		var document []byte
		if err = rows.Scan(&v.WorkspaceID, &v.Name, &v.Version.ID, &v.Version.ContractID, &v.Version.MajorVersion, &v.Version.MinorVersion, &v.Version.PatchVersion, &v.Version.Status, &v.Version.SourceSHA256, &document, &v.Version.CreatedAt, &v.Version.PublishedAt); err != nil {
			rows.Close()
			return result, err
		}
		if err = json.Unmarshal(document, &v.Version.Document); err != nil {
			rows.Close()
			return result, err
		}
		result.Versions = append(result.Versions, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

// Dependencies and dispositions are selected from the same repeatable-read
// snapshot. Only representatives referenced by this exact target are visible.
func readBindingFacts(ctx context.Context, tx pgx.Tx, query q.Query) ([]domain.AcceptanceBindingFact, error) {
	const scoped = `WITH r AS (SELECT * FROM acceptance_responsibility_binding WHERE workspace_id=$1 AND delivery_operation_id=$2 AND dataset_version_id=$3), a AS (SELECT * FROM acceptance_standard_applicability WHERE workspace_id=$1 AND delivery_operation_id=$2 AND dataset_version_id=$3), p AS (SELECT p.* FROM acceptance_party_representation p WHERE p.workspace_id=$1 AND p.id IN(SELECT representation_id FROM r)), d AS (SELECT d.* FROM acceptance_binding_disposition d WHERE d.workspace_id=$1 AND (d.representation_id IN(SELECT id FROM p) OR d.responsibility_id IN(SELECT id FROM r) OR d.applicability_id IN(SELECT id FROM a))) `
	base := `'id',id,'workspaceId',workspace_id,'basisEvidenceId',basis_evidence_id,'recordedBy',recorded_by,'createdAt',created_at`
	sql := scoped + `SELECT jsonb_build_object(` + base + `,'kind','REPRESENTATION','origin',origin,'validFrom',valid_from,'validTo',valid_to,'actorId',actor_id,'subject',principal_subject,'partyRef',party_ref) FROM p UNION ALL SELECT jsonb_build_object(` + base + `,'kind','RESPONSIBILITY','origin',origin,'validFrom',valid_from,'validTo',valid_to,'deliveryOperationId',delivery_operation_id,'datasetVersionId',dataset_version_id,'role',role,'representationId',representation_id) FROM r UNION ALL SELECT jsonb_build_object(` + base + `,'kind','APPLICABILITY','origin',origin,'validFrom',valid_from,'validTo',valid_to,'deliveryOperationId',delivery_operation_id,'datasetVersionId',dataset_version_id,'contractVersionId',contract_version_id,'sourceSha256',source_sha256,'criteriaSetHash',criteria_set_hash,'confirmerBindingId',confirmer_binding_id) FROM a UNION ALL SELECT jsonb_build_object(` + base + `,'kind','DISPOSITION','targetId',coalesce(representation_id,responsibility_id,applicability_id),'targetKind',CASE WHEN representation_id IS NOT NULL THEN 'REPRESENTATION' WHEN responsibility_id IS NOT NULL THEN 'RESPONSIBILITY' ELSE 'APPLICABILITY' END,'disposition',disposition,'effectiveAt',effective_at,'reason',reason) FROM d`
	rows, err := tx.Query(ctx, sql, query.WorkspaceID, query.DeliveryID, query.DatasetVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := []domain.AcceptanceBindingFact{}
	for rows.Next() {
		var raw []byte
		var f domain.AcceptanceBindingFact
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}
