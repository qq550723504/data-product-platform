// poc-demo prepares synthetic data on the isolated Compose demo only.
// It never confirms entity candidates or publishes a ProductRelease for the operator.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	complianceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/application"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productapp "github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	productdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	qualityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/application"
	qualityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourcedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/traceability"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

const statePath = "/demo-state/manifest.json"
const policy = "park/matching/company-match-policy-v1.yaml"
const purpose = "ENTERPRISE_CREDIT_RISK_SUPPORT"

type manifest struct {
	Schema       int                           `json:"schema"`
	Stage        string                        `json:"stage"`
	Workspace    uuid.UUID                     `json:"workspaceId"`
	Actor        uuid.UUID                     `json:"seedActorId"`
	Reviewer     uuid.UUID                     `json:"reviewerId"`
	Publisher    uuid.UUID                     `json:"publisherId"`
	Resources    []uuid.UUID                   `json:"resourceIds"`
	Inputs       []workflowdomain.InputBinding `json:"inputs"`
	Curated      uuid.UUID                     `json:"curatedDatasetId"`
	Job          uuid.UUID                     `json:"jobId"`
	Workflow     uuid.UUID                     `json:"workflowVersionId"`
	Execution    uuid.UUID                     `json:"executionId"`
	Product      uuid.UUID                     `json:"productId"`
	Release      uuid.UUID                     `json:"releaseId"`
	Blocked      uuid.UUID                     `json:"blockedReleaseId"`
	RightsExpiry time.Time                     `json:"rightsExpireAt"`
}

type demo struct {
	ctx   context.Context
	cfg   config.Config
	pool  *pgxpool.Pool
	store *storage.Store
	m     manifest
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "poc-demo:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || !allowedCommand(os.Args[1]) {
		return errors.New("usage: poc-demo prepare|advance|status|verify")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := validateConfig(cfg, os.Getenv("POC_DEMO_ACK")); err != nil {
		return err
	}
	// This demo runs with every external engine disabled, so its routing profile
	// is the explicit retention-only one. Freeze the obligation on events as they
	// are recorded instead of letting a dispatcher guess later.
	obligationRouter, err := routing.NewRouter(cfg.OpenMetadata.Enabled)
	if err != nil {
		return err
	}
	outbox.ConfigureAppendObligation(obligationRouter)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var locked bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(764393001)").Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return errors.New("another demo command is running; no duplicate work was started")
	}
	defer func() { _, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(764393001)") }()
	store, err := storage.New(cfg.Storage.Endpoint, cfg.Storage.AccessKey, cfg.Storage.SecretKey, cfg.Storage.Bucket, cfg.Storage.UseSSL)
	if err != nil {
		return err
	}
	d := demo{ctx: ctx, cfg: cfg, pool: pool, store: store}
	data, err := os.ReadFile(statePath)
	if err == nil {
		if err := json.Unmarshal(data, &d.m); err != nil {
			return fmt.Errorf("invalid demo manifest; do not reseed automatically: %w", err)
		}
		if d.m.Schema != 1 || d.m.Workspace == uuid.Nil || d.m.Reviewer == uuid.Nil || d.m.Publisher == uuid.Nil {
			return errors.New("unsupported/incomplete demo manifest")
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if os.Args[1] != "prepare" {
		return errors.New("demo not initialized; run up first")
	}
	if d.m.Stage == "INITIALIZING" || d.m.Stage == "ADVANCING" {
		return errors.New("previous preparation did not finish; inspect logs, then explicitly reset this synthetic demo; no blind retry")
	}
	switch os.Args[1] {
	case "prepare":
		if d.m.Schema == 0 {
			if err := d.prepare(); err != nil {
				return err
			}
		}
	case "advance":
		if d.m.Stage == "REVIEW" {
			if err := d.advance(); err != nil {
				return err
			}
		}
	case "verify":
		return d.verify()
	}
	return d.status()
}

func allowedCommand(command string) bool {
	return command == "prepare" || command == "advance" || command == "status" || command == "verify"
}

func validateConfig(cfg config.Config, ack string) error {
	u, err := url.Parse(cfg.PostgresDSN)
	if err != nil || u.Scheme != "postgres" || u.Host != "postgres:5432" || u.Path != "/dpp_demo" {
		return errors.New("only the dedicated Compose dpp_demo database is allowed")
	}
	if ack != "SYNTHETIC_LOCAL_ONLY" || cfg.Environment != "poc-demo" || cfg.Redis.Addr != "redis:6379" || cfg.Redis.DB != 0 || cfg.Storage.Endpoint != "minio:9000" || cfg.Storage.Bucket != "dpp-demo" || cfg.Storage.UseSSL || cfg.OpenMetadata.Enabled || cfg.Hop.Enabled || cfg.Splink.Enabled {
		return errors.New("refusing non-demo service configuration")
	}
	return nil
}

func (d *demo) save(stage string) error {
	d.m.Stage = stage
	b, err := json.MarshalIndent(d.m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(statePath+".tmp", b, 0644); err != nil {
		return err
	}
	return os.Rename(statePath+".tmp", statePath)
}

func (d *demo) fixture(parts ...string) ([]byte, error) {
	return os.ReadFile(filepath.Join(append([]string{filepath.Dir(d.cfg.IndustryPackRoot), "examples", "enterprise-activity"}, parts...)...))
}

func (d *demo) prepare() error {
	var count int
	if err := d.pool.QueryRow(d.ctx, "SELECT (SELECT count(*) FROM data_resource)+(SELECT count(*) FROM dataset)+(SELECT count(*) FROM data_product)").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("manifest missing but business database is nonempty; refusing to seed")
	}
	d.m = manifest{Schema: 1, Workspace: uuid.New(), Actor: uuid.New(), Reviewer: uuid.New(), Publisher: uuid.New()}
	if err := d.save("INITIALIZING"); err != nil {
		return err
	}
	if err := d.store.EnsureBucket(d.ctx); err != nil {
		return err
	}
	tx := transaction.NewManager(d.pool)
	resources := resourceapp.NewCreateService(tx, resourceinfra.NewPostgresRepository())
	datasets := datasetinfra.NewPostgresRepository(d.pool)
	create := datasetapp.NewCreateDatasetService(tx, datasets)
	upload := datasetapp.NewUploadVersionService(tx, datasets, d.store)
	var source string
	for _, name := range []string{"enterprise", "lease", "energy"} {
		r, err := resources.Handle(d.ctx, resourceapp.CreateDataResourceCommand{WorkspaceID: d.m.Workspace, Code: "DEMO-" + name, Name: "合成演示来源 / " + name, DomainCode: "PARK_ENTERPRISE_ACTIVITY", ResourceType: resourcedomain.ResourceTypeTableLike, SensitivityLevel: "INTERNAL", ActorID: &d.m.Actor})
		if err != nil {
			return err
		}
		d.m.Resources = append(d.m.Resources, r.ID)
		ds, err := create.Handle(d.ctx, datasetapp.CreateDatasetCommand{WorkspaceID: d.m.Workspace, Code: "DEMO-RAW-" + name, Name: "合成演示 RAW / " + name, DatasetType: datasetdomain.DatasetTypeRaw, SourceResourceID: &r.ID, ActorID: &d.m.Actor})
		if err != nil {
			return err
		}
		content, err := d.fixture("data", name+".csv")
		if err != nil {
			return err
		}
		filename := name + "-" + d.m.Workspace.String() + ".csv"
		v, err := upload.Handle(d.ctx, datasetapp.UploadVersionCommand{DatasetID: ds.ID, Filename: filename, ContentType: "text/csv; charset=utf-8", Content: content, ActorID: &d.m.Actor})
		if err != nil {
			return err
		}
		d.m.Inputs = append(d.m.Inputs, workflowdomain.InputBinding{Name: name + "_raw", DatasetVersionID: v.ID})
		if name == "enterprise" {
			source = filename
		}
	}
	standard, err := create.Handle(d.ctx, datasetapp.CreateDatasetCommand{WorkspaceID: d.m.Workspace, Code: "DEMO-STANDARDIZED", Name: "人工审核后的实体解析版本", DatasetType: datasetdomain.DatasetTypeStandardized, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	curated, err := create.Handle(d.ctx, datasetapp.CreateDatasetCommand{WorkspaceID: d.m.Workspace, Code: "DEMO-CURATED", Name: "合成经营活跃度数据", DatasetType: datasetdomain.DatasetTypeCurated, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	d.m.Curated = curated.ID
	entity := entityapp.NewMatchService(d.cfg.IndustryPackRoot, tx, entityinfra.NewPostgresRepository(d.pool), datasets, upload, d.store)
	job, err := entity.Start(d.ctx, entityapp.StartJobCommand{WorkspaceID: d.m.Workspace, InputDatasetVersionID: d.m.Inputs[0].DatasetVersionID, OutputDatasetID: standard.ID, SourceType: "CSV", SourceRef: source, SourceRole: entitydomain.SourceAnchor, PolicyRef: policy, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	if job.Status != entitydomain.JobWaitingReview {
		return fmt.Errorf("expected manual review, got %s", job.Status)
	}
	d.m.Job = job.ID
	return d.save("REVIEW")
}

func (d *demo) advance() error {
	job, err := entityinfra.NewPostgresRepository(d.pool).GetJob(d.ctx, d.m.Job)
	if err != nil {
		return err
	}
	if job.WorkspaceID != d.m.Workspace || job.Status != entitydomain.JobSucceeded || job.OutputDatasetVersionID == nil {
		return fmt.Errorf("complete the browser review first (job=%s); nothing was enqueued", job.Status)
	}
	if err := d.save("ADVANCING"); err != nil {
		return err
	}
	tx := transaction.NewManager(d.pool)
	datasets := datasetinfra.NewPostgresRepository(d.pool)
	workflows := workflowinfra.NewPostgresRepository(d.pool)
	definition, err := d.fixture("workflow", "workflow-v1.yaml")
	if err != nil {
		return err
	}
	wf, err := workflowapp.NewWorkflowVersionService(tx, workflows).Create(d.ctx, workflowapp.CreateWorkflowVersionCommand{WorkspaceID: d.m.Workspace, Code: "DEMO-ACTIVITY", Name: "合成样本原生加工", Version: "1.0.0", DefinitionRef: "examples/enterprise-activity/workflow/workflow-v1.yaml", DefinitionYAML: definition, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	d.m.Workflow = wf.ID
	execution, err := workflowapp.NewExecutionService(tx, workflows).Create(d.ctx, workflowapp.CreateExecutionCommand{WorkspaceID: d.m.Workspace, WorkflowVersionID: wf.ID, OutputDatasetID: d.m.Curated, TargetPeriod: "2025-03", Inputs: d.m.Inputs, IdempotencyKey: "poc-demo-create-" + wf.ID.String(), ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	d.m.Execution = execution.ID
	if err := d.save("ADVANCING"); err != nil {
		return err
	}
	for execution.Status != workflowdomain.ExecutionSucceeded {
		if execution.Status == workflowdomain.ExecutionFailed {
			return errors.New("native Worker failed; inspect logs; no automatic duplicate enqueue")
		}
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
		execution, err = workflows.GetExecution(d.ctx, execution.ID)
		if err != nil {
			return err
		}
	}
	if execution.OutputDatasetVersionID == nil {
		return errors.New("successful execution has no output version")
	}
	output, err := datasets.GetVersion(d.ctx, *execution.OutputDatasetVersionID)
	if err != nil {
		return err
	}
	quality, err := qualityapp.NewService(d.cfg.IndustryPackRoot, tx, datasets, qualityinfra.NewPostgresRepository(d.pool), d.store).Run(d.ctx, qualityapp.RunCommand{WorkspaceID: d.m.Workspace, DatasetVersionID: output.ID, RuleSetRef: "park/quality/enterprise-activity-quality-v1.yaml", ActorID: &d.m.Actor, Now: time.Now().UTC()})
	if err != nil {
		return err
	}
	compliance, err := complianceapp.NewService(d.cfg.IndustryPackRoot, tx, datasets, complianceinfra.NewPostgresRepository(d.pool), d.store).Run(d.ctx, complianceapp.RunCommand{WorkspaceID: d.m.Workspace, DatasetVersionID: output.ID, PolicyRef: "park/compliance/enterprise-activity-compliance-v1.yaml", ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	contracts := contractapp.NewService(tx, contractinfra.NewPostgresRepository(d.pool))
	document, err := d.fixture("contract", "data-contract-v1.yaml")
	if err != nil {
		return err
	}
	contract, err := contracts.CreateVersionFromYAML(d.ctx, contractapp.CreateVersionFromYAMLCommand{WorkspaceID: d.m.Workspace, SourceRef: "examples/enterprise-activity/contract/data-contract-v1.yaml", DocumentYAML: document, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	contract, err = contracts.PublishVersion(d.ctx, contractapp.PublishVersionCommand{VersionID: contract.ID, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	rights := rightsapp.NewService(tx, rightsinfra.NewPostgresRepository(d.pool))
	grants := make([]rightsdomain.ResourceGrantSpec, 0, len(d.m.Resources))
	for _, id := range d.m.Resources {
		grants = append(grants, rightsdomain.ResourceGrantSpec{DataResourceID: id, Actions: []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"}, Scope: map[string]any{"useCase": purpose}})
	}
	from := time.Now().UTC().Add(-time.Minute)
	d.m.RightsExpiry = time.Now().UTC().Add(7 * 24 * time.Hour)
	auth, err := rights.Create(d.ctx, rightsapp.CreateAuthorizationCommand{WorkspaceID: d.m.Workspace, Code: "DEMO-AUTH", GrantorRef: "SYNTHETIC-PARK-OPERATOR", GranteeRef: "DATA-PRODUCT-PLATFORM", Purpose: purpose, ValidFrom: &from, ValidTo: &d.m.RightsExpiry, Resources: grants, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	transition := rightsapp.TransitionCommand{AuthorizationID: auth.ID, ActorID: &d.m.Actor, At: time.Now().UTC()}
	if _, err = rights.Submit(d.ctx, transition); err != nil {
		return err
	}
	if _, err = rights.Approve(d.ctx, transition); err != nil {
		return err
	}
	if _, err = rights.Activate(d.ctx, transition); err != nil {
		return err
	}
	products := productapp.NewService(tx, productinfra.NewPostgresRepository(d.pool))
	product, err := products.CreateProduct(d.ctx, productapp.CreateProductCommand{WorkspaceID: d.m.Workspace, Code: "DEMO-ACTIVITY", Name: "合成演示：企业经营活跃度", Description: "仅合成样本；不代表真实企业评价", DomainCode: "PARK_ENTERPRISE_ACTIVITY", ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	d.m.Product = product.ID
	version, err := products.CreateVersion(d.ctx, productapp.CreateVersionCommand{ProductID: product.ID, MajorVersion: 1, WorkflowVersionID: &wf.ID, ContractVersionID: &contract.ID, EntityPolicyRef: policy, IndicatorSetRef: "park-enterprise-activity@1.0.0", Definition: map[string]any{"syntheticDemo": true}, Assets: []productdomain.AssetSpec{{AssetType: productdomain.AssetDataset, Name: "enterprise_activity_curated", DatasetID: &d.m.Curated, DeliveryConfig: map[string]any{"mode": "DATASET", "rawExport": false}}}, ActorID: &d.m.Actor})
	if err != nil {
		return err
	}
	for _, number := range []string{"R-DEMO-READY", "R-DEMO-BLOCKED"} {
		release, err := products.CreateRelease(d.ctx, productapp.CreateReleaseCommand{ProductID: product.ID, ProductVersionID: version.ID, ReleaseNo: number, Datasets: []productdomain.ReleaseDataset{{DatasetVersionID: output.ID, Role: productdomain.DatasetPrimary}}, ActorID: &d.m.Actor})
		if err != nil {
			return err
		}
		if number == "R-DEMO-BLOCKED" {
			d.m.Blocked = release.ID
			continue
		}
		d.m.Release = release.ID
		snapshot, err := rights.CreateSnapshot(d.ctx, rightsapp.CreateSnapshotCommand{WorkspaceID: d.m.Workspace, ProductReleaseID: &release.ID, Purpose: purpose, ConsumerRef: "LICENSED_BANK", AsOf: time.Now().UTC(), AuthorizationIDs: []uuid.UUID{auth.ID}, ActorID: &d.m.Actor})
		if err != nil {
			return err
		}
		result, err := products.ValidateRelease(d.ctx, productapp.ValidateReleaseCommand{ReleaseID: release.ID, ContractVersionID: contract.ID, RightsSnapshotID: snapshot.ID, QualityResultID: quality.ID, ComplianceResultID: compliance.ID, ActorID: &d.m.Actor})
		if err != nil {
			return err
		}
		if result.Overall != "READY" {
			return fmt.Errorf("Core readiness blocks the demo: %v", result.Blockers)
		}
	}
	return d.save("READY")
}

func (d *demo) status() error {
	job, err := entityinfra.NewPostgresRepository(d.pool).GetJob(d.ctx, d.m.Job)
	if err != nil {
		return err
	}
	if job.WorkspaceID != d.m.Workspace {
		return errors.New("manifest and database workspace differ")
	}
	counts := map[string]int{}
	for _, table := range []string{"data_resource", "dataset", "data_product", "execution"} {
		var count int
		if err := d.pool.QueryRow(d.ctx, "SELECT count(*) FROM "+table+" WHERE workspace_id=$1", d.m.Workspace).Scan(&count); err != nil {
			return err
		}
		counts[table] = count
	}
	result := map[string]any{"manifest": d.m, "jobStatus": job.Status, "counts": counts, "reviewURL": "http://127.0.0.1:3180/reviews"}
	if d.m.Release != uuid.Nil {
		r, err := productinfra.NewPostgresRepository(d.pool).GetRelease(d.ctx, d.m.Release)
		if err != nil {
			return err
		}
		result["releaseStatus"] = r.Status
		result["productURL"] = "http://127.0.0.1:3180/products/" + d.m.Product.String()
		result["traceURL"] = result["productURL"].(string) + "/releases/" + d.m.Release.String()
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func (d *demo) verify() error {
	if d.m.Release == uuid.Nil {
		return errors.New("no release yet; complete review and run advance")
	}
	trace, err := traceability.NewRepository(d.pool).ProductRelease(d.ctx, d.m.Release)
	if err != nil {
		return err
	}
	if trace.Status != "PUBLISHED" || trace.ProductID != d.m.Product || trace.EvidenceSnapshot == nil || !trace.EvidenceSnapshot.IntegrityValid || len(trace.DatasetVersions) != 5 || len(trace.Executions) != 1 || trace.Executions[0].ID != d.m.Execution {
		return errors.New("publish in the browser first; expected persisted release, snapshot and five-version lineage")
	}
	for _, v := range trace.DatasetVersions {
		r, err := d.store.Get(d.ctx, v.StorageURI)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, copyErr := io.Copy(h, r)
		closeErr := r.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), v.ChecksumValue) {
			return fmt.Errorf("object checksum mismatch: %s", v.ID)
		}
	}
	for _, e := range trace.Evidence {
		if !e.IntegrityValid {
			return fmt.Errorf("evidence integrity failure: %s", e.ID)
		}
	}
	var publications, auditCount, reviews int
	if err := d.pool.QueryRow(d.ctx, "SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='ProductReleased'", d.m.Release).Scan(&publications); err != nil {
		return err
	}
	if err := d.pool.QueryRow(d.ctx, "SELECT count(*) FROM audit_event WHERE workspace_id=$1 AND object_id=$2 AND action='PRODUCT_RELEASE_PUBLISHED' AND actor_id=$3", d.m.Workspace, d.m.Release, d.m.Publisher).Scan(&auditCount); err != nil {
		return err
	}
	for _, m := range trace.EntityMappings {
		if m.ReviewedBy != nil && *m.ReviewedBy == d.m.Reviewer && strings.TrimSpace(m.ReviewerReason) != "" && m.EvidenceID != nil {
			reviews++
		}
	}
	if publications != 1 || auditCount != 1 || reviews == 0 || len(trace.CostEvents) == 0 {
		return errors.New("publish audit/event, reviewer reason or cost facts missing/duplicated")
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"verified": true, "releaseId": d.m.Release, "snapshotId": trace.EvidenceSnapshot.ID, "rootHash": trace.EvidenceSnapshot.RootHash, "checksummedVersions": 5, "reviewedMappings": reviews, "publishEvents": publications, "publishAudits": auditCount})
}
