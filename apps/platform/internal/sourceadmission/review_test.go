package sourceadmission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
)

type orderedSnapshotStore struct {
	VersionStore
	entered chan struct{}
	release chan struct{}
}

func (s *orderedSnapshotStore) PutVersion(ctx context.Context, v dd.DatasetVersion, key string, r io.Reader, size int64, mediaType string) (string, error) {
	if v.VersionNo == 1 && strings.HasSuffix(key, "/manifest.json") {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return s.VersionStore.PutVersion(ctx, v, key, r, size, mediaType)
}

func TestPostgresOutOfOrderSnapshotPublicationTerminatesOriginalKey(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	base, _ := f.s.Store(f.c)
	store := &orderedSnapshotStore{VersionStore: base, entered: make(chan struct{}), release: make(chan struct{})}
	f.s.Store = func(Connection) (VersionStore, error) { return store, nil }
	oldRequest := f.r
	type result struct {
		receipt Receipt
		err     error
	}
	done := make(chan result, 1)
	go func() {
		rec, e := f.s.Admit(ctx, oldRequest, f.caller)
		done <- result{rec, e}
	}()
	select {
	case <-store.entered:
	case <-ctx.Done():
		t.Fatal("first snapshot never reserved its version")
	}
	// The first request has finished all source reads and is paused outside a
	// transaction. Publish a distinct, internally valid snapshot ahead of it.
	f.sourceMu.Lock()
	var envelope map[string]any
	_ = json.Unmarshal(f.blobs["manifest.json"], &envelope)
	f.r.SnapshotID, f.r.ReleaseID, f.r.Key = uuid.New(), uuid.New(), "newer-snapshot"
	envelope["snapshot_id"] = f.r.SnapshotID.String()
	envelope["receipt"].(map[string]any)["release_id"] = f.r.ReleaseID.String()
	f.blobs["manifest.json"] = nativeEncode(envelope)
	f.r.ManifestSHA = Hash(f.blobs["manifest.json"])
	f.sourceMu.Unlock()
	newer := f.admit(t)
	close(store.release)
	older := <-done
	const want = "SOURCE_PUBLICATION_CONFLICT"
	if older.err == nil || older.err.Error() != want || older.receipt.State != "FAILED_FINAL" {
		t.Fatalf("stale reservation remains retryable: %+v %v", older.receipt, older.err)
	}
	for _, byKey := range []bool{false, true} {
		var rec Receipt
		var e error
		if byKey {
			rec, e = f.s.LookupKey(ctx, oldRequest.WorkspaceID, oldRequest.Key, f.caller)
		} else {
			rec, e = f.s.Lookup(ctx, oldRequest.WorkspaceID, older.receipt.ID, f.caller)
		}
		if e != nil || rec.State != "FAILED_FINAL" || rec.ErrorCode != want || rec.VersionID != nil {
			t.Fatalf("lost terminal response not recoverable: %+v %v", rec, e)
		}
	}
	calls, writes := f.sourceCalls.Load(), f.port.writes
	replay, e := f.s.Admit(ctx, oldRequest, f.caller)
	if e == nil || e.Error() != want || replay.ID != older.receipt.ID || replay.State != "FAILED_FINAL" {
		t.Fatalf("original key changed outcome: %+v %v", replay, e)
	}
	if f.sourceCalls.Load() != calls || f.port.writes != writes {
		t.Fatal("terminal replay performed external work")
	}
	if n := f.count(t, `SELECT count(*) FROM dataset WHERE id=$1 AND current_version_id=$2`, f.r.DatasetID, newer.VersionID); n != 1 {
		t.Fatal("late lower version rewound current version")
	}
	if n := f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID); n != 2 {
		t.Fatalf("terminal replay allocated another version: %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_capture WHERE dataset_id=$1 AND state='ACCEPTED_SOURCE'`, f.r.DatasetID); n != 1 {
		t.Fatal("late snapshot published partial acceptance")
	}
	if n := f.count(t, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='SourceAdmissionRejected'`, older.receipt.ID); n != 1 {
		t.Fatalf("terminal rejection facts=%d want=1", n)
	}
	w := httptest.NewRecorder()
	respond(w, replay, e)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), want) {
		t.Fatalf("terminal HTTP contract: %d %s", w.Code, w.Body.String())
	}
}

func TestPostgresReplacementConnectionBindings(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for name, change := range map[string]func(*Connection){
		"dataset":       func(c *Connection) { c.DatasetID = uuid.New() },
		"authorization": func(c *Connection) { c.AuthorizationID = uuid.New() },
		"consumer":      func(c *Connection) { c.Consumer = "replacement-consumer" },
		"endpoint":      func(c *Connection) { c.BaseURL = "https://replacement.invalid" },
		"token-file":    func(c *Connection) { c.TokenFile = filepath.Join(t.TempDir(), "replacement-token") },
	} {
		t.Run(name, func(t *testing.T) {
			c := f.c
			change(&c)
			if e := f.s.RegisterConnection(ctx, c); !errors.Is(e, ErrConflict) {
				t.Fatalf("existing connection rebound: %v", e)
			}
			c.ID = uuid.New()
			if e := f.s.RegisterConnection(ctx, c); e != nil {
				t.Fatalf("explicit replacement rejected: %v", e)
			}
			if e := f.s.RegisterConnection(ctx, c); e != nil {
				t.Fatalf("replacement registration not idempotent: %v", e)
			}
		})
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_connection WHERE workspace_id=$1 AND namespace_id=$2 AND project_id=$3`, f.c.WorkspaceID, f.c.Namespace, f.c.Project); n != 6 {
		t.Fatalf("replacement connections=%d want=6", n)
	}
}

func TestNativeTextURLSchemeBoundary(t *testing.T) {
	for _, tc := range []struct {
		value string
		deny  bool
	}{
		{"https://example.invalid/file", true},
		{"HTTPS://example.invalid/file", true},
		{"hTtPs://example.invalid/file", true},
		{"HTTP://example.invalid/file", true},
		{" \tHtTp://example.invalid/file\n", true},
		{"/data/upload/source.txt", true},
		{"Literal text mentions HTTPS://example.invalid/file 中文", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			f := &fixture{blobs: map[string][]byte{}}
			for _, name := range []string{"manifest.json", "records.json", "records.jsonl"} {
				b, e := os.ReadFile(filepath.Join("testdata", name))
				if e != nil {
					t.Fatal(e)
				}
				f.blobs[name] = b
			}
			_ = json.Unmarshal(f.blobs["manifest.json"], &f.manifest)
			changeSourceText(f, tc.value)
			c := Connection{Namespace: uuid.MustParse(f.manifest.Content.Namespace), Project: f.manifest.Content.Project, Organization: f.manifest.Content.Organization, Mode: "fork_approved"}
			m, e := ValidateManifest(f.blobs["manifest.json"], c, f.r)
			if e != nil {
				t.Fatal(e)
			}
			e = ValidateBundle(m, f.blobs)
			if (tc.deny && !errors.Is(e, ErrUnsupported)) || (!tc.deny && e != nil) {
				t.Fatalf("text boundary deny=%t: %v", tc.deny, e)
			}
		})
	}
}

func TestPostgresDatasetPublicationFactsKeepAggregateContract(t *testing.T) {
	f := newFixture(t)
	first := f.admit(t)
	var envelope map[string]any
	_ = json.Unmarshal(f.blobs["manifest.json"], &envelope)
	f.r.SnapshotID, f.r.ReleaseID, f.r.Key = uuid.New(), uuid.New(), "next-snapshot"
	envelope["snapshot_id"] = f.r.SnapshotID.String()
	envelope["receipt"].(map[string]any)["release_id"] = f.r.ReleaseID.String()
	f.blobs["manifest.json"] = nativeEncode(envelope)
	f.r.ManifestSHA = Hash(f.blobs["manifest.json"])
	second := f.admit(t)
	f.admit(t) // Replay must not duplicate publication facts.
	for _, rec := range []Receipt{first, second} {
		if n := f.count(t, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND aggregate_type='DATASET_VERSION' AND event_type='DatasetVersionCreated' AND payload->>'datasetId'=$2 AND payload->>'status'='READY' AND payload->>'previousStatus'='CREATED' AND payload->>'checksum'=$3 AND (payload->>'versionNo')::int>0`, rec.VersionID, f.r.DatasetID.String(), Hash(f.blobs["records.jsonl"])); n != 1 {
			t.Fatalf("dataset publication event not discoverable: %d", n)
		}
		if n := f.count(t, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND object_type='DATASET_VERSION' AND action='DATASET_VERSION_READY' AND after_state->>'datasetId'=$2`, rec.VersionID, f.r.DatasetID.String()); n != 1 {
			t.Fatalf("dataset publication audit not discoverable: %d", n)
		}
		if n := f.count(t, `SELECT count(*) FROM evidence WHERE source_id=$1 AND source_type='DATASET_VERSION' AND evidence_type='DATASET_VERSION' AND title='DatasetVersionCreated'`, rec.VersionID); n != 1 {
			t.Fatalf("dataset publication evidence not discoverable: %d", n)
		}
		if n := f.count(t, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND aggregate_type<>'DATASET_VERSION'`, rec.VersionID); n != 0 {
			t.Fatal("dataset facts mislabeled as source admission")
		}
		if n := f.count(t, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND aggregate_type='SOURCE_ADMISSION' AND event_type='SourceAdmissionAccepted'`, rec.CaptureID); n != 1 {
			t.Fatal("source acceptance fact lost its own aggregate")
		}
	}
	if n := f.count(t, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND aggregate_type='DATASET_VERSION' AND event_type='DatasetVersionSuperseded' AND payload->>'replacementVersionId'=$2 AND payload->>'previousStatus'='READY' AND payload->>'status'='SUPERSEDED'`, first.VersionID, second.VersionID.String()); n != 1 {
		t.Fatalf("dataset supersession event not discoverable: %d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND object_type='DATASET_VERSION' AND action='DATASET_VERSION_SUPERSEDED' AND before_state->>'status'='READY' AND after_state->>'status'='SUPERSEDED'`, first.VersionID); n != 1 {
		t.Fatalf("dataset supersession audit not discoverable: %d", n)
	}
}

func TestPostgresReplacementPreservesFrozenSourceAndRevocation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	original := f.admit(t)
	old, oldRequest := f.c, f.r
	if _, e := f.pool.Exec(ctx, `UPDATE source_admission_connection SET enabled=false WHERE id=$1`, old.ID); e != nil {
		t.Fatal(e)
	}
	if e := f.s.RegisterConnection(ctx, old); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.Admit(ctx, oldRequest, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatalf("registration re-enabled old connection: %v", e)
	}
	f.c.ID = uuid.New()
	f.c.TokenFile = filepath.Join(t.TempDir(), "replacement-fork-token")
	if e := os.WriteFile(f.c.TokenFile, []byte("synthetic-fork-token"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := f.s.RegisterConnection(ctx, f.c); e != nil {
		t.Fatal(e)
	}
	f.r.ConnectionID = f.c.ID
	if _, e := f.s.Admit(ctx, f.r, f.caller); !errors.Is(e, ErrConflict) {
		t.Fatalf("replacement rewrote original operation: %v", e)
	}
	f.r.Key = "replacement-original-source"
	f.admit(t)
	changeSource(f)
	if _, e := f.s.Admit(ctx, f.r, f.caller); !errors.Is(e, ErrConflict) {
		t.Fatalf("replacement rewrote source identity: %v", e)
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_capture WHERE id=$1 AND connection_id=$2 AND version_id=$3 AND state='ACCEPTED_SOURCE'`, original.CaptureID, old.ID, original.VersionID); n != 1 {
		t.Fatal("replacement rewrote original frozen capture")
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_identity WHERE namespace_id=$1`, old.Namespace); n != 3 {
		t.Fatalf("replacement duplicated source identities: %d", n)
	}
}

func TestPostgresSourceTransportFailureRecoversOriginalKey(t *testing.T) {
	for _, stage := range []string{"manifest", "asset", "rate-limit"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			first := true
			f.sourceResponse = func(w http.ResponseWriter, _ *http.Request, name string) bool {
				if !first || (stage == "asset" && name == "manifest.json") {
					return false
				}
				first = false
				if stage == "rate-limit" {
					w.WriteHeader(429)
					return true
				}
				w.Header().Set("Content-Length", fmt.Sprint(len(f.blobs[name])))
				_, _ = w.Write(f.blobs[name][:16]) // A genuinely interrupted HTTP response.
				return true
			}
			rec, e := f.s.Admit(context.Background(), f.r, f.caller)
			if !errors.Is(e, ErrUnknown) {
				t.Fatalf("transport failure became terminal: %v", e)
			}
			saved, e := f.s.LookupKey(context.Background(), f.r.WorkspaceID, f.r.Key, f.caller)
			if e != nil || saved.ID != rec.ID || saved.State != "PREPARED" {
				t.Fatalf("lost original operation: %+v %v", saved, e)
			}
			if f.count(t, `SELECT count(*) FROM source_admission_observation o JOIN source_admission_call c ON c.id=o.call_id WHERE c.operation_id=$1 AND o.outcome='UNKNOWN'`, rec.ID) != 1 {
				t.Fatal("uncertainty was not retained")
			}
			got := f.admit(t)
			if got.ID != rec.ID || f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID) != 1 {
				t.Fatal("retry replaced original operation/version")
			}
		})
	}
}

func TestPostgresSourceDoesNotHideAutomaticTransportRetry(t *testing.T) {
	f := newFixture(t)
	first := true
	f.sourceResponse = func(w http.ResponseWriter, _ *http.Request, name string) bool {
		if !first || name == "manifest.json" {
			return false
		}
		first = false
		conn, _, e := w.(http.Hijacker).Hijack()
		if e != nil {
			t.Error(e)
			return true
		}
		_ = conn.Close() // Close a reused connection after receiving an actual GET.
		return true
	}
	rec, e := f.s.Admit(context.Background(), f.r, f.caller)
	if !errors.Is(e, ErrUnknown) {
		t.Fatalf("client silently retried a physical GET: %v", e)
	}
	got := f.admit(t)
	if got.ID != rec.ID {
		t.Fatal("manual resume lost original identity")
	}
	if int64(f.count(t, `SELECT count(*) FROM source_admission_call WHERE operation_id=$1`, rec.ID)) != f.sourceCalls.Load() {
		t.Fatal("physical source attempts escaped typed invocation accounting")
	}
}

func TestTextProfileRejectsRemoteValueType(t *testing.T) {
	if _, e := textFields(`<View><Text name="text" value="$text" valueType="url"/></View>`); !errors.Is(e, ErrUnsupported) {
		t.Fatalf("remote Text accepted by inline profile: %v", e)
	}
}

func TestPostgresFrozenClosureRejectsMissingFacts(t *testing.T) {
	f := newFixture(t)
	base, _ := f.s.Store(f.c)
	f.s.Store = func(Connection) (VersionStore, error) { return &publicationFault{VersionStore: base, fail: true}, nil }
	if _, e := f.s.Admit(context.Background(), f.r, f.caller); e == nil {
		t.Fatal("expected stopped publication")
	}
	for _, field := range []string{"row_count", "content_type", "checksum_algorithm", "storage_type"} {
		t.Run(field, func(t *testing.T) {
			tx, e := f.pool.Begin(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback(context.Background())
			var capture, version uuid.UUID
			if e = tx.QueryRow(context.Background(), `SELECT id,version_id FROM source_admission_capture WHERE dataset_id=$1`, f.r.DatasetID).Scan(&capture, &version); e != nil {
				t.Fatal(e)
			}
			for _, i := range f.manifest.Content.Items {
				if _, e = tx.Exec(context.Background(), `INSERT INTO source_admission_identity(connection_id,namespace_id,source_id,record_sha256) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, f.c.ID, f.c.Namespace, i.Source.ID, i.RecordHash); e != nil {
					t.Fatal(e)
				}
				if _, e = tx.Exec(context.Background(), `INSERT INTO source_admission_member(capture_id,connection_id,namespace_id,source_id,assignment_id,task_id,revision,result_sha256,record_sha256) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, capture, f.c.ID, f.c.Namespace, i.Source.ID, i.Assignment, i.Task, i.Revision, i.ResultHash, i.RecordHash); e != nil {
					t.Fatal(e)
				}
			}
			query := `UPDATE dataset_version SET status='READY',storage_type='OBJECT_STORAGE',storage_uri=(SELECT storage_uri FROM source_admission_asset WHERE capture_id=$1 AND name='records.jsonl'),checksum_algorithm='SHA256',checksum_value=$2,byte_size=$3,row_count=3,content_type='application/x-ndjson',` + pgx.Identifier{field}.Sanitize() + `=NULL WHERE id=$4`
			// Assign each nullable field once, so the NULL assignment is authoritative.
			replacements := map[string]string{"row_count": ",row_count=3", "content_type": ",content_type='application/x-ndjson'", "checksum_algorithm": ",checksum_algorithm='SHA256'", "storage_type": ",storage_type='OBJECT_STORAGE'"}
			query = strings.Replace(query, replacements[field], "", 1)
			if _, e = tx.Exec(context.Background(), query, capture, Hash(f.blobs["records.jsonl"]), len(f.blobs["records.jsonl"]), version); e != nil {
				t.Fatal(e)
			}
			if _, e = tx.Exec(context.Background(), `UPDATE source_admission_capture SET state='ACCEPTED_SOURCE' WHERE id=$1`, capture); e == nil {
				t.Fatal("incomplete version metadata was frozen as accepted")
			}
		})
	}
	if _, e := f.pool.Exec(context.Background(), `UPDATE source_admission_operation SET state='FAILED_FINAL',error_code=NULL WHERE workspace_id=$1`, f.r.WorkspaceID); e == nil {
		t.Fatal("terminal rejection without reason was frozen")
	}
}

type uncertainStream struct{ io.ReadCloser }

func (s uncertainStream) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type uncertainReadStore struct {
	VersionStore
	fail bool
}

func (s *uncertainReadStore) Get(ctx context.Context, uri string) (io.ReadCloser, error) {
	r, e := s.VersionStore.Get(ctx, uri)
	if e != nil {
		return nil, e
	}
	if s.fail {
		s.fail = false
		return uncertainStream{r}, nil
	}
	return r, nil
}
func TestPostgresCopyReadInterruptionKeepsReservedVersion(t *testing.T) {
	f := newFixture(t)
	base, _ := f.s.Store(f.c)
	store := &uncertainReadStore{VersionStore: base, fail: true}
	f.s.Store = func(Connection) (VersionStore, error) { return store, nil }
	rec, e := f.s.Admit(context.Background(), f.r, f.caller)
	if !errors.Is(e, ErrUnknown) {
		t.Fatalf("incomplete copy read became terminal: %v", e)
	}
	saved, e := f.s.LookupKey(context.Background(), f.r.WorkspaceID, f.r.Key, f.caller)
	if e != nil || saved.State != "PREPARED" {
		t.Fatalf("original copy not recoverable: %+v %v", saved, e)
	}
	got := f.admit(t)
	if got.ID != rec.ID || f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID) != 1 {
		t.Fatal("copy recovery abandoned original reservation")
	}
}

func TestPostgresHTTPRechecksProfileAndCredentialRevocation(t *testing.T) {
	f := newFixture(t)
	rec := f.admit(t)
	dir := t.TempDir()
	p := Profile{Enabled: true, Connection: f.c, ActorID: f.p.ActorID, Subject: f.p.Subject, APITokenFile: filepath.Join(dir, "api-token")}
	path := filepath.Join(dir, "profile.json")
	write := func() {
		t.Helper()
		b, _ := json.Marshal(p)
		if e := os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if e := os.WriteFile(p.APITokenFile, []byte("initial-test-token"), 0600); e != nil {
		t.Fatal(e)
	}
	write()
	resolver := profileResolver{path, p.binding()}
	mux := http.NewServeMux()
	(&Handler{Service: f.s, Resolver: resolver}).Register(mux)
	lookup := func(token string, expected int) {
		t.Helper()
		for _, suffix := range []string{"/" + rec.ID.String(), "?key=" + f.r.Key} {
			request := httptest.NewRequest("GET", "/api/v1/workspaces/"+f.r.WorkspaceID.String()+"/source-admissions"+suffix, nil)
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("X-Authenticated-Principal", f.p.Subject)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != expected {
				t.Fatalf("stale credential/profile lookup: %s status=%d want=%d", suffix, response.Code, expected)
			}
		}
	}
	lookup("initial-test-token", 200)
	if e := os.WriteFile(p.APITokenFile, []byte("rotated-test-token"), 0600); e != nil {
		t.Fatal(e)
	}
	lookup("initial-test-token", 403)
	lookup("rotated-test-token", 200)
	p.Enabled = false
	write()
	lookup("rotated-test-token", 403)
	p.Enabled = true
	p.ActorID = uuid.New()
	write()
	lookup("rotated-test-token", 403)
}
