// Package bindings owns platform actor references and current application grants.
// It owns no passwords, login protocol, IdP accounts, or general policy engine.
package bindings

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
)

const CapabilityIdentityAdmin = "IDENTITY_BINDING_ADMIN"
const mutationFence int64 = 810051001

var (
	ErrInvalid     = errors.New("invalid identity state command")
	ErrConflict    = errors.New("identity state revision conflict")
	ErrUnavailable = principal.ErrAuthorityUnavailable
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// SubjectAuthority is a short-lived confirmation for an LS identity projection.
// Revision changes on actor/binding lifecycle transitions; it is not a token.
type SubjectAuthority struct {
	PrincipalID string    `json:"principal_id"`
	Revision    string    `json:"revision"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (s *Store) Confirm(ctx context.Context, identity oidc.Identity) (SubjectAuthority, error) {
	if s == nil || s.pool == nil {
		return SubjectAuthority{}, ErrUnavailable
	}
	var actor uuid.UUID
	var ar, br int64
	var expires time.Time
	err := s.pool.QueryRow(ctx, `SELECT a.actor_id,a.revision,b.revision,
		LEAST(COALESCE(b.valid_until,statement_timestamp()+interval '5 minutes'),statement_timestamp()+interval '5 minutes')
		FROM platform_subject_binding b JOIN platform_actor_state a ON a.actor_id=b.actor_id
		WHERE b.issuer=$1 AND b.subject=$2 AND b.enabled AND a.enabled
		AND b.valid_from<=statement_timestamp() AND (b.valid_until IS NULL OR b.valid_until>statement_timestamp())`, identity.Issuer, identity.Subject).Scan(&actor, &ar, &br, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return SubjectAuthority{}, principal.ErrUnauthenticated
	}
	if err != nil {
		return SubjectAuthority{}, ErrUnavailable
	}
	return SubjectAuthority{actor.String(), strconv.FormatInt(ar, 10) + ":" + strconv.FormatInt(br, 10), expires}, nil
}

func (s *Store) Lookup(ctx context.Context, identity oidc.Identity, workspace uuid.UUID) (oidc.Binding, error) {
	if s == nil || s.pool == nil {
		return oidc.Binding{}, ErrUnavailable
	}
	return lookupBinding(ctx, s.pool, identity, workspace)
}

func lookupBinding(ctx context.Context, q queryer, identity oidc.Identity, workspace uuid.UUID) (oidc.Binding, error) {
	var actor uuid.UUID
	var caps []string
	var ar, br, gr int64
	var expires time.Time
	err := q.QueryRow(ctx, `SELECT a.actor_id,g.capabilities,a.revision,b.revision,g.revision,
		LEAST(COALESCE(b.valid_until,statement_timestamp()+interval '5 minutes'),
		COALESCE(g.valid_until,statement_timestamp()+interval '5 minutes'),statement_timestamp()+interval '5 minutes')
		FROM platform_subject_binding b
		JOIN platform_actor_state a ON a.actor_id=b.actor_id
		JOIN platform_workspace_grant g ON g.actor_id=a.actor_id AND g.workspace_id=$3
		WHERE b.issuer=$1 AND b.subject=$2 AND b.enabled AND a.enabled AND g.enabled
		AND b.valid_from<=statement_timestamp() AND (b.valid_until IS NULL OR b.valid_until>statement_timestamp())
		AND g.valid_from<=statement_timestamp() AND (g.valid_until IS NULL OR g.valid_until>statement_timestamp())`, identity.Issuer, identity.Subject, workspace).Scan(&actor, &caps, &ar, &br, &gr, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return oidc.Binding{}, nil
	}
	if err != nil {
		return oidc.Binding{}, ErrUnavailable
	}
	permissions := make(map[string]struct{}, len(caps))
	for _, capability := range caps {
		permissions[capability] = struct{}{}
	}
	return oidc.Binding{Active: true, Subject: actor.String(), ActorID: actor, WorkspaceID: workspace, Capabilities: permissions,
		AuthorityRevision: strconv.FormatInt(ar, 10) + ":" + strconv.FormatInt(br, 10) + ":" + strconv.FormatInt(gr, 10), AuthorityExpiresAt: expires}, nil
}
