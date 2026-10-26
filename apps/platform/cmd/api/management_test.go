package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A real main() subprocess proves invalid selection cannot reach full-service
// database/outbox/queue initialization. All inherited application config is removed.
func TestInvalidManagementProfileStopsMainBeforeFullService(t *testing.T) {
	if os.Getenv("DPP_MANAGEMENT_MAIN_TEST") == "1" {
		main()
		os.Exit(99)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInvalidManagementProfileStopsMainBeforeFullService$")
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if strings.HasPrefix(k, "PLATFORM_") || strings.HasPrefix(k, "PG") || k == "DATABASE_URL" || k == "DPP_MANAGEMENT_MAIN_TEST" {
			continue
		}
		cmd.Env = append(cmd.Env, v)
	}
	cmd.Env = append(cmd.Env, "DPP_MANAGEMENT_MAIN_TEST=1", "PLATFORM_MANAGEMENT_PROFILE="+filepath.Join(t.TempDir(), "PRIVATE_MISSING_PROFILE"), "DATABASE_URL=postgres://unused:unused@127.0.0.1:1/never_connect")
	b, e := cmd.CombinedOutput()
	exit, ok := e.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 || !strings.Contains(string(b), "management query-only composition unavailable") || strings.Contains(string(b), "outbox routing obligation configured") || strings.Contains(string(b), "PRIVATE_MISSING_PROFILE") {
		t.Fatal("invalid management selection reached full service or exposed private configuration")
	}
}
