package main

import (
	"encoding/json"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

// Whole file-only entry with an explicitly labelled expectation. No old receipt
// is rewritten to pretend that the new users existed during the old audit.
func sixMock(t *testing.T, group, role string) (string, string, string, string, string) {
	t.Helper()
	old, fresh, expectation, inventory := postMock(t, group)
	baseline := clone(t, old)
	a, err := readReceipt(baseline)
	if err != nil {
		t.Fatal(err)
	}
	a.Status = "FROZEN_SIX_EXPECTATION_NOT_OBSERVATION"
	a.Source = "six-map.json"
	frozen, err := os.ReadFile(filepath.Join("..", "lifecycle-fixture-reader", "testdata", "post-v22", group+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(baseline, "frozen-inventory.json"), frozen, 0600); err != nil {
		t.Fatal(err)
	}
	var r postV22Receipt
	if _, err = strictPostFile(filepath.Join(fresh, "post-v22-native-verification.json"), &r, 15); err != nil {
		t.Fatal(err)
	}
	var rows []struct{ Copy domain.Copy }
	raw, _ := os.ReadFile(inventory)
	if json.Unmarshal(raw, &rows) != nil {
		t.Fatal("inventory")
	}
	cp := []domain.Copy{}
	for _, row := range rows {
		cp = append(cp, row.Copy)
	}
	users, policies, err := pilot.RemainingAuditPolicies(group, cp)
	if err != nil {
		t.Fatal(err)
	}
	for user, name := range users {
		a.Users[user] = name
		r.Verification.Users[user] = name
	}
	for name, policy := range policies {
		a.Policies[name] = digest(policy)
		r.Verification.Policies[name] = digest(policy)
		for _, dir := range []string{baseline, fresh} {
			if os.WriteFile(filepath.Join(dir, "native-policy-"+name+".json"), policy, 0600) != nil {
				t.Fatal("policy")
			}
		}
	}
	putPostJSON(t, filepath.Join(baseline, "expected-contract.json"), a)
	m := pilot.RemainingEnvironmentMap{Execution: pilot.RemainingExecution, Profile: pilot.RemainingProfile}
	for n, name := range []string{"business-races", "late-hold", "mixed"} {
		var row struct {
			Group           string
			Source, Restore [3]string
		}
		row.Group = name
		source := pilot.RemainingEnvironmentSources()[name]
		row.Source = [3]string{source[0], source[1], source[2]}
		for i := 0; i < 3; i++ {
			row.Restore[i] = strings.Repeat(string(rune('1'+n*3+i)), 64)
		}
		m.Groups = append(m.Groups, row)
	}
	mapping := filepath.Join(fresh, "six-map.json")
	putPostJSON(t, mapping, m)
	raw, _ = os.ReadFile(mapping)
	var want postV22Expectation
	if _, err = strictPostFile(expectation, &want, 11); err != nil {
		t.Fatal(err)
	}
	for _, g := range m.Groups {
		if g.Group == group {
			actual := g.Source
			if role == "RESTORE" {
				actual = g.Restore
			}
			want.Core, want.Native, want.Network = actual[0], actual[1], actual[2]
		}
	}
	want.Candidate = digest(raw)
	putPostJSON(t, expectation, sixExpectation{postV22Expectation: want, Execution: m.Execution, Profile: m.Profile, Role: role})
	r.Schema = "SIX_TARGET_PREPARATION_NATIVE_READONLY"
	r.Candidate = want.Candidate
	r.Core, r.Native, r.Network = want.Core, want.Native, want.Network
	r.Verification.Source = "six-readonly.json"
	putPostJSON(t, filepath.Join(fresh, "six-native-verification.json"), r)
	absent := map[string]any{}
	for _, user := range pilot.RemainingUncreatedIdentities(group) {
		id := uuid.NewString()
		absent[user] = map[string]any{"attempt_id": id, "status": 404, "code": "XMinioAdminNoSuchUser"}
		putPostJSON(t, filepath.Join(fresh, "readonly-start-"+id+".json"), map[string]any{"id": id, "audit_id": r.Audit, "method": "GET", "path": "/minio/admin/v3/user-info", "query": "accessKey=" + user, "at": r.Started})
		putPostJSON(t, filepath.Join(fresh, "readonly-observation-"+id+".json"), map[string]any{"id": id, "audit_id": r.Audit, "status": 404, "transport_error": false, "provider_error_code": "", "provider_request_id": "mock-request", "at": r.Started})
	}
	putPostJSON(t, filepath.Join(fresh, "six-uncreated-users.json"), map[string]any{"audit_id": r.Audit, "users": absent})

	return baseline, fresh, expectation, inventory, mapping
}

func TestSixWholeFileSourceRestoreAndNewPolicyRejection(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		for _, role := range []string{"SOURCE", "RESTORE"} {
			t.Run(group+"/"+role, func(t *testing.T) {
				base, fresh, expectation, inventory, mapping := sixMock(t, group, role)
				out, err := checkSix(base, fresh, group, expectation, inventory, mapping)
				if err != nil || out.Status != "PASS_COMPLETE_SIX_NATIVE_AGGREGATION" || out.Copies != map[string]int{"business-races": 7, "late-hold": 6, "mixed": 4}[group] {
					t.Fatal(out, err)
				}
				name := pilot.RemainingPolicyName(group, "delete")
				policy := filepath.Join(fresh, "native-policy-"+name+".json")
				mutateJSON(t, policy, func(d map[string]any) {
					d["Statement"].([]any)[0].(map[string]any)["Resource"] = []string{"arn:aws:s3:::*"}
				})
				if _, err = checkSix(base, fresh, group, expectation, inventory, mapping); err == nil {
					t.Fatal("expanded new policy accepted")
				}
				if err = os.Remove(policy); err != nil {
					t.Fatal(err)
				}
				if _, err = checkSix(base, fresh, group, expectation, inventory, mapping); err == nil {
					t.Fatal("missing new policy accepted")
				}
			})
		}
	}
}

func TestSixWholeFileRejectsAliasedRestoreAndForeignExecution(t *testing.T) {
	base, fresh, expectation, inventory, mapping := sixMock(t, "business-races", "RESTORE")
	mutateJSON(t, mapping, func(d map[string]any) { row := d["Groups"].([]any)[0].(map[string]any); row["Restore"] = row["Source"] })
	raw, _ := os.ReadFile(mapping)
	mutateJSON(t, expectation, func(d map[string]any) { d["candidate_sha256"] = digest(raw) })
	if _, err := checkSix(base, fresh, "business-races", expectation, inventory, mapping); err == nil {
		t.Fatal("rehashing waived source/restore alias")
	}
}

func TestSixExpectationExportKeepsHistoricalReceiptAndUsesKnownInventory(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		baseline, _, _, inventory := postMock(t, group)
		original, e := os.ReadFile(filepath.Join(baseline, "native-verification.json"))
		if e != nil {
			t.Fatal(e)
		}
		frozen := filepath.Join("..", "lifecycle-fixture-reader", "testdata", "post-v22", group+".json")
		output := filepath.Join(t.TempDir(), "expectation")
		if e = exportSixExpected(baseline, output, group, inventory, frozen); e != nil {
			t.Fatal(group, e)
		}
		expected, e := readNativeContract(output, "expected-contract.json", "FROZEN_SIX_EXPECTATION_NOT_OBSERVATION", "six-map.json")
		if e != nil {
			t.Fatal(e)
		}
		if len(expected.Users) != map[string]int{"business-races": 15, "late-hold": 10, "mixed": 10}[group] {
			t.Fatal("new IAM expectation incomplete")
		}
		for user, policy := range pilot.RemainingRetainedIdentities(group) {
			if expected.Users[user] != policy {
				t.Fatal("retired identity absent from current read expectation", group, user)
			}
		}
		after, _ := os.ReadFile(filepath.Join(baseline, "native-verification.json"))
		if string(after) != string(original) {
			t.Fatal("historical receipt rewritten")
		}
		if e = exportSixExpected(baseline, output, group, inventory, frozen); e == nil {
			t.Fatal("existing expectation overwritten")
		}
		bad := filepath.Join(t.TempDir(), "bad.json")
		if e = os.WriteFile(bad, []byte("[]"), 0600); e != nil {
			t.Fatal(e)
		}
		if e = exportSixExpected(baseline, filepath.Join(t.TempDir(), "rejected"), group, inventory, bad); e == nil {
			t.Fatal("unknown historical inventory accepted")
		}
	}
}

func TestSixUncreatedIdentityAndDormantEvidenceFailClosed(t *testing.T) {
	for _, fault := range []string{"missing-file", "unknown-user", "wrong-status", "dormant-policy"} {
		t.Run(fault, func(t *testing.T) {
			base, fresh, want, inventory, mapping := sixMock(t, "business-races", "SOURCE")
			proof := filepath.Join(fresh, "six-uncreated-users.json")
			switch fault {
			case "missing-file":
				if err := os.Remove(proof); err != nil {
					t.Fatal(err)
				}
			case "unknown-user":
				mutateJSON(t, proof, func(d map[string]any) { d["users"].(map[string]any)["other"] = map[string]any{} })
			case "wrong-status":
				mutateJSON(t, proof, func(d map[string]any) {
					for _, r := range d["users"].(map[string]any) {
						r.(map[string]any)["status"] = 200
					}
				})
			case "dormant-policy":
				mutateJSON(t, filepath.Join(base, "expected-contract.json"), func(d map[string]any) { d["disabled_identities"].(map[string]any)["i4e524c04808a6960"] = "admin" })
			}
			if _, err := checkSix(base, fresh, "business-races", want, inventory, mapping); err == nil {
				t.Fatal("absence/dormant expansion admitted")
			}
		})
	}
}
