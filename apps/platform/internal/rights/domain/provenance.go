package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	PermissionUse   = "USE"
	PermissionGrant = "GRANT"

	DeclarationVerified = "VERIFIED"
	DeclarationRejected = "REJECTED"

	DispositionInvalidated = "INVALIDATED"
	DispositionSuperseded  = "SUPERSEDED"

	AuthorityDirect    = "DIRECT_DECLARATION_PARTY"
	AuthorityDelegated = "DELEGATED"

	DecisionAllowed    = "ALLOWED"
	DecisionNotAllowed = "NOT_ALLOWED"
)

var (
	ErrInvalidRightsDeclaration = errors.New("rights declaration is invalid")
	ErrDeclarationNotVerified   = errors.New("rights declaration is not verified")
	ErrDeclarationTerminal      = errors.New("rights declaration already has a terminal verification")
	ErrRightsDisposition        = errors.New("rights disposition is invalid")
	ErrInvalidBinding           = errors.New("authorization provenance binding is invalid")
	ErrBindingNotCurrent        = errors.New("authorization provenance binding is not current")
	ErrEntitlementBlocked       = errors.New("current entitlement is blocked")
	ErrEffectiveRights          = errors.New("effective rights calculation is not allowed")
)

var SupportedRightsActions = []string{"USE", "PROCESS", "DERIVE", "SHARE", "RAW_EXPORT", "RESALE", "AI_TRAINING"}

type NormalizedScope struct {
	Type string `json:"type"`
	Ref  string `json:"ref"`
}

func NewNormalizedScope(scopeType, scopeRef string) (NormalizedScope, error) {
	scopeType = strings.ToUpper(strings.TrimSpace(scopeType))
	scopeRef = strings.TrimSpace(scopeRef)
	if scopeRef == "" {
		return NormalizedScope{}, ErrInvalidBinding
	}
	switch scopeType {
	case "ALL_RESOURCE", "OBJECT", "ROW", "PREFIX", "POLICY":
	default:
		return NormalizedScope{}, ErrInvalidBinding
	}
	return NormalizedScope{Type: scopeType, Ref: scopeRef}, nil
}

type RightsParty struct {
	PartyRef string `json:"partyRef"`
	Role     string `json:"role"`
}

type RightsPermission struct {
	Kind    string          `json:"kind"`
	Action  string          `json:"action,omitempty"`
	Purpose string          `json:"purpose,omitempty"`
	Scope   NormalizedScope `json:"scope,omitempty"`
}

type RightsDeclaration struct {
	ID                uuid.UUID
	WorkspaceID       uuid.UUID
	DataResourceID    uuid.UUID
	ClaimantRef       string
	BasisType         string
	BasisRef          string
	ConsumerScopeType string
	ConsumerRef       string
	EffectiveFrom     *time.Time
	EffectiveTo       *time.Time
	Parties           []RightsParty
	Permissions       []RightsPermission
	Restrictions      map[string]any
	EvidenceIDs       []uuid.UUID
	CreatedAt         time.Time
	CreatedBy         *uuid.UUID
}

type RightsDeclarationSpec struct {
	WorkspaceID       uuid.UUID
	DataResourceID    uuid.UUID
	ClaimantRef       string
	BasisType         string
	BasisRef          string
	ConsumerScopeType string
	ConsumerRef       string
	EffectiveFrom     *time.Time
	EffectiveTo       *time.Time
	Parties           []RightsParty
	Permissions       []RightsPermission
	Restrictions      map[string]any
	EvidenceIDs       []uuid.UUID
	ActorID           *uuid.UUID
}

func NewRightsDeclaration(spec RightsDeclarationSpec) (RightsDeclaration, error) {
	if spec.WorkspaceID == uuid.Nil || spec.DataResourceID == uuid.Nil || strings.TrimSpace(spec.ClaimantRef) == "" || strings.TrimSpace(spec.BasisType) == "" || strings.TrimSpace(spec.BasisRef) == "" || len(spec.Parties) == 0 || len(spec.Permissions) == 0 {
		return RightsDeclaration{}, ErrInvalidRightsDeclaration
	}
	consumerScope := strings.ToUpper(strings.TrimSpace(spec.ConsumerScopeType))
	if consumerScope == "" {
		consumerScope = "ANY"
	}
	consumerRef := strings.TrimSpace(spec.ConsumerRef)
	if consumerScope != "ANY" && (consumerScope != "EXPLICIT" || consumerRef == "") {
		return RightsDeclaration{}, ErrInvalidRightsDeclaration
	}
	if consumerScope == "ANY" {
		consumerRef = ""
	}
	if spec.EffectiveFrom != nil && spec.EffectiveTo != nil && !spec.EffectiveTo.After(*spec.EffectiveFrom) {
		return RightsDeclaration{}, ErrInvalidRightsDeclaration
	}
	declaration := RightsDeclaration{
		ID: uuid.New(), WorkspaceID: spec.WorkspaceID, DataResourceID: spec.DataResourceID,
		ClaimantRef: strings.TrimSpace(spec.ClaimantRef), BasisType: strings.TrimSpace(spec.BasisType), BasisRef: strings.TrimSpace(spec.BasisRef),
		ConsumerScopeType: consumerScope, ConsumerRef: consumerRef, EffectiveFrom: spec.EffectiveFrom, EffectiveTo: spec.EffectiveTo,
		Restrictions: spec.Restrictions, EvidenceIDs: append([]uuid.UUID(nil), spec.EvidenceIDs...), CreatedAt: time.Now().UTC(), CreatedBy: spec.ActorID,
	}
	if declaration.Restrictions == nil {
		declaration.Restrictions = map[string]any{}
	}
	seenParty := map[string]struct{}{}
	for _, party := range spec.Parties {
		party.PartyRef = strings.TrimSpace(party.PartyRef)
		party.Role = strings.ToUpper(strings.TrimSpace(party.Role))
		if party.PartyRef == "" || party.Role == "" {
			return RightsDeclaration{}, ErrInvalidRightsDeclaration
		}
		key := party.PartyRef + "\x00" + party.Role
		if _, ok := seenParty[key]; ok {
			continue
		}
		seenParty[key] = struct{}{}
		declaration.Parties = append(declaration.Parties, party)
	}
	for _, permission := range spec.Permissions {
		permission.Kind = strings.ToUpper(strings.TrimSpace(permission.Kind))
		permission.Action = strings.ToUpper(strings.TrimSpace(permission.Action))
		permission.Purpose = strings.TrimSpace(permission.Purpose)
		if permission.Kind != PermissionUse && permission.Kind != PermissionGrant {
			return RightsDeclaration{}, ErrInvalidRightsDeclaration
		}
		if permission.Action == "" || !contains(SupportedRightsActions, permission.Action) {
			return RightsDeclaration{}, ErrInvalidRightsDeclaration
		}
		if permission.Scope.Ref != "" {
			var err error
			permission.Scope, err = NewNormalizedScope(permission.Scope.Type, permission.Scope.Ref)
			if err != nil {
				return RightsDeclaration{}, ErrInvalidRightsDeclaration
			}
		} else if permission.Action == "" {
			return RightsDeclaration{}, ErrInvalidRightsDeclaration
		}
		declaration.Permissions = append(declaration.Permissions, permission)
	}
	return declaration, nil
}

func (d RightsDeclaration) HasParty(partyRef, role string) bool {
	partyRef, role = strings.TrimSpace(partyRef), strings.ToUpper(strings.TrimSpace(role))
	for _, party := range d.Parties {
		if party.PartyRef == partyRef && party.Role == role {
			return true
		}
	}
	return false
}

func (d RightsDeclaration) Allows(kind, action, purpose, consumer string, scope NormalizedScope, asOf time.Time) bool {
	if d.EffectiveFrom != nil && asOf.Before(*d.EffectiveFrom) {
		return false
	}
	if d.EffectiveTo != nil && !asOf.Before(*d.EffectiveTo) {
		return false
	}
	if d.ConsumerScopeType == "EXPLICIT" && d.ConsumerRef != strings.TrimSpace(consumer) {
		return false
	}
	for _, permission := range d.Permissions {
		if permission.Kind != strings.ToUpper(kind) {
			continue
		}
		if permission.Action != strings.ToUpper(action) {
			if permission.Purpose == strings.TrimSpace(purpose) && action == "" {
				return true
			}
			continue
		}
		if permission.Scope.Ref != "" && !ScopeCovers(permission.Scope, scope) {
			continue
		}
		return true
	}
	return false
}

func ScopeCovers(allowed, requested NormalizedScope) bool {
	if allowed.Type == "ALL_RESOURCE" {
		return allowed.Ref == requested.Ref || requested.Ref != ""
	}
	return allowed.Type == requested.Type && allowed.Ref == requested.Ref
}

type RightsVerification struct {
	ID            uuid.UUID
	DeclarationID uuid.UUID
	Outcome       string
	Reason        string
	EvidenceID    *uuid.UUID
	OccurredAt    time.Time
	ActorID       *uuid.UUID
	ActivityID    *uuid.UUID
}

type RightsDisposition struct {
	ID            uuid.UUID
	DeclarationID uuid.UUID
	Disposition   string
	EffectiveAt   time.Time
	Reason        string
	SupersededBy  *uuid.UUID
	EvidenceID    *uuid.UUID
	ActivityID    *uuid.UUID
	ActorID       *uuid.UUID
}

type AuthorizationProvenanceBinding struct {
	ID                  uuid.UUID
	WorkspaceID         uuid.UUID
	AuthorizationID     uuid.UUID
	DataResourceID      uuid.UUID
	DeclarationID       uuid.UUID
	GrantorRef          string
	AuthorityMode       string
	DelegationChainID   *uuid.UUID
	DelegationChainHash string
	CreatedAt           time.Time
	CreatedBy           *uuid.UUID
}

type BindingDisposition struct {
	ID           uuid.UUID
	BindingID    uuid.UUID
	Disposition  string
	EffectiveAt  time.Time
	Reason       string
	SupersededBy *uuid.UUID
	EvidenceID   *uuid.UUID
	ActivityID   *uuid.UUID
	ActorID      *uuid.UUID
}

type DelegationEdge struct {
	ID                uuid.UUID
	Ordinal           int
	DelegatorRef      string
	DelegateRef       string
	DataResourceID    uuid.UUID
	GrantableActions  []string
	GrantablePurposes []string
	Scope             NormalizedScope
	ValidFrom         *time.Time
	ValidTo           *time.Time
}

type DelegationChain struct {
	ID                  uuid.UUID
	WorkspaceID         uuid.UUID
	SourceDeclarationID uuid.UUID
	Status              string
	ChainHash           string
	Edges               []DelegationEdge
	CreatedAt           time.Time
	CreatedBy           *uuid.UUID
}

type DelegationDisposition struct {
	ID          uuid.UUID
	ChainID     uuid.UUID
	EdgeID      *uuid.UUID
	Disposition string
	EffectiveAt time.Time
	Reason      string
	EvidenceID  *uuid.UUID
	ActorID     *uuid.UUID
}

func HashDelegationEdges(edges []DelegationEdge) string {
	parts := make([]string, 0, len(edges))
	for _, edge := range edges {
		actions := append([]string(nil), edge.GrantableActions...)
		sort.Strings(actions)
		purposes := append([]string(nil), edge.GrantablePurposes...)
		sort.Strings(purposes)
		parts = append(parts, strings.Join([]string{edge.ID.String(), edge.DelegatorRef, edge.DelegateRef, edge.DataResourceID.String(), strings.Join(actions, ","), strings.Join(purposes, ","), edge.Scope.Type, edge.Scope.Ref}, "|"))
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(digest[:])
}

type EffectiveRightsInput struct {
	ID                    uuid.UUID
	InputDatasetVersionID uuid.UUID
	DataResourceID        uuid.UUID
	RightsSnapshotID      *uuid.UUID
	DeclarationID         *uuid.UUID
	BindingID             *uuid.UUID
	InputHash             string
}

type EffectiveRightsAction struct {
	ID              uuid.UUID
	Action          string
	Decision        string
	Reason          string
	BlockingInputID *uuid.UUID
}

type EffectiveRightsSnapshot struct {
	ID                     uuid.UUID
	WorkspaceID            uuid.UUID
	TargetDatasetVersionID uuid.UUID
	CalculationAsOf        time.Time
	ConsumerRef            string
	Purpose                string
	CalculationRuleVersion string
	CalculationRuleHash    string
	RequiredInputHash      string
	Status                 string
	RootHash               string
	Inputs                 []EffectiveRightsInput
	Actions                []EffectiveRightsAction
	CreatedAt              time.Time
	CreatedBy              *uuid.UUID
}

type EntitlementPath string

const (
	EntitlementDirectUse  EntitlementPath = "DIRECT_USE"
	EntitlementDownstream EntitlementPath = "DOWNSTREAM_AUTHORIZATION"
)

type EntitlementRequest struct {
	WorkspaceID     uuid.UUID
	AuthorizationID uuid.UUID
	DataResourceID  uuid.UUID
	ConsumerRef     string
	Purpose         string
	Action          string
	Scope           NormalizedScope
	AsOf            time.Time
	Path            EntitlementPath
}

type EntitlementDecision struct {
	Decision        string
	Reason          string
	AuthorizationID uuid.UUID
	DeclarationID   *uuid.UUID
	BindingID       *uuid.UUID
	DataResourceID  uuid.UUID
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
