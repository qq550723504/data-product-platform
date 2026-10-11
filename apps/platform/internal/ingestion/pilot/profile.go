// Package pilot composes the controlled B2 slice using existing infrastructure.
// It has no scheduler or IAM implementation. Revocable pilot authority and
// secret references are read from a private server profile on every boundary.
package pilot

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7/pkg/credentials"
	ds "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	adapter "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/hop"
	repo "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/infrastructure"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	hop "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/hop"
)

type Identity struct{ AccessKey, SecretKey, SessionToken string }
type TargetBinding struct {
	WorkspaceID uuid.UUID `json:"workspaceId"`
	Code        string    `json:"code"`
	ResourceID  uuid.UUID `json:"resourceId"`
	DatasetID   uuid.UUID `json:"datasetId"`
}
type Profile struct {
	Enabled                                                                                     bool
	TokenFile, ActorID, Subject                                                                 string
	Workspaces, Capabilities                                                                    []string
	ConnectionRef, InstanceRef, Database, Reader, SourceDSN, SourcePasswordFile                 string
	TemplateFile, MetadataFile                                                                  string
	Tables                                                                                      []adapter.Table
	Targets                                                                                     []TargetBinding
	HopURL, HopUser, HopPasswordFile                                                            string
	LocalRoot, EngineRoot, Bucket, StorageEndpoint, CoreCredentialFile, PublisherCredentialFile string
	StorageTLS                                                                                  bool
}

func read(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return d.ErrDenied
	}
	if json.Unmarshal(b, v) != nil {
		return d.ErrDenied
	}
	return nil
}
func secret(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 || len(b) > 16384 {
		return "", d.ErrDenied
	}
	return strings.TrimSpace(string(b)), nil
}
func Load(path string) (Profile, error) { var p Profile; err := read(path, &p); return p, err }
func (p Profile) Source() (adapter.Config, error) {
	c := adapter.Config{ConnectionRef: p.ConnectionRef, InstanceRef: p.InstanceRef, Database: p.Database, Reader: p.Reader, Tables: p.Tables}
	runtime, _ := json.Marshal(struct {
		HopURL, HopUser, LocalRoot, EngineRoot, Bucket, StorageEndpoint string
		StorageTLS                                                      bool
		Targets                                                         []TargetBinding
	}{p.HopURL, p.HopUser, p.LocalRoot, p.EngineRoot, p.Bucket, p.StorageEndpoint, p.StorageTLS, p.Targets})
	c.RuntimeHash = d.Hash(runtime)
	var err error
	c.Template, err = os.ReadFile(p.TemplateFile)
	if err != nil {
		return c, d.ErrDenied
	}
	if read(p.MetadataFile, &c.Metadata) != nil {
		return c, d.ErrDenied
	}
	conn, err := pgx.ParseConfig(p.SourceDSN)
	if err != nil {
		return c, d.ErrDenied
	}
	password, err := secret(p.SourcePasswordFile)
	if err != nil {
		return c, err
	}
	uri, err := url.Parse(p.SourceDSN)
	if err != nil || (uri.Scheme != "postgres" && uri.Scheme != "postgresql") || uri.Host == "" {
		return c, d.ErrDenied
	}
	uri.User = url.UserPassword(conn.User, password)
	c.SourceDSN = uri.String()
	return c, nil
}

// Only the private operator profile can bind a slot to Core identities. Generic
// CRUD codes/names are labels and cannot establish ingestion authority.
func configuredTargets(p Profile, ws uuid.UUID) ([]TargetBinding, error) {
	if len(p.Tables) != 3 {
		return nil, d.ErrDenied
	}
	codes := map[string]bool{}
	for _, table := range p.Tables {
		if codes[table.Code] {
			return nil, d.ErrDenied
		}
		codes[table.Code] = true
	}
	bindings := []TargetBinding{}
	seen, resources, datasets := map[string]bool{}, map[uuid.UUID]bool{}, map[uuid.UUID]bool{}
	for _, binding := range p.Targets {
		if binding.WorkspaceID != ws {
			continue
		}
		if !codes[binding.Code] || seen[binding.Code] || binding.ResourceID == uuid.Nil || binding.DatasetID == uuid.Nil || resources[binding.ResourceID] || datasets[binding.DatasetID] {
			return nil, d.ErrDenied
		}
		seen[binding.Code], resources[binding.ResourceID], datasets[binding.DatasetID] = true, true, true
		bindings = append(bindings, binding)
	}
	if len(bindings) != 3 {
		return nil, d.ErrDenied
	}
	return bindings, nil
}

func validateTargets(ctx context.Context, q repo.Querier, p Profile, r d.Request) error {
	if p.ConnectionRef != r.ConnectionRef || len(p.Tables) != 3 || len(r.Slots) != 3 {
		return d.ErrDenied
	}
	bindings, err := configuredTargets(p, r.WorkspaceID)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		var valid bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM data_resource r JOIN dataset d ON d.source_resource_id=r.id AND d.workspace_id=r.workspace_id WHERE r.id=$1 AND d.id=$2 AND r.workspace_id=$3 AND r.resource_type='TABLE_LIKE' AND d.dataset_type='RAW')`, binding.ResourceID, binding.DatasetID, r.WorkspaceID).Scan(&valid); err != nil || !valid {
			return d.ErrDenied
		}
		matched := false
		for _, slot := range r.Slots {
			if slot.Code == binding.Code && slot.ResourceID == binding.ResourceID && slot.DatasetID == binding.DatasetID {
				matched = true
			}
		}
		if !matched {
			return d.ErrDenied
		}
	}
	return nil
}

type runtimeBinding struct {
	HopURL, HopUser, HopPassword, SourcePassword   string
	LocalRoot, EngineRoot, Bucket, StorageEndpoint string
	StorageTLS                                     bool
	Core                                           Identity
}

func (p Profile) runtimeBinding() (runtimeBinding, error) {
	b := runtimeBinding{HopURL: p.HopURL, HopUser: p.HopUser, LocalRoot: p.LocalRoot, EngineRoot: p.EngineRoot, Bucket: p.Bucket, StorageEndpoint: p.StorageEndpoint, StorageTLS: p.StorageTLS}
	var err error
	b.HopPassword, err = secret(p.HopPasswordFile)
	if err != nil {
		return b, err
	}
	b.SourcePassword, err = secret(p.SourcePasswordFile)
	if err != nil || read(p.CoreCredentialFile, &b.Core) != nil {
		return b, d.ErrDenied
	}
	return b, nil
}
func Build(pool *pgxpool.Pool, path string) (*transport.Handler, error) {
	p, err := Load(path)
	if err != nil {
		return nil, err
	}
	startup, err := p.runtimeBinding()
	if err != nil {
		return nil, err
	}
	// This pilot reads local staging and owns a local container runtime. A
	// remote Hop host would give loopback/Docker-host endpoints other meanings.
	// Only the verified HTTP/1 transport has complete-write cost observation.
	endpoint, err := url.Parse(p.HopURL)
	if err != nil || endpoint.Scheme != "http" {
		return nil, d.ErrDenied
	}
	ip := net.ParseIP(endpoint.Hostname())
	if endpoint.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, d.ErrDenied
	}
	client, err := hop.NewClient(p.HopURL, p.HopUser, startup.HopPassword, adapter.NewHTTPClient(10*time.Second, nil))
	if err != nil {
		return nil, d.ErrDenied
	}
	core := startup.Core
	store, err := storage.NewWithCredentials(p.StorageEndpoint, p.Bucket, p.StorageTLS, credentials.NewStaticV4(core.AccessKey, core.SecretKey, core.SessionToken))
	if err != nil {
		return nil, d.ErrDenied
	}
	engine := &adapter.Adapter{Client: client, LocalRoot: p.LocalRoot, EngineRoot: p.EngineRoot, Bucket: p.Bucket}
	engine.Load = func(context.Context) (adapter.Config, error) {
		fresh, err := Load(path)
		if err != nil {
			return adapter.Config{}, err
		}
		// Client and local/storage bindings are fixed for this process. A new
		// private profile cannot claim a new runtime while using cached clients.
		current, err := fresh.runtimeBinding()
		if err != nil || current != startup {
			return adapter.Config{}, d.ErrDenied
		}
		return fresh.Source()
	}
	engine.Publisher = func(ctx context.Context, op d.Operation) (*storage.Store, error) {
		fresh, err := Load(path)
		if err != nil {
			return nil, err
		}
		var parent Identity
		if read(fresh.PublisherCredentialFile, &parent) != nil {
			return nil, d.ErrDenied
		}
		// MinIO's native AssumeRole forbids temporary/service-account parents.
		if parent.SessionToken != "" {
			return nil, d.ErrDenied
		}
		prefix := strings.TrimPrefix(d.Prefix(fresh.Bucket, op), "s3://"+fresh.Bucket+"/")
		policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:PutObject"}, "Resource": []string{"arn:aws:s3:::" + fresh.Bucket + "/" + prefix + "*"}}}})
		scheme := "http://"
		if fresh.StorageTLS {
			scheme = "https://"
		}
		creds, err := credentials.NewSTSAssumeRole(scheme+fresh.StorageEndpoint, credentials.STSAssumeRoleOptions{AccessKey: parent.AccessKey, SecretKey: parent.SecretKey, SessionToken: parent.SessionToken, Policy: string(policy)})
		if err != nil {
			return nil, d.ErrDenied
		}
		return storage.NewWithCredentials(fresh.StorageEndpoint, fresh.Bucket, fresh.StorageTLS, creds)
	}
	service := app.New(transaction.NewManager(pool), repo.New(pool), ds.NewPostgresRepository(pool), engine, store, p.Bucket)
	service.TargetGuard = func(ctx context.Context, tx pgx.Tx, r d.Request) error {
		fresh, err := Load(path)
		if err != nil {
			return d.ErrDenied
		}
		return validateTargets(ctx, tx, fresh, r)
	}
	h := &transport.Handler{Service: service}
	h.Source = func(ctx context.Context, ws uuid.UUID) (any, error) {
		fresh, err := Load(path)
		if err != nil {
			return nil, d.ErrDenied
		}
		targets, err := configuredTargets(fresh, ws)
		if err != nil {
			return nil, err
		}
		// Public contract only: never serialize Profile, DSN or secret references.
		return map[string]any{"connectionRef": fresh.ConnectionRef, "instanceRef": fresh.InstanceRef, "database": fresh.Database, "reader": fresh.Reader, "tables": fresh.Tables, "targets": targets}, nil
	}
	h.Resolver = func() (principal.Resolver, error) {
		fresh, err := Load(path)
		if err != nil {
			return nil, err
		}
		token, err := secret(fresh.TokenFile)
		if err != nil {
			return nil, err
		}
		return principal.NewStaticResolver(fresh.Enabled, token, fresh.Subject, fresh.ActorID, fresh.Workspaces, fresh.Capabilities)
	}
	h.Bind = func(ctx context.Context, ws uuid.UUID, key, batch string, targets []transport.Target) (d.Request, error) {
		fresh, err := Load(path)
		if err != nil {
			return d.Request{}, err
		}
		c, err := fresh.Source()
		if err != nil {
			return d.Request{}, err
		}
		config, template, schema := c.Identity()
		r := d.Request{WorkspaceID: ws, RequestKey: key, BatchKey: batch, ConnectionRef: c.ConnectionRef, InstanceRef: c.InstanceRef, ConfigHash: config, TemplateHash: template, SchemaHash: schema}
		for _, target := range targets {
			found := false
			for _, table := range c.Tables {
				if table.Code == target.Code {
					r.Slots = append(r.Slots, d.Slot{Code: target.Code, DatasetID: target.DatasetID, ResourceID: target.ResourceID, Columns: table.Columns, Rows: table.Rows})
					found = true
					break
				}
			}
			if !found {
				return r, d.ErrDenied
			}
		}
		if err := validateTargets(ctx, pool, fresh, r); err != nil {
			return r, err
		}
		return r, nil
	}
	return h, nil
}
