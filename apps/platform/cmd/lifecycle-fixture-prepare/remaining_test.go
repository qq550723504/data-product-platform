package main

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/minio/pkg/v3/policy"
)

func TestRemainingFixedMembershipAndNativeIAM(t *testing.T) {
	total, sends := 0, 0
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		layout, e := remainingLayout(group)
		if e != nil {
			t.Fatal(e)
		}
		_, versions := remainingVersions(layout)
		objects := make([]object, len(layout))
		seen := map[string]bool{}
		for i, s := range layout {
			if seen[s.Namespace+s.Key] {
				t.Fatal("duplicate key")
			}
			seen[s.Namespace+s.Key] = true
			objects[i] = object{Namespace: s.Namespace, Key: s.Key, Version: "native-version-" + s.Content, DatasetVersionID: versions[s.Content]}
			total++
			if s.FutureSend {
				sends++
			}
		}
		if group == "mixed" {
			if objects[0].DatasetVersionID != objects[1].DatasetVersionID || objects[1].DatasetVersionID != objects[2].DatasetVersionID || !bytes.Equal(remainingPayload(group, layout[0].Content), remainingPayload(group, layout[2].Content)) || !layout[2].Retained || layout[2].FutureSend {
				t.Fatal("mixed membership/protection drift")
			}
		}
		raw, e := remainingPolicy("disabled", objects, layout)
		if e != nil {
			t.Fatal(e)
		}
		p, e := policy.ParseConfig(bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		for i, o := range objects {
			for _, action := range []policy.Action{policy.DeleteObjectAction, policy.DeleteObjectVersionAction} {
				args := policy.Args{Action: action, BucketName: o.Namespace, ObjectName: o.Key, ConditionValues: map[string][]string{"versionid": {o.Version}}}
				if p.IsAllowed(args) != layout[i].FutureSend {
					t.Fatal("exact native action scope drift", group, o.Key, action)
				}
				args.ConditionValues = nil
				if p.IsAllowed(args) {
					t.Fatal("bare delete allowed")
				}
				args.ConditionValues = map[string][]string{"versionid": {"other-version"}}
				if p.IsAllowed(args) {
					t.Fatal("other version allowed")
				}
				for _, other := range []policy.Action{policy.GetObjectAction, policy.PutObjectAction, policy.BypassGovernanceRetentionAction, policy.PutObjectLegalHoldAction} {
					args.Action = other
					if p.IsAllowed(args) {
						t.Fatal("foreign capability allowed")
					}
				}
			}
		}
	}
	if total != 16 || sends != 7 {
		t.Fatal("finite budget/membership drift", total, sends)
	}
	if _, e := remainingLayout("old-v15"); e == nil {
		t.Fatal("unknown profile admitted")
	}
}

func TestRemainingRejectsLegacyModesBeforeIO(t *testing.T) {
	c := fixtureConfig(t)
	c.RemainingGroup = "mixed"
	c.ArchiveAcceptance = true
	c.ProcessingDeliveryAcceptance = true
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, c)
	for _, key := range []string{"LIFECYCLE_FIXTURE_INSPECT_FAILED", "LIFECYCLE_FIXTURE_PREPARE_UNPROTECTED_PLAN", "LIFECYCLE_FIXTURE_INSPECT_REGISTERED", "LIFECYCLE_FIXTURE_RESUME_EMPTY"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "1")
			if e := prepare(context.Background(), path); e == nil || e.Error() != "remaining profile refuses legacy modes" {
				t.Fatal(e)
			}
		})
	}
	c.RemainingGroup = "foreign"
	writeConfig(t, path, c)
	if e := prepare(context.Background(), path); e == nil || e.Error() != "invalid remaining profile" {
		t.Fatal(e)
	}
	for _, method := range []string{"DELETE", "PUT", "POST", "PATCH"} {
		r, _ := http.NewRequest(method, "http://127.0.0.1:9000/", nil)
		if _, e := (remainingReadTransport{}).RoundTrip(r); e == nil {
			t.Fatal("readonly mutation admitted")
		}
	}
}
