package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

func ledgerDatabase(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("isolated TEST_POSTGRES_DSN required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("owned test PostgreSQL unavailable")
	}
	// This exact name exercises the real retained CLI constraint. CREATE must
	// succeed before cleanup is registered; never adopt or drop a preexisting DB.
	if _, err = admin.Exec(ctx, `CREATE DATABASE dpp_trial_core TEMPLATE template0`); err != nil {
		admin.Close()
		t.Fatal("fresh isolated CLI database name unavailable", err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec(context.Background(), `DROP DATABASE dpp_trial_core`)
		admin.Close()
		if err != nil {
			t.Error("drop own ledger scratch database", err)
		}
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("test connection URL required")
	}
	parsed.Path = "/dpp_trial_core"
	if parsed.Hostname() == "localhost" {
		parsed.Host = "127.0.0.1:" + parsed.Port()
	}
	pool, err := pgxpool.New(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, parsed.String()
}

func ledgerPrivate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'
$acl=[Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true,$false)
foreach($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User,[Security.Principal.SecurityIdentifier]::new('S-1-5-18'))){$acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow'))}
[IO.Directory]::SetAccessControl($env:DPP_LEDGER_TEST_DIRECTORY,$acl)`)
		cmd.Env = append(os.Environ(), "DPP_LEDGER_TEST_DIRECTORY="+root)
		if cmd.Run() != nil {
			t.Fatal("private test ACL unavailable")
		}
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func stageLedgerMigrations(t *testing.T, ceiling int) (string, []int) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(file), "../../../../migrations")
	dir := t.TempDir()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	versions := []int{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		number, err := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if err != nil {
			t.Fatal(err)
		}
		if number > ceiling {
			continue
		}
		b, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, entry.Name()), b, 0600); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, number)
	}
	sort.Ints(versions)
	return dir, versions
}

func assertLedgerPreserved(t *testing.T, before, after inventory) {
	t.Helper()
	for name, old := range before.Tables {
		current, ok := after.Tables[name]
		if !ok || !reflect.DeepEqual(old.Columns, current.Columns) {
			t.Fatal("old table or column lost", name)
		}
		counts := map[string]int{}
		for _, row := range current.Rows {
			counts[row]++
		}
		for _, row := range old.Rows {
			counts[row]--
			if counts[row] < 0 {
				t.Fatal("old logical row changed/lost", name)
			}
		}
	}
}

func TestLedgerBootstrap68To71PreservesFiveVersionsAndRealCLICustodyAdmission(t *testing.T) {
	pool, dsn := ledgerDatabase(t)
	ctx := context.Background()
	dir, history68 := stageLedgerMigrations(t, 68)
	if err := migration.NewRunner(pool, dir).Up(ctx); err != nil {
		t.Fatal(err)
	}
	body := []byte("id,value\n1,retained\n")
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	runID := strings.ReplaceAll(uuid.NewString(), "-", "")
	private := filepath.Join(ledgerPrivate(t), "dpp-trial-"+runID)
	if err := os.MkdirAll(private, 0700); err != nil {
		t.Fatal(err)
	}
	ws, actor := uuid.New(), uuid.New()
	versions := []expectedVersion{}
	for i, kind := range []string{"RAW", "RAW", "RAW", "STANDARDIZED", "CURATED"} {
		dataset, version := uuid.New(), uuid.New()
		uri := "s3://raw/datasets/" + strconv.Itoa(i) + ".csv"
		if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'Isolated retained five',$4)`, dataset, ws, uuid.NewString(), kind); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size) VALUES($1,$2,1,'READY','OBJECT_STORAGE',$3,'SHA256',$4,$5)`, version, dataset, uri, checksum, len(body)); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, expectedVersion{ID: version, SHA256: checksum, Bytes: int64(len(body)), URI: uri})
	}
	before, err := snapshot(ctx, pool, inventory{})
	if err != nil {
		t.Fatal(err)
	}
	var providerCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		if r.Method != "GET" || r.URL.RawQuery != "" || (r.URL.Path != "/raw/datasets/3.csv" && r.URL.Path != "/raw/datasets/4.csv") {
			t.Errorf("unexpected bounded custody request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("ETag", `"controlled"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	p := lc.CustodyPlan{WorkspaceID: ws, ActorID: actor, RunID: runID, PrincipalRef: "trial-operator-" + runID, ProviderWitness: "isolated-same-native-composition", Endpoint: strings.TrimPrefix(server.URL, "http://"), OutputDirectory: filepath.Join(private, "output"), PlanSHA256: strings.Repeat("a", 64), ExpiresAt: time.Now().UTC().Add(20 * time.Minute).Truncate(time.Microsecond).Add(789 * time.Nanosecond),
		First: lc.CustodyTarget{VersionID: versions[3].ID, URI: versions[3].URI, SHA256: checksum, Bytes: int64(len(body))}, Second: lc.CustodyTarget{VersionID: versions[4].ID, URI: versions[4].URI, SHA256: checksum, Bytes: int64(len(body))}}
	if err = os.Mkdir(p.OutputDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config{DSN: dsn, Private: private, Workspace: ws, Actor: actor}
	cfg.Profile.Subject = p.PrincipalRef
	cfg.Profile.Bucket = "raw"
	cfg.Profile.StorageEndpoint = p.Endpoint
	configPath := filepath.Join(private, "config.json")
	data, _ := json.Marshal(cfg)
	if err = os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	attempt := filepath.Join(private, "bootstrap-attempts", "isolated")
	if err = os.MkdirAll(attempt, 0700); err != nil {
		t.Fatal(err)
	}
	in := input{ConfigFile: configPath, Versions: versions, Schema: 68, SchemaVersions: history68}
	cli := func(in input) error {
		t.Helper()
		value, _ := json.Marshal(in)
		path := filepath.Join(attempt, uuid.NewString()+".json")
		if err := os.WriteFile(path, value, 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DPP_TRIAL_ACTIVATION_INPUT", path)
		previousArgs, previousOut := os.Args, os.Stdout
		output, err := os.CreateTemp(private, "sql-admission-")
		if err != nil {
			t.Fatal(err)
		}
		os.Args = []string{"trial-activate", "admit"}
		os.Stdout = output
		defer func() { os.Args = previousArgs; os.Stdout = previousOut; _ = output.Close() }()
		return run()
	}
	if err = cli(in); err != nil {
		t.Fatal("actual schema68 admission", err)
	}
	for _, ceiling := range []int{69, 70, 71} {
		dir, history := stageLedgerMigrations(t, ceiling)
		if err = migration.NewRunner(pool, dir).Up(ctx); err != nil {
			t.Fatal(err)
		}
		next := in
		next.Schema = 71
		_, next.SchemaVersions = stageLedgerMigrations(t, 71)
		if ceiling < 71 {
			if _, err = admit(ctx, pool, cfg, in); err == nil {
				t.Fatal("partial migration treated as fresh68")
			}
			if _, err = admit(ctx, pool, cfg, next); err == nil {
				t.Fatal("partial migration treated as71")
			}
		} else {
			in = next
			if err = cli(in); err != nil {
				t.Fatal("actual schema71 admission", err)
			}
			if !reflect.DeepEqual(history, in.SchemaVersions) {
				t.Fatal("pinned migration manifest mismatch")
			}
		}
		after, e := snapshot(ctx, pool, before)
		if e != nil {
			t.Fatal(e)
		}
		assertLedgerPreserved(t, before, after)
	}
	in.CustodyID = uuid.New()
	in.CustodyPlan = &p
	in.ManifestSHA256 = strings.Repeat("b", 64)
	if err = cli(in); err == nil || providerCalls.Load() != 0 {
		t.Fatal("local-only/unaccepted custody admitted or provider requested")
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	service, err := lc.NewOperatorCustody(pool, p)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := storage.NewOperatorCustodyAdapter(p, credentials.NewStaticV4("controlled-root", "controlled-private-secret", ""))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := pilot.ExportOperatorCustody(ctx, service, p, "isolated-pair", adapter)
	if err != nil {
		t.Fatal("controlled SDK custody", err)
	}
	if providerCalls.Load() != 2 {
		t.Fatal("bounded custody GET budget", providerCalls.Load())
	}
	in.CustodyID = service.ID()
	in.ManifestSHA256 = hash
	if err = cli(in); err != nil {
		t.Fatal("actual accepted Core custody CLI admission", err)
	}
	beforeReads, err := snapshot(ctx, pool, inventory{})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"workspace", "actor", "manifest", "missing-version", "history", "custody-id", "expiry-nanos"} {
		bad := in
		plan := p
		bad.CustodyPlan = &plan
		switch mode {
		case "workspace":
			plan.WorkspaceID = uuid.New()
		case "actor":
			plan.ActorID = uuid.New()
		case "expiry-nanos":
			plan.ExpiresAt = plan.ExpiresAt.Add(time.Nanosecond)
		case "manifest":
			bad.ManifestSHA256 = strings.Repeat("c", 64)
		case "missing-version":
			bad.Versions = bad.Versions[:4]
		case "history":
			bad.SchemaVersions = bad.SchemaVersions[1:]
		case "custody-id":
			bad.CustodyID = uuid.New()
		}
		if err = cli(bad); err == nil {
			t.Fatal("foreign admission accepted", mode)
		}
	}
	afterReads, err := snapshot(ctx, pool, inventory{})
	if err != nil || !reflect.DeepEqual(beforeReads, afterReads) || providerCalls.Load() != 2 {
		t.Fatal("SQL admission wrote facts or called provider", err)
	}
	dir, _ = stageLedgerMigrations(t, 71)
	if err = migration.NewRunner(pool, dir).Up(ctx); err != nil {
		t.Fatal("canonical71 replay", err)
	}
	after, err := snapshot(ctx, pool, before)
	if err != nil {
		t.Fatal(err)
	}
	assertLedgerPreserved(t, before, after)
	// Helper output contains only metadata, never payload or provider credentials.
	files, _ := filepath.Glob(filepath.Join(private, "sql-admission-*"))
	for _, f := range files {
		reader, _ := os.Open(f)
		b, _ := io.ReadAll(reader)
		_ = reader.Close()
		if strings.Contains(string(b), "controlled-private-secret") || strings.Contains(string(b), "postgres://") || strings.Contains(string(b), string(body)) {
			t.Fatal("private admission output leaked")
		}
	}
}
