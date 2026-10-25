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
