package sourceadmission

// Explicit synthetic real-Fork/native-MinIO composition. No testPort,
// newFixture, cmd/api, Core engine, shared credentials or deletion path.
import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	ra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	ri "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

func TestLiveForkHostFixture(t *testing.T) {
	if os.Getenv("FORK_HOST_LIVE_SYNTHETIC") != "1" {
		t.Skip("explicit isolated real-service trial only")
	}
	ctx := context.Background()
	root := os.Getenv("HFRAW_OUTPUT")
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	u, e := url.Parse(dsn)
	if e != nil || u.Hostname() != "127.0.0.1" || u.Path != "/hfraw_host" || root == "" {
		t.Fatal("dedicated loopback hfraw_host database/private directory required")
	}
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	save := func(name string, v any) {
		b, e := json.Marshal(v)
		must(e)
		must(os.WriteFile(filepath.Join(root, name), b, 0600))
	}
	read := func(name string, v any) {
		b, e := os.ReadFile(filepath.Join(root, name))
		must(e)
		must(json.Unmarshal(b, v))
	}
	router, e := routing.NewRouter(false)
	must(e)
	outbox.ConfigureAppendObligation(router)
	var fork struct{ Namespace, Project, Organization string }
	read("fork-fixture.json", &fork)
	var adminID storageIdentity
	read("minio-admin.json", &adminID)
	native, e := minio.New("127.0.0.1:25512", &minio.Options{Creds: credentials.NewStaticV4(adminID.AccessKey, adminID.SecretKey, ""), Region: "us-east-1", BucketLookup: minio.BucketLookupPath})
	must(e)
	var p Profile
	profilePath := filepath.Join(root, "host-profile.json")
	if _, e = os.Stat(profilePath); os.IsNotExist(e) {
		ws, actor, resource, dataset := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		consumer, purpose := "HFRAW-SYNTHETIC", "SOURCE_ADMISSION"
		scope := rd.NormalizedScope{Type: "ALL_RESOURCE", Ref: resource.String()}
		_, e = pool.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type,lifecycle_status) VALUES($1,$2,$3,'synthetic real Fork source','FILE_COLLECTION','READY')`, resource, ws, resource.String())
		must(e)
		_, e = pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type,source_resource_id,lifecycle_status) VALUES($1,$2,$3,'synthetic real RAW','RAW',$4,'ACTIVE')`, dataset, ws, dataset.String(), resource)
		must(e)
		rights := ra.NewService(transaction.NewManager(pool), ri.NewPostgresRepository(pool))
		declaration, e := rights.CreateRightsDeclaration(ctx, ra.CreateRightsDeclarationCommand{Spec: rd.RightsDeclarationSpec{WorkspaceID: ws, DataResourceID: resource, ClaimantRef: consumer, BasisType: "LICENSE", BasisRef: "synthetic-only-no-business-grant", ConsumerScopeType: "EXPLICIT", ConsumerRef: consumer, Parties: []rd.RightsParty{{PartyRef: consumer, Role: "RIGHTS_HOLDER"}}, Permissions: []rd.RightsPermission{{Kind: rd.PermissionUse, Action: "READ", Purpose: purpose, Scope: scope}, {Kind: rd.PermissionUse, Action: "PROCESS", Purpose: purpose, Scope: scope}}}})
		must(e)
		_, e = rights.VerifyRightsDeclaration(ctx, ra.VerifyRightsDeclarationCommand{DeclarationID: declaration.ID, Outcome: rd.DeclarationVerified})
		must(e)
		save("host-rights.json", map[string]any{"declaration": declaration.ID})
		bucket := "hfraw-" + ws.String()
		must(native.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: "us-east-1"}))
		policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:PutObject"}, "Resource": []string{"arn:aws:s3:::" + bucket + "/core/" + ws.String() + "/*"}}}})
		sts, e := credentials.NewSTSAssumeRole("http://127.0.0.1:25512", credentials.STSAssumeRoleOptions{AccessKey: adminID.AccessKey, SecretKey: adminID.SecretKey, Policy: string(policy), DurationSeconds: 3600})
		must(e)
		value, e := sts.Get()
		must(e)
		save("storage-runtime.json", storageIdentity{value.AccessKeyID, value.SecretAccessKey, value.SessionToken})
		p = Profile{Enabled: true, Connection: Connection{ID: uuid.New(), WorkspaceID: ws, Namespace: uuid.MustParse(fork.Namespace), DatasetID: dataset, ResourceID: resource, Project: fork.Project, Organization: fork.Organization, Mode: "fork_approved", BaseURL: "http://127.0.0.1:25517", TokenFile: filepath.Join(root, "fork-token"), Consumer: consumer, Purpose: purpose, Scope: scope}, ActorID: actor, Subject: "hfraw-synthetic-operator", APITokenFile: filepath.Join(root, "host-token"), StorageScope: lc.ProtectedScope{WorkspaceID: ws, ActorID: actor, NamespaceID: uuid.New(), ProviderInstanceID: uuid.New(), IncarnationID: uuid.New(), PrincipalRef: "hfraw-synthetic-operator", Bucket: bucket, ExpiresAt: time.Now().Add(time.Hour)}, StorageEndpoint: "127.0.0.1:25516", StorageCredentialFile: filepath.Join(root, "storage-runtime.json")}
		save("host-profile.json", p)
	} else {
		read("host-profile.json", &p)
	}
	// Transparent failure injection forwards to the NEW real MinIO. It never
	// synthesizes successful provider data; the successful lost PUT is retained.
	var fault atomic.Bool
	upstream, _ := url.Parse("http://127.0.0.1:25512")
	reverse := httputil.NewSingleHostReverseProxy(upstream)
	trace, e := os.OpenFile(filepath.Join(root, "storage-transport.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	must(e)
	defer trace.Close()
	reverse.ModifyResponse = func(r *http.Response) error {
		b, _ := json.Marshal(map[string]any{"method": r.Request.Method, "path": r.Request.URL.Path, "status": r.StatusCode})
		_, _ = trace.Write(append(b, '\n'))
		if fault.Load() && r.Request.Method == "PUT" && r.StatusCode >= 200 && r.StatusCode < 300 {
			return fmt.Errorf("synthetic lost successful native PUT reply")
		}
		return nil
	}
	reverse.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) { w.WriteHeader(502) }
	proxy := &http.Server{Addr: "127.0.0.1:25516", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fault.Load() && r.Method == "GET" {
			b, _ := json.Marshal(map[string]any{"method": r.Method, "path": r.URL.Path, "status": 502, "fault": "temporary-read-outage"})
			_, _ = trace.Write(append(b, '\n'))
			w.WriteHeader(502)
			return
		}
		reverse.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second}
	listener, e := net.Listen("tcp", proxy.Addr)
	must(e)
	go proxy.Serve(listener)
	defer proxy.Close()
	handler, e := Build(pool, profilePath)
	must(e)
	mux := http.NewServeMux()
	handler.Register(mux)
	// Control plane is test-only loopback and requires a private random secret.
	secret, e := os.ReadFile(filepath.Join(root, "control-token"))
	must(e)
	stop := make(chan struct{}, 1)
	count := func(q string, args ...any) int { var n int; must(pool.QueryRow(ctx, q, args...).Scan(&n)); return n }
	mux.HandleFunc("POST /__live/control", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Live-Control") != string(secret) {
			w.WriteHeader(403)
			return
		}
		var cmd struct{ Action string }
		if json.NewDecoder(r.Body).Decode(&cmd) != nil {
			w.WriteHeader(400)
			return
		}
		switch cmd.Action {
		case "deny", "enable":
			_, e = pool.Exec(r.Context(), `UPDATE source_admission_connection SET enabled=$2 WHERE id=$1`, p.Connection.ID, cmd.Action == "enable")
		case "storage-fault":
			fault.Store(true)
		case "storage-heal":
			fault.Store(false)
		case "rights-deny":
			var fact struct{ Declaration uuid.UUID }
			read("host-rights.json", &fact)
			rights := ra.NewService(transaction.NewManager(pool), ri.NewPostgresRepository(pool))
			_, e = rights.DisposeRightsDeclaration(r.Context(), ra.DisposeRightsDeclarationCommand{DeclarationID: fact.Declaration, Disposition: "INVALIDATED", EffectiveAt: time.Now().UTC(), Reason: "synthetic current-rights negative", ActorID: &p.ActorID})
		case "scope-negatives":
			var identity storageIdentity
			read("storage-runtime.json", &identity)
			client, err := minio.New("127.0.0.1:25512", &minio.Options{Creds: credentials.NewStaticV4(identity.AccessKey, identity.SecretKey, identity.SessionToken), Region: "us-east-1", BucketLookup: minio.BucketLookupPath})
			must(err)
			_, err = client.PutObject(ctx, p.StorageScope.Bucket, "foreign/synthetic-denied", strings.NewReader("synthetic"), 9, minio.PutObjectOptions{DisableMultipart: true})
			if minio.ToErrorResponse(err).StatusCode != 403 {
				t.Error("foreign prefix PUT did not deny")
				w.WriteHeader(500)
				return
			}
			err = client.RemoveObject(ctx, p.StorageScope.Bucket, p.StorageScope.Prefix()+"absent-deny-probe", minio.RemoveObjectOptions{})
			if minio.ToErrorResponse(err).StatusCode != 403 {
				t.Error("DELETE not denied")
				w.WriteHeader(500)
				return
			}
			_, err = client.ListBuckets(ctx)
			if minio.ToErrorResponse(err).StatusCode != 403 {
				t.Error("LIST not denied")
				w.WriteHeader(500)
				return
			}
		case "corrupt-copy":
			var key string
			e = pool.QueryRow(ctx, `SELECT i.object_key FROM lifecycle_protected_object_intent i JOIN source_admission_capture c ON c.version_id=i.dataset_version_id WHERE c.workspace_id=$1 AND c.state='ACCEPTED_SOURCE' AND i.object_key LIKE '%/records.jsonl' ORDER BY c.created_at DESC LIMIT 1`, p.Connection.WorkspaceID).Scan(&key)
			if e == nil {
				_, e = native.PutObject(ctx, p.StorageScope.Bucket, key, strings.NewReader("synthetic-corruption"), 20, minio.PutObjectOptions{DisableMultipart: true})
			}
		case "stop":
			select {
			case stop <- struct{}{}:
			default:
			}
		default:
			w.WriteHeader(400)
			return
		}
		if e != nil {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /__live/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Live-Control") != string(secret) {
			w.WriteHeader(403)
			return
		}
		ws := p.Connection.WorkspaceID
		data := map[string]any{"versions": count(`SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, p.Connection.DatasetID), "captures": count(`SELECT count(*) FROM source_admission_capture WHERE workspace_id=$1`, ws), "accepted": count(`SELECT count(*) FROM source_admission_capture WHERE workspace_id=$1 AND state='ACCEPTED_SOURCE'`, ws), "calls": count(`SELECT count(*) FROM source_admission_call WHERE workspace_id=$1`, ws), "artifacts": count(`SELECT count(*) FROM source_admission_call WHERE workspace_id=$1 AND asset_name<>'manifest.json'`, ws), "costs": count(`SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type='SOURCE_ADMISSION_FETCH'`, ws), "allocations": count(`SELECT count(*) FROM cost_allocation a JOIN source_admission_call c ON c.id=a.source_admission_call_id WHERE c.workspace_id=$1`, ws), "copies": count(`SELECT count(*) FROM lifecycle_protected_object_intent WHERE workspace_id=$1`, ws), "storageCalls": count(`SELECT count(*) FROM lifecycle_storage_call WHERE workspace_id=$1`, ws), "storageCosts": count(`SELECT count(*) FROM cost_event WHERE workspace_id=$1 AND cost_type='LIFECYCLE_PROVIDER_INVOCATION'`, ws), "rejected": count(`SELECT count(*) FROM source_admission_operation WHERE workspace_id=$1 AND state='FAILED_FINAL'`, ws)}
		_ = json.NewEncoder(w).Encode(data)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	mux.HandleFunc("GET /__live/closure", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Live-Control") != string(secret) {
			w.WriteHeader(403)
			return
		}
		ws := p.Connection.WorkspaceID
		data := map[string]any{}
		for _, table := range []string{"annotation_result", "annotation_review_decision", "annotation_snapshot", "dataset_certification", "delivery_operation"} {
			data[table] = count("SELECT count(*) FROM "+table+" WHERE workspace_id=$1", ws)
		}
		data["product_release"] = count(`SELECT count(*) FROM product_release r JOIN data_product p ON p.id=r.product_id WHERE p.workspace_id=$1`, ws)
		data["badSourceCosts"] = count(`SELECT count(*) FROM source_admission_call c LEFT JOIN cost_event e ON e.activity_id=c.id AND e.cost_type='SOURCE_ADMISSION_FETCH' LEFT JOIN cost_allocation a ON a.cost_event_id=e.id AND a.source_admission_call_id=c.id WHERE c.workspace_id=$1 AND (e.id IS NULL OR a.id IS NULL OR e.quantity<>1 OR e.unit<>'invocation' OR e.amount IS NOT NULL OR e.currency IS NOT NULL OR e.pricing_mode<>'ACTUAL')`, ws)
		data["badStorageCosts"] = count(`SELECT count(*) FROM lifecycle_storage_call c JOIN lifecycle_storage_observation o ON o.call_id=c.id AND o.invoked_at IS NOT NULL LEFT JOIN cost_event e ON e.activity_id=c.id AND e.cost_type='LIFECYCLE_PROVIDER_INVOCATION' LEFT JOIN cost_allocation a ON a.cost_event_id=e.id AND a.lifecycle_storage_call_id=c.id WHERE c.workspace_id=$1 AND (e.id IS NULL OR a.id IS NULL OR e.quantity<>1 OR e.unit<>'invocation' OR e.amount IS NOT NULL OR e.currency IS NOT NULL OR e.pricing_mode<>'ACTUAL')`, ws)
		data["badAssets"] = count(`SELECT count(*) FROM source_admission_asset a JOIN source_admission_capture c ON c.id=a.capture_id JOIN lifecycle_protected_object_intent i ON i.dataset_version_id=c.version_id AND i.object_key LIKE '%/'||a.name WHERE c.workspace_id=$1 AND c.state='ACCEPTED_SOURCE' AND (i.content_sha256<>a.sha256 OR i.byte_size<>a.byte_size OR a.storage_uri IS NULL)`, ws)
		data["identities"] = count(`SELECT count(*) FROM source_admission_identity WHERE namespace_id=$1`, p.Connection.Namespace)
		_ = json.NewEncoder(w).Encode(data)
	})
	listener, e = net.Listen("tcp", "127.0.0.1:25513")
	must(e)
	go server.Serve(listener)
	defer server.Close()
	save("live-ready.json", map[string]any{"workspaceId": p.Connection.WorkspaceID, "datasetId": p.Connection.DatasetID, "connectionId": p.Connection.ID, "profile": profilePath, "gatewayTokenFile": filepath.Join(root, "gateway-token"), "api": "http://127.0.0.1:25513", "subject": p.Subject})
	select {
	case <-stop:
	case <-time.After(45 * time.Minute):
		t.Fatal("owned live harness timed out")
	}
}
