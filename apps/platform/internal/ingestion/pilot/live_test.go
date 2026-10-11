package pilot_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	adapter "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/hop"
	ingestioninfra "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/pilot"
	transport "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/transport/http"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/httpserver"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type config struct {
	DSN, Private, Output, Migrations, ControlURL, ControlToken, RootKey, RootSecret string
	Profile                                                                         pilot.Profile
}
type live struct {
	t                  *testing.T
	ctx                context.Context
	cfg                config
	pool               *pgxpool.Pool
	profilePath, token string
	ws                 uuid.UUID
	server             *httptest.Server
	handler            *transport.Handler
	checks             map[string]string
	receipts           []d.Receipt
	manifests          []d.Manifest
	storageFault       atomic.Value
}

func (l *live) saveProfile(p pilot.Profile) {
	l.t.Helper()
	b, _ := json.Marshal(p)
	if os.WriteFile(l.profilePath, b, 0600) != nil {
		l.t.Fatal("write private profile")
	}
}
func (l *live) call(method, path string, body any, want int) []byte {
	l.t.Helper()
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, l.server.URL+path, bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+l.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		l.t.Fatal("Core HTTP transport:", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		l.t.Fatalf("%s %s status=%d want=%d response=%s", method, path, res.StatusCode, want, raw)
	}
	return raw
}
func (l *live) command(op d.Operation, command string, body any, want int) []byte {
	return l.call("POST", "/api/v1/ingestions/"+op.ID.String()+"/"+command, body, want)
}
func (l *live) fault(name string) {
	l.t.Helper()
	req, _ := http.NewRequest("POST", l.cfg.ControlURL+"/"+name, nil)
	req.Header.Set("Authorization", "Bearer "+l.cfg.ControlToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		l.t.Fatal("fault control unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 204 {
		l.t.Fatal("fault control rejected")
	}
}
func (l *live) count(sql string, args ...any) int {
	l.t.Helper()
	var n int
	if err := l.pool.QueryRow(l.ctx, sql, args...).Scan(&n); err != nil {
		l.t.Fatal("independent Core query:", err)
	}
	return n
}
func (l *live) empty(op d.Operation) {
	l.t.Helper()
	if l.count(`SELECT count(*) FROM dataset_version WHERE storage_uri LIKE $1`, "s3://raw/core/"+l.ws.String()+"/"+op.ID.String()+"/%") != 0 || l.count(`SELECT count(*) FROM ingestion_receipt WHERE operation_id=$1`, op.ID) != 0 {
		l.t.Fatal("partial RAW admission escaped")
	}
}
func (l *live) prepare() d.Operation {
	l.t.Helper()
	targets := []transport.Target{}
	for _, binding := range l.cfg.Profile.Targets {
		targets = append(targets, transport.Target{Code: binding.Code, DatasetID: binding.DatasetID, ResourceID: binding.ResourceID})
	}
	body := map[string]any{"workspaceId": l.ws, "requestKey": "r-" + uuid.NewString(), "batchKey": "b-" + uuid.NewString(), "targets": targets}
	var op d.Operation
	_ = json.Unmarshal(l.call("POST", "/api/v1/ingestions", body, 200), &op)
	var replay d.Operation
	_ = json.Unmarshal(l.call("POST", "/api/v1/ingestions", body, 200), &replay)
	if op.ID == uuid.Nil || op.ID != replay.ID || op.AttemptID != replay.AttemptID {
		l.t.Fatal("Prepare replay changed operation/attempt")
	}
	body["batchKey"] = "conflicting-batch"
	l.call("POST", "/api/v1/ingestions", body, 409)
	return op
}
func (l *live) completed() d.Operation {
	op := l.prepare()
	_ = json.Unmarshal(l.command(op, "dispatch", nil, 200), &op)
	return l.finish(op)
}
func (l *live) finish(op d.Operation) d.Operation {
	l.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("POST", l.server.URL+"/api/v1/ingestions/"+op.ID.String()+"/recover", nil)
		req.Header.Set("Authorization", "Bearer "+l.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			l.t.Fatal("recover transport")
		}
		raw, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode == 200 {
			_ = json.Unmarshal(raw, &op)
		} else if res.StatusCode == 503 {
			_ = json.Unmarshal(l.call("GET", "/api/v1/ingestions/"+op.ID.String(), nil, 200), &op)
		} else {
			l.t.Fatalf("recover rejected: %d %s", res.StatusCode, raw)
		}
		if op.State == "SUCCEEDED" || op.State == "FAILED" {
			return op
		}
		time.Sleep(500 * time.Millisecond)
	}
	l.t.Fatal("Hop did not reach observed terminal outcome")
	return op
}
func (l *live) accepted(op d.Operation) d.Receipt {
	l.t.Helper()
	if op.State != "SUCCEEDED" || op.Manifest == nil {
		l.t.Fatal("no successful manifest")
	}
	var rec d.Receipt
	_ = json.Unmarshal(l.command(op, "admit", op.Manifest, 200), &rec)
	if len(rec.Bindings) != 3 || rec.ID == uuid.Nil {
		l.t.Fatal("incomplete receipt")
	}
	var again d.Receipt
	_ = json.Unmarshal(l.command(op, "admit", op.Manifest, 200), &again)
	a, _ := json.Marshal(rec)
	b, _ := json.Marshal(again)
	if !bytes.Equal(a, b) {
		l.t.Fatal("Admit replay changed receipt")
	}
	if l.count(`SELECT count(*) FROM ingestion_receipt WHERE operation_id=$1`, op.ID) != 1 || l.count(`SELECT count(*) FROM dataset_version v JOIN ingestion_receipt_slot s ON s.version_id=v.id WHERE s.receipt_id=$1 AND v.status='READY'`, rec.ID) != 3 {
		l.t.Fatal("receipt is not exactly three READY versions")
	}
	if l.count(`SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='IngestionAccepted'`, op.ID) != 1 || l.count(`SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='IngestionAccepted'`, op.ID) != 1 || l.count(`SELECT count(*) FROM evidence_relation er JOIN ingestion_receipt h ON h.evidence_id=er.evidence_id WHERE h.id=$1 AND er.object_type='DATASET_VERSION'`, rec.ID) != 3 {
		l.t.Fatal("missing/duplicate transactional event, audit or evidence")
	}
	for _, binding := range rec.Bindings {
		r, err := l.handler.Service.Store.Get(l.ctx, binding.URI)
		if err != nil {
			l.t.Fatal("Core object unavailable")
		}
		got, _ := io.ReadAll(r)
		_ = r.Close()
		original, err := os.ReadFile(filepath.Join(l.cfg.Profile.LocalRoot, op.AttemptID.String(), binding.Code+".csv"))
		if err != nil || !bytes.Equal(got, original) || d.Hash(got) != binding.SHA256 {
			l.t.Fatal("Core changed original Hop bytes")
		}
	}
	l.receipts = append(l.receipts, rec)
	l.manifests = append(l.manifests, *op.Manifest)
	return rec
}
func clone(m d.Manifest) d.Manifest {
	b, _ := json.Marshal(m)
	var result d.Manifest
	_ = json.Unmarshal(b, &result)
	return result
}
func TestLiveCoreBatch(t *testing.T) {
	path := os.Getenv("TEST_INGESTION_B2_CONFIG")
	if path == "" {
		t.Skip("TEST_INGESTION_B2_CONFIG is not set")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read private config")
	}
	var cfg config
	if json.Unmarshal(data, &cfg) != nil {
		t.Fatal("decode private config")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		t.Fatal("Core config invalid")
	}
	defer pool.Close()
	if err = migration.NewRunner(pool, cfg.Migrations).Up(ctx); err != nil {
		t.Fatal("migrate isolated Core:", err)
	}
	router, _ := routing.NewRouter(false)
	outbox.ConfigureAppendObligation(router)
	defer outbox.ConfigureAppendObligation(nil)
	l := &live{t: t, ctx: ctx, cfg: cfg, pool: pool, ws: uuid.New(), profilePath: filepath.Join(cfg.Private, "profile.json"), checks: map[string]string{}}
	p := cfg.Profile
	p.ActorID = uuid.NewString()
	p.Workspaces = []string{l.ws.String()}
	p.Capabilities = []string{"INGEST_RAW"}
	root, err := storage.New(p.StorageEndpoint, cfg.RootKey, cfg.RootSecret, p.Bucket, false)
	if err != nil || root.EnsureBucket(ctx) != nil {
		t.Fatal("bootstrap storage")
	}
	scoped := func(statements []any) pilot.Identity {
		policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": statements})
		c, err := credentials.NewSTSAssumeRole("http://"+p.StorageEndpoint, credentials.STSAssumeRoleOptions{AccessKey: cfg.RootKey, SecretKey: cfg.RootSecret, Policy: string(policy)})
		if err != nil {
			t.Fatal("native scope configuration")
		}
		value, err := c.Get()
		if err != nil {
			t.Fatal("native scope issuance")
		}
		return pilot.Identity{AccessKey: value.AccessKeyID, SecretKey: value.SecretAccessKey, SessionToken: value.SessionToken}
	}
	allow := func(prefix string, actions ...string) any {
		return map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::raw/" + prefix + "*"}}
	}
	stage := "staging/" + l.ws.String() + "/"
	core := "core/" + l.ws.String() + "/"
	// MinIO expressly forbids STS-to-STS chaining. Use its native regular user
	// with a staging-only parent policy, then native per-attempt STS credentials.
	admin, err := madmin.New(p.StorageEndpoint, cfg.RootKey, cfg.RootSecret, false)
	if err != nil {
		t.Fatal("native IAM bootstrap")
	}
	publisher := pilot.Identity{AccessKey: "publisher-" + uuid.NewString(), SecretKey: d.Hash([]byte(uuid.NewString() + uuid.NewString()))}
	policyName := "b2-stage-" + l.ws.String()
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{allow(stage, "s3:GetObject", "s3:PutObject")}})
	if admin.AddCannedPolicy(ctx, policyName, policy) != nil || admin.SetUser(ctx, publisher.AccessKey, publisher.SecretKey, madmin.AccountEnabled) != nil {
		t.Fatal("native publisher user/policy bootstrap")
	}
	if _, err = admin.AttachPolicy(ctx, madmin.PolicyAssociationReq{Policies: []string{policyName}, User: publisher.AccessKey}); err != nil {
		t.Fatal("attach staging-only native publisher policy")
	}
	for file, identity := range map[string]pilot.Identity{p.CoreCredentialFile: scoped([]any{allow(stage, "s3:GetObject"), allow(core, "s3:GetObject", "s3:PutObject")}), p.PublisherCredentialFile: publisher} {
		b, _ := json.Marshal(identity)
		if os.WriteFile(file, b, 0600) != nil {
			t.Fatal("write private scoped identity")
		}
	}
	// Proxy real storage requests for a failed third PUT and a lost PUT reply.
	upstream, _ := url.Parse("http://" + p.StorageEndpoint)
	reverse := httputil.NewSingleHostReverseProxy(upstream)
	l.storageFault.Store("")
	var dropped atomic.Bool
	reverse.ModifyResponse = func(r *http.Response) error {
		if l.storageFault.Load().(string) == "revoke-after-put" && r.Request.Method == "PUT" && strings.Contains(r.Request.URL.Path, "/core/") {
			revoked := p
			revoked.Enabled = false
			l.saveProfile(revoked)
		}
		if l.storageFault.Load().(string) == "drop-put" && r.Request.Method == "PUT" && strings.Contains(r.Request.URL.Path, "/core/") && !dropped.Swap(true) {
			return fmt.Errorf("test dropped actual successful PUT reply")
		}
		return nil
	}
	reverse.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) { w.WriteHeader(502) }
	storageProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l.storageFault.Load().(string) == "fail-slot" && r.Method == "PUT" && strings.Contains(r.URL.Path, "/core/") && strings.HasSuffix(r.URL.Path, "/energy_raw.csv") {
			w.WriteHeader(403)
			return
		}
		reverse.ServeHTTP(w, r)
	}))
	defer storageProxy.Close()
	p.StorageEndpoint = strings.TrimPrefix(storageProxy.URL, "http://")
	// Trusted isolated operator provisioning, before exposing the HTTP Handler.
	for _, table := range p.Tables {
		rid, did := uuid.New(), uuid.New()
		code := p.ConnectionRef + "-" + table.Code
		if _, err := pool.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type,lifecycle_status) VALUES($1,$2,$3,$3,'TABLE_LIKE','READY')`, rid, l.ws, code); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type,source_resource_id) VALUES($1,$2,$3,$3,'RAW',$4)`, did, l.ws, code, rid); err != nil {
			t.Fatal(err)
		}
		p.Targets = append(p.Targets, pilot.TargetBinding{WorkspaceID: l.ws, Code: table.Code, ResourceID: rid, DatasetID: did})
	}
	l.cfg.Profile = p
	l.saveProfile(p)
	l.token = string(mustRead(t, p.TokenFile))
	h, err := pilot.Build(pool, l.profilePath)
	if err != nil {
		t.Fatal("build B2 profile")
	}
	l.handler = h
	l.server = httptest.NewServer(httpserver.NewMuxWithReadiness(pool.Ping, h.Register))
	defer l.server.Close()

	t.Run("server-owned-targets", func(t *testing.T) {
		l.t = t
		op := l.prepare()
		targets := []transport.Target{}
		for _, s := range op.Request.Slots {
			targets = append(targets, transport.Target{Code: s.Code, DatasetID: s.DatasetID, ResourceID: s.ResourceID})
		}
		before := l.count(`SELECT count(*) FROM ingestion_operation WHERE workspace_id=$1`, l.ws)
		for _, kind := range []string{"swapped", "repeated", "arbitrary"} {
			bad := slices.Clone(targets)
			switch kind {
			case "swapped":
				bad[0].DatasetID, bad[1].DatasetID = bad[1].DatasetID, bad[0].DatasetID
				bad[0].ResourceID, bad[1].ResourceID = bad[1].ResourceID, bad[0].ResourceID
			case "repeated":
				bad[0].DatasetID, bad[0].ResourceID = bad[1].DatasetID, bad[1].ResourceID
			case "arbitrary":
				rid, did := uuid.New(), uuid.New()
				if _, err := pool.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type,lifecycle_status) VALUES($1,$2,$3,$3,'TABLE_LIKE','READY')`, rid, l.ws, "unrelated-"+rid.String()); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type,source_resource_id) VALUES($1,$2,$3,$3,'RAW',$4)`, did, l.ws, "unrelated-"+did.String(), rid); err != nil {
					t.Fatal(err)
				}
				bad[0].DatasetID, bad[0].ResourceID = did, rid
			}
			l.call("POST", "/api/v1/ingestions", map[string]any{"workspaceId": l.ws, "requestKey": "bad-" + uuid.NewString(), "batchKey": "bad-" + kind, "targets": bad}, 403)
		}
		if l.count(`SELECT count(*) FROM ingestion_operation WHERE workspace_id=$1`, l.ws) != before || l.count(`SELECT count(*) FROM ingestion_invocation WHERE workspace_id=$1`, l.ws) != 0 {
			t.Fatal("rejected binding created operation or remote work")
		}
		l.empty(op)
		// Names may be provisioned through generic CRUD. Replacing the rows at
		// those names must not replace the trusted profile's target identities.
		original := targets[0]
		code := p.ConnectionRef + "-" + original.Code
		for _, table := range []string{"data_resource", "dataset"} {
			id := original.ResourceID
			if table == "dataset" {
				id = original.DatasetID
			}
			if _, err := pool.Exec(ctx, `UPDATE `+table+` SET code='original-'||id::text WHERE id=$1`, id); err != nil {
				t.Fatal(err)
			}
		}
		impostorResource, impostorDataset := uuid.New(), uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type,lifecycle_status) VALUES($1,$2,$3,$3,'TABLE_LIKE','READY')`, impostorResource, l.ws, code); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type,source_resource_id) VALUES($1,$2,$3,$3,'RAW',$4)`, impostorDataset, l.ws, code, impostorResource); err != nil {
			t.Fatal(err)
		}
		claimed := slices.Clone(targets)
		claimed[0].ResourceID, claimed[0].DatasetID = impostorResource, impostorDataset
		l.call("POST", "/api/v1/ingestions", map[string]any{"workspaceId": l.ws, "requestKey": "impostor-" + uuid.NewString(), "batchKey": "impostor", "targets": claimed}, 403)
		for _, table := range []string{"data_resource", "dataset"} {
			id, other := original.ResourceID, impostorResource
			if table == "dataset" {
				id, other = original.DatasetID, impostorDataset
			}
			if _, err := pool.Exec(ctx, `UPDATE `+table+` SET code='impostor-'||id::text WHERE id=$1`, other); err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `UPDATE `+table+` SET code=$2 WHERE id=$1`, id, code); err != nil {
				t.Fatal(err)
			}
		}
		// Changing the trusted profile after Prepare blocks the old binding.
		changed := p
		changed.Targets = slices.Clone(p.Targets)
		changed.Targets[0].ResourceID, changed.Targets[0].DatasetID = impostorResource, impostorDataset
		l.saveProfile(changed)
		l.command(op, "dispatch", nil, 403)
		l.saveProfile(p)
		if l.count(`SELECT count(*) FROM ingestion_invocation WHERE operation_id=$1`, op.ID) != 0 {
			t.Fatal("changed binding dispatched remote work")
		}
		l.checks["server_owned_target_binding"] = "PASS"
	})

	t.Run("manifest-negative-matrix", func(t *testing.T) {
		l.t = t
		op := l.completed()
		if op.State != "SUCCEEDED" {
			t.Fatal("Hop extraction failed")
		}
		l.empty(op)
		cases := map[string]func(*d.Manifest){
			"missing": func(m *d.Manifest) { m.Outputs = m.Outputs[:2] }, "extra": func(m *d.Manifest) { m.Outputs = append(m.Outputs, m.Outputs[0]) }, "duplicate": func(m *d.Manifest) { m.Outputs[1] = m.Outputs[0] },
			"workspace": func(m *d.Manifest) { m.WorkspaceID = uuid.New() }, "source": func(m *d.Manifest) { m.ConnectionRef = "other-source" }, "instance": func(m *d.Manifest) { m.InstanceRef = "other-instance" }, "schema": func(m *d.Manifest) { m.SchemaHash = d.Hash([]byte("different")) }, "template": func(m *d.Manifest) { m.TemplateHash = d.Hash([]byte("different")) }, "config": func(m *d.Manifest) { m.ConfigHash = d.Hash([]byte("different")) }, "batch": func(m *d.Manifest) { m.BatchKey = "different" }, "request": func(m *d.Manifest) { m.RequestKey = "different" }, "attempt": func(m *d.Manifest) { m.AttemptID = uuid.New() }, "engine-id": func(m *d.Manifest) { m.RemoteID = uuid.NewString() }, "engine-version": func(m *d.Manifest) { m.EngineVersion = "other" }, "dataset": func(m *d.Manifest) { m.Outputs[0].DatasetID = uuid.New() }, "path": func(m *d.Manifest) { m.Outputs[0].URI += "?redirect=other" }, "traversal": func(m *d.Manifest) {
				m.Outputs[0].URI = strings.Replace(m.Outputs[0].URI, "/enterprise_raw.csv", "/../enterprise_raw.csv", 1)
			}, "columns": func(m *d.Manifest) { m.Outputs[0].Columns[0] = "other" }, "rows": func(m *d.Manifest) { m.Outputs[0].Rows++ },
		}
		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				l.t = t
				m := clone(*op.Manifest)
				mutate(&m)
				l.command(op, "admit", m, 400)
				l.empty(op)
			})
		}
		l.t = t
		for _, field := range []string{"hash", "bytes", "timestamp"} {
			m := clone(*op.Manifest)
			switch field {
			case "hash":
				m.Outputs[0].SHA256 = d.Hash([]byte("tampered"))
			case "bytes":
				m.Outputs[0].Bytes++
			case "timestamp":
				m.ObservedAt = m.ObservedAt.Add(time.Second)
			}
			l.command(op, "admit", m, 409)
			l.empty(op)
		}
		// Real object tampering with the declared manifest unchanged.
		out := op.Manifest.Outputs[0]
		original := mustRead(t, filepath.Join(p.LocalRoot, op.AttemptID.String(), out.Code+".csv"))
		bad := bytes.Replace(original, []byte("2020/06/08"), []byte("2020-06-08"), 1)
		if bytes.Equal(original, bad) {
			bad = append(original, 'x')
		}
		_, err = root.Put(ctx, strings.TrimPrefix(out.URI, "s3://raw/"), bytes.NewReader(bad), int64(len(bad)), "text/csv")
		if err != nil {
			t.Fatal("inject actual staging corruption")
		}
		l.command(op, "admit", op.Manifest, 400)
		l.empty(op)
		_, err = root.Put(ctx, strings.TrimPrefix(out.URI, "s3://raw/"), bytes.NewReader(original), int64(len(original)), "text/csv")
		if err != nil {
			t.Fatal("restore actual staging")
		}
		l.accepted(op)
		l.checks["PGI05_manifest_and_bytes"] = "PASS"
	})
	t.Run("concurrent-prepare-and-admit", func(t *testing.T) {
		l.t = t
		op := l.completed()
		targets := []transport.Target{}
		for _, s := range op.Request.Slots {
			targets = append(targets, transport.Target{Code: s.Code, DatasetID: s.DatasetID, ResourceID: s.ResourceID})
		}
		body := map[string]any{"workspaceId": op.Request.WorkspaceID, "requestKey": op.Request.RequestKey, "batchKey": op.Request.BatchKey, "targets": targets}
		var prepareWG sync.WaitGroup
		for i := 0; i < 6; i++ {
			prepareWG.Add(1)
			go func() {
				defer prepareWG.Done()
				var replay d.Operation
				_ = json.Unmarshal(l.call("POST", "/api/v1/ingestions", body, 200), &replay)
				if replay.ID != op.ID || replay.AttemptID != op.AttemptID {
					t.Error("concurrent prepare changed identity")
				}
			}()
		}
		prepareWG.Wait()
		if l.count(`SELECT count(*) FROM ingestion_attempt WHERE operation_id=$1`, op.ID) != 1 {
			t.Fatal("concurrent prepare duplicated attempt")
		}
		var wg sync.WaitGroup
		ids := make(chan uuid.UUID, 6)
		var badVisibility atomic.Bool
		observing := make(chan struct{})
		observed := make(chan struct{})
		go func() {
			defer close(observed)
			for {
				select {
				case <-observing:
					return
				default:
				}
				var n int
				if pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version WHERE storage_uri LIKE $1`, "s3://raw/core/"+l.ws.String()+"/"+op.ID.String()+"/%").Scan(&n) != nil || (n != 0 && n != 3) {
					badVisibility.Store(true)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
		for i := 0; i < 6; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var rec d.Receipt
				_ = json.Unmarshal(l.command(op, "admit", op.Manifest, 200), &rec)
				ids <- rec.ID
			}()
		}
		wg.Wait()
		close(observing)
		<-observed
		if badVisibility.Load() {
			t.Fatal("independent reader observed partial batch")
		}
		close(ids)
		var first uuid.UUID
		for id := range ids {
			if first == uuid.Nil {
				first = id
			}
			if id != first {
				t.Fatal("concurrent admission produced different receipts")
			}
		}
		l.accepted(op)
		l.checks["PGI06_concurrent_admission_replay"] = "PASS"
	})
	t.Run("native-attempt-publisher-isolation", func(t *testing.T) {
		l.t = t
		op := l.completed()
		rec := l.accepted(op)
		publisher, err := l.handler.Service.Engine.(*adapter.Adapter).Publisher(ctx, op)
		if err != nil {
			t.Fatal("native attempt publisher unavailable")
		}
		_, readErr := publisher.Get(ctx, rec.Bindings[0].URI)
		_, writeErr := publisher.PutImmutable(ctx, d.CoreKey(op, op.Manifest.Outputs[0]), bytes.NewReader([]byte("forbidden")), 9, "text/csv")
		_, siblingErr := publisher.PutImmutable(ctx, "staging/"+l.ws.String()+"/other-operation/other-attempt/x.csv", bytes.NewReader([]byte("forbidden")), 9, "text/csv")
		for _, err := range []error{readErr, writeErr, siblingErr} {
			var response minio.ErrorResponse
			if !errors.As(err, &response) || response.StatusCode != 403 {
				t.Fatal("publisher boundary was not denied by the real provider")
			}
		}
		l.checks["native_attempt_identity_isolation"] = "PASS"
	})
	t.Run("commit-reply-loss-recovers-same-receipt", func(t *testing.T) {
		l.t = t
		op := l.completed()
		parsed, _ := url.Parse(cfg.DSN)
		proxy := newCommitProxy(t, parsed.Host)
		parsed.Host = proxy.listener.Addr().String()
		faultPool, err := pgxpool.New(ctx, parsed.String())
		if err != nil {
			t.Fatal("configure real commit-loss pool")
		}
		defer faultPool.Close()
		oldServer, oldHandler := l.server, l.handler
		copyService := *oldHandler.Service
		copyService.Tx = transaction.NewManager(faultPool)
		copyService.Repo = ingestioninfra.New(faultPool)
		copyService.Datasets = datasetinfra.NewPostgresRepository(faultPool)
		copyHandler := *oldHandler
		copyHandler.Service = &copyService
		faultServer := httptest.NewServer(httpserver.NewMux(copyHandler.Register))
		defer faultServer.Close()
		l.server = faultServer
		l.handler = &copyHandler
		proxy.armed.Store(true)
		l.command(op, "admit", op.Manifest, 503)
		select {
		case <-proxy.dropped:
		case <-time.After(3 * time.Second):
			t.Fatal("real PostgreSQL COMMIT reply was not dropped")
		}
		l.server = oldServer
		l.handler = oldHandler
		if l.count(`SELECT count(*) FROM ingestion_receipt WHERE operation_id=$1`, op.ID) != 1 {
			t.Fatal("backend did not commit before lost reply")
		}
		l.accepted(op)
		l.checks["PGI06_commit_reply_loss"] = "PASS"
	})
	t.Run("revocation-between-object-write-and-core-commit", func(t *testing.T) {
		l.t = t
		op := l.completed()
		l.storageFault.Store("revoke-after-put")
		l.command(op, "admit", op.Manifest, 403)
		l.empty(op)
		l.storageFault.Store("")
		l.saveProfile(p)
		l.accepted(op)
		l.checks["PGI07_admission_rechecks_authority"] = "PASS"
	})
	t.Run("third-object-failure-and-recovery", func(t *testing.T) {
		l.t = t
		op := l.completed()
		l.storageFault.Store("fail-slot")
		l.command(op, "admit", op.Manifest, 503)
		l.empty(op)
		l.storageFault.Store("")
		l.accepted(op)
		l.checks["PGI06_failed_slot_recovery"] = "PASS"
	})
	t.Run("lost-object-put-response", func(t *testing.T) {
		l.t = t
		op := l.completed()
		l.storageFault.Store("drop-put")
		l.accepted(op)
		if !dropped.Load() {
			t.Fatal("real PUT reply was not dropped")
		}
		l.storageFault.Store("")
		l.checks["PGI06_put_reply_loss"] = "PASS"
	})
	t.Run("database-rollback-and-recovery", func(t *testing.T) {
		l.t = t
		op := l.completed()
		previousEvidence := l.count(`SELECT count(*) FROM evidence WHERE source_id=$1`, op.ID)
		_, err := pool.Exec(ctx, `CREATE FUNCTION test_ingestion_slot_failure() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'synthetic second slot failure'; END $$ LANGUAGE plpgsql; CREATE TRIGGER test_ingestion_slot_failure BEFORE INSERT ON ingestion_receipt_slot FOR EACH ROW WHEN(NEW.code='lease_raw') EXECUTE FUNCTION test_ingestion_slot_failure()`)
		if err != nil {
			t.Fatal(err)
		}
		l.command(op, "admit", op.Manifest, 503)
		l.empty(op)
		if l.count(`SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='IngestionAccepted'`, op.ID) != 0 || l.count(`SELECT count(*) FROM evidence WHERE source_id=$1`, op.ID) != previousEvidence {
			t.Fatal("failed Core transaction retained acceptance side effects")
		}
		_, err = pool.Exec(ctx, `DROP TRIGGER test_ingestion_slot_failure ON ingestion_receipt_slot;DROP FUNCTION test_ingestion_slot_failure()`)
		if err != nil {
			t.Fatal(err)
		}
		l.accepted(op)
		l.checks["PGI06_transaction_rollback_recovery"] = "PASS"
	})
	t.Run("current-authority-and-source-revocation", func(t *testing.T) {
		l.t = t
		op := l.completed()
		invocations := l.count(`SELECT count(*) FROM ingestion_invocation WHERE operation_id=$1`, op.ID)
		denied := p
		denied.Enabled = false
		l.saveProfile(denied)
		l.command(op, "admit", op.Manifest, 403)
		l.command(op, "recover", nil, 403)
		l.empty(op)
		l.saveProfile(p)
		changed := p
		changed.InstanceRef = "replacement-instance"
		l.saveProfile(changed)
		l.command(op, "admit", op.Manifest, 403)
		l.command(op, "recover", nil, 403)
		l.empty(op)
		l.saveProfile(p)
		l.fault("source-revoke")
		l.command(op, "admit", op.Manifest, 403)
		l.command(op, "recover", nil, 403)
		l.empty(op)
		l.fault("source-restore")
		_, err = pool.Exec(ctx, `UPDATE data_resource SET lifecycle_status='BLOCKED' WHERE id=$1`, op.Request.Slots[0].ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		l.command(op, "admit", op.Manifest, 403)
		l.empty(op)
		_, err = pool.Exec(ctx, `UPDATE data_resource SET lifecycle_status='READY' WHERE id=$1`, op.Request.Slots[0].ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		if l.count(`SELECT count(*) FROM ingestion_invocation WHERE operation_id=$1`, op.ID) != invocations {
			t.Fatal("revocation caused a new engine invocation")
		}
		l.accepted(op)
		denied.Enabled = false
		l.saveProfile(denied)
		l.command(op, "admit", op.Manifest, 403)
		l.saveProfile(p)
		l.checks["PGI07_fresh_authority_source_config"] = "PASS"
	})
	t.Run("unknown-register-never-restarted", func(t *testing.T) {
		l.t = t
		op := l.prepare()
		l.fault("drop-register")
		l.command(op, "dispatch", nil, 503)
		l.command(op, "dispatch", nil, 409)
		l.command(op, "recover", nil, 503)
		l.empty(op)
		if l.count(`SELECT count(*) FROM ingestion_invocation WHERE operation_id=$1 AND kind='START'`, op.ID) != 0 {
			t.Fatal("unknown register was started")
		}
		l.checks["PGI07_unknown_register"] = "PASS"
	})
	t.Run("lost-start-status-recovery", func(t *testing.T) {
		l.t = t
		op := l.prepare()
		l.fault("drop-start")
		l.command(op, "dispatch", nil, 503)
		l.command(op, "dispatch", nil, 409)
		op = l.finish(op)
		l.accepted(op)
		if l.count(`SELECT count(*) FROM ingestion_invocation WHERE operation_id=$1 AND kind='START'`, op.ID) != 1 {
			t.Fatal("lost start was repeated")
		}
		l.checks["PGI07_unknown_start_recovery"] = "PASS"
	})
	t.Run("unknown-status-retains-attempt", func(t *testing.T) {
		l.t = t
		op := l.prepare()
		_ = json.Unmarshal(l.command(op, "dispatch", nil, 200), &op)
		l.fault("status-unavailable")
		l.command(op, "recover", nil, 503)
		l.empty(op)
		l.command(op, "retry-failed", nil, 409)
		l.fault("reset")
		op = l.finish(op)
		l.accepted(op)
		l.checks["PGI07_unknown_status"] = "PASS"
	})
	t.Run("status-reply-must-confirm-exact-identity", func(t *testing.T) {
		l.t = t
		op := l.prepare()
		_ = json.Unmarshal(l.command(op, "dispatch", nil, 200), &op)
		l.fault("status-without-identity")
		l.command(op, "recover", nil, 503)
		l.empty(op)
		l.fault("reset")
		op = l.finish(op)
		l.accepted(op)
		if l.count(`SELECT count(*) FROM ingestion_invocation WHERE operation_id=$1 AND kind='START'`, op.ID) != 1 {
			t.Fatal("malformed status caused a repeated start")
		}
		l.checks["PGI07_status_identity_required"] = "PASS"
	})
	t.Run("failed-hop-explicit-new-attempt", func(t *testing.T) {
		l.t = t
		op := l.prepare()
		old := op.AttemptID
		l.fault("fail-start")
		_ = json.Unmarshal(l.command(op, "dispatch", nil, 200), &op)
		time.Sleep(1200 * time.Millisecond)
		l.fault("source-restore")
		op = l.finish(op)
		if op.State != "FAILED" {
			t.Fatal("Hop failure was not independently confirmed")
		}
		l.empty(op)
		_ = json.Unmarshal(l.command(op, "retry-failed", nil, 200), &op)
		if op.AttemptID == old {
			t.Fatal("physical retry reused failed attempt")
		}
		_ = json.Unmarshal(l.command(op, "dispatch", nil, 200), &op)
		op = l.finish(op)
		l.accepted(op)
		if l.count(`SELECT count(*) FROM ingestion_attempt WHERE operation_id=$1`, op.ID) != 2 || l.count(`SELECT count(*) FROM ingestion_invocation WHERE operation_id=$1 AND kind='START'`, op.ID) != 2 {
			t.Fatal("retry did not retain distinct physical attempts/costs")
		}
		l.checks["PGI07_failed_attempt_recovery"] = "PASS"
	})
	t.Run("hop-restart-id-loss-fails-closed", func(t *testing.T) {
		l.t = t
		op := l.prepare()
		_ = json.Unmarshal(l.command(op, "dispatch", nil, 200), &op)
		l.fault("restart-hop")
		l.command(op, "recover", nil, 503)
		l.command(op, "retry-failed", nil, 409)
		l.command(op, "dispatch", nil, 409)
		l.empty(op)
		l.checks["PGI07_restart_identity_loss"] = "PASS"
	})
	l.t = t
	if t.Failed() {
		return
	}
	invocations := l.count(`SELECT count(*) FROM ingestion_invocation`)
	if invocations != l.count(`SELECT count(*) FROM cost_allocation a JOIN cost_event c ON c.id=a.cost_event_id WHERE a.ingestion_invocation_id IS NOT NULL AND c.activity_id=a.ingestion_invocation_id AND c.quantity=1 AND c.unit='invocation'`) {
		t.Fatal("physical invocation cost missing/merged")
	}
	l.checks["attempt_cost_history"] = "PASS"
	// Scan Core facts as well as wire/log/report artifacts. Only a boolean is
	// returned; secret values and private configuration never enter diagnostics.
	var coreIdentity pilot.Identity
	_ = json.Unmarshal(mustRead(t, p.CoreCredentialFile), &coreIdentity)
	secrets := []string{cfg.RootKey, cfg.RootSecret, l.token, publisher.AccessKey, publisher.SecretKey, coreIdentity.AccessKey, coreIdentity.SecretKey, coreIdentity.SessionToken, string(mustRead(t, p.SourcePasswordFile)), string(mustRead(t, p.HopPasswordFile))}
	for _, secret := range secrets {
		if secret == "" {
			continue
		}
		var leaked bool
		err = pool.QueryRow(ctx, `WITH facts AS (
	 SELECT to_jsonb(x)::text v FROM ingestion_operation x UNION ALL SELECT to_jsonb(x)::text FROM ingestion_request_slot x
	 UNION ALL SELECT to_jsonb(x)::text FROM ingestion_invocation x UNION ALL SELECT to_jsonb(x)::text FROM ingestion_observation x
	 UNION ALL SELECT to_jsonb(x)::text FROM ingestion_receipt x UNION ALL SELECT to_jsonb(x)::text FROM ingestion_receipt_slot x
	 UNION ALL SELECT to_jsonb(x)::text FROM cost_event x UNION ALL SELECT to_jsonb(x)::text FROM audit_event x
	 UNION ALL SELECT to_jsonb(x)::text FROM evidence x UNION ALL SELECT to_jsonb(x)::text FROM outbox_event x)
	 SELECT EXISTS(SELECT 1 FROM facts WHERE strpos(v,$1)>0)`, secret).Scan(&leaked)
		if err != nil || leaked {
			t.Fatal("secret appeared in Core facts")
		}
	}
	l.checks["core_fact_secret_isolation"] = "PASS"
	// Strong guards reject direct mutation of historical facts.
	for _, sql := range []string{`UPDATE ingestion_attempt SET started_at=now()`, `DELETE FROM ingestion_invocation`, `UPDATE ingestion_observation SET state='FAILED'`, `DELETE FROM ingestion_receipt`, `UPDATE ingestion_receipt_slot SET content_hash=repeat('0',64)`, `UPDATE ingestion_request_slot SET row_count=0`, `UPDATE ingestion_operation SET batch_key='rewrite'`} {
		if _, err = pool.Exec(ctx, sql); err == nil {
			t.Fatal("historical mutation allowed")
		}
	}
	l.checks["immutable_history"] = "PASS"
	sortReceipts := slices.Clone(l.receipts)
	slices.SortFunc(sortReceipts, func(a, b d.Receipt) int { return strings.Compare(a.ID.String(), b.ID.String()) })
	facts := map[string]any{"checks": l.checks, "invocations": invocations, "accepted_batches": len(l.receipts), "receipts": sortReceipts, "manifests": l.manifests}
	result, _ := json.MarshalIndent(facts, "", "  ")
	if os.WriteFile(filepath.Join(cfg.Output, "b2-facts.json"), append(result, '\n'), 0644) != nil {
		t.Fatal("save sanitized evidence")
	}
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read controlled file")
	}
	return b
}
