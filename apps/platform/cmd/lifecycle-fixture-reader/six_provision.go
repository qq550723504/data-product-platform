package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/pkg/v3/policy"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

const sixProvisionPurpose = "SIX_PREPARATION_PROVISION_DISABLED_ONLY"

type sixProvisionTransport struct {
	base             http.RoundTripper
	c                config
	users            map[string]string
	policies         map[string][]byte
	mutations        map[string]bool
	restoreInspector string
	restoreStatus    madmin.AccountStatus
}

func (t *sixProvisionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	user, name := req.URL.Query().Get("accessKey"), req.URL.Query().Get("name")
	read := req.Method == "GET" && ((req.URL.Path == "/minio/admin/v3/user-info" && t.users[user] != "") || (req.URL.Path == "/minio/admin/v3/info-canned-policy" && name != ""))
	add := req.Method == "PUT" && ((req.URL.Path == "/minio/admin/v3/add-user" && t.users[user] != "") || (req.URL.Path == "/minio/admin/v3/add-canned-policy" && name != ""))
	attach := req.Method == "POST" && req.URL.Path == "/minio/admin/v3/idp/builtin/policy/attach"
	statusOnly := t.restoreInspector != "" && req.Method == "PUT" && req.URL.Path == "/minio/admin/v3/set-user-status" && user == t.restoreInspector && req.URL.Query().Get("status") == string(t.restoreStatus)
	if t.restoreInspector != "" {
		add = false
		attach = false
	}
	if name != "" {
		known := false
		for _, n := range t.users {
			known = known || n == name
		}
		read = read && known
		add = add && known
	}
	if req.URL.Scheme != "http" || req.URL.Host != "127.0.0.1:9000" || (!read && !add && !attach && !statusOnly) {
		return nil, errors.New("disabled-only bootstrap API outside finite scope")
	}
	if !read {
		var raw []byte
		var err error
		if !statusOnly {
			raw, err = io.ReadAll(io.LimitReader(req.Body, 1<<20))
		}
		if err != nil || len(raw) == 1<<20 {
			return nil, errors.New("bounded bootstrap request required")
		}
		req.Body = io.NopCloser(bytes.NewReader(raw))
		key := req.Method + " " + req.URL.RequestURI()
		if statusOnly {
			// Only the restored fixture's sole read-only inspector can change.
		} else if req.URL.Path == "/minio/admin/v3/add-canned-policy" {
			a, e := policy.ParseConfig(bytes.NewReader(raw))
			b, e2 := policy.ParseConfig(bytes.NewReader(t.policies[name]))
			if e != nil || e2 != nil || !a.Equals(*b) {
				return nil, errors.New("bootstrap policy is not the exact generated scope")
			}
		} else {
			plain, err := madmin.DecryptData(t.c.RootSecret, bytes.NewReader(raw))
			if err != nil {
				return nil, err
			}
			if attach {
				var association madmin.PolicyAssociationReq
				if strictPostV22(plain, &association) != nil || t.users[association.User] == "" || association.Group != "" || len(association.Policies) != 1 || association.Policies[0] != t.users[association.User] {
					return nil, errors.New("bootstrap association expands scope")
				}
				key += " " + association.User
			} else {
				var creation madmin.AddOrUpdateUserReq
				if strictPostV22(plain, &creation) != nil || creation.Status != madmin.AccountDisabled || creation.Policy != "" || len(creation.SecretKey) < 32 {
					return nil, errors.New("bootstrap may create only disabled independent users")
				}
			}
		}
		// No SDK mutation replay. A failed/unknown preparation is retained.
		if t.mutations[key] {
			return nil, errors.New("bootstrap mutation replay denied")
		}
		t.mutations[key] = true
	}
	id := uuid.New()
	if err := write(t.c, "prepare-start-"+id.String()+".json", map[string]any{"id": id, "method": req.Method, "path": req.URL.Path, "query": req.URL.RawQuery, "at": time.Now().UTC()}); err != nil {
		return nil, err
	}
	response, err := t.base.RoundTrip(req)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	if record := write(t.c, "prepare-observation-"+id.String()+".json", map[string]any{"id": id, "status": status, "transport_error": err != nil, "at": time.Now().UTC()}); record != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, record
	}
	return response, err
}

func sixProvisionDisabled(ctx context.Context) error {
	raw, err := os.ReadFile("/run/private/six-provision.json")
	if err != nil {
		return err
	}
	mapping, err := os.ReadFile("/run/private/six-map.json")
	if err != nil {
		return err
	}
	p, err := parseSixMaintenanceProfile(raw, mapping, time.Now().UTC(), sixProvisionPurpose)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(executable)
	if err != nil || postV22Hash(binary) != p.BinarySHA256 {
		return errors.New("exact preparation binary differs")
	}
	var full sixReadProfile
	_ = json.Unmarshal(raw, &full)
	if full.Role != "SOURCE" {
		return errors.New("provision only original source; restore receives the frozen archive")
	}
	_, actual, _, err := postV22InventoriesFromFiles(p)
	if err != nil {
		return err
	}
	var secrets map[string]string
	secretRaw, err := os.ReadFile("/run/private/new-secrets.json")
	if err != nil || strictPostV22(secretRaw, &secrets) != nil {
		return errors.New("new disabled identity secrets absent")
	}
	copies := []domain.Copy{}
	for _, row := range actual {
		copies = append(copies, row.Copy)
	}
	users, policies, err := pilot.RemainingPolicies(p.Group, copies)
	if err != nil {
		return err
	}
	if len(secrets) != 2 {
		return errors.New("exact new identity pair required")
	}
	for user := range users {
		if len(secrets[user]) < 32 {
			return errors.New("new independent secret absent")
		}
	}
	c := config{Endpoint: p.Endpoint, RootAccess: p.RootAccess, RootSecret: p.RootSecret, OutputDirectory: "/run/output"}
	transport := &sixProvisionTransport{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, c: c, users: users, policies: policies, mutations: map[string]bool{}}
	admin, err := madmin.NewWithOptions(c.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Transport: transport})
	if err != nil {
		return err
	}
	for _, kind := range []string{"inspect", "delete"} {
		user, name := pilot.RemainingIdentity(p.Group, kind), pilot.RemainingPolicyName(p.Group, kind)
		_, err = admin.GetUserInfo(ctx, user)
		var provider madmin.ErrorResponse
		if !errors.As(err, &provider) || provider.Code != "XMinioAdminNoSuchUser" {
			return errors.New("new username already exists or absence unconfirmed")
		}
		_, err = admin.InfoCannedPolicy(ctx, name)
		if !errors.As(err, &provider) || provider.Code != "XMinioAdminNoSuchPolicy" {
			return errors.New("new policy already exists or absence unconfirmed")
		}
		if _, err = policy.ParseConfig(bytes.NewReader(policies[name])); err != nil {
			return err
		}
		if err = admin.AddCannedPolicy(ctx, name, policies[name]); err != nil {
			return err
		}
		// AddUser enables by default. Use the existing SDK's explicit disabled
		// creation API so no enabled DELETE identity exists during preparation.
		if err = admin.SetUser(ctx, user, secrets[user], madmin.AccountDisabled); err != nil {
			return err
		}
		info, err := admin.GetUserInfo(ctx, user)
		if err != nil || info.Status != madmin.AccountDisabled {
			return errors.New("disabled creation readback failed")
		}
		if _, err = admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{name}, User: user}); err != nil {
			return err
		}
		info, err = admin.GetUserInfo(ctx, user)
		if err != nil || info.Status != madmin.AccountDisabled || !remainingExactUserPolicy(info, name) {
			return errors.New("exact disabled association readback failed")
		}
	}
	return write(c, "disabled-provision.json", map[string]any{"execution": full.Execution, "group": p.Group, "identities": users, "status": "PASS_CREATED_DISABLED_NO_TRANSIENT_ENABLE", "native_delete": 0, "enabled_identities": 0})
}

func postV22InventoriesFromFiles(p postV22ReadProfile) ([]remainingExpected, []remainingExpected, *domain.Copy, error) {
	frozen, err := os.ReadFile("/run/private/frozen-inventory.json")
	if err != nil {
		return nil, nil, nil, err
	}
	present, err := os.ReadFile("/run/private/present-inventory.json")
	if err != nil {
		return nil, nil, nil, err
	}
	return postV22Inventories(p, frozen, present)
}
