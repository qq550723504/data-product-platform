package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	di "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	ld "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// ProtectedScope is trusted composition, not a caller-selected workspace.
// Provider-version capabilities remain on the existing Copy DTO/ports.
type ProtectedScope struct {
	WorkspaceID, ActorID, NamespaceID, ProviderInstanceID, IncarnationID uuid.UUID
	PrincipalRef, Bucket                                                 string
	ExpiresAt                                                            time.Time
}

func (s ProtectedScope) Prefix() string { return "core/" + s.WorkspaceID.String() + "/" }
func (s ProtectedScope) Validate() error {
	if s.WorkspaceID == uuid.Nil || s.ActorID == uuid.Nil || s.NamespaceID == uuid.Nil || s.ProviderInstanceID == uuid.Nil || s.IncarnationID == uuid.Nil || !validText(s.PrincipalRef, 255) || !validText(s.Bucket, 255) || s.ExpiresAt.IsZero() {
		return ErrStorageScope
	}
	return nil
}

type ProtectedObjectIntent struct {
	ID, WorkspaceID, NamespaceID, DatasetVersionID uuid.UUID
	Purpose, Key, SHA256, ContentType              string
	ByteSize                                       int64
	ReadPublished                                  bool
}

type ProtectedObjectPort interface {
	CreateProtected(context.Context, ProtectedObjectIntent, []byte) error
	ReadProtected(context.Context, ProtectedObjectIntent) ([]byte, error)
}

type ProtectedObjectJournal interface {
	StartProtectedCall(context.Context, ProtectedObjectIntent, string) (uuid.UUID, error)
	ObserveProtectedCall(context.Context, StorageCallObservation) error
}

var ErrProtectedJournal = errors.New("protected object journal unavailable")

// ProtectedObjects reuses the lifecycle inventory and physical-call ledger.
// It deliberately offers no delete/archive/restore or arbitrary-URI fallback.
type ProtectedObjects struct {
	Pool  *pgxpool.Pool
	tx    *transaction.Manager
	repo  *di.PostgresRepository
	scope ProtectedScope
	port  ProtectedObjectPort
}

func NewProtectedObjects(pool *pgxpool.Pool, scope ProtectedScope) (*ProtectedObjects, error) {
	if pool == nil || scope.Validate() != nil {
		return nil, ErrStorageScope
	}
	return &ProtectedObjects{Pool: pool, tx: transaction.NewManager(pool), repo: di.NewPostgresRepository(pool), scope: scope}, nil
}

// BindPort is a one-time composition step; there is no default/no-op provider.
func (s *ProtectedObjects) BindPort(port ProtectedObjectPort) error {
	if port == nil || s.port != nil {
		return ErrStorageScope
	}
	s.port = port
	return nil
}

func (s *ProtectedObjects) CheckWorkspace(_ context.Context, w uuid.UUID) error {
	if w != s.scope.WorkspaceID || !time.Now().UTC().Before(s.scope.ExpiresAt) {
		return ErrStorageDenied
	}
	return nil
}

func (s *ProtectedObjects) caller() principal.RequestPrincipal {
	return principal.RequestPrincipal{WorkspaceID: s.scope.WorkspaceID, ActorID: s.scope.ActorID, Subject: s.scope.PrincipalRef}
}

func (s *ProtectedObjects) Bucket() string { return s.scope.Bucket }

func (s *ProtectedObjects) namespace(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace,identity_kind,object_prefix)
	 VALUES($1,$2,$3,$4,$5,'PROTECTED_CREATE',$6) ON CONFLICT(id) DO NOTHING`, s.scope.NamespaceID, s.scope.WorkspaceID, s.scope.ProviderInstanceID, s.scope.IncarnationID, s.scope.Bucket, s.scope.Prefix())
	if err != nil {
		return err
	}
	var ok bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_fixture_namespace WHERE id=$1 AND workspace_id=$2 AND provider_instance_id=$3 AND incarnation_id=$4 AND namespace=$5 AND identity_kind='PROTECTED_CREATE' AND object_prefix=$6)`, s.scope.NamespaceID, s.scope.WorkspaceID, s.scope.ProviderInstanceID, s.scope.IncarnationID, s.scope.Bucket, s.scope.Prefix()).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrStorageScope
	}
	return nil
}

func (s *ProtectedObjects) gate(ctx context.Context, tx pgx.Tx, version uuid.UUID, pending bool) (dd.DatasetVersion, error) {
	if err := s.CheckWorkspace(ctx, s.scope.WorkspaceID); err != nil {
		return dd.DatasetVersion{}, err
	}
	return validateRetainedVersion(ctx, tx, s.repo, s.scope.WorkspaceID, version, pending)
}

// Shared admission semantics for protected creation and operator acquisition;
// callers take the workspace fence and validate their own finite authority.
func validateRetainedVersion(ctx context.Context, tx pgx.Tx, repo *di.PostgresRepository, workspace, version uuid.UUID, pending bool) (dd.DatasetVersion, error) {
	v, err := repo.GetVersionTx(ctx, tx, version)
	if err != nil {
		return v, err
	}
	w, err := repo.WorkspaceForDatasetTx(ctx, tx, v.DatasetID)
	if err != nil {
		return v, err
	}
	if w != workspace {
		return v, ErrStorageScope
	}
	allowed := v.Status == dd.VersionReady || v.Status == dd.VersionSuperseded
	if pending {
		allowed = allowed || v.Status == dd.VersionCreated || v.Status == dd.VersionProcessing || v.Status == dd.VersionFailed
	}
	if !allowed {
		return v, ErrStorageDenied
	}
	var blocked bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_content_use_restriction WHERE workspace_id=$1 AND dataset_version_id=$2)
	 OR EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch x JOIN lifecycle_fixture_copy c ON c.id=x.copy_id
	 WHERE c.workspace_id=$1 AND c.dataset_version_id=$2 AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=x.id))`, w, v.ID).Scan(&blocked)
	if err != nil {
		return v, err
	}
	if blocked {
		return v, ErrStorageDenied
	}
	return v, nil
}

func (s *ProtectedObjects) sources(ctx context.Context, tx pgx.Tx, v dd.DatasetVersion) error {
	return validateRetainedSources(ctx, tx, s.repo, s.scope.WorkspaceID, s.scope.ExpiresAt, v)
}
func validateRetainedSources(ctx context.Context, tx pgx.Tx, repo *di.PostgresRepository, workspace uuid.UUID, expires time.Time, v dd.DatasetVersion) error {
	var ids []uuid.UUID
	if v.GeneratedByExecutionID != nil {
		rows, err := tx.Query(ctx, `SELECT i.dataset_version_id FROM execution_input i JOIN execution e ON e.id=i.execution_id WHERE i.execution_id=$1 AND e.workspace_id=$2`, *v.GeneratedByExecutionID, workspace)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return ErrStorageScope
		}
	}
	if v.GeneratedByEntityMatchJobID != nil {
		var id uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT input_dataset_version_id FROM entity_match_job WHERE id=$1 AND workspace_id=$2 AND output_dataset_id=$3`, *v.GeneratedByEntityMatchJobID, workspace, v.DatasetID).Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		if !time.Now().UTC().Before(expires) {
			return ErrStorageDenied
		}
		if _, err := validateRetainedVersion(ctx, tx, repo, workspace, id, false); err != nil {
			return err
		}
	}
	return nil
}

func (s *ProtectedObjects) intent(ctx context.Context, version uuid.UUID, key, purpose, hash string, size int64, contentType, commandKey string) (ProtectedObjectIntent, error) {
	i := ProtectedObjectIntent{ID: uuid.New(), WorkspaceID: s.scope.WorkspaceID, NamespaceID: s.scope.NamespaceID, DatasetVersionID: version, Purpose: purpose, Key: key, SHA256: hash, ByteSize: size, ContentType: contentType}
	if s.port == nil || !strings.HasPrefix(key, s.scope.Prefix()) || path.Clean(key) != key || strings.ContainsAny(key, "%\\\x00\r\n") || size <= 0 || size > 128<<20 {
		return i, ErrStorageScope
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, s.scope.WorkspaceID)
		if err != nil {
			return err
		}
		v, err := s.gate(ctx, tx, version, purpose == "CREATE")
		if err != nil {
			return err
		}
		if purpose != "CREATE" && (v.ChecksumValue != hash || v.ByteSize == nil || *v.ByteSize != size) {
			return ErrStorageContract
		}
		if err = s.sources(ctx, tx, v); err != nil {
			return err
		}
		if err = s.namespace(ctx, tx); err != nil {
			return err
		}
		fingerprint := sha256.Sum256([]byte(version.String() + ":" + hash + ":" + fmt.Sprint(size) + ":" + s.scope.ActorID.String() + ":" + s.scope.PrincipalRef))
		var replayID uuid.UUID
		if commandKey != "" {
			var old string
			e := tx.QueryRow(ctx, `SELECT object_id,request_fingerprint FROM command_idempotency WHERE workspace_id=$1 AND command_type='LIFECYCLE.PROTECTED_COPY' AND idempotency_key=$2`, s.scope.WorkspaceID, commandKey).Scan(&replayID, &old)
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if e == nil && old != hex.EncodeToString(fingerprint[:]) {
				return ErrIdempotencyConflict
			}
		}
		recordCommand := func() error {
			if commandKey == "" {
				return nil
			}
			if replayID != uuid.Nil {
				if replayID != i.ID {
					return ErrIdempotencyConflict
				}
				return nil
			}
			_, e := tx.Exec(ctx, `INSERT INTO command_idempotency(workspace_id,command_type,idempotency_key,object_id,result_ref,request_fingerprint) VALUES($1,'LIFECYCLE.PROTECTED_COPY',$2,$3,$3,$4)`, s.scope.WorkspaceID, commandKey, i.ID, hex.EncodeToString(fingerprint[:]))
			return e
		}
		var existing ProtectedObjectIntent
		err = tx.QueryRow(ctx, `SELECT id,workspace_id,namespace_id,dataset_version_id,purpose,object_key,content_sha256,byte_size,content_type FROM lifecycle_protected_object_intent WHERE namespace_id=$1 AND object_key=$2 AND purpose=$3`, i.NamespaceID, key, purpose).Scan(&existing.ID, &existing.WorkspaceID, &existing.NamespaceID, &existing.DatasetVersionID, &existing.Purpose, &existing.Key, &existing.SHA256, &existing.ByteSize, &existing.ContentType)
		if err == nil {
			i.ID = existing.ID
			if existing != i {
				return ErrStorageScope
			}
			return recordCommand()
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_protected_object_intent(id,workspace_id,namespace_id,dataset_version_id,purpose,object_key,content_sha256,byte_size,content_type,actor_id,principal_ref,dependency_revision,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, i.ID, i.WorkspaceID, i.NamespaceID, i.DatasetVersionID, i.Purpose, i.Key, i.SHA256, i.ByteSize, i.ContentType, s.scope.ActorID, s.scope.PrincipalRef, rev, time.Now().UTC())
		if err != nil {
			return err
		}
		if err = recordCommand(); err != nil {
			return err
		}
		return appendOccupancyFacts(ctx, tx, "ProtectedObjectIntentCreated", "PROTECTED_OBJECT_INTENT", i.ID, s.caller(), rev, time.Now().UTC(), map[string]any{"dataset_version_id": version, "purpose": purpose})
	})
	return i, err
}

// Plain Put cannot associate bytes with an allocated platform version.
func (s *ProtectedObjects) Put(context.Context, string, io.Reader, int64, string) (string, error) {
	return "", ErrStorageUnsupported
}

func (s *ProtectedObjects) PutVersion(ctx context.Context, v dd.DatasetVersion, key string, reader io.Reader, n int64, contentType string) (string, error) {
	if !strings.HasPrefix(key, fmt.Sprintf("%sdatasets/%s/v%06d/", s.scope.Prefix(), v.DatasetID, v.VersionNo)) {
		return "", ErrStorageScope
	}
	data, err := io.ReadAll(io.LimitReader(reader, (128<<20)+1))
	if err != nil || int64(len(data)) != n {
		return "", ErrStorageContract
	}
	h := sha256.Sum256(data)
	i, err := s.intent(ctx, v.ID, key, "CREATE", hex.EncodeToString(h[:]), n, contentType, "")
	if err != nil {
		return "", err
	}
	return s.materialize(ctx, i, data)
}

// AddRetainedCopy receives bytes from explicit custody, never root credentials.
// The key is stable for this command; a replay recovers the same object/copy.
func (s *ProtectedObjects) AddRetainedCopy(ctx context.Context, version uuid.UUID, key string, data []byte) (string, error) {
	if !validText(key, 255) {
		return "", ErrStorageScope
	}
	v, err := s.repo.GetVersion(ctx, version)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	hash := hex.EncodeToString(h[:])
	if v.ChecksumAlgorithm != "SHA256" || v.ChecksumValue != hash || v.ByteSize == nil || *v.ByteSize != int64(len(data)) {
		return "", ErrStorageContract
	}
	fingerprint := sha256.Sum256([]byte(version.String() + ":" + key))
	objectKey := fmt.Sprintf("%sdatasets/%s/v%06d/retained/%x", s.scope.Prefix(), v.DatasetID, v.VersionNo, fingerprint)
	i, err := s.intent(ctx, version, objectKey, "RETAINED_COPY", hash, int64(len(data)), v.ContentType, key)
	if err != nil {
		return "", err
	}
	var uri string
	err = s.Pool.QueryRow(ctx, `SELECT 's3://'||n.namespace||'/'||c.object_key FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id JOIN lifecycle_protected_object_verification x ON x.id=c.protected_verification_id WHERE x.intent_id=$1`, i.ID).Scan(&uri)
	if err == nil {
		return uri, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	return s.materialize(ctx, i, data)
}

func (s *ProtectedObjects) materialize(ctx context.Context, i ProtectedObjectIntent, data []byte) (string, error) {
	putErr := s.port.CreateProtected(ctx, i, data)
	if errors.Is(putErr, ErrStorageDenied) || errors.Is(putErr, ErrStorageScope) || errors.Is(putErr, ErrProtectedJournal) {
		return "", putErr
	}
	actual, err := s.port.ReadProtected(ctx, i)
	if err != nil {
		return "", errors.Join(putErr, err)
	}
	if !bytes.Equal(data, actual) {
		return "", ErrStorageContract
	}
	uri := "s3://" + s.scope.Bucket + "/" + i.Key
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, i.WorkspaceID)
		if err != nil {
			return err
		}
		v, err := s.gate(ctx, tx, i.DatasetVersionID, i.Purpose == "CREATE" && !i.ReadPublished)
		if err != nil {
			return err
		}
		if err = s.sources(ctx, tx, v); err != nil {
			return err
		}
		if (v.Status == dd.VersionReady || v.Status == dd.VersionSuperseded) && v.ChecksumValue != i.SHA256 {
			return ErrStorageContract
		}
		var existing uuid.UUID
		err = tx.QueryRow(ctx, `SELECT id FROM lifecycle_protected_object_verification WHERE intent_id=$1`, i.ID).Scan(&existing)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var createCall, readCall uuid.UUID
		var confirmed bool
		if err = tx.QueryRow(ctx, `SELECT c.id,EXISTS(SELECT 1 FROM lifecycle_storage_observation o WHERE o.call_id=c.id AND o.http_status=200 AND o.failure='') confirmed FROM lifecycle_storage_call c WHERE c.protected_object_intent_id=$1 AND c.operation='PROTECTED_PUT' AND NOT EXISTS(SELECT 1 FROM lifecycle_storage_observation o WHERE o.call_id=c.id AND o.http_status BETWEEN 400 AND 599 AND o.http_status<>412) ORDER BY confirmed DESC,c.started_at,c.id LIMIT 1`, i.ID).Scan(&createCall, &confirmed); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `SELECT c.id FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id WHERE c.protected_object_intent_id=$1 AND c.operation='PROTECTED_GET' AND o.response_sha256=$2 AND o.failure='' AND o.http_status=200 ORDER BY c.started_at,c.id LIMIT 1`, i.ID, i.SHA256).Scan(&readCall); err != nil {
			return err
		}
		verification, copyID := uuid.New(), uuid.New()
		identity := ld.ProtectedCopy{ID: copyID, WorkspaceID: i.WorkspaceID, DatasetVersionID: i.DatasetVersionID, ProviderInstanceID: s.scope.ProviderInstanceID, IncarnationID: s.scope.IncarnationID, VerificationID: verification, Namespace: s.scope.Bucket, Key: i.Key, ContentSHA256: i.SHA256, ByteSize: i.ByteSize}
		if err = identity.Validate(); err != nil {
			return err
		}
		creation := "RECONCILED"
		if confirmed {
			creation = "CONFIRMED"
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_protected_object_verification(id,workspace_id,intent_id,create_call_id,read_call_id,content_sha256,byte_size,verified_at,creation_observation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, verification, i.WorkspaceID, i.ID, createCall, readCall, i.SHA256, i.ByteSize, time.Now().UTC(), creation)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size,identity_kind,protected_verification_id) VALUES($1,$2,$3,$4,$5,NULL,$6,$7,'PROTECTED_CREATE',$8)`, copyID, i.WorkspaceID, i.NamespaceID, i.DatasetVersionID, i.Key, i.SHA256, i.ByteSize, verification)
		if err != nil {
			return err
		}
		return appendOccupancyFacts(ctx, tx, "ProtectedObjectVerified", "LIFECYCLE_COPY", copyID, s.caller(), rev, time.Now().UTC(), map[string]any{"dataset_version_id": i.DatasetVersionID, "verification_id": verification, "identity_kind": "PROTECTED_CREATE"})
	})
	return uri, err
}

func (s *ProtectedObjects) LockPublication(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	_, err := deliveryfence.Lock(ctx, tx, s.scope.WorkspaceID)
	if err != nil {
		return err
	}
	v, err := s.gate(ctx, tx, id, true)
	if err != nil {
		return err
	}
	return s.sources(ctx, tx, v)
}
func (s *ProtectedObjects) FinalizePublication(ctx context.Context, tx pgx.Tx, v dd.DatasetVersion) error {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id WHERE c.workspace_id=$1 AND c.dataset_version_id=$2 AND c.identity_kind='PROTECTED_CREATE' AND c.content_sha256=$3 AND c.byte_size=$4 AND 's3://'||n.namespace||'/'||c.object_key=$5)`, s.scope.WorkspaceID, v.ID, v.ChecksumValue, v.ByteSize, v.StorageURI).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrStorageContract
	}
	return nil
}

func (s *ProtectedObjects) Get(ctx context.Context, uri string) (io.ReadCloser, error) {
	rows, err := s.Pool.Query(ctx, `SELECT v.id FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE v.storage_uri=$1 AND d.workspace_id=$2 LIMIT 2`, uri, s.scope.WorkspaceID)
	if err != nil {
		return nil, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) != 1 {
		return nil, ErrStorageScope
	}
	return s.ReadVersion(ctx, ids[0])
}

func (s *ProtectedObjects) ReadVersion(ctx context.Context, id uuid.UUID) (io.ReadCloser, error) {
	var v dd.DatasetVersion
	var i ProtectedObjectIntent
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := deliveryfence.Lock(ctx, tx, s.scope.WorkspaceID)
		if err != nil {
			return err
		}
		v, err = s.gate(ctx, tx, id, false)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT i.id,i.workspace_id,i.namespace_id,i.dataset_version_id,i.purpose,i.object_key,i.content_sha256,i.byte_size,i.content_type FROM lifecycle_fixture_copy c JOIN lifecycle_protected_object_verification x ON x.id=c.protected_verification_id JOIN lifecycle_protected_object_intent i ON i.id=x.intent_id WHERE c.workspace_id=$1 AND c.dataset_version_id=$2 AND c.identity_kind='PROTECTED_CREATE' AND c.content_sha256=$3 AND c.byte_size=$4 AND c.namespace_id=$5 ORDER BY c.id LIMIT 1`, s.scope.WorkspaceID, id, v.ChecksumValue, v.ByteSize, s.scope.NamespaceID).Scan(&i.ID, &i.WorkspaceID, &i.NamespaceID, &i.DatasetVersionID, &i.Purpose, &i.Key, &i.SHA256, &i.ByteSize, &i.ContentType)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		u, e := url.Parse(v.StorageURI)
		if e != nil || u.Scheme != "s3" || u.Host != s.scope.Bucket || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(strings.TrimPrefix(u.Path, "/"), s.scope.Prefix()) || v.ByteSize == nil {
			return nil, ErrStorageScope
		}
		i, err = s.intent(ctx, id, strings.TrimPrefix(u.Path, "/"), "SOURCE_READ", v.ChecksumValue, *v.ByteSize, v.ContentType, "")
	}
	if err != nil {
		return nil, err
	}
	i.ReadPublished = true
	data, err := s.port.ReadProtected(ctx, i)
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(data)
	if int64(len(data)) != i.ByteSize || hex.EncodeToString(h[:]) != i.SHA256 {
		return nil, ErrStorageContract
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, e := deliveryfence.Lock(ctx, tx, s.scope.WorkspaceID); e != nil {
			return e
		}
		_, e := s.gate(ctx, tx, id, false)
		return e
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *ProtectedObjects) StartProtectedCall(ctx context.Context, i ProtectedObjectIntent, method string) (uuid.UUID, error) {
	id := uuid.New()
	if method != "PUT" && method != "GET" && method != "HEAD" {
		return uuid.Nil, ErrStorageUnsupported
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rev, err := deliveryfence.Lock(ctx, tx, s.scope.WorkspaceID)
		if err != nil {
			return err
		}
		v, err := s.gate(ctx, tx, i.DatasetVersionID, i.Purpose == "CREATE" && !i.ReadPublished)
		if err != nil {
			return err
		}
		if err = s.sources(ctx, tx, v); err != nil {
			return err
		}
		var valid bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_protected_object_intent WHERE id=$1 AND workspace_id=$2 AND namespace_id=$3 AND dataset_version_id=$4 AND object_key=$5 AND content_sha256=$6 AND byte_size=$7 AND purpose=$8)`, i.ID, s.scope.WorkspaceID, s.scope.NamespaceID, i.DatasetVersionID, i.Key, i.SHA256, i.ByteSize, i.Purpose).Scan(&valid)
		if err != nil {
			return err
		}
		if !valid || (method == "PUT" && (i.Purpose == "SOURCE_READ" || i.ReadPublished)) {
			return ErrStorageScope
		}
		_, err = tx.Exec(ctx, `INSERT INTO lifecycle_storage_call(id,workspace_id,copy_id,operation,started_at,actor_id,principal_ref,dependency_revision,protected_object_intent_id) VALUES($1,$2,NULL,$3,$4,$5,$6,$7,$8)`, id, s.scope.WorkspaceID, "PROTECTED_"+method, time.Now().UTC(), s.scope.ActorID, s.scope.PrincipalRef, rev, i.ID)
		if err != nil {
			return err
		}
		return appendOccupancyFacts(ctx, tx, StorageCallStartedEvent, "LIFECYCLE_STORAGE_CALL", id, s.caller(), rev, time.Now().UTC(), map[string]any{"operation": "PROTECTED_" + method, "protected_object_intent_id": i.ID})
	})
	return id, err
}

// Observations use the original start's authority; incurred costs survive expiry.
func (s *ProtectedObjects) ObserveProtectedCall(ctx context.Context, o StorageCallObservation) error {
	if !validStorageObservation(o) {
		return ErrStorageContract
	}
	b, err := json.Marshal(o)
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var actor uuid.UUID
		var subject string
		var rev int64
		err := tx.QueryRow(ctx, `SELECT actor_id,principal_ref,dependency_revision FROM lifecycle_storage_call WHERE id=$1 AND workspace_id=$2 AND protected_object_intent_id IS NOT NULL`, o.CallID, s.scope.WorkspaceID).Scan(&actor, &subject, &rev)
		if err != nil {
			return err
		}
		id := uuid.New()
		tag, err := tx.Exec(ctx, `INSERT INTO lifecycle_storage_observation(id,workspace_id,call_id,observed_at,invoked_at,http_status,request_ref,response_sha256,failure,proof_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(call_id,proof_sha256) DO NOTHING`, id, s.scope.WorkspaceID, o.CallID, o.ObservedAt, o.InvokedAt, o.HTTPStatus, o.RequestRef, o.ResponseSHA256, o.Failure, hex.EncodeToString(h[:]))
		if err != nil {
			return err
		}
		if o.InvokedAt != nil {
			if err = cost.AppendLifecycleStorageActivity(ctx, tx, s.scope.WorkspaceID, o.CallID, *o.InvokedAt); err != nil {
				return err
			}
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		caller := principal.RequestPrincipal{WorkspaceID: s.scope.WorkspaceID, ActorID: actor, Subject: subject}
		return appendOccupancyFacts(ctx, tx, StorageCallObservedEvent, "LIFECYCLE_STORAGE_OBSERVATION", id, caller, rev, time.Now().UTC(), map[string]any{"call_id": o.CallID, "http_status": o.HTTPStatus, "invocation_confirmed": o.InvokedAt != nil, "failure": o.Failure})
	})
}
