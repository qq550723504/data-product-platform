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
const RemainingExecution = "8e41cfba-a41e-44dd-9ce8-d6365b3bb90e"
const RemainingProfile = "candidate12/v22"
const RemainingNotBefore = "2026-10-19T06:00:00Z"
const RemainingNotAfter = "2026-10-19T06:40:00Z"

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
