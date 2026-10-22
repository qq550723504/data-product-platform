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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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
