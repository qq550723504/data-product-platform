// One-shot closed synthetic reader acceptance. No DELETE, COPY, hold release,
// arbitrary locator, production delivery or externally published HTTP route.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

var namespaces = []string{"dpp-lifecycle307-online", "dpp-lifecycle307-quarantine"}

type config struct {
	RunID, WorkspaceID, ProviderInstanceID, IncarnationID, ActorID, Epoch                                       uuid.UUID
	Enabled                                                                                                     bool
	Plane                                                                                                       bool
	ReferenceDemand                                                                                             bool
	StartedAt, ExpiresAt                                                                                        time.Time
	Endpoint, PostgresDSN, Token, Access, Secret, InspectAccess, InspectSecret, OutputDirectory, ManifestSHA256 string
	RootAccess, RootSecret                                                                                      string // populated only in a separate admin-only mount
	ProducerAccess, ProducerSecret                                                                              string
	RedisAddr                                                                                                   string // fresh LC-13 closed queue only
	LC13Continuation                                                                                            bool
	OwnerStopPublicKey                                                                                          string
	OwnerStopIdentity                                                                                           pilot.ProcessingOwnerIdentity
	IssuerAccess, IssuerSecret                                                                                  string
	CredentialStopPublicKey                                                                                     string
	CredentialStopIdentity                                                                                      pilot.CredentialOwnerIdentity
}
type manifest struct {
	Objects []struct {
		CopyID           uuid.UUID `json:"copy_id"`
		DatasetVersionID uuid.UUID `json:"dataset_version_id"`
		Namespace        string    `json:"namespace"`
		Key              string    `json:"key"`
		Version          string    `json:"version_id"`
		SHA              string    `json:"sha256"`
		Bytes            int64     `json:"bytes"`
	} `json:"objects"`
	Plan struct {
		ID uuid.UUID `json:"id"`
	} `json:"plan"`
}

func load() (config, error) {
	var c config
	b, e := os.ReadFile("/run/private/reader.json")
	if e != nil || len(b) > 128<<10 {
		return c, errors.New("reader configuration unavailable")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return c, errors.New("invalid reader configuration")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, errors.New("trailing configuration")
	}
	at := time.Now().UTC()
	if !c.Enabled || c.RunID == uuid.Nil || c.WorkspaceID == uuid.Nil || c.ProviderInstanceID == uuid.Nil || c.IncarnationID == uuid.Nil || c.ActorID == uuid.Nil || c.Epoch == uuid.Nil || c.Endpoint != "127.0.0.1:9000" || c.PostgresDSN == "" || c.Token == "" || c.Access == "" || c.Secret == "" || !filepath.IsAbs(c.OutputDirectory) || len(c.ManifestSHA256) != 64 || at.Before(c.StartedAt) || !at.Before(c.ExpiresAt) || c.ExpiresAt.Sub(c.StartedAt) > time.Hour {
		return c, errors.New("reader authority unavailable")
	}
	return c, nil
}
func write(c config, name string, v any) error {
	f, e := os.OpenFile(filepath.Join(c.OutputDirectory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = json.NewEncoder(f).Encode(v); e != nil {
		return e
	}
	return f.Sync()
}
func copies(c config) (manifest, []domain.Copy, error) {
	var m manifest
	b, e := os.ReadFile("/run/private/manifest.json")
	sum := sha256.Sum256(b)
	if e != nil || hex.EncodeToString(sum[:]) != c.ManifestSHA256 || json.Unmarshal(b, &m) != nil || len(m.Objects) != 4 || m.Plan.ID == uuid.Nil {
		return m, nil, lc.ErrStorageScope
	}
	cs := []domain.Copy{}
	seen := map[string]bool{}
	for _, o := range m.Objects {
		cp := domain.Copy{ID: o.CopyID, WorkspaceID: c.WorkspaceID, DatasetVersionID: o.DatasetVersionID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespace: o.Namespace, Key: o.Key, ProviderVersion: o.Version, ContentSHA256: o.SHA, ByteSize: o.Bytes}
		slot := cp.Namespace + "/" + cp.Key
		if cp.Validate() != nil || cp.ByteSize > 128<<10 || (cp.Namespace != namespaces[0] && cp.Namespace != namespaces[1]) || (cp.Key != "fixture/a.csv" && cp.Key != "fixture/b.csv") || seen[slot] {
			return m, nil, lc.ErrStorageScope
		}
		seen[slot] = true
		cs = append(cs, cp)
	}
	if cs[0].Namespace != namespaces[0] || cs[0].Key != "fixture/a.csv" {
		return m, nil, lc.ErrStorageScope
	}
	return m, cs, nil
}

type authority struct{}

func (authority) Resolve(r *http.Request, w uuid.UUID, cap string) (principal.RequestPrincipal, error) {
	c, e := load()
	if e != nil {
		return principal.RequestPrincipal{}, principal.ErrUnauthenticated
	}
	caps := []string{lc.CapabilityAdmitReader, lc.CapabilityTerminateReader, lc.CapabilityInspectOccupancy, lc.CapabilityInspectStorage, lc.CapabilityRestrictUse, lc.CapabilityCopyCustody}
	if c.Plane && c.ReferenceDemand {
		caps = append(caps, lc.CapabilityPreparePlan, lc.CapabilityApproveDisposition, lc.CapabilityDispatch)
		for _, module := range []string{"dataset", "entity", "workflow", "quality", "certification", "delivery"} {
			caps = append(caps, lc.ReferenceOwnerCapability(module))
		}
	}
	if c.LC13Continuation {
		caps = append(caps, lc.ReferenceOwnerCapability("delivery"), lc.CapabilityCheckpointStorage)
	}
	resolver, e := principal.NewStaticResolver(true, c.Token, "lifecycle-fixture:"+c.RunID.String(), c.ActorID.String(), []string{c.WorkspaceID.String()}, caps)
	if e != nil {
		return principal.RequestPrincipal{}, e
	}
	return resolver.Resolve(r, w, cap)
}
func (a authority) AuthorizeLifecycle(_ context.Context, _ pgx.Tx, w uuid.UUID, cap string, _ time.Time) (principal.RequestPrincipal, error) {
	c, e := load()
	if e != nil {
		return principal.RequestPrincipal{}, e
	}
	r, _ := http.NewRequest("POST", "http://fixture.invalid/internal", nil)
	r.Header.Set("Authorization", "Bearer "+c.Token)
	return a.Resolve(r, w, cap)
}

type gate struct{ s *lc.CopyService }

func (g *gate) EvaluateFixtureUse(ctx context.Context, tx pgx.Tx, r lc.UseGateRequest) (domain.UseDecision, error) {
	if g.s == nil {
		return domain.UseDecision{}, domain.ErrUseBlocked
	}
	return g.s.EvaluateFixtureUse(ctx, tx, r)
}

type noCopyProtection struct{}

func (noCopyProtection) CurrentCopyProtection(context.Context, pgx.Tx, domain.Copy, time.Time) (lc.CopyProtection, error) {
	return lc.CopyProtection{}, lc.ErrStorageUnsupported
}

// Each administrative call has a one-send owner; no hidden SDK resend. This
// provisioning helper is never mounted into the reader's credential directory.
type adminKey struct{}
type adminCall struct {
	mu   sync.Mutex
	sent bool
}
type adminTransport struct {
	base http.RoundTripper
	c    config
}

func (t adminTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	s, ok := r.Context().Value(adminKey{}).(*adminCall)
	if !ok || r.URL.Scheme != "http" || r.URL.Host != t.c.Endpoint || r.URL.User != nil || (r.Method != "PUT" && r.Method != "POST" && !(t.c.LC13Continuation && r.Method == "GET" && r.URL.Path == "/minio/admin/v3/info-service-account")) || len(r.URL.Path) < 16 || r.URL.Path[:16] != "/minio/admin/v3/" {
		return nil, errors.New("admin scope denied")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent {
		return nil, errors.New("admin resend denied")
	}
	s.sent = true
	id := uuid.New()
	name := "reader-admin-" + id.String() + ".json"
	if e := write(t.c, name, map[string]any{"id": id, "fact": "START", "method": r.Method, "path": r.URL.Path, "at": time.Now().UTC()}); e != nil {
		return nil, e
	}
	resp, e := t.base.RoundTrip(r)
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	failure := ""
	if e != nil {
		failure = "TRANSPORT_UNKNOWN"
	}
	if j := write(t.c, "reader-admin-observation-"+id.String()+".json", map[string]any{"id": id, "http_status": status, "failure": failure, "at": time.Now().UTC()}); j != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, errors.New("admin observation unavailable")
	}
	return resp, e
}
func provision(ctx context.Context, c config, cs []domain.Copy, disable bool) error {
	if c.RootAccess == "" || c.RootSecret == "" {
		return errors.New("separate admin mount required")
	}
	a, e := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if e != nil {
		return e
	}
	a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, c})
	call := func(fn func(context.Context) error) error {
		c, can := context.WithTimeout(context.WithValue(ctx, adminKey{}, &adminCall{}), 15*time.Second)
		defer can()
		return fn(c)
	}
	if disable {
		return call(func(ctx context.Context) error { return a.SetUserStatus(ctx, c.Access, madmin.AccountDisabled) })
	}
	statements := []any{}
	for _, cp := range cs {
		statements = append(statements, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + cp.Namespace + "/" + cp.Key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": cp.ProviderVersion}}})
	}
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
	name := "lifecycle307-reader-" + c.Access
	if e = call(func(ctx context.Context) error { return a.AddCannedPolicy(ctx, name, policy) }); e != nil {
		return errors.New("reader policy unknown; no retry")
	}
	if e = call(func(ctx context.Context) error { return a.AddUser(ctx, c.Access, c.Secret) }); e != nil {
		return errors.New("reader identity unknown; no retry")
	}
	if e = call(func(ctx context.Context) error {
		_, e := a.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{name}, User: c.Access})
		return e
	}); e != nil {
		return errors.New("reader attachment unknown; no retry")
	}
	return write(c, "reader-iam-policy.json", json.RawMessage(policy))
}

type port struct {
	inner lc.PinnedReaderPort
	c     config
	crash bool
}

func (p *port) ReadExactPinned(ctx context.Context, c domain.Copy, pin domain.ReaderPin) (lc.PinnedReadResult, error) {
	r, e := p.inner.ReadExactPinned(ctx, c, pin)
	if e == nil && p.crash {
		if write(p.c, "reader-crash.json", map[string]any{"pin": pin, "call_id": r.CallID, "native_stopped_at": r.StoppedAt, "crash_before_output_and_termination": true}) != nil {
			panic("crash evidence unavailable")
		}
		os.Exit(95)
	}
	return r, e
}

type slowWriter struct {
	http.ResponseWriter
	entered, release chan struct{}
	ctx              context.Context
}

func (w slowWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w slowWriter) Write(b []byte) (int, error) {
	close(w.entered)
	<-w.release
	if w.ctx.Err() != nil {
		return 0, io.ErrClosedPipe
	}
	return w.ResponseWriter.Write(b)
}
func occupancy(ctx context.Context, o *lc.OccupancyService, c config, m manifest, cp domain.Copy) (domain.Occupancy, error) {
	r, e := o.InspectPlanOccupancy(ctx, c.WorkspaceID, m.Plan.ID)
	if e != nil {
		return domain.Occupancy{}, e
	}
	for _, x := range r.Copies {
		if x.CopyID == cp.ID {
			return x, nil
		}
	}
	return domain.Occupancy{}, lc.ErrStorageScope
}
func count(ctx context.Context, p *pgxpool.Pool, q string) (int, error) {
	var n int
	e := p.QueryRow(ctx, q).Scan(&n)
	return n, e
}
func accept(ctx context.Context, c config, m manifest, cs []domain.Copy, pool *pgxpool.Pool, o *lc.OccupancyService, s *lc.FixtureReaderService) error {
	entered, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	h := transport.ReaderHandler{Service: s, Resolver: authority{}, WorkspaceID: c.WorkspaceID}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return e
	}
	srv := &http.Server{ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") == "native-slow-cancel" {
			defer close(stopped)
			h.ServeHTTP(slowWriter{w, entered, release, r.Context()}, r)
		} else {
			h.ServeHTTP(w, r)
		}
	})}
	defer srv.Close()
	go srv.Serve(listener)
	url := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 10 * time.Second}
	body := func(cp uuid.UUID, reader uuid.UUID) string {
		return fmt.Sprintf(`{"copy_id":%q,"reader_id":%q}`, cp.String(), reader.String())
	}
	request := func(ctx context.Context, path, key, token, body string) (int, []byte, error) {
		r, e := http.NewRequestWithContext(ctx, "POST", url+path, bytes.NewBufferString(body))
		if e != nil {
			return 0, nil, e
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Idempotency-Key", key)
		resp, e := client.Do(r)
		if e != nil {
			return 0, nil, e
		}
		defer resp.Body.Close()
		b, e := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
		return resp.StatusCode, b, e
	}
	readerID := uuid.New()
	goodBody := body(cs[0].ID, readerID)
	status, b, e := request(ctx, "/fixture/read", "native-complete", c.Token, goodBody)
	sum := sha256.Sum256(b)
	if e != nil || status != 200 || int64(len(b)) != cs[0].ByteSize || hex.EncodeToString(sum[:]) != cs[0].ContentSHA256 {
		return errors.New("native HTTP bytes failed")
	}
	before, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation='READ_PINNED'`)
	if e != nil || before != 2 {
		return errors.New("crash and complete native calls mismatch")
	}
	tests := []struct {
		name, path, key, token, body string
		status                       int
	}{
		{"replay", "/fixture/read", "native-complete", c.Token, goodBody, 409},
		{"crashed-replay", "/fixture/read", "native-crash", c.Token, "", 409},
		{"unauthenticated", "/fixture/read", "bad-auth", "wrong", goodBody, 403},
		{"foreign-copy", "/fixture/read", "foreign-copy", c.Token, body(uuid.New(), uuid.New()), 403},
		{"restricted-B", "/fixture/read", "restricted-b", c.Token, body(cs[1].ID, uuid.New()), 403},
		{"arbitrary-locator", "/fixture/read", "untrusted-locator", c.Token, goodBody[:len(goodBody)-1] + `,"uri":"s3://foreign/key"}`, 400},
		{"legacy-download", "/storage/download", "legacy", c.Token, goodBody, 404},
		{"production-delivery", "/api/v1/datasets/deliver", "delivery", c.Token, goodBody, 404},
	}
	var crash struct {
		Pin domain.ReaderPin `json:"pin"`
	}
	raw, e := os.ReadFile(filepath.Join(c.OutputDirectory, "reader-crash.json"))
	if e != nil || json.Unmarshal(raw, &crash) != nil {
		return errors.New("crash pin receipt unavailable")
	}
	tests[1].body = body(cs[0].ID, crash.Pin.ReaderID)
	outcomes := map[string]int{}
	for _, t := range tests {
		status, b, e := request(ctx, t.path, t.key, t.token, t.body)
		if e != nil || status != t.status || bytes.Contains(b, []byte("alpha")) {
			return errors.New("closed reader rejection failed: " + t.name)
		}
		outcomes[t.name] = status
	}
	after, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation='READ_PINNED'`)
	if e != nil || after != before {
		return errors.New("denied/replay opened native I/O")
	}
	slowCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = request(slowCtx, "/fixture/read", "native-slow-cancel", c.Token, body(cs[0].ID, uuid.New()))
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	active, e := occupancy(ctx, o, c, m, cs[0])
	if e != nil || active.ActiveReaders != 2 {
		return errors.New("slow native read missing durable pin")
	}
	if e = write(c, "reader-output-active.json", active); e != nil {
		return e
	}
	restriction, e := o.RestrictContentUse(ctx, lc.RestrictContentUseCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "native-reader-current-restriction"}, DatasetVersionID: cs[0].DatasetVersionID, BasisRef: "controlled-native-restriction-during-active-reader"})
	if e != nil {
		return e
	}
	status, b, e = request(ctx, "/fixture/read", "restricted-new-read", c.Token, body(cs[0].ID, uuid.New()))
	if e != nil || status != 403 || bytes.Contains(b, []byte("alpha")) {
		return errors.New("current restriction allowed new bytes")
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	active, e = occupancy(ctx, o, c, m, cs[0])
	if e != nil || active.ActiveReaders != 2 {
		return errors.New("cancellation released live output owner")
	}
	if e = write(c, "reader-cancel-before-stop.json", active); e != nil {
		return e
	}
	close(release)
	select {
	case <-stopped:
	case <-ctx.Done():
		return ctx.Err()
	}
	final, e := occupancy(ctx, o, c, m, cs[0])
	if e != nil || final.ActiveReaders != 1 {
		return errors.New("confirmed stop failed to drain only owned pin")
	}
	calls, e := count(ctx, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE operation='READ_PINNED'`)
	if e != nil || calls != 3 {
		return errors.New("read physical attempts mismatch")
	}
	var costs, observations, term int
	if e = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM lifecycle_storage_observation x JOIN lifecycle_storage_call c ON c.id=x.call_id WHERE c.operation='READ_PINNED' AND x.http_status=200 AND x.failure='' AND x.invoked_at IS NOT NULL),(SELECT count(*) FROM cost_allocation a JOIN lifecycle_storage_call c ON c.id=a.lifecycle_storage_call_id WHERE c.operation='READ_PINNED'),(SELECT count(*) FROM lifecycle_reader_termination)`).Scan(&observations, &costs, &term); e != nil || observations != 3 || costs != 3 || term != 2 {
		return errors.New("native read observation/cost/stop facts mismatch")
	}
	return write(c, "reader-acceptance.json", map[string]any{"run_id": c.RunID, "workspace_id": c.WorkspaceID, "current_restriction": restriction, "active_after_confirmed_stop": final, "native_calls": calls, "native_observations": observations, "native_cost_allocations": costs, "terminations": term, "rejections": outcomes, "native_delete": 0, "copy": 0, "cleanup": 0, "acceptance": map[string]string{"durable_pin_before_exact_GET": "PASS", "native_HTTP_bytes": "PASS", "closed_HTTP_auth_and_routes": "PASS", "cancel_is_not_quiescence": "PASS", "crashed_owner_pin_retained": "PASS", "replay_no_payload_or_IO": "PASS", "restriction_new_use_blocked": "PASS", "full_D1": "NOT_RUN", "production_delivery_LC12": "NOT_RUN", "processing_integration": "NOT_RUN"}})
}
func execute(ctx context.Context, mode string) error {
	if strings.HasPrefix(mode, "plane-") {
		return planeStep(ctx, strings.TrimPrefix(mode, "plane-"))
	}
	c, e := load()
	if e != nil {
		return e
	}
	m, cs, e := copies(c)
	if e != nil {
		return e
	}
	if mode == "provision" || mode == "seal" {
		return provision(ctx, c, cs, mode == "seal")
	}
	if c.RootAccess != "" || c.RootSecret != "" {
		return errors.New("reader must not have root credentials")
	}
	pool, e := pgxpool.New(ctx, c.PostgresDSN)
	if e != nil {
		return errors.New("database unavailable")
	}
	defer pool.Close()
	var schema int
	if pool.QueryRow(ctx, `SELECT max(version) FROM schema_migration`).Scan(&schema) != nil || schema != 66 {
		return errors.New("new isolated schema66 required")
	}
	router, e := routing.NewRouter(false)
	if e != nil {
		return e
	}
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	g := &gate{}
	o := lc.NewOccupancyService(transaction.NewManager(pool), &infrastructure.PostgresRepository{}, authority{}, g, nil)
	var ns uuid.UUID
	if e = pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND namespace=$2`, c.WorkspaceID, namespaces[1]).Scan(&ns); e != nil {
		return e
	}
	g.s = lc.NewCopyService(o, []lc.CopyTargetSlot{{Slot: "reader-unused-copy", Action: domain.ArchiveCopy, Namespace: namespaces[1], Key: "archive/a.csv", NamespaceID: ns}}, noCopyProtection{}, nil, c.Epoch)
	if mode == "bind" {
		r, e := g.s.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "native-reader-policy"}, DatasetVersionID: cs[0].DatasetVersionID, UseUntil: c.ExpiresAt, RestoreUntil: c.ExpiresAt})
		if e != nil {
			return e
		}
		return write(c, "reader-policy.json", r)
	}
	if mode == "final" {
		ins, e := storage.NewLifecycleInspector(storage.LifecycleInspectionConfig{Endpoint: c.Endpoint, AccessKey: c.InspectAccess, SecretKey: c.InspectSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.NewPostgresStorageCallJournal(o)})
		if e != nil {
			return e
		}
		objects := []any{}
		for _, cp := range cs {
			stat, e := ins.InspectExactCopy(ctx, cp)
			if e != nil {
				return e
			}
			pr, e := ins.InspectRetentionAndHold(ctx, cp)
			if e != nil {
				return e
			}
			objects = append(objects, map[string]any{"copy": cp, "stat_call_id": stat.CallID, "protection": pr})
		}
		inv := []lc.NamespaceInventory{}
		for _, cp := range []domain.Copy{cs[0], cs[2]} {
			r, e := ins.InspectNamespace(ctx, cp)
			if e != nil {
				return e
			}
			inv = append(inv, r)
		}
		return write(c, "reader-originals-final.json", map[string]any{"objects": objects, "inventories": inv, "delete": 0, "cleanup": 0})
	}
	native, e := storage.NewLifecycleReader(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: c.Access, SecretKey: c.Secret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.PinnedReaderCallJournal{Service: o, Observations: lc.NewPostgresStorageCallJournal(o)}}, cs)
	if e != nil {
		return e
	}
	s := lc.NewFixtureReaderService(o, &port{native, c, mode == "crash"})
	if s == nil {
		return errors.New("reader composition unavailable")
	}
	if mode == "crash" {
		p, e := authority{}.AuthorizeLifecycle(ctx, nil, c.WorkspaceID, lc.CapabilityAdmitReader, time.Now())
		if e != nil {
			return e
		}
		_, e = s.ReadTo(lc.WithFixtureReaderCaller(ctx, p), lc.AdmitFixtureReaderCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "native-crash"}, CopyID: cs[0].ID, ReaderID: uuid.New()}, io.Discard)
		return e
	}
	if mode == "accept" {
		return accept(ctx, c, m, cs, pool, o, s)
	}
	return errors.New("unsupported reader mode")
}
func main() {
	mode := flag.String("mode", "", "bounded reader step")
	flag.Parse()
	if *mode == "post-v22-verify-native" {
		ctx, cancel := context.WithTimeout(context.Background(), 13*time.Minute)
		defer cancel()
		if err := postV22Verify(ctx); err != nil {
			fmt.Println("Post-v22 readonly verification failed:", err, "; DELETE=0; resources retained")
			os.Exit(1)
		}
		fmt.Println("Post-v22 readonly verification complete; DELETE=0")
		return
	}
	if strings.HasPrefix(*mode, "remaining-") {
		ctx, cancel := context.WithTimeout(context.Background(), 13*time.Minute)
		defer cancel()
		if err := remainingStep(ctx, *mode); err != nil {
			fmt.Println("Remaining step failed:", err, "; all resources retained; DELETE=0")
			os.Exit(1)
		}
		fmt.Println("Remaining step complete; DELETE=0, cleanup=0")
		return
	}
	limit := 90 * time.Second
	if *mode == "plane-reference-races" {
		limit = 13 * time.Minute
	}
	if *mode == "plane-lc13-orphan" {
		limit = 3 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	if e := execute(ctx, *mode); e != nil {
		fmt.Println("Reader step failed:", e, "; all resources retained, no cleanup or automatic retry")
		os.Exit(1)
	}
	fmt.Println("Reader step complete; DELETE=0, cleanup=0")
}
