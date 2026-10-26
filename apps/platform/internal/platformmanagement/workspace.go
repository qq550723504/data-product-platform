package platformmanagement

import (
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"time"
)

// This projects existing identity/configuration only. There is no workspace
// lifecycle record, owner, quota, directory health check or reusable grant here.
type WorkspaceDirectory struct {
	State          string  `json:"state"`
	Source         string  `json:"source"`
	OrganizationID *string `json:"organizationId"`
	ProjectID      *string `json:"projectId"`
	Availability   string  `json:"availability"`
}
type WorkspaceQueries struct {
	Members bool `json:"members"`
	Users   bool `json:"users"`
	Roles   bool `json:"roles"`
	Audit   bool `json:"audit"`
}
type WorkspaceOverview struct {
	RecordState           string             `json:"recordState"`
	ConfirmationExpiresAt time.Time          `json:"confirmationExpiresAt"`
	Directory             WorkspaceDirectory `json:"directory"`
	Queries               WorkspaceQueries   `json:"queries"`
}

func workspaceOverview(b Binding, audit AuditBinding, a principal.Authority) (WorkspaceOverview, error) {
	if a.Principal.ActorID == uuid.Nil || a.Principal.WorkspaceID == uuid.Nil || a.EvaluatedAt.IsZero() || !a.ExpiresAt.After(a.EvaluatedAt) || !a.ExpiresAt.After(time.Now()) {
		return WorkspaceOverview{}, ErrUnavailable
	}
	result := WorkspaceOverview{RecordState: "UNCONFIGURED", ConfirmationExpiresAt: a.ExpiresAt.UTC(), Directory: WorkspaceDirectory{State: "UNCONFIGURED", Source: "UNCONFIGURED", Availability: "NOT_CHECKED"}}
	result.Queries.Audit = audit.Configured()
	// Same requirements as the existing query paths, without invoking a provider.
	ready := b.Directory != nil && b.Catalog != nil && IsIdentifier(b.OrganizationID) && IsIdentifier(b.ProjectID) && (b.Source == "CONFIGURED" || b.Source == "SYNTHETIC_FIXTURE")
	if ready {
		org, project := b.OrganizationID, b.ProjectID
		result.Directory = WorkspaceDirectory{State: "CONFIGURED", Source: b.Source, OrganizationID: &org, ProjectID: &project, Availability: "NOT_CHECKED"}
		result.Queries = WorkspaceQueries{Members: true, Users: b.Users != nil, Roles: true, Audit: audit.Configured()}
	}
	return result, nil
}
