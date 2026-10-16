package main

import (
	"bytes"
	"testing"

	"github.com/minio/pkg/v3/policy"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

// Exercise the OSS policy evaluator used by the pinned native server, rather
// than accepting a structurally valid policy which cannot authorize its route.
// MinIO 9e49d5e7a648 auth-handler.go first checks version-delete denies, then
// requires DeleteObject. This is distinct from merely allowing VersionAction.
func TestReferenceIAMAuthorizesPinnedNativeRouteAndRejectsOtherScope(t *testing.T) {
	p, err := policy.ParseConfig(bytes.NewReader(referenceIAMPolicy()))
	if err != nil {
		t.Fatal(err)
	}
	for _, copy := range pilot.ReferenceCopies() {
		allowed := false
		for _, target := range pilot.ReferenceTargets() {
			allowed = allowed || target == copy
		}
		for _, action := range []policy.Action{policy.DeleteObjectAction, policy.DeleteObjectVersionAction} {
			args := policy.Args{Action: action, BucketName: copy.Namespace, ObjectName: copy.Key, ConditionValues: map[string][]string{"versionid": {copy.ProviderVersion}}}
			if got := p.IsAllowed(args); got != allowed {
				t.Fatalf("native action %s for %s/%s@%s: got %v, want %v", action, copy.Namespace, copy.Key, copy.ProviderVersion, got, allowed)
			}
			args.ConditionValues = nil
			if p.IsAllowed(args) {
				t.Fatal("bare-key deletion permitted")
			}
			args.ConditionValues = map[string][]string{"versionid": {"unconfirmed-version"}}
			if p.IsAllowed(args) {
				t.Fatal("other version permitted")
			}
		}
	}
	for _, target := range pilot.ReferenceTargets() {
		args := policy.Args{Action: policy.DeleteObjectVersionAction, BucketName: target.Namespace, ObjectName: target.Key, ConditionValues: map[string][]string{"versionid": {target.ProviderVersion}}, DenyOnly: true}
		if !p.IsAllowed(args) {
			t.Fatal("pinned server version-delete deny check rejects exact target")
		}
		for _, action := range []policy.Action{policy.PutObjectAction, policy.GetObjectAction, policy.BypassGovernanceRetentionAction, policy.PutObjectLegalHoldAction} {
			args.Action, args.DenyOnly = action, false
			if p.IsAllowed(args) {
				t.Fatalf("unapproved capability %s permitted", action)
			}
		}
	}
}
