// Reuses shuomi-ai-commerce's Casbin role-to-permission model; no commerce policies imported.
package platformmanagement

import (
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"sort"
	"strings"
)

const roleModel = `
[request_definition]
r = sub, obj
[policy_definition]
p = sub, obj
[role_definition]
g = _, _
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = (r.sub == p.sub || g(r.sub, p.sub)) && r.obj == p.obj
`

var permissionNames = map[string]string{"management.members.read": "查看空间成员", "management.roles.read": "查看应用角色权限", "management.audit.read": "查看空间审计"}

type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}
type Catalog struct {
	roles    []Role
	enforcer *casbin.Enforcer
}

func NewCatalog(roles []Role) (*Catalog, error) {
	if len(roles) == 0 || len(roles) > 64 {
		return nil, ErrUnconfigured
	}
	m, err := model.NewModelFromString(roleModel)
	if err != nil {
		return nil, ErrInvalidResponse
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, ErrInvalidResponse
	}
	result := &Catalog{enforcer: e}
	seen := map[string]bool{}
	for _, r := range roles {
		if !IsIdentifier(r.ID) || seen[r.ID] || strings.TrimSpace(r.Name) == "" || len(r.Name) > 200 || len(r.Permissions) > len(permissionNames) {
			return nil, ErrInvalidResponse
		}
		seen[r.ID] = true
		copyRole := Role{ID: r.ID, Name: r.Name, Permissions: []string{}}
		permissions := map[string]bool{}
		for _, p := range r.Permissions {
			if permissionNames[p] == "" || permissions[p] {
				return nil, ErrInvalidResponse
			}
			permissions[p] = true
			if _, err = e.AddPolicy(r.ID, p); err != nil {
				return nil, ErrInvalidResponse
			}
			copyRole.Permissions = append(copyRole.Permissions, p)
		}
		sort.Strings(copyRole.Permissions)
		result.roles = append(result.roles, copyRole)
	}
	sort.Slice(result.roles, func(i, j int) bool { return result.roles[i].ID < result.roles[j].ID })
	return result, nil
}
func (c *Catalog) Has(id string) bool {
	for _, r := range c.roles {
		if r.ID == id {
			return true
		}
	}
	return false
}
func (c *Catalog) Roles() []Role {
	result := make([]Role, 0, len(c.roles))
	for _, r := range c.roles {
		result = append(result, Role{r.ID, r.Name, append([]string{}, r.Permissions...)})
	}
	return result
}
func (c *Catalog) Permissions(roles []string) ([]string, error) {
	result := []string{}
	for p := range permissionNames {
		for _, r := range roles {
			ok, err := c.enforcer.Enforce(r, p)
			if err != nil {
				return nil, ErrInvalidResponse
			}
			if ok {
				result = append(result, p)
				break
			}
		}
	}
	sort.Strings(result)
	return result, nil
}
