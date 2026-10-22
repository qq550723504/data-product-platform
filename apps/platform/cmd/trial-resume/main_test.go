package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testConfig(t *testing.T, endpoint string) (config, string) {
	t.Helper()
	c := config{Private: filepath.Join(t.TempDir(), "dpp-trial-retained"), Workspace: uuid.NewString(), RootKey: "synthetic-parent", RootSecret: "synthetic-secret"}
	c.Profile.StorageEndpoint = strings.TrimPrefix(endpoint, "http://")
	c.Profile.Bucket = "raw"
	dir := filepath.Join(c.Private, "resume", uuid.NewString())
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return c, filepath.Join(dir, "core-identity.json")
}

func response(w http.ResponseWriter, expiry time.Time) {
	w.Header().Set("Content-Type", "application/xml")
	fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><AssumeRoleResult><Credentials><AccessKeyId>synthetic-access</AccessKeyId><SecretAccessKey>synthetic-secret</SecretAccessKey><SessionToken>synthetic-session</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleResult></AssumeRoleResponse>`, expiry.UTC().Format(time.RFC3339))
}

func TestNativeSDKRefreshKeepsWorkspacePolicyAndDoesNotReplay(t *testing.T) {
	calls := 0
	var gotPolicy map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.Header.Get("Authorization") == "" {
			t.Error("native signed STS POST required")
		}
		if r.ParseForm() != nil || r.Form.Get("Action") != "AssumeRole" || r.Form.Get("DurationSeconds") != "3600" {
			t.Error("finite native STS request required")
		}
		if json.Unmarshal([]byte(r.Form.Get("Policy")), &gotPolicy) != nil {
			t.Error("policy required")
		}
		response(w, time.Now().Add(time.Hour))
	}))
	defer server.Close()
	c, output := testConfig(t, server.URL)
	result, err := renew(c, output)
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	b, _ := policy(c)
	_ = json.Unmarshal(b, &expected)
	actual, _ := json.Marshal(gotPolicy)
	want, _ := json.Marshal(expected)
	if string(actual) != string(want) || !strings.Contains(string(actual), "staging/"+c.Workspace+"/*") || !strings.Contains(string(actual), "core/"+c.Workspace+"/*") {
		t.Fatal("workspace policy expanded or changed")
	}
	before, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = renew(c, output); err == nil {
		t.Fatal("same output must not reissue")
	}
	after, _ := os.ReadFile(output)
	if string(before) != string(after) || calls != 1 {
		t.Fatal("credential overwritten or invocation replayed")
	}
	public, _ := json.Marshal(result)
	if strings.Contains(string(public), "synthetic-") {
		t.Fatal("secret in public receipt")
	}
}

func TestInvalidContextOrOutputMakesNoInvocation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; response(w, time.Now().Add(time.Hour)) }))
	defer server.Close()
	for _, kind := range []string{"workspace", "endpoint", "bucket", "output"} {
		t.Run(kind, func(t *testing.T) {
			c, out := testConfig(t, server.URL)
			switch kind {
			case "workspace":
				c.Workspace = uuid.Nil.String()
			case "endpoint":
				c.Profile.StorageEndpoint = "example.com:9000"
			case "bucket":
				c.Profile.Bucket = "other"
			case "output":
				out = filepath.Join(t.TempDir(), "core-identity.json")
			}
			if _, err := renew(c, out); err == nil {
				t.Fatal("invalid context accepted")
			}
		})
	}
	if calls != 0 {
		t.Fatal("invalid request reached provider")
	}
}

func TestExpiredCapabilityAndRedirectFailClosed(t *testing.T) {
	for _, kind := range []string{"expired", "redirect"} {
		t.Run(kind, func(t *testing.T) {
			followed := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed++ }))
			defer target.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "redirect" {
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				} else {
					response(w, time.Now().Add(-time.Minute))
				}
			}))
			defer server.Close()
			c, out := testConfig(t, server.URL)
			if _, err := renew(c, out); err == nil {
				t.Fatal("unsafe STS result accepted")
			}
			if followed != 0 {
				t.Fatal("redirect followed")
			}
			b, _ := os.ReadFile(out)
			if len(b) != 0 {
				t.Fatal("unsafe credential persisted")
			}
		})
	}
}

func TestScopedIdentityReadsExactRetainedBytesAndRejectsForeignReceipt(t *testing.T) {
	data := []byte("synthetic retained RAW\n")
	h := sha256.Sum256(data)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Amz-Security-Token") != "synthetic-session" {
			t.Error("fresh session required")
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("ETag", `"synthetic-etag"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		if r.Method != "HEAD" {
			_, _ = w.Write(data)
		}
	}))
	defer server.Close()
	c, out := testConfig(t, server.URL)
	identity := []byte(`{"AccessKey":"synthetic-access","SecretKey":"synthetic-secret","SessionToken":"synthetic-session"}`)
	if err := os.WriteFile(out, identity, 0600); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{"workspaceId": c.Workspace, "objects": []any{map[string]any{"uri": "s3://raw/core/" + c.Workspace + "/retained.csv", "bytes": len(data), "sha256": hex.EncodeToString(h[:])}}}
	file := filepath.Join(t.TempDir(), "receipt.json")
	writeReceipt := func() {
		b, _ := json.Marshal(receipt)
		if err := os.WriteFile(file, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeReceipt()
	if _, err := verifyCore(c, out, file); err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("no native read")
	}
	before := calls
	receipt["workspaceId"] = uuid.NewString()
	writeReceipt()
	if _, err := verifyCore(c, out, file); err == nil {
		t.Fatal("foreign workspace accepted")
	}
	receipt["workspaceId"] = c.Workspace
	receipt["objects"] = []any{map[string]any{"uri": "s3://raw/core/" + uuid.NewString() + "/other.csv", "bytes": len(data), "sha256": hex.EncodeToString(h[:])}}
	writeReceipt()
	if _, err := verifyCore(c, out, file); err == nil {
		t.Fatal("foreign resource accepted")
	}
	if calls != before {
		t.Fatal("foreign receipt made native call")
	}
}
