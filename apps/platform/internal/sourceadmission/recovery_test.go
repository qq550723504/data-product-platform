package sourceadmission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
)

func nativeEncode(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// A hostile source produces a self-consistent new envelope with the same
// Submission identity and changed immutable native content. The Host must reject
// it even though every independently recomputed byte hash is internally valid.
func changeSource(f *fixture) {
	lines := bytes.Split(bytes.TrimSuffix(f.blobs["records.jsonl"], []byte("\n")), []byte("\n"))
	var row map[string]json.RawMessage
	_ = json.Unmarshal(lines[0], &row)
	snapshot := rawObject(row, "result_snapshot")
	task := rawObject(snapshot, "task")
	data := rawObject(task, "data")
	data["text"] = json.RawMessage(`"changed source 中文"`)
	task["data"] = nativeEncode(data)
	snapshot["task"] = nativeEncode(task)
	row["result_snapshot"] = nativeEncode(snapshot)
	rh := Hash(row["result_snapshot"])
	row["result_hash"] = nativeEncode(rh)
	lines[0] = nativeEncode(row)
	f.blobs["records.jsonl"] = append(bytes.Join(lines, []byte("\n")), '\n')
	raw := make([]json.RawMessage, len(lines))
	for k, b := range lines {
		raw[k] = b
	}
	f.blobs["records.json"] = nativeEncode(raw)
	var manifest map[string]any
	_ = json.Unmarshal(f.blobs["manifest.json"], &manifest)
	content := manifest["content"].(map[string]any)
	item := content["items"].([]any)[0].(map[string]any)
	item["record_sha256"] = Hash(lines[0])
	item["result_hash"] = rh
	b, _ := jsoncanonicalizer.Transform(nativeEncode(content))
	manifest["content_digest"] = Hash(b)
	f.r.SnapshotID = uuid.New()
	manifest["snapshot_id"] = f.r.SnapshotID.String()
	f.r.ReleaseID = uuid.New()
	manifest["receipt"].(map[string]any)["release_id"] = f.r.ReleaseID.String()
	for _, a := range manifest["artifacts"].([]any) {
		asset := a.(map[string]any)
		blob := f.blobs[asset["name"].(string)]
		asset["sha256"] = Hash(blob)
		asset["size_bytes"] = len(blob)
	}
	b, _ = jsoncanonicalizer.Transform(nativeEncode(manifest))
	f.blobs["manifest.json"] = b
	f.r.ManifestSHA = Hash(b)
	f.r.Key = "hostile-new-snapshot"
	_ = json.Unmarshal(b, &f.manifest)
}
func TestPostgresSameSourceConflictRollsBackPublication(t *testing.T) {
	f := newFixture(t)
	original := f.admit(t)
	changeSource(f)
	if _, e := ValidateManifest(f.blobs["manifest.json"], f.c, f.r); e != nil {
		t.Fatal(e)
	}
	if e := ValidateBundle(f.manifest, f.blobs); e != nil {
		t.Fatal(e)
	}
	rec, e := f.s.Admit(context.Background(), f.r, f.caller)
	if !errors.Is(e, ErrConflict) {
		t.Fatalf("same-source conflict=%v", e)
	}
	if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1 AND status='READY'`, f.r.DatasetID) != 1 {
		t.Fatal("conflicting source published")
	}
	lookup, e := f.s.LookupKey(context.Background(), f.r.WorkspaceID, f.r.Key, f.caller)
	if e != nil || lookup.State != "FAILED_FINAL" || lookup.ID != rec.ID {
		t.Fatalf("terminal failure lost: %+v %v", lookup, e)
	}
	var current uuid.UUID
	if e = f.pool.QueryRow(context.Background(), `SELECT current_version_id FROM dataset WHERE id=$1`, f.r.DatasetID).Scan(&current); e != nil || current != *original.VersionID {
		t.Fatalf("current original rewritten: %v", e)
	}
}

func TestPostgresConnectionRotationCannotBypassSourceIdentity(t *testing.T) {
	f := newFixture(t)
	f.admit(t)
	// Another independently authorized workspace/connection may consume the
	// same source, but cannot rewrite its immutable identity in that namespace.
	g := newFixture(t)
	changeSource(g)
	if _, e := g.s.Admit(context.Background(), g.r, g.caller); !errors.Is(e, ErrConflict) {
		t.Fatalf("connection bypassed source identity: %v", e)
	}
	if g.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1 AND status='READY'`, g.r.DatasetID) != 0 {
		t.Fatal("changed source published through another connection")
	}
}

type publicationFault struct {
	VersionStore
	before func()
	fail   bool
}

func (p *publicationFault) FinalizePublication(ctx context.Context, tx pgx.Tx, v dd.DatasetVersion) error {
	if p.before != nil {
		p.before()
	}
	if p.fail {
		return errors.New("synthetic commit prerequisite failure")
	}
	return p.VersionStore.FinalizePublication(ctx, tx, v)
}
func TestPostgresAtomicFinalizationAndLateMembership(t *testing.T) {
	f := newFixture(t)
	base, _ := f.s.Store(f.c)
	fault := &publicationFault{VersionStore: base, fail: true}
	f.s.Store = func(Connection) (VersionStore, error) { return fault, nil }
	if _, e := f.s.Admit(context.Background(), f.r, f.caller); e == nil {
		t.Fatal("commit prerequisite failure hidden")
	}
	for _, table := range []string{"source_admission_member", "source_admission_identity"} {
		if f.count(t, `SELECT count(*) FROM `+table+` WHERE connection_id=$1`, f.c.ID) != 0 {
			t.Fatal("partial source facts committed")
		}
	}
	if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1 AND status='READY'`, f.r.DatasetID) != 0 {
		t.Fatal("partial version committed")
	}
	fault.fail = false
	entered, release := make(chan struct{}), make(chan struct{})
	fault.before = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() { _, e := f.s.Admit(context.Background(), f.r, f.caller); done <- e }()
	<-entered
	var capture uuid.UUID
	if e := f.pool.QueryRow(context.Background(), `SELECT id FROM source_admission_capture WHERE dataset_id=$1`, f.r.DatasetID).Scan(&capture); e != nil {
		t.Fatal(e)
	}
	late := make(chan error, 1)
	go func() {
		_, e := f.pool.Exec(context.Background(), `INSERT INTO source_admission_asset(capture_id,name,sha256,byte_size) VALUES($1,'records.json',repeat('0',64),1)`, capture)
		late <- e
	}()
	select {
	case e := <-late:
		t.Fatalf("late writer did not share finalization fence: %v", e)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := <-late; e == nil || !strings.Contains(e.Error(), "sealed source membership") {
		t.Fatalf("late writer crossed freeze: %v", e)
	}
}
func TestPostgresLostResponseLookupAndUnknown(t *testing.T) {
	f := newFixture(t)
	original := f.admit(t)
	got, e := f.s.LookupKey(context.Background(), f.r.WorkspaceID, f.r.Key, f.caller)
	if e != nil || got.ID != original.ID || *got.VersionID != *original.VersionID {
		t.Fatalf("response loss lookup=%v", e)
	}
	before := f.sourceCalls.Load()
	if _, e = f.s.LookupKey(context.Background(), f.r.WorkspaceID, "not-recorded", f.caller); !errors.Is(e, ErrUnknown) {
		t.Fatalf("unknown lookup=%v", e)
	}
	if before != f.sourceCalls.Load() {
		t.Fatal("lookup sent source traffic")
	}
}
func TestSourceRedirectDoesNotForwardCredential(t *testing.T) {
	f := newFixture(t)
	var received bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { received = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	c := f.c
	c.BaseURL = redirect.URL
	rec, e := f.s.prepare(context.Background(), f.r, f.caller)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Source.Fetch(context.Background(), f.s, rec.ID, f.r, c, "manifest.json", ""); e == nil || received {
		t.Fatal("redirect forwarded credential")
	}
}
