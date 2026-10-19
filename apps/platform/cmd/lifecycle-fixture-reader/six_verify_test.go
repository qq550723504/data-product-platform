package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func sixMap(t *testing.T) ([]byte, sixEnvironmentMap) {
	t.Helper()
	m := sixEnvironmentMap{Execution: pilot.RemainingExecution, Profile: pilot.RemainingProfile}
	for n, g := range []string{"business-races", "late-hold", "mixed"} {
		var row struct {
			Group           string
			Source, Restore [3]string
		}
		row.Group = g
		source := postV22Sources[g]
		row.Source = [3]string{source[0], source[1], source[2]}
		for i := 0; i < 3; i++ {
			row.Restore[i] = strings.Repeat(string(rune('a'+n*3+i)), 64)
		}
		// Use canonical hexadecimal for all three generated restore identities.
		if n == 2 {
			row.Restore = [3]string{strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)}
		}
		m.Groups = append(m.Groups, row)
	}
	b, _ := json.Marshal(m)
	return b, m
}

func TestSixReadOnlySourceRestoreMapsAndProfileCounterexamples(t *testing.T) {
	mapping, m := sixMap(t)
	// The fixture above generates 'g' for the third group only, overridden to 1/2/3.
	for n, g := range m.Groups {
		for _, role := range []string{"SOURCE", "RESTORE"} {
			p, _, _ := postProfile(t, g.Group)
			p.Purpose = sixReadPurpose
			p.CandidateSHA256 = postV22Hash(mapping)
			actual := g.Source
			if role == "RESTORE" {
				actual = g.Restore
			}
			p.Core, p.Native, p.Network = actual[0], actual[1], actual[2]
			s := sixReadProfile{postV22ReadProfile: p, Execution: m.Execution, Profile: m.Profile, Role: role, MapSHA256: postV22Hash(mapping)}
			raw, _ := json.Marshal(s)
			if _, err := parseSixReadProfile(raw, mapping, time.Now().UTC()); err != nil {
				t.Fatalf("%d %s: %v", n, role, err)
			}
			for _, field := range []string{"Enabled", "MaxProviderDeletes", "Execution", "Profile", "Role", "MapSHA256", "Core", "Native", "Network", "FrozenInventorySHA256", "ExpiresAt"} {
				var bad map[string]any
				_ = json.Unmarshal(raw, &bad)
				delete(bad, field)
				changed, _ := json.Marshal(bad)
				if _, err := parseSixReadProfile(changed, mapping, time.Now().UTC()); err == nil {
					t.Fatalf("missing %s admitted", field)
				}
			}
			bad := s
			bad.Native = m.Groups[0].Source[1]
			changed, _ := json.Marshal(bad)
			if role == "RESTORE" {
				if _, err := parseSixReadProfile(changed, mapping, time.Now().UTC()); err == nil {
					t.Fatal("source masqueraded as restore")
				}
			}
		}
	}
}
