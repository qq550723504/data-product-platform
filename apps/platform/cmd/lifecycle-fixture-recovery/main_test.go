package main

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewScopeAuthorityNeverGrantsApprovalOrDispatch(t *testing.T) {
	dir := t.TempDir()
	c := config{Mode: "review", WorkspaceID: uuid.New(), ActorID: uuid.New(), PostgresDSN: "test-no-provider-or-db", RecoveryConfig: filepath.Join(dir, "recovery.json"), OutputDirectory: dir, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	path := filepath.Join(dir, "review.json")
	b, _ := json.Marshal(c)
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	a := sqlAuthority{path}
	if _, e := a.AuthorizePlan(context.Background(), nil, c.WorkspaceID); e != nil {
		t.Fatal(e)
	}
	for _, cap := range []string{lc.CapabilityApproveDisposition, lc.CapabilityDispatch, lc.CapabilityInspectStorage} {
		if _, e := a.AuthorizeLifecycle(context.Background(), nil, c.WorkspaceID, cap, time.Now().UTC()); e == nil {
			t.Fatal("scope review granted mutation/provider capability", cap)
		}
	}
	if _, e := a.AuthorizePlan(context.Background(), nil, uuid.New()); e == nil {
		t.Fatal("cross-workspace review accepted")
	}
}
