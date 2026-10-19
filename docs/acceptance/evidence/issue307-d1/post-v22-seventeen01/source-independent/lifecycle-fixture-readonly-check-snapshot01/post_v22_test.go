package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func putPostJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	if os.WriteFile(path, b, 0600) != nil {
		t.Fatal("write")
	}
}

// This is a file-only MOCK composition using frozen original object identities
// and policies. It does not stand in for new native reads or a Core checkpoint.
func postMock(t *testing.T, group string) (string, string, string, string) {
	t.Helper()
	baseline := filepath.Join(evidence(t), "remaining-candidate12", group, "readonly")
	fresh := clone(t, baseline)
	a, e := readReceipt(baseline)
	if e != nil {
		t.Fatal(e)
	}
	b := a
	b.Copies = nil
	b.Source = "post-v22-readonly.json"
	var p1 json.RawMessage
	for _, row := range a.Copies {
		var d map[string]json.RawMessage
		_ = json.Unmarshal(row, &d)
		var cp struct{ ID string }
		_ = json.Unmarshal(d["Copy"], &cp)
		if cp.ID == p1Copy {
			p1 = d["Copy"]
		} else {
			b.Copies = append(b.Copies, row)
		}
	}
	inventory := filepath.Join(fresh, "present-inventory.json")
	present, _ := json.Marshal(b.Copies)
	_ = os.WriteFile(inventory, present, 0600)
	b.Inventory = digest(present)
	at := time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	want := postV22Expectation{Audit: uuid.NewString(), Profile: strings.Repeat("a", 64), Binary: strings.Repeat("b", 64), Candidate: postV22Candidate, Frozen: a.Inventory, Present: b.Inventory, Core: strings.Repeat("c", 64), Native: strings.Repeat("d", 64), Network: strings.Repeat("e", 64), NotBefore: at, ExpiresAt: at.Add(40 * time.Minute)}
	expectation := filepath.Join(fresh, "expectation.json")
	putPostJSON(t, expectation, want)
	receipt := map[string]any{"schema": postV22Schema, "audit_id": want.Audit, "profile_sha256": want.Profile, "binary_sha256": want.Binary, "candidate_sha256": want.Candidate, "frozen_inventory_sha256": want.Frozen, "core": want.Core, "native": want.Native, "network": want.Network, "not_before": want.NotBefore, "expires_at": want.ExpiresAt, "started_at": at.Add(time.Second), "finished_at": at.Add(time.Minute), "p1_absence": nil, "verification": b}
	if group == "business-races" {
		id := uuid.NewString()
		receipt["p1_absence"] = map[string]any{"copy": p1, "attempt_id": id, "status": 404, "code": "NoSuchVersion"}
		var cp struct{ Namespace, Key, ProviderVersion string }
		_ = json.Unmarshal(p1, &cp)
		putPostJSON(t, filepath.Join(fresh, "readonly-start-"+id+".json"), map[string]any{"id": id, "method": "HEAD", "path": "/" + cp.Namespace + "/" + cp.Key, "query": "versionId=" + cp.ProviderVersion, "at": at.Add(2 * time.Second)})
		putPostJSON(t, filepath.Join(fresh, "readonly-observation-"+id+".json"), map[string]any{"id": id, "status": 404, "provider_error_code": "NoSuchVersion", "provider_request_id": "mock-provider-request", "transport_error": false, "at": at.Add(3 * time.Second)})
	}
	putPostJSON(t, filepath.Join(fresh, "post-v22-native-verification.json"), receipt)
	if e := os.Remove(filepath.Join(fresh, "native-verification.json")); e != nil {
		t.Fatal(e)
	}
	return baseline, fresh, expectation, inventory
}
func TestPostV22FileCompositionAndPolicyReordering(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		t.Run(group, func(t *testing.T) {
			baseline, fresh, expectation, inventory := postMock(t, group)
			policies, _ := filepath.Glob(filepath.Join(fresh, "native-policy-*.json"))
			for _, p := range policies {
				mutateJSON(t, p, func(d map[string]any) {
					// Official Equals treats Action/Resource as sets, while it
					// compares statement positions. Keep that bounded API contract.
					for _, row := range d["Statement"].([]any) {
						for _, key := range []string{"Action", "Resource"} {
							values, ok := row.(map[string]any)[key].([]any)
							if !ok {
								continue
							}
							for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
								values[i], values[j] = values[j], values[i]
							}
						}
					}
				})
			}
			out, e := checkPostV22(baseline, fresh, group, expectation, inventory)
			if e != nil || out.Copies != map[string]int{"business-races": 7, "late-hold": 6, "mixed": 4}[group] || out.Status != "PASS_COMPLETE_POST_V22_NATIVE_AGGREGATION" {
				t.Fatalf("%+v %v", out, e)
			}
			if _, e = check(baseline, fresh, group); e == nil && group == "business-races" {
				t.Fatal("post-v22 data accidentally accepted by legacy checker")
			}
		})
	}
}
func TestPostV22FileRejectsReceiptAndAbsenceSubstitutions(t *testing.T) {
	changes := map[string]func(map[string]any){
		"audit":           func(d map[string]any) { d["audit_id"] = uuid.NewString() },
		"binary":          func(d map[string]any) { d["binary_sha256"] = strings.Repeat("f", 64) },
		"profile":         func(d map[string]any) { d["profile_sha256"] = strings.Repeat("f", 64) },
		"core":            func(d map[string]any) { d["core"] = strings.Repeat("f", 64) },
		"unknown":         func(d map[string]any) { d["bypass"] = true },
		"outside-window":  func(d map[string]any) { d["finished_at"] = "2026-10-19T16:00:00Z" },
		"missing-absence": func(d map[string]any) { d["p1_absence"] = nil },
		"generic404":      func(d map[string]any) { d["p1_absence"].(map[string]any)["code"] = "NotFound" },
		"presentP1":       func(d map[string]any) { d["p1_absence"].(map[string]any)["status"] = 200 },
		"foreign-copy": func(d map[string]any) {
			d["p1_absence"].(map[string]any)["copy"].(map[string]any)["ID"] = uuid.NewString()
		},
		"foreign-version": func(d map[string]any) {
			d["p1_absence"].(map[string]any)["copy"].(map[string]any)["ProviderVersion"] = uuid.NewString()
		},
		"missing-zero": func(d map[string]any) { delete(d["verification"].(map[string]any), "delete") },
		"delete":       func(d map[string]any) { d["verification"].(map[string]any)["delete"] = 1 },
		"hold": func(d map[string]any) {
			d["verification"].(map[string]any)["copies"].([]any)[0].(map[string]any)["LegalHold"] = false
		},
		"size": func(d map[string]any) {
			d["verification"].(map[string]any)["copies"].([]any)[0].(map[string]any)["Copy"].(map[string]any)["ByteSize"] = 1
		},
		"missing-present": func(d map[string]any) { v := d["verification"].(map[string]any); v["copies"] = v["copies"].([]any)[1:] },
		"foreign-user": func(d map[string]any) {
			d["verification"].(map[string]any)["disabled_identities"].(map[string]any)["admin"] = "lifecycle307-admin"
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			base, fresh, expectation, inventory := postMock(t, "business-races")
			mutateJSON(t, filepath.Join(fresh, "post-v22-native-verification.json"), change)
			if _, e := checkPostV22(base, fresh, "business-races", expectation, inventory); e == nil {
				t.Fatal("bad post-v22 receipt accepted")
			}
		})
	}
}
func TestPostV22RejectsHEADSelectorAndPolicyExpansion(t *testing.T) {
	for _, label := range []string{"version", "selector-missing", "GET", "foreign-path", "transport-failure", "status", "generic-provider404", "missing-request-id", "absent-journal", "policy-expanded", "present-inventory-drift"} {
		t.Run(label, func(t *testing.T) {
			base, fresh, expectation, inventory := postMock(t, "business-races")
			var r postV22Receipt
			_, _ = strictPostFile(filepath.Join(fresh, "post-v22-native-verification.json"), &r, 15)
			start := filepath.Join(fresh, "readonly-start-"+r.Absence.Attempt+".json")
			obs := filepath.Join(fresh, "readonly-observation-"+r.Absence.Attempt+".json")
			switch label {
			case "version":
				mutateJSON(t, start, func(d map[string]any) { d["query"] = "versionId=foreign" })
			case "selector-missing":
				mutateJSON(t, start, func(d map[string]any) { d["query"] = "" })
			case "GET":
				mutateJSON(t, start, func(d map[string]any) { d["method"] = "GET" })
			case "foreign-path":
				mutateJSON(t, start, func(d map[string]any) { d["path"] = "/foreign/key" })
			case "transport-failure":
				mutateJSON(t, obs, func(d map[string]any) { d["transport_error"] = true })
			case "status":
				mutateJSON(t, obs, func(d map[string]any) { d["status"] = 400 })
			case "generic-provider404":
				mutateJSON(t, obs, func(d map[string]any) { d["provider_error_code"] = "" })
			case "missing-request-id":
				mutateJSON(t, obs, func(d map[string]any) { d["provider_request_id"] = "" })
			case "absent-journal":
				_ = os.Remove(start)
			case "present-inventory-drift":
				_ = os.WriteFile(inventory, []byte("[]"), 0600)
			case "policy-expanded":
				files, _ := filepath.Glob(filepath.Join(fresh, "native-policy-*.json"))
				mutateJSON(t, files[0], func(d map[string]any) { d["Statement"].([]any)[0].(map[string]any)["Action"] = []string{"s3:*"} })
			}
			if _, e := checkPostV22(base, fresh, "business-races", expectation, inventory); e == nil {
				t.Fatal("bad HEAD/policy/inventory accepted")
			}
		})
	}
}
