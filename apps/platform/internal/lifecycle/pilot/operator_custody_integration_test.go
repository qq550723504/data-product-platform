package pilot_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

func custodyDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN absent: custody E2E NOT_RUN")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "custody_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q TEMPLATE template0`, name)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE %q WITH (FORCE)`, name)); err != nil {
			t.Error("drop owned custody scratch database", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	if err = migration.NewRunner(pool, filepath.Join(filepath.Dir(file), "../../../../../migrations")).Up(ctx); err != nil {
		t.Fatal(err)
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	t.Cleanup(func() { outbox.ConfigureAppendObligation(nil) })
	return pool
}

func TestOperatorCustodyRealSDKCoreAndPrivateFilesReplay(t *testing.T) {
	pool := custodyDatabase(t)
	ctx := context.Background()
	body := []byte("id,value\n1,unchanged\n")
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.RawQuery != "" || (r.URL.Path != "/raw/datasets/first.csv" && r.URL.Path != "/raw/datasets/second.csv") {
			t.Errorf("unexpected custody invocation %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("ETag", `"not-a-hash-proof"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		// Same exact-DACL primitive as examples/temporary_containers.py, scoped
		// exclusively to this test-owned directory; production never repairs ACLs.
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `
$ErrorActionPreference='Stop'
$acl=[Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true,$false)
foreach($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User,[Security.Principal.SecurityIdentifier]::new('S-1-5-18'))){
 $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow'))
}
[IO.Directory]::SetAccessControl($env:DPP_CUSTODY_TEST_DIRECTORY,$acl)`)
		cmd.Env = append(os.Environ(), "DPP_CUSTODY_TEST_DIRECTORY="+dir)
		if err := cmd.Run(); err != nil {
			t.Fatal("private test ACL provisioning", err)
		}
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := pilot.CheckCustodyPrivatePath(dir, true); err != nil {
		t.Fatal("owned temporary directory ACL is not private", err)
	}
	p := lc.CustodyPlan{WorkspaceID: uuid.New(), ActorID: uuid.New(), PrincipalRef: "synthetic-private-operator", RunID: "retained-synthetic-run", ProviderWitness: "controlled-loopback-SDK-contract", Endpoint: strings.TrimPrefix(server.URL, "http://"), OutputDirectory: dir, PlanSHA256: strings.Repeat("a", 64), ExpiresAt: time.Now().Add(20 * time.Minute).UTC().Truncate(time.Microsecond),
		First: lc.CustodyTarget{VersionID: uuid.New(), URI: "s3://raw/datasets/first.csv", SHA256: hash, Bytes: int64(len(body))}, Second: lc.CustodyTarget{VersionID: uuid.New(), URI: "s3://raw/datasets/second.csv", SHA256: hash, Bytes: int64(len(body))}}
	for i, target := range []lc.CustodyTarget{p.First, p.Second} {
		dataset := uuid.New()
		kind := []string{"STANDARDIZED", "CURATED"}[i]
		if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'Synthetic custody E2E',$4)`, dataset, p.WorkspaceID, uuid.NewString(), kind); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size) VALUES($1,$2,1,'READY','OBJECT_STORAGE',$3,'SHA256',$4,$5)`, target.VersionID, dataset, target.URI, hash, target.Bytes); err != nil {
			t.Fatal(err)
		}
	}
	a, err := storage.NewOperatorCustodyAdapter(p, credentials.NewStaticV4("synthetic-access", "synthetic-secret", ""))
	if err != nil {
		t.Fatal(err)
	}
	s, _ := lc.NewOperatorCustody(pool, p)
	manifestHash, err := pilot.ExportOperatorCustody(ctx, s, p, "export-pair", a)
	if err != nil {
		t.Fatal("SDK/Core/private export", err)
	}
	if calls != 2 || len(manifestHash) != 64 {
		t.Fatal("finite export missing", calls)
	}
	var observations, costs, accepted, versions int
	for query, dest := range map[string]*int{
		`SELECT count(*) FROM operator_custody_observation WHERE failure='' AND eof AND close_ok`: &observations,
		`SELECT count(*) FROM cost_allocation WHERE operator_custody_call_id IS NOT NULL`:         &costs,
		`SELECT count(*) FROM operator_custody_termination WHERE outcome='ACCEPTED'`:              &accepted,
		`SELECT count(*) FROM dataset_version`:                                                    &versions,
	} {
		if err = pool.QueryRow(ctx, query).Scan(dest); err != nil {
			t.Fatal(err)
		}
	}
	if observations != 2 || costs != 2 || accepted != 1 || versions != 2 {
		t.Fatalf("facts %d/%d/%d/%d", observations, costs, accepted, versions)
	}
	replay, _ := lc.NewOperatorCustody(pool, p)
	again, err := pilot.ExportOperatorCustody(ctx, replay, p, "export-pair", a)
	if err != nil || again != manifestHash || calls != 2 {
		t.Fatal("same-key replay invoked provider or changed receipt", err, calls)
	}
	// The completed receipt cannot mask local corruption, and a refused replay
	// still performs zero provider I/O. Files and immutable Core facts survive.
	if err = os.WriteFile(filepath.Join(dir, p.First.VersionID.String()+".csv"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	replay, _ = lc.NewOperatorCustody(pool, p)
	if _, err = pilot.ExportOperatorCustody(ctx, replay, p, "export-pair", a); err == nil || calls != 2 {
		t.Fatal("tampered replay accepted or fetched replacement")
	}
}

func TestOperatorCustodyPrivatePathRejectsLinkAndPublicDirectory(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if pilot.CheckCustodyPrivatePath(dir, true) == nil {
			t.Fatal("public output accepted")
		}
	}
	if err := pilot.CheckCustodyPrivatePath("relative/output", true); err == nil {
		t.Fatal("relative output accepted")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err == nil {
		if pilot.CheckCustodyPrivatePath(link, true) == nil {
			t.Fatal("symlink output accepted")
		}
	}
}
