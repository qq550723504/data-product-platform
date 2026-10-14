package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
)

type config struct {
	RootAccess, RootSecret, RunID string
	Enabled                       bool
	Users                         map[string]string
}
type observation struct {
	Role    string `json:"role"`
	HTTP    int    `json:"http_status"`
	Invoked bool   `json:"invoked"`
}
type transport struct {
	base         http.RoundTripper
	roles        map[string]string
	seen         map[string]bool
	observations []observation
}

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	key := r.URL.Query().Get("accessKey")
	role, ok := t.roles[key]
	if !ok || t.seen[key] || r.Method != "GET" || r.URL.Path != "/minio/admin/v3/user-info" || len(r.URL.Query()) != 1 {
		return nil, fmt.Errorf("read-only IAM scope or resend refused")
	}
	t.seen[key] = true
	response, e := t.base.RoundTrip(r)
	code := 0
	if response != nil {
		code = response.StatusCode
	}
	t.observations = append(t.observations, observation{role, code, true})
	return response, e
}
func run() error {
	var c config
	raw, e := os.ReadFile("/run/private/iam.json")
	if e != nil || json.Unmarshal(raw, &c) != nil || c.Enabled || c.RunID != "5d2274ee-a671-4bdc-b406-22b8dad8fd61" || c.RootAccess == "" || c.RootSecret == "" || len(c.Users) != 4 {
		return fmt.Errorf("fixed read-only configuration refused")
	}
	roles := map[string]string{}
	for _, role := range []string{"reader", "producer", "inspector", "executor"} {
		key := c.Users[role]
		if key == "" || roles[key] != "" {
			return fmt.Errorf("fixed account membership refused")
		}
		roles[key] = role
	}
	if c.Users["executor"] != "refdelete614cd9ad" {
		return fmt.Errorf("executor identity refused")
	}
	tr := &transport{base: http.DefaultTransport, roles: roles, seen: map[string]bool{}}
	admin, e := madmin.New("127.0.0.1:9000", c.RootAccess, c.RootSecret, false)
	if e != nil {
		return fmt.Errorf("native admin client refused")
	}
	admin.SetCustomTransport(tr)
	minio.MaxRetry = 1
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	facts := []map[string]any{}
	good := true
	for _, role := range []string{"reader", "producer", "inspector", "executor"} {
		info, e := admin.GetUserInfo(ctx, c.Users[role])
		status := fmt.Sprint(info.Status)
		code := ""
		if e != nil {
			code = madmin.ToErrorResponse(e).Code
			status = "UNKNOWN"
			if role == "executor" && code == "XMinioAdminNoSuchUser" {
				status = "NOT_PROVISIONED"
			}
		}
		expected := info.Status == madmin.AccountDisabled && e == nil
		if role == "inspector" {
			expected = info.Status == madmin.AccountEnabled && e == nil
		}
		if role == "executor" {
			expected = status == "NOT_PROVISIONED"
		}
		good = good && expected
		facts = append(facts, map[string]any{"role": role, "status": status, "error_code": code, "expected": expected})
	}
	result := map[string]any{"checked_at": time.Now().UTC(), "facts": facts, "rpc": tr.observations, "rpc_count": len(tr.observations), "mutations": 0, "delete": 0, "root_payload_mount": false, "management_only": true, "status": "PASS"}
	if !good {
		result["status"] = "BLOCKED"
	}
	file, e := os.OpenFile("/run/output/native-iam-readonly.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	if e = enc.Encode(result); e != nil {
		return e
	}
	if !good || len(tr.observations) != 4 {
		return fmt.Errorf("native account precondition blocked")
	}
	return nil
}
func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "read-only IAM probe failed; retained, no automatic retry")
		os.Exit(1)
	}
}
