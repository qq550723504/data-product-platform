package platformmanagement

import "github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"

// Deliberately assembled from allowlisted runtime composition. Never serialize
// an environment/config object, connection string, credential or partial secret.
type ConfigurationDirectory struct {
	State        string `json:"state"`
	Source       string `json:"source"`
	Availability string `json:"availability"`
}
type ConfigurationQueries struct {
	Workspace    bool `json:"workspace"`
	Organization bool `json:"organization"`
	Members      bool `json:"members"`
	Users        bool `json:"users"`
	Roles        bool `json:"roles"`
	Audit        bool `json:"audit"`
}
type ConfigurationOverview struct {
	Scope            string                 `json:"scope"`
	Mode             string                 `json:"mode"`
	Directory        ConfigurationDirectory `json:"directory"`
	Queries          ConfigurationQueries   `json:"queries"`
	RolePolicySource *string                `json:"rolePolicySource"`
}

func configurationOverview(b Binding, audit AuditBinding, a principal.Authority) (ConfigurationOverview, error) {
	w, err := workspaceOverview(b, audit, a)
	if err != nil {
		return ConfigurationOverview{}, err
	}
	result := ConfigurationOverview{Scope: "CURRENT_WORKSPACE_MANAGEMENT", Mode: "QUERY_ONLY", Directory: ConfigurationDirectory{w.Directory.State, w.Directory.Source, w.Directory.Availability}, Queries: ConfigurationQueries{Workspace: true, Organization: w.Queries.Members && b.Organizations != nil, Members: w.Queries.Members, Users: w.Queries.Users, Roles: w.Queries.Roles, Audit: w.Queries.Audit}}
	if w.Queries.Roles {
		source := "APPLICATION_CATALOG"
		result.RolePolicySource = &source
	}
	return result, nil
}
