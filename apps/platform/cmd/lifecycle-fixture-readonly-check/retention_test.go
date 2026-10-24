package main

import (
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func retentionMock(t *testing.T, group, phase string) (string, string, string, string) {
	t.Helper()
	baseline, fresh, expectation, inventory, _ := sixMock(t, group, "SOURCE")
	var old sixExpectation
	strictPostFile(expectation, &old, 14)
	end := time.Date(2026, 10, 24, 10, 40, 0, 0, time.UTC)
	want := retentionExpectation{old.postV22Expectation, uuid.NewString(), phase, old.Candidate, end.Add(24 * time.Hour), end}
	var r postV22Receipt
	strictPostFile(filepath.Join(fresh, "six-native-verification.json"), &r, 15)
	rows, err := retentionPhaseRows(group, phase, r.Verification.Copies, want.RetainUntil)
	if err != nil {
		t.Fatal(err)
	}
	r.Verification.Copies = rows
	raw, _ := json.Marshal(rows)
	if os.WriteFile(inventory, raw, 0600) != nil {
		t.Fatal("inventory")
	}
	want.Present = digest(raw)
	r.Verification.Inventory = want.Present
	r.RetainUntil = &want.RetainUntil
	r.MaintenanceEnd = &want.MaintenanceEnd
	r.MaintenanceID = want.MaintenanceID
	r.Phase = want.Phase
	r.BindingsSHA256 = want.BindingsSHA256
	r.Schema = "RETENTION_MAINTENANCE_NATIVE_READONLY"
	r.Verification.Source = "retention-readonly.json"
	putPostJSON(t, expectation, want)
	putPostJSON(t, filepath.Join(fresh, "retention-native-verification.json"), r)
	return baseline, fresh, expectation, inventory
}

func TestRetentionWholeFileAllSeventeenBothPhases(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		total := 0
		for _, group := range []string{"business-races", "late-hold", "mixed"} {
			b, f, e, i := retentionMock(t, group, phase)
			out, err := checkRetentionMaintenance(b, f, group, e, i)
			if err != nil {
				t.Fatal(group, phase, err)
			}
			total += out.Copies
		}
		if total != 17 {
			t.Fatal("not full seventeen", total)
		}
	}
}

func TestRetentionWholeFileRejectsIdentityProtectionPhaseAndJournalDrift(t *testing.T) {
	for _, fault := range []string{"wrong-phase", "foreign-new-id", "missing-identity", "old-id", "source-alias", "hold", "version", "unrelated-expiry", "journal", "missing-user", "history"} {
		t.Run(fault, func(t *testing.T) {
			b, f, e, i := retentionMock(t, "business-races", "after")
			var want retentionExpectation
			strictPostFile(e, &want, 16)
			var r postV22Receipt
			path := filepath.Join(f, "retention-native-verification.json")
			strictPostFile(path, &r, 20)
			switch fault {
			case "foreign-new-id":
				want.MaintenanceID = uuid.NewString()
			case "missing-identity":
				r.MaintenanceID = ""
			case "wrong-phase":
				want.Phase = "before"
			case "old-id":
				want.MaintenanceID = "4fad1007-6595-4037-a7a5-1d8d38cb8bcc"
			case "source-alias":
				want.Native = want.Core
			case "journal":
				starts, _ := filepath.Glob(filepath.Join(f, "readonly-start-*.json"))
				os.Remove(starts[0])
			case "missing-user":
				for user := range r.Verification.Users {
					delete(r.Verification.Users, user)
					break
				}
			case "history":
				os.WriteFile(filepath.Join(b, "frozen-inventory.json"), []byte(`[]`), 0600)
			default:
				var row map[string]any
				json.Unmarshal(r.Verification.Copies[0], &row)
				if fault == "hold" {
					row["LegalHold"] = !row["LegalHold"].(bool)
				}
				if fault == "unrelated-expiry" {
					row["RetainUntil"] = "2026-10-24T12:40:00Z"
				}
				if fault == "version" {
					row["Copy"].(map[string]any)["ProviderVersion"] = "foreign"
				}
				r.Verification.Copies[0], _ = json.Marshal(row)
			}
			putPostJSON(t, e, want)
			putPostJSON(t, path, r)
			if _, err := checkRetentionMaintenance(b, f, "business-races", e, i); err == nil {
				t.Fatal("admitted", fault)
			}
		})
	}
}

func TestRetentionLateHoldPhaseCannotBeRelabelled(t *testing.T) {
	b, f, e, i := retentionMock(t, "late-hold", "before")
	var want retentionExpectation
	strictPostFile(e, &want, 16)
	want.Phase = "after"
	putPostJSON(t, e, want)
	if _, err := checkRetentionMaintenance(b, f, "late-hold", e, i); err == nil {
		t.Fatal("unchanged copies hid phase relabel")
	}
}

func TestRetentionAllGroupsRequireActualBoundDatesEvenWhenRowsUnchanged(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		for _, phase := range []string{"before", "after"} {
			for _, fault := range []string{"actual-until", "actual-end", "missing-until", "missing-end", "short-horizon", "long-horizon"} {
				t.Run(group+"/"+phase+"/"+fault, func(t *testing.T) {
					b, f, e, i := retentionMock(t, group, phase)
					var want retentionExpectation
					strictPostFile(e, &want, 16)
					var actual postV22Receipt
					path := filepath.Join(f, "retention-native-verification.json")
					strictPostFile(path, &actual, 20)
					changed := want.RetainUntil.Add(time.Second)
					switch fault {
					case "actual-until":
						actual.RetainUntil = &changed
					case "actual-end":
						actual.MaintenanceEnd = &changed
					case "missing-until":
						actual.RetainUntil = nil
					case "missing-end":
						actual.MaintenanceEnd = nil
					case "short-horizon":
						want.RetainUntil = want.MaintenanceEnd.Add(2*time.Hour - time.Second)
						actual.RetainUntil = &want.RetainUntil
					case "long-horizon":
						want.RetainUntil = want.MaintenanceEnd.Add(72*time.Hour + time.Second)
						actual.RetainUntil = &want.RetainUntil
					}
					putPostJSON(t, e, want)
					putPostJSON(t, path, actual)
					if _, err := checkRetentionMaintenance(b, f, group, e, i); err == nil {
						t.Fatal("date relabel admitted")
					}
				})
			}
		}
	}
}
