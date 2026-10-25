package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/minio-go/v7/pkg/signer"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

const nativePermissionCheck = "X-Minio-Source-Replication-Check"

type nativeAuditTransport struct {
	base   http.RoundTripper
	output string
}

func (a *nativeAuditTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	// No ordinary native DELETE, including SDK cleanup paths, can leave this
	// test process. Evidence start precedes every actual synthetic native I/O.
	if r.Method == "DELETE" && r.Header.Get(nativePermissionCheck) != "true" {
		return nil, errors.New("ordinary DELETE is forbidden")
	}
	id := uuid.NewString()
	start := time.Now().UTC()
	if e := save(filepath.Join(a.output, "request-start-"+id+".json"), map[string]any{"id": id, "method": r.Method, "path": r.URL.Path, "at": start, "native_permission_check": r.Header.Get(nativePermissionCheck) == "true"}); e != nil {
		return nil, e
	}
	response, err := a.base.RoundTrip(r)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	if e := save(filepath.Join(a.output, "request-observation-"+id+".json"), map[string]any{"id": id, "at": time.Now().UTC(), "status": status, "transport_error": err != nil, "invocation_quantity": 1, "unit": "synthetic_native_http_request", "amount": "NOT_ASSESSED"}); e != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, e
	}
	return response, err
}

// This test ONLY runs against a new isolated pinned native fixture. Every
// DELETE is the native replication permission check, which returns BEFORE
// delOpts/DeleteObject. No ordinary DELETE is ever a positive control.
type permissionProbeTransport struct {
	base  http.RoundTripper
	calls int
}

func (p *permissionProbeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodDelete || r.Header.Get(nativePermissionCheck) != "true" || !strings.Contains(r.Header.Get("Authorization"), "SignedHeaders=") || !strings.Contains(r.Header.Get("Authorization"), strings.ToLower(nativePermissionCheck)) {
		return nil, errors.New("non-mutating signed native check required")
	}
	if r.URL.Scheme != "http" || !strings.HasPrefix(r.URL.Host, "127.0.0.1:") {
		return nil, errors.New("isolated loopback only")
	}
	p.calls++
	return p.base.RoundTrip(r)
}

func TestNativeExactDeletePermissionWithoutObjectDeletion(t *testing.T) {
	path := os.Getenv("TEST_LIFECYCLE_PERMISSION_CONFIG")
	if path == "" {
		t.Skip("isolated native fixture is not configured")
	}
	var cfg struct {
		Endpoint, AccessKey, SecretKey, Output string
		Copies                                 map[string][]domain.Copy
	}
	raw, e := os.ReadFile(path)
	if e != nil || json.Unmarshal(raw, &cfg) != nil || cfg.Endpoint != "127.0.0.1:9000" {
		t.Fatal("private native config")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	base := &nativeAuditTransport{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 5 * time.Second}, output: cfg.Output}
	root, e := minio.New(cfg.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Region: "us-east-1", MaxRetries: 1, Transport: base})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 40; i++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+cfg.Endpoint+"/minio/health/ready", nil)
		r, err := base.RoundTrip(req)
		if r != nil {
			r.Body.Close()
		}
		if err == nil && r.StatusCode == 200 {
			break
		}
		if i == 39 {
			t.Fatal("native not ready")
		}
		time.Sleep(250 * time.Millisecond)
	}
	admin, e := madmin.NewWithOptions(cfg.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""), Transport: base})
	if e != nil {
		t.Fatal(e)
	}
	madmin.MaxRetry = 1
	probe := &permissionProbeTransport{base: base}
	var records []any
	var totalObjects int
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		bucket := "permission-" + group
		if e = root.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); e != nil {
			t.Fatal(e)
		}
		if e = root.SetBucketVersioning(ctx, bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); e != nil {
			t.Fatal(e)
		}
		copies := cfg.Copies[group]
		content := []byte("isolated native non-delete permission regression\n")
		for i := range copies {
			cp := &copies[i]
			cp.Namespace = bucket
			cp.ByteSize = int64(len(content))
			sum := sha256.Sum256(content)
			cp.ContentSHA256 = hex.EncodeToString(sum[:])
			info, err := root.PutObject(ctx, bucket, cp.Key, bytes.NewReader(content), int64(len(content)), minio.PutObjectOptions{DisableMultipart: true})
			if err != nil {
				t.Fatal(err)
			}
			cp.ProviderVersion = info.VersionID
			totalObjects++
		}
		_, policies, err := pilot.RemainingPolicies(group, copies)
		if err != nil {
			t.Fatal(err)
		}
		fixed := policies[pilot.RemainingPolicyName(group, "delete")]
		// Reproduce the original generated policy by removing ONLY the missing
		// native entry action. Resource/conditions/versions remain identical.
		var old map[string]any
		json.Unmarshal(fixed, &old)
		for _, s := range old["Statement"].([]any) {
			s.(map[string]any)["Action"] = []string{"s3:DeleteObjectVersion"}
		}
		oldRaw, _ := json.Marshal(old)
		for _, variant := range []struct {
			name   string
			policy []byte
		}{{"old", oldRaw}, {"fixed", fixed}} {
			user := "probe-" + variant.name + "-" + group
			secret := cfg.SecretKey + variant.name
			name := user + "-policy"
			if err = admin.AddCannedPolicy(ctx, name, variant.policy); err != nil {
				t.Fatal(err)
			}
			if err = admin.AddUser(ctx, user, secret); err != nil {
				t.Fatal(err)
			}
			if err = admin.SetPolicy(ctx, name, user, false); err != nil {
				t.Fatal(err)
			}
			disabled := false
			defer func() {
				if !disabled {
					admin.SetUserStatus(context.Background(), user, madmin.AccountDisabled)
				}
			}()
			for _, cp := range copies {
				isTarget := cp.Key == "fixture/p2.csv" || cp.Key == "fixture/t1.csv" || cp.Key == "fixture/t2.csv" || cp.Key == "fixture/h2-a.csv" || cp.Key == "fixture/m1-a.csv" || cp.Key == "fixture/m1-b.csv"
				for _, mode := range []string{"exact", "foreign-version", "bare-key", "foreign-key", "foreign-bucket", "bad-signature", "host-change", "relay"} {
					if mode != "exact" && !isTarget {
						continue
					}
					key, version, requestBucket := cp.Key, cp.ProviderVersion, bucket
					if mode == "foreign-version" {
						version = uuid.NewString()
					}
					if mode == "bare-key" {
						version = ""
					}
					if mode == "foreign-key" {
						key = "other.csv"
					}
					if mode == "foreign-bucket" {
						requestBucket = "another-bucket"
					}
					endpoint := "http://" + cfg.Endpoint
					var relay *httptest.Server
					var receiptDir string
					if mode == "relay" {
						_, private, _ := ed25519.GenerateKey(rand.Reader)
						backend, _ := url.Parse(endpoint)
						receiptDir = filepath.Join(cfg.Output, "wire-"+group+"-"+variant.name+"-"+cp.ID.String())
						if err = os.Mkdir(receiptDir, 0700); err != nil {
							t.Fatal(err)
						}
						relay = httptest.NewServer(&wireHandler{scope: []domain.Copy{cp}, access: user, key: private, backend: backend, transport: probe, output: receiptDir, budget: t.TempDir(), fresh: func() error { return nil }, attempt: func(context.Context, domain.Copy) (domain.ProviderAttempt, error) {
							return domain.ProviderAttempt{ID: uuid.New(), CopyID: cp.ID, WorkspaceID: cp.WorkspaceID}, nil
						}})
						endpoint = relay.URL
					}
					u := endpoint + "/" + requestBucket + "/" + key
					if version != "" {
						u += "?versionId=" + version
					}
					req, _ := http.NewRequestWithContext(ctx, "DELETE", u, nil)
					req.Header.Set(nativePermissionCheck, "true")
					req.Header.Set("X-Amz-Content-Sha256", hex.EncodeToString(sha256.New().Sum(nil)))
					signingSecret := secret
					if mode == "bad-signature" {
						signingSecret += "wrong"
					}
					req = signer.SignV4(*req, user, signingSecret, "", "us-east-1")
					if mode == "host-change" {
						req.Host = "changed.invalid"
					}
					response, err := probe.RoundTrip(req)
					if relay != nil {
						relay.Close()
					}
					if err != nil {
						t.Fatal(err)
					}
					body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
					response.Body.Close()
					if err != nil || len(body) > 65536 {
						t.Fatal("bounded native response")
					}
					detail := storage.InspectExactDeleteResponse(response.StatusCode, body, true)
					want, status := "AccessDenied", 403
					if mode == "bad-signature" || mode == "host-change" {
						want = "SignatureDoesNotMatch"
					} else if variant.name == "fixed" && isTarget && (mode == "exact" || mode == "relay") {
						want, status = "ReplicationPermissionCheck", 400
					}
					if detail.ErrorCode != want || detail.HTTPStatus != status {
						t.Fatalf("%s %s %s %s: %+v wanted %s/%d", group, variant.name, cp.Key, mode, detail, want, status)
					}
					if receiptDir != "" {
						files, _ := os.ReadDir(receiptDir)
						if len(files) != 1 {
							t.Fatal("wire response receipt")
						}
					}
					records = append(records, map[string]any{"group": group, "policy": variant.name, "key": cp.Key, "mode": mode, "response": detail})
				}
			}
			if err = admin.SetUserStatus(ctx, user, madmin.AccountDisabled); err != nil {
				t.Fatal(err)
			}
			userInfo, err := admin.GetUserInfo(ctx, user)
			if err != nil || userInfo.Status != madmin.AccountDisabled || userInfo.PolicyName != name || len(userInfo.MemberOf) != 0 {
				t.Fatal("fresh native IAM disabled readback")
			}
			disabled = true
		}
		seen := map[string]bool{}
		for o := range root.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
			if o.Err != nil || o.IsDeleteMarker || seen[o.Key] {
				t.Fatal("inventory changed", o.Err)
			}
			seen[o.Key] = true
			var cp domain.Copy
			for _, c := range copies {
				if c.Key == o.Key {
					cp = c
				}
			}
			if cp.ProviderVersion != o.VersionID {
				t.Fatal("version changed")
			}
			obj, err := root.GetObject(ctx, bucket, o.Key, minio.GetObjectOptions{VersionID: o.VersionID})
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(obj)
			obj.Close()
			if err != nil || !bytes.Equal(b, content) {
				t.Fatal("bytes changed")
			}
		}
		if len(seen) != len(copies) {
			t.Fatal("membership changed")
		}
	}
	// The transport guard is exercised independently with zero outgoing I/O.
	before := probe.calls
	req, _ := http.NewRequest("DELETE", "http://"+cfg.Endpoint+"/forbidden/key", nil)
	if _, err := probe.RoundTrip(req); err == nil || probe.calls != before {
		t.Fatal("unguarded DELETE left process")
	}
	if e = save(filepath.Join(cfg.Output, "native-permission-result.json"), map[string]any{"status": "PASS_NATIVE_NON_MUTATING_PERMISSION_REGRESSION", "objects_unchanged": totalObjects, "probe_requests": records, "transport_roundtrips": probe.calls, "actual_object_deletions": 0, "ordinary_delete_requests": 0}); e != nil {
		t.Fatal(e)
	}
}
