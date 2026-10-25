package main

import (
	madmin "github.com/minio/madmin-go/v3"
	"testing"
)

func TestNativeVerifierRejectsExtraDirectAndInheritedPolicies(t *testing.T) {
	for _, tc := range []struct {
		policy string
		groups []string
		want   bool
	}{
		{"exact", nil, true}, {"exact,exact", nil, true}, {"exact,admin", nil, false},
		{"wrong", nil, false}, {"", nil, false}, {"exact,", nil, false},
		{"exact", []string{"administrators"}, false},
	} {
		if got := remainingExactUserPolicy(madmin.UserInfo{PolicyName: tc.policy, MemberOf: tc.groups}, "exact"); got != tc.want {
			t.Fatalf("policy=%q groups=%v accepted=%v", tc.policy, tc.groups, got)
		}
	}
}

func TestAuditAssociationAllowsOnlyExactRetainedPartialWithoutPolicy(t *testing.T) {
	const dormant = "i4e524c04808a6960"
	off := madmin.UserInfo{Status: madmin.AccountDisabled}
	if !remainingAuditAssociation("business-races", dormant, "", off) {
		t.Fatal("retained actual partial rejected")
	}
	for _, tc := range []struct {
		group, user, want string
		info              madmin.UserInfo
	}{
		{"late-hold", dormant, "", off}, {"business-races", "other", "", off},
		{"business-races", dormant, "", madmin.UserInfo{Status: madmin.AccountEnabled}},
		{"business-races", dormant, "", madmin.UserInfo{Status: madmin.AccountDisabled, PolicyName: "policy"}},
		{"business-races", dormant, "", madmin.UserInfo{Status: madmin.AccountDisabled, MemberOf: []string{"admin"}}},
	} {
		if remainingAuditAssociation(tc.group, tc.user, tc.want, tc.info) {
			t.Fatal("partial exception expanded", tc)
		}
	}
}
