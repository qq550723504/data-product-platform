package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRightsDeclarationRequiresExplicitGrantAuthorityAndNormalizedScope(t *testing.T) {
	base := RightsDeclarationSpec{
		WorkspaceID: uuid.New(), DataResourceID: uuid.New(), ClaimantRef: "party-a", BasisType: "LICENSE", BasisRef: "license-1",
		Parties:     []RightsParty{{PartyRef: "party-a", Role: "RIGHTS_HOLDER"}},
		Permissions: []RightsPermission{{Kind: PermissionUse, Action: "USE", Purpose: "P1", Scope: NormalizedScope{Type: "PREFIX", Ref: "/data/a"}}},
	}
	if _, err := NewRightsDeclaration(base); err != nil {
		t.Fatalf("valid declaration rejected: %v", err)
	}
	base.Permissions = []RightsPermission{{Kind: PermissionUse, Action: "USE", Purpose: "P1"}}
	if _, err := NewRightsDeclaration(base); err == nil {
		t.Fatal("scope-less permission was accepted")
	}
	base.Permissions = []RightsPermission{{Kind: PermissionUse, Action: "USE", Scope: NormalizedScope{Type: "PREFIX", Ref: "/data/a"}}}
	if _, err := NewRightsDeclaration(base); err == nil {
		t.Fatal("purpose-less permission was accepted")
	}
	base.Permissions = []RightsPermission{{Kind: PermissionUse, Action: "NOT_A_REAL_ACTION", Purpose: "P1", Scope: NormalizedScope{Type: "PREFIX", Ref: "/data/a"}}}
	if _, err := NewRightsDeclaration(base); err == nil {
		t.Fatal("unsupported action was accepted")
	}
}

func TestDeclarationAllowsChecksEveryContextDimension(t *testing.T) {
	now := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	d, err := NewRightsDeclaration(RightsDeclarationSpec{
		WorkspaceID: uuid.New(), DataResourceID: uuid.New(), ClaimantRef: "party-a", BasisType: "LICENSE", BasisRef: "license-1", ConsumerScopeType: "EXPLICIT", ConsumerRef: "consumer-b",
		EffectiveFrom: func() *time.Time { v := now.Add(-time.Hour); return &v }(), EffectiveTo: func() *time.Time { v := now.Add(time.Hour); return &v }(),
		Parties:     []RightsParty{{PartyRef: "party-a", Role: "RIGHTS_HOLDER"}},
		Permissions: []RightsPermission{{Kind: PermissionUse, Action: "SHARE", Purpose: "P1", Scope: NormalizedScope{Type: "PREFIX", Ref: "/a"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allows(PermissionUse, "SHARE", "P1", "consumer-b", NormalizedScope{Type: "PREFIX", Ref: "/a"}, now) {
		t.Fatal("matching context was blocked")
	}
	for name, consumer := range map[string]string{"consumer mismatch": "consumer-a", "purpose mismatch": "P2"} {
		t.Run(name, func(t *testing.T) {
			purpose := "P1"
			if name == "purpose mismatch" {
				purpose = "P2"
			}
			if d.Allows(PermissionUse, "SHARE", purpose, consumer, NormalizedScope{Type: "PREFIX", Ref: "/a"}, now) {
				t.Fatal("mismatched context was allowed")
			}
		})
	}
	if d.Allows(PermissionUse, "SHARE", "P1", "consumer-b", NormalizedScope{Type: "PREFIX", Ref: "/b"}, now) {
		t.Fatal("mismatched scope was allowed")
	}
}

func TestDelegationHashIsStableForOrderedEdges(t *testing.T) {
	edges := []DelegationEdge{{ID: uuid.New(), Ordinal: 0, DelegatorRef: "a", DelegateRef: "b", DataResourceID: uuid.New(), Scope: NormalizedScope{Type: "ALL_RESOURCE", Ref: "resource"}}}
	if HashDelegationEdges(edges) == "" || HashDelegationEdges(edges) != HashDelegationEdges(edges) {
		t.Fatal("delegation hash is not stable")
	}
}

func TestDelegationHashBindsValidityAndOrdinal(t *testing.T) {
	from := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	base := DelegationEdge{
		ID: uuid.New(), Ordinal: 0, DelegatorRef: "a", DelegateRef: "b", DataResourceID: uuid.New(),
		GrantableActions: []string{"USE"}, GrantablePurposes: []string{"P1"},
		Scope: NormalizedScope{Type: "ALL_RESOURCE", Ref: "resource"}, ValidFrom: &from, ValidTo: &to,
	}
	baseHash := HashDelegationEdges([]DelegationEdge{base})

	changedFrom := base
	laterFrom := from.Add(time.Hour)
	changedFrom.ValidFrom = &laterFrom
	if HashDelegationEdges([]DelegationEdge{changedFrom}) == baseHash {
		t.Fatal("delegation hash did not bind valid_from")
	}

	changedTo := base
	laterTo := to.Add(time.Hour)
	changedTo.ValidTo = &laterTo
	if HashDelegationEdges([]DelegationEdge{changedTo}) == baseHash {
		t.Fatal("delegation hash did not bind valid_to")
	}

	changedOrdinal := base
	changedOrdinal.Ordinal = 1
	if HashDelegationEdges([]DelegationEdge{changedOrdinal}) == baseHash {
		t.Fatal("delegation hash did not bind ordinal")
	}
}
