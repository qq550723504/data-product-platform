package pilot

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/minio/pkg/v3/policy"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func TestRemainingDeletePolicyCoversNativeEntryWithoutExpandingExactScope(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		raw, e := os.ReadFile("../../../cmd/lifecycle-fixture-reader/testdata/post-v22/" + group + ".json")
		if e != nil {
			t.Fatal(e)
		}
		var rows []struct{ Copy domain.Copy }
		if json.Unmarshal(raw, &rows) != nil {
			t.Fatal("fixture")
		}
		var present []domain.Copy
		for _, row := range rows {
			if row.Copy.Key == "fixture/p1.csv" {
				continue
			}
			present = append(present, row.Copy)
		}
		_, all, e := RemainingAuditPolicies(group, present)
		if e != nil {
			t.Fatal(e)
		}
		current, e := policy.ParseConfig(bytes.NewReader(all[RemainingPolicyName(group, "delete")]))
		if e != nil {
			t.Fatal(e)
		}
		for _, cp := range present {
			target := cp.Key == "fixture/p2.csv" || cp.Key == "fixture/t1.csv" || cp.Key == "fixture/t2.csv" || cp.Key == "fixture/h2-a.csv" || cp.Key == "fixture/m1-a.csv" || cp.Key == "fixture/m1-b.csv"
			for _, action := range []policy.Action{policy.DeleteObjectAction, policy.DeleteObjectVersionAction} {
				for _, version := range []string{cp.ProviderVersion, "", "foreign"} {
					args := policy.Args{Action: action, BucketName: cp.Namespace, ObjectName: cp.Key, ConditionValues: map[string][]string{"versionid": {version}}}
					if current.IsAllowed(args) != (target && version == cp.ProviderVersion) {
						t.Fatalf("%s %s %s %s", group, cp.Key, action, version)
					}
					args.ObjectName = "foreign.csv"
					if current.IsAllowed(args) {
						t.Fatal("foreign key")
					}
				}
			}
		}
		for user, name := range RemainingRetainedIdentities(group) {
			if user[0] != 'd' {
				continue
			}
			p, e := policy.ParseConfig(bytes.NewReader(all[name]))
			if e != nil {
				t.Fatal(e)
			}
			for _, cp := range present {
				if p.IsAllowed(policy.Args{Action: policy.DeleteObjectAction, BucketName: cp.Namespace, ObjectName: cp.Key, ConditionValues: map[string][]string{"versionid": {cp.ProviderVersion}}}) {
					t.Fatal("retained frozen policy widened")
				}
			}
		}
	}
}
