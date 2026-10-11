package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/ports"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/reference"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/testkit"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type usageFixture struct {
	*executionFixture
	usage  *app.UsageImporter
	feed   *testkit.UsageSource
	source uuid.UUID
}

func usageSetup(t *testing.T, httpMode, publish bool) *usageFixture {
	f := &usageFixture{executionFixture: executionSetup(t, false), feed: &testkit.UsageSource{}}
	adapter := f.provider.Adapter(f.providerName, "synthetic")
	adapter.Usage = f.feed
	adapter.Capabilities.UsageEvents = true
	if httpMode {
		server := f.provider.ServerWithUsage(f.feed)
		t.Cleanup(server.Close)
		client, err := reference.New(server.URL)
		must(t, err)
		adapter = client.Adapter(f.providerName, "synthetic")
	}
	registry, err := ports.NewRegistry(adapter)
	must(t, err)
	f.executor.Registry = registry
	f.usage = app.NewUsageImporter(f.executor)
	rec, err := f.usage.CreateSource(f.ctx, f.ws, f.caller, "source", d.UsageSourceSpec{BindingID: f.binding, Stream: "urn:fixture:" + f.ws.String()})
	must(t, err)
	f.source = rec.ObjectID
	if publish {
		f.publish(t)
	}
	return f
}
func (f *usageFixture) publish(t *testing.T) {
	f.prepare(t)
	must(t, f.executor.RunPublication(f.ctx, f.ws, f.pub))
}
func (f *usageFixture) event(id, category string) d.UsageOccurrence {
	return d.UsageOccurrence{ID: id, Source: "urn:fixture:" + f.ws.String(), Category: category, OccurredAt: time.Date(2026, 10, 11, 0, 0, 0, 123456000, time.UTC), ExternalSubjectID: "subject:" + f.providerName, ExternalIdentifier: "release:" + f.release.String(), RemoteID: "catalog:" + f.pub.String()}
}
func (f *usageFixture) pull(t *testing.T, key string) d.UsagePull {
	rec, err := f.usage.RequestPull(f.ctx, f.ws, f.caller, key, f.source)
	must(t, err)
	p, err := f.service.Repo.UsagePull(f.ctx, f.pool, f.ws, rec.FactID)
	must(t, err)
	return p
}
func (f *usageFixture) events(t *testing.T) []d.UsageEvent {
	v, err := f.service.Repo.UsageEvents(f.ctx, f.ws, "", 100, 0)
	must(t, err)
	return v
}
func (f *usageFixture) usageCosts(t *testing.T, want int) {
	var costs, allocated, dispatched int
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type='EXTERNAL_USAGE_INVOCATION' AND quantity=1 AND unit='invocation' AND pricing_mode='ACTUAL' AND amount IS NULL`, f.ws).Scan(&costs))
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id JOIN external_usage_call c ON c.id=a.external_usage_call_id WHERE c.workspace_id=$1 AND e.activity_id=c.id`, f.ws).Scan(&allocated))
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM external_usage_dispatch WHERE workspace_id=$1`, f.ws).Scan(&dispatched))
	if costs != want || allocated != want || dispatched != want {
		t.Fatalf("physical costs %d/%d/%d, want %d", costs, allocated, dispatched, want)
	}
}
func TestUsageCategoriesCursorReplayAndProductionSnapshot(t *testing.T) {
	for _, httpMode := range []bool{false, true} {
		t.Run(fmt.Sprint(httpMode), func(t *testing.T) {
			f := usageSetup(t, httpMode, true)
			before := f.coreDigest(t)
			var snapshot evidence.Snapshot
			must(t, f.service.Tx.Do(f.ctx, func(ctx context.Context, tx pgx.Tx) error {
				var err error
				snapshot, err = evidence.CreateSnapshot(ctx, tx, f.ws, "ProductRelease", f.release, map[string]any{"fixture": "production"}, []evidence.SnapshotItem{{EvidenceID: f.basis, Category: "PRODUCTION"}}, &f.actor)
				return err
			}))
			digest := func() string {
				var h string
				must(t, f.pool.QueryRow(f.ctx, `SELECT md5(jsonb_build_object('header',to_jsonb(s),'items',(SELECT jsonb_agg(to_jsonb(i) ORDER BY i.evidence_id,i.category) FROM evidence_snapshot_item i WHERE i.snapshot_id=s.id))::text) FROM evidence_snapshot s WHERE s.id=$1`, snapshot.ID).Scan(&h))
				return h
			}
			snapshotBefore := digest()
			f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
				events := []d.UsageOccurrence{}
				for _, cat := range []string{"USAGE", "ACCESS", "DELIVERY", "CONNECTOR_LOG"} {
					events = append(events, f.event(cat, cat))
				}
				return testkit.UsagePage(q, events...), nil
			})
			p := f.pull(t, "pull-1")
			must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
			items := f.events(t)
			if len(items) != 4 {
				t.Fatal(items)
			}
			for _, v := range items {
				if v.Status != "ACCEPTED" {
					t.Fatalf("not mapped %+v", v)
				}
			}
			var facts int
			must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM evidence WHERE workspace_id=$1 AND source_type='ExternalUsageEvent'`, f.ws).Scan(&facts))
			if facts != 8 {
				t.Fatal(facts)
			}
			replay := f.pull(t, "pull-1")
			if replay.ID != p.ID {
				t.Fatal("command replay changed pull")
			}
			must(t, f.usage.RunPull(f.ctx, f.ws, replay.ID))
			p2 := f.pull(t, "pull-2")
			must(t, f.usage.RunPull(f.ctx, f.ws, p2.ID))
			if len(f.events(t)) != 4 || f.feed.Reads.Load() != 2 {
				t.Fatal("duplicate occurrences/calls")
			}
			var after int
			must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM evidence WHERE workspace_id=$1 AND source_type='ExternalUsageEvent'`, f.ws).Scan(&after))
			if after != facts {
				t.Fatal("duplicate evidence")
			}
			s, err := f.service.Repo.UsageSource(f.ctx, f.pool, f.ws, f.source, false)
			must(t, err)
			if s.Revision != 2 || s.Cursor != "nn" {
				t.Fatal(s)
			}
			if before != f.coreDigest(t) || snapshotBefore != digest() {
				t.Fatal("upstream production history changed")
			}
			f.usageCosts(t, 2)
			var relations int
			must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM evidence_relation WHERE object_type='ProductRelease' AND object_id=$1 AND relation_type='DOWNSTREAM_RUNTIME'`, f.release).Scan(&relations))
			if relations != 4 {
				t.Fatal(relations)
			}
		})
	}
}
func TestUsageUnmappedRecheckAndConflictQuarantine(t *testing.T) {
	f := usageSetup(t, false, false)
	v := f.event("later", "USAGE")
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		return testkit.UsagePage(q, v), nil
	})
	p := f.pull(t, "initial")
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	events := f.events(t)
	if len(events) != 1 || events[0].Status != "QUARANTINED" {
		t.Fatal(events)
	}
	_, err := f.usage.Recheck(f.ctx, f.ws, f.caller, "no-map", events[0].ID)
	if !errors.Is(err, d.ErrState) {
		t.Fatal(err)
	}
	f.publish(t)
	r, err := f.usage.Recheck(f.ctx, f.ws, f.caller, "mapped", events[0].ID)
	must(t, err)
	r2, err := f.usage.Recheck(f.ctx, f.ws, f.caller, "mapped", events[0].ID)
	must(t, err)
	if r != r2 {
		t.Fatal("recheck replay changed receipt")
	}
	decisions, err := f.service.Repo.UsageDecisions(f.ctx, f.pool, f.ws, events[0].ID)
	must(t, err)
	if len(decisions) != 2 || f.events(t)[0].Status != "ACCEPTED" {
		t.Fatal(decisions)
	}
	v.Category = "ACCESS"
	p = f.pull(t, "conflict")
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	p = f.pull(t, "duplicate-conflict")
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	conflicts, err := f.service.Repo.UsageConflicts(f.ctx, f.ws, 100, 0)
	must(t, err)
	if len(conflicts) != 1 || len(f.events(t)) != 1 || f.events(t)[0].UsageOccurrence.Category != "USAGE" {
		t.Fatal("history replaced/duplicate conflicts")
	}
	for _, sql := range []string{`UPDATE external_usage_event SET category='ACCESS' WHERE workspace_id=$1`, `DELETE FROM external_usage_decision WHERE workspace_id=$1`, `UPDATE external_usage_checkpoint SET cursor='rewind' WHERE workspace_id=$1`, `DELETE FROM external_usage_outcome WHERE workspace_id=$1`} {
		if _, err = f.pool.Exec(f.ctx, sql, f.ws); err == nil {
			t.Fatal("immutable mutation accepted", sql)
		}
	}
}
func TestUsageIsolationUnknownAndProviderContext(t *testing.T) {
	f := usageSetup(t, false, true)
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		valid := f.event("good", "USAGE")
		subject := f.event("subject", "ACCESS")
		subject.ExternalSubjectID = "foreign"
		remote := f.event("remote", "DELIVERY")
		remote.RemoteID = "another-publication"
		missing := f.event("missing", "CONNECTOR_LOG")
		missing.ExternalIdentifier = ""
		unknown := f.event("future", "FUTURE")
		return testkit.UsagePage(q, valid, subject, remote, missing, unknown), nil
	})
	p := f.pull(t, "mixed")
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	items := f.events(t)
	accepted := 0
	for _, v := range items {
		if v.Status == "ACCEPTED" {
			accepted++
		}
	}
	if len(items) != 5 || accepted != 1 {
		t.Fatal(items)
	}
	other := usageSetup(t, false, true)
	if _, err := f.service.Repo.UsageEvent(f.ctx, f.pool, other.ws, items[0].ID, false); !errors.Is(err, d.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := other.usage.RequestPull(other.ctx, other.ws, other.caller, "foreign", f.source); !errors.Is(err, d.ErrNotFound) {
		t.Fatal(err)
	}
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		page := testkit.UsagePage(q, f.event("wrong-envelope", "USAGE"))
		page.Instance = "other-instance"
		return page, nil
	})
	p = f.pull(t, "wrong-envelope")
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	source, err := f.service.Repo.UsageSource(f.ctx, f.pool, f.ws, f.source, false)
	must(t, err)
	if source.Revision != 1 || len(f.events(t)) != 5 {
		t.Fatal("unverified envelope advanced cursor")
	}
	f.usageCosts(t, 2)
}
func TestUsageConcurrentPullsAndFreshAuthority(t *testing.T) {
	f := usageSetup(t, false, true)
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		return testkit.UsagePage(q, f.event("concurrent", "USAGE")), nil
	})
	p1 := f.pull(t, "one")
	p2 := f.pull(t, "two")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- f.usage.RunPull(f.ctx, f.ws, p1.ID) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		must(t, err)
	}
	must(t, f.usage.RunPull(f.ctx, f.ws, p2.ID))
	p2, err := f.service.Repo.UsagePull(f.ctx, f.pool, f.ws, p2.ID)
	must(t, err)
	if p2.Outcome != "STALE" || f.feed.Reads.Load() != 1 {
		t.Fatal(p2)
	}
	f.usageCosts(t, 1)
	p3 := f.pull(t, "revoke")
	f.denied.Store(true)
	_, err = f.usage.RequestPull(f.ctx, f.ws, f.caller, "revoke", f.source)
	if !errors.Is(err, d.ErrDenied) {
		t.Fatal("stale command authority replay", err)
	}
	must(t, f.usage.RunPull(f.ctx, f.ws, p3.ID))
	if f.feed.Reads.Load() != 1 {
		t.Fatal("revoked caller dispatched")
	}
}
func TestUsageSavedPageSurvivesAcceptanceFault(t *testing.T) {
	f := usageSetup(t, false, true)
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		return testkit.UsagePage(q, f.event("saved", "USAGE")), nil
	})
	p := f.pull(t, "fault")
	name := "usage_fault_" + strings.ReplaceAll(f.ws.String(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION `+ident+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='`+f.ws.String()+`'::uuid THEN RAISE EXCEPTION 'synthetic checkpoint fault';END IF;RETURN NEW;END $$;CREATE TRIGGER `+ident+` BEFORE INSERT ON external_usage_checkpoint FOR EACH ROW EXECUTE FUNCTION `+ident+`()`)
	must(t, err)
	drop := func() {
		_, err := f.pool.Exec(f.ctx, `DROP TRIGGER IF EXISTS `+ident+` ON external_usage_checkpoint;DROP FUNCTION IF EXISTS `+ident+`() `)
		must(t, err)
	}
	t.Cleanup(drop)
	if err = f.usage.RunPull(f.ctx, f.ws, p.ID); err == nil {
		t.Fatal("fault ignored")
	}
	if len(f.events(t)) != 0 {
		t.Fatal("partial page acceptance")
	}
	f.usageCosts(t, 1)
	drop()
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	if len(f.events(t)) != 1 || f.feed.Reads.Load() != 1 {
		t.Fatal("saved page reread/lost")
	}
	f.usageCosts(t, 1)
}
func TestUsageLostResponseStartOnlyAndAuthorityAfterIO(t *testing.T) {
	for _, mode := range []string{"lost", "start-only", "revoke-after"} {
		t.Run(mode, func(t *testing.T) {
			f := usageSetup(t, false, true)
			p := f.pull(t, "pull")
			f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
				if mode == "lost" {
					return d.UsagePage{}, errors.New("private raw provider diagnostic")
				}
				if mode == "revoke-after" {
					f.denied.Store(true)
				}
				return testkit.UsagePage(q, f.event("recovered", "USAGE")), nil
			})
			if mode == "start-only" {
				_, err := f.pool.Exec(f.ctx, `INSERT INTO external_usage_call(id,pull_id,source_id,workspace_id,request_hash,evidence_id)VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), p.ID, f.source, f.ws, strings.Repeat("a", 64), f.basis)
				must(t, err)
			}
			must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
			f.usageCosts(t, 1)
			source, err := f.service.Repo.UsageSource(f.ctx, f.pool, f.ws, f.source, false)
			must(t, err)
			if mode == "start-only" {
				if source.Revision != 1 || len(f.events(t)) != 1 {
					t.Fatal("start-only did not recover")
				}
			} else {
				if source.Revision != 0 || len(f.events(t)) != 0 {
					t.Fatal("failure/revocation imported page")
				}
			}
		})
	}
}
func TestUsageHTTPAuthIsolationAndStrictCommands(t *testing.T) {
	f := usageSetup(t, false, true)
	resolver, err := principal.NewStaticResolver(true, "synthetic-token", "fixture-subject", f.actor.String(), []string{f.ws.String()}, []string{d.Capability})
	must(t, err)
	// Fixture's authenticated subject must agree with the durable source.
	source, err := f.service.Repo.UsageSource(f.ctx, f.pool, f.ws, f.source, false)
	must(t, err)
	resolver, err = principal.NewStaticResolver(true, "synthetic-token", source.PrincipalRef, f.actor.String(), []string{f.ws.String()}, []string{d.Capability})
	must(t, err)
	mux := http.NewServeMux()
	transport.New(f.service, resolver).WithExecutor(f.executor).WithUsage(f.usage).Register(mux)
	base := "/api/v1/workspaces/" + f.ws.String() + "/external-integration"
	for _, tc := range []struct {
		path, method, body, token string
		want                      int
	}{{"/usage-sources", "GET", "", "", 401}, {"/usage-sources/" + f.source.String() + "/pull", "POST", `{"releaseId":"bad"}`, "synthetic-token", 400}, {"/usage-sources/" + uuid.NewString(), "GET", "", "synthetic-token", 404}, {"/usage-events?status=BAD", "GET", "", "synthetic-token", 400}, {"/usage-sources", "GET", "", "synthetic-token", 200}} {
		req := httptest.NewRequest(tc.method, base+tc.path, strings.NewReader(tc.body))
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		req.Header.Set("Idempotency-Key", "http-test")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("%s=%d %s", tc.path, w.Code, w.Body.String())
		}
	}
	raw, err := json.Marshal(d.UsageEvent{UsageOccurrence: f.event("provider-id", "USAGE")})
	must(t, err)
	if !strings.Contains(string(raw), `"id":"provider-id"`) {
		t.Fatal("provider event ID missing from read model")
	}
}

func TestUsageOutcomeCommitFaultRereadsSameCursorWithNewCost(t *testing.T) {
	f := usageSetup(t, false, true)
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		if q.Cursor != "" {
			t.Error("cursor advanced after failed outcome commit")
		}
		return testkit.UsagePage(q, f.event("outcome-fault", "USAGE")), nil
	})
	p := f.pull(t, "fault")
	name := "usage_outcome_fault_" + strings.ReplaceAll(f.ws.String(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION `+ident+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='`+f.ws.String()+`'::uuid THEN RAISE EXCEPTION 'synthetic outcome fault';END IF;RETURN NEW;END $$;CREATE TRIGGER `+ident+` BEFORE INSERT ON external_usage_outcome FOR EACH ROW EXECUTE FUNCTION `+ident+`()`)
	must(t, err)
	drop := func() {
		_, err := f.pool.Exec(f.ctx, `DROP TRIGGER IF EXISTS `+ident+` ON external_usage_outcome;DROP FUNCTION IF EXISTS `+ident+`() `)
		must(t, err)
	}
	t.Cleanup(drop)
	if f.usage.RunPull(f.ctx, f.ws, p.ID) == nil {
		t.Fatal("outcome fault ignored")
	}
	f.usageCosts(t, 1)
	drop()
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	if len(f.events(t)) != 1 || f.feed.Reads.Load() != 2 {
		t.Fatal("read-only lost page not recovered")
	}
	f.usageCosts(t, 2)
}
func TestUsageBindingAndAdapterChangesFailClosed(t *testing.T) {
	for _, mode := range []string{"binding-before", "binding-after", "instance", "revision", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			f := usageSetup(t, false, true)
			p := f.pull(t, "pull")
			f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
				if mode == "binding-after" {
					_, err := f.service.InvalidateIdentity(ctx, f.ws, f.caller, "revoke", app.InvalidateSpec{ID: f.binding, Reason: "synthetic invalidation"})
					must(t, err)
				}
				return testkit.UsagePage(q, f.event("event", "USAGE")), nil
			})
			if mode == "binding-before" {
				_, err := f.service.InvalidateIdentity(f.ctx, f.ws, f.caller, "revoke", app.InvalidateSpec{ID: f.binding, Reason: "synthetic invalidation"})
				must(t, err)
			}
			if mode == "instance" || mode == "revision" || mode == "unsupported" {
				a := f.provider.Adapter(f.providerName, "synthetic")
				a.Usage = f.feed
				a.Capabilities.UsageEvents = true
				if mode == "instance" {
					a.Instance = "other"
				}
				if mode == "revision" {
					a.Revision = "other"
				}
				if mode == "unsupported" {
					a.Usage = nil
					a.Capabilities.UsageEvents = false
				}
				reg, err := ports.NewRegistry(a)
				must(t, err)
				f.executor.Registry = reg
			}
			if mode == "unsupported" {
				must(t, f.usage.RunOnce(f.ctx))
			} else {
				must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
			}
			want := int64(0)
			if mode == "binding-after" {
				want = 1
			}
			if f.feed.Reads.Load() != want || len(f.events(t)) != 0 {
				t.Fatal("stale source imported/dispatched")
			}
			f.usageCosts(t, int(want))
		})
	}
}
func TestUsageCheckpointCannotSkipPageOccurrences(t *testing.T) {
	f := usageSetup(t, false, true)
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		return testkit.UsagePage(q, f.event("required", "USAGE")), nil
	})
	p := f.pull(t, "pull")
	name := "usage_skip_fault_" + strings.ReplaceAll(f.ws.String(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION `+ident+`() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='`+f.ws.String()+`'::uuid THEN RAISE EXCEPTION 'synthetic checkpoint fault';END IF;RETURN NEW;END $$;CREATE TRIGGER `+ident+` BEFORE INSERT ON external_usage_checkpoint FOR EACH ROW EXECUTE FUNCTION `+ident+`()`)
	must(t, err)
	drop := func() {
		_, err := f.pool.Exec(f.ctx, `DROP TRIGGER IF EXISTS `+ident+` ON external_usage_checkpoint;DROP FUNCTION IF EXISTS `+ident+`() `)
		must(t, err)
	}
	t.Cleanup(drop)
	if f.usage.RunPull(f.ctx, f.ws, p.ID) == nil {
		t.Fatal("fault ignored")
	}
	drop()
	saved, err := f.service.Repo.UsageRecovery(f.ctx, f.ws, p.ID)
	must(t, err)
	tx, err := f.pool.Begin(f.ctx)
	must(t, err)
	defer tx.Rollback(f.ctx)
	_, err = tx.Exec(f.ctx, `INSERT INTO external_usage_checkpoint(source_id,workspace_id,revision,pull_id,call_id,cursor,evidence_id)VALUES($1,$2,1,$3,$4,$5,$6)`, f.source, f.ws, p.ID, saved.ID, saved.Page.NextCursor, f.basis)
	if err == nil {
		t.Fatal("checkpoint advanced without any usage/quarantine decision")
	}
	must(t, tx.Rollback(f.ctx))
	// The same completeness invariant must also prevent orphan occurrences from
	// becoming durable before any pull can acknowledge them.
	tx, err = f.pool.Begin(f.ctx)
	must(t, err)
	defer tx.Rollback(f.ctx)
	v := saved.Page.Events[0]
	_, err = tx.Exec(f.ctx, `INSERT INTO external_usage_event(id,source_id,workspace_id,call_id,page_index,provider,instance_ref,event_source,provider_event_id,category,occurred_at,external_subject_id,external_identifier,remote_id,payload_hash,evidence_id)VALUES($1,$2,$3,$4,0,$5,'synthetic',$6,$7,$8,$9,$10,$11,$12,$13,$14)`, uuid.New(), f.source, f.ws, saved.ID, f.providerName, v.Source, v.ID, v.Category, v.OccurredAt, v.ExternalSubjectID, v.ExternalIdentifier, v.RemoteID, v.Hash(), f.basis)
	must(t, err)
	if err = tx.Commit(f.ctx); err == nil {
		t.Fatal("occurrence committed without its immutable decision")
	}
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	if len(f.events(t)) != 1 || f.feed.Reads.Load() != 1 {
		t.Fatal("complete saved page did not recover")
	}
}

func TestUsageReadDeadlinePreservesInvocationCost(t *testing.T) {
	f := usageSetup(t, false, true)
	f.feed.Set(func(ctx context.Context, q d.UsageReadRequest) (d.UsagePage, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("provider read has no bounded deadline")
		}
		<-ctx.Done()
		return d.UsagePage{}, ctx.Err()
	})
	p := f.pull(t, "timeout")
	must(t, f.usage.RunPull(f.ctx, f.ws, p.ID))
	got, err := f.service.Repo.UsagePull(f.ctx, f.pool, f.ws, p.ID)
	must(t, err)
	if got.Outcome != "FAILED" || len(f.events(t)) != 0 {
		t.Fatal("timed out read imported or stayed pending")
	}
	f.usageCosts(t, 1)
}
