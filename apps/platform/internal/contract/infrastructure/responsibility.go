package infrastructure

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	q "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/readmodel"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
)

// CheckResponsibilityInTx is a narrow identity/role Port for a caller-owned
// transaction. It is NOT a Case command, source-trust or deployment permission.
// A future command must also validate its case/source/evidence/state in this tx.
func (r AcceptanceIdentityReader) CheckResponsibilityInTx(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal, query q.Query, command string) (principal.Authority, *domain.AcceptanceResponsibility, error) {
	var role, cap string
	switch command {
	case "CreateBusinessAcceptance", "ReviseAcceptanceDraft":
		role, cap = "PROVIDER", principal.CapabilityAcceptanceDraft
	case "ConfirmAcceptanceBasis":
		role, cap = "BASIS_CONFIRMER", principal.CapabilityAcceptanceBasis
	case "RecordBusinessReceipt":
		role, cap = "RECEIVER", principal.CapabilityReceiptRecord
	default:
		return principal.Authority{}, nil, principal.ErrCapabilityDenied
	}
	if p.WorkspaceID != query.WorkspaceID || query.DeliveryID == nil || query.DatasetVersionID == nil {
		return principal.Authority{}, nil, principal.ErrWorkspaceDenied
	}
	if _, e := r.Authority.ValidateInTx(ctx, tx, p, principal.CapabilityAcceptanceRead); e != nil {
		return principal.Authority{}, nil, e
	}
	if e := bindings.LockAcceptanceBindings(ctx, tx, p.WorkspaceID); e != nil {
		return principal.Authority{}, nil, e
	}
	a, e := r.Authority.ValidateInTx(ctx, tx, p, cap)
	if e != nil {
		return a, nil, e
	}
	if !a.Principal.HasCapability(principal.CapabilityAcceptanceRead) {
		return a, nil, principal.ErrCapabilityDenied
	}
	snapshot, e := r.Repository.ReadAcceptanceSourcesInTx(ctx, tx, query, a.EvaluatedAt)
	if e != nil {
		return a, nil, e
	}
	roles, _, _ := domain.EvaluateAcceptanceBindings(domain.AcceptanceBindingContext{WorkspaceID: p.WorkspaceID.String(), DeliveryID: query.DeliveryID.String(), DatasetVersionID: query.DatasetVersionID.String(), ConsumerRef: snapshot.Delivery.ConsumerRef, ActorID: p.ActorID.String(), Subject: p.Subject, At: a.EvaluatedAt}, snapshot.Bindings)
	for _, v := range roles {
		if v.Role == role && v.Status == "ASSIGNED" && v.IsCurrentCaller {
			return a, &v, nil
		}
	}
	return a, nil, principal.ErrCapabilityDenied
}
