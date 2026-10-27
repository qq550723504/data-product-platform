package infrastructure

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
)

type AcceptanceTarget struct {
	DeliveryID uuid.UUID `json:"deliveryOperationId"`
	VersionID  uuid.UUID `json:"datasetVersionId"`
	Name       string    `json:"name"`
	Roles      []string  `json:"roles"`
}
type AcceptanceTargets struct {
	WorkspaceID uuid.UUID          `json:"workspaceId"`
	Items       []AcceptanceTarget `json:"items"`
}

// Visible targets are derived from complete current role facts, not token roles
// or owner IDs. No raw storage locations/provider references are selected.
func (r AcceptanceIdentityReader) Targets(ctx context.Context, p principal.RequestPrincipal) (AcceptanceTargets, error) {
	result := AcceptanceTargets{WorkspaceID: p.WorkspaceID, Items: []AcceptanceTarget{}}
	tx, err := r.Repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	if _, err = r.Authority.ValidateInTx(ctx, tx, p, principal.CapabilityAcceptanceRead); err != nil {
		return result, err
	}
	if err = bindings.LockAcceptanceBindings(ctx, tx, p.WorkspaceID); err != nil {
		return result, err
	}
	a, err := r.Authority.ValidateInTx(ctx, tx, p, principal.CapabilityAcceptanceRead)
	if err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT o.id,o.dataset_version_id,d.name FROM acceptance_responsibility_binding r
 JOIN acceptance_party_representation rep ON rep.id=r.representation_id AND rep.workspace_id=r.workspace_id
 JOIN delivery_operation o ON o.id=r.delivery_operation_id AND o.workspace_id=r.workspace_id
 JOIN dataset_version v ON v.id=o.dataset_version_id JOIN dataset d ON d.id=v.dataset_id AND d.workspace_id=r.workspace_id
 WHERE r.workspace_id=$1 AND rep.actor_id=$2 AND rep.principal_subject=$3 ORDER BY o.id LIMIT 101`, p.WorkspaceID, p.ActorID, p.Subject)
	if err != nil {
		return result, err
	}
	candidates := []AcceptanceTarget{}
	for rows.Next() {
		var t AcceptanceTarget
		if err = rows.Scan(&t.DeliveryID, &t.VersionID, &t.Name); err != nil {
			rows.Close()
			return result, err
		}
		candidates = append(candidates, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(candidates) > 100 {
		return result, principal.ErrAuthorityUnavailable
	}
	for _, t := range candidates {
		query := q.Query{WorkspaceID: p.WorkspaceID, ActorID: p.ActorID, Subject: p.Subject, DeliveryID: &t.DeliveryID, DatasetVersionID: &t.VersionID}
		facts, e := readBindingFacts(ctx, tx, query)
		if e != nil {
			return result, e
		}
		var consumer string
		if e = tx.QueryRow(ctx, `SELECT effective_consumer_ref FROM delivery_operation WHERE workspace_id=$1 AND id=$2`, p.WorkspaceID, t.DeliveryID).Scan(&consumer); e != nil {
			return result, e
		}
		roles, _, _ := domain.EvaluateAcceptanceBindings(domain.AcceptanceBindingContext{WorkspaceID: p.WorkspaceID.String(), DeliveryID: t.DeliveryID.String(), DatasetVersionID: t.VersionID.String(), ConsumerRef: consumer, ActorID: p.ActorID.String(), Subject: p.Subject, At: a.EvaluatedAt}, facts)
		t.Roles = []string{}
		for _, role := range roles {
			if role.Status == "ASSIGNED" && role.IsCurrentCaller {
				t.Roles = append(t.Roles, role.Role)
			}
		}
		if len(t.Roles) > 0 {
			result.Items = append(result.Items, t)
		}
	}
	return result, tx.Commit(ctx)
}
