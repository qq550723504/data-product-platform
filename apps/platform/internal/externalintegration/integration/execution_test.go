package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/reference"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/testkit"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type executionFixture struct {
	*fixture
	executor                 *app.Executor
	provider                 *testkit.Provider
	pub, binding, identifier uuid.UUID
	denied                   atomic.Bool
	providerName             string
}

func executionSetup(t *testing.T, useHTTP bool) *executionFixture {
	f := &executionFixture{fixture: setup(t), provider: testkit.New()}
	f.providerName = "fixture-" + f.ws.String()
	b := f.bindingReceipt(t)
	f.binding = b.ObjectID
	id, err := f.service.RecordIdentifier(f.ctx, f.ws, f.caller, "release-identifier", d.IdentifierSpec{Provider: f.providerName, ExternalID: "release:" + f.release.String(), ObjectType: "PRODUCT_RELEASE", ObjectID: f.release, BasisEvidenceID: f.basis})
	must(t, err)
	f.identifier = id.ObjectID
	pub := f.fixture.publication(t, b, f.providerName)
	f.pub = pub.ObjectID
	_, err = f.service.RequestPublication(f.ctx, f.ws, f.caller, "request", f.pub)
	must(t, err)
	adapter := f.provider.Adapter(f.providerName, "synthetic")
	if useHTTP {
		s := f.provider.Server()
		t.Cleanup(s.Close)
		c, err := reference.New(s.URL)
		must(t, err)
		adapter = c.Adapter(f.providerName, "synthetic")
	}
	registry, err := ports.NewRegistry(adapter)
	must(t, err)
	f.executor = app.NewExecutor(f.service, registry, func(ctx context.Context, ws, actor uuid.UUID, subject string) (principal.RequestPrincipal, error) {
		if f.denied.Load() {
			return principal.RequestPrincipal{}, d.ErrDenied
		}
		return f.caller(ctx)
	})
	// Production uses the worker's existing dedicated lock pool. Exercise that
	// composition here so ownership does not exhaust repository connections.
	cfg := f.pool.Config().Copy()
	cfg.MaxConns = 8
	locks, err := pgxpool.NewWithConfig(f.ctx, cfg)
	must(t, err)
	t.Cleanup(locks.Close)
	f.executor.WithLocker(transaction.NewManager(locks))
	return f
}
func (f *executionFixture) bindingReceipt(t *testing.T) d.Receipt {
	return f.fixture.binding(t, f.providerName)
}
func (f *executionFixture) prepare(t *testing.T) d.Attempt {
	_, err := f.executor.Prepare(f.ctx, f.ws, f.caller, "execute", f.pub, false)
	must(t, err)
	a, err := f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	return a
}
func (f *executionFixture) costs(t *testing.T, want int) {
	t.Helper()
	var dispatched, events, allocated int
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM external_provider_dispatch d JOIN external_provider_call c ON c.id=d.call_id WHERE c.workspace_id=$1`, f.ws).Scan(&dispatched))
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type='EXTERNAL_PROVIDER_INVOCATION' AND quantity=1 AND unit='invocation' AND pricing_mode='ACTUAL' AND amount IS NULL`, f.ws).Scan(&events))
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id JOIN external_provider_call c ON c.id=a.external_provider_call_id WHERE e.workspace_id=$1 AND e.activity_id=c.id AND c.workspace_id=e.workspace_id`, f.ws).Scan(&allocated))
	if dispatched != want || events != want || allocated != want {
		t.Fatalf("dispatch/cost/allocation=%d/%d/%d want %d", dispatched, events, allocated, want)
	}
}
func TestPublicationConcurrentClaimSingleSubmitAndCoreFreeze(t *testing.T) {
	for _, httpAdapter := range []bool{false, true} {
		t.Run(fmt.Sprint("http=", httpAdapter), func(t *testing.T) {
			f := executionSetup(t, httpAdapter)
			before := f.coreDigest(t)
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			facts := make(chan uuid.UUID, 8)
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					r, err := f.executor.Prepare(f.ctx, f.ws, f.caller, "execute", f.pub, false)
					errs <- err
					facts <- r.FactID
				}()
			}
			wg.Wait()
			close(errs)
			close(facts)
			for err := range errs {
				must(t, err)
			}
			var first uuid.UUID
			for id := range facts {
				if first == uuid.Nil {
					first = id
				}
				if id != first {
					t.Fatal("command replay changed attempt")
				}
			}
			errs = make(chan error, 8)
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- f.executor.RunPublication(f.ctx, f.ws, f.pub) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				must(t, err)
			}
			must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
			if f.provider.Count("SUBMIT") != 1 || f.provider.Facts() != 1 {
				t.Fatal("duplicate remote publication")
			}
			h, err := f.service.Repo.PublicationHistory(f.ctx, f.ws, f.pub)
			must(t, err)
			if h.Status != d.Published || len(h.Transitions) != 4 || h.Transitions[2].ProviderObservationID == nil || h.Transitions[3].ProviderObservationID == nil {
				t.Fatalf("unconfirmed history: %+v", h)
			}
			if before != f.coreDigest(t) {
				t.Fatal("external execution changed frozen Core")
			}
			f.costs(t, 3)
			var n int
			must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ExternalPublicationExecutionRequested' AND required_handlers @> ARRAY['external-publication-queue']::text[]`, f.pub).Scan(&n))
			if n != 1 {
				t.Fatal("execution obligation missing")
			}
			f.denied.Store(true)
			if _, err = f.executor.Prepare(f.ctx, f.ws, f.caller, "execute", f.pub, false); !errors.Is(err, d.ErrDenied) {
				t.Fatalf("replay skipped fresh authority: %v", err)
			}
		})
	}
}
func TestPublicationUnknownRecoversByLookupWithoutResubmit(t *testing.T) {
	f := executionSetup(t, true)
	f.provider.LostResponse = true
	a := f.prepare(t)
	if err := f.executor.RunPublication(f.ctx, f.ws, f.pub); !errors.Is(err, d.ErrUnavailable) {
		t.Fatalf("lost response: %v", err)
	}
	if _, err := f.executor.Prepare(f.ctx, f.ws, f.caller, "unsafe-retry", f.pub, true); !errors.Is(err, d.ErrState) {
		t.Fatalf("unknown retry: %v", err)
	}
	must(t, f.executor.RunOnce(f.ctx))
	latest, err := f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	if latest.ID != a.ID || latest.Resolution != "PUBLISHED" || f.provider.Count("SUBMIT") != 1 || f.provider.Count("POLL") != 1 {
		t.Fatal("unknown work was resubmitted")
	}
	f.costs(t, 4)
}
func TestPublicationDefiniteFailureRetryCreatesNewAttemptAndCost(t *testing.T) {
	f := executionSetup(t, false)
	f.provider.SubmitState = "FAILED"
	first := f.prepare(t)
	before := f.coreDigest(t)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	f.costs(t, 3)
	prior, err := f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	if prior.Resolution != "FAILED" {
		t.Fatal("definite failure missing")
	}
	f.provider.SubmitState = "PUBLISHED"
	_, err = f.executor.Prepare(f.ctx, f.ws, f.caller, "retry", f.pub, true)
	must(t, err)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	latest, err := f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	if latest.ID == first.ID || latest.Number != 2 || latest.Request.RequestKey == first.Request.RequestKey || latest.Resolution != "PUBLISHED" {
		t.Fatal("retry reused historical attempt")
	}
	if before != f.coreDigest(t) || f.provider.Count("SUBMIT") != 2 {
		t.Fatal("retry changed Core or lost actual work")
	}
	f.costs(t, 6)
}
func TestPublicationSavedProviderResultSurvivesObservationCommitFailure(t *testing.T) {
	f := executionSetup(t, false)
	f.prepare(t)
	fn := "fault_" + strings.ReplaceAll(f.ws.String(), "-", "")
	trigger := fn
	_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION `+pgx.Identifier{fn}.Sanitize()+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='`+f.ws.String()+`'::uuid AND NEW.state='PUBLISHED' THEN RAISE EXCEPTION 'injected post-provider observation fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER `+pgx.Identifier{trigger}.Sanitize()+` BEFORE INSERT ON external_provider_observation FOR EACH ROW EXECUTE FUNCTION `+pgx.Identifier{fn}.Sanitize()+`()`)
	must(t, err)
	drop := func() {
		_, _ = f.pool.Exec(f.ctx, `DROP TRIGGER IF EXISTS `+pgx.Identifier{trigger}.Sanitize()+` ON external_provider_observation; DROP FUNCTION IF EXISTS `+pgx.Identifier{fn}.Sanitize()+`()`)
	}
	t.Cleanup(drop)
	if err = f.executor.RunPublication(f.ctx, f.ws, f.pub); err == nil {
		t.Fatal("injected DB failure ignored")
	}
	f.costs(t, 3)
	if f.provider.Facts() != 1 {
		t.Fatal("provider was not called")
	}
	drop()
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	if f.provider.Count("SUBMIT") != 1 || f.provider.Count("POLL") != 1 {
		t.Fatal("recovery repeated submit after committed claim")
	}
	f.costs(t, 4)
}
func TestPublicationRevokedAfterResponseRetainsFactAndBlocksProjection(t *testing.T) {
	f := executionSetup(t, false)
	f.prepare(t)
	f.provider.Hook = func(kind string, _ d.Request, _ ports.Invocation) {
		if kind == "SUBMIT" {
			f.denied.Store(true)
		}
	}
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	a, err := f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	if a.Resolution != "BLOCKED" {
		t.Fatal("revoked result not blocked")
	}
	h, err := f.service.Repo.PublicationHistory(f.ctx, f.ws, f.pub)
	must(t, err)
	if h.Status != d.Submitting || len(h.Transitions) != 2 {
		t.Fatal("revoked caller changed projection")
	}
	calls, err := f.service.Repo.Calls(f.ctx, f.pool, f.ws, f.pub, a.ID, 50, 0)
	must(t, err)
	if len(calls) != 3 || calls[2].Result.State != "PUBLISHED" {
		t.Fatal("incurred provider result lost after revocation")
	}
	f.costs(t, 3)
}
func TestPublicationStoredObservationReplaysAfterProjectionCommitFailure(t *testing.T) {
	f := executionSetup(t, false)
	a := f.prepare(t)
	fn := "projection_fault_" + strings.ReplaceAll(f.ws.String(), "-", "")
	_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION `+pgx.Identifier{fn}.Sanitize()+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='`+f.ws.String()+`'::uuid AND NEW.status='PUBLISHED' THEN RAISE EXCEPTION 'injected projection fault'; END IF; RETURN NEW; END $$; CREATE TRIGGER `+pgx.Identifier{fn}.Sanitize()+` BEFORE UPDATE ON external_publication FOR EACH ROW EXECUTE FUNCTION `+pgx.Identifier{fn}.Sanitize()+`()`)
	must(t, err)
	drop := func() {
		_, _ = f.pool.Exec(f.ctx, `DROP TRIGGER IF EXISTS `+pgx.Identifier{fn}.Sanitize()+` ON external_publication; DROP FUNCTION IF EXISTS `+pgx.Identifier{fn}.Sanitize()+`()`)
	}
	t.Cleanup(drop)
	if err = f.executor.RunPublication(f.ctx, f.ws, f.pub); err == nil {
		t.Fatal("projection fault ignored")
	}
	calls, err := f.service.Repo.Calls(f.ctx, f.pool, f.ws, f.pub, a.ID, 50, 0)
	must(t, err)
	if len(calls) != 3 || calls[2].Result.State != "PUBLISHED" {
		t.Fatal("projection failure lost committed receipt")
	}
	f.costs(t, 3)
	drop()
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	if f.provider.Count("SUBMIT") != 1 || f.provider.Count("POLL") != 0 {
		t.Fatal("saved receipt recovery repeated provider work")
	}
	f.costs(t, 3)
}
func TestPublicationRegisteredPollAndProviderContextMismatch(t *testing.T) {
	f := executionSetup(t, false)
	f.provider.SubmitState = "REGISTERED"
	f.prepare(t)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	h, err := f.service.Repo.PublicationHistory(f.ctx, f.ws, f.pub)
	must(t, err)
	if h.Status != d.Registered {
		t.Fatal("unpublished provider result bypassed status")
	}
	f.provider.PollState = "PUBLISHED"
	f.provider.Mismatch = true
	if err = f.executor.RunPublication(f.ctx, f.ws, f.pub); !errors.Is(err, d.ErrUnavailable) {
		t.Fatalf("expanded context: %v", err)
	}
	h, err = f.service.Repo.PublicationHistory(f.ctx, f.ws, f.pub)
	must(t, err)
	if h.Status != d.Registered {
		t.Fatal("invalid provider receipt advanced projection")
	}
	f.provider.Mismatch = false
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	if f.provider.Count("SUBMIT") != 1 {
		t.Fatal("registered recovery resubmitted")
	}
	f.costs(t, 5)
}
func TestPublicationOldIntentScannerAndExecutionHTTPIsolation(t *testing.T) {
	f := executionSetup(t, false)
	must(t, f.executor.RunOnce(f.ctx))
	if f.provider.Count("SUBMIT") != 1 {
		t.Fatal("old retention-only intent stranded")
	}
	p, _ := f.caller(f.ctx)
	resolver, err := principal.NewStaticResolver(true, "test-token", p.Subject, p.ActorID.String(), []string{f.ws.String()}, []string{d.Capability})
	must(t, err)
	mux := http.NewServeMux()
	transport.New(f.service, resolver).WithExecutor(f.executor).Register(mux)
	a, err := f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	base := "/api/v1/workspaces/" + f.ws.String() + "/external-integration"
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/providers", "", 200},
		{"GET", "/publications/" + f.pub.String() + "/attempts", "", 200},
		{"GET", "/publications/" + f.pub.String() + "/attempts/" + a.ID.String() + "/calls", "", 200},
		{"GET", "/publications/" + f.pub.String() + "/attempts/" + uuid.NewString() + "/calls", "", 404},
		{"POST", "/publications/" + f.pub.String() + "/reconcile", `{"status":"PUBLISHED"}`, 400},
		{"POST", "/publications/" + f.pub.String() + "/reconcile", "{}", 202},
		{"GET", "/publications/" + f.pub.String() + "/attempts?limit=101", "", 400},
	} {
		req := httptest.NewRequest(tc.method, base+tc.path, strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}
	foreign := httptest.NewRequest("GET", strings.Replace(base, f.ws.String(), uuid.NewString(), 1)+"/providers", nil)
	foreign.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, foreign)
	if rec.Code != 403 {
		t.Fatal("foreign workspace read allowed")
	}
	var out any
	must(t, json.Unmarshal(rec.Body.Bytes(), &out))
}
func TestPublicationDatabaseImmutableAndProjectionBypassGuards(t *testing.T) {
	f := executionSetup(t, false)
	a := f.prepare(t)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	for _, sql := range []string{
		`UPDATE external_publication_attempt SET request_key='changed' WHERE id=$1`,
		`DELETE FROM external_publication_attempt WHERE id=$1`,
		`UPDATE external_provider_call SET kind='POLL' WHERE attempt_id=$1`,
		`DELETE FROM external_provider_observation WHERE call_id IN (SELECT id FROM external_provider_call WHERE attempt_id=$1)`,
		`DELETE FROM external_publication_attempt_resolution WHERE attempt_id=$1`,
	} {
		if _, err := f.pool.Exec(f.ctx, sql, a.ID); err == nil {
			t.Fatalf("immutable bypass allowed: %s", sql)
		}
	}
	g := executionSetup(t, false)
	if _, err := g.pool.Exec(g.ctx, `UPDATE external_publication SET status='REGISTERED',revision=2 WHERE id=$1`, g.pub); err == nil {
		t.Fatal("projection advanced without evidence/history")
	}
}
func TestPublicationPreflightFailureCrashDoesNotResumeSubmission(t *testing.T) {
	f := executionSetup(t, false)
	a := f.prepare(t)
	// A committed failed verification with no resolution simulates a crash
	// between observation and terminal recording.
	call, obs := uuid.New(), uuid.New()
	err := f.service.Tx.Do(f.ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO external_provider_call(id,attempt_id,workspace_id,kind,evidence_id) VALUES($1,$2,$3,'IDENTITY',$4)`, call, a.ID, f.ws, f.basis); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO external_provider_observation(id,call_id,workspace_id,state,error_code,instance_ref,request_key,request_hash,evidence_id) VALUES($1,$2,$3,'FAILED','OK',$4,$5,$6,$7)`, obs, call, f.ws, a.Request.Instance, a.Request.RequestKey, a.Request.Hash(), f.basis)
		return err
	})
	must(t, err)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	a, err = f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	if a.Resolution != "FAILED" || f.provider.Count("IDENTITY") != 0 || f.provider.Count("SUBMIT") != 0 {
		t.Fatal("failed verification was replaced by new work")
	}
	f.costs(t, 0)
}
func TestPublicationStartOnlyCrashQueriesAbsentBeforeExplicitRetry(t *testing.T) {
	f := executionSetup(t, false)
	a := f.prepare(t)
	_, err := f.pool.Exec(f.ctx, `INSERT INTO external_provider_call(id,attempt_id,workspace_id,kind,evidence_id) VALUES($1,$2,$3,'SUBMIT',$4)`, uuid.New(), a.ID, f.ws, f.basis)
	must(t, err)
	f.costs(t, 0)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	a, err = f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
	must(t, err)
	if a.Resolution != "FAILED" || f.provider.Count("SUBMIT") != 0 || f.provider.Count("POLL") != 1 {
		t.Fatal("unobserved claim blindly resubmitted")
	}
	f.costs(t, 1)
	_, err = f.executor.Prepare(f.ctx, f.ws, f.caller, "retry-absent", f.pub, true)
	must(t, err)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
	f.costs(t, 4)
}
func TestPublicationCurrentBindingAndAdapterInstanceCannotBeReused(t *testing.T) {
	for _, scenario := range []string{"identity-before", "identifier-before", "identity-after", "instance-before"} {
		t.Run(scenario, func(t *testing.T) {
			f := executionSetup(t, false)
			f.prepare(t)
			switch scenario {
			case "identity-before":
				_, err := f.service.InvalidateIdentity(f.ctx, f.ws, f.caller, "invalidate", app.InvalidateSpec{ID: f.binding, Reason: "synthetic revoke"})
				must(t, err)
			case "identifier-before":
				_, err := f.service.InvalidateIdentifier(f.ctx, f.ws, f.caller, "invalidate", app.InvalidateSpec{ID: f.identifier, Reason: "synthetic revoke"})
				must(t, err)
			case "instance-before":
				registry, err := ports.NewRegistry(f.provider.Adapter(f.providerName, "replaced"))
				must(t, err)
				f.executor.Registry = registry
			case "identity-after":
				f.provider.Hook = func(kind string, _ d.Request, _ ports.Invocation) {
					if kind == "SUBMIT" {
						_, err := f.service.InvalidateIdentity(f.ctx, f.ws, f.caller, "invalidate", app.InvalidateSpec{ID: f.binding, Reason: "synthetic revoke"})
						must(t, err)
					}
				}
			}
			err := f.executor.RunPublication(f.ctx, f.ws, f.pub)
			if scenario == "identity-after" {
				must(t, err)
				a, err := f.service.Repo.LatestAttempt(f.ctx, f.pool, f.ws, f.pub)
				must(t, err)
				if a.Resolution != "BLOCKED" {
					t.Fatal("invalidated association advanced projection")
				}
				f.costs(t, 3)
			} else {
				if err == nil {
					t.Fatal("stale dependency invoked adapter")
				}
				f.costs(t, 0)
				if f.provider.Count("IDENTITY") != 0 {
					t.Fatal("stale authority caused I/O")
				}
			}
		})
	}
}
