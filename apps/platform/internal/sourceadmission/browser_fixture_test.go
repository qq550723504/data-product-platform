package sourceadmission

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
)

type browserResolver struct {
	f        *fixture
	resolver principal.Resolver
}

func (r browserResolver) Resolve(req *http.Request, ws uuid.UUID, cap string) (principal.RequestPrincipal, error) {
	if !r.f.allowed.Load() {
		return principal.RequestPrincipal{}, ErrDenied
	}
	return r.resolver.Resolve(req, ws, cap)
}

// Explicit test-only harness: real Host HTTP Command -> real PostgreSQL -> real
// protected-object journal, controlled provider/Fork Ports. Never cmd/api,
// a shared Core runtime, native providers or an existing LS environment.
func TestBrowserFixture(t *testing.T) {
	if os.Getenv("SOURCE_ADMISSION_E2E") != "1" {
		t.Skip("explicit browser acceptance harness only")
	}
	root := os.Getenv("SOURCE_ADMISSION_E2E_DIR")
	if root == "" {
		t.Fatal("private output directory required")
	}
	f := newFixture(t)
	originalBlob, originalManifest := f.blobs["manifest.json"], f.manifest
	budget := &fixture{blobs: map[string][]byte{"manifest.json": originalBlob}, c: f.c, r: f.r}
	declareBundleTotal(t, budget, MaxBytes+1)
	tokenPath := filepath.Join(root, "api-token")
	profilePath := filepath.Join(root, "web-profile.json")
	gatewayPath := filepath.Join(root, "web-gateway-token")
	for path, value := range map[string]string{tokenPath: "synthetic-host-api-token", gatewayPath: "synthetic-source-gateway"} {
		if e := os.WriteFile(path, []byte(value), 0600); e != nil {
			t.Fatal(e)
		}
	}
	p := Profile{Enabled: true, Connection: f.c, ActorID: f.p.ActorID, Subject: f.p.Subject, APITokenFile: tokenPath}
	b, _ := json.Marshal(p)
	if e := os.WriteFile(profilePath, b, 0600); e != nil {
		t.Fatal(e)
	}
	resolver, e := principal.NewStaticResolver(true, "synthetic-host-api-token", f.p.Subject, f.p.ActorID.String(), []string{f.c.WorkspaceID.String()}, []string{Capability})
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	(&Handler{Service: f.s, Resolver: browserResolver{f, resolver}}).Register(mux)
	stop := make(chan struct{}, 1)
	mux.HandleFunc("POST /__fixture/control", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Fixture-Control") != "synthetic-hfa-control" {
			w.WriteHeader(403)
			return
		}
		var cmd struct{ Action string }
		if json.NewDecoder(r.Body).Decode(&cmd) != nil {
			w.WriteHeader(400)
			return
		}
		switch cmd.Action {
		case "enable", "budget":
			f.allowed.Store(true)
			f.sourceAllowed.Store(true)
			f.sourceMu.Lock()
			f.blobs["manifest.json"], f.manifest = originalBlob, originalManifest
			if cmd.Action == "budget" {
				f.blobs["manifest.json"], f.manifest = budget.blobs["manifest.json"], budget.manifest
			}
			f.sourceMu.Unlock()
			_, e = f.pool.Exec(r.Context(), `UPDATE source_admission_connection SET enabled=true WHERE id=$1`, f.c.ID)
		case "deny":
			_, e = f.pool.Exec(r.Context(), `UPDATE source_admission_connection SET enabled=false WHERE id=$1`, f.c.ID)
		case "source-deny":
			f.sourceAllowed.Store(false)
		case "stop":
			select {
			case stop <- struct{}{}:
			default:
			}
		default:
			w.WriteHeader(400)
			return
		}
		if e != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /__fixture/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Fixture-Control") != "synthetic-hfa-control" {
			w.WriteHeader(403)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"versions": f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.c.DatasetID), "accepted": f.count(t, `SELECT count(*) FROM source_admission_capture WHERE dataset_id=$1 AND state='ACCEPTED_SOURCE'`, f.c.DatasetID), "sourceCalls": f.sourceCalls.Load()})
	})
	mux.HandleFunc("GET /__fixture/budget-state", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Fixture-Control") != "synthetic-hfa-control" {
			w.WriteHeader(403)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"calls":     f.sourceCalls.Load(),
			"artifacts": f.count(t, `SELECT count(*) FROM source_admission_call WHERE workspace_id=$1 AND asset_name<>'manifest.json'`, f.c.WorkspaceID),
			"costs":     f.count(t, `SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type='SOURCE_ADMISSION_FETCH'`, f.c.WorkspaceID),
			"received":  f.count(t, `SELECT count(*) FROM source_admission_call c JOIN source_admission_observation o ON o.call_id=c.id WHERE c.workspace_id=$1 AND c.asset_name='manifest.json' AND o.outcome='RECEIVED' AND o.status_code=200`, f.c.WorkspaceID),
			"rejected":  f.count(t, `SELECT count(*) FROM source_admission_operation WHERE workspace_id=$1 AND state='FAILED_FINAL' AND error_code='SOURCE_PROFILE_UNSUPPORTED'`, f.c.WorkspaceID),
			"versions":  f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.c.DatasetID),
			"captures":  f.count(t, `SELECT count(*) FROM source_admission_capture WHERE dataset_id=$1`, f.c.DatasetID),
			"copies":    f.count(t, `SELECT count(*) FROM lifecycle_protected_object_intent WHERE workspace_id=$1`, f.c.WorkspaceID),
		})
	})
	listener, e := net.Listen("tcp", "127.0.0.1:25481")
	if e != nil {
		t.Fatal(e)
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() { _ = server.Serve(listener) }()
	meta := map[string]any{"workspaceId": f.c.WorkspaceID, "datasetId": f.c.DatasetID, "connectionId": f.c.ID, "releaseId": f.r.ReleaseID, "snapshotId": f.r.SnapshotID, "manifestSha256": f.r.ManifestSHA, "profile": profilePath, "gatewayTokenFile": gatewayPath, "api": "http://127.0.0.1:25481"}
	meta["budgetManifestSha256"] = budget.r.ManifestSHA
	b, _ = json.Marshal(meta)
	if e = os.WriteFile(filepath.Join(root, "browser-fixture.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	select {
	case <-stop:
	case <-time.After(15 * time.Minute):
		t.Fatal("browser harness timed out")
	}
	_ = server.Shutdown(context.Background())
}
