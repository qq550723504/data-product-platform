package platformmanagement

import (
	"context"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// A directory organization is an external identity scope, not a workspace,
// company/legal identity or department hierarchy.
type DirectoryOrganization struct {
	ID        string  `json:"id"`
	Name      *string `json:"name"`
	State     string  `json:"state"`
	CreatedAt *string `json:"createdAt"`
	ChangedAt *string `json:"changedAt"`
}
type OrganizationReader interface {
	ReadOrganization(context.Context, string) (DirectoryOrganization, error)
}
type OrganizationOverview struct {
	Organization             DirectoryOrganization `json:"organization"`
	DepartmentHierarchyState string                `json:"departmentHierarchyState"`
}

func QueryOrganization(ctx context.Context, b Binding) (OrganizationOverview, error) {
	if b.Organizations == nil || !IsIdentifier(b.OrganizationID) {
		return OrganizationOverview{}, ErrUnconfigured
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	org, err := b.Organizations.ReadOrganization(ctx, b.OrganizationID)
	if err != nil {
		return OrganizationOverview{}, err
	}
	if org.ID != b.OrganizationID || !ValidDirectoryOrganization(org) {
		return OrganizationOverview{}, ErrInvalidResponse
	}
	if ctx.Err() != nil {
		return OrganizationOverview{}, ErrUnavailable
	}
	return OrganizationOverview{org, "UNCONFIGURED"}, nil
}
func ValidDirectoryOrganization(o DirectoryOrganization) bool {
	if !IsIdentifier(o.ID) || (o.State != "active" && o.State != "inactive" && o.State != "removed") {
		return false
	}
	if o.Name != nil && (!utf8.ValidString(*o.Name) || len(*o.Name) > 512 || strings.TrimSpace(*o.Name) == "" || strings.ContainsFunc(*o.Name, unicode.IsControl)) {
		return false
	}
	for _, d := range []*string{o.CreatedAt, o.ChangedAt} {
		if d != nil {
			if _, err := time.Parse(time.RFC3339Nano, *d); err != nil {
				return false
			}
		}
	}
	return true
}
