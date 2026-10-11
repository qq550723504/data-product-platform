package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/infrastructure"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/externalintegration/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func init() {
	router, err := routing.NewRouter(false)
	if err != nil {
		panic(err)
	}
	outbox.ConfigureAppendObligation(router)
}

type fixture struct {
	ctx                                                 context.Context
	pool                                                *pgxpool.Pool
	service                                             *app.Service
	ws, actor, basis, release, draft, product, resource uuid.UUID
	caller                                              app.Caller
	appName                                             string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := "issue84-" + uuid.NewString()
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &fixture{ctx: ctx, pool: pool, ws: uuid.New(), actor: uuid.New(), basis: uuid.New(), release: uuid.New(), draft: uuid.New(), product: uuid.New(), resource: uuid.New(), appName: name}
	f.service = app.New(transaction.NewManager(pool), infra.New(pool))
	f.caller = func(context.Context) (principal.RequestPrincipal, error) {
		return principal.RequestPrincipal{WorkspaceID: f.ws, ActorID: f.actor, Subject: "trusted/" + f.actor.String(), Capabilities: map[string]struct{}{d.Capability: {}}}, nil
	}
	must(t, f.service.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := evidence.Append(ctx, tx, evidence.Record{ID: f.basis, WorkspaceID: f.ws, EvidenceType: "TEST_BASIS", Title: "Synthetic caller supplied association evidence", CreatedBy: &f.actor})
		return err
	}))
	ps := productapp.NewService(f.service.Tx, productinfra.NewPostgresRepository(pool))
	product, err := ps.CreateProduct(ctx, productapp.CreateProductCommand{WorkspaceID: f.ws, Code: "EXT-" + uuid.NewString(), Name: "Synthetic external release fixture", ActorID: &f.actor})
	must(t, err)
	f.product = product.ID
	dataset, version := uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type,lifecycle_status) VALUES($1,$2,$3,'fixture','CURATED','ACTIVE')`, dataset, f.ws, "EXT-"+uuid.NewString())
	must(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_type,storage_uri,content_type,checksum_algorithm,checksum_value,ready_at) VALUES($1,$2,1,'READY','OBJECT_STORAGE','s3://fixture/external.csv','text/csv','SHA256',$3,now())`, version, dataset, strings.Repeat("a", 64))
	must(t, err)
	pv, err := ps.CreateVersion(ctx, productapp.CreateVersionCommand{ProductID: f.product, MajorVersion: 1, Assets: []productdomain.AssetSpec{{AssetType: productdomain.AssetDataset, Name: "fixture", DatasetID: &dataset}}, ActorID: &f.actor})
	must(t, err)
	release, err := ps.CreateRelease(ctx, productapp.CreateReleaseCommand{ProductID: f.product, ProductVersionID: pv.ID, ReleaseNo: "EXT-PUBLISHED", Datasets: []productdomain.ReleaseDataset{{DatasetVersionID: version, Role: "PRIMARY"}}, ActorID: &f.actor})
	must(t, err)
	f.release = release.ID
	draft, err := ps.CreateRelease(ctx, productapp.CreateReleaseCommand{ProductID: f.product, ProductVersionID: pv.ID, ReleaseNo: "EXT-DRAFT", Datasets: []productdomain.ReleaseDataset{{DatasetVersionID: version, Role: "PRIMARY"}}, ActorID: &f.actor})
	must(t, err)
	f.draft = draft.ID
	// Synthetic PUBLISHED prerequisite for #84 boundary tests. This is not
	// ProductRelease readiness/business acceptance or a provider publication.
	must(t, f.service.Tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := deliveryfence.Lock(ctx, tx, f.ws); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE product_release SET status='READY' WHERE id=$1`, f.release); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE product_release SET status='PUBLISHED',released_at=now(),released_by=$2 WHERE id=$1`, f.release, f.actor)
		return err
	}))
	_, err = pool.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type,lifecycle_status) VALUES($1,$2,$3,'fixture','TABLE_LIKE','READY')`, f.resource, f.ws, "EXT-"+uuid.NewString())
	must(t, err)
	return f
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) binding(t *testing.T, provider string) d.Receipt {
	t.Helper()
	r, err := f.service.BindIdentity(f.ctx, f.ws, f.caller, "bind-"+provider, d.BindingSpec{Provider: provider, ExternalSubjectID: "subject:" + provider, BasisEvidenceID: f.basis})
	must(t, err)
	return r
}
func (f *fixture) publication(t *testing.T, binding d.Receipt, provider string) d.Receipt {
	t.Helper()
	r, err := f.service.CreatePublication(f.ctx, f.ws, f.caller, "create-"+provider, d.PublicationSpec{Provider: provider, IdentityBindingID: binding.ObjectID, ProductReleaseID: f.release})
	must(t, err)
	return r
}
func (f *fixture) coreDigest(t *testing.T) string {
	t.Helper()
	var hash string
	must(t, f.pool.QueryRow(f.ctx, `SELECT md5(jsonb_build_object('release',to_jsonb(r),'membership',(SELECT jsonb_agg(to_jsonb(m) ORDER BY m.dataset_version_id,m.role) FROM product_release_dataset m WHERE m.release_id=r.id),'version',to_jsonb(v),'assets',(SELECT jsonb_agg(to_jsonb(a) ORDER BY a.id) FROM product_asset a WHERE a.product_version_id=v.id))::text) FROM product_release r JOIN product_version v ON v.id=r.product_version_id WHERE r.id=$1`, f.release).Scan(&hash))
	return hash
}
func TestExternalCoreVerticalSliceReplayIsolationAndReleaseHistory(t *testing.T) {
	f := setup(t)
	before := f.coreDigest(t)
	b := f.binding(t, "exchange-a")
	pub := f.publication(t, b, "exchange-a")
	b2 := f.binding(t, "space-b")
	_ = f.publication(t, b2, "space-b")
	spec := d.PublicationSpec{Provider: "exchange-a", IdentityBindingID: b.ObjectID, ProductReleaseID: f.release}
	var wg sync.WaitGroup
	receipts := make(chan d.Receipt, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.service.CreatePublication(f.ctx, f.ws, f.caller, "create-exchange-a", spec)
			receipts <- r
			errs <- e
		}()
	}
	wg.Wait()
	close(receipts)
	close(errs)
	for e := range errs {
		must(t, e)
	}
	for r := range receipts {
		if r != pub {
			t.Fatal("concurrent replay changed receipt")
		}
	}
	changed := spec
	changed.ProductReleaseID = f.draft
	_, err := f.service.CreatePublication(f.ctx, f.ws, f.caller, "create-exchange-a", changed)
	if !errors.Is(err, d.ErrConflict) {
		t.Fatalf("changed fingerprint: %v", err)
	}
	_, err = f.service.CreatePublication(f.ctx, f.ws, f.caller, "new-key-same-publication", spec)
	if !errors.Is(err, d.ErrConflict) {
		t.Fatalf("duplicate natural intent: %v", err)
	}
	_, err = f.service.CreatePublication(f.ctx, f.ws, f.caller, "nonpublished", changed)
	if !errors.Is(err, d.ErrState) {
		t.Fatalf("draft release allowed: %v", err)
	}
	foreign := uuid.New()
	_, err = f.service.CreatePublication(f.ctx, foreign, func(context.Context) (principal.RequestPrincipal, error) {
		p, _ := f.caller(f.ctx)
		p.WorkspaceID = foreign
		return p, nil
	}, "foreign", spec)
	if !errors.Is(err, d.ErrNotFound) {
		t.Fatalf("foreign binding revealed: %v", err)
	}
	req, err := f.service.RequestPublication(f.ctx, f.ws, f.caller, "request", pub.ObjectID)
	must(t, err)
	again, err := f.service.RequestPublication(f.ctx, f.ws, f.caller, "request", pub.ObjectID)
	must(t, err)
	if req != again {
		t.Fatal("request replay changed history")
	}
	_, err = f.service.RequestPublication(f.ctx, f.ws, f.caller, "another-request", pub.ObjectID)
	if !errors.Is(err, d.ErrState) {
		t.Fatalf("repeat submit became provider success: %v", err)
	}
	p, err := f.service.Repo.PublicationHistory(f.ctx, f.ws, pub.ObjectID)
	must(t, err)
	if p.Status != d.Submitting || p.Revision != 1 || len(p.Transitions) != 2 || p.Transitions[1].ID != req.FactID {
		t.Fatalf("projection/history %#v", p)
	}
	var count int
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM external_publication WHERE product_release_id=$1`, f.release).Scan(&count))
	if count != 2 {
		t.Fatalf("multi provider count %d", count)
	}
	for _, event := range []string{"ExternalPublicationCreated", "ExternalPublicationRequested"} {
		var events, audits, evidences int
		must(t, f.pool.QueryRow(f.ctx, `SELECT (SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type=$2),(SELECT count(*) FROM audit_event WHERE object_id=$1 AND action=$2),(SELECT count(*) FROM evidence_relation r JOIN evidence e ON e.id=r.evidence_id WHERE r.object_id=$1 AND e.title=$2)`, pub.ObjectID, event).Scan(&events, &audits, &evidences))
		if events != 1 || audits != 1 || evidences != 1 {
			t.Fatalf("non-atomic/double history %s=%d,%d,%d", event, events, audits, evidences)
		}
	}
	if after := f.coreDigest(t); after != before {
		t.Fatal("external lifecycle changed Core release")
	}
}
func TestExternalIdentifierTypedReferencesAndAppendOnlyCorrection(t *testing.T) {
	f := setup(t)
	b := f.binding(t, "a")
	for _, tc := range []struct {
		kind    string
		id      uuid.UUID
		binding *uuid.UUID
	}{{"SUBJECT", f.actor, &b.ObjectID}, {"DATA_RESOURCE", f.resource, nil}, {"DATA_PRODUCT", f.product, nil}, {"PRODUCT_RELEASE", f.release, nil}} {
		s := d.IdentifierSpec{Provider: "a", ObjectType: tc.kind, ObjectID: tc.id, SubjectBindingID: tc.binding, ExternalID: "id:" + tc.kind, BasisEvidenceID: f.basis}
		rec, err := f.service.RecordIdentifier(f.ctx, f.ws, f.caller, "id-"+tc.kind, s)
		must(t, err)
		again, err := f.service.RecordIdentifier(f.ctx, f.ws, f.caller, "id-"+tc.kind, s)
		must(t, err)
		if rec != again {
			t.Fatal("identifier replay")
		}
		_, err = f.pool.Exec(f.ctx, `UPDATE external_identifier SET external_id='changed' WHERE id=$1`, rec.ObjectID)
		if err == nil {
			t.Fatal("identifier overwritten")
		}
		_, err = f.pool.Exec(f.ctx, `DELETE FROM external_identifier WHERE id=$1`, rec.ObjectID)
		if err == nil {
			t.Fatal("identifier deleted")
		}
		inv, err := f.service.InvalidateIdentifier(f.ctx, f.ws, f.caller, "invalidate-"+tc.kind, app.InvalidateSpec{ID: rec.ObjectID, Reason: "incorrect association"})
		must(t, err)
		current, err := f.service.Repo.Identifier(f.ctx, f.pool, f.ws, rec.ObjectID, false)
		must(t, err)
		if current.Status != "INVALIDATED" || current.ExternalID != s.ExternalID {
			t.Fatal("history rewritten")
		}
		_, err = f.pool.Exec(f.ctx, `UPDATE external_identifier_disposition SET reason='changed' WHERE id=$1`, inv.FactID)
		if err == nil {
			t.Fatal("disposition overwritten")
		}
		s.ExternalID += "-replacement"
		_, err = f.service.RecordIdentifier(f.ctx, f.ws, f.caller, "replacement-"+tc.kind, s)
		must(t, err)
	}
	_, err := f.pool.Exec(f.ctx, `UPDATE external_identity_binding SET external_subject_id='changed' WHERE id=$1`, b.ObjectID)
	if err == nil {
		t.Fatal("identity overwritten")
	}
	_, err = f.service.InvalidateIdentity(f.ctx, f.ws, f.caller, "invalidate-b", app.InvalidateSpec{ID: b.ObjectID, Reason: "incorrect mapping"})
	must(t, err)
	replacement, err := f.service.BindIdentity(f.ctx, f.ws, f.caller, "replacement-b", d.BindingSpec{Provider: "a", ExternalSubjectID: "subject:replacement", BasisEvidenceID: f.basis})
	must(t, err)
	if replacement.ObjectID == b.ObjectID {
		t.Fatal("binding overwritten")
	}
	_, err = f.service.BindIdentity(f.ctx, f.ws, f.caller, "reassign-old-id", d.BindingSpec{Provider: "a", ExternalSubjectID: "subject:a", BasisEvidenceID: f.basis})
	if !errors.Is(err, d.ErrConflict) {
		t.Fatalf("old external identity reassigned: %v", err)
	}
	other := setup(t)
	s := d.IdentifierSpec{Provider: "a", ObjectType: "PRODUCT_RELEASE", ObjectID: other.release, ExternalID: "foreign", BasisEvidenceID: f.basis}
	_, err = f.service.RecordIdentifier(f.ctx, f.ws, f.caller, "foreign-release", s)
	if !errors.Is(err, d.ErrNotFound) {
		t.Fatalf("foreign object: %v", err)
	}
	s.ObjectID = f.release
	s.BasisEvidenceID = other.basis
	_, err = f.service.RecordIdentifier(f.ctx, f.ws, f.caller, "foreign-evidence", s)
	if !errors.Is(err, d.ErrNotFound) {
		t.Fatalf("foreign evidence: %v", err)
	}
}
func TestExternalAtomicRollbackAndDatabaseBypassGuards(t *testing.T) {
	f := setup(t)
	b := f.binding(t, "a")
	pub := f.publication(t, b, "a")
	before := f.coreDigest(t)
	// Fault injection at outbox persistence: prior Evidence/Audit must roll back.
	router, err := outbox.NewRouter("reject-external", []outbox.Route{})
	must(t, err)
	outbox.ConfigureAppendObligation(router)
	_, err = f.service.RequestPublication(f.ctx, f.ws, f.caller, "outbox-failed", pub.ObjectID)
	good, _ := routing.NewRouter(false)
	outbox.ConfigureAppendObligation(good)
	if err == nil {
		t.Fatal("outbox failure committed")
	}
	var n int
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM evidence WHERE workspace_id=$1 AND title='ExternalPublicationRequested'`, f.ws).Scan(&n))
	if n != 0 {
		t.Fatal("evidence survived rollback")
	}
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM audit_event WHERE workspace_id=$1 AND action='ExternalPublicationRequested'`, f.ws).Scan(&n))
	if n != 0 {
		t.Fatal("audit survived rollback")
	}
	p, err := f.service.Repo.PublicationHistory(f.ctx, f.ws, pub.ObjectID)
	must(t, err)
	if p.Status != d.Draft || len(p.Transitions) != 1 {
		t.Fatal("projection survived rollback")
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE external_publication SET status='SUBMITTING',revision=1 WHERE id=$1`, pub.ObjectID)
	if err == nil {
		t.Fatal("projection committed without immutable transition")
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE external_publication SET status='PUBLISHED' WHERE id=$1`, pub.ObjectID)
	if err == nil {
		t.Fatal("fake provider publication accepted")
	}
	_, err = f.pool.Exec(f.ctx, `UPDATE external_publication SET product_release_id=$2 WHERE id=$1`, pub.ObjectID, f.draft)
	if err == nil {
		t.Fatal("release reference rewritten")
	}
	_, err = f.pool.Exec(f.ctx, `DELETE FROM external_publication_transition WHERE publication_id=$1`, pub.ObjectID)
	if err == nil {
		t.Fatal("history deleted")
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO external_publication(id,workspace_id,provider,identity_binding_id,product_release_id,status,revision,principal_ref,created_by,created_at) VALUES($1,$2,'a',$3,$4,'DRAFT',0,'synthetic',$5,now())`, uuid.New(), f.ws, b.ObjectID, f.draft, f.actor)
	if err == nil {
		t.Fatal("DB bypass accepted draft release")
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO external_identity_binding(id,workspace_id,provider,subject_type,subject_id,principal_ref,external_subject_id,basis_evidence_id,evidence_id,created_by,created_at) VALUES($1,$2,'other','PRINCIPAL',$3,'synthetic','foreign-basis',$4,$4,$3,now())`, uuid.New(), uuid.New(), f.actor, f.basis)
	if err == nil {
		t.Fatal("DB bypass accepted cross workspace evidence")
	}
	// Same key succeeds after full rollback; no poisoned receipt.
	_, err = f.service.RequestPublication(f.ctx, f.ws, f.caller, "outbox-failed", pub.ObjectID)
	must(t, err)
	if after := f.coreDigest(t); after != before {
		t.Fatal("failed external operations changed release")
	}
}

func TestExternalConcurrentNaturalBindingAndFreshReplayAuthority(t *testing.T) {
	f := setup(t)
	spec := d.BindingSpec{Provider: "a", ExternalSubjectID: "subject:concurrent", BasisEvidenceID: f.basis}
	errs := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.service.BindIdentity(f.ctx, f.ws, f.caller, fmt.Sprint("unique-key-", i), spec)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, d.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 7 {
		t.Fatalf("natural claim: success=%d conflict=%d", success, conflict)
	}
	var n int
	must(t, f.pool.QueryRow(f.ctx, `SELECT count(*) FROM evidence WHERE workspace_id=$1 AND title='ExternalIdentityBound'`, f.ws).Scan(&n))
	if n != 1 {
		t.Fatal("losing natural claims left evidence")
	}
	b := f.binding(t, "other")
	_, err := f.service.BindIdentity(f.ctx, f.ws, func(ctx context.Context) (principal.RequestPrincipal, error) {
		p, _ := f.caller(ctx)
		p.Capabilities = nil
		return p, nil
	}, "bind-other", d.BindingSpec{Provider: "other", ExternalSubjectID: "subject:other", BasisEvidenceID: f.basis})
	if !errors.Is(err, d.ErrDenied) {
		t.Fatalf("unauthorized replay: %v", err)
	}
	_, err = f.service.BindIdentity(f.ctx, f.ws, func(ctx context.Context) (principal.RequestPrincipal, error) {
		p, _ := f.caller(ctx)
		p.ActorID = uuid.New()
		return p, nil
	}, "bind-other", d.BindingSpec{Provider: "other", ExternalSubjectID: "subject:other", BasisEvidenceID: f.basis})
	if !errors.Is(err, d.ErrConflict) {
		t.Fatalf("actor-change replay: %v", err)
	}
	if b.ObjectID == uuid.Nil {
		t.Fatal("missing original binding")
	}
}
func TestExternalIdentityInvalidationAndSubmissionSerializeBothOrders(t *testing.T) {
	for _, invalidateFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(invalidateFirst), func(t *testing.T) {
			f := setup(t)
			b := f.binding(t, "a")
			pub := f.publication(t, b, "a")
			hold, err := f.pool.Begin(f.ctx)
			must(t, err)
			defer hold.Rollback(f.ctx)
			_, err = hold.Exec(f.ctx, `SELECT 1 FROM external_identity_binding WHERE id=$1 FOR UPDATE`, b.ObjectID)
			must(t, err)
			submitted, invalidated := make(chan error, 1), make(chan error, 1)
			submit := func() {
				_, e := f.service.RequestPublication(f.ctx, f.ws, f.caller, "concurrent-submit", pub.ObjectID)
				submitted <- e
			}
			invalidate := func() {
				_, e := f.service.InvalidateIdentity(f.ctx, f.ws, f.caller, "concurrent-invalidate", app.InvalidateSpec{ID: b.ObjectID, Reason: "incorrect identity"})
				invalidated <- e
			}
			if invalidateFirst {
				go invalidate()
			} else {
				go submit()
			}
			waitBindingWaiters(t, f, 1)
			if invalidateFirst {
				go submit()
			} else {
				go invalidate()
			}
			waitBindingWaiters(t, f, 2)
			must(t, hold.Commit(f.ctx))
			must(t, <-invalidated)
			err = <-submitted
			if invalidateFirst {
				if !errors.Is(err, d.ErrState) {
					t.Fatalf("late submit crossed invalidation: %v", err)
				}
			} else {
				must(t, err)
			}
			p, err := f.service.Repo.PublicationHistory(f.ctx, f.ws, pub.ObjectID)
			must(t, err)
			if invalidateFirst && p.Status != d.Draft {
				t.Fatal("invalidated-first changed projection")
			}
			if !invalidateFirst && p.Status != d.Submitting {
				t.Fatal("submitted-first history lost")
			}
		})
	}
}
func waitBindingWaiters(t *testing.T, f *fixture, want int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query LIKE '%FOR UPDATE OF b%'`, f.appName).Scan(&n)
		must(t, err)
		if n >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("missing observed parent-lock waiters: %d", want)
}
func TestExternalHTTPUsesTrustedPrincipalAndWorkspaceReadModel(t *testing.T) {
	f := setup(t)
	resolver, err := principal.NewStaticResolver(true, "test-token", "trusted/"+f.actor.String(), f.actor.String(), []string{f.ws.String()}, []string{d.Capability})
	must(t, err)
	h := transport.New(f.service, resolver)
	mux := http.NewServeMux()
	h.Register(mux)
	base := "/api/v1/workspaces/" + f.ws.String() + "/external-integration"
	call := func(method, path, token, key string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			must(t, json.NewEncoder(&buf).Encode(body))
		}
		req := httptest.NewRequest(method, path, &buf)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Idempotency-Key", key)
		req.Header.Set("X-Actor-ID", uuid.NewString())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	if w := call("GET", base+"/publications", "", "", nil); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
	if w := call("GET", strings.Replace(base, f.ws.String(), uuid.NewString(), 1)+"/publications", "test-token", "", nil); w.Code != 403 {
		t.Fatalf("foreign workspace: %d", w.Code)
	}
	forged := map[string]any{"provider": "a", "externalSubjectId": "fake", "basisEvidenceId": f.basis, "subjectId": uuid.NewString()}
	if w := call("POST", base+"/identity-bindings", "test-token", "fake", forged); w.Code != 400 {
		t.Fatalf("request minted subject: %d", w.Code)
	}
	w := call("POST", base+"/identity-bindings", "test-token", "http-bind", d.BindingSpec{Provider: "a", ExternalSubjectID: "subject:HTTP", BasisEvidenceID: f.basis})
	if w.Code != 201 {
		t.Fatalf("binding: %d %s", w.Code, w.Body)
	}
	var b d.Receipt
	must(t, json.Unmarshal(w.Body.Bytes(), &b))
	bound, err := f.service.Repo.Binding(f.ctx, f.pool, f.ws, b.ObjectID, false)
	must(t, err)
	if bound.SubjectID != f.actor {
		t.Fatal("X-Actor-ID trusted")
	}
	w = call("POST", base+"/publications", "test-token", "http-create", d.PublicationSpec{Provider: "a", IdentityBindingID: b.ObjectID, ProductReleaseID: f.release})
	if w.Code != 201 {
		t.Fatalf("publication: %d %s", w.Code, w.Body)
	}
	var pub d.Receipt
	must(t, json.Unmarshal(w.Body.Bytes(), &pub))
	w = call("POST", base+"/publications/"+pub.ObjectID.String()+"/request", "test-token", "http-request", map[string]any{})
	if w.Code != 200 {
		t.Fatalf("request: %d %s", w.Code, w.Body)
	}
	w = call("GET", base+"/publications/"+pub.ObjectID.String(), "test-token", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "SUBMITTING") {
		t.Fatalf("read: %d %s", w.Code, w.Body)
	}
	if w := call("GET", base+"/publications?limit=100000", "test-token", "", nil); w.Code != 400 {
		t.Fatalf("unbounded query: %d", w.Code)
	}
	if w := call("GET", base+"/publications/"+uuid.NewString(), "test-token", "", nil); w.Code != 404 {
		t.Fatalf("missing publication: %d", w.Code)
	}
	if w := call("PATCH", base+"/publications/"+pub.ObjectID.String(), "test-token", "patch", map[string]any{"status": "PUBLISHED"}); w.Code != 405 {
		t.Fatalf("generic patch exists: %d", w.Code)
	}
	// A configured principal with no external capability cannot query or mutate.
	denied, err := principal.NewStaticResolver(true, "test-token", "trusted", f.actor.String(), []string{f.ws.String()}, []string{"HUMAN_DECISION"})
	must(t, err)
	h.Resolver = denied
	if w := call("GET", base+"/publications", "test-token", "", nil); w.Code != 403 {
		t.Fatalf("missing capability: %d", w.Code)
	}
	h.Resolver = nil
	if w := call("GET", base+"/publications", "test-token", "", nil); w.Code != 503 {
		t.Fatalf("unconfigured resolver: %d", w.Code)
	}
}
func TestExternalMigrationEmptyRoundTripAndHistoricalDownRefused(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	must(t, err)
	defer admin.Close()
	name := "external_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()+` TEMPLATE template0`)
	must(t, err)
	defer admin.Exec(ctx, `DROP DATABASE `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
	cfg, err := pgxpool.ParseConfig(dsn)
	must(t, err)
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	must(t, err)
	defer pool.Close()
	_, source, _, _ := runtime.Caller(0)
	dir := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../../../migrations"))
	// Pin this guard test to its own migration so later increments cannot make
	// Down exercise another version and silently stop proving #84's invariant.
	pinned := t.TempDir()
	entries, err := os.ReadDir(dir)
	must(t, err)
	for _, entry := range entries {
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok || entry.IsDir() {
			continue
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version > 55 {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		must(t, err)
		must(t, os.WriteFile(filepath.Join(pinned, entry.Name()), content, 0600))
	}
	runner := migration.NewRunner(pool, pinned)
	must(t, runner.Up(ctx))
	must(t, runner.Down(ctx))
	must(t, runner.Up(ctx))
	ws, actor, basis := uuid.New(), uuid.New(), uuid.New()
	tx, err := pool.Begin(ctx)
	must(t, err)
	_, err = evidence.Append(ctx, tx, evidence.Record{ID: basis, WorkspaceID: ws, EvidenceType: "TEST_BASIS"})
	must(t, err)
	must(t, tx.Commit(ctx))
	writer, err := pool.Begin(ctx)
	must(t, err)
	defer writer.Rollback(ctx)
	_, err = writer.Exec(ctx, `INSERT INTO external_identity_binding(id,workspace_id,provider,subject_type,subject_id,principal_ref,external_subject_id,basis_evidence_id,evidence_id,created_by,created_at) VALUES($1,$2,'a','PRINCIPAL',$3,'trusted','subject:1',$4,$4,$3,now())`, uuid.New(), ws, actor, basis)
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- runner.Down(ctx) }()
	deadline := time.Now().Add(8 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		var n int
		must(t, admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND wait_event_type='Lock' AND query LIKE '%LOCK TABLE external_identity_binding%'`, name).Scan(&n))
		if n > 0 {
			waiting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("down did not wait for concurrent history writer")
	}
	must(t, writer.Commit(ctx))
	if err := <-done; err == nil {
		t.Fatal("destructive historical down succeeded")
	}
	var n int
	must(t, pool.QueryRow(ctx, `SELECT count(*) FROM external_identity_binding`).Scan(&n))
	if n != 1 {
		t.Fatal("down destroyed history")
	}
}
