package main

import (
	"bytes"
	"testing"

	"github.com/minio/pkg/v3/policy"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestRemainingReaderProducerAreSeparateAndVersionBound(t *testing.T) {
	scope, err := pilot.RemainingScopeFor("business-races")
	if err != nil {
		t.Fatal(err)
	}
	r, w, err := remainingPolicies(scope.Copies)
	if err != nil {
		t.Fatal(err)
	}
	read, err := policy.ParseConfig(bytes.NewReader(r))
	if err != nil {
		t.Fatal(err)
	}
	put, err := policy.ParseConfig(bytes.NewReader(w))
	if err != nil {
		t.Fatal(err)
	}
	for i, cp := range scope.Copies {
		args := policy.Args{Action: policy.GetObjectVersionAction, BucketName: cp.Namespace, ObjectName: cp.Key, ConditionValues: map[string][]string{"versionid": {cp.ProviderVersion}}}
		if read.IsAllowed(args) != (i < 4) {
			t.Fatal("reader scope differs", cp.Key)
		}
		if put.IsAllowed(args) {
			t.Fatal("producer can read")
		}
		args.ConditionValues = map[string][]string{"versionid": {"foreign"}}
		if read.IsAllowed(args) {
			t.Fatal("foreign version read")
		}
		for _, action := range []policy.Action{policy.GetObjectAction, policy.DeleteObjectAction, policy.DeleteObjectVersionAction, policy.PutObjectAction, policy.BypassGovernanceRetentionAction, policy.PutObjectLegalHoldAction} {
			args.Action = action
			if read.IsAllowed(args) || put.IsAllowed(args) {
				t.Fatal("source/admin capability allowed", action)
			}
		}
	}
	for _, key := range []string{"fixture/remaining-standardized.csv", "fixture/remaining-processed.csv"} {
		args := policy.Args{Action: policy.PutObjectAction, BucketName: namespaces[0], ObjectName: key}
		if !put.IsAllowed(args) || read.IsAllowed(args) {
			t.Fatal("producer output missing")
		}
	}
}
