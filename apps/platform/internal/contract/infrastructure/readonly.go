package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
)

func (r *PostgresRepository) ReadAcceptanceSources(ctx context.Context, query q.Query) (q.Snapshot, error) {
	result := q.Snapshot{Versions: []q.StoredVersion{}}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if query.DeliveryID != nil {
		d := &q.Delivery{}
		err = tx.QueryRow(ctx, `SELECT o.id,o.dataset_version_id,o.status FROM delivery_operation o JOIN dataset_version v ON v.id=o.dataset_version_id JOIN dataset d ON d.id=v.dataset_id WHERE o.workspace_id=$1 AND d.workspace_id=$1 AND o.id=$2 AND o.dataset_version_id=$3`, query.WorkspaceID, query.DeliveryID, query.DatasetVersionID).Scan(&d.ID, &d.DatasetVersionID, &d.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			return result, q.ErrNotFound
		}
		if err != nil {
			return result, err
		}
		result.Delivery = d
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
