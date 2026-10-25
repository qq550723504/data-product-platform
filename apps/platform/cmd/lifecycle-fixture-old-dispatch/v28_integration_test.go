package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// Full retained-schema Commands on a separate restored instance. The HEAD is
// a test HTTP oracle, not native acceptance or a host containment attestation.
func TestV28ActualRestoredCommandAndReplay(t *testing.T) {
	if os.Getenv("TEST_POST_V28_RESTORED_SOURCE") != "1" {
		t.Skip("isolated actual-schema restore required")
	}
	var c config
	if e := read(configPath, &c); e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	boundedRun := func(results *[]map[string]any) error {
		return pilot.WithRecoveryRouting(func(_ *outbox.Router) error { return run(ctx, c, results) })
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	var marker string
	if e = pool.QueryRow(ctx, `SHOW lifecycle.rehearsal_marker`).Scan(&marker); e != nil || marker != "V28_ACTUAL_SCHEMA_ISOLATED_NO_AUTHORITY" {
		t.Fatal("not isolated", e)
	}
	scope, _ := pilot.RemainingScopeFor("business-races")
	target := scope.Copies[0]
	for _, copy := range scope.Copies {
		if copy.ID == pilot.PostV28BusinessDispatches()[0].Copy {
			target = copy
		}
	}
	var calls atomic.Int32
	var status atomic.Int32
	status.Store(403)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "HEAD" || r.URL.Path != "/"+target.Namespace+"/"+target.Key || r.URL.Query().Get("versionId") != target.ProviderVersion {
			t.Error("expanded request", r.Method, r.URL)
			w.WriteHeader(400)
			return
		}
		if status.Load() != 200 {
			w.WriteHeader(int(status.Load()))
			return
		}
		w.Header().Set("x-amz-version-id", target.ProviderVersion)
		w.Header().Set("Content-Length", strconv.FormatInt(target.ByteSize, 10))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("ETag", "\"test-only\"")
		w.WriteHeader(200)
	}))
	server.Listener, e = net.Listen("tcp", "127.0.0.1:9000")
	if e != nil {
		t.Fatal(e)
	}
	server.Start()
	defer server.Close()
	// Instrumented failed HEAD must not release the original DENIED reservation.
	results := []map[string]any{}
	if e = boundedRun(&results); e == nil {
		t.Fatal("403 HEAD accepted")
	}
	var released bool
	d := pilot.PostV28BusinessDispatches()[0]
	if e = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_dispatch_release WHERE dispatch_id=$1)`, d.Dispatch).Scan(&released); e != nil || released {
		t.Fatal("failed HEAD released", e)
	}
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, authority{c}, nil, nil)
	s := lc.NewExactExecution(lc.NewDispositionService(o, nil, nil, nil))
	// Current authority alone cannot invent storage proof: foreign copy, stale
	// observation and an unpersisted CallID are rejected by Command/DB contract.
	for _, kind := range []string{"foreign-copy", "stale", "unpersisted-call"} {
		presence := lc.ExactCopyInspection{Copy: target, ObservedAt: time.Now().UTC(), CallID: uuid.New()}
		if kind == "foreign-copy" {
			presence.Copy.ID = uuid.New()
		}
		if kind == "stale" {
			presence.ObservedAt = time.Now().Add(-2 * time.Minute)
		}
		if _, err := s.ReconcileDeniedAttempt(ctx, lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: c.AuditID.String() + ":" + kind}, d.Attempt, presence); err == nil {
			t.Fatal("invented presence accepted", kind)
		}
	}
	status.Store(200)
	results = nil
	if e = boundedRun(&results); e != nil || len(results) != 3 {
		t.Fatal("full restored command", e, results)
	}
	count := calls.Load()
	if count != 2 {
		t.Fatal("HEAD attempts", count)
	}
	results = nil
	if e = boundedRun(&results); e != nil || len(results) != 3 || calls.Load() != count {
		t.Fatal("replay repeated provider work", e, results)
	}
	var notSent, denied, releases int
	if e = pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE certainty='NOT_SENT'), count(*) FILTER(WHERE certainty='CONFIRMED' AND outcome='DENIED') FROM lifecycle_provider_observation WHERE attempt_id=ANY($1::uuid[])`, []uuid.UUID{d.Attempt, pilot.PostV28BusinessDispatches()[1].Attempt, pilot.PostV28BusinessDispatches()[2].Attempt}).Scan(&notSent, &denied); e != nil || notSent != 2 || denied != 1 {
		t.Fatal("old DENIED changed", notSent, denied, e)
	}
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_dispatch_release WHERE dispatch_id=ANY($1::uuid[])`, []uuid.UUID{d.Dispatch, pilot.PostV28BusinessDispatches()[1].Dispatch, pilot.PostV28BusinessDispatches()[2].Dispatch}).Scan(&releases); e != nil || releases != 3 {
		t.Fatal(releases, e)
	}
}
