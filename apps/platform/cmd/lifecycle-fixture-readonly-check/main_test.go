package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func evidence(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source missing")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../docs/acceptance/evidence/issue307-d1"))
}
func clone(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	files, e := os.ReadDir(src)
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range files {
		if !f.IsDir() {
			b, e := os.ReadFile(filepath.Join(src, f.Name()))
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(dst, f.Name()), b, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	return dst
}
func mutateJSON(t *testing.T, path string, mutate func(map[string]any)) {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var doc map[string]any
	if json.Unmarshal(b, &doc) != nil {
		t.Fatal("bad fixture")
	}
	mutate(doc)
	b, e = json.Marshal(doc)
	if e != nil {
		t.Fatal(e)
	}
	if os.WriteFile(path, b, 0600) != nil {
		t.Fatal("write failed")
	}
}
func TestCompleteAggregationRecordedReorderingAndAllGroups(t *testing.T) {
	root := evidence(t)
	for _, g := range []string{"business-races", "late-hold", "mixed"} {
		t.Run(g, func(t *testing.T) {
			baseline := filepath.Join(root, "remaining-candidate07", g, "readonly")
			fresh := baseline
			if g == "business-races" {
				fresh = filepath.Join(root, "delete-confirmed-v17-window01", g, "readonly")
			}
			out, e := check(baseline, fresh, g)
			if e != nil || out.Status != "PASS_COMPLETE_NATIVE_AGGREGATION" {
				t.Fatalf("full receipt rejected: %+v %v", out, e)
			}
			changed := 0
			for _, p := range out.Policies {
				if p.BaselineFileSHA256 != p.FreshFileSHA256 {
					changed++
				}
			}
			if g == "business-races" && changed != 3 {
				t.Fatalf("lost actual three order changes: %d", changed)
			}
		})
	}
}
func TestCompleteAggregationRejectsChangedAuthorization(t *testing.T) {
	baseline := filepath.Join(evidence(t), "remaining-candidate07/business-races/readonly")
	policyName := "native-policy-lifecycle307-remaining-business-remainread808a6960.json"
	cases := map[string]func(map[string]any){
		"extra-action":      func(s map[string]any) { s["Action"] = []string{"s3:GetObjectVersion", "s3:DeleteObjectVersion"} },
		"wildcard-resource": func(s map[string]any) { s["Resource"] = []string{"arn:aws:s3:::*/*"} },
		"foreign-resource":  func(s map[string]any) { s["Resource"] = []string{"arn:aws:s3:::foreign/fixture/p1.csv"} },
		"foreign-version": func(s map[string]any) {
			s["Condition"] = map[string]any{"StringEquals": map[string]string{"s3:versionid": "foreign-version"}}
		},
		"removed-condition": func(s map[string]any) { delete(s, "Condition") },
		"changed-effect":    func(s map[string]any) { s["Effect"] = "Deny" },
	}
	for label, change := range cases {
		t.Run(label, func(t *testing.T) {
			fresh := clone(t, baseline)
			mutateJSON(t, filepath.Join(fresh, policyName), func(d map[string]any) { change(d["Statement"].([]any)[0].(map[string]any)) })
			if _, e := check(baseline, fresh, "business-races"); e == nil {
				t.Fatal("expanded or changed authorization accepted")
			}
		})
	}
}
func TestCompleteAggregationRejectsIncompleteAndTamperedReceipts(t *testing.T) {
	baseline := filepath.Join(evidence(t), "remaining-candidate07/business-races/readonly")
	cases := map[string]func(map[string]any){
		"missing-delete":   func(d map[string]any) { delete(d, "delete") },
		"mutation":         func(d map[string]any) { d["mutations"] = 1 },
		"delete":           func(d map[string]any) { d["delete"] = 1 },
		"status":           func(d map[string]any) { d["status"] = "FAIL" },
		"foreign-group":    func(d map[string]any) { d["group"] = "mixed" },
		"inventory":        func(d map[string]any) { d["inventory_sha256"] = strings.Repeat("a", 64) },
		"user-policy":      func(d map[string]any) { d["disabled_identities"].(map[string]any)["delete808a6960"] = "foreign" },
		"identity-removed": func(d map[string]any) { delete(d["disabled_identities"].(map[string]any), "delete808a6960") },
		"policy-substituted": func(d map[string]any) {
			m := d["policy_sha256"].(map[string]any)
			for n, v := range m {
				delete(m, n)
				m["lifecycle307-foreign"] = v
				break
			}
		},
		"hold": func(d map[string]any) { d["copies"].([]any)[0].(map[string]any)["LegalHold"] = false },
		"version": func(d map[string]any) {
			d["copies"].([]any)[0].(map[string]any)["Copy"].(map[string]any)["ProviderVersion"] = "foreign"
		},
		"size": func(d map[string]any) {
			d["copies"].([]any)[0].(map[string]any)["Copy"].(map[string]any)["ByteSize"] = 1
		},
	}
	for label, change := range cases {
		t.Run(label, func(t *testing.T) {
			fresh := clone(t, baseline)
			mutateJSON(t, filepath.Join(fresh, "native-verification.json"), change)
			if _, e := check(baseline, fresh, "business-races"); e == nil {
				t.Fatal("unsafe complete receipt accepted")
			}
		})
	}
	for _, label := range []string{"missing-policy", "extra-policy", "malformed-policy", "trailing-policy"} {
		t.Run(label, func(t *testing.T) {
			fresh := clone(t, baseline)
			files, _ := filepath.Glob(filepath.Join(fresh, "native-policy-*.json"))
			switch label {
			case "missing-policy":
				_ = os.Remove(files[0])
			case "extra-policy":
				_ = os.WriteFile(filepath.Join(fresh, "native-policy-lifecycle307-extra.json"), []byte("{}"), 0600)
			case "malformed-policy":
				_ = os.WriteFile(files[0], []byte("invalid"), 0600)
			case "trailing-policy":
				raw, _ := os.ReadFile(files[0])
				_ = os.WriteFile(files[0], append(raw, []byte("{}")...), 0600)
			}
			if _, e := check(baseline, fresh, "business-races"); e == nil {
				t.Fatal("policy file collection accepted")
			}
		})
	}
}

func TestExactSizeComparisonDoesNotRoundLargeIntegers(t *testing.T) {
	source := filepath.Join(evidence(t), "remaining-candidate07/business-races/readonly")
	baseline, fresh := clone(t, source), clone(t, source)
	for _, v := range []struct{ dir, size string }{{baseline, "9007199254740992"}, {fresh, "9007199254740993"}} {
		mutateJSON(t, filepath.Join(v.dir, "native-verification.json"), func(d map[string]any) {
			d["copies"].([]any)[0].(map[string]any)["Copy"].(map[string]any)["ByteSize"] = json.Number(v.size)
		})
	}
	if _, e := check(baseline, fresh, "business-races"); e == nil {
		t.Fatal("different exact sizes rounded together")
	}
}
