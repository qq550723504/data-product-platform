package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
)

func cliPlan(base, endpoint string, body []byte) lc.CustodyPlan {
	run := strings.ReplaceAll(uuid.NewString(), "-", "")
	hash := sha256.Sum256(body)
	return lc.CustodyPlan{WorkspaceID: uuid.New(), ActorID: uuid.New(), RunID: run, PrincipalRef: "trial-operator-" + run, Endpoint: endpoint,
		ProviderWitness: "isolated-CLI-loopback", OutputDirectory: filepath.Join(base, "dpp-trial-"+run, "custody", "one"), PlanSHA256: strings.Repeat("a", 64), ExpiresAt: time.Now().UTC().Add(20 * time.Minute).Truncate(time.Microsecond),
		First:  lc.CustodyTarget{VersionID: uuid.New(), URI: "s3://raw/datasets/standardized.csv", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(body))},
		Second: lc.CustodyTarget{VersionID: uuid.New(), URI: "s3://raw/datasets/curated.csv", SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(body))}}
}

// Literal shape produced by runtime.provision_config; do not serialize the
// decoder's own DTO, which would conceal another producer/consumer mismatch.
func retainedConfig(p lc.CustodyPlan, dsn string) map[string]any {
	private := filepath.Dir(filepath.Dir(p.OutputDirectory))
	return map[string]any{"DSN": dsn, "Private": private, "Repo": "synthetic-source", "RootKey": "controlled-native-root", "RootSecret": "controlled-native-secret",
		"Workspace": p.WorkspaceID.String(), "Actor": p.ActorID.String(), "Profile": map[string]any{
			"Enabled": true, "Subject": p.PrincipalRef, "Database": "dpp_ingestion_source", "Bucket": "raw", "StorageEndpoint": p.Endpoint,
			"CoreCredentialFile": filepath.Join(private, "core-identity.json"), "PublisherCredentialFile": filepath.Join(private, "publisher-identity.json")}}
}

func TestNativeCompositionUsesRetainedNestedProfileAndExactIdentity(t *testing.T) {
	p := cliPlan(t.TempDir(), "127.0.0.1:9000", []byte("unchanged"))
	dsn := "postgres://synthetic@127.0.0.1:5432/retained?sslmode=disable"
	file := filepath.Join(filepath.Dir(filepath.Dir(p.OutputDirectory)), "config.json")
	for _, mode := range []string{"current", "root-endpoint-only", "workspace", "actor", "principal", "bucket", "tls", "endpoint", "dsn", "run", "outside-output", "moved-config"} {
		t.Run(mode, func(t *testing.T) {
			c := retainedConfig(p, dsn)
			profile := c["Profile"].(map[string]any)
			plan := p
			path := file
			actualDSN := dsn
			switch mode {
			case "root-endpoint-only":
				delete(profile, "StorageEndpoint")
				c["StorageEndpoint"] = p.Endpoint
			case "workspace":
				c["Workspace"] = uuid.NewString()
			case "actor":
				c["Actor"] = uuid.NewString()
			case "principal":
				profile["Subject"] = "foreign-principal"
			case "bucket":
				profile["Bucket"] = "foreign"
			case "tls":
				profile["StorageTLS"] = true
			case "endpoint":
				profile["StorageEndpoint"] = "127.0.0.1:9001"
			case "dsn":
				actualDSN = dsn + "&application_name=foreign"
			case "run":
				plan.RunID = "foreign-run"
			case "outside-output":
				plan.OutputDirectory = filepath.Join(t.TempDir(), "outside")
			case "moved-config":
				path = filepath.Join(t.TempDir(), "config.json")
			}
			b, _ := json.Marshal(c)
			native, err := nativeComposition(b, plan, path, actualDSN)
			if mode == "current" {
				if err != nil || native.Profile.StorageEndpoint != p.Endpoint {
					t.Fatal("current retained configuration refused", err)
				}
			} else if err == nil {
				t.Fatal("foreign composition accepted", mode)
			}
			if err != nil && (strings.Contains(err.Error(), "controlled-native") || strings.Contains(err.Error(), "postgres://")) {
				t.Fatal("private configuration leaked")
			}
		})
	}
}

func privateCLIDirectory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		// Existing temporary_containers.py exact-DACL primitive, test-owned path.
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `
$ErrorActionPreference='Stop'
$acl=[Security.AccessControl.DirectorySecurity]::new()
$acl.SetAccessRuleProtection($true,$false)
foreach($sid in @([Security.Principal.WindowsIdentity]::GetCurrent().User,[Security.Principal.SecurityIdentifier]::new('S-1-5-18'))){
 $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow'))
}
[IO.Directory]::SetAccessControl($env:DPP_CUSTODY_TEST_DIRECTORY,$acl)`)
		cmd.Env = append(os.Environ(), "DPP_CUSTODY_TEST_DIRECTORY="+dir)
		if cmd.Run() != nil {
			t.Fatal("private test ACL unavailable")
		}
	}
	if os.Chmod(dir, 0700) != nil || pilot.CheckCustodyPrivatePath(dir, true) != nil {
		t.Fatal("private test directory unavailable")
	}
	return dir
}

func cliDatabase(t *testing.T) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN absent: real custody CLI PostgreSQL NOT_RUN")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("private test database configuration unavailable")
	}
	t.Cleanup(admin.Close)
	name := "custody_cli_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q TEMPLATE template0`, name)); err != nil {
		t.Fatal("create owned custody test database", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE %q WITH (FORCE)`, name)); err != nil {
			t.Error("drop owned custody test database", err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse owned test connection")
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open owned test connection")
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	if err = migration.NewRunner(pool, filepath.Join(filepath.Dir(file), "../../../../migrations")).Up(ctx); err != nil {
		t.Fatal(err)
	}
	connection, err := url.Parse(dsn)
	if err != nil || (connection.Scheme != "postgres" && connection.Scheme != "postgresql") {
		t.Fatal("owned test URL required")
	}
	connection.Path = "/" + name
	return pool, connection.String()
}

func TestCLIExistingNativeConfigTwoGETsTypedFactsAndZeroIOReplay(t *testing.T) {
	pool, dsn := cliDatabase(t)
	ctx := context.Background()
	body := []byte("id,value\n1,unchanged\n")
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "GET" || r.URL.RawQuery != "" || r.Header.Get("Range") != "" ||
			(r.URL.Path != "/raw/datasets/standardized.csv" && r.URL.Path != "/raw/datasets/curated.csv") || !strings.Contains(r.Header.Get("Authorization"), "Credential=controlled-native-root/") {
			t.Errorf("unexpected CLI SDK request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("ETag", `"not-content-proof"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		_, _ = w.Write(body)
	}))
	defer server.Close()
	p := cliPlan(privateCLIDirectory(t), strings.TrimPrefix(server.URL, "http://"), body)
	if err := os.MkdirAll(p.OutputDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	for i, target := range []lc.CustodyTarget{p.First, p.Second} {
		dataset := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'Synthetic CLI custody',$4)`, dataset, p.WorkspaceID, uuid.NewString(), []string{"STANDARDIZED", "CURATED"}[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size) VALUES($1,$2,1,'READY','OBJECT_STORAGE',$3,'SHA256',$4,$5)`, target.VersionID, dataset, target.URI, target.SHA256, target.Bytes); err != nil {
			t.Fatal(err)
		}
	}
	private := filepath.Dir(filepath.Dir(p.OutputDirectory))
	configFile := filepath.Join(private, "config.json")
	b, _ := json.Marshal(retainedConfig(p, dsn))
	if err := os.WriteFile(configFile, b, 0600); err != nil {
		t.Fatal(err)
	}
	credentialHash := sha256.Sum256(b)
	profile := filepath.Join(private, "operator-custody-profile.json")
	b, _ = json.Marshal(map[string]any{"plan": p, "credentialFile": configFile, "credentialFileSha256": hex.EncodeToString(credentialHash[:])})
	if err := os.WriteFile(profile, b, 0600); err != nil {
		t.Fatal(err)
	}
	profileHash := sha256.Sum256(b)
	digest := hex.EncodeToString(profileHash[:])
	t.Setenv("POSTGRES_DSN", dsn)
	if err := run(profile, digest, "cli-pair"); err != nil {
		t.Fatal("actual CLI run rejected retained producer shape", err)
	}
	if calls.Load() != 2 {
		t.Fatal("CLI exact GET budget", calls.Load())
	}
	if err := run(profile, digest, "cli-pair"); err != nil || calls.Load() != 2 {
		t.Fatal("actual CLI replay performed provider work", err)
	}
	for query, want := range map[string]int{
		`SELECT count(*) FROM operator_custody_termination WHERE outcome='ACCEPTED'`:                                         1,
		`SELECT count(*) FROM operator_custody_observation WHERE failure='' AND eof AND close_ok`:                            2,
		`SELECT count(*) FROM cost_allocation WHERE operator_custody_call_id IS NOT NULL`:                                    2,
		`SELECT count(*) FROM cost_event WHERE cost_type='OPERATOR_CUSTODY_INVOCATION' AND quantity=1 AND unit='invocation'`: 2,
		`SELECT count(*) FROM audit_event WHERE action='OperatorCustodyCallObserved'`:                                        2,
		`SELECT count(*) FROM outbox_event WHERE event_type='OperatorCustodyCallObserved'`:                                   2,
		`SELECT count(*) FROM evidence WHERE title='OperatorCustodyCallObserved'`:                                            2,
		`SELECT count(*) FROM dataset_version`:                                                                               2,
	} {
		var n int
		if err := pool.QueryRow(ctx, query).Scan(&n); err != nil || n != want {
			t.Fatalf("CLI atomic facts count=%d want=%d error=%v", n, want, err)
		}
	}
	// Same pinned profile cannot be redirected to another Core connection.
	t.Setenv("POSTGRES_DSN", dsn+" application_name=foreign")
	if err := run(profile, digest, "foreign-core"); err == nil || calls.Load() != 2 {
		t.Fatal("foreign database accepted or provider called")
	}
}
