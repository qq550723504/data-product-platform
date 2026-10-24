package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func mappingResult(user, name string) []byte {
	return []byte(`{"skipped":{},"removed":{},"added":{"userPolicies":[{"` + user + `":["` + name + `"]}]},"failed":{}}`)
}

func zipMembers(t *testing.T, names []string, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range names {
		f, e := w.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func TestSixMappingRejectsArchiveAndAssociationExpansion(t *testing.T) {
	user, name := "fixed-user", "fixed-policy"
	users := map[string]string{user: name}
	valid, _ := sixMappingZIP(user, name)
	if u, n, e := sixMappingScope(valid, users); e != nil || u != user || n != name {
		t.Fatal(u, n, e)
	}
	for label, raw := range map[string][]byte{
		"empty": nil, "not-zip": []byte(`{}`), "too-large": make([]byte, 2049),
		"other-asset":      zipMembers(t, []string{"iam-assets/users.json"}, sixMappingJSON(user, name)),
		"duplicate-member": zipMembers(t, []string{sixMappingMember, sixMappingMember}, sixMappingJSON(user, name)),
		"extra-asset":      zipMembers(t, []string{sixMappingMember, "iam-assets/groups.json"}, sixMappingJSON(user, name)),
		"traversal":        zipMembers(t, []string{"../" + sixMappingMember}, sixMappingJSON(user, name)),
		"foreign-user":     zipMembers(t, []string{sixMappingMember}, sixMappingJSON("other", name)),
		"foreign-policy":   zipMembers(t, []string{sixMappingMember}, sixMappingJSON(user, "admin")),
		"two-policies":     zipMembers(t, []string{sixMappingMember}, sixMappingJSON(user, name+",admin")),
		"two-users":        zipMembers(t, []string{sixMappingMember}, []byte(`{"fixed-user":{"version":1,"policy":"fixed-policy"},"other":{"version":1,"policy":"admin"}}`)),
		"duplicate-json":   zipMembers(t, []string{sixMappingMember}, []byte(`{"fixed-user":{"version":1,"policy":"admin","policy":"fixed-policy"}}`)),
		"suffix":           append(append([]byte{}, valid...), 0), "prefix": append([]byte{0}, valid...),
	} {
		t.Run(label, func(t *testing.T) {
			if _, _, e := sixMappingScope(raw, users); e == nil {
				t.Fatal("expanded IAM archive admitted")
			}
		})
	}
}

func TestSixImportResponseRequiresExactConfirmedOutcome(t *testing.T) {
	good := mappingResult("fixed-user", "fixed-policy")
	if e := sixImportOutcome(good, "fixed-user", "fixed-policy"); e != nil {
		t.Fatal(e)
	}
	// Provider object ordering/formatting is semantic; request ZIP bytes are ours.
	formatted := []byte(`{ "failed": {}, "added": {"userPolicies":[{"fixed-user":["fixed-policy"]}]}, "removed":{}, "skipped":{} }`)
	if e := sixImportOutcome(formatted, "fixed-user", "fixed-policy"); e != nil {
		t.Fatal(e)
	}
	for name, raw := range map[string][]byte{
		"empty": nil, "empty-success": []byte(`{}`), "oversize": make([]byte, 65537),
		"wrong-user": mappingResult("other", "fixed-policy"), "wrong-policy": mappingResult("fixed-user", "admin"),
		"duplicate-top":   bytes.Replace(good, []byte(`"skipped":{}`), []byte(`"skipped":{},"skipped":{}`), 1),
		"unknown-top":     bytes.Replace(good, []byte(`"skipped":{}`), []byte(`"extra":{},"skipped":{}`), 1),
		"unknown-nested":  bytes.Replace(good, []byte(`"skipped":{}`), []byte(`"skipped":{"extra":[]}`), 1),
		"skipped-user":    bytes.Replace(good, []byte(`"skipped":{}`), []byte(`"skipped":{"users":["other"]}`), 1),
		"removed-user":    bytes.Replace(good, []byte(`"removed":{}`), []byte(`"removed":{"users":["other"]}`), 1),
		"failed-user":     bytes.Replace(good, []byte(`"failed":{}`), []byte(`"failed":{"users":[{"user":"other"}]}`), 1),
		"unrelated-added": bytes.Replace(good, []byte(`"added":{`), []byte(`"added":{"users":["other"],`), 1),
		"null-category":   bytes.Replace(good, []byte(`"failed":{}`), []byte(`"failed":null`), 1),
		"two-policies":    bytes.Replace(good, []byte(`["fixed-policy"]`), []byte(`["fixed-policy","admin"]`), 1),
		"duplicate-user":  bytes.Replace(good, []byte(`{"fixed-user":["fixed-policy"]}`), []byte(`{"fixed-user":["admin"],"fixed-user":["fixed-policy"]}`), 1),
		"trailing":        append(append([]byte{}, good...), []byte(` {}`)...),
	} {
		t.Run(name, func(t *testing.T) {
			if sixImportOutcome(raw, "fixed-user", "fixed-policy") == nil {
				t.Fatal("unconfirmed import admitted")
			}
		})
	}
}

func TestSixImportUnknownAndBadResponseNeverReplay(t *testing.T) {
	for _, kind := range []string{"unknown", "malformed", "non-200"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			user, name := "fixed-user", "fixed-policy"
			tr := &sixProvisionTransport{c: config{OutputDirectory: t.TempDir()}, users: map[string]string{user: name}, mutations: map[string]bool{}}
			tr.base = postRoundTripper(func(*http.Request) (*http.Response, error) {
				calls++
				if kind == "unknown" {
					return nil, errors.New("unknown transport")
				}
				status := 200
				if kind == "non-200" {
					status = 500
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
			})
			raw, _ := sixMappingZIP(user, name)
			request := func() *http.Request {
				r, _ := http.NewRequest("PUT", "http://127.0.0.1:9000/minio/admin/v3/import-iam-v2", bytes.NewReader(raw))
				return r
			}
			response, err := tr.RoundTrip(request())
			if response != nil {
				response.Body.Close()
			}
			if kind != "non-200" && err == nil {
				t.Fatal("unconfirmed response accepted")
			}
			if _, err = tr.RoundTrip(request()); err == nil {
				t.Fatal("same mutation replay admitted")
			}
			r := request()
			r.URL.RawQuery = "extra=1"
			if _, err = tr.RoundTrip(r); err == nil {
				t.Fatal("query expansion admitted")
			}
			if calls != 1 {
				t.Fatal("upstream replay", calls)
			}
		})
	}
}

// Opt-in adapter contract uses only a newly allocated, network-isolated same-image
// MinIO. The caller supplies fresh scratch root credentials; no source config.
func TestSixNativeDisabledBootstrapContract(t *testing.T) {
	path := os.Getenv("DPP307_SIX_NATIVE_CONTRACT")
	if path == "" {
		t.Skip("isolated native contract not configured")
	}
	var cfg struct{ RootAccess, RootSecret, Output string }
	raw, err := os.ReadFile(path)
	if err != nil || strictPostV22(raw, &cfg) != nil || len(cfg.RootSecret) < 32 {
		t.Fatal("fresh scratch contract config required")
	}
	_, _, inventory := postProfile(t, "business-races")
	var expected []remainingExpected
	if err = json.Unmarshal(inventory, &expected); err != nil {
		t.Fatal(err)
	}
	copies := []domain.Copy{}
	for _, row := range expected {
		copies = append(copies, row.Copy)
	}
	users, policies, err := pilot.RemainingPolicies("business-races", copies)
	if err != nil {
		t.Fatal(err)
	}
	c := config{RootAccess: cfg.RootAccess, RootSecret: cfg.RootSecret, OutputDirectory: cfg.Output}
	tr := &sixProvisionTransport{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, c: c, users: users, policies: policies, mutations: map[string]bool{}}
	admin, err := madmin.NewWithOptions("127.0.0.1:9000", &madmin.Options{Creds: credentials.NewStaticV4(cfg.RootAccess, cfg.RootSecret, ""), Transport: tr})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, kind := range []string{"inspect", "delete"} {
		user, name := pilot.RemainingIdentity("business-races", kind), pilot.RemainingPolicyName("business-races", kind)
		if err = sixCreateDisabled(ctx, admin, user, name, strings.Repeat("scratch-independent-", 4), policies[name]); err != nil {
			t.Fatal(err)
		}
		mapping, _ := sixMappingZIP(user, name)
		if _, err = admin.ImportIAMV2(ctx, io.NopCloser(bytes.NewReader(mapping))); err == nil {
			t.Fatal("replay admitted")
		}
	}
	if err = write(c, "native-contract-result.json", map[string]any{"status": "PASS_PRODUCTION_DISABLED_BOOTSTRAP_ADAPTER", "execution": pilot.RemainingExecution, "users": users, "enabled": 0, "object_put": 0, "native_delete": 0, "source_calls": 0}); err != nil {
		t.Fatal(err)
	}
	if files, _ := filepath.Glob(filepath.Join(cfg.Output, "prepare-request-*.zip")); len(files) != 2 {
		t.Fatal("expected two exact mutation archives", len(files))
	}
}

func TestSixUncreatedUsersRequireExactSDKAbsenceAndJournal(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		status     int
		ok         bool
	}{{"absent", "XMinioAdminNoSuchUser", 404, true}, {"exists", "", 200, false}, {"unknown", "AccessDenied", 404, false}, {"wrong-http", "XMinioAdminNoSuchUser", 403, false}} {
		t.Run(tc.name, func(t *testing.T) {
			folder := t.TempDir()
			c := config{Endpoint: "127.0.0.1:9000", RootAccess: "test-root", RootSecret: strings.Repeat("test-only", 8), OutputDirectory: folder}
			tr := &postV22VerifyTransport{c: c, base: postRoundTripper(func(*http.Request) (*http.Response, error) {
				body := `{"status":"disabled"}`
				if tc.status != 200 {
					b, _ := json.Marshal(madmin.ErrorResponse{Code: tc.code, Message: "synthetic exact contract"})
					body = string(b)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
			})}
			err := sixUncreatedUsers(context.Background(), c, "business-races", tr)
			if (err == nil) != tc.ok {
				t.Fatal(tc.name, err)
			}
			_, statErr := os.Stat(filepath.Join(folder, "six-uncreated-users.json"))
			if tc.ok && statErr != nil {
				t.Fatal(statErr)
			}
			if !tc.ok && statErr == nil {
				t.Fatal("failed absence wrote success proof")
			}
		})
	}
}
