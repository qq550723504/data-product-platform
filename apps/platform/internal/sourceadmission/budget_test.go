package sourceadmission

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	ra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	ri "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

// Change only declarations, preserving schema/JCS/content digest/scope. These
// envelopes test preflight, not acceptance of the unserved oversized bodies.
func declareBundleTotal(t *testing.T, f *fixture, total int64) {
	t.Helper()
	var envelope map[string]any
	if e := json.Unmarshal(f.blobs["manifest.json"], &envelope); e != nil {
		t.Fatal(e)
	}
	assets := envelope["artifacts"].([]any)
	first := int64(MaxBytes / 2)
	second := total - first - int64(len(f.blobs["manifest.json"]))
	for k := 0; k < 5; k++ {
		assets[0].(map[string]any)["size_bytes"] = first
		assets[1].(map[string]any)["size_bytes"] = second
		b, e := jsoncanonicalizer.Transform(nativeEncode(envelope))
		if e != nil {
			t.Fatal(e)
		}
		f.blobs["manifest.json"] = b
		f.r.ManifestSHA = Hash(b)
		f.manifest, e = ValidateManifest(b, f.c, f.r)
		if e != nil {
			t.Fatalf("invalid budget fixture: %v", e)
		}
		if first+second+int64(len(b)) == total {
			return
		}
		second = total - first - int64(len(b))
	}
	t.Fatal("budget fixture length did not converge")
}

func TestDeclaredBundleBudgetBoundaries(t *testing.T) {
	b, e := os.ReadFile("testdata/manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = json.Unmarshal(b, &m); e != nil {
		t.Fatal(e)
	}
	f := &fixture{blobs: map[string][]byte{"manifest.json": b}, manifest: m}
	f.c = Connection{Namespace: uuid.MustParse(m.Content.Namespace), Project: m.Content.Project, Organization: m.Content.Organization, Mode: "fork_approved"}
	f.r = Request{SnapshotID: uuid.MustParse(m.Snapshot), ReleaseID: uuid.MustParse(m.Receipt.ID)}
	for _, delta := range []int64{-1, 0, 1} {
		declareBundleTotal(t, f, MaxBytes+delta)
		e = validateDeclaredBundleBudget(f.blobs["manifest.json"], f.manifest)
		if (delta > 0 && !errors.Is(e, ErrUnsupported)) || (delta <= 0 && e != nil) {
			t.Fatalf("delta=%d: %v", delta, e)
		}
	}
	// Comparison before subtraction avoids overflow even for hostile int64 input.
	for _, sizes := range [][]int64{{math.MaxInt64, math.MaxInt64}, {MaxBytes, 1}} {
		m.Artifacts[0].Size, m.Artifacts[1].Size = sizes[0], sizes[1]
		if !errors.Is(validateDeclaredBundleBudget(b, m), ErrUnsupported) {
			t.Fatal("overflow/omitted envelope budget")
		}
	}
}

func assertNoBudgetPublication(t *testing.T, f *fixture) {
	t.Helper()
	for _, q := range []string{
		`SELECT count(*) FROM dataset_version WHERE dataset_id=$1`,
		`SELECT count(*) FROM source_admission_capture WHERE dataset_id=$1`,
	} {
		if n := f.count(t, q, f.r.DatasetID); n != 0 {
			t.Fatalf("unexpected allocation: %d", n)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM lifecycle_protected_object_intent WHERE workspace_id=$1`, f.r.WorkspaceID); n != 0 {
		t.Fatalf("copy intents=%d", n)
	}
	if n := f.count(t, `SELECT count(*) FROM lifecycle_fixture_copy WHERE workspace_id=$1`, f.r.WorkspaceID); n != 0 {
		t.Fatalf("copy allocations=%d", n)
	}
	// newFixture has already recorded its independent rights verification.
	if n := f.count(t, `SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type NOT IN('SOURCE_ADMISSION_FETCH','RIGHTS_DECLARATION_VERIFICATION')`, f.r.WorkspaceID); n != 0 {
		t.Fatalf("unexpected non-source costs=%d", n)
	}
	if f.port.writes != 0 {
		t.Fatal("rejection wrote storage")
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_identity WHERE connection_id=$1`, f.r.ConnectionID); n != 0 {
		t.Fatal("rejection allocated source identity")
	}
}

func assertBudgetCalls(t *testing.T, f *fixture, calls int, outcome string, status int) {
	t.Helper()
	if got := f.sourceCalls.Load(); got != int64(calls) {
		t.Fatalf("physical calls=%d want=%d", got, calls)
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_call WHERE workspace_id=$1`, f.r.WorkspaceID); n != calls {
		t.Fatalf("durable calls=%d want=%d", n, calls)
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_call c JOIN cost_allocation a ON a.source_admission_call_id=c.id JOIN cost_event e ON e.id=a.cost_event_id JOIN source_admission_observation o ON o.call_id=c.id WHERE c.workspace_id=$1 AND c.asset_name='manifest.json' AND e.workspace_id=c.workspace_id AND e.activity_id=c.id AND e.cost_type='SOURCE_ADMISSION_FETCH' AND e.quantity=1 AND e.unit='invocation' AND e.pricing_mode='ACTUAL' AND o.outcome=$2 AND o.status_code=$3`, f.r.WorkspaceID, outcome, status); n != calls {
		t.Fatalf("exact invocation/cost/observation closure=%d want=%d", n, calls)
	}
	if n := f.count(t, `SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type='SOURCE_ADMISSION_FETCH'`, f.r.WorkspaceID); n != calls {
		t.Fatalf("source costs=%d want=%d", n, calls)
	}
}

func assertBudgetRejectionFacts(t *testing.T, f *fixture, id uuid.UUID, count int) {
	t.Helper()
	for _, q := range []string{
		`SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='SourceAdmissionRejected'`,
		`SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='SourceAdmissionRejected'`,
		`SELECT count(*) FROM evidence WHERE source_id=$1 AND title='SourceAdmissionRejected'`,
	} {
		if n := f.count(t, q, id); n != count {
			t.Fatalf("rejection facts=%d want=%d", n, count)
		}
	}
}

func TestPostgresBudgetRejectsBeforeArtifactsAndRecoversOriginalKey(t *testing.T) {
	f := newFixture(t)
	declareBundleTotal(t, f, MaxBytes+1)
	ctx := context.Background()
	rec, e := f.s.Admit(ctx, f.r, f.caller)
	if !errors.Is(e, ErrUnsupported) || rec.State != "FAILED_FINAL" {
		t.Fatalf("overbudget: %+v %v", rec, e)
	}
	w := httptest.NewRecorder()
	respond(w, rec, e)
	if w.Code != 422 || !strings.Contains(w.Body.String(), ErrUnsupported.Error()) {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	// Ignore the first response: lookup by both identities recovers frozen truth.
	for _, byKey := range []bool{true, false} {
		var got Receipt
		if byKey {
			got, e = f.s.LookupKey(ctx, f.r.WorkspaceID, f.r.Key, f.caller)
		} else {
			got, e = f.s.Lookup(ctx, f.r.WorkspaceID, rec.ID, f.caller)
		}
		if e != nil || got.ID != rec.ID || got.State != "FAILED_FINAL" || got.ErrorCode != ErrUnsupported.Error() || got.VersionID != nil {
			t.Fatalf("lost-response recovery: %+v %v", got, e)
		}
	}
	got, e := f.s.Admit(ctx, f.r, f.caller)
	if !errors.Is(e, ErrUnsupported) || got.ID != rec.ID {
		t.Fatalf("terminal replay: %+v %v", got, e)
	}
	changed := f.r
	changed.ManifestSHA = strings.Repeat("0", 64)
	if _, e = f.s.Admit(ctx, changed, f.caller); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed-key inputs: %v", e)
	}
	assertBudgetCalls(t, f, 1, "RECEIVED", 200)
	assertNoBudgetPublication(t, f)
	assertBudgetRejectionFacts(t, f, rec.ID, 1)
	// A historical rejection grants no current authority, via either lookup.
	f.allowed.Store(false)
	if _, e = f.s.Admit(ctx, f.r, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatalf("revoked replay: %v", e)
	}
	if _, e = f.s.LookupKey(ctx, f.r.WorkspaceID, f.r.Key, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatalf("revoked key lookup: %v", e)
	}
	if _, e = f.s.Lookup(ctx, f.r.WorkspaceID, rec.ID, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatalf("revoked object lookup: %v", e)
	}
	f.allowed.Store(true)
	other := func(ctx context.Context, ws uuid.UUID) (principal.RequestPrincipal, error) {
		p, e := f.caller(ctx, ws)
		p.ActorID = uuid.New()
		return p, e
	}
	if _, e = f.s.Lookup(ctx, f.r.WorkspaceID, rec.ID, other); !errors.Is(e, ErrDenied) {
		t.Fatalf("foreign actor lookup: %v", e)
	}
	assertBudgetCalls(t, f, 1, "RECEIVED", 200)
	assertBudgetRejectionFacts(t, f, rec.ID, 1)
}

type budgetRoundTripper func(*http.Request) (*http.Response, error)

func (f budgetRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPostgresBudgetUnauthorizedBeforeFetch(t *testing.T) {
	for _, kind := range []string{"workspace", "dataset", "connection", "capability", "disabled", "rights", "principal"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			declareBundleTotal(t, f, MaxBytes+1)
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
			case "principal":
				f.allowed.Store(false)
			case "disabled":
				if _, e := f.pool.Exec(context.Background(), `UPDATE source_admission_connection SET enabled=false WHERE id=$1`, f.c.ID); e != nil {
					t.Fatal(e)
				}
			case "rights":
				if _, e := ra.NewService(f.s.Tx, ri.NewPostgresRepository(f.pool)).DisposeRightsDeclaration(context.Background(), ra.DisposeRightsDeclarationCommand{DeclarationID: f.declaration, Disposition: rd.DispositionInvalidated, EffectiveAt: time.Now().UTC(), Reason: "budget permission negative"}); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := f.s.Admit(context.Background(), r, f.caller); !errors.Is(e, ErrDenied) {
				t.Fatalf("unauthorized budget request: %v", e)
			}
			assertBudgetCalls(t, f, 0, "RECEIVED", 200)
			if f.count(t, `SELECT count(*) FROM source_admission_operation WHERE workspace_id=$1`, f.r.WorkspaceID) != 0 {
				t.Fatal("denied request prepared operation")
			}
			if f.count(t, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, f.r.DatasetID) != 0 || f.port.writes != 0 {
				t.Fatal("unauthorized publication")
			}
		})
	}
}

func TestPostgresConcurrentBudgetRejectionCountsEveryPhysicalCall(t *testing.T) {
	f := newFixture(t)
	declareBundleTotal(t, f, MaxBytes+1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	transport := f.s.Source.Client.Transport
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	f.s.Source.Client.Transport = budgetRoundTripper(func(r *http.Request) (*http.Response, error) {
		res, e := transport.RoundTrip(r)
		arrived <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		return res, e
	})
	type result struct {
		rec Receipt
		err error
	}
	done := make(chan result, 2)
	for k := 0; k < 2; k++ {
		go func() { rec, e := f.s.Admit(ctx, f.r, f.caller); done <- result{rec, e} }()
	}
	for k := 0; k < 2; k++ {
		select {
		case <-arrived:
		case <-ctx.Done():
			close(release)
			t.Fatal("concurrent Manifest calls did not arrive")
		}
	}
	close(release)
	var id uuid.UUID
	for k := 0; k < 2; k++ {
		got := <-done
		if !errors.Is(got.err, ErrUnsupported) {
			t.Fatalf("concurrent rejection: %v", got.err)
		}
		if id == uuid.Nil {
			id = got.rec.ID
		}
		if got.rec.ID != id {
			t.Fatal("same key created two operations")
		}
	}
	assertBudgetCalls(t, f, 2, "RECEIVED", 200)
	assertBudgetRejectionFacts(t, f, id, 1)
	assertNoBudgetPublication(t, f)
	if _, e := f.s.Admit(ctx, f.r, f.caller); !errors.Is(e, ErrUnsupported) {
		t.Fatal(e)
	}
	assertBudgetCalls(t, f, 2, "RECEIVED", 200)
}

func TestPostgresBudgetRevocationBeforeTerminalCommit(t *testing.T) {
	f := newFixture(t)
	declareBundleTotal(t, f, MaxBytes+1)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.sourceResponse = func(w http.ResponseWriter, r *http.Request, name string) bool {
		if name == "manifest.json" {
			once.Do(func() { close(entered); <-release })
		}
		return false
	}
	done := make(chan error, 1)
	go func() { _, e := f.s.Admit(context.Background(), f.r, f.caller); done <- e }()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("Manifest not requested")
	}
	if _, e := f.pool.Exec(context.Background(), `UPDATE source_admission_connection SET enabled=false WHERE id=$1`, f.c.ID); e != nil {
		close(release)
		t.Fatal(e)
	}
	close(release)
	if e := <-done; !errors.Is(e, ErrDenied) {
		t.Fatalf("terminal gate bypassed: %v", e)
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_operation WHERE workspace_id=$1 AND state='PREPARED'`, f.r.WorkspaceID); n != 1 {
		t.Fatal("revocation rewrote terminal history")
	}
	var id uuid.UUID
	if e := f.pool.QueryRow(context.Background(), `SELECT id FROM source_admission_operation WHERE workspace_id=$1`, f.r.WorkspaceID).Scan(&id); e != nil {
		t.Fatal(e)
	}
	assertBudgetRejectionFacts(t, f, id, 0)
	assertBudgetCalls(t, f, 1, "RECEIVED", 200)
	assertNoBudgetPublication(t, f)
	// While disabled, even the original key cannot allocate another call.
	if _, e := f.s.Admit(context.Background(), f.r, f.caller); !errors.Is(e, ErrDenied) {
		t.Fatal(e)
	}
	assertBudgetCalls(t, f, 1, "RECEIVED", 200)
	if _, e := f.pool.Exec(context.Background(), `UPDATE source_admission_connection SET enabled=true WHERE id=$1`, f.c.ID); e != nil {
		t.Fatal(e)
	}
	rec, e := f.s.Admit(context.Background(), f.r, f.caller)
	if !errors.Is(e, ErrUnsupported) || rec.ID != id {
		t.Fatalf("original-key resume: %+v %v", rec, e)
	}
	assertBudgetCalls(t, f, 2, "RECEIVED", 200)
	assertBudgetRejectionFacts(t, f, id, 1)
}

func TestPostgresBudgetDoesNotReplaceManifestOrActualIntegrity(t *testing.T) {
	for _, kind := range []string{"hash", "scope", "core-only", "invalid-size", "invalid-content-digest", "noncanonical", "exact-budget", "underdeclared"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			declareBundleTotal(t, f, MaxBytes+1)
			var env map[string]any
			_ = json.Unmarshal(f.blobs["manifest.json"], &env)
			want, calls := ErrIntegrity, 1
			switch kind {
			case "hash":
				f.r.ManifestSHA = strings.Repeat("0", 64)
			case "scope":
				f.r.SnapshotID = uuid.New()
				env["snapshot_id"] = uuid.New().String()
			case "core-only":
				env["origin_mode"] = "core_only_review"
			case "invalid-size":
				env["artifacts"].([]any)[0].(map[string]any)["size_bytes"] = 0
			case "invalid-content-digest":
				env["content_digest"] = strings.Repeat("0", 64)
			case "noncanonical":
			case "exact-budget":
				declareBundleTotal(t, f, MaxBytes)
				env = nil
				calls = 3
			case "underdeclared":
				for _, a := range env["artifacts"].([]any) {
					a.(map[string]any)["size_bytes"] = 1
				}
				calls = 3
			}
			if env != nil && kind != "hash" {
				b, e := jsoncanonicalizer.Transform(nativeEncode(env))
				if e != nil {
					t.Fatal(e)
				}
				if kind == "noncanonical" {
					b = append(b, ' ')
				}
				f.blobs["manifest.json"], f.r.ManifestSHA = b, Hash(b)
			}
			_, e := f.s.Admit(context.Background(), f.r, f.caller)
			if !errors.Is(e, want) {
				t.Fatalf("precedence/integrity: %v want %v", e, want)
			}
			if f.sourceCalls.Load() != int64(calls) {
				t.Fatalf("calls=%d want=%d", f.sourceCalls.Load(), calls)
			}
			assertNoBudgetPublication(t, f)
		})
	}
}

func TestPostgresBudgetTransportUncertaintyRetainsOriginalOperation(t *testing.T) {
	f := newFixture(t)
	declareBundleTotal(t, f, MaxBytes+1)
	f.sourceResponse = func(w http.ResponseWriter, r *http.Request, name string) bool { w.WriteHeader(429); return true }
	rec, e := f.s.Admit(context.Background(), f.r, f.caller)
	if !errors.Is(e, ErrUnknown) {
		t.Fatal(e)
	}
	saved, e := f.s.LookupKey(context.Background(), f.r.WorkspaceID, f.r.Key, f.caller)
	if e != nil || saved.ID != rec.ID || saved.State != "PREPARED" {
		t.Fatalf("uncertainty lost: %+v %v", saved, e)
	}
	assertBudgetCalls(t, f, 1, "UNKNOWN", 429)
	assertBudgetRejectionFacts(t, f, rec.ID, 0)
	f.sourceMu.Lock()
	f.sourceResponse = nil
	f.sourceMu.Unlock()
	resumed, e := f.s.Admit(context.Background(), f.r, f.caller)
	if !errors.Is(e, ErrUnsupported) || resumed.ID != rec.ID {
		t.Fatalf("unknown resume: %+v %v", resumed, e)
	}
	if f.sourceCalls.Load() != 2 {
		t.Fatal("resumed work not counted")
	}
	if n := f.count(t, `SELECT count(*) FROM source_admission_call c JOIN cost_allocation a ON a.source_admission_call_id=c.id JOIN cost_event e ON e.id=a.cost_event_id JOIN source_admission_observation o ON o.call_id=c.id WHERE c.workspace_id=$1 AND c.asset_name='manifest.json' AND e.activity_id=c.id AND e.quantity=1 AND e.unit='invocation' AND e.cost_type='SOURCE_ADMISSION_FETCH' AND e.pricing_mode='ACTUAL' AND ((o.outcome='UNKNOWN' AND o.status_code=429) OR (o.outcome='RECEIVED' AND o.status_code=200))`, f.r.WorkspaceID); n != 2 {
		t.Fatalf("uncertain and resumed attempts lost accounting: %d", n)
	}
	assertBudgetRejectionFacts(t, f, rec.ID, 1)
	assertNoBudgetPublication(t, f)
}
