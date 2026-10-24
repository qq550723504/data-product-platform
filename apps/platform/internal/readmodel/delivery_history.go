package readmodel

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ReadonlyHistory projects explicit non-secret fields. Recovery keys, credentials,
// manifests and provider payloads are never part of this query contract.
func (r *Repository) ReadonlyHistory(ctx context.Context, workspace uuid.UUID, archive bool, selected *uuid.UUID, limit, offset int) (List[json.RawMessage], error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return List[json.RawMessage]{}, err
	}
	defer tx.Rollback(ctx)
	from := ` FROM delivery_operation o WHERE o.workspace_id=$1 AND ($2::uuid IS NULL OR o.id=$2)`
	projection := `jsonb_build_object('id',o.id,'workspaceId',o.workspace_id,'datasetVersionId',o.dataset_version_id,'certificationId',o.certification_ref,'retryOfId',o.retry_of_delivery_operation_id,'status',o.status,'gateDecision',o.current_gate_decision,'purpose',o.purpose,'action',o.action,'channel',o.delivery_channel,'mode',o.delivery_mode,'createdAt',o.created_at,'updatedAt',o.updated_at,
 'evaluations',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',g.id,'stage',g.stage,'decision',g.decision,'createdAt',g.created_at) ORDER BY g.created_at,g.id) FROM delivery_gate_evaluation g WHERE g.delivery_operation_id=o.id),'[]'::jsonb),
 'transitions',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',t.id,'from',t.from_status,'to',t.to_status,'evaluationId',t.gate_evaluation_id,'createdAt',t.created_at) ORDER BY t.created_at,t.id) FROM delivery_transition t WHERE t.delivery_operation_id=o.id),'[]'::jsonb))`
	if archive {
		from = ` FROM evidence_snapshot o WHERE o.workspace_id=$1 AND ($2::uuid IS NULL OR o.id=$2)`
		projection = `jsonb_build_object('id',o.id,'workspaceId',o.workspace_id,'objectType',o.object_type,'objectId',o.object_id,'rootHash',o.root_hash,'createdAt',o.created_at,'itemCount',(SELECT count(*) FROM evidence_snapshot_item i WHERE i.snapshot_id=o.id),'releaseIds',COALESCE((SELECT jsonb_agg(r.id ORDER BY r.id) FROM product_release r JOIN data_product p ON p.id=r.product_id WHERE r.evidence_snapshot_id=o.id AND p.workspace_id=$1),'[]'::jsonb))`
	}
	result := List[json.RawMessage]{Items: []json.RawMessage{}, Page: Page{Limit: limit, Offset: offset}}
	if err = tx.QueryRow(ctx, "SELECT count(*)"+from, workspace, selected).Scan(&result.Page.Total); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, "SELECT "+projection+from+" ORDER BY o.created_at DESC,o.id LIMIT $3 OFFSET $4", workspace, selected, limit, offset)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var row json.RawMessage
		if err = rows.Scan(&row); err != nil {
			rows.Close()
			return result, err
		}
		result.Items = append(result.Items, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
