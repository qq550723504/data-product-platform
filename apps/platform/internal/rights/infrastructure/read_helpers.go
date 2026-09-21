package infrastructure

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
)

func (r *PostgresRepository) GetAuthorizationProvenanceBinding(ctx context.Context, bindingID uuid.UUID) (domain.AuthorizationProvenanceBinding, error) {
	var binding domain.AuthorizationProvenanceBinding
	err := r.pool.QueryRow(ctx, `
		SELECT id,workspace_id,authorization_id,data_resource_id,rights_declaration_id,
		       grantor_ref,grantor_authority_mode,delegation_chain_id,
		       COALESCE(delegation_chain_hash,''),created_at,created_by,activity_id
		FROM authorization_provenance_binding
		WHERE id=$1
	`, bindingID).Scan(
		&binding.ID, &binding.WorkspaceID, &binding.AuthorizationID, &binding.DataResourceID,
		&binding.DeclarationID, &binding.GrantorRef, &binding.AuthorityMode,
		&binding.DelegationChainID, &binding.DelegationChainHash, &binding.CreatedAt,
		&binding.CreatedBy, &binding.ActivityID,
	)
	if err == pgx.ErrNoRows {
		return domain.AuthorizationProvenanceBinding{}, ErrNotFound
	}
	if err != nil {
		return domain.AuthorizationProvenanceBinding{}, fmt.Errorf("get authorization provenance binding: %w", err)
	}
	return binding, nil
}
