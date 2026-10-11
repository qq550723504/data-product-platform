// lifecycle-fixture-prepare creates four synthetic native versions exactly once.
// It has no deletion, cleanup, reader, approval, dispatch or delivery command.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

var namespaces = []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}

type config struct {
	RunID, WorkspaceID, ProviderInstanceID, IncarnationID, ActorID                     uuid.UUID
	Enabled                                                                            bool
	Endpoint, PostgresDSN, RootAccess, RootSecret, InspectAccess, InspectSecret, Token string
	OutputDirectory                                                                    string
	Environment                                                                        json.RawMessage
}

func load(path string) (config, error) {
	var c config
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 128<<10 {
		return c, errors.New("private configuration unreadable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, errors.New("private configuration invalid")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("private configuration trailing data")
	}
	host, port, err := net.SplitHostPort(c.Endpoint)
	if err != nil || host != "127.0.0.1" || port == "" || c.RunID == uuid.Nil || c.WorkspaceID == uuid.Nil ||
		c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil || c.ActorID == uuid.Nil ||
		c.PostgresDSN == "" || c.RootAccess == "" || c.RootSecret == "" || c.InspectAccess == "" ||
		c.InspectSecret == "" || c.Token == "" || !filepath.IsAbs(c.OutputDirectory) || !json.Valid(c.Environment) {
		return c, errors.New("private configuration scope incomplete")
	}
	return c, nil
}

// Re-read current server configuration for every admission. Reuse the existing
// controlled-pilot principal resolver; no client can choose actor/workspace/caps.
type authority struct {
	path, token string
	run         uuid.UUID
}

func (a authority) resolve(w uuid.UUID, cap string) (principal.RequestPrincipal, error) {
	c, err := load(a.path)
	if err != nil || c.RunID != a.run {
		return principal.RequestPrincipal{}, principal.ErrNotConfigured
	}
	r, err := principal.NewStaticResolver(c.Enabled, c.Token, "lifecycle-fixture:"+c.RunID.String(), c.ActorID.String(),
		[]string{c.WorkspaceID.String()}, []string{lc.CapabilityPreparePlan, lc.CapabilityInspectStorage, lc.CapabilityRestrictUse})
	if err != nil {
		return principal.RequestPrincipal{}, err
	}
	req, _ := http.NewRequest(http.MethodPost, "http://fixture.invalid/internal", nil)
	req.Header.Set("Authorization", "Bearer "+a.token)
	return r.Resolve(req, w, cap)
}
func (a authority) AuthorizePlan(_ context.Context, _ pgx.Tx, w uuid.UUID) (principal.RequestPrincipal, error) {
	return a.resolve(w, lc.CapabilityPreparePlan)
}
func (a authority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, _ time.Time) (principal.RequestPrincipal, error) {
	return a.resolve(w, cap)
}

type stepKey struct{}
type step struct {
	mu    sync.Mutex
	id    uuid.UUID
	label string
	sent  bool
}

// Bootstrap receipts are infrastructure provisioning evidence, separate from
// business CostEvent. Actual platform inspections use the PostgreSQL journal.
type bootstrapTransport struct {
	base           http.RoundTripper
	host, receipts string
	mu             sync.Mutex
}

func (t *bootstrapTransport) append(v any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	f, err := os.OpenFile(t.receipts, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = json.NewEncoder(f).Encode(v); err != nil {
		return err
	}
	return f.Sync()
}
func (t *bootstrapTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	s, ok := r.Context().Value(stepKey{}).(*step)
	if !ok || r.URL.Scheme != "http" || r.URL.Host != t.host || r.URL.User != nil ||
		(r.Method != "GET" && r.Method != "HEAD" && r.Method != "PUT" && r.Method != "POST") {
		return nil, errors.New("bootstrap transport scope denied")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent {
		return nil, errors.New("bootstrap hidden retry denied; reconcile without repeating writes")
	}
	s.sent = true
	if err := t.append(map[string]any{"id": s.id, "operation": s.label, "fact": "START", "at": time.Now().UTC()}); err != nil {
		return nil, err
	}
	var invoked *time.Time
	var invocationMu sync.Mutex
	trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) {
		at := time.Now().UTC()
		invocationMu.Lock()
		invoked = &at
		invocationMu.Unlock()
	}}
	response, err := t.base.RoundTrip(r.WithContext(httptrace.WithClientTrace(r.Context(), trace)))
	invocationMu.Lock()
	observedInvocation := invoked
	invocationMu.Unlock()
	status := 0
	if response != nil {
		status = response.StatusCode
		if observedInvocation == nil {
			at := time.Now().UTC()
			observedInvocation = &at
		}
	}
	failure := ""
	if err != nil {
		failure = "TRANSPORT_UNKNOWN"
	}
	if journalErr := t.append(map[string]any{"id": s.id, "operation": s.label, "fact": "OBSERVATION", "at": time.Now().UTC(),
		"invoked_at": observedInvocation, "http_status": status, "failure": failure}); journalErr != nil {
		if response != nil {
			response.Body.Close()
		}
		return nil, errors.New("bootstrap receipt persistence failed; do not retry")
	}
	return response, err
}

func one(ctx context.Context, label string, fn func(context.Context) error) error {
	fmt.Println("prepare:", label)
	ctx, cancel := context.WithTimeout(context.WithValue(ctx, stepKey{}, &step{id: uuid.New(), label: label}), 20*time.Second)
	defer cancel()
	if err := fn(ctx); err != nil {
		return fmt.Errorf("%s failed; environment preserved, no automatic retry", label)
	}
	return nil
}

type object struct {
	CopyID               uuid.UUID  `json:"copy_id"`
	DatasetVersionID     uuid.UUID  `json:"dataset_version_id"`
	Namespace            string     `json:"namespace"`
	Key                  string     `json:"key"`
	Version              string     `json:"version_id"`
	SHA256               string     `json:"sha256"`
	Bytes                int64      `json:"bytes"`
	LegalHold            bool       `json:"legal_hold"`
	Retention            string     `json:"retention_mode"`
	RetainUntil          *time.Time `json:"retain_until"`
	BytesVerifiedAt      time.Time  `json:"bytes_verified_at"`
	ProtectionObservedAt time.Time  `json:"protection_observed_at"`
	copy                 domain.Copy
}

func prepare(ctx context.Context, path string) error {
	c, err := load(path)
	if err != nil {
		return err
	}
	if !c.Enabled {
		return errors.New("fixture authority disabled")
	}
	if os.Getenv("LIFECYCLE_FIXTURE_PREPARE_UNPROTECTED_PLAN") == "1" {
		return prepareUnprotectedPlan(ctx, path, c)
	}
	if os.Getenv("LIFECYCLE_FIXTURE_INSPECT_REGISTERED") == "1" {
		return inspectRegistered(ctx, path, c)
	}
	if _, err = os.Stat(c.OutputDirectory); !os.IsNotExist(err) {
		// Only explicit recovery of an EMPTY provider/database is permitted.
		// Any persisted policy/manifest or native namespace below blocks writes.
		if os.Getenv("LIFECYCLE_FIXTURE_RESUME_EMPTY") != "1" {
			return errors.New("output already exists; one-shot preparation refuses replay")
		}
		entries, e := os.ReadDir(c.OutputDirectory)
		if e != nil {
			return errors.New("recovery output unreadable")
		}
		for _, entry := range entries {
			if entry.Name() != "bootstrap-receipts.jsonl" || entry.IsDir() {
				return errors.New("recovery refuses nonempty fixture preparation")
			}
		}
	} else if err = os.Mkdir(c.OutputDirectory, 0700); err != nil {
		return errors.New("cannot create output")
	}
	pool, err := pgxpool.New(ctx, c.PostgresDSN)
	if err != nil {
		return errors.New("fixture database unavailable")
	}
	defer pool.Close()
	var n int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM dataset)+(SELECT count(*) FROM lifecycle_fixture_namespace)`).Scan(&n); err != nil || n != 0 {
		return errors.New("fixture database must be fresh and fully migrated")
	}
	t := &bootstrapTransport{host: c.Endpoint, receipts: filepath.Join(c.OutputDirectory, "bootstrap-receipts.jsonl"),
		base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false,
			DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 64 << 10}}
	root, err := minio.New(c.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Secure: false,
		Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: t})
	if err != nil {
		return errors.New("bootstrap client invalid")
	}
	admin, err := madmin.NewWithOptions(c.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Secure: false, Transport: t})
	if err != nil {
		return errors.New("bootstrap admin client invalid")
	}
	err = one(ctx, "verify-empty-provider", func(ctx context.Context) error {
		buckets, e := root.ListBuckets(ctx)
		if e != nil {
			return e
		}
		if len(buckets) != 0 {
			return errors.New("provider not empty")
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, ns := range namespaces {
		if err = one(ctx, "create-locked-namespace:"+ns, func(ctx context.Context) error {
			return root.MakeBucket(ctx, ns, minio.MakeBucketOptions{Region: "us-east-1", ObjectLocking: true})
		}); err != nil {
			return err
		}
		if err = one(ctx, "verify-versioning:"+ns, func(ctx context.Context) error {
			v, e := root.GetBucketVersioning(ctx, ns)
			if e != nil {
				return e
			}
			if v.Status != "Enabled" {
				return errors.New("versioning not enabled")
			}
			return nil
		}); err != nil {
			return err
		}
	}
	policy := map[string]any{"Version": "2012-10-17", "Statement": []any{
		map[string]any{"Effect": "Allow", "Action": []string{"s3:ListBucketVersions"}, "Resource": []string{"arn:aws:s3:::" + namespaces[0], "arn:aws:s3:::" + namespaces[1]}},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectVersion", "s3:GetObjectRetention", "s3:GetObjectLegalHold"},
			"Resource": []string{"arn:aws:s3:::" + namespaces[0] + "/fixture/a.csv", "arn:aws:s3:::" + namespaces[0] + "/fixture/b.csv", "arn:aws:s3:::" + namespaces[1] + "/fixture/a.csv", "arn:aws:s3:::" + namespaces[1] + "/fixture/b.csv"}},
	}}
	policyBytes, _ := json.MarshalIndent(policy, "", "  ")
	policyName := "lifecycle307-inspection-" + c.RunID.String()
	if err = one(ctx, "create-inspection-policy", func(ctx context.Context) error { return admin.AddCannedPolicy(ctx, policyName, policyBytes) }); err != nil {
		return err
	}
	if err = one(ctx, "create-inspection-identity", func(ctx context.Context) error { return admin.AddUser(ctx, c.InspectAccess, c.InspectSecret) }); err != nil {
		return err
	}
	if err = one(ctx, "attach-inspection-policy", func(ctx context.Context) error {
		_, e := admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{policyName}, User: c.InspectAccess})
		return e
	}); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(c.OutputDirectory, "inspection-policy.json"), policyBytes, 0600); err != nil {
		return err
	}
	versions := []uuid.UUID{uuid.New(), uuid.New()}
	var objects []object
	retentionUntil := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	for ni, ns := range namespaces {
		for i, key := range []string{"fixture/a.csv", "fixture/b.csv"} {
			payload := []byte([]string{"id,value\n1,synthetic-alpha\n", "id,value\n2,synthetic-beta\n"}[i])
			digest := sha256.Sum256(payload)
			o := object{CopyID: uuid.New(), DatasetVersionID: versions[i], Namespace: ns, Key: key, SHA256: hex.EncodeToString(digest[:]), Bytes: int64(len(payload)), Retention: "NONE"}
			opts := minio.PutObjectOptions{ContentType: "text/csv", DisableMultipart: true}
			opts.SetMatchETagExcept("*")
			if ni == 0 && i == 1 {
				opts.LegalHold = minio.LegalHoldEnabled
				o.LegalHold = true
			}
			if ni == 1 && i == 0 {
				opts.Mode = minio.Governance
				opts.RetainUntilDate = retentionUntil
				o.Retention = "GOVERNANCE"
				o.RetainUntil = &retentionUntil
			}
			if err = one(ctx, "create-synthetic-version:"+ns+":"+key, func(ctx context.Context) error {
				info, e := root.PutObject(ctx, ns, key, bytes.NewReader(payload), int64(len(payload)), opts)
				if e == nil {
					o.Version = info.VersionID
				}
				return e
			}); err != nil {
				return err
			}
			if o.Version == "" || o.Version == "null" {
				return errors.New("native version identity missing")
			}
			if err = one(ctx, "verify-exact-synthetic-bytes:"+ns+":"+key, func(ctx context.Context) error {
				obj, e := root.GetObject(ctx, ns, key, minio.GetObjectOptions{VersionID: o.Version})
				if e != nil {
					return e
				}
				defer obj.Close()
				b, e := io.ReadAll(io.LimitReader(obj, o.Bytes+1))
				if e != nil {
					return e
				}
				if !bytes.Equal(b, payload) {
					return errors.New("exact bytes mismatch")
				}
				info, e := obj.Stat()
				if e != nil {
					return e
				}
				if info.VersionID != o.Version || info.Size != o.Bytes {
					return errors.New("read identity mismatch")
				}
				o.BytesVerifiedAt = time.Now().UTC()
				return nil
			}); err != nil {
				return err
			}
			o.copy = domain.Copy{ID: o.CopyID, WorkspaceID: c.WorkspaceID, DatasetVersionID: o.DatasetVersionID, ProviderInstanceID: c.ProviderInstanceID,
				IncarnationID: c.IncarnationID, Namespace: ns, Key: key, ProviderVersion: o.Version, ContentSHA256: o.SHA256, ByteSize: o.Bytes}
			objects = append(objects, o)
		}
	}
	// Fixture bootstrap records real content identity, not product certification,
	// legal rights, archive/restore success, or normal business-reader acceptance.
	err = transaction.NewManager(pool).Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for i, v := range versions {
			d := uuid.New()
			o := objects[i]
			if _, e := tx.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,$3,'Closed synthetic lifecycle fixture','RAW')`, d, c.WorkspaceID, "LC307-"+v.String()); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,checksum_algorithm,checksum_value,byte_size) VALUES($1,$2,1,'READY','OBJECT_STORAGE',$3,'SHA256',$4,$5)`, v, d, "s3://"+o.Namespace+"/"+o.Key, o.SHA256, o.Bytes); e != nil {
				return e
			}
		}
		for _, ns := range namespaces {
			id := uuid.New()
			if _, e := tx.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace) VALUES($1,$2,$3,$4,$5)`, id, c.WorkspaceID, c.ProviderInstanceID, c.IncarnationID, ns); e != nil {
				return e
			}
			for _, o := range objects {
				if o.Namespace != ns {
					continue
				}
				if _, e := tx.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, o.CopyID, c.WorkspaceID, id, o.DatasetVersionID, o.Key, o.Version, o.SHA256, o.Bytes); e != nil {
					return e
				}
			}
		}
		return nil
	})
	if err != nil {
		return errors.New("fixture registry transaction failed; preserve provider versions")
	}
	return finish(ctx, path, c, pool, objects)
}

// This separate non-destructive command prepares the two-object execution
// candidate from the already verified manifest. It performs no storage I/O.
func prepareUnprotectedPlan(ctx context.Context, path string, c config) error {
	var m struct {
		Schema      string    `json:"schema"`
		Run         uuid.UUID `json:"run_id"`
		Workspace   uuid.UUID `json:"workspace_id"`
		Provider    uuid.UUID `json:"provider_instance_id"`
		Incarnation uuid.UUID `json:"incarnation_id"`
		Objects     []object  `json:"objects"`
	}
	b, err := os.ReadFile(filepath.Join(c.OutputDirectory, "manifest.json"))
	if err != nil || json.Unmarshal(b, &m) != nil || m.Schema != "lifecycle307-predelete-manifest-v1" || m.Run != c.RunID || m.Workspace != c.WorkspaceID || m.Provider != c.ProviderInstanceID || m.Incarnation != c.IncarnationID || len(m.Objects) != 4 {
		return errors.New("verified manifest scope required")
	}
	output := filepath.Join(c.OutputDirectory, "deletion-candidate.json")
	if _, err = os.Stat(output); !os.IsNotExist(err) {
		return errors.New("deletion candidate already prepared")
	}
	var ids []uuid.UUID
	var targets []object
	for _, o := range m.Objects {
		if (o.Namespace == namespaces[0] && o.Key == "fixture/a.csv") || (o.Namespace == namespaces[1] && o.Key == "fixture/b.csv") {
			if o.CopyID == uuid.Nil || o.LegalHold || o.Retention != "NONE" || o.Version == "" || o.Version == "null" {
				return errors.New("unprotected candidate manifest invalid")
			}
			ids = append(ids, o.CopyID)
			targets = append(targets, o)
		}
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		return errors.New("exact two-object candidate required")
	}
	pool, err := pgxpool.New(ctx, c.PostgresDSN)
	if err != nil {
		return errors.New("fixture database unavailable")
	}
	defer pool.Close()
	// Registry identities are immutable; verify manifest equality before the
	// command can append a plan. The command re-loads/re-authorizes independently.
	err = transaction.NewManager(pool).Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		copies, e := (&infrastructure.PostgresRepository{}).LoadFixtureCopies(ctx, tx, c.WorkspaceID, ids)
		if e != nil {
			return e
		}
		for _, copy := range copies {
			found := false
			for _, o := range targets {
				if copy.ID == o.CopyID && copy.DatasetVersionID == o.DatasetVersionID && copy.ProviderInstanceID == c.ProviderInstanceID && copy.IncarnationID == c.IncarnationID && copy.Namespace == o.Namespace && copy.Key == o.Key && copy.ProviderVersion == o.Version && copy.ContentSHA256 == o.SHA256 && copy.ByteSize == o.Bytes {
					found = true
				}
			}
			if !found {
				return errors.New("registered candidate differs from verified manifest")
			}
		}
		return nil
	})
	if err != nil {
		return errors.New("candidate registry validation failed")
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		return err
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	a := authority{path: path, token: c.Token, run: c.RunID}
	p, err := lc.NewPlanService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, a).PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: ids, IdempotencyKey: "unprotected-candidate:" + c.RunID.String()})
	if err != nil {
		return errors.New("unprotected scope preparation failed")
	}
	b, err = json.MarshalIndent(map[string]any{"run_id": c.RunID, "workspace_id": c.WorkspaceID, "plan_id": p.ID, "scope_hash": p.ContentHash, "hash_algorithm": domain.PlanHashAlgorithm, "status": p.Status, "action": p.Action, "approval": "PENDING_USER_CONFIRMATION", "targets": targets, "storage_requests": 0, "delete_requests": 0, "cleanup_requests": 0}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(output, append(b, '\n'), 0600)
}

// Registered recovery is read-only at the provider. It cannot create namespaces,
// identities, policies or versions, and must use the exact existing registry.
func inspectRegistered(ctx context.Context, path string, c config) error {
	if _, err := os.Stat(filepath.Join(c.OutputDirectory, "manifest.json")); !os.IsNotExist(err) {
		return errors.New("manifest already finalized; refuse preparation replay")
	}
	pool, err := pgxpool.New(ctx, c.PostgresDSN)
	if err != nil {
		return errors.New("fixture database unavailable")
	}
	defer pool.Close()
	rows, err := pool.Query(ctx, `SELECT c.id,c.dataset_version_id,n.namespace,c.object_key,c.provider_version,c.content_sha256,c.byte_size
	 FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id
	 JOIN dataset_version v ON v.id=c.dataset_version_id JOIN dataset d ON d.id=v.dataset_id
	 WHERE c.workspace_id=$1 AND n.provider_instance_id=$2 AND n.incarnation_id=$3 AND d.workspace_id=$1
	 AND v.checksum_algorithm='SHA256' AND v.checksum_value=c.content_sha256 AND v.byte_size=c.byte_size
	 ORDER BY n.namespace,c.object_key`, c.WorkspaceID, c.ProviderInstanceID, c.IncarnationID)
	if err != nil {
		return errors.New("registered fixture query failed")
	}
	var objects []object
	seen := map[string]bool{}
	for rows.Next() {
		var o object
		if err = rows.Scan(&o.CopyID, &o.DatasetVersionID, &o.Namespace, &o.Key, &o.Version, &o.SHA256, &o.Bytes); err != nil {
			rows.Close()
			return errors.New("registered fixture scan failed")
		}
		if (o.Namespace != namespaces[0] && o.Namespace != namespaces[1]) || (o.Key != "fixture/a.csv" && o.Key != "fixture/b.csv") || seen[o.Namespace+"/"+o.Key] {
			rows.Close()
			return errors.New("registered fixture scope mismatch")
		}
		seen[o.Namespace+"/"+o.Key] = true
		o.Retention = "NONE"
		if o.Namespace == namespaces[0] && o.Key == "fixture/b.csv" {
			o.LegalHold = true
		}
		// Native retention is recovered below, never cleared or overwritten.
		if o.Namespace == namespaces[1] && o.Key == "fixture/a.csv" {
			o.Retention = "GOVERNANCE"
		}
		o.copy = domain.Copy{ID: o.CopyID, WorkspaceID: c.WorkspaceID, DatasetVersionID: o.DatasetVersionID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: o.Namespace, Key: o.Key, ProviderVersion: o.Version, ContentSHA256: o.SHA256, ByteSize: o.Bytes}
		if o.copy.Validate() != nil {
			rows.Close()
			return errors.New("registered fixture identity invalid")
		}
		objects = append(objects, o)
	}
	rows.Close()
	if rows.Err() != nil || len(objects) != 4 {
		return errors.New("registered fixture membership incomplete")
	}
	var total int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_fixture_copy`).Scan(&total); err != nil || total != 4 {
		return errors.New("registered environment has foreign copies")
	}
	t := &bootstrapTransport{host: c.Endpoint, receipts: filepath.Join(c.OutputDirectory, "bootstrap-receipts.jsonl"), base: &http.Transport{Proxy: nil, DisableKeepAlives: true, ForceAttemptHTTP2: false, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, ResponseHeaderTimeout: 10 * time.Second}}
	root, err := minio.New(c.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath, MaxRetries: 1, Transport: t})
	if err != nil {
		return errors.New("registered inspection client invalid")
	}
	for i := range objects {
		o := &objects[i]
		payload := []byte("id,value\n1,synthetic-alpha\n")
		if o.Key == "fixture/b.csv" {
			payload = []byte("id,value\n2,synthetic-beta\n")
		}
		digest := sha256.Sum256(payload)
		if o.SHA256 != hex.EncodeToString(digest[:]) || o.Bytes != int64(len(payload)) {
			return errors.New("registered synthetic content identity mismatch")
		}
		if err = one(ctx, "reverify-existing-exact-bytes:"+o.Namespace+":"+o.Key, func(ctx context.Context) error {
			obj, e := root.GetObject(ctx, o.Namespace, o.Key, minio.GetObjectOptions{VersionID: o.Version})
			if e != nil {
				return e
			}
			defer obj.Close()
			b, e := io.ReadAll(io.LimitReader(obj, o.Bytes+1))
			if e != nil {
				return e
			}
			if !bytes.Equal(b, payload) {
				return errors.New("registered bytes mismatch")
			}
			info, e := obj.Stat()
			if e != nil {
				return e
			}
			if info.VersionID != o.Version || info.Size != o.Bytes {
				return errors.New("registered read identity mismatch")
			}
			o.BytesVerifiedAt = time.Now().UTC()
			return nil
		}); err != nil {
			return err
		}
	}
	return finish(ctx, path, c, pool, objects)
}

func finish(ctx context.Context, path string, c config, pool *pgxpool.Pool, objects []object) error {
	router, err := routing.NewRouter(false)
	if err != nil {
		return errors.New("fixture routing configuration failed")
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	versions := []uuid.UUID{}
	seenVersions := map[uuid.UUID]bool{}
	for _, o := range objects {
		if !seenVersions[o.DatasetVersionID] {
			versions = append(versions, o.DatasetVersionID)
			seenVersions[o.DatasetVersionID] = true
		}
	}
	if len(versions) != 2 {
		return errors.New("fixture content version scope mismatch")
	}
	a := authority{path: path, token: c.Token, run: c.RunID}
	repo := &infrastructure.PostgresRepository{}
	manager := transaction.NewManager(pool)
	occupancy := lc.NewOccupancyService(manager, repo, a, nil, nil)
	for _, v := range versions {
		if _, err = occupancy.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "fixture-restrict:" + v.String()}, DatasetVersionID: v, BasisRef: "closed-synthetic-fixture-no-business-reader"}); err != nil {
			return errors.New("fixture restriction failed")
		}
	}
	ids := make([]uuid.UUID, 0, len(objects))
	for _, o := range objects {
		ids = append(ids, o.CopyID)
	}
	plan, err := lc.NewPlanService(manager, repo, a).PrepareDeletePlan(ctx, lc.PrepareDeletePlanCommand{WorkspaceID: c.WorkspaceID, CopyIDs: ids, IdempotencyKey: "fixture-plan:" + c.RunID.String()})
	if err != nil {
		return errors.New("fixture plan preparation failed")
	}
	inspector, err := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret,
		WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.NewPostgresStorageCallJournal(occupancy)})
	if err != nil {
		return err
	}
	var priorCalls, priorObservations, priorCosts int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM lifecycle_storage_call),(SELECT count(*) FROM lifecycle_storage_observation),(SELECT count(*) FROM cost_allocation)`).Scan(&priorCalls, &priorObservations, &priorCosts); err != nil {
		return errors.New("fixture prior inspection facts unreadable")
	}
	for i := range objects {
		o := &objects[i]
		if _, err = inspector.InspectExactCopy(ctx, o.copy); err != nil {
			return fmt.Errorf("native exact inspection failed: %w", err)
		}
		p, e := inspector.InspectRetentionAndHold(ctx, o.copy)
		if e != nil {
			return fmt.Errorf("native protection inspection failed: %w", e)
		}
		if p.LegalHold != o.LegalHold || p.RetentionMode != o.Retention || (o.RetainUntil != nil && (p.RetainUntil == nil || !p.RetainUntil.Equal(*o.RetainUntil))) || (o.Retention == "GOVERNANCE" && p.RetainUntil == nil) {
			return errors.New("native protection mismatch")
		}
		o.RetainUntil = p.RetainUntil
		o.ProtectionObservedAt = p.ObservedAt
	}
	var inventories []lc.NamespaceInventory
	for _, ns := range namespaces {
		var subject domain.Copy
		expected := map[string]int64{}
		for _, o := range objects {
			if o.Namespace == ns {
				subject = o.copy
				expected[o.Key+"\x00"+o.Version] = o.Bytes
			}
		}
		inventory, e := inspector.InspectNamespace(ctx, subject)
		if e != nil {
			return fmt.Errorf("native inventory failed: %w", e)
		}
		if len(inventory.Versions) != len(expected) {
			return errors.New("native inventory membership mismatch")
		}
		for _, v := range inventory.Versions {
			size, ok := expected[v.Key+"\x00"+v.Version]
			if !ok || v.DeleteMarker || v.ByteSize != size {
				return errors.New("native inventory has unexpected version")
			}
		}
		inventories = append(inventories, inventory)
	}
	counts := map[string]int{}
	for _, table := range []string{"lifecycle_fixture_copy", "lifecycle_content_use_restriction", "lifecycle_disposition_approval", "lifecycle_disposition_dispatch", "lifecycle_provider_attempt", "lifecycle_reader_pin", "lifecycle_storage_call", "lifecycle_storage_observation", "cost_allocation"} {
		var count int
		if err = pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			return errors.New("fixture final facts query failed")
		}
		counts[table] = count
	}
	if counts["lifecycle_storage_call"] != priorCalls+14 || counts["lifecycle_storage_observation"] != priorObservations+14 || counts["cost_allocation"] != priorCosts+14 || counts["lifecycle_fixture_copy"] != 4 || counts["lifecycle_content_use_restriction"] != 2 || counts["lifecycle_disposition_approval"] != 0 || counts["lifecycle_disposition_dispatch"] != 0 || counts["lifecycle_provider_attempt"] != 0 || counts["lifecycle_reader_pin"] != 0 {
		return errors.New("fixture final fact counts mismatch")
	}
	manifest := map[string]any{"schema": "lifecycle307-predelete-manifest-v1", "created_at": time.Now().UTC(), "run_id": c.RunID, "workspace_id": c.WorkspaceID,
		"provider_instance_id": c.ProviderInstanceID, "incarnation_id": c.IncarnationID, "environment": json.RawMessage(c.Environment), "objects": objects, "inventories": inventories, "facts": counts,
		"plan":                 map[string]any{"id": plan.ID, "scope_hash": plan.ContentHash, "hash_algorithm": domain.PlanHashAlgorithm, "status": plan.Status, "approval": "PENDING_USER_CONFIRMATION", "targets": len(plan.Targets)},
		"destructive_requests": 0, "object_cleanup_requests": 0, "inspection_identity_policy": "lifecycle307-inspection-" + c.RunID.String(),
		"acceptance": map[string]string{"native_exact_bytes": "PASS", "native_version_inventory": "PASS", "native_retention_hold": "PASS", "platform_per_call_journal_cost": "PASS",
			"delete": "NOT_RUN", "absence_finalize": "NOT_RUN", "archive_restore": "NOT_RUN", "business_reader": "NOT_RUN", "full_D1": "NOT_RUN"}}
	b, _ := json.MarshalIndent(manifest, "", "  ")
	if err = os.WriteFile(filepath.Join(c.OutputDirectory, "manifest.json"), append(b, '\n'), 0600); err != nil {
		return err
	}
	fmt.Println("Fixture prepared; approval pending. Manifest:", filepath.Join(c.OutputDirectory, "manifest.json"))
	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	path := os.Getenv("LIFECYCLE_FIXTURE_CONFIG")
	if path == "" {
		fmt.Fprintln(os.Stderr, "LIFECYCLE_FIXTURE_CONFIG private path required")
		os.Exit(1)
	}
	if err := prepare(ctx, path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
