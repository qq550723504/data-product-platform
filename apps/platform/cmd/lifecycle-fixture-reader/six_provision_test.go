package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func TestSixDisabledProvisionUsesRealSDKAndRejectsEnableExpansionReplay(t *testing.T) {
	_, _, present := postProfile(t, "business-races")
	var expected []remainingExpected
	_ = json.Unmarshal(present, &expected)
	cp := []domain.Copy{}
	for _, row := range expected {
		cp = append(cp, row.Copy)
	}
	users, policies, err := pilot.RemainingPolicies("business-races", cp)
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.Repeat("test-only-", 8)
	requests := 0
	transport := &sixProvisionTransport{c: config{RootSecret: secret, OutputDirectory: t.TempDir()}, users: users, policies: policies, mutations: map[string]bool{}}
	transport.base = postRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.Path == "/minio/admin/v3/add-user" {
			raw, e := madmin.DecryptData(secret, req.Body)
			if e != nil {
				t.Fatal(e)
			}
			var user madmin.AddOrUpdateUserReq
			_ = json.Unmarshal(raw, &user)
			if user.Status != madmin.AccountDisabled {
				t.Fatal("real SDK enabled an identity")
			}
		}
		var body []byte
		if req.URL.Path == "/minio/admin/v3/import-iam-v2" {
			raw, _ := io.ReadAll(req.Body)
			user, name, e := sixMappingScope(raw, users)
			if e != nil {
				t.Fatal(e)
			}
			body = []byte(`{"skipped":{},"removed":{},"added":{"userPolicies":[{"` + user + `":["` + name + `"]}]},"failed":{}}`)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	admin, err := madmin.NewWithOptions("127.0.0.1:9000", &madmin.Options{Creds: credentials.NewStaticV4("test-root", secret, ""), Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, kind := range []string{"inspect", "delete"} {
		user, name := pilot.RemainingIdentity("business-races", kind), pilot.RemainingPolicyName("business-races", kind)
		if err = admin.SetUser(ctx, user, strings.Repeat("independent-", 4), madmin.AccountDisabled); err != nil {
			t.Fatal(err)
		}
		if err = admin.AddCannedPolicy(ctx, name, policies[name]); err != nil {
			t.Fatal(err)
		}
		mapping, _ := sixMappingZIP(user, name)
		if _, err = admin.ImportIAMV2(ctx, io.NopCloser(bytes.NewReader(mapping))); err != nil {
			t.Fatal(err)
		}
	}
	if requests != 6 {
		t.Fatalf("expected six bounded simulated mutations: %d", requests)
	}
	user := pilot.RemainingIdentity("business-races", "delete")
	rejectEncrypted := func(method, path string, value any) {
		plain, _ := json.Marshal(value)
		encrypted, e := madmin.EncryptData(secret, plain)
		if e != nil {
			t.Fatal(e)
		}
		request, _ := http.NewRequest(method, "http://127.0.0.1:9000"+path, bytes.NewReader(encrypted))
		if _, e = transport.RoundTrip(request); e == nil {
			t.Fatal("invalid enable/replay/association admitted")
		}
	}
	rejectEncrypted("PUT", "/minio/admin/v3/add-user?accessKey="+user, madmin.AddOrUpdateUserReq{SecretKey: strings.Repeat("test-", 8), Status: madmin.AccountEnabled})
	rejectEncrypted("PUT", "/minio/admin/v3/add-user?accessKey="+user, madmin.AddOrUpdateUserReq{SecretKey: strings.Repeat("test-", 8), Status: madmin.AccountDisabled})
	rejectEncrypted("POST", "/minio/admin/v3/idp/builtin/policy/attach", madmin.PolicyAssociationReq{Policies: []string{pilot.RemainingPolicyName("business-races", "inspect"), pilot.RemainingPolicyName("business-races", "delete")}, User: user})
	request, _ := http.NewRequest("DELETE", "http://127.0.0.1:9000/forbidden?versionId=test", nil)
	if _, err = transport.RoundTrip(request); err == nil {
		t.Fatal("provider DELETE admitted")
	}
	if requests != 6 {
		t.Fatal("rejected action reached simulated upstream")
	}
}
