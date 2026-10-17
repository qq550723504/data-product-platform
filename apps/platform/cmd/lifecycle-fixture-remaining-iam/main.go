// Trusted bootstrap only. Reuses MinIO's admin API; no S3 DELETE/COPY/hold API.
// Root credentials must never be mounted into a business/sender/recovery owner.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/pkg/v3/policy"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

const execution = pilot.RemainingExecution

type rootConfig struct{ Group, Endpoint, RootAccess, RootSecret string }
type candidate struct {
	Profile, Execution, OwnerBinarySHA256 string
	NotBefore, NotAfter                   time.Time
	Groups                                []struct{ Group, IAMSHA256 string }
}

func digest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func read(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func write(path string, v any) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = json.NewEncoder(f).Encode(v); e != nil {
		return e
	}
	return f.Sync()
}
func authorized() error {
	var c candidate
	var p struct {
		Enabled                                        bool
		Execution, CandidateSHA256, ConfirmationSHA256 string
		NotBefore, ExpiresAt, ConfirmedAt              time.Time
		MaxPerTarget, MaxTotal                         int
	}
	var frozen struct {
		CandidateSHA256    string `json:"candidate_sha256"`
		ConfirmationSHA256 string `json:"confirmation_sha256"`
	}
	if read("/run/private/candidate.json", &c) != nil || read("/run/private/permission.json", &p) != nil {
		return errors.New("new exact permission absent")
	}
	b, e := os.ReadFile("/run/private/candidate.json")
	if e != nil {
		return e
	}
	h, e := os.ReadFile("/run/private/human-confirmation.txt")
	expected, e2 := os.ReadFile("/run/private/confirmation-template.txt")
	now := time.Now().UTC()
	if read("/run/private/status.json", &frozen) != nil || frozen.CandidateSHA256 != digest(b) || frozen.ConfirmationSHA256 != digest(expected) {
		return errors.New("trusted confirmation template binding differs")
	}
	if e != nil || e2 != nil || len(h) == 0 || !bytes.Contains(h, []byte(execution)) || !bytes.Contains(h, []byte(digest(b))) || !bytes.Equal(h, expected) || !p.Enabled || p.Execution != execution || c.Execution != execution || c.Profile != pilot.RemainingProfile || digest(b) != p.CandidateSHA256 || digest(h) != p.ConfirmationSHA256 || c.NotBefore.Format(time.RFC3339) != pilot.RemainingNotBefore || c.NotAfter.Format(time.RFC3339) != pilot.RemainingNotAfter || now.Before(c.NotBefore) || !now.Before(c.NotAfter) || !p.NotBefore.Equal(c.NotBefore) || !p.ExpiresAt.Equal(c.NotAfter) || p.ConfirmedAt.IsZero() || now.Before(p.ConfirmedAt) || p.MaxPerTarget != 1 || p.MaxTotal != 7 {
		return errors.New("new exact IAM enable authority denied")
	}
	return nil
}

type transport struct {
	base   http.RoundTripper
	users  map[string]string
	enable bool
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	allowed := req.Method == "GET" && (req.URL.Path == "/minio/admin/v3/user-info" || req.URL.Path == "/minio/admin/v3/info-canned-policy")
	if req.Method == "PUT" && req.URL.Path == "/minio/admin/v3/set-user-status" {
		_, known := t.users[req.URL.Query().Get("accessKey")]
		status := req.URL.Query().Get("status")
		allowed = known && (status == "disabled" || (t.enable && status == "enabled" && authorized() == nil))
	}
	if req.URL.Scheme != "http" || req.URL.Host != "127.0.0.1:9000" || !allowed {
		return nil, errors.New("bootstrap permits only exact user status and policy reads")
	}
	id := uuid.NewString()
	if e := write("/run/output/start-"+id+".json", map[string]any{"method": req.Method, "path": req.URL.Path, "access": req.URL.Query().Get("accessKey"), "status": req.URL.Query().Get("status"), "at": time.Now().UTC()}); e != nil {
		return nil, e
	}
	response, e := t.base.RoundTrip(req)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	if j := write("/run/output/observation-"+id+".json", map[string]any{"http_status": status, "transport_error": e != nil, "at": time.Now().UTC()}); j != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, j
	}
	return response, e
}
func run(ctx context.Context, action string) error {
	disabling := action == "disable-send" || action == "disable-all"
	var c rootConfig
	if read("/run/private/root.json", &c) != nil || c.Endpoint != "127.0.0.1:9000" || c.RootAccess == "" || c.RootSecret == "" {
		return errors.New("separate fixed bootstrap config required")
	}
	s, e := pilot.RemainingScopeFor(c.Group)
	if e != nil {
		return e
	}
	var frozen candidate
	if !disabling && (read("/run/private/candidate.json", &frozen) != nil || frozen.Execution != execution || frozen.Profile != pilot.RemainingProfile) {
		return errors.New("foreign bootstrap candidate")
	}
	self, e := os.Executable()
	if e != nil {
		return e
	}
	binary, e := os.ReadFile(self)
	if !disabling && (e != nil || digest(binary) != frozen.OwnerBinarySHA256) {
		return errors.New("bootstrap binary drift")
	}
	enable := action == "enable-read" || action == "enable-send"
	if enable {
		if e = authorized(); e != nil {
			return e
		}
	} else if action != "check-disabled" && action != "disable-send" && action != "disable-all" {
		return errors.New("unknown bounded IAM action")
	}
	run := s.Run.String()
	inspection := "inspect" + strings.ReplaceAll(run, "-", "")[:12]
	deletion := "delete" + run[:8]
	users := map[string]string{inspection: "lifecycle307-inspection-" + run, deletion: "lifecycle307-remaining-delete-" + run}
	if c.Group == "business-races" {
		for _, user := range []string{"remainread808a6960", "remainput808a6960"} {
			users[user] = "lifecycle307-remaining-business-" + user
		}
	}
	admin, e := madmin.NewWithOptions(c.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Transport: transport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 8 * time.Second}, users, enable}})
	if e != nil {
		return e
	}
	changed := map[string]string{}
	if disabling {
		failures := []string{}
		for user := range users {
			if action == "disable-send" && user != deletion {
				continue
			}
			if err := admin.SetUserStatus(ctx, user, madmin.AccountDisabled); err != nil {
				failures = append(failures, user)
				continue
			}
			info, err := admin.GetUserInfo(ctx, user)
			if err != nil || info.Status != madmin.AccountDisabled {
				failures = append(failures, user)
				continue
			}
			changed[user] = "disabled"
		}
		if e = write("/run/output/status-changes.json", map[string]any{"action": action, "disabled_readback": changed, "unconfirmed": failures, "policy_validation": "AFTER_CONTAINMENT", "native_delete": 0}); e != nil {
			return e
		}
		if len(failures) > 0 {
			return errors.New("some IAM containment readbacks remain unconfirmed")
		}
		_ = read("/run/private/candidate.json", &frozen)
	}
	var iam struct {
		PolicyFiles map[string]string `json:"policy_file_sha256"`
	}
	iamRaw, e := os.ReadFile("/run/private/iam.json")
	if e != nil {
		return e
	}
	bound := ""
	for _, g := range frozen.Groups {
		if g.Group == c.Group {
			bound = g.IAMSHA256
		}
	}
	if digest(iamRaw) != bound || json.Unmarshal(iamRaw, &iam) != nil || len(iam.PolicyFiles) != len(users) {
		return errors.New("frozen IAM manifest differs; containment status is retained separately")
	}
	before := map[string]string{}
	for user, name := range users {
		info, e := admin.GetUserInfo(ctx, user)
		if e != nil {
			return e
		}
		set := map[string]bool{}
		for _, p := range strings.Split(info.PolicyName, ",") {
			set[strings.TrimSpace(p)] = true
		}
		if len(info.MemberOf) != 0 || len(set) != 1 || !set[name] {
			return errors.New("IAM effective policy association drift")
		}
		raw, e := admin.InfoCannedPolicy(ctx, name)
		if e != nil {
			return e
		}
		want, e := os.ReadFile("/run/private/policies/" + name + ".json")
		if e != nil || digest(want) != iam.PolicyFiles[name] {
			return errors.New("frozen policy file differs")
		}
		a, e := policy.ParseConfig(bytes.NewReader(raw))
		if e != nil {
			return e
		}
		b, e := policy.ParseConfig(bytes.NewReader(want))
		if e != nil || !a.Equals(*b) {
			return errors.New("frozen IAM policy content drift")
		}
		if action == "check-disabled" && info.Status != madmin.AccountDisabled {
			return errors.New("IAM identity not disabled")
		}
		before[user] = string(info.Status)
	}
	if enable {
		for user := range users {
			selected := action == "disable-all" || (action == "enable-read" && user != deletion) || ((action == "enable-send" || action == "disable-send") && user == deletion)
			if !selected {
				continue
			}
			status := madmin.AccountDisabled
			if enable {
				status = madmin.AccountEnabled
			}
			if e = admin.SetUserStatus(ctx, user, status); e != nil {
				return e
			}
			info, e := admin.GetUserInfo(ctx, user)
			if e != nil || info.Status != status {
				return errors.New("IAM status readback differs")
			}
			changed[user] = string(status)
		}
	}
	return write("/run/output/result.json", map[string]any{"group": c.Group, "action": action, "before": before, "changed": changed, "exact_policy_set_verified": true, "native_delete": 0, "root_scope": "bootstrap only; no worker mount"})
}
func main() {
	action := flag.String("action", "check-disabled", "bounded bootstrap action")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := run(ctx, *action); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
