package pilot

import (
	_ "embed"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

// These identities are the independently reviewed v16 sources. A configuration
// file cannot expand the native fixture, substitute another version or select an
// old consumed execution. New output copies are separately verified survivors.
//
//go:embed remaining_scope.json
var remainingScopes []byte

// A different UTC slot requires a new frozen candidate/confirmation. It cannot
// be opened by editing a permission file or replaying an old confirmation text.
const RemainingExecution = "aa2605a5-1fcf-4e9d-b6f2-8f5b13321b8a"
const RemainingProfile = "candidate16/v26"
const RemainingNotBefore = "2026-10-20T10:00:00Z"
const RemainingNotAfter = "2026-10-20T10:40:00Z"

// New execution-owned identities never reuse the retained v22 users. Policy
// construction and the root status owner share this one naming contract.
func RemainingIdentity(group, kind string) string {
	s, err := RemainingScopeFor(group)
	if err != nil || (kind != "inspect" && kind != "delete") {
		return ""
	}
	prefix := map[string]string{"inspect": "i", "delete": "d"}[kind]
	return prefix + RemainingExecution[:8] + s.Run.String()[:8]
}

func RemainingPolicyName(group, kind string) string {
	id := RemainingIdentity(group, kind)
	if id == "" {
		return ""
	}
	return "lifecycle307-six-" + id
}

type RemainingPlan struct {
	Case      string    `json:"case"`
	ID        uuid.UUID `json:"id"`
	Hash      string    `json:"scope_hash"`
	Algorithm string    `json:"hash_algorithm"`
	Status    string    `json:"status"`
	Targets   int       `json:"targets"`
}
type RemainingScope struct {
	Group                                 string
	Run, Workspace, Provider, Incarnation uuid.UUID
	ManifestSHA256                        string
	Copies                                []domain.Copy
	Plans                                 []RemainingPlan
}

func RemainingScopeFor(group string) (RemainingScope, error) {
	var scopes []RemainingScope
	if json.Unmarshal(remainingScopes, &scopes) != nil {
		return RemainingScope{}, errors.New("invalid compiled remaining scope")
	}
	for _, s := range scopes {
		if s.Group == group {
			return s, nil
		}
	}
	return RemainingScope{}, errors.New("remaining group unsupported")
}
