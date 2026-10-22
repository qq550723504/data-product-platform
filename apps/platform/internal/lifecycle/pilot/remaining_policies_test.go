package pilot

import (
	"reflect"
	"testing"
)

func TestRemainingRetainedIdentitiesKeepActualC14C15C16AndTerminalC17C18Users(t *testing.T) {
	want := map[string][]string{
		"business-races": {"iafc0e66c808a6960", "dafc0e66c808a6960", "i9f9c67d7808a6960", "d9f9c67d7808a6960", "iaa2605a5808a6960", "daa2605a5808a6960", "i6f40bca9808a6960", "d6f40bca9808a6960", "i4fd5a7e7808a6960", "d4fd5a7e7808a6960", "ia5d1e569808a6960", "da5d1e569808a6960"},
		"late-hold":      {"i9f9c67d7754ea8f0", "d9f9c67d7754ea8f0", "iaa2605a5754ea8f0", "daa2605a5754ea8f0", "i6f40bca9754ea8f0", "d6f40bca9754ea8f0", "i4fd5a7e7754ea8f0", "d4fd5a7e7754ea8f0", "ia5d1e569754ea8f0", "da5d1e569754ea8f0"},
		"mixed":          {"i9f9c67d7dfda43b4", "d9f9c67d7dfda43b4", "iaa2605a5dfda43b4", "daa2605a5dfda43b4", "i6f40bca9dfda43b4", "d6f40bca9dfda43b4", "i4fd5a7e7dfda43b4", "d4fd5a7e7dfda43b4", "ia5d1e569dfda43b4", "da5d1e569dfda43b4"},
	}
	for group, names := range want {
		t.Run(group, func(t *testing.T) {
			expected := map[string]string{}
			for _, name := range names {
				expected[name] = "lifecycle307-six-" + name
			}
			if got := RemainingRetainedIdentities(group); !reflect.DeepEqual(got, expected) {
				t.Fatalf("retained actual identities differ: %v", got)
			}
			for _, kind := range []string{"inspect", "delete"} {
				if expected[RemainingIdentity(group, kind)] != "" {
					t.Fatal("a retained identity is selected by the current execution")
				}
			}
		})
	}
	if len(RemainingRetainedIdentities("unbound")) != 0 {
		t.Fatal("an unbound group acquired retained identities")
	}
}

func TestRemainingUncreatedC20NeverBecomesDisabledOrCurrentIdentity(t *testing.T) {
	for group, count := range map[string]int{"business-races": 3, "late-hold": 6, "mixed": 6} {
		scope, err := RemainingScopeFor(group)
		if err != nil {
			t.Fatal(err)
		}
		users := RemainingUncreatedIdentities(group)
		if len(users) != count {
			t.Fatal("uncreated membership differs", group, users)
		}
		seen := map[string]bool{}
		for _, user := range users {
			if seen[user] || RemainingRetainedIdentities(group)[user] != "" || user == RemainingIdentity(group, "inspect") || user == RemainingIdentity(group, "delete") {
				t.Fatal("uncreated user was adopted or duplicated", group, user)
			}
			seen[user] = true
		}
		for _, prefix := range []string{"i", "d"} {
			if !seen[prefix+"5a54e01a"+scope.Run.String()[:8]] {
				t.Fatal("C20 absence omitted", group)
			}
		}
	}
	if RemainingUncreatedIdentities("unbound") != nil {
		t.Fatal("unbound group acquired an identity")
	}
}
