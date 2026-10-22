package pilot_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	da "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	dd "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	di "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	ea "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	ed "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	ei "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/industrypack/park/indicator"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	ri "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	wa "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	wd "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	wi "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/native"
	wq "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/transport/queue"
)

func nativeWorkspace(t *testing.T) (lc.ProtectedScope, *lc.ProtectedObjects, *pgxpool.Pool, *minio.Client, *storage.Store, string) {
	t.Helper()
	path := os.Getenv("TEST_WORKSPACE_STORAGE_CONFIG")
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if path == "" || dsn == "" {
		t.Skip("owned native storage/PostgreSQL config not set")
	}
	ctx := context.Background()
	var cfg struct{ Endpoint, AccessKey, SecretKey string }
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &cfg) != nil {
		t.Fatal("private test config unavailable")
	}
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	root, err := storage.New(cfg.Endpoint, cfg.AccessKey, cfg.SecretKey, "raw", false)
	if err != nil {
		t.Fatal(err)
	}
	if err = root.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	} // fixture provisioning only
	w := uuid.New()
	allow := func(prefix string, actions ...string) any {
		return map[string]any{"Effect": "Allow", "Action": actions, "Resource": []string{"arn:aws:s3:::raw/" + prefix + "*"}}
	}
	policy, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{allow("staging/"+w.String()+"/", "s3:GetObject"), allow("core/"+w.String()+"/", "s3:GetObject", "s3:PutObject")}})
	c, err := credentials.NewSTSAssumeRole("http://"+cfg.Endpoint, credentials.STSAssumeRoleOptions{AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey, Policy: string(policy), DurationSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.Get()
	if err != nil {
		t.Fatal("native finite STS unavailable")
	}
	scope := lc.ProtectedScope{WorkspaceID: w, ActorID: uuid.New(), NamespaceID: uuid.New(), ProviderInstanceID: uuid.NewSHA1(uuid.NameSpaceURL, []byte(cfg.Endpoint)), IncarnationID: uuid.NewSHA1(uuid.NameSpaceURL, []byte("issue330-owned-regression:"+cfg.Endpoint)), PrincipalRef: "issue330-native-owner", Bucket: "raw", ExpiresAt: v.Expiration}
	d := t.TempDir()
	credential := filepath.Join(d, "identity.json")
	b, _ = json.Marshal(map[string]string{"AccessKey": v.AccessKeyID, "SecretKey": v.SecretAccessKey, "SessionToken": v.SessionToken})
	if err = os.WriteFile(credential, b, 0600); err != nil {
		t.Fatal(err)
	}
	profile := pilot.WorkspaceObjectProfile{Scope: scope, Endpoint: cfg.Endpoint, CredentialFile: credential}
	b, _ = json.Marshal(profile)
	profilePath := filepath.Join(d, "profile.json")
	_ = os.WriteFile(profilePath, b, 0600)
	service, err := pilot.BuildWorkspaceObjects(pool, profilePath)
	if err != nil {
		t.Fatal(err)
	}
	client, err := minio.New(cfg.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(v.AccessKeyID, v.SecretAccessKey, v.SessionToken), Secure: false, Region: "us-east-1", MaxRetries: 1})
	if err != nil {
		t.Fatal(err)
	}
	return scope, service, pool, client, root, profilePath
}

type nativeLegacyFixture struct {
	root   *storage.Store
	prefix string
}

func (s nativeLegacyFixture) Put(ctx context.Context, _ string, r io.Reader, n int64, kind string) (string, error) {
	return s.root.Put(ctx, s.prefix+"/"+uuid.NewString()+".csv", r, n, kind)
}

func TestProtectedWorkspaceNativeSTSAndRetainedCopy(t *testing.T) {
	scope, s, pool, client, root, profilePath := nativeWorkspace(t)
	ctx := context.Background()
	tx := transaction.NewManager(pool)
	repo := di.NewPostgresRepository(pool)
	create := da.NewCreateDatasetService(tx, repo, ri.NewPostgresRepository())
	newDataset := func(kind dd.DatasetType) dd.Dataset {
		d, e := create.Handle(ctx, da.CreateDatasetCommand{WorkspaceID: scope.WorkspaceID, ActorID: &scope.ActorID, Code: uuid.NewString(), Name: "native workspace storage", DatasetType: kind})
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	data := []byte("id,value\n1,native-sts\n")
	upload := da.NewUploadVersionService(tx, repo, s).WithPublicationLease(s)
	d := newDataset(dd.DatasetTypeCurated)
	v, err := upload.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "curated.csv", ContentType: "text/csv", Content: data})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.ReadVersion(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("native scoped bytes mismatch")
	}
	key := strings.TrimPrefix(v.StorageURI, "s3://raw/")
	object, err := client.GetObject(ctx, "raw", key, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got, err = io.ReadAll(object)
	_ = object.Close()
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("direct native scoped GET failed")
	}
	_, err = client.PutObject(ctx, "raw", key, bytes.NewReader([]byte("overwrite")), 9, minio.PutObjectOptions{DisableMultipart: true})
	if minio.ToErrorResponse(err).StatusCode != 403 {
		t.Fatal("gateway permits unconditional PUT")
	}
	foreignKey := "core/" + uuid.NewString() + "/foreign.csv"
	_, err = root.PutImmutable(ctx, foreignKey, bytes.NewReader(data), int64(len(data)), "text/csv")
	if err != nil {
		t.Fatal(err)
	}
	object, err = client.GetObject(ctx, "raw", foreignKey, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(object)
	_ = object.Close()
	if minio.ToErrorResponse(err).StatusCode != 403 {
		t.Fatal("foreign workspace GET was not denied")
	}
	_, err = client.PutObject(ctx, "raw", foreignKey+".new", bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{})
	if minio.ToErrorResponse(err).StatusCode != 403 {
		t.Fatal("foreign workspace PUT was not denied")
	}

	for _, kind := range []dd.DatasetType{dd.DatasetTypeStandardized, dd.DatasetTypeCurated} {
		d := newDataset(kind)
		oldWriter := da.NewUploadVersionService(tx, repo, nativeLegacyFixture{root: root, prefix: "datasets/issue330-" + scope.WorkspaceID.String()})
		old, e := oldWriter.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: "old.csv", ContentType: "text/csv", Content: data})
		if e != nil {
			t.Fatal(e)
		}
		var before, after string
		_ = pool.QueryRow(ctx, `SELECT to_jsonb(v)::text FROM dataset_version v WHERE id=$1`, old.ID).Scan(&before)
		object, e := client.GetObject(ctx, "raw", strings.TrimPrefix(old.StorageURI, "s3://raw/"), minio.GetObjectOptions{})
		if e != nil {
			t.Fatal(e)
		}
		_, e = io.ReadAll(object)
		_ = object.Close()
		if minio.ToErrorResponse(e).StatusCode != 403 {
			t.Fatal("legacy namespace unexpectedly granted")
		}
		if _, e = s.ReadVersion(ctx, old.ID); e == nil {
			t.Fatal("unregistered legacy reader fallback")
		}
		custody := filepath.Join(t.TempDir(), "custody.csv")
		if e = os.WriteFile(custody, data, 0600); e != nil {
			t.Fatal(e)
		}
		command := exec.CommandContext(ctx, "go", "run", "../../../cmd/workspace-object-copy", "-profile", profilePath, "-version", old.ID.String(), "-content-file", custody, "-idempotency-key", "explicit-native-custody-"+string(kind))
		output, e := command.CombinedOutput()
		if e != nil {
			t.Fatalf("explicit operator entry point failed: %v: %s", e, output)
		}
		var receipt struct {
			PhysicalCopyURI string `json:"physicalCopyURI"`
		}
		if e = json.Unmarshal(output, &receipt); e != nil || receipt.PhysicalCopyURI == "" {
			t.Fatal("operator receipt missing verified copy")
		}
		copyURI := receipt.PhysicalCopyURI
		var countBefore, countAfter int
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_storage_call WHERE workspace_id=$1`, scope.WorkspaceID).Scan(&countBefore)
		again, e := s.AddRetainedCopy(ctx, old.ID, "explicit-native-custody-"+string(kind), data)
		if e != nil || again != copyURI {
			t.Fatal("native copy replay mismatch")
		}
		_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_storage_call WHERE workspace_id=$1`, scope.WorkspaceID).Scan(&countAfter)
		if countBefore != countAfter {
			t.Fatal("native copy replay invoked provider")
		}
		r, e := s.Get(ctx, old.StorageURI)
		if e != nil {
			t.Fatal(e)
		}
		got, _ := io.ReadAll(r)
		_ = r.Close()
		if !bytes.Equal(got, data) {
			t.Fatal("retained native read mismatch")
		}
		_ = pool.QueryRow(ctx, `SELECT to_jsonb(v)::text FROM dataset_version v WHERE id=$1`, old.ID).Scan(&after)
		if before != after {
			t.Fatal("old business version rewritten")
		}
	}
	var calls, costs int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_storage_call WHERE workspace_id=$1 AND protected_object_intent_id IS NOT NULL`, scope.WorkspaceID).Scan(&calls)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation a JOIN lifecycle_storage_call c ON c.id=a.lifecycle_storage_call_id WHERE c.workspace_id=$1 AND c.protected_object_intent_id IS NOT NULL`, scope.WorkspaceID).Scan(&costs)
	if calls == 0 || calls != costs {
		t.Fatalf("physical native call costs: %d calls / %d costs", calls, costs)
	}
	t.Logf("NATIVE PASS: new scoped write/read; 2 retained versions unchanged; cross-workspace GET/PUT 403; %d recorded physical calls; no provider DELETE", calls)
}

func TestProtectedWorkspaceNativeEntityReviewAndFrozenWorkflow(t *testing.T) {
	scope, s, pool, _, root, _ := nativeWorkspace(t)
	ctx := context.Background()
	tx := transaction.NewManager(pool)
	datasets := di.NewPostgresRepository(pool)
	entities := ei.NewPostgresRepository(pool)
	workflows := wi.NewPostgresRepository(pool)
	create := da.NewCreateDatasetService(tx, datasets, ri.NewPostgresRepository())
	newDataset := func(kind dd.DatasetType) dd.Dataset {
		d, e := create.Handle(ctx, da.CreateDatasetCommand{WorkspaceID: scope.WorkspaceID, ActorID: &scope.ActorID, Code: uuid.NewString(), Name: "native scoped pipeline", DatasetType: kind})
		if e != nil {
			t.Fatal(e)
		}
		return d
	}
	adminFixtureWriter := da.NewUploadVersionService(tx, datasets, root)
	writer := da.NewUploadVersionService(tx, datasets, s).WithPublicationLease(s)
	repoRoot := filepath.Clean("../../../../../")
	versions := map[string]dd.DatasetVersion{}
	for _, name := range []string{"enterprise", "lease", "energy"} {
		d := newDataset(dd.DatasetTypeRaw)
		data, e := os.ReadFile(filepath.Join(repoRoot, "examples/enterprise-activity/data", name+".csv"))
		if e != nil {
			t.Fatal(e)
		}
		v, e := adminFixtureWriter.Handle(ctx, da.UploadVersionCommand{DatasetID: d.ID, Filename: name + ".csv", ContentType: "text/csv", Content: data, ActorID: &scope.ActorID})
		if e != nil {
			t.Fatal(e)
		}
		versions[name] = v
	}
	standardized, curated := newDataset(dd.DatasetTypeStandardized), newDataset(dd.DatasetTypeCurated)
	matcher := ea.NewMatchService(filepath.Join(repoRoot, "industry-packs"), tx, entities, datasets, writer, s)
	job, err := matcher.Start(ctx, ea.StartJobCommand{WorkspaceID: scope.WorkspaceID, InputDatasetVersionID: versions["enterprise"].ID, OutputDatasetID: standardized.ID, SourceType: "CSV", SourceRef: "enterprise.csv", SourceRole: ed.SourceAnchor, PolicyRef: "park/matching/company-match-policy-v1.yaml", ActorID: &scope.ActorID})
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := entities.ListCandidates(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	reviewed := 0
	for _, candidate := range candidates {
		if candidate.Status == ed.CandidatePending {
			job, err = matcher.Confirm(ctx, ea.ReviewCommand{CandidateID: candidate.ID, ReviewerID: scope.ActorID, Reason: "controlled native isolation test alias", TraceID: "issue330-native-review"})
			if err != nil {
				t.Fatal(err)
			}
			reviewed++
		}
	}
	if job.Status != ed.JobSucceeded || reviewed != 1 || job.OutputDatasetVersionID == nil {
		t.Fatal("native entity review not completed")
	}
	definition, err := os.ReadFile(filepath.Join(repoRoot, "examples/enterprise-activity/workflow/workflow-v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	wv, err := wa.NewWorkflowVersionService(tx, workflows).Create(ctx, wa.CreateWorkflowVersionCommand{WorkspaceID: scope.WorkspaceID, Code: uuid.NewString(), Name: "scoped frozen workflow", Version: "1.0.0", DefinitionRef: "examples/enterprise-activity/workflow/workflow-v1.yaml", DefinitionYAML: definition, ActorID: &scope.ActorID})
	if err != nil {
		t.Fatal(err)
	}
	service := wa.NewExecutionService(tx, workflows)
	execution, err := service.Create(ctx, wa.CreateExecutionCommand{WorkspaceID: scope.WorkspaceID, WorkflowVersionID: wv.ID, OutputDatasetID: curated.ID, TargetPeriod: "2025-03", Inputs: []wd.InputBinding{{Name: "enterprise_raw", DatasetVersionID: versions["enterprise"].ID}, {Name: "lease_raw", DatasetVersionID: versions["lease"].ID}, {Name: "energy_raw", DatasetVersionID: versions["energy"].ID}, {Name: "enterprise_resolution", DatasetVersionID: *job.OutputDatasetVersionID}}, IdempotencyKey: uuid.NewString(), ActorID: &scope.ActorID})
	if err != nil {
		t.Fatal(err)
	}
	engine := native.NewEngine(filepath.Join(repoRoot, "industry-packs"), tx, datasets, entities, workflows, writer, s, indicator.NewCalculator()).WithInputReader(s)
	handler := wq.NewHandler(service, workflows, engine)
	payload, _ := json.Marshal(map[string]any{"executionId": execution.ID})
	task := asynq.NewTask(wq.TaskExecute, payload)
	if err = handler.Handle(ctx, task); err != nil {
		t.Fatal(err)
	}
	result, err := workflows.GetExecution(ctx, execution.ID)
	if err != nil || result.Status != wd.ExecutionSucceeded || result.OutputDatasetVersionID == nil {
		t.Fatalf("native workflow: %v %+v", err, result)
	}
	for _, id := range []uuid.UUID{*job.OutputDatasetVersionID, *result.OutputDatasetVersionID} {
		v, e := datasets.GetVersion(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.HasPrefix(v.StorageURI, "s3://raw/core/"+scope.WorkspaceID.String()+"/") {
			t.Fatal("derived output missing workspace prefix")
		}
		r, e := s.ReadVersion(ctx, id)
		if e != nil {
			t.Fatal(e)
		}
		data, e := io.ReadAll(r)
		_ = r.Close()
		if e != nil || len(data) == 0 {
			t.Fatal("derived native scoped GET failed")
		}
	}
	var before, after int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_storage_call WHERE workspace_id=$1`, scope.WorkspaceID).Scan(&before)
	if err = handler.Handle(ctx, task); err != nil {
		t.Fatal(err)
	}
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_storage_call WHERE workspace_id=$1`, scope.WorkspaceID).Scan(&after)
	if before != after {
		t.Fatal("succeeded workflow replay invoked storage")
	}
	var inputs, bindings, usage int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM execution_input WHERE execution_id=$1`, execution.ID).Scan(&inputs)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM execution_dependency_binding WHERE execution_id=$1`, execution.ID).Scan(&bindings)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM execution_mapping_usage WHERE execution_id=$1`, execution.ID).Scan(&usage)
	if inputs != 4 || bindings != 3 || usage == 0 {
		t.Fatal("frozen execution dependencies incomplete")
	}
	t.Logf("NATIVE PASS: actual entity review + frozen workflow, same Actor/workspace; %d frozen inputs / %d dependencies / %d mappings; both derived outputs readable with scoped STS", inputs, bindings, usage)
}
