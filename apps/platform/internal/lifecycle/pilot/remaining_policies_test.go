package pilot

import (
	"reflect"
	"testing"
)

func TestRemainingRetainedIdentitiesKeepActualC14C15C16AndTerminalC17Users(t *testing.T) {
	want := map[string][]string{
		"business-races": {"iafc0e66c808a6960", "dafc0e66c808a6960", "i9f9c67d7808a6960", "d9f9c67d7808a6960", "iaa2605a5808a6960", "daa2605a5808a6960", "i6f40bca9808a6960", "d6f40bca9808a6960"},
		"late-hold":      {"i9f9c67d7754ea8f0", "d9f9c67d7754ea8f0", "iaa2605a5754ea8f0", "daa2605a5754ea8f0", "i6f40bca9754ea8f0", "d6f40bca9754ea8f0"},
		"mixed":          {"i9f9c67d7dfda43b4", "d9f9c67d7dfda43b4", "iaa2605a5dfda43b4", "daa2605a5dfda43b4", "i6f40bca9dfda43b4", "d6f40bca9dfda43b4"},
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
