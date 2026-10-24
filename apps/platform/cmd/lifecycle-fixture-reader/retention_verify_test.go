package main

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"strings"
	"testing"
	"time"
)

func TestRetentionReadonlyAllGroupsBothPhasesAndCrossFieldDenials(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		for _, phase := range []string{"before", "after"} {
			t.Run(group+"/"+phase, func(t *testing.T) {
				p, frozen, present := postProfile(t, group)
				p.Purpose = retentionReadPurpose
				p.CandidateSHA256 = strings.Repeat("c", 64)
				_, actual, _, err := postV22Inventories(p, frozen, present)
				if err != nil {
					t.Fatal(err)
				}
				effective, err := pilot.RetentionMaintenanceInventory(group, phase, actual)
				if err != nil {
					t.Fatal(err)
				}
				rawExpected, _ := json.Marshal(effective)
				profile := retentionReadProfile{p, uuid.New(), phase, p.CandidateSHA256, postV22Hash(rawExpected)}
				raw, _ := json.Marshal(profile)
				parsed, err := parseRetentionReadProfile(raw, time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				if _, err = retentionEffectiveInventory(parsed, actual, rawExpected); err != nil {
					t.Fatal(err)
				}
				for _, fault := range []string{"phase", "execution", "source", "delete", "hash", "deadline", "missing-zero", "duplicate"} {
					var bad map[string]any
					json.Unmarshal(raw, &bad)
					switch fault {
					case "phase":
						bad["Phase"] = "repair"
					case "execution":
						bad["MaintenanceID"] = pilot.RemainingExecution
					case "source":
						bad["Native"] = strings.Repeat("0", 64)
					case "delete":
						bad["MaxProviderDeletes"] = 1
					case "hash":
						bad["BindingsSHA256"] = strings.Repeat("d", 64)
					case "deadline":
						bad["ExpiresAt"] = p.NotBefore
					case "missing-zero":
						delete(bad, "MaxProviderDeletes")
					}
					changed, _ := json.Marshal(bad)
					if fault == "duplicate" {
						changed = append([]byte(`{"Phase":"before",`), changed[1:]...)
					}
					if _, err := parseRetentionReadProfile(changed, time.Now().UTC()); err == nil {
						t.Fatal("admitted", fault)
					}
				}
				// Rehashing supplied data cannot expand identity or legal hold changes.
				bad := append([]remainingExpected{}, effective...)
				bad[0].LegalHold = !bad[0].LegalHold
				changed, _ := json.Marshal(bad)
				parsed.EffectiveInventorySHA256 = postV22Hash(changed)
				if _, err = retentionEffectiveInventory(parsed, actual, changed); err == nil {
					t.Fatal("rehashed hold drift admitted")
				}
			})
		}
	}
}
