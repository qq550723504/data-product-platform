package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

func (r *PostgresRepository) InsertDelegationChain(ctx context.Context, tx pgx.Tx, chain domain.DelegationChain) error {
	var workspace uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT workspace_id FROM rights_declaration WHERE id=$1`, chain.SourceDeclarationID).Scan(&workspace); err != nil {
		return err
	}
	if workspace != chain.WorkspaceID {
		return domain.ErrResourceWorkspace
	}
	if _, err := tx.Exec(ctx, `INSERT INTO grantor_authority_delegation_chain(id,workspace_id,source_declaration_id,status,created_at,created_by) VALUES($1,$2,$3,'DRAFT',$4,$5)`, chain.ID, chain.WorkspaceID, chain.SourceDeclarationID, chain.CreatedAt, chain.CreatedBy); err != nil {
		return fmt.Errorf("insert delegation chain: %w", err)
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

func (r *PostgresRepository) FinalizeDelegationChain(ctx context.Context, tx pgx.Tx, chain domain.DelegationChain) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM grantor_authority_delegation_chain WHERE id=$1 FOR UPDATE`, chain.ID).Scan(&status); err != nil {
		return err
	}
	if status != "DRAFT" {
		return domain.ErrInvalidBinding
	}
	_, err := tx.Exec(ctx, `UPDATE grantor_authority_delegation_chain SET status='FINALIZED',chain_hash=$2,finalized_at=now() WHERE id=$1`, chain.ID, chain.ChainHash)
	return err
}

func (r *PostgresRepository) InsertDelegationDisposition(ctx context.Context, tx pgx.Tx, disposition domain.DelegationDisposition) error {
	_, err := tx.Exec(ctx, `INSERT INTO grantor_authority_delegation_disposition(id,chain_id,edge_id,disposition,effective_at,reason,evidence_id,actor_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, disposition.ID, disposition.ChainID, disposition.EdgeID, disposition.Disposition, disposition.EffectiveAt, disposition.Reason, disposition.EvidenceID, disposition.ActorID)
	return err
}
