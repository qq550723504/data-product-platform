// trial-activate inspects retained facts or verifies already-bound versions.
// It has no seed, migration, STS issuance, IAM or legacy/root storage client.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
)

type table struct {
	Columns []string `json:"columns"`
	Rows    []string `json:"rows"`
}
type inventory struct {
	Tables map[string]table `json:"tables"`
}
type expectedVersion struct {
	ID     uuid.UUID `json:"id"`
	SHA256 string    `json:"sha256"`
	Bytes  int64     `json:"bytes"`
	URI    string    `json:"uri"`
}
type input struct {
	ConfigFile     string            `json:"configFile"`
	Baseline       string            `json:"baseline"`
	Profile        string            `json:"profile"`
	Versions       []expectedVersion `json:"versions"`
	Schema         int               `json:"schema"`
	SchemaVersions []int             `json:"schemaVersions"`
	CustodyID      uuid.UUID         `json:"custodyId"`
	CustodyPlan    *lc.CustodyPlan   `json:"custodyPlan"`
	ManifestSHA256 string            `json:"manifestSha256"`
}
type config struct {
	DSN, Private     string
	Workspace, Actor uuid.UUID
	Profile          struct{ Subject, StorageEndpoint, Bucket string }
}

func load(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 64<<20 || json.Unmarshal(b, dst) != nil {
		return fmt.Errorf("private activation input unavailable")
	}
	return nil
}

// Canonical JSON strings preserve numeric precision. A post-DDL snapshot uses
// exactly the original column projection, so added columns do not erase facts.
func snapshot(ctx context.Context, pool *pgxpool.Pool, baseline inventory) (inventory, error) {
	result := inventory{Tables: map[string]table{}}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`)
	if err != nil {
		return result, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return result, err
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(names) > 512 {
		return result, fmt.Errorf("bounded public inventory required")
	}
	total := 0
	for _, name := range names {
		var columns []string
		if err = tx.QueryRow(ctx, `SELECT array_agg(column_name::text ORDER BY ordinal_position) FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, name).Scan(&columns); err != nil {
			return result, err
		}
		if old, ok := baseline.Tables[name]; ok {
			for _, column := range old.Columns {
				found := false
				for _, actual := range columns {
					found = found || column == actual
				}
				if !found {
					return result, fmt.Errorf("original column missing")
				}
			}
			columns = old.Columns
		}
		if len(columns) == 0 {
			return result, fmt.Errorf("original columns required")
		}
		quoted := make([]string, len(columns))
		for i, column := range columns {
			quoted[i] = pgx.Identifier{column}.Sanitize()
		}
		query := `SELECT to_jsonb(x)::text FROM (SELECT ` + strings.Join(quoted, ",") + ` FROM ` + pgx.Identifier{"public", name}.Sanitize() + `) x ORDER BY to_jsonb(x)::text`
		data, err := tx.Query(ctx, query)
		if err != nil {
			return result, err
		}
		t := table{Columns: columns, Rows: []string{}}
		for data.Next() {
			var row string
			if err = data.Scan(&row); err != nil {
				data.Close()
				return result, err
			}
			total += len(row)
			if total > 64<<20 || len(t.Rows) >= 100000 {
				data.Close()
				return result, fmt.Errorf("inventory exceeds synthetic trial bound")
			}
			t.Rows = append(t.Rows, row)
		}
		err = data.Err()
		data.Close()
		if err != nil {
			return result, err
		}
		result.Tables[name] = t
	}
	for name := range baseline.Tables {
		if _, ok := result.Tables[name]; !ok {
			return result, fmt.Errorf("original table missing")
		}
	}
	return result, tx.Commit(ctx)
}

func verify(ctx context.Context, pool *pgxpool.Pool, c config, in input) (any, error) {
	var profile pilot.WorkspaceObjectProfile
	if load(in.Profile, &profile) != nil || profile.Scope.WorkspaceID != c.Workspace || profile.Scope.ActorID != c.Actor ||
		profile.Scope.PrincipalRef != c.Profile.Subject || profile.Scope.Bucket != "raw" || profile.Endpoint != c.Profile.StorageEndpoint || profile.Secure ||
		profile.Scope.NamespaceID != uuid.NewSHA1(c.Workspace, []byte("core-object-namespace")) ||
		profile.Scope.ProviderInstanceID != uuid.NewSHA1(uuid.NameSpaceURL, []byte(c.Private+"/objects")) ||
		profile.Scope.IncarnationID != uuid.NewSHA1(uuid.NameSpaceURL, []byte(c.Private+"/objects/data")) || len(in.Versions) != 5 {
		return nil, fmt.Errorf("same protected composition and five versions required")
	}
	seen := map[uuid.UUID]bool{}
	for _, v := range in.Versions {
		hash, err := hex.DecodeString(v.SHA256)
		if v.ID == uuid.Nil || seen[v.ID] || v.Bytes <= 0 || v.Bytes > 128<<20 || err != nil || len(hash) != 32 || strings.ToLower(v.SHA256) != v.SHA256 {
			return nil, fmt.Errorf("exact version facts required")
		}
		seen[v.ID] = true
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		return nil, err
	}
	outbox.ConfigureAppendObligation(router)
	service, err := pilot.BuildWorkspaceObjects(pool, in.Profile)
	if err != nil {
		return nil, err
	}
	for _, v := range in.Versions {
		reader, err := service.ReadVersion(ctx, v.ID)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(reader, (128<<20)+1))
		closeErr := reader.Close()
		h := sha256.Sum256(data)
		if err != nil || closeErr != nil || int64(len(data)) != v.Bytes || hex.EncodeToString(h[:]) != v.SHA256 {
			return nil, fmt.Errorf("same version content proof failed")
		}
	}
	return map[string]any{"status": "PASS", "verifiedVersions": 5, "runtimeRootFallback": false}, nil
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "snapshot" && os.Args[1] != "verify" && os.Args[1] != "admit" && os.Args[1] != "private") {
		return fmt.Errorf("snapshot|verify|admit|private required")
	}
	var in input
	var c config
	if load(os.Getenv("DPP_TRIAL_ACTIVATION_INPUT"), &in) != nil || load(in.ConfigFile, &c) != nil {
		return fmt.Errorf("private activation configuration required")
	}
	inputPath, err := filepath.Abs(os.Getenv("DPP_TRIAL_ACTIVATION_INPUT"))
	configPath, configErr := filepath.Abs(in.ConfigFile)
	rel, relErr := filepath.Rel(c.Private, inputPath)
	parts := strings.Split(rel, string(filepath.Separator))
	if err != nil || configErr != nil || relErr != nil || rel == "." || strings.HasPrefix(rel, "..") || configPath != filepath.Join(c.Private, "config.json") {
		return fmt.Errorf("same private activation paths required")
	}
	if len(parts) < 3 || (parts[0] != "activations" && parts[0] != "bootstrap-attempts" && parts[0] != "custody-attempts") ||
		(os.Args[1] == "verify" && parts[0] != "activations") {
		return fmt.Errorf("explicit retained stage path required")
	}
	// Reuse the operator custody private-path/ACL primitive before any SQL or
	// backup. Native metadata and old content hashes do not attest current ACLs.
	if pilot.CheckCustodyPrivatePath(c.Private, true) != nil || pilot.CheckCustodyPrivatePath(filepath.Dir(inputPath), true) != nil ||
		pilot.CheckCustodyPrivatePath(inputPath, false) != nil || pilot.CheckCustodyPrivatePath(configPath, false) != nil {
		return fmt.Errorf("same operator-private stage paths required")
	}
	if os.Args[1] == "private" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "PASS", "privateACL": true})
	}
	u, err := url.Parse(c.DSN)
	if err != nil || u.Scheme != "postgres" || u.Hostname() != "127.0.0.1" || u.Path != "/dpp_trial_core" || c.Workspace == uuid.Nil || c.Actor == uuid.Nil ||
		!filepath.IsAbs(c.Private) || c.Profile.Bucket != "raw" || c.Profile.Subject == "" {
		return fmt.Errorf("same isolated Core required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, c.DSN)
	if err != nil {
		return fmt.Errorf("Core unavailable")
	}
	defer pool.Close()
	var result any
	if os.Args[1] == "snapshot" {
		var before inventory
		if in.Baseline != "" && load(in.Baseline, &before) != nil {
			return fmt.Errorf("baseline unavailable")
		}
		result, err = snapshot(ctx, pool, before)
	} else if os.Args[1] == "admit" {
		result, err = admit(ctx, pool, c, in)
	} else {
		result, err = verify(ctx, pool, c, in)
	}
	if err != nil {
		return fmt.Errorf("activation inspection/verification refused; retain attempt")
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
