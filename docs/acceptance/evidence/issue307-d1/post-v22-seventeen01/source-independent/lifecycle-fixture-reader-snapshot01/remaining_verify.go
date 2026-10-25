package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/pkg/v3/policy"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

type remainingExpected struct {
	Copy        domain.Copy
	LegalHold   bool
	Retention   string
	RetainUntil *time.Time
}
type remainingVerifyTransport struct {
	c    config
	base http.RoundTripper
}

func (t remainingVerifyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "http" || req.URL.Host != t.c.Endpoint || req.URL.User != nil || (req.Method != "GET" && req.Method != "HEAD") {
		return nil, errors.New("native verifier permits only GET/HEAD")
	}
	id := uuid.New()
	if e := write(t.c, "readonly-start-"+id.String()+".json", map[string]any{"id": id, "method": req.Method, "path": req.URL.Path, "at": time.Now().UTC()}); e != nil {
		return nil, e
	}
	response, e := t.base.RoundTrip(req)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	if j := write(t.c, "readonly-observation-"+id.String()+".json", map[string]any{"id": id, "status": status, "transport_error": e != nil, "at": time.Now().UTC()}); j != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, j
	}
	return response, e
}
func remainingVerifyNative(ctx context.Context, r remainingConfig) error {
	c := r.Base
	if c.RootAccess == "" || c.RootSecret == "" {
		return errors.New("separate read-only native verification admin required")
	}
	b, e := os.ReadFile("/run/private/inventory.json")
	sum := sha256.Sum256(b)
	if e != nil || hex.EncodeToString(sum[:]) != r.InventorySHA256 {
		return errors.New("native verification inventory binding differs")
	}
	var expected []remainingExpected
	if json.Unmarshal(b, &expected) != nil {
		return errors.New("bad native inventory")
	}
	if e := validateRemainingInventory(r.Group, expected); e != nil {
		return e
	}
	transport := remainingVerifyTransport{c, &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}}
	result, e := collectRemainingNative(ctx, c, r.Group, expected, expected, transport)
	if e != nil {
		return e
	}
	result["inventory_sha256"] = r.InventorySHA256
	result["source_config"] = filepath.Base("remaining.json")
	return write(c, "native-verification.json", result)
}

func validateRemainingInventory(group string, expected []remainingExpected) error {
	scope, _ := pilot.RemainingScopeFor(group)
	found := map[uuid.UUID]bool{}
	keys := map[string]bool{}
	extra := 0
	for _, o := range expected {
		cp := o.Copy
		if cp.Validate() != nil || found[cp.ID] || keys[cp.Namespace+"/"+cp.Key] || cp.WorkspaceID != scope.Workspace || cp.ProviderInstanceID != scope.Provider || cp.IncarnationID != scope.Incarnation {
			return errors.New("native inventory scope differs")
		}
		found[cp.ID] = true
		keys[cp.Namespace+"/"+cp.Key] = true
		original := false
		for _, source := range scope.Copies {
			original = original || cp == source
		}
		if !original {
			if group != "business-races" || cp.Namespace != namespaces[0] || (cp.Key != "fixture/remaining-standardized.csv" && cp.Key != "fixture/remaining-processed.csv") {
				return errors.New("unexpected native output")
			}
			extra++
		}
	}
	if len(expected) != len(scope.Copies)+extra || (group == "business-races" && extra != 2) {
		return errors.New("incomplete native inventory")
	}
	for _, cp := range scope.Copies {
		if !found[cp.ID] {
			return errors.New("original native copy missing")
		}
	}
	return nil
}

// Present versions and policy resources are separate inputs. Legacy verification
// passes the same complete inventory twice; post-v22 keeps the old policy scope.
func collectRemainingNative(ctx context.Context, c config, group string, expected, policyExpected []remainingExpected, transport http.RoundTripper) (map[string]any, error) {
	scope, e := pilot.RemainingScopeFor(group)
	if e != nil {
		return nil, e
	}
	client, e := minio.New(c.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: transport})
	if e != nil {
		return nil, e
	}
	var verified []remainingExpected
	all := []domain.Copy{}
	for _, o := range policyExpected {
		all = append(all, o.Copy)
	}
	for _, o := range expected {
		cp := o.Copy
		object, e := client.GetObject(ctx, cp.Namespace, cp.Key, minio.GetObjectOptions{VersionID: cp.ProviderVersion})
		if e != nil {
			return nil, e
		}
		content, e := io.ReadAll(io.LimitReader(object, cp.ByteSize+1))
		info, statErr := object.Stat()
		closeErr := object.Close()
		hash := sha256.Sum256(content)
		if e != nil || statErr != nil || closeErr != nil || int64(len(content)) != cp.ByteSize || hex.EncodeToString(hash[:]) != cp.ContentSHA256 || info.VersionID != cp.ProviderVersion {
			return nil, errors.New("native exact bytes differ")
		}
		mode, until, e := client.GetObjectRetention(ctx, cp.Namespace, cp.Key, cp.ProviderVersion)
		if e != nil {
			native := minio.ToErrorResponse(e)
			if !(o.Retention == "NONE" && native.StatusCode == 400 && native.Code == "NoSuchObjectLockConfiguration") {
				return nil, e
			}
		} else {
			actual := "NONE"
			if mode != nil {
				actual = string(*mode)
			}
			if actual != o.Retention || (until == nil) != (o.RetainUntil == nil) || (until != nil && !until.Equal(*o.RetainUntil)) {
				return nil, errors.New("native retention differs")
			}
		}
		hold, e := client.GetObjectLegalHold(ctx, cp.Namespace, cp.Key, minio.GetObjectLegalHoldOptions{VersionID: cp.ProviderVersion})
		if e != nil {
			native := minio.ToErrorResponse(e)
			if !(!o.LegalHold && native.StatusCode == 400 && native.Code == "NoSuchObjectLockConfiguration") {
				return nil, e
			}
		} else if (hold != nil && *hold == minio.LegalHoldEnabled) != o.LegalHold {
			return nil, errors.New("native legal hold differs")
		}
		verified = append(verified, o)
	}
	for _, namespace := range namespaces {
		want := map[string]int64{}
		for _, o := range expected {
			if o.Copy.Namespace == namespace {
				want[o.Copy.Key+"\x00"+o.Copy.ProviderVersion] = o.Copy.ByteSize
			}
		}
		for object := range client.ListObjects(ctx, namespace, minio.ListObjectsOptions{WithVersions: true, Recursive: true, MaxKeys: 1000}) {
			key := object.Key + "\x00" + object.VersionID
			size, ok := want[key]
			if object.Err != nil || !ok || object.IsDeleteMarker || object.Size != size {
				return nil, errors.New("native namespace inventory differs")
			}
			delete(want, key)
		}
		if len(want) != 0 {
			return nil, errors.New("native inventory incomplete")
		}
	}
	admin, e := madmin.NewWithOptions(c.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Secure: false, Transport: transport})
	if e != nil {
		return nil, e
	}
	users := map[string]string{c.InspectAccess: "lifecycle307-inspection-" + c.RunID.String(), "delete" + c.RunID.String()[:8]: "lifecycle307-remaining-delete-" + c.RunID.String()}
	if group == "business-races" {
		for _, name := range []string{"remainread808a6960", "remainput808a6960"} {
			users[name] = "lifecycle307-remaining-business-" + name
		}
	}
	for name, want := range users {
		user, e := admin.GetUserInfo(ctx, name)
		if e != nil {
			return nil, e
		}
		if user.Status != madmin.AccountDisabled || !remainingExactUserPolicy(user, want) {
			return nil, errors.New("native IAM identity is not disabled with its sole exact policy")
		}
	}
	statements := []any{}
	for _, cp := range scope.Copies {
		send := false
		for _, key := range []string{"fixture/p1.csv", "fixture/p2.csv", "fixture/t1.csv", "fixture/t2.csv", "fixture/h2-a.csv", "fixture/m1-a.csv", "fixture/m1-b.csv"} {
			send = send || cp.Key == key
		}
		if send {
			statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:DeleteObject", "s3:DeleteObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + cp.Namespace + "/" + cp.Key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": cp.ProviderVersion}}})
		}
	}
	deletion, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
	policies := map[string][]byte{"lifecycle307-remaining-delete-" + c.RunID.String(): deletion, "lifecycle307-inspection-" + c.RunID.String(): remainingInspectionPolicy(all)}
	if group == "business-races" {
		read, put, _ := remainingPolicies(all)
		policies["lifecycle307-remaining-business-remainread808a6960"] = read
		policies["lifecycle307-remaining-business-remainput808a6960"] = put
	}
	hashes := map[string]string{}
	for name, want := range policies {
		raw, e := admin.InfoCannedPolicy(ctx, name)
		if e != nil {
			return nil, e
		}
		actual, e := policy.ParseConfig(bytes.NewReader(raw))
		if e != nil {
			return nil, e
		}
		expected, e := policy.ParseConfig(bytes.NewReader(want))
		if e != nil || !actual.Equals(*expected) {
			return nil, errors.New("native exact policy differs")
		}
		if e = write(c, "native-policy-"+name+".json", json.RawMessage(raw)); e != nil {
			return nil, e
		}
		s := sha256.Sum256(raw)
		hashes[name] = hex.EncodeToString(s[:])
	}
	return map[string]any{"group": group, "status": "PASS_EXACT_NATIVE_READONLY", "copies": verified, "disabled_identities": users, "policy_sha256": hashes, "delete": 0, "mutations": 0}, nil
}

func remainingExactUserPolicy(user madmin.UserInfo, want string) bool {
	// Current fixture users have no group membership. Rejecting all groups also
	// rejects inherited permissions, rather than checking only direct policies.
	if want == "" || len(user.MemberOf) != 0 {
		return false
	}
	policies := map[string]bool{}
	for _, name := range strings.Split(user.PolicyName, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			return false
		}
		policies[name] = true
	}
	return len(policies) == 1 && policies[want]
}
