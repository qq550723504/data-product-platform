package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestRestoreInspectorSDKStatusOnlyCannotEnableSenderOrProvision(t *testing.T) {
	user := pilot.RemainingIdentity("mixed", "inspect")
	calls := 0
	tr := &sixProvisionTransport{c: config{OutputDirectory: t.TempDir()}, users: map[string]string{user: pilot.RemainingPolicyName("mixed", "inspect")}, mutations: map[string]bool{}, restoreInspector: user, restoreStatus: madmin.AccountEnabled}
	tr.base = postRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	admin, e := madmin.NewWithOptions("127.0.0.1:9000", &madmin.Options{Creds: credentials.NewStaticV4("root", "test-only", ""), Transport: tr})
	if e != nil {
		t.Fatal(e)
	}
	if e = admin.SetUserStatus(context.Background(), user, madmin.AccountEnabled); e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{
		"/minio/admin/v3/set-user-status?accessKey=" + pilot.RemainingIdentity("mixed", "delete") + "&status=enabled",
		"/minio/admin/v3/set-user-status?accessKey=" + user + "&status=disabled",
		"/minio/admin/v3/add-user?accessKey=" + user,
		"/minio/admin/v3/set-user-status?accessKey=" + user + "&status=enabled", // replay
	} {
		r, _ := http.NewRequest("PUT", "http://127.0.0.1:9000"+path, strings.NewReader(""))
		if _, e = tr.RoundTrip(r); e == nil {
			t.Fatal("expanded/replayed restore operation admitted")
		}
	}
	if calls != 1 {
		t.Fatal("invalid operation reached upstream")
	}
	// Role SOURCE is rejected before any admin client or provider request.
	mapping, m := sixMap(t)
	p, _, _ := postProfile(t, "mixed")
	p.Purpose = sixRestoreInspectorPurpose
	p.CandidateSHA256 = postV22Hash(mapping)
	s := sixReadProfile{postV22ReadProfile: p, Execution: m.Execution, Profile: m.Profile, Role: "SOURCE", MapSHA256: postV22Hash(mapping)}
	raw, _ := json.Marshal(s)
	if _, e = parseSixRestoreInspector(raw, mapping, time.Now().UTC()); e == nil {
		t.Fatal("SOURCE inspector enable admitted")
	}
	row := m.Groups[2]
	s.Role = "RESTORE"
	s.Core, s.Native, s.Network = row.Restore[0], row.Restore[1], row.Restore[2]
	raw, _ = json.Marshal(s)
	if _, e = parseSixRestoreInspector(raw, mapping, time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
}
