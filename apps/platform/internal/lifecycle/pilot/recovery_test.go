package pilot

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

func TestRecoveryWindowRevocationAndScopeChangeDenyNewAuthority(t *testing.T) {
	c := RecoveryConfig{Enabled: true, SyntheticOnly: true, RunID: uuid.New(), WorkspaceID: uuid.New(), ProviderInstanceID: uuid.New(), IncarnationID: uuid.New(), ActorID: uuid.New(), Endpoint: "127.0.0.1:9000", InspectAccess: "test-inspect-only", InspectSecret: "test-secret-not-real", Token: "test-private-token-not-real", ManifestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ValidUntil: time.Now().UTC().Add(time.Minute)}
	path := filepath.Join(t.TempDir(), "recovery.json")
	save := func(v RecoveryConfig) {
		t.Helper()
		b, _ := json.Marshal(v)
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	save(c)
	a := authority{path, c.Token, c}
	if _, e := a.AuthorizeLifecycle(context.Background(), nil, c.WorkspaceID, lc.CapabilityInspectStorage, time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
	if _, e := a.AuthorizeLifecycle(context.Background(), nil, c.WorkspaceID, lc.CapabilityDispatch, time.Now().UTC()); e == nil {
		t.Fatal("read-only window granted DELETE dispatch")
	}
	off := c
	off.Enabled = false
	save(off)
	if _, e := a.AuthorizeLifecycle(context.Background(), nil, c.WorkspaceID, lc.CapabilityInspectStorage, time.Now().UTC()); e == nil {
		t.Fatal("disabled window used")
	}
	changed := c
	changed.IncarnationID = uuid.New()
	save(changed)
	if _, e := a.AuthorizeLifecycle(context.Background(), nil, c.WorkspaceID, lc.CapabilityInspectStorage, time.Now().UTC()); e == nil {
		t.Fatal("scope changed under pinned worker")
	}
	save(c)
	if _, e := a.AuthorizeLifecycle(context.Background(), nil, c.WorkspaceID, lc.CapabilityReconcileDisposition, c.ValidUntil); e == nil {
		t.Fatal("expired window used")
	}
}
