// Package platformmanagement owns read projections, not identity accounts or business writes.
package platformmanagement

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"regexp"
	"time"
)

var (
	ErrInvalidRequest  = errors.New("invalid management query")
	ErrUnavailable     = errors.New("management source unavailable")
	ErrInvalidResponse = errors.New("invalid management source response")
	ErrNotFound        = errors.New("management object unavailable")
	ErrUnconfigured    = errors.New("management source unconfigured")
)
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func IsIdentifier(s string) bool { return identifier.MatchString(s) }

// Provider identities are external references; they are never canonical platform actors.
type Member struct {
	ID             string   `json:"id"`
	UserID         string   `json:"userId"`
	OrganizationID string   `json:"organizationId"`
	ProjectID      string   `json:"projectId"`
	DisplayName    string   `json:"displayName"`
	LoginName      string   `json:"loginName"`
	Roles          []string `json:"roles"`
	State          string   `json:"state"`
	CreatedAt      string   `json:"createdAt"`
	ChangedAt      string   `json:"changedAt"`
	Permissions    []string `json:"permissions"`
}
type PageRequest struct {
	Limit, Offset int
	Filter        ListFilter
}
type ListFilter struct{ Search, Role, State string }
type Page struct {
	Items []Member
	Total int
}
type Directory interface {
	List(context.Context, string, PageRequest) (Page, error)
	Read(context.Context, string, string) (Member, error)
}
type Binding struct {
	OrganizationID, ProjectID, Source string
	Directory                         Directory
	Catalog                           *Catalog
}
type Gate interface {
	Confirm(context.Context, principal.RequestPrincipal) (principal.Authority, error)
}
type AuditEvent struct {
	ID         uuid.UUID  `json:"id"`
	ActorID    *uuid.UUID `json:"actorId"`
	ActorType  string     `json:"actorType"`
	Action     string     `json:"action"`
	ObjectType string     `json:"objectType"`
	ObjectID   uuid.UUID  `json:"objectId"`
	OccurredAt time.Time  `json:"occurredAt"`
}
type AuditPage struct {
	Items []AuditEvent `json:"items"`
	Next  string       `json:"next"`
}
type AuditReader interface {
	List(context.Context, uuid.UUID, uuid.UUID, string) (AuditPage, error)
}
type Envelope struct {
	ProfileVersion    string    `json:"profileVersion"`
	WorkspaceID       uuid.UUID `json:"workspaceId"`
	PrincipalID       uuid.UUID `json:"principalId"`
	AuthorityRevision string    `json:"authorityRevision"`
	EvaluatedAt       time.Time `json:"evaluatedAt"`
	Source            string    `json:"source"`
	WriteEnabled      bool      `json:"writeEnabled"`
}
