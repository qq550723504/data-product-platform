package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

type absenceRow struct {
	n   int
	err error
}

func (r absenceRow) Scan(dest ...any) error {
	if r.err == nil {
		*dest[0].(*int) = r.n
	}
	return r.err
}

type absenceStore struct {
	receipts map[uuid.UUID]map[uuid.UUID]bool
	err      error
	queries  int
}

func (s *absenceStore) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	s.queries++
	n := 0
	for _, id := range args[1].([]uuid.UUID) {
		if s.receipts[args[0].(uuid.UUID)][id] {
			n++
		}
	}
	return absenceRow{n, s.err}
}

func TestPredecessorReceiptsRejectWrongWorkspaceMissingVersionAndLookupFailure(t *testing.T) {
	scope, _ := pilot.RemainingScopeFor("business-races")
	p1, _ := historicalCopy(scope.Group, "P1")
	foreign := uuid.New()
	store := &absenceStore{receipts: map[uuid.UUID]map[uuid.UUID]bool{
		foreign:         {p1.ID: true},
		scope.Workspace: {uuid.New(): true},
	}}
	if err := requirePriorAbsences(context.Background(), store, scope, "P1"); !errors.Is(err, lc.ErrStorageScope) || store.queries != 0 {
		t.Fatal("historical P1 became an executable stage", err)
	}
	if err := requirePriorAbsences(context.Background(), store, scope, "P2"); !errors.Is(err, lc.ErrStorageDenied) {
		t.Fatal("foreign tenant or another exact copy counted as predecessor absence", err)
	}
	store.receipts[scope.Workspace][p1.ID] = true
	if err := requirePriorAbsences(context.Background(), store, scope, "P2"); err != nil {
		t.Fatal(err)
	}
	if err := requirePriorAbsences(context.Background(), store, scope, "T1"); !errors.Is(err, lc.ErrStorageDenied) {
		t.Fatal("only the oldest predecessor was checked", err)
	}
	store.err = errors.New("Core unavailable")
	if err := requirePriorAbsences(context.Background(), store, scope, "P2"); !errors.Is(err, store.err) {
		t.Fatal("Core read failure bypassed sequence", err)
	}
	if _, err := predecessorCopies("business-races", "H0"); !errors.Is(err, lc.ErrStorageScope) {
		t.Fatal("survivor/foreign stage admitted")
	}
	mixed, _ := predecessorCopies("mixed", "M1B")
	a, _ := fixedTarget("mixed", "M1A")
	if !reflect.DeepEqual(mixed, []uuid.UUID{a.ID}) {
		t.Fatal("mixed B lost its A absence prerequisite")
	}
}

type sequenceReadJournal struct{ starts, observations int }

func (s *sequenceReadJournal) StartStorageCall(context.Context, lc.StorageCallStart) error {
	s.starts++
	return nil
}
func (s *sequenceReadJournal) ObserveStorageCall(context.Context, lc.StorageCallObservation) error {
	s.observations++
	return nil
}

// Real SDK + real wire + SDK LIST, against a loopback simulated provider only.
// The receipt store below is a fake Core; PostgreSQL receipt invariants have a
// separate application integration test. This never calls a retained MinIO.
func TestSixSendDisableRecoveryCompositionCannotBatchPastUnreconciledDeletion(t *testing.T) {
	for _, group := range []string{"business-races", "late-hold", "mixed"} {
		t.Run(group, func(t *testing.T) {
			testGroupSendDisableRecovery(t, group, map[string][]string{"business-races": {"P2", "T1", "T2"}, "late-hold": {"H2"}, "mixed": {"M1A", "M1B"}}[group])
		})
	}
}
func testGroupSendDisableRecovery(t *testing.T, group string, stages []string) {
	ctx := context.Background()
	scope, _ := pilot.RemainingScopeFor(group)
	var targets []domain.Copy
	attempts := map[uuid.UUID]domain.ProviderAttempt{}
	remaining := map[uuid.UUID]domain.Copy{}
	for _, stage := range stages {
		cp, a := returnAttempt(t, scope.Group, stage)
		targets = append(targets, cp)
		attempts[cp.ID], remaining[cp.ID] = a, cp
	}
	// Seed every retained business version too: a complete namespace lookup
	// must retain these, rather than passing after the target namespace empties.
	survivors := map[uuid.UUID]domain.Copy{}
	for _, cp := range scope.Copies {
		if _, target := attempts[cp.ID]; !target && cp.Key != "fixture/p1.csv" {
			survivors[cp.ID], remaining[cp.ID] = cp, cp
		}
	}
	// Exact immutable produced copies from candidate12/bindings.json, group
	// business-races. They supplement the six original RemainingScope copies.
	var err error
	if group == "business-races" {
		raw, err := os.ReadFile(filepath.Join("testdata", "business-produced-copies.json"))
		var produced []domain.Copy
		if err != nil || json.Unmarshal(raw, &produced) != nil || len(produced) != 2 {
			t.Fatal("produced survivor fixture invalid", err)
		}
		for _, cp := range produced {
			if cp.WorkspaceID != scope.Workspace || cp.ProviderInstanceID != scope.Provider || cp.IncarnationID != scope.Incarnation || cp.ProviderVersion == "" || cp.ByteSize <= 0 {
				t.Fatal("produced survivor scope invalid")
			}
			if _, exists := remaining[cp.ID]; exists {
				t.Fatal("produced survivor duplicates a target or control")
			}
			survivors[cp.ID], remaining[cp.ID] = cp, cp
		}
	}
	if len(survivors) != map[string]int{"business-races": 4, "late-hold": 5, "mixed": 2}[group] {
		t.Fatal("business survivor fixture changed")
	}
	var mu sync.Mutex
	var deletes, lookups int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if req.Method == "DELETE" {
			for id, cp := range remaining {
				if req.URL.Path == "/"+cp.Namespace+"/"+cp.Key && req.URL.Query().Get("versionId") == cp.ProviderVersion {
					delete(remaining, id)
					deletes++
					w.WriteHeader(204)
					return
				}
			}
			t.Error("unexpected or repeated simulated provider deletion")
			w.WriteHeader(403)
			return
		}
		if req.Method != "GET" || !req.URL.Query().Has("versions") {
			t.Error("unexpected simulated recovery request")
			w.WriteHeader(403)
			return
		}
		lookups++
		type version struct {
			Key, LastModified, ETag string
			VersionID               string `xml:"VersionId"`
			Size                    int64
		}
		listing := struct {
			XMLName     xml.Name  `xml:"ListVersionsResult"`
			Name        string    `xml:"Name"`
			Prefix      string    `xml:"Prefix"`
			IsTruncated bool      `xml:"IsTruncated"`
			Versions    []version `xml:"Version"`
		}{Name: strings.Trim(req.URL.Path, "/")}
		for _, cp := range remaining {
			if cp.Namespace == listing.Name {
				listing.Versions = append(listing.Versions, version{Key: cp.Key, VersionID: cp.ProviderVersion, LastModified: time.Now().UTC().Format(time.RFC3339), ETag: "fixture", Size: cp.ByteSize})
			}
		}
		body, err := xml.Marshal(listing)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.Write(body)
	}))
	defer provider.Close()
	backend, _ := url.Parse(provider.URL)
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	budget, wireOutput := t.TempDir(), t.TempDir()
	sendEnabled := true
	witnessedLate := 0
	wire := httptest.NewServer(&wireHandler{scope: targets, access: "test-delete", key: key,
		backend: backend, transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
		output: wireOutput, budget: budget, fresh: func() error {
			if !sendEnabled {
				return errDenied
			}
			return nil
		},
		after: func(_ context.Context, p signed) error {
			var w wireWitness
			if json.Unmarshal(p.Body, &w) != nil || w.UpstreamStatus != 204 || w.Copy.Key != "fixture/h2-a.csv" {
				return errDenied
			}
			witnessedLate++
			return nil
		},
		attempt: func(_ context.Context, cp domain.Copy) (domain.ProviderAttempt, error) { return attempts[cp.ID], nil }})
	defer wire.Close()
	p1, _ := historicalCopy(scope.Group, "P1")
	store := &absenceStore{receipts: map[uuid.UUID]map[uuid.UUID]bool{scope.Workspace: {}}}
	if group == "business-races" {
		store.receipts[scope.Workspace][p1.ID] = true
	}
	readJournal := &sequenceReadJournal{}
	inspector, err := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{
		Endpoint: strings.TrimPrefix(provider.URL, "http://"), AccessKey: "test-inspector", SecretKey: "test-only",
		WorkspaceID: scope.Workspace, ProviderInstanceID: scope.Provider, IncarnationID: scope.Incarnation,
		Namespaces: []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}, Journal: readJournal})
	if err != nil {
		t.Fatal(err)
	}
	var trace []string
	for i, stage := range stages {
		sendEnabled = true
		cp := targets[i]
		if err = requirePriorAbsences(ctx, store, scope, stage); err != nil {
			t.Fatal(err)
		}
		exact := &returnTestJournal{}
		j := &journal{exact: exact, authorize: func() error { return nil }, budget: budget}
		sendErr := testDeleter(t, wire.URL, cp, j).DeleteExactCopy(ctx, attempts[cp.ID], cp)
		if err = finishSendReturn(filepath.Join(t.TempDir(), stage+".json"), stage, attempts[cp.ID], cp, j, sendErr, func() (string, string, error) { return exact.proof.Certainty, exact.proof.Outcome, nil }); err != nil {
			t.Fatal(err)
		}
		trace = append(trace, "send-"+stage)
		sendEnabled = false
		trace = append(trace, "disable-"+stage)
		retry := &journal{exact: &returnTestJournal{}, authorize: func() error { return errDenied }, budget: budget}
		if e := testDeleter(t, wire.URL, cp, retry).DeleteExactCopy(ctx, attempts[cp.ID], cp); e == nil {
			t.Fatal("disabled sender replay admitted")
		}
		if i+1 < len(stages) {
			next := targets[i+1]
			if err = requirePriorAbsences(ctx, store, scope, stages[i+1]); !errors.Is(err, lc.ErrStorageDenied) {
				t.Fatal("204 without committed absence admitted next target", err)
			}
			if _, err = os.Stat(filepath.Join(budget, next.ID.String()+".json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("early next-target check consumed a durable slot", err)
			}
		}
		inv, err := inspector.InspectNamespace(ctx, cp)
		if err != nil {
			t.Fatal(err)
		}
		for _, version := range inv.Versions {
			if version.Key == cp.Key && version.Version == cp.ProviderVersion {
				t.Fatal("recovery treated an existing exact version as absent")
			}
		}
		wantVersions := 0
		for _, present := range remaining {
			if present.Namespace == cp.Namespace {
				wantVersions++
			}
		}
		if len(inv.Versions) != wantVersions {
			t.Fatal("recovery accepted an incomplete namespace")
		}
		// Simulated receipt commit, deliberately after the real SDK LIST.
		store.receipts[scope.Workspace][cp.ID] = true
		trace = append(trace, "recover-"+stage)
	}
	var want []string
	for _, stage := range stages {
		want = append(want, "send-"+stage, "disable-"+stage, "recover-"+stage)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(trace, want) || deletes != len(stages) || lookups != len(stages) || !reflect.DeepEqual(remaining, survivors) || readJournal.starts != len(stages) || readJournal.observations != len(stages) {
		t.Fatalf("composition incomplete: trace=%v deletes=%d lookups=%d remaining=%d", trace, deletes, lookups, len(remaining))
	}
	if (group == "late-hold" && witnessedLate != 1) || (group != "late-hold" && witnessedLate != 0) {
		t.Fatal("late witness order invalid")
	}
	files, _ := filepath.Glob(filepath.Join(budget, "*.json"))
	sort.Strings(files)
	if len(files) != 2*len(stages) {
		t.Fatal(fmt.Sprintf("expected one target and one wire slot per stage, got %v", files))
	}
}
