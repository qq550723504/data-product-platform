package bindings

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

// ValidateInTx acquires the same namespace as the IAM mutation owner, on the
// CALLER'S transaction. Identity triggers also take its exclusive form. No
// network call, pool query or second connection is allowed in this method.
func (s *Store) ValidateInTx(ctx context.Context, tx pgx.Tx, p principal.RequestPrincipal, capability string) (principal.Authority, error) {
	var a principal.Authority
	if tx == nil || p.ActorID == uuid.Nil || p.WorkspaceID == uuid.Nil || p.IdentityIssuer == "" || p.IdentitySubject == "" || p.IdentityRevision == "" || p.IdentityExpiresAt.IsZero() {
		return a, principal.ErrUnauthenticated
	}
	var isolation string
	if err := tx.QueryRow(ctx, `SELECT current_setting('transaction_isolation')`).Scan(&isolation); err != nil || isolation != "read committed" {
		return a, principal.ErrAuthorityUnavailable
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared($1)`, mutationFence); err != nil {
		return a, principal.ErrAuthorityUnavailable
	}
	var caps []string
	var actor uuid.UUID
	err := tx.QueryRow(ctx, `WITH t AS MATERIALIZED (SELECT clock_timestamp() AS at)
 SELECT a.actor_id,b.id,g.id,a.revision,b.revision,g.revision,g.capabilities,t.at,
 LEAST(COALESCE(b.valid_until,t.at+interval '5 minutes'),COALESCE(g.valid_until,t.at+interval '5 minutes'),t.at+interval '5 minutes',$4::timestamptz)
 FROM platform_subject_binding b JOIN platform_actor_state a ON a.actor_id=b.actor_id
 JOIN platform_workspace_grant g ON g.actor_id=a.actor_id CROSS JOIN t
 WHERE b.issuer=$1 AND b.subject=$2 AND g.workspace_id=$3 AND b.enabled AND a.enabled AND g.enabled
 AND b.valid_from<=t.at AND (b.valid_until IS NULL OR b.valid_until>t.at)
 AND g.valid_from<=t.at AND (g.valid_until IS NULL OR g.valid_until>t.at) AND $4::timestamptz>t.at`, p.IdentityIssuer, p.IdentitySubject, p.WorkspaceID, p.IdentityExpiresAt).Scan(&actor, &a.SubjectBindingID, &a.WorkspaceGrantID, &a.ActorRevision, &a.SubjectRevision, &a.GrantRevision, &caps, &a.EvaluatedAt, &a.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, principal.ErrWorkspaceDenied
	}
	if err != nil {
		return a, principal.ErrAuthorityUnavailable
	}
	revision := fmt.Sprintf("%d:%d:%d", a.ActorRevision, a.SubjectRevision, a.GrantRevision)
	if actor != p.ActorID || actor.String() != p.Subject || revision != p.IdentityRevision || !a.ExpiresAt.After(a.EvaluatedAt) {
		return a, principal.ErrUnauthenticated
	}
	current := map[string]struct{}{}
	for _, c := range caps {
		current[c] = struct{}{}
	}
	if _, ok := current[capability]; !ok {
		return a, principal.ErrCapabilityDenied
	}
	p.Capabilities = current
	p.IdentityExpiresAt = a.ExpiresAt.UTC()
	a.Principal = p
	a.EvaluatedAt = a.EvaluatedAt.UTC()
	a.ExpiresAt = a.ExpiresAt.UTC()
	return a, nil
}

// LockAcceptanceBindings follows IAM -> workspace binding ordering. Mutation
// triggers use the identical text namespace and exclusive transaction lock.
func LockAcceptanceBindings(ctx context.Context, tx pgx.Tx, workspace uuid.UUID) error {
	if workspace == uuid.Nil {
		return principal.ErrWorkspaceDenied
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended('acceptance-binding:'||$1::text,0))`, workspace.String())
	return err
}

// Expiry remains checked after any subsequent dependency lock wait. Call
// ValidateInTx again after the last such lock before admitting a future write.
