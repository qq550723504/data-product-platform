// trial-prepare provisions only the fixed synthetic trial prerequisites.
// Business ingestion is performed by the existing authenticated /batches flow.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/pilot"
	lifecycleapp "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	lifecyclepilot "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourcedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

type config struct {
	DSN, Private, Repo, RootKey, RootSecret string
	Workspace, Actor                        uuid.UUID
	Profile                                 pilot.Profile
}

func write(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func validate(c config) error {
	u, err := url.Parse(c.DSN)
	if err != nil || u.Scheme != "postgres" || u.Hostname() != "127.0.0.1" || u.Path != "/dpp_trial_core" ||
		c.Workspace == uuid.Nil || c.Actor == uuid.Nil || !filepath.IsAbs(c.Private) ||
		!strings.HasPrefix(filepath.Base(c.Private), "dpp-trial-") || c.Profile.Database != "dpp_ingestion_source" ||
		c.Profile.Reader != "dpp_reader" || c.Profile.Bucket != "raw" || len(c.Profile.Tables) != 3 {
		return fmt.Errorf("isolated synthetic trial configuration required")
	}
	return nil
}

func prepare(ctx context.Context, pool *pgxpool.Pool, c config) error {
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_tables WHERE schemaname='public'`).Scan(&n); err != nil || n != 0 {
		return fmt.Errorf("prepare requires a fresh dedicated Core; never retry uncertain bootstrap")
	}
	if err := migration.NewRunner(pool, filepath.Join(c.Repo, "migrations")).Up(ctx); err != nil {
		return fmt.Errorf("migrations failed")
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		return fmt.Errorf("routing configuration failed")
	}
	outbox.ConfigureAppendObligation(router)
	p := c.Profile
	p.ActorID, p.Workspaces, p.Capabilities = c.Actor.String(), []string{c.Workspace.String()}, []string{"INGEST_RAW"}
	root, err := storage.New(p.StorageEndpoint, c.RootKey, c.RootSecret, p.Bucket, false)
	if err != nil || root.EnsureBucket(ctx) != nil {
		return fmt.Errorf("native bucket preparation failed")
	}
	allow := func(prefix string, actions ...string) any {
		return map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::raw/" + prefix + "*"}}
	}
	stage, core := "staging/"+c.Workspace.String()+"/", "core/"+c.Workspace.String()+"/"
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{allow(stage, "s3:GetObject"), allow(core, "s3:GetObject", "s3:PutObject")}})
	creds, err := credentials.NewSTSAssumeRole("http://"+p.StorageEndpoint, credentials.STSAssumeRoleOptions{AccessKey: c.RootKey, SecretKey: c.RootSecret, Policy: string(policy)})
	if err != nil {
		return fmt.Errorf("native Core identity preparation failed")
	}
	cv, err := creds.Get()
	if err != nil {
		return fmt.Errorf("native Core identity issuance failed")
	}
	if err := write(p.CoreCredentialFile, pilot.Identity{AccessKey: cv.AccessKeyID, SecretKey: cv.SecretAccessKey, SessionToken: cv.SessionToken}); err != nil {
		return err
	}
	// Same native issuance and policy; only add a non-secret runtime binding.
	profile := lifecyclepilot.WorkspaceObjectProfile{Endpoint: p.StorageEndpoint, CredentialFile: p.CoreCredentialFile,
		Scope: lifecycleapp.ProtectedScope{WorkspaceID: c.Workspace, ActorID: c.Actor, PrincipalRef: p.Subject, Bucket: p.Bucket, ExpiresAt: cv.Expiration,
			NamespaceID:        uuid.NewSHA1(c.Workspace, []byte("core-object-namespace")),
			ProviderInstanceID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(c.Private+"/objects")),
			IncarnationID:      uuid.NewSHA1(uuid.NameSpaceURL, []byte(c.Private+"/objects/data"))}}
	if err := write(filepath.Join(c.Private, "workspace-object-profile.json"), profile); err != nil {
		return err
	}
	admin, err := madmin.New(p.StorageEndpoint, c.RootKey, c.RootSecret, false)
	if err != nil {
		return fmt.Errorf("native IAM client failed")
	}
	publisher := pilot.Identity{AccessKey: "publisher-" + uuid.NewString(), SecretKey: uuid.NewString() + uuid.NewString()}
	policy, _ = json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{allow(stage, "s3:GetObject", "s3:PutObject")}})
	pn := "trial-stage-" + c.Workspace.String()
	if admin.AddCannedPolicy(ctx, pn, policy) != nil || admin.SetUser(ctx, publisher.AccessKey, publisher.SecretKey, madmin.AccountEnabled) != nil {
		return fmt.Errorf("native publisher preparation failed")
	}
	if _, err := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{pn}, User: publisher.AccessKey}); err != nil {
		return fmt.Errorf("native publisher policy failed")
	}
	if err := write(p.PublisherCredentialFile, publisher); err != nil {
		return err
	}
	tx, rr := transaction.NewManager(pool), resourceinfra.NewPostgresRepository()
	for _, table := range p.Tables {
		code := p.ConnectionRef + "-" + table.Code
		r, err := resourceapp.NewCreateService(tx, rr).Handle(ctx, resourceapp.CreateDataResourceCommand{WorkspaceID: c.Workspace, Code: code, Name: "PostgreSQL · " + table.Name, Description: "固定合成试用来源", ResourceType: resourcedomain.ResourceTypeTableLike, DomainCode: "PARK_ENTERPRISE_ACTIVITY", SensitivityLevel: "INTERNAL", OwnerID: &c.Actor, ActorID: &c.Actor})
		if err != nil {
			return fmt.Errorf("resource prerequisite command failed")
		}
		ds, err := datasetapp.NewCreateDatasetService(tx, datasetinfra.NewPostgresRepository(pool), rr).Handle(ctx, datasetapp.CreateDatasetCommand{WorkspaceID: c.Workspace, Code: code, Name: table.Name + " RAW", DatasetType: datasetdomain.DatasetTypeRaw, SourceResourceID: &r.ID, OwnerID: &c.Actor, ActorID: &c.Actor})
		if err != nil {
			return fmt.Errorf("dataset prerequisite command failed")
		}
		p.Targets = append(p.Targets, pilot.TargetBinding{WorkspaceID: c.Workspace, Code: table.Code, ResourceID: r.ID, DatasetID: ds.ID})
	}
	return write(filepath.Join(c.Private, "profile.json"), p)
}

// inspect performs only SELECT and GET. Core owns facts; this is a receipt,
// never an alternative state store or a way to bypass admission.
func inspect(ctx context.Context, pool *pgxpool.Pool, c config) (any, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("read-only inspection transaction failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	counts := map[string]int{}
	for _, table := range []string{"data_resource", "dataset", "ingestion_operation", "dataset_version", "workflow_version", "execution", "quality_result", "dataset_certification", "delivery_operation", "audit_event", "evidence", "cost_event"} {
		q := "SELECT count(*) FROM " + table + " WHERE workspace_id=$1"
		if table == "dataset_version" {
			q = `SELECT count(*) FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1`
		}
		if table == "workflow_version" {
			q = `SELECT count(*) FROM workflow_version v JOIN workflow w ON w.id=v.workflow_id WHERE w.workspace_id=$1`
		}
		var n int
		if err := tx.QueryRow(ctx, q, c.Workspace).Scan(&n); err != nil {
			return nil, fmt.Errorf("fact inspection failed: %s", table)
		}
		counts[table] = n
	}
	var facts json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT json_build_object(
	 'operations',coalesce((SELECT json_agg(to_jsonb(o) ORDER BY id) FROM ingestion_operation o WHERE workspace_id=$1),'[]'),
	 'receipts',coalesce((SELECT json_agg(to_jsonb(r) ORDER BY r.id) FROM ingestion_receipt r JOIN ingestion_operation o ON o.id=r.operation_id WHERE o.workspace_id=$1),'[]'),
	 'versions',coalesce((SELECT json_agg(to_jsonb(v) ORDER BY v.id) FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1),'[]'),
	 'bindings',coalesce((SELECT json_agg(to_jsonb(s) ORDER BY s.receipt_id,s.code) FROM ingestion_receipt_slot s JOIN ingestion_receipt r ON r.id=s.receipt_id JOIN ingestion_operation o ON o.id=r.operation_id WHERE o.workspace_id=$1),'[]'))`, c.Workspace).Scan(&facts); err != nil {
		return nil, fmt.Errorf("receipt inspection failed")
	}
	store, err := storage.New(c.Profile.StorageEndpoint, c.RootKey, c.RootSecret, "raw", false)
	if err != nil {
		return nil, fmt.Errorf("read-only object client failed")
	}
	rows, err := tx.Query(ctx, `SELECT s.code,s.storage_uri,s.content_hash,v.row_count,v.metadata FROM ingestion_receipt_slot s JOIN dataset_version v ON v.id=s.version_id JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1 ORDER BY s.code`, c.Workspace)
	if err != nil {
		return nil, fmt.Errorf("object references failed")
	}
	defer rows.Close()
	objects := []any{}
	for rows.Next() {
		var code, uri, expected string
		var count int64
		var schema json.RawMessage
		if rows.Scan(&code, &uri, &expected, &count, &schema) != nil {
			return nil, fmt.Errorf("object reference decode failed")
		}
		r, err := store.Get(ctx, uri)
		if err != nil {
			return nil, fmt.Errorf("object read failed")
		}
		data, err := io.ReadAll(io.LimitReader(r, 128<<20))
		_ = r.Close()
		if err != nil {
			return nil, fmt.Errorf("object read failed")
		}
		h := sha256.Sum256(data)
		records, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
		if err != nil || len(records) == 0 || hex.EncodeToString(h[:]) != expected || int64(len(records)-1) != count {
			return nil, fmt.Errorf("stored RAW identity mismatch")
		}
		objects = append(objects, map[string]any{"code": code, "uri": uri, "sha256": expected, "bytes": len(data), "rows": count, "columns": records[0], "schema": schema})
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("object inspection failed")
	}
	return map[string]any{"workspaceId": c.Workspace, "counts": counts, "facts": facts, "objects": objects}, nil
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "prepare" && os.Args[1] != "inspect") {
		return fmt.Errorf("prepare|inspect required")
	}
	b, err := os.ReadFile(os.Getenv("DPP_TRIAL_CONFIG"))
	if err != nil {
		return fmt.Errorf("private trial config required")
	}
	var c config
	if json.Unmarshal(b, &c) != nil || validate(c) != nil {
		return fmt.Errorf("invalid trial configuration")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, c.DSN)
	if err != nil {
		return fmt.Errorf("Core connection failed")
	}
	defer pool.Close()
	if os.Args[1] == "prepare" {
		return prepare(ctx, pool, c)
	}
	v, err := inspect(ctx, pool, c)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(v)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
