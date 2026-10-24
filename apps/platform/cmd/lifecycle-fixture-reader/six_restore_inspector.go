package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/minio/pkg/v3/policy"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

const sixRestoreInspectorPurpose = "SIX_RESTORE_INSPECTOR_NONDELETE_REHEARSAL"

func parseSixRestoreInspector(raw, mapping []byte, at time.Time) (postV22ReadProfile, error) {
	p, err := parseSixMaintenanceProfile(raw, mapping, at, sixRestoreInspectorPurpose)
	var full sixReadProfile
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(raw, &full)
	if full.Role != "RESTORE" {
		return p, errors.New("non-delete rehearsal inspector cannot target SOURCE")
	}
	return p, nil
}

// A separate preparation grant, only on an actually bound RESTORE namespace.
// It never enables a source identity or any identity with DELETE permission.
func sixRestoreInspector(ctx context.Context, enable bool) error {
	raw, err := os.ReadFile("/run/private/restore-inspector.json")
	if err != nil {
		return err
	}
	mapping, err := os.ReadFile("/run/private/six-map.json")
	if err != nil {
		return err
	}
	p, err := parseSixRestoreInspector(raw, mapping, time.Now().UTC())
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(executable)
	if err != nil || postV22Hash(binary) != p.BinarySHA256 {
		return errors.New("restore inspector binary differs")
	}
	_, actual, _, err := postV22InventoriesFromFiles(p)
	if err != nil {
		return err
	}
	cp := []domain.Copy{}
	for _, row := range actual {
		cp = append(cp, row.Copy)
	}
	users, policies, err := pilot.RemainingPolicies(p.Group, cp)
	if err != nil {
		return err
	}
	user, name := pilot.RemainingIdentity(p.Group, "inspect"), pilot.RemainingPolicyName(p.Group, "inspect")
	status := madmin.AccountDisabled
	if enable {
		status = madmin.AccountEnabled
	}
	c := config{RootSecret: p.RootSecret, OutputDirectory: "/run/output"}
	transport := &sixProvisionTransport{base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, c: c, users: map[string]string{user: users[user]}, policies: map[string][]byte{name: policies[name]}, mutations: map[string]bool{}, restoreInspector: user, restoreStatus: status}
	admin, err := madmin.NewWithOptions(p.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(p.RootAccess, p.RootSecret, ""), Transport: transport})
	if err != nil {
		return err
	}
	if enable {
		info, e := admin.GetUserInfo(ctx, user)
		if e != nil || info.Status != madmin.AccountDisabled || !remainingExactUserPolicy(info, name) {
			return errors.New("restore inspector not exactly disabled and scoped")
		}
		raw, e := admin.InfoCannedPolicy(ctx, name)
		if e != nil {
			return e
		}
		a, e := policy.ParseConfig(bytes.NewReader(raw))
		b, e2 := policy.ParseConfig(bytes.NewReader(policies[name]))
		if e != nil || e2 != nil || !a.Equals(*b) {
			return errors.New("restore inspector policy expanded")
		}
	}
	// Disable happens before any optional policy validation. All actual readback
	// failures remain unconfirmed and the host still stops every known owner.
	if err = admin.SetUserStatus(ctx, user, status); err != nil {
		return err
	}
	info, err := admin.GetUserInfo(ctx, user)
	if err != nil || info.Status != status {
		return errors.New("restore inspector status unconfirmed")
	}
	return write(c, "restore-inspector.json", map[string]any{"group": p.Group, "role": "RESTORE", "identity": user, "status": string(status), "native_delete": 0, "source_identity_changed": false})
}
