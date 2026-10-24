package infrastructure

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
)

// AcceptanceIdentityReader composes existing source projections with current
// authority on ONE connection. No Case, Receipt or business command is written.
type AcceptanceIdentityReader struct {
	Repository *PostgresRepository
	Authority  *bindings.Store
}

func (r AcceptanceIdentityReader) ReadAcceptanceSources(ctx context.Context, query q.Query) (q.Snapshot, error) {
	var result q.Snapshot
	err := r.withAcceptanceAuthority(ctx, query, func(_ pgx.Tx, _ principal.Authority, s q.Snapshot) error { result = s; return nil })
	return result, err
}

func (r AcceptanceIdentityReader) withAcceptanceAuthority(ctx context.Context, query q.Query, read func(pgx.Tx, principal.Authority, q.Snapshot) error) error {
	var result q.Snapshot
	if query.Principal == nil || query.Principal.WorkspaceID != query.WorkspaceID {
		return principal.ErrUnauthenticated
	}
	tx, err := r.Repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = r.Authority.ValidateInTx(ctx, tx, *query.Principal, principal.CapabilityAcceptanceRead); err != nil {
		return err
	}
	if err = bindings.LockAcceptanceBindings(ctx, tx, query.WorkspaceID); err != nil {
		return err
	}
	// Refresh the DB clock and complete authority after the last fence wait.
	a, err := r.Authority.ValidateInTx(ctx, tx, *query.Principal, principal.CapabilityAcceptanceRead)
	if err != nil {
		return err
	}
	result, err = r.Repository.ReadAcceptanceSourcesInTx(ctx, tx, query, a.EvaluatedAt)
	if err != nil {
		return err
	}
	if query.DeliveryID != nil {
		bc := domain.AcceptanceBindingContext{WorkspaceID: query.WorkspaceID.String(), DeliveryID: query.DeliveryID.String(), DatasetVersionID: query.DatasetVersionID.String(), ConsumerRef: result.Delivery.ConsumerRef, ActorID: a.Principal.ActorID.String(), Subject: a.Principal.Subject, At: a.EvaluatedAt}
		roles, _, _ := domain.EvaluateAcceptanceBindings(bc, result.Bindings)
		visible := false
		for _, role := range roles {
			if role.Status == "ASSIGNED" && role.IsCurrentCaller {
				visible = true
			}
		}
		if !visible {
			return q.ErrNotFound
		}
	}
	result.Authority = &a
	if err = read(tx, a, result); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
