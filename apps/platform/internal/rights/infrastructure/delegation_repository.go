package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

var ErrDelegationChainIdempotentReplay = errors.New("delegation chain idempotent replay")
var ErrDelegationFinalizeIdempotentReplay = errors.New("delegation finalization idempotent replay")
var ErrDelegationDispositionIdempotentReplay = errors.New("delegation disposition idempotent replay")

func (r *PostgresRepository) InsertDelegationChain(ctx context.Context, tx pgx.Tx, chain domain.DelegationChain) error {
	var workspace uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM rights_declaration WHERE id=$1`, chain.SourceDeclarationID).Scan(&workspace); err != nil {
		return err
	}
	if workspace != chain.WorkspaceID {
		return domain.ErrResourceWorkspace
	}
	tag, err := tx.Exec(ctx, `INSERT INTO grantor_authority_delegation_chain(id,workspace_id,source_declaration_id,status,created_at,created_by) VALUES($1,$2,$3,'DRAFT',$4,$5) ON CONFLICT (id) DO NOTHING`, chain.ID, chain.WorkspaceID, chain.SourceDeclarationID, chain.CreatedAt, chain.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert delegation chain: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrDelegationChainIdempotentReplay
	}
	for _, edge := range chain.Edges {
		if _, err := tx.Exec(ctx, `INSERT INTO grantor_authority_delegation_edge(id,chain_id,ordinal,delegator_ref,delegate_ref,data_resource_id,grantable_actions,grantable_purposes,scope_type,scope_ref,valid_from,valid_to) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, edge.ID, chain.ID, edge.Ordinal, edge.DelegatorRef, edge.DelegateRef, edge.DataResourceID, edge.GrantableActions, edge.GrantablePurposes, edge.Scope.Type, edge.Scope.Ref, edge.ValidFrom, edge.ValidTo); err != nil {
			return fmt.Errorf("insert delegation edge: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) GetDelegationChain(ctx context.Context, id uuid.UUID) (domain.DelegationChain, error) {
	var chain domain.DelegationChain
	err := r.pool.QueryRow(ctx, `SELECT id,workspace_id,source_declaration_id,status,COALESCE(chain_hash,''),created_at,created_by FROM grantor_authority_delegation_chain WHERE id=$1`, id).Scan(&chain.ID, &chain.WorkspaceID, &chain.SourceDeclarationID, &chain.Status, &chain.ChainHash, &chain.CreatedAt, &chain.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return chain, ErrNotFound
	}
	if err != nil {
		return chain, err
	}
	rows, err := r.pool.Query(ctx, `SELECT id,ordinal,delegator_ref,delegate_ref,data_resource_id,grantable_actions,grantable_purposes,scope_type,scope_ref,valid_from,valid_to FROM grantor_authority_delegation_edge WHERE chain_id=$1 ORDER BY ordinal`, id)
	if err != nil {
		return chain, err
	}
	defer rows.Close()
	for rows.Next() {
		var edge domain.DelegationEdge
		if err := rows.Scan(&edge.ID, &edge.Ordinal, &edge.DelegatorRef, &edge.DelegateRef, &edge.DataResourceID, &edge.GrantableActions, &edge.GrantablePurposes, &edge.Scope.Type, &edge.Scope.Ref, &edge.ValidFrom, &edge.ValidTo); err != nil {
			return chain, err
		}
		chain.Edges = append(chain.Edges, edge)
	}
	return chain, rows.Err()
}

func (r *PostgresRepository) FinalizeDelegationChain(ctx context.Context, tx pgx.Tx, chainID uuid.UUID) (domain.DelegationChain, error) {
	var chain domain.DelegationChain
	var status string
	if err := tx.QueryRow(ctx, `SELECT id,workspace_id,source_declaration_id,status,COALESCE(chain_hash,''),created_at,created_by FROM grantor_authority_delegation_chain WHERE id=$1 FOR UPDATE`, chainID).Scan(&chain.ID, &chain.WorkspaceID, &chain.SourceDeclarationID, &status, &chain.ChainHash, &chain.CreatedAt, &chain.CreatedBy); err != nil {
		return chain, err
	}
	if status == "FINALIZED" {
		chain.Status = status
		return chain, ErrDelegationFinalizeIdempotentReplay
	}
	if status != "DRAFT" {
		return chain, domain.ErrInvalidBinding
	}
	rows, err := tx.Query(ctx, `SELECT id,ordinal,delegator_ref,delegate_ref,data_resource_id,grantable_actions,grantable_purposes,scope_type,scope_ref,valid_from,valid_to FROM grantor_authority_delegation_edge WHERE chain_id=$1 ORDER BY ordinal`, chain.ID)
	if err != nil {
		return chain, err
	}
	defer rows.Close()
	for rows.Next() {
		var edge domain.DelegationEdge
		if err := rows.Scan(&edge.ID, &edge.Ordinal, &edge.DelegatorRef, &edge.DelegateRef, &edge.DataResourceID, &edge.GrantableActions, &edge.GrantablePurposes, &edge.Scope.Type, &edge.Scope.Ref, &edge.ValidFrom, &edge.ValidTo); err != nil {
			return chain, err
		}
		chain.Edges = append(chain.Edges, edge)
	}
	if err := rows.Err(); err != nil {
		return chain, err
	}
	if len(chain.Edges) == 0 {
		return chain, domain.ErrInvalidBinding
	}
	chain.ChainHash = domain.HashDelegationEdges(chain.Edges)
	if _, err := tx.Exec(ctx, `UPDATE grantor_authority_delegation_chain SET status='FINALIZED',chain_hash=$2,finalized_at=now() WHERE id=$1`, chain.ID, chain.ChainHash); err != nil {
		return chain, err
	}
	chain.Status = "FINALIZED"
	return chain, nil
}

func (r *PostgresRepository) InsertDelegationDisposition(ctx context.Context, tx pgx.Tx, disposition domain.DelegationDisposition) error {
	var inserted uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO grantor_authority_delegation_disposition(id,chain_id,edge_id,disposition,effective_at,reason,evidence_id,activity_id,actor_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING RETURNING id`, disposition.ID, disposition.ChainID, disposition.EdgeID, disposition.Disposition, disposition.EffectiveAt, disposition.Reason, disposition.EvidenceID, disposition.ActivityID, disposition.ActorID).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDelegationDispositionIdempotentReplay
	}
	return err
}

func (r *PostgresRepository) GetDelegationDispositionByActivityID(ctx context.Context, chainID uuid.UUID, disposition string, activityID uuid.UUID) (domain.DelegationDisposition, error) {
	var d domain.DelegationDisposition
	err := r.pool.QueryRow(ctx, `SELECT id,chain_id,edge_id,disposition,effective_at,reason,evidence_id,activity_id,actor_id FROM grantor_authority_delegation_disposition WHERE chain_id=$1 AND disposition=$2 AND activity_id=$3`, chainID, disposition, activityID).
		Scan(&d.ID, &d.ChainID, &d.EdgeID, &d.Disposition, &d.EffectiveAt, &d.Reason, &d.EvidenceID, &d.ActivityID, &d.ActorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DelegationDisposition{}, ErrNotFound
	}
	if err != nil {
		return domain.DelegationDisposition{}, fmt.Errorf("get delegation disposition by activity: %w", err)
	}
	return d, nil
}
