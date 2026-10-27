package platformmanagement

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/bindings"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal/oidc"
)

// References must come from an explicitly trusted mapping owner. Native user
// IDs, login names and email addresses are never guessed to be OIDC subjects.
// The independent fixture supplies synthetic references; real sources stay nil.
type UserSubjectReference struct{ Issuer, Subject, BasisRef string }
type BoundUserIdentities struct {
	Store      *bindings.Store
	Workspace  uuid.UUID
	References map[string]UserSubjectReference
}

func (r BoundUserIdentities) ReadUserIdentity(ctx context.Context, workspace uuid.UUID, user string) (UserIdentityLink, error) {
	if workspace == uuid.Nil || workspace != r.Workspace {
		return UserIdentityLink{}, ErrInvalidRequest
	}
	link := UserIdentityLink{State: "UNCONFIGURED", WorkspaceID: workspace}
	if r.References == nil {
		return link, nil
	}
	link.State = "UNLINKED"
	ref, ok := r.References[user]
	if !ok {
		return link, nil
	}
	if r.Store == nil || r.Workspace == uuid.Nil || !IsIdentifier(user) || ref.Issuer == "" || ref.Subject == "" || ref.BasisRef == "" || len(ref.BasisRef) > 200 {
		return UserIdentityLink{}, ErrUnavailable
	}
	b, err := r.Store.Lookup(ctx, oidc.Identity{Issuer: ref.Issuer, Subject: ref.Subject}, r.Workspace)
	if err != nil {
		return UserIdentityLink{}, ErrUnavailable
	}
	if !b.Active {
		return link, nil
	}
	if b.ActorID == uuid.Nil || b.Subject != b.ActorID.String() || b.WorkspaceID != r.Workspace || b.AuthorityRevision == "" || !b.AuthorityExpiresAt.After(time.Now()) {
		return UserIdentityLink{}, ErrInvalidResponse
	}
	actor := b.ActorID.String()
	expires := b.AuthorityExpiresAt.UTC()
	return UserIdentityLink{State: "LINKED", WorkspaceID: workspace, ActorID: &actor, Revision: b.AuthorityRevision, BasisRef: ref.BasisRef, ExpiresAt: &expires}, nil
}
