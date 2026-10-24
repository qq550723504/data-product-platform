package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRetentionWaitFalseToTrueThenFreshAuthorization(t *testing.T) {
	f := testFixture(t)
	dir := f.dirs["business-races"]
	raw, _ := os.ReadFile(filepath.Join(dir, "bindings.json"))
	hash := digest(raw)
	writeJSON(t, filepath.Join(dir, "permission.json"), permission{false, hash})
	ticks := 0
	now := func() time.Time {
		ticks++
		if ticks == 2 {
			writeJSON(t, filepath.Join(dir, "permission.json"), permission{true, hash})
		}
		return f.at
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if waitPermission(ctx, dir, raw, now, time.Millisecond) != nil || f.requests != 0 {
		t.Fatal("wait failed or contacted provider")
	}
	// A true control is never authorization; stale proof is still rejected.
	os.WriteFile(filepath.Join(dir, "topology.json"), []byte(`{}`), 0600)
	if f.engine(t, "business-races").run(ctx) == nil || f.requests != 0 {
		t.Fatal("wait bypassed complete authorization")
	}
}

func TestRetentionWaitRejectsPreEnableDriftExpiryAndCancellation(t *testing.T) {
	for _, fault := range []string{"pre-enabled", "binding", "permission-hash", "duplicate", "expired", "early", "cancelled"} {
		t.Run(fault, func(t *testing.T) {
			f := testFixture(t)
			dir := f.dirs["business-races"]
			raw, _ := os.ReadFile(filepath.Join(dir, "bindings.json"))
			p := permission{false, digest(raw)}
			at := f.at
			if fault == "pre-enabled" {
				p.Enabled = true
			}
			if fault == "permission-hash" {
				p.BindingsSHA256 = digest([]byte("other"))
			}
			if fault == "expired" {
				at = f.b.NotAfter
			}
			if fault == "early" {
				at = f.b.NotBefore.Add(-time.Nanosecond)
			}
			writeJSON(t, filepath.Join(dir, "permission.json"), p)
			if fault == "binding" {
				os.WriteFile(filepath.Join(dir, "bindings.json"), append(raw, ' '), 0600)
			}
			if fault == "duplicate" {
				os.WriteFile(filepath.Join(dir, "permission.json"), []byte(`{"Enabled":false,"Enabled":true}`), 0600)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if fault == "cancelled" {
				cancel()
			}
			if waitPermission(ctx, dir, raw, func() time.Time { return at }, time.Millisecond) == nil || f.requests != 0 {
				t.Fatal("unsafe wait admitted")
			}
		})
	}
}

func TestRetentionLinuxRealDirectorySyncAndTwoOwnerReadback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("actual Linux fsync acceptance runs in network-none probe")
	}
	f := testFixture(t)
	for _, group := range []string{"business-races", "mixed"} {
		r := f.engine(t, group)
		r.syncDir = syncDirectory
		if err := r.run(context.Background()); err != nil {
			t.Fatal(group, err)
		}
		var terminal map[string]any
		raw, err := os.ReadFile(filepath.Join(f.outputs[group], "terminal.json"))
		if err != nil || json.Unmarshal(raw, &terminal) != nil || terminal["readback_confirmed"] != true {
			t.Fatal("real fsync readback missing")
		}
	}
	if f.puts != 2 {
		t.Fatal("test double attempts", f.puts)
	}
}

func TestRetentionLinuxMountedVolumeCustody(t *testing.T) {
	if os.Getenv("D1_RETENTION_VOLUME_PROBE") != "1" {
		t.Skip("isolated new-volume acceptance only")
	}
	if runtime.GOOS != "linux" || checkPrivateDirectories([]string{"/run/private", "/run/output", "/run/budget"}) != nil {
		t.Fatal("mount custody rejected")
	}
	for _, dir := range []string{"/run/output", "/run/budget"} {
		r := engine{syncDir: syncDirectory}
		if r.save(dir, "synthetic-fsync.json", map[string]any{"test_only": true, "provider_io": 0}) != nil {
			t.Fatal("mounted directory fsync failed")
		}
	}
}
