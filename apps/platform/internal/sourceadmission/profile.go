package sourceadmission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

// ProtectedObjects.Get intentionally reads a DatasetVersion's primary payload.
// Evidence sidecars instead consume its registered exact-copy intent through
// the same protected journal/adapter. No lifecycle implementation is changed.
type evidenceStore struct {
	*lc.ProtectedObjects
	pool  *pgxpool.Pool
	scope lc.ProtectedScope
	port  lc.ProtectedObjectPort
}

func (s *evidenceStore) Get(ctx context.Context, uri string) (io.ReadCloser, error) {
	var i lc.ProtectedObjectIntent
	e := s.pool.QueryRow(ctx, `SELECT i.id,i.workspace_id,i.namespace_id,i.dataset_version_id,i.purpose,i.object_key,i.content_sha256,i.byte_size,i.content_type FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id JOIN lifecycle_protected_object_verification x ON x.id=c.protected_verification_id JOIN lifecycle_protected_object_intent i ON i.id=x.intent_id WHERE c.workspace_id=$1 AND c.namespace_id=$2 AND c.identity_kind='PROTECTED_CREATE' AND 's3://'||n.namespace||'/'||c.object_key=$3`, s.scope.WorkspaceID, s.scope.NamespaceID, uri).Scan(&i.ID, &i.WorkspaceID, &i.NamespaceID, &i.DatasetVersionID, &i.Purpose, &i.Key, &i.SHA256, &i.ByteSize, &i.ContentType)
	if e != nil {
		if e == pgx.ErrNoRows {
			return nil, ErrIntegrity
		}
		return nil, ErrUnknown
	}
	data, e := s.port.ReadProtected(ctx, i)
	if e != nil {
		// Native exact-copy length/contract verification can fail before our
		// hash comparison. Preserve proven corruption as an integrity failure;
		// actual transport uncertainty must remain recoverable as UNKNOWN.
		if errors.Is(e, lc.ErrStorageContract) {
			return nil, ErrIntegrity
		}
		return nil, e
	}
	if int64(len(data)) != i.ByteSize || Hash(data) != i.SHA256 {
		return nil, ErrIntegrity
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Profile is private, operator-owned composition. No profile/configuration
// mutation is exposed over HTTP. Runtime identity changes require a restart and
// a newly registered connection; disabling and token rotation are fresh checks.
type Profile struct {
	Enabled               bool              `json:"enabled"`
	Connection            Connection        `json:"connection"`
	ActorID               uuid.UUID         `json:"actorId"`
	Subject               string            `json:"subject"`
	APITokenFile          string            `json:"apiTokenFile"`
	StorageScope          lc.ProtectedScope `json:"storageScope"`
	StorageEndpoint       string            `json:"storageEndpoint"`
	StorageTLS            bool              `json:"storageTLS"`
	StorageCredentialFile string            `json:"storageCredentialFile"`
}
type storageIdentity struct{ AccessKey, SecretKey, SessionToken string }

func loadProfile(path string) (Profile, error) {
	var p Profile
	b, e := os.ReadFile(path)
	if e != nil || len(b) > 65536 || strictJSON(b, &p) != nil {
		return p, ErrDenied
	}
	return p, nil
}
func (p Profile) binding() string { p.Enabled = false; b, _ := json.Marshal(p); return Hash(b) }

type profileResolver struct{ path, binding string }

func (r profileResolver) Resolve(request *http.Request, workspace uuid.UUID, capability string) (principal.RequestPrincipal, error) {
	p, e := loadProfile(r.path)
	if e != nil || !p.Enabled || p.binding() != r.binding || workspace != p.Connection.WorkspaceID {
		return principal.RequestPrincipal{}, ErrDenied
	}
	b, e := os.ReadFile(p.APITokenFile)
	if e != nil || len(b) > 16384 {
		return principal.RequestPrincipal{}, ErrDenied
	}
	resolver, e := principal.NewStaticResolver(true, strings.TrimSpace(string(b)), p.Subject, p.ActorID.String(), []string{workspace.String()}, []string{Capability})
	if e != nil {
		return principal.RequestPrincipal{}, ErrDenied
	}
	return resolver.Resolve(request, workspace, capability)
}
func Build(pool *pgxpool.Pool, path string) (*Handler, error) {
	p, e := loadProfile(path)
	if e != nil || !p.Connection.valid() || p.ActorID == uuid.Nil || p.Subject == "" || p.StorageScope.WorkspaceID != p.Connection.WorkspaceID || p.StorageScope.ActorID != p.ActorID || p.StorageScope.PrincipalRef != p.Subject {
		return nil, ErrDenied
	}
	if _, e = sourceBase(p.Connection); e != nil {
		return nil, e
	}
	var identity storageIdentity
	b, e := os.ReadFile(p.StorageCredentialFile)
	if e != nil || strictJSON(b, &identity) != nil || identity.AccessKey == "" || identity.SecretKey == "" {
		return nil, ErrDenied
	}
	objects, e := lc.NewProtectedObjects(pool, p.StorageScope)
	if e != nil {
		return nil, e
	}
	adapter, e := storage.NewProtectedAdapter(p.StorageEndpoint, p.StorageScope.Bucket, p.StorageTLS, credentials.NewStaticV4(identity.AccessKey, identity.SecretKey, identity.SessionToken), objects)
	if e != nil {
		return nil, e
	}
	if e = objects.BindPort(adapter); e != nil {
		return nil, e
	}
	store := &evidenceStore{ProtectedObjects: objects, pool: pool, scope: p.StorageScope, port: adapter}
	service := New(pool)
	service.Source = NewHTTPSource()
	binding := p.binding()
	service.Load = func(_ context.Context, id uuid.UUID) (Connection, error) {
		fresh, e := loadProfile(path)
		if e != nil || !fresh.Enabled || fresh.binding() != binding || id != p.Connection.ID {
			return Connection{}, ErrDenied
		}
		return fresh.Connection, nil
	}
	service.Store = func(c Connection) (VersionStore, error) {
		fresh, e := loadProfile(path)
		var actual storageIdentity
		secret, readErr := os.ReadFile(p.StorageCredentialFile)
		if e != nil || !fresh.Enabled || fresh.binding() != binding || c.fingerprint() != p.Connection.fingerprint() || readErr != nil || strictJSON(secret, &actual) != nil || actual != identity {
			return nil, ErrDenied
		}
		return store, nil
	}
	if e = service.RegisterConnection(context.Background(), p.Connection); e != nil {
		return nil, e
	}
	return &Handler{Service: service, Resolver: profileResolver{path, binding}}, nil
}
