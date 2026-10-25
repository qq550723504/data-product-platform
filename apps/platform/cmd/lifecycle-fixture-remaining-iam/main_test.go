package main

import (
	madmin "github.com/minio/madmin-go/v3"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"net/http"
	"testing"
)

type shouldNotCall struct{ t *testing.T }

func (s shouldNotCall) RoundTrip(*http.Request) (*http.Response, error) {
	s.t.Fatal("forbidden bootstrap operation reached transport")
	return nil, nil
}
func TestBootstrapNeverExposesObjectDeletionOrUnconfirmedEnable(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"DELETE", "/dpp-lifecycle307-online/fixture/p1.csv?versionId=exact"},
		{"PUT", "/minio/admin/v3/set-user-status?accessKey=foreign&status=disabled"},
		{"PUT", "/minio/admin/v3/set-user-status?accessKey=delete808a6960&status=enabled"},
		{"PUT", "/minio/admin/v3/set-user-status?accessKey=iafc0e66c808a6960&status=enabled"},
		{"PUT", "/minio/admin/v3/set-user-status?accessKey=dafc0e66c808a6960&status=enabled"},
		{"PUT", "/minio/admin/v3/set-user-status?accessKey=i6f40bca9808a6960&status=enabled"},
		{"PUT", "/minio/admin/v3/set-user-status?accessKey=d6f40bca9808a6960&status=enabled"},
		{"PUT", "/minio/admin/v3/add-user"}, {"GET", "/dpp-lifecycle307-online/fixture/p1.csv?versionId=exact"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			req, _ := http.NewRequest(tc.method, "http://127.0.0.1:9000"+tc.path, nil)
			if _, e := (transport{base: shouldNotCall{t}, users: map[string]string{"delete808a6960": "fixed", "iafc0e66c808a6960": "retained-inspect", "dafc0e66c808a6960": "retained-delete", "i6f40bca9808a6960": "retired-inspect", "d6f40bca9808a6960": "retired-delete"}, enable: true, group: "business-races"}).RoundTrip(req); e == nil {
				t.Fatal("forbidden request admitted")
			}
		})
	}
}

func TestBootstrapSelectsOnlyCurrentExactIdentityAndDormantIsNeverEnabled(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed", "foreign"} {
		for _, inspectOnly := range []bool{false, true} {
			tr := transport{group: group, inspectOnly: inspectOnly}
			inspector, deletion := pilot.RemainingIdentity(group, "inspect"), pilot.RemainingIdentity(group, "delete")
			for _, user := range []string{inspector, deletion, "i4e524c04808a6960", "inspect808a69606c36", "delete808a6960", "remainread808a6960", inspector + "extra", ""} {
				want := user != "" && (user == inspector || (!inspectOnly && user == deletion))
				if tr.selected(user) != want {
					t.Fatal(group, inspectOnly, user)
				}
			}
			for user := range pilot.RemainingRetainedIdentities(group) {
				if tr.selected(user) {
					t.Fatal("retired execution identity selected", group, inspectOnly, user)
				}
			}
		}
	}
	dormant, _ := pilot.RemainingDormantIdentity("business-races")
	if !exactAssociation("business-races", dormant, "", madmin.UserInfo{Status: madmin.AccountDisabled}) {
		t.Fatal("exact retained partial rejected")
	}
	for _, info := range []madmin.UserInfo{{Status: madmin.AccountEnabled}, {Status: madmin.AccountDisabled, PolicyName: "policy"}, {Status: madmin.AccountDisabled, PolicyName: " "}, {Status: madmin.AccountDisabled, MemberOf: []string{"admins"}}} {
		if exactAssociation("business-races", dormant, "", info) {
			t.Fatal("expanded dormant association admitted")
		}
	}
	if exactAssociation("late-hold", dormant, "", madmin.UserInfo{Status: madmin.AccountDisabled}) || exactAssociation("business-races", "other", "", madmin.UserInfo{Status: madmin.AccountDisabled}) {
		t.Fatal("dormant exception leaked")
	}
}
