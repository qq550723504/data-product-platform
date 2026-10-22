package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func exerciseProtectedResume(t *testing.T, pool *pgxpool.Pool, cfg config, in input, private string, body []byte, mode *atomic.Int32, providerCalls *atomic.Int64) {
	t.Helper()
	ctx := context.Background()
	identity := filepath.Join(private, "controlled-identity.json")
	if err := os.WriteFile(identity, []byte(`{"AccessKey":"controlled-access","SecretKey":"controlled-secret","SessionToken":"controlled-finite-session"}`), 0600); err != nil {
		t.Fatal(err)
	}
	profile := pilot.WorkspaceObjectProfile{Endpoint: cfg.Profile.StorageEndpoint, CredentialFile: identity, Scope: lc.ProtectedScope{
		WorkspaceID: cfg.Workspace, ActorID: cfg.Actor, NamespaceID: uuid.NewSHA1(cfg.Workspace, []byte("core-object-namespace")),
		ProviderInstanceID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(private+"/objects")), IncarnationID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(private+"/objects/data")),
		PrincipalRef: cfg.Profile.Subject, Bucket: "raw", ExpiresAt: time.Now().UTC().Add(time.Hour)}}
	profilePath := filepath.Join(private, "controlled-profile.json")
	value, _ := json.Marshal(profile)
	if err := os.WriteFile(profilePath, value, 0600); err != nil {
		t.Fatal(err)
	}
	in.Profile = profilePath
	service, err := pilot.BuildWorkspaceObjects(pool, profilePath)
	if err != nil {
		t.Fatal(err)
	}
	mode.Store(1)
	// Only fixture preparation creates the retained copies. The continuation
	// itself must read these two copies while preserving their legacy URI facts.
	for i, v := range in.Versions {
		if i >= 3 {
			if _, err := service.AddRetainedCopy(ctx, v.ID, "isolated-resume-copy-"+v.ID.String(), body); err != nil {
				t.Fatal("prepare independent protected copy", err)
			}
		} else {
			reader, err := service.ReadVersion(ctx, v.ID)
			if err != nil {
				t.Fatal("prepare existing SOURCE_READ intent", err)
			}
			_, _ = io.Copy(io.Discard, reader)
			_ = reader.Close()
		}
	}
	folder := filepath.Join(private, "resume-observations", "isolated")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	cli := func(candidate input, stage string) error {
		t.Helper()
		data, _ := json.Marshal(candidate)
		path := filepath.Join(private, stage, "isolated", uuid.NewString()+"-input.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DPP_TRIAL_ACTIVATION_INPUT", path)
		previousArgs, previousOut := os.Args, os.Stdout
		output, err := os.CreateTemp(folder, "resume-cli-")
		if err != nil {
			t.Fatal(err)
		}
		os.Args, os.Stdout = []string{"trial-activate", "resume-verify"}, output
		defer func() { os.Args, os.Stdout = previousArgs, previousOut; _ = output.Close() }()
		return run()
	}
	before, err := snapshot(ctx, pool, inventory{})
	if err != nil {
		t.Fatal(err)
	}
	requests := providerCalls.Load()
	// Identity, exact version facts, schema/custody and path are checked before
	// the first physical HEAD/GET, including a still-valid historical credential.
	for _, mutation := range []string{"versions", "uri", "schema", "custody", "path"} {
		candidate := in
		candidate.Versions = append([]expectedVersion(nil), in.Versions...)
		stage := "resume-observations"
		switch mutation {
		case "versions":
			candidate.Versions = candidate.Versions[:4]
		case "uri":
			candidate.Versions[0].URI += "/wrong"
		case "schema":
			candidate.Schema = 68
		case "custody":
			candidate.CustodyID = uuid.New()
		case "path":
			stage = "activations"
		}
		if cli(candidate, stage) == nil || providerCalls.Load() != requests {
			t.Fatal("unqualified continuation performed provider I/O", mutation)
		}
	}
	if err := cli(in, "resume-observations"); err != nil {
		t.Fatal("actual protected continuation CLI", err)
	}
	after, err := snapshot(ctx, pool, before)
	if err != nil || providerCalls.Load()-requests != 10 {
		t.Fatal("continuation must perform exactly five controlled HEAD/GET pairs", err)
	}
	assertLedgerPreserved(t, before, after)
	pythonCompareRealProbe(t, cfg, in, before, after, folder)
	countCosts := func() int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_event ce JOIN cost_allocation ca ON ca.cost_event_id=ce.id JOIN lifecycle_storage_call c ON c.id=ca.lifecycle_storage_call_id WHERE c.workspace_id=$1 AND c.operation IN ('PROTECTED_HEAD','PROTECTED_GET') AND ce.activity_id=c.id AND ce.quantity=1 AND ce.unit='invocation'`, cfg.Workspace).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	// HTTP 200/hash mismatch and denied HTTP calls both retain a real physical
	// attempt's cost. No retry, re-copy, STS request or host launch is possible.
	for _, failureMode := range []int32{2, 3} {
		mode.Store(failureMode)
		priorCosts, priorRequests := countCosts(), providerCalls.Load()
		if cli(in, "resume-observations") == nil {
			t.Fatal("failed/hash-mismatching provider admitted")
		}
		expectedCalls := int64(1) // A denied HEAD prevents the GET.
		if failureMode == 2 {
			expectedCalls = 2 // Successful HEAD followed by a mismatching GET.
		}
		if providerCalls.Load()-priorRequests != expectedCalls || int64(countCosts()-priorCosts) != expectedCalls {
			t.Fatal("failed physical attempt lost cost or repeated I/O")
		}
	}
	mode.Store(1)
	files, _ := filepath.Glob(filepath.Join(folder, "resume-cli-*"))
	for _, path := range files {
		data, _ := os.ReadFile(path)
		for _, secret := range []string{"controlled-secret", "controlled-finite-session", "postgres://", string(body)} {
			if strings.Contains(string(data), secret) {
				t.Fatal("continuation output leaked private input")
			}
		}
	}
}

func pythonCompareRealProbe(t *testing.T, cfg config, in input, before, after inventory, folder string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		python, err = exec.LookPath("python")
	}
	if err != nil {
		t.Fatal("Python required for real SQL/SDK/controller ledger contract")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
	state := map[string]any{"runId": strings.TrimPrefix(cfg.Profile.Subject, "trial-operator-"), "workspaceId": cfg.Workspace, "actorId": cfg.Actor}
	wrap := func(value inventory) map[string]any {
		return map[string]any{"contract": "issue325/protected-resume-v1", "workspaceId": cfg.Workspace,
			"identity": map[string]any{"runId": state["runId"], "workspaceId": cfg.Workspace, "actorId": cfg.Actor, "principalRef": cfg.Profile.Subject},
			"expected": map[string]any{"schema": in.Schema, "schemaVersions": in.SchemaVersions, "versions": in.Versions}, "source": map[string]any{}, "inventory": value}
	}
	inputPath := filepath.Join(folder, "python-ledger-input.json")
	value, _ := json.Marshal(map[string]any{"state": state, "before": wrap(before), "after": wrap(after)})
	if err := os.WriteFile(inputPath, value, 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(python, "-B", "-c", `import json,sys;from pathlib import Path;sys.path.insert(0,str(Path(sys.argv[1])/'deploy/trial'));import protected_resume as p;v=json.loads(Path(sys.argv[2]).read_text(encoding='utf-8'));proof=p.compare(v['state'],v['before'],v['after'],probe=True);assert len(proof['callIds'])==10`, root, inputPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("real SDK/PostgreSQL ledger not admitted by controller: %v\n%s", err, output)
	}
}
