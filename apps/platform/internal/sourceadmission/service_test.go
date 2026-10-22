package sourceadmission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	ld "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	ra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	ri "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

// Controlled provider Port with the real protected-object PostgreSQL journal.
// This is not evidence of native MinIO or real Fork integration.
type testPort struct {
	mu         sync.Mutex
	objects    map[string][]byte
	writes     int
	s          *lc.ProtectedObjects
	failReads  int
	afterWrite bool
}

func (p *testPort) observe(ctx context.Context, i lc.ProtectedObjectIntent, method string, data []byte, status int) error {
	id, e := p.s.StartProtectedCall(ctx, i, method)
	if e != nil {
		return e
	}
	at := time.Now().UTC()
	return p.s.ObserveProtectedCall(ctx, lc.StorageCallObservation{CallID: id, InvokedAt: &at, ObservedAt: at, HTTPStatus: status, ResponseSHA256: Hash(data)})
}
func (p *testPort) CreateProtected(ctx context.Context, i lc.ProtectedObjectIntent, b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes++
	if old, ok := p.objects[i.Key]; ok {
		if !bytes.Equal(old, b) {
			return ErrIntegrity
		}
	} else {
		p.objects[i.Key] = bytes.Clone(b)
	}
	if e := p.observe(ctx, i, "PUT", nil, 200); e != nil {
		return e
	}
	if p.afterWrite {
		p.afterWrite = false
		p.failReads = 1
		return lc.ErrStorageUnknown
	}
	return nil
}
func (p *testPort) ReadProtected(ctx context.Context, i lc.ProtectedObjectIntent) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failReads > 0 {
		p.failReads--
		return nil, lc.ErrStorageUnknown
	}
	b, ok := p.objects[i.Key]
	if !ok {
		return nil, lc.ErrStorageNotFound
	}
	if e := p.observe(ctx, i, "GET", b, 200); e != nil {
		return nil, e
	}
	return bytes.Clone(b), nil
}

type fixture struct {
	pool                   *pgxpool.Pool
	s                      *Service
	c                      Connection
	r                      Request
	p                      principal.RequestPrincipal
	caller                 Caller
	port                   *testPort
	allowed, sourceAllowed atomic.Bool
	sourceCalls            atomic.Int64
	source                 *httptest.Server
	blobs                  map[string][]byte
	manifest               Manifest
	declaration            uuid.UUID
	beforeAsset            func()
	sourceResponse         func(http.ResponseWriter, *http.Request, string) bool
	sourceMu               sync.Mutex
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	pool, e := pgxpool.New(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	router, e := routing.NewRouter(false)
	if e != nil {
		t.Fatal(e)
	}
	outbox.ConfigureAppendObligation(router)
	f := &fixture{pool: pool, blobs: map[string][]byte{}}
	f.allowed.Store(true)
	f.sourceAllowed.Store(true)
	for _, name := range []string{"manifest.json", "records.json", "records.jsonl"} {
		b, e := os.ReadFile(filepath.Join("testdata", name))
		if e != nil {
			t.Fatal(e)
		}
		f.blobs[name] = b
	}
	if e = json.Unmarshal(f.blobs["manifest.json"], &f.manifest); e != nil {
		t.Fatal(e)
	}
	ws, actor, resource, dataset := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.p = principal.RequestPrincipal{Subject: "approved-source-test", ActorID: actor, WorkspaceID: ws, Capabilities: map[string]struct{}{Capability: {}}}
	f.c = Connection{ID: uuid.New(), WorkspaceID: ws, Namespace: uuid.MustParse(f.manifest.Content.Namespace), DatasetID: dataset, ResourceID: resource, Project: f.manifest.Content.Project, Organization: f.manifest.Content.Organization, Mode: "fork_approved", Consumer: "HOST-SOURCE-TEST", Purpose: "SOURCE_ADMISSION", Scope: rd.NormalizedScope{Type: "ALL_RESOURCE", Ref: resource.String()}}
	f.r = Request{WorkspaceID: ws, DatasetID: dataset, ConnectionID: f.c.ID, ReleaseID: uuid.MustParse(f.manifest.Receipt.ID), SnapshotID: uuid.MustParse(f.manifest.Snapshot), ManifestSHA: Hash(f.blobs["manifest.json"]), Key: "source-operation"}
	ctx := context.Background()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO data_resource(id,workspace_id,code,name,resource_type,lifecycle_status) VALUES($1,$2,$3,'synthetic annotation source','FILE_COLLECTION','READY')`, []any{resource, ws, resource.String()}},
		{`INSERT INTO dataset(id,workspace_id,code,name,dataset_type,source_resource_id,lifecycle_status) VALUES($1,$2,$3,'synthetic source RAW','RAW',$4,'ACTIVE')`, []any{dataset, ws, dataset.String(), resource}},
	} {
		if _, e = pool.Exec(ctx, q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	rights := ra.NewService(transaction.NewManager(pool), ri.NewPostgresRepository(pool))
	declaration, e := rights.CreateRightsDeclaration(ctx, ra.CreateRightsDeclarationCommand{Spec: rd.RightsDeclarationSpec{WorkspaceID: ws, DataResourceID: resource, ClaimantRef: f.c.Consumer, BasisType: "LICENSE", BasisRef: "synthetic-source-only", ConsumerScopeType: "EXPLICIT", ConsumerRef: f.c.Consumer, Parties: []rd.RightsParty{{PartyRef: f.c.Consumer, Role: "RIGHTS_HOLDER"}}, Permissions: []rd.RightsPermission{{Kind: rd.PermissionUse, Action: "READ", Purpose: f.c.Purpose, Scope: f.c.Scope}, {Kind: rd.PermissionUse, Action: "PROCESS", Purpose: f.c.Purpose, Scope: f.c.Scope}}}})
	if e != nil {
		t.Fatal(e)
	}
	f.declaration = declaration.ID
	if _, e = rights.VerifyRightsDeclaration(ctx, ra.VerifyRightsDeclarationCommand{DeclarationID: declaration.ID, Outcome: rd.DeclarationVerified}); e != nil {
		t.Fatal(e)
	}
	f.source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.sourceCalls.Add(1)
		if !f.sourceAllowed.Load() || r.Header.Get("Authorization") != "Token synthetic-fork-token" {
			w.WriteHeader(403)
			return
		}
		f.sourceMu.Lock()
		defer f.sourceMu.Unlock()
		name := ""
		base := "/api/projects/" + f.c.Project + "/approved-dataset-releases/" + f.r.ReleaseID.String() + "/snapshots/" + f.r.SnapshotID.String() + "/"
		if r.URL.Path == base+"manifest/" {
			name = "manifest.json"
		} else {
			for _, a := range f.manifest.Artifacts {
				if r.URL.Path == base+"assets/"+a.ID+"/download/" {
					name = a.Name
				}
			}
		}
		if f.beforeAsset != nil && name == "records.json" {
			f.beforeAsset()
		}
		if name == "" {
			w.WriteHeader(404)
			return
		}
		if f.sourceResponse != nil && f.sourceResponse(w, r, name) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(f.blobs[name])
	}))
	t.Cleanup(f.source.Close)
	f.c.BaseURL = f.source.URL
	token := filepath.Join(t.TempDir(), "fork-token")
	if e = os.WriteFile(token, []byte("synthetic-fork-token"), 0600); e != nil {
		t.Fatal(e)
	}
	f.c.TokenFile = token
	f.s = New(pool)
	f.s.Source = NewHTTPSource()
	f.s.Load = func(_ context.Context, id uuid.UUID) (Connection, error) {
		if id != f.c.ID {
			return Connection{}, ErrDenied
		}
		return f.c, nil
	}
	f.caller = func(_ context.Context, ws uuid.UUID) (principal.RequestPrincipal, error) {
		if !f.allowed.Load() || ws != f.p.WorkspaceID {
			return principal.RequestPrincipal{}, ErrDenied
		}
		return f.p, nil
	}
	scope := lc.ProtectedScope{WorkspaceID: ws, ActorID: actor, NamespaceID: uuid.New(), ProviderInstanceID: uuid.New(), IncarnationID: uuid.New(), PrincipalRef: f.p.Subject, Bucket: "hfa-synthetic", ExpiresAt: time.Now().Add(time.Hour)}
	objects, e := lc.NewProtectedObjects(pool, scope)
	if e != nil {
		t.Fatal(e)
	}
	f.port = &testPort{objects: map[string][]byte{}, s: objects}
	if e = objects.BindPort(f.port); e != nil {
		t.Fatal(e)
	}
	store := &evidenceStore{ProtectedObjects: objects, pool: pool, scope: scope, port: f.port}
	f.s.Store = func(Connection) (VersionStore, error) { return store, nil }
	if e = f.s.RegisterConnection(ctx, f.c); e != nil {
		t.Fatal(e)
	}
	return f
}
func (f *fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if e := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func (f *fixture) admit(t *testing.T) Receipt {
	t.Helper()
	r, e := f.s.Admit(context.Background(), f.r, f.caller)
	if e != nil {
		t.Fatal(e)
	}
	if r.State != "ACCEPTED_SOURCE" || r.RowCount != 3 || r.Gold || r.Published || r.VersionID == nil {
		t.Fatalf("bad receipt %+v", r)
	}
	return r
}

func TestGoldenManifestAndNativeIntegrity(t *testing.T) {
	blobs := map[string][]byte{}
	for _, name := range []string{"manifest.json", "records.json", "records.jsonl"} {
		b, e := os.ReadFile(filepath.Join("testdata", name))
		if e != nil {
			t.Fatal(e)
		}
		blobs[name] = b
	}
	var original Manifest
	_ = json.Unmarshal(blobs["manifest.json"], &original)
	c := Connection{Namespace: uuid.MustParse(original.Content.Namespace), Project: original.Content.Project, Organization: original.Content.Organization, Mode: "fork_approved"}
	r := Request{SnapshotID: uuid.MustParse(original.Snapshot), ReleaseID: uuid.MustParse(original.Receipt.ID), ManifestSHA: Hash(blobs["manifest.json"])}
	m, e := ValidateManifest(blobs["manifest.json"], c, r)
	if e != nil {
		t.Fatal(e)
	}
	if e = ValidateBundle(m, blobs); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"records.json", "records.jsonl"} {
		t.Run("tampered "+name, func(t *testing.T) {
			b := map[string][]byte{}
			for k, v := range blobs {
				b[k] = v
			}
			b[name] = append(bytes.Clone(b[name]), ' ')
			if ValidateBundle(m, b) == nil {
				t.Fatal("tampered native accepted")
			}
		})
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]any)
	}{
		{"core-only denied", func(v map[string]any) { v["origin_mode"] = "core_only_internal" }},
		{"unsupported media", func(v map[string]any) { v["content"].(map[string]any)["media_profile"] = "managed-upload-capture/v1" }},
		{"changed content", func(v map[string]any) { v["content"].(map[string]any)["project_id"] = "999" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var v map[string]any
			_ = json.Unmarshal(blobs["manifest.json"], &v)
			tc.edit(v)
			b, _ := json.Marshal(v)
			b, _ = jsoncanonicalizer.Transform(b)
			rr := r
			rr.ManifestSHA = Hash(b)
			if _, e := ValidateManifest(b, c, rr); e == nil {
				t.Fatal("invalid envelope accepted")
			}
		})
	}
	for _, b := range []string{`{"x":1,"x":2}`, `{} {}`, "{\"a\":\"\xff\"}"} {
		var v any
		if strictJSON([]byte(b), &v) == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
}

func TestTextProfileRejectsAmbiguousXML(t *testing.T) {
	for _, config := range []string{`<View><Text value="$text"/></View><View/>`, `text<View><Text value="$text"/></View>`, `<?xml-stylesheet href="https://example.invalid"?><View><Text value="$text"/></View>`, `<Text value="$text"/>`} {
		if _, e := textFields(config); !errors.Is(e, ErrUnsupported) {
			t.Fatalf("ambiguous XML accepted: %q: %v", config, e)
		}
	}
}

func TestPostgresLifecycleReferenceQualificationAndInvalidReplay(t *testing.T) {
	f := newFixture(t)
	rec := f.admit(t)
	ctx := context.Background()
	if e := f.s.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		check, refs, e := lc.InspectFixtureReferences(ctx, tx, ld.Copy{WorkspaceID: f.r.WorkspaceID, DatasetVersionID: *rec.VersionID})
		if e != nil {
			return e
		}
		if check != ld.Denied {
			t.Fatalf("accepted source evidence must pin content: %s", check)
		}
		found := false
		for _, ref := range refs {
			if ref.Table == "source_admission_capture" && ref.Column == "version_id" {
				found = ref.Count == 1 && ref.Unreleased == 1
			}
		}
		if !found {
			t.Fatal("source version reference missing from existing lifecycle inspection")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(ctx, `UPDATE dataset_version SET status='INVALID',invalidated_at=now(),invalidation_reason='synthetic revocation' WHERE id=$1`, *rec.VersionID); e != nil {
		t.Fatal(e)
	}
	writes := f.port.writes
	if _, e := f.s.Admit(ctx, f.r, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatalf("invalid version replay: %v", e)
	}
	if f.port.writes != writes {
		t.Fatal("invalid version replay wrote storage")
	}
}
func TestPostgresAdmissionReplayAndCopyClosure(t *testing.T) {
	f := newFixture(t)
	first := f.admit(t)
	writes := f.port.writes
	same := f.admit(t)
	if *same.VersionID != *first.VersionID || same.ID != first.ID {
		t.Fatal("same-key duplicated version")
	}
	f.r.Key = "another-key"
	other := f.admit(t)
	if *other.VersionID != *first.VersionID || other.ID == first.ID {
		t.Fatal("snapshot mapping not reused")
	}
	if f.port.writes != writes {
		t.Fatal("accepted replay rewrote objects")
	}
	if n := f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID); n != 1 {
		t.Fatalf("versions=%d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM lifecycle_fixture_copy WHERE dataset_version_id=$1`, first.VersionID); n != 3 {
		t.Fatalf("protected objects=%d", n)
	}
	for name, mediaType := range map[string]string{"manifest.json": "application/json", "records.json": "application/json", "records.jsonl": "application/x-ndjson"} {
		if n := f.count(t, `SELECT count(*) FROM lifecycle_protected_object_intent WHERE dataset_version_id=$1 AND purpose='CREATE' AND object_key LIKE '%/'||$2 AND content_type=$3 AND content_sha256=$4`, first.VersionID, name, mediaType, Hash(f.blobs[name])); n != 1 {
			t.Fatalf("frozen media type for %s: matching intents=%d", name, n)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_member WHERE capture_id=$1`, first.CaptureID); n != 3 {
		t.Fatalf("members=%d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='SourceAdmissionAccepted'`, first.CaptureID); n != 1 {
		t.Fatalf("accepted events=%d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_call c LEFT JOIN cost_allocation a ON a.source_admission_call_id=c.id WHERE c.workspace_id=$1 AND a.id IS NULL`, f.c.WorkspaceID); n != 0 {
		t.Fatal("unallocated physical attempts")
	}
	got, e := f.s.Lookup(context.Background(), f.r.WorkspaceID, other.ID, f.caller)
	if e != nil || *got.VersionID != *first.VersionID {
		t.Fatalf("lost-response lookup: %v", e)
	}
	f.sourceAllowed.Store(false)
	if _, e = f.s.Admit(context.Background(), f.r, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatalf("source revoked replay: %v", e)
	}
}
func TestPostgresPermissionsAndRevokedStaleRequests(t *testing.T) {
	for _, kind := range []string{"workspace", "dataset", "connection", "capability", "disabled", "rights"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			r := f.r
			switch kind {
			case "workspace":
				r.WorkspaceID = uuid.New()
			case "dataset":
				r.DatasetID = uuid.New()
			case "connection":
				r.ConnectionID = uuid.New()
			case "capability":
				f.p.Capabilities = nil
			case "disabled":
				_, _ = f.pool.Exec(context.Background(), `UPDATE source_admission_connection SET enabled=false WHERE id=$1`, f.c.ID)
			case "rights":
				_, e := ra.NewService(f.s.Tx, ri.NewPostgresRepository(f.pool)).DisposeRightsDeclaration(context.Background(), ra.DisposeRightsDeclarationCommand{DeclarationID: f.declaration, Disposition: rd.DispositionInvalidated, EffectiveAt: time.Now().UTC(), Reason: "synthetic permission negative"})
				if e != nil {
					t.Fatal(e)
				}
			}
			if _, e := f.s.Admit(context.Background(), r, f.caller); !errors.Is(e, ErrDenied) {
				t.Fatalf("denial=%v", e)
			}
			if f.sourceCalls.Load() != 0 || f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID) != 0 {
				t.Fatal("denied request performed work")
			}
		})
	}
	f := newFixture(t)
	r := f.admit(t)
	f.allowed.Store(false)
	if _, e := f.s.Admit(context.Background(), f.r, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatalf("stale replay=%v", e)
	}
	if _, e := f.s.Lookup(context.Background(), f.r.WorkspaceID, r.ID, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatal("stale lookup leaked receipt")
	}
}
func TestPostgresConcurrentSameSnapshotAndIdempotencyConflict(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan Receipt, 6)
	errs := make(chan error, 6)
	for k := 0; k < 6; k++ {
		go func(k int) {
			<-start
			r := f.r
			if k >= 3 {
				r.Key = "other-" + string(rune('a'+k))
			}
			got, e := f.s.Admit(ctx, r, f.caller)
			results <- got
			errs <- e
		}(k)
	}
	close(start)
	var version uuid.UUID
	for k := 0; k < 6; k++ {
		r := <-results
		e := <-errs
		if e != nil {
			t.Fatal(e)
		}
		if r.VersionID == nil {
			t.Fatal("missing version")
		}
		if version == uuid.Nil {
			version = *r.VersionID
		}
		if version != *r.VersionID {
			t.Fatal("duplicate versions")
		}
	}
	if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID) != 1 {
		t.Fatal("concurrent allocation duplicated")
	}
	changed := f.r
	changed.ManifestSHA = strings.Repeat("0", 64)
	if _, e := f.s.Admit(context.Background(), changed, f.caller); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed-key replay=%v", e)
	}
}
func TestPostgresRevocationDuringSourceRead(t *testing.T) {
	f := newFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.beforeAsset = func() { once.Do(func() { close(entered); <-release }) }
	done := make(chan error, 1)
	go func() { _, e := f.s.Admit(context.Background(), f.r, f.caller); done <- e }()
	<-entered
	if _, e := f.pool.Exec(context.Background(), `UPDATE source_admission_connection SET enabled=false WHERE id=$1`, f.c.ID); e != nil {
		t.Fatal(e)
	}
	close(release)
	if e := <-done; !errors.Is(e, ErrDenied) {
		t.Fatalf("revoked capture=%v", e)
	}
	if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1 AND status='READY'`, f.r.DatasetID) != 0 {
		t.Fatal("revoked source published")
	}
}
func TestPostgresStorageFailureRecoveryAndNoRepair(t *testing.T) {
	f := newFixture(t)
	f.port.afterWrite = true
	if _, e := f.s.Admit(context.Background(), f.r, f.caller); e == nil {
		t.Fatal("copy interruption hidden")
	}
	if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1 AND status='READY'`, f.r.DatasetID) != 0 {
		t.Fatal("partial copy published")
	}
	first := f.admit(t)
	if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID) != 1 {
		t.Fatal("recovery allocated new version")
	}
	f.port.mu.Lock()
	for k := range f.port.objects {
		f.port.objects[k] = []byte("corrupt")
		break
	}
	writes := f.port.writes
	f.port.mu.Unlock()
	if _, e := f.s.Admit(context.Background(), f.r, f.caller); !errors.Is(e, ErrIntegrity) {
		t.Fatalf("corrupt copy replay=%v", e)
	}
	if writes != f.port.writes {
		t.Fatal("corrupt accepted copy repaired")
	}
	if first.VersionID == nil {
		t.Fatal("missing original receipt")
	}
}

func TestPostgresUnknownCopyReadKeepsOriginalOperationRecoverable(t *testing.T) {
	f := newFixture(t)
	original := f.admit(t)
	f.r.Key = "new-key-existing-snapshot"
	f.port.failReads = 1
	rec, e := f.s.Admit(context.Background(), f.r, f.caller)
	if !errors.Is(e, ErrUnknown) {
		t.Fatalf("uncertain storage read became terminal: %v", e)
	}
	lookup, e := f.s.LookupKey(context.Background(), f.r.WorkspaceID, f.r.Key, f.caller)
	if e != nil || lookup.ID != rec.ID || lookup.State != "PREPARED" {
		t.Fatalf("original request not recoverable: %+v %v", lookup, e)
	}
	recovered := f.admit(t)
	if recovered.ID != rec.ID || *recovered.VersionID != *original.VersionID {
		t.Fatal("recovery allocated another operation/version")
	}
}
func TestPostgresFrozenBindingsAndRollback(t *testing.T) {
	f := newFixture(t)
	rec := f.admit(t)
	for _, q := range []string{
		`UPDATE source_admission_capture SET content_digest=repeat('0',64) WHERE id=$1`,
		`DELETE FROM source_admission_asset WHERE capture_id=$1`,
		`UPDATE source_admission_member SET task_id='999' WHERE capture_id=$1`,
		`DELETE FROM source_admission_member WHERE capture_id=$1`,
		`INSERT INTO source_admission_asset(capture_id,name,sha256,byte_size) VALUES($1,'records.json',repeat('0',64),1)`,
	} {
		if _, e := f.pool.Exec(context.Background(), q, rec.CaptureID); e == nil {
			t.Fatalf("frozen fact mutated: %s", q)
		}
	}
	if _, e := f.pool.Exec(context.Background(), `UPDATE source_admission_connection SET namespace_id=$2 WHERE id=$1`, f.c.ID, uuid.New()); e == nil {
		t.Fatal("namespace rotated in place")
	}
	down, e := os.ReadFile("../../../../migrations/000072_fork_approved_source_admission.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	tx, e := f.pool.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(context.Background(), string(down))
	_ = tx.Rollback(context.Background())
	if e == nil {
		t.Fatal("retained history rolled back")
	}
}
func TestHTTPRejectsMutableActorAndUnknownLookup(t *testing.T) {
	f := newFixture(t)
	resolver, _ := principal.NewStaticResolver(true, "host-test-token", f.p.Subject, f.p.ActorID.String(), []string{f.c.WorkspaceID.String()}, []string{Capability})
	mux := http.NewServeMux()
	(&Handler{Service: f.s, Resolver: resolver}).Register(mux)
	body, _ := json.Marshal(f.r)
	body = append(body[:len(body)-1], []byte(`,"actorId":"forged"}`)...)
	req := httptest.NewRequest("POST", "/api/v1/workspaces/"+f.c.WorkspaceID.String()+"/source-admissions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer host-test-token")
	req.Header.Set("Idempotency-Key", f.r.Key)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 409 || f.sourceCalls.Load() != 0 {
		t.Fatalf("actor injection status=%d", w.Code)
	}
	if _, e := f.s.Lookup(context.Background(), f.c.WorkspaceID, uuid.New(), f.caller); !errors.Is(e, ErrDenied) {
		t.Fatal("unknown lookup created/claimed absence")
	}
	_ = io.EOF
	_ = pgx.ErrNoRows
}
