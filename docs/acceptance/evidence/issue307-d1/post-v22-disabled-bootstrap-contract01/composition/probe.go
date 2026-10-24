// Isolated same-image SDK contract probe. No source profile or object API exists.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type config struct{ RootAccess, RootSecret string }
type journal struct {
	base http.RoundTripper
	seen map[string]bool
}

func save(name string, v any) error {
	raw, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	raw = append(raw, '\n')
	f, e := os.OpenFile(filepath.Join("/run/output", name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	_, e = f.Write(raw)
	return e
}
func (t *journal) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "http" || r.URL.Host != "127.0.0.1:9000" {
		return nil, errors.New("probe endpoint differs")
	}
	allowed := map[string]bool{"GET /minio/admin/v3/user-info": true, "PUT /minio/admin/v3/add-user": true, "PUT /minio/admin/v3/add-canned-policy": true, "POST /minio/admin/v3/idp/builtin/policy/attach": true, "PUT /minio/admin/v3/set-user-or-group-policy": true, "PUT /minio/admin/v3/import-iam-v2": true}
	if !allowed[r.Method+" "+r.URL.Path] {
		return nil, errors.New("probe non-admin or unexpected operation denied")
	}
	if r.Method != "GET" {
		key := r.Method + " " + r.URL.RequestURI()
		if t.seen[key] {
			return nil, errors.New("probe mutation replay denied")
		}
		t.seen[key] = true
	}
	id := uuid.NewString()
	if e := save("start-"+id+".json", map[string]any{"id": id, "at": time.Now().UTC(), "method": r.Method, "path": r.URL.Path, "query": r.URL.RawQuery}); e != nil {
		return nil, e
	}
	result, e := t.base.RoundTrip(r)
	status := 0
	if result != nil {
		status = result.StatusCode
	}
	if e2 := save("observation-"+id+".json", map[string]any{"id": id, "at": time.Now().UTC(), "status": status, "transport_error": e != nil}); e2 != nil {
		return nil, e2
	}
	if r.URL.Path == "/minio/admin/v3/import-iam-v2" && result != nil {
		raw, e2 := io.ReadAll(io.LimitReader(result.Body, 65537))
		result.Body.Close()
		if e2 != nil || len(raw) > 65536 {
			return nil, errors.New("bounded import response required")
		}
		f, e2 := os.OpenFile("/run/output/import-response.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e2 != nil {
			return nil, e2
		}
		_, e2 = f.Write(raw)
		f.Close()
		if e2 != nil {
			return nil, e2
		}
		result.Body = io.NopCloser(bytes.NewReader(raw))
	}
	return result, e
}
func check(ok bool, message string) error {
	if !ok {
		return errors.New(message)
	}
	return nil
}
func run() error {
	raw, e := os.ReadFile("/run/private/probe.json")
	if e != nil {
		return e
	}
	var cfg config
	if e = json.Unmarshal(raw, &cfg); e != nil {
		return e
	}
	tr := &journal{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, seen: map[string]bool{}}
	admin, e := madmin.NewWithOptions("127.0.0.1:9000", &madmin.Options{Creds: credentials.NewStaticV4(cfg.RootAccess, cfg.RootSecret, ""), Transport: tr})
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	user, name := "dpp307-contract-disabled", "dpp307-contract-readonly"
	doc := []byte(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObjectVersion"],"Resource":["arn:aws:s3:::dpp307-contract/fixture.csv"]}]}`)
	if e = admin.AddCannedPolicy(ctx, name, doc); e != nil {
		return e
	}
	if e = admin.SetUser(ctx, user, "isolated-test-only-never-source-9d7b8f3a", madmin.AccountDisabled); e != nil {
		return e
	}
	info, e := admin.GetUserInfo(ctx, user)
	if e != nil {
		return e
	}
	if e = check(info.Status == madmin.AccountDisabled && info.PolicyName == "" && len(info.MemberOf) == 0, "new disabled empty user readback differs"); e != nil {
		return e
	}
	classify := func(e error) string {
		var p madmin.ErrorResponse
		if errors.As(e, &p) {
			return p.Code
		}
		return "NOT_PROVIDER_NO_SUCH_USER"
	}
	attach := classify(func() error {
		_, e := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{name}, User: user})
		return e
	}())
	legacy := classify(admin.SetPolicy(ctx, name, user, false))
	if e = check(attach == "XMinioAdminNoSuchUser" && legacy == "XMinioAdminNoSuchUser", "pinned disabled association rejection was not reproduced"); e != nil {
		return e
	}
	var blob bytes.Buffer
	zw := zip.NewWriter(&blob)
	entry, e := zw.CreateHeader(&zip.FileHeader{Name: "iam-assets/user_mappings.json", Method: zip.Store})
	if e != nil {
		return e
	}
	mapping := map[string]any{user: map[string]any{"version": 1, "policy": name}}
	data, _ := json.Marshal(mapping)
	if _, e = entry.Write(data); e != nil {
		return e
	}
	if e = zw.Close(); e != nil {
		return e
	}
	f, e := os.OpenFile("/run/output/request.zip", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(blob.Bytes())
	f.Close()
	if e != nil {
		return e
	}
	result, e := admin.ImportIAMV2(ctx, io.NopCloser(bytes.NewReader(blob.Bytes())))
	if e != nil {
		return e
	}
	info, e = admin.GetUserInfo(ctx, user)
	if e != nil {
		return e
	}
	if e = check(info.Status == madmin.AccountDisabled && info.PolicyName == name && len(info.MemberOf) == 0, "disabled sole-policy readback differs after import"); e != nil {
		return e
	}
	if e = save("sdk-import-result.json", result); e != nil {
		return e
	}
	return save("result.json", map[string]any{"status": "PASS_SAME_IMAGE_DISABLED_MAPPING_API_PROBE", "attach_policy_error": attach, "set_policy_error": legacy, "account_status": info.Status, "sole_policy": info.PolicyName, "groups": info.MemberOf, "native_object_put": 0, "native_delete": 0, "source_calls": 0, "production_adapter_contract": "NOT_YET_VALIDATED", "amount": "NOT_ASSESSED"})
}
func main() {
	if e := run(); e != nil {
		_ = save("failure.json", map[string]any{"error": e.Error(), "source_calls": 0, "native_delete": 0})
		fmt.Println("isolated contract failed:", e)
		os.Exit(1)
	}
	fmt.Println("isolated contract probe PASS; source_calls=0; DELETE=0")
}
