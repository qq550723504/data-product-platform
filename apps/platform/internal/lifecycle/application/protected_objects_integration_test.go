package application_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	da "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	di "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	li "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	ri "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

// Controlled port, not native-provider evidence. The same immutable PG journal
// and inventory constraints are exercised in CI without a MinIO service.
type protectedControlledPort struct {
	s            *lc.ProtectedObjects
	mu           sync.Mutex
	objects      map[string][]byte
	lostResponse bool
	denied       bool
	calls        int
}

func (p *protectedControlledPort) call(ctx context.Context, i lc.ProtectedObjectIntent, method string, data []byte, status int, failure string) error {
	id, err := p.s.StartProtectedCall(ctx, i, method)
	if err != nil {
		return err
	}
	at := time.Now().UTC()
	h := sha256.Sum256(data)
	return p.s.ObserveProtectedCall(ctx, lc.StorageCallObservation{CallID: id, InvokedAt: &at, ObservedAt: at, HTTPStatus: status, ResponseSHA256: hex.EncodeToString(h[:]), Failure: failure})
}
func (p *protectedControlledPort) CreateProtected(ctx context.Context, i lc.ProtectedObjectIntent, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.denied {
		if err := p.call(ctx, i, "PUT", nil, 403, ""); err != nil {
			return err
		}
		return lc.ErrStorageDenied
	}
	if _, ok := p.objects[i.Key]; !ok {
		p.objects[i.Key] = append([]byte(nil), data...)
	}
	if p.lostResponse {
		if err := p.call(ctx, i, "PUT", nil, 0, "TRANSPORT"); err != nil {
			return err
		}
		return lc.ErrStorageUnknown
	}
	return p.call(ctx, i, "PUT", nil, 200, "")
}
func (p *protectedControlledPort) ReadProtected(ctx context.Context, i lc.ProtectedObjectIntent) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	data, ok := p.objects[i.Key]
	if !ok {
		return nil, lc.ErrStorageNotFound
	}
	if err := p.call(ctx, i, "GET", data, 200, ""); err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

type retainedFixtureStore struct{}

func (retainedFixtureStore) Put(_ context.Context, _ string, _ io.Reader, _ int64, _ string) (string, error) {
	return "s3://raw/datasets/" + uuid.NewString() + "/old.csv", nil
}

func TestProtectedObjectsControlledPostgresContract(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	router, e := routing.NewRouter(false)
	if e != nil {
		t.Fatal(e)
	}
	outbox.ConfigureAppendObligation(router)
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	scope := lc.ProtectedScope{WorkspaceID: uuid.New(), ActorID: uuid.New(), NamespaceID: uuid.New(), ProviderInstanceID: uuid.New(), IncarnationID: uuid.New(), PrincipalRef: "issue330-controlled-owner", Bucket: "raw", ExpiresAt: time.Now().Add(time.Hour)}
	s, err := lc.NewProtectedObjects(pool, scope)
	if err != nil {
		t.Fatal(err)
	}
	port := &protectedControlledPort{s: s, objects: map[string][]byte{}}
	if err = s.BindPort(port); err != nil {
		t.Fatal(err)
	}
	tx := transaction.NewManager(pool)
	repo := di.NewPostgresRepository(pool)
	create := da.NewCreateDatasetService(tx, repo, ri.NewPostgresRepository())
	newDataset := func(w uuid.UUID) dd.Dataset {
		d, e := create.Handle(ctx, da.CreateDatasetCommand{WorkspaceID: w, Code: uuid.NewString(), Name: "controlled object", DatasetType: dd.DatasetTypeCurated, ActorID: &scope.ActorID})
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	upload := da.NewUploadVersionService(tx, repo, s).WithPublicationLease(s)
	data := []byte("id,value\n1,unchanged\n")
	d := newDataset(scope.WorkspaceID)
	v, err := upload.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "result.csv", ContentType: "text/csv", Content: data, IdempotencyKey: "new-output"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.StorageURI, "s3://raw/core/"+scope.WorkspaceID.String()+"/datasets/"+d.ID.String()+"/") {
		t.Fatal("workspace namespace absent")
	}
	before := port.calls
	replay, err := upload.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "result.csv", ContentType: "text/csv", Content: data, IdempotencyKey: "new-output"})
	if err != nil || replay.ID != v.ID || port.calls != before {
		t.Fatalf("published replay mutated provider: %v", err)
	}
	r, err := s.ReadVersion(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("read mismatch")
	}
	t.Run("namespace identities do not overlap physical ownership", func(t *testing.T) {
		_, e := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace) VALUES($1,$2,$3,$4,$5)`, uuid.New(), uuid.New(), scope.ProviderInstanceID, scope.IncarnationID, scope.Bucket)
		if e == nil {
			t.Fatal("overlapping whole-bucket namespace admitted")
		}
		other := uuid.New()
		_, e = pool.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace,identity_kind,object_prefix) VALUES($1,$2,$3,$4,$5,'PROTECTED_CREATE',$6)`, uuid.New(), other, scope.ProviderInstanceID, scope.IncarnationID, scope.Bucket, "core/"+other.String()+"/")
		if e != nil {
			t.Fatal(e)
		}
		_, e = pool.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace,identity_kind,object_prefix) VALUES($1,$2,$3,$4,$5,'PROTECTED_CREATE',$6)`, uuid.New(), scope.WorkspaceID, scope.ProviderInstanceID, scope.IncarnationID, scope.Bucket, scope.Prefix())
		if e == nil {
			t.Fatal("duplicate physical prefix admitted with another namespace ID")
		}
	})

	t.Run("foreign workspace denied before allocation and I/O", func(t *testing.T) {
		foreign := newDataset(uuid.New())
		before := port.calls
		_, err := upload.Handle(ctx, da.UploadVersionCommand{DatasetID: foreign.ID, Filename: "x.csv", ContentType: "text/csv", Content: data})
		if !errors.Is(err, lc.ErrStorageDenied) || port.calls != before {
			t.Fatalf("foreign write: %v", err)
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, foreign.ID).Scan(&n)
		if n != 0 {
			t.Fatal("foreign version allocated")
		}
	})
	t.Run("lost PUT response recovers bytes and records both calls", func(t *testing.T) {
		port.lostResponse = true
		defer func() { port.lostResponse = false }()
		d := newDataset(scope.WorkspaceID)
		v, e := upload.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "lost.csv", ContentType: "text/csv", Content: data})
		if e != nil || v.Status != dd.VersionReady {
			t.Fatalf("lost response recovery: %v", e)
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_storage_observation o JOIN lifecycle_storage_call c ON c.id=o.call_id JOIN lifecycle_protected_object_intent i ON i.id=c.protected_object_intent_id WHERE i.dataset_version_id=$1 AND o.failure='TRANSPORT'`, v.ID).Scan(&n)
		if n != 1 {
			t.Fatal("unknown observation lost")
		}
	})
	t.Run("provider denial has cost but no verified copy or READY", func(t *testing.T) {
		port.denied = true
		defer func() { port.denied = false }()
		d := newDataset(scope.WorkspaceID)
		_, e := upload.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "denied.csv", ContentType: "text/csv", Content: data})
		if !errors.Is(e, lc.ErrStorageDenied) {
			t.Fatalf("denial: %v", e)
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_fixture_copy c JOIN dataset_version v ON v.id=c.dataset_version_id WHERE v.dataset_id=$1`, d.ID).Scan(&n)
		if n != 0 {
			t.Fatal("denied output registered")
		}
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation a JOIN lifecycle_storage_call c ON c.id=a.lifecycle_storage_call_id JOIN lifecycle_protected_object_intent i ON i.id=c.protected_object_intent_id JOIN dataset_version v ON v.id=i.dataset_version_id WHERE v.dataset_id=$1`, d.ID).Scan(&n)
		if n != 1 {
			t.Fatal("failed physical invocation cost missing")
		}
	})
	t.Run("retained URI/version unchanged and replay has no I/O", func(t *testing.T) {
		d := newDataset(scope.WorkspaceID)
		oldUpload := da.NewUploadVersionService(tx, repo, retainedFixtureStore{})
		old, e := oldUpload.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "old.csv", ContentType: "text/csv", Content: data})
		if e != nil {
			t.Fatal(e)
		}
		var rowBefore, rowAfter string
		_ = pool.QueryRow(ctx, `SELECT to_jsonb(v)::text FROM dataset_version v WHERE id=$1`, old.ID).Scan(&rowBefore)
		if _, e = s.ReadVersion(ctx, old.ID); !errors.Is(e, lc.ErrStorageScope) {
			t.Fatal("legacy root fallback")
		}
		uri, e := s.AddRetainedCopy(ctx, old.ID, "one-custody-command", data)
		if e != nil {
			t.Fatal(e)
		}
		before := port.calls
		again, e := s.AddRetainedCopy(ctx, old.ID, "one-custody-command", data)
		if e != nil || uri != again || port.calls != before {
			t.Fatal("copy replay wrote again")
		}
		r, e := s.Get(ctx, old.StorageURI)
		if e != nil {
			t.Fatal(e)
		}
		got, _ := io.ReadAll(r)
		_ = r.Close()
		if !bytes.Equal(got, data) {
			t.Fatal("retained resolution mismatch")
		}
		_ = pool.QueryRow(ctx, `SELECT to_jsonb(v)::text FROM dataset_version v WHERE id=$1`, old.ID).Scan(&rowAfter)
		if rowAfter != rowBefore {
			t.Fatal("historical version changed")
		}
		if _, e = s.AddRetainedCopy(ctx, old.ID, "one-custody-command", []byte("wrong")); !errors.Is(e, lc.ErrStorageContract) {
			t.Fatal("copy accepts different bytes")
		}
		other := newDataset(scope.WorkspaceID)
		otherVersion, e := oldUpload.Handle(ctx, da.UploadVersionCommand{DatasetID: other.ID, Filename: "other.csv", ContentType: "text/csv", Content: data})
		if e != nil {
			t.Fatal(e)
		}
		before = port.calls
		if _, e = s.AddRetainedCopy(ctx, otherVersion.ID, "one-custody-command", data); !errors.Is(e, lc.ErrIdempotencyConflict) || before != port.calls {
			t.Fatal("same copy key accepted another version")
		}
		var copyID uuid.UUID
		_ = pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_copy WHERE dataset_version_id=$1`, old.ID).Scan(&copyID)
		e = tx.Do(ctx, func(ctx context.Context, txn pgx.Tx) error {
			_, e := (&li.PostgresRepository{}).LoadFixtureCopies(ctx, txn, scope.WorkspaceID, []uuid.UUID{copyID})
			return e
		})
		if !errors.Is(e, li.ErrScopeUnavailable) {
			t.Fatal("protected copy exposed as provider version")
		}
		_, e = pool.Exec(ctx, `INSERT INTO lifecycle_disposition_plan_target(plan_id,workspace_id,copy_id) VALUES($1,$2,$3)`, uuid.New(), scope.WorkspaceID, copyID)
		if e == nil || !strings.Contains(e.Error(), "protected object destructive capability unsupported") {
			t.Fatalf("protected identity admitted to a delete plan: %v", e)
		}
		_, e = pool.Exec(ctx, `INSERT INTO lifecycle_copy_operation(id,workspace_id,source_copy_id) VALUES($1,$2,$3)`, uuid.New(), scope.WorkspaceID, copyID)
		if e == nil || !strings.Contains(e.Error(), "protected object destructive capability unsupported") {
			t.Fatalf("protected identity admitted to archive/restore: %v", e)
		}
		if _, e = pool.Exec(ctx, `UPDATE lifecycle_fixture_copy SET content_sha256=repeat('0',64) WHERE id=$1`, copyID); e == nil {
			t.Fatal("copy identity mutable")
		}
		if _, e = pool.Exec(ctx, `UPDATE lifecycle_protected_object_intent SET byte_size=1 WHERE dataset_version_id=$1`, old.ID); e == nil {
			t.Fatal("creation identity mutable")
		}
	})
	t.Run("concurrent custody command shares one physical location", func(t *testing.T) {
		d := newDataset(scope.WorkspaceID)
		legacy := da.NewUploadVersionService(tx, repo, retainedFixtureStore{})
		old, e := legacy.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "concurrent.csv", ContentType: "text/csv", Content: data})
		if e != nil {
			t.Fatal(e)
		}
		type result struct {
			uri string
			err error
		}
		results := make(chan result, 2)
		start := make(chan struct{})
		for n := 0; n < 2; n++ {
			go func() {
				<-start
				uri, e := s.AddRetainedCopy(ctx, old.ID, "concurrent-custody", data)
				results <- result{uri, e}
			}()
		}
		close(start)
		a, b := <-results, <-results
		if a.err != nil || b.err != nil || a.uri != b.uri {
			t.Fatalf("concurrent copy: %v %v", a.err, b.err)
		}
		var n int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_fixture_copy WHERE dataset_version_id=$1`, old.ID).Scan(&n)
		if n != 1 {
			t.Fatal("duplicate physical location")
		}
	})
	t.Run("first copy and real source FK serialize in both orders", func(t *testing.T) {
		for _, sourceFirst := range []bool{true, false} {
			t.Run(fmt.Sprint(sourceFirst), func(t *testing.T) {
				legacy := da.NewUploadVersionService(tx, repo, retainedFixtureStore{})
				makeVersion := func() dd.DatasetVersion {
					d := newDataset(scope.WorkspaceID)
					v, e := legacy.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "barrier.csv", ContentType: "text/csv", Content: data})
					if e != nil {
						t.Fatal(e)
					}
					return v
				}
				input, output := makeVersion(), makeVersion()
				sum := sha256.Sum256(data)
				i := lc.ProtectedObjectIntent{ID: uuid.New(), WorkspaceID: scope.WorkspaceID, NamespaceID: scope.NamespaceID, DatasetVersionID: input.ID, Purpose: "RETAINED_COPY", Key: scope.Prefix() + "barrier/" + uuid.NewString(), SHA256: hex.EncodeToString(sum[:]), ByteSize: int64(len(data)), ContentType: "text/csv"}
				_, e := pool.Exec(ctx, `INSERT INTO lifecycle_protected_object_intent(id,workspace_id,namespace_id,dataset_version_id,purpose,object_key,content_sha256,byte_size,content_type,actor_id,principal_ref,dependency_revision,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,0,$12)`, i.ID, i.WorkspaceID, i.NamespaceID, i.DatasetVersionID, i.Purpose, i.Key, i.SHA256, i.ByteSize, i.ContentType, scope.ActorID, scope.PrincipalRef, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				put, e := s.StartProtectedCall(ctx, i, "PUT")
				if e != nil {
					t.Fatal(e)
				}
				at := time.Now().UTC()
				if e = s.ObserveProtectedCall(ctx, lc.StorageCallObservation{CallID: put, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200}); e != nil {
					t.Fatal(e)
				}
				get, e := s.StartProtectedCall(ctx, i, "GET")
				if e != nil {
					t.Fatal(e)
				}
				at = time.Now().UTC()
				if e = s.ObserveProtectedCall(ctx, lc.StorageCallObservation{CallID: get, ObservedAt: at, InvokedAt: &at, HTTPStatus: 200, ResponseSHA256: i.SHA256}); e != nil {
					t.Fatal(e)
				}
				proof, copyID := uuid.New(), uuid.New()
				_, e = pool.Exec(ctx, `INSERT INTO lifecycle_protected_object_verification(id,workspace_id,intent_id,create_call_id,read_call_id,content_sha256,byte_size,verified_at,creation_observation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'CONFIRMED')`, proof, scope.WorkspaceID, i.ID, put, get, i.SHA256, i.ByteSize, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				insertCopy := func(txn pgx.Tx) error {
					_, e := txn.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size,identity_kind,protected_verification_id) VALUES($1,$2,$3,$4,$5,NULL,$6,$7,'PROTECTED_CREATE',$8)`, copyID, scope.WorkspaceID, scope.NamespaceID, input.ID, i.Key, i.SHA256, i.ByteSize, proof)
					return e
				}
				insertSource := func(txn pgx.Tx) error {
					_, e := txn.Exec(ctx, `INSERT INTO dataset_version_lineage(id,output_version_id,input_version_id,relation_type) VALUES($1,$2,$3,'DERIVED_FROM')`, uuid.New(), output.ID, input.ID)
					return e
				}
				first, e := pool.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer first.Rollback(context.Background())
				if sourceFirst {
					e = insertSource(first)
				} else {
					e = insertCopy(first)
				}
				if e != nil {
					t.Fatal(e)
				}
				second, e := pool.Acquire(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer second.Release()
				pid := second.Conn().PgConn().PID()
				result := make(chan error, 1)
				go func() {
					txn, e := second.Begin(ctx)
					if e != nil {
						result <- e
						return
					}
					defer txn.Rollback(context.Background())
					if sourceFirst {
						e = insertCopy(txn)
					} else {
						e = insertSource(txn)
					}
					if e == nil {
						e = txn.Commit(ctx)
					}
					result <- e
				}()
				deadline := time.Now().Add(4 * time.Second)
				waiting := false
				for time.Now().Before(deadline) {
					var wait *string
					_ = pool.QueryRow(ctx, `SELECT wait_event_type FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&wait)
					if wait != nil && *wait == "Lock" {
						waiting = true
						break
					}
					select {
					case e := <-result:
						t.Fatalf("late transaction crossed admission fence: %v", e)
					default:
					}
					time.Sleep(10 * time.Millisecond)
				}
				if !waiting {
					t.Fatal("real PostgreSQL lock wait not observed")
				}
				if e = first.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				select {
				case e = <-result:
					if e != nil {
						t.Fatal(e)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("fenced transaction did not finish")
				}
				var n int
				_ = pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version_lineage WHERE input_version_id=$1 AND output_version_id=$2`, input.ID, output.ID).Scan(&n)
				if n != 1 {
					t.Fatal("source fact lost across first-copy admission")
				}
			})
		}
	})
	t.Run("restriction and expired identity deny read with zero new calls", func(t *testing.T) {
		_, e := pool.Exec(ctx, `INSERT INTO lifecycle_content_use_restriction(id,workspace_id,dataset_version_id,basis_ref,occurred_at,actor_id,principal_ref,dependency_revision) VALUES($1,$2,$3,'TEST',$4,$5,'test',1)`, uuid.New(), scope.WorkspaceID, v.ID, time.Now(), scope.ActorID)
		if e != nil {
			t.Fatal(e)
		}
		before := port.calls
		if _, e = s.ReadVersion(ctx, v.ID); !errors.Is(e, lc.ErrStorageDenied) || port.calls != before {
			t.Fatalf("restriction not enforced: %v", e)
		}
		expired := scope
		expired.ExpiresAt = time.Now().Add(-time.Second)
		x, e := lc.NewProtectedObjects(pool, expired)
		if e != nil {
			t.Fatal(e)
		}
		_ = x.BindPort(port)
		if _, e = x.ReadVersion(ctx, v.ID); !errors.Is(e, lc.ErrStorageDenied) {
			t.Fatal("expired scope read")
		}
	})
}
