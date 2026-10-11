package application_test

import (
	"bytes"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	ingestionapp "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/application"
	ingestiondomain "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
	ingestioninfra "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/principal"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceapp "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/application"
	resourcedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	workflowdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Controlled provider bytes; real existing ingestion admission and entity
// Commands construct their owner facts. No receipt/job success rows are seeded.
type batchEngine struct {
	store   *memoryStore
	content map[string][]byte
}

func (e batchEngine) Version() string                                      { return "c1-batch-test" }
func (e batchEngine) Check(context.Context, ingestiondomain.Request) error { return nil }
func (e batchEngine) Register(_ context.Context, _ ingestiondomain.Operation, notify func()) (string, ingestiondomain.DispatchState, error) {
	notify()
	return "controlled-run", ingestiondomain.Dispatched, nil
}
func (e batchEngine) Start(_ context.Context, _ ingestiondomain.Operation, notify func()) (ingestiondomain.DispatchState, error) {
	notify()
	return ingestiondomain.Dispatched, nil
}
func (e batchEngine) Status(_ context.Context, _ ingestiondomain.Operation, notify func()) (string, ingestiondomain.DispatchState, error) {
	notify()
	return "SUCCEEDED", ingestiondomain.Dispatched, nil
}
func (e batchEngine) Manifest(_ context.Context, op ingestiondomain.Operation) (ingestiondomain.Manifest, error) {
	r := op.Request
	m := ingestiondomain.Manifest{Contract: ingestiondomain.Contract, OperationID: op.ID, AttemptID: op.AttemptID, WorkspaceID: r.WorkspaceID, RequestKey: r.RequestKey, BatchKey: r.BatchKey, ConnectionRef: r.ConnectionRef, InstanceRef: r.InstanceRef, ConfigHash: r.ConfigHash, TemplateHash: r.TemplateHash, SchemaHash: r.SchemaHash, EngineVersion: e.Version(), RemoteID: op.RemoteID, ObservedAt: time.Now().UTC()}
	for _, slot := range r.Slots {
		data := e.content[slot.Code]
		uri := ingestiondomain.Prefix("c1-test", op) + slot.Code + ".csv"
		e.store.objects[uri] = data
		m.Outputs = append(m.Outputs, ingestiondomain.Output{Code: slot.Code, DatasetID: slot.DatasetID, URI: uri, Bytes: int64(len(data)), Rows: slot.Rows, SHA256: ingestiondomain.Hash(data), Columns: slot.Columns})
	}
	return m, nil
}
func (s *memoryStore) PutImmutable(ctx context.Context, name string, r io.Reader, n int64, kind string) (string, error) {
	return s.Put(ctx, name, r, n, kind)
}

func acceptedBatch(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tx *transaction.Manager, ds *datasetinfra.PostgresRepository, store *memoryStore, creator *datasetapp.CreateDatasetService, writer *datasetapp.UploadVersionService, root string, ws, actor uuid.UUID) (datasetdomain.Dataset, datasetdomain.DatasetVersion, []workflowdomain.InputBinding) {
	t.Helper()
	content := map[string][]byte{
		"energy_raw":     []byte("meter_id,energy_kwh\nM-001,10\nM-005,-999\n"),
		"lease_raw":      []byte("lease_id\nL-001\n"),
		"enterprise_raw": []byte("source_company_id,company_name,unified_social_credit_code,legal_representative,registered_address,entry_date,company_status\nENT-001,C1 Company,91440300C100000001,C1 Person,C1 Address,2022-01-01,ACTIVE\n"),
	}
	request := ingestiondomain.Request{WorkspaceID: ws, ActorID: actor, PrincipalRef: "c1-test-operator", RequestKey: uuid.NewString(), BatchKey: uuid.NewString(), ConnectionRef: "c1-test-source", InstanceRef: "c1-test-instance", ConfigHash: strings.Repeat("a", 64), TemplateHash: strings.Repeat("b", 64), SchemaHash: strings.Repeat("c", 64)}
	var raw datasetdomain.Dataset
	for _, name := range []string{"enterprise_raw", "lease_raw", "energy_raw"} {
		resource, e := resourceapp.NewCreateService(tx, resourceinfra.NewPostgresRepository()).Handle(ctx, resourceapp.CreateDataResourceCommand{WorkspaceID: ws, Code: uuid.NewString(), Name: name, ResourceType: resourcedomain.ResourceTypeTableLike})
		if e != nil {
			t.Fatal(e)
		}
		d, e := creator.Handle(ctx, datasetapp.CreateDatasetCommand{WorkspaceID: ws, Code: uuid.NewString(), Name: name, DatasetType: datasetdomain.DatasetTypeRaw, SourceResourceID: &resource.ID})
		if e != nil {
			t.Fatal(e)
		}
		if name == "energy_raw" {
			raw = d
		}
		columns := strings.Split(strings.SplitN(string(content[name]), "\n", 2)[0], ",")
		rows := int64(bytes.Count(content[name], []byte("\n")) - 1)
		request.Slots = append(request.Slots, ingestiondomain.Slot{Code: name, DatasetID: d.ID, ResourceID: resource.ID, Columns: columns, Rows: rows})
	}
	caller := func(context.Context, uuid.UUID) (principal.RequestPrincipal, error) {
		return principal.RequestPrincipal{WorkspaceID: ws, ActorID: actor, Subject: request.PrincipalRef, Capabilities: map[string]struct{}{"INGEST_RAW": {}}}, nil
	}
	service := ingestionapp.New(tx, ingestioninfra.New(pool), ds, batchEngine{store, content}, store, "c1-test")
	service.TargetGuard = func(_ context.Context, _ pgx.Tx, r ingestiondomain.Request) error {
		a, _ := r.Fingerprint()
		b, _ := request.Fingerprint()
		if a != b {
			return ingestiondomain.ErrDenied
		}
		return nil
	}
	op, e := service.Prepare(ctx, request, caller)
	if e != nil {
		t.Fatal(e)
	}
	op, e = service.Dispatch(ctx, op.ID, caller)
	if e != nil {
		t.Fatal(e)
	}
	op, e = service.Recover(ctx, op.ID, caller)
	if e != nil {
		t.Fatal(e)
	}
	receipt, e := service.Admit(ctx, op.ID, *op.Manifest, caller)
	if e != nil {
		t.Fatal(e)
	}
	inputs := []workflowdomain.InputBinding{}
	var input datasetdomain.DatasetVersion
	var anchor uuid.UUID
	for _, b := range receipt.Bindings {
		inputs = append(inputs, workflowdomain.InputBinding{Name: b.Code, DatasetVersionID: b.VersionID})
		if b.Code == "energy_raw" {
			input, e = ds.GetVersion(ctx, b.VersionID)
			if e != nil {
				t.Fatal(e)
			}
		}
		if b.Code == "enterprise_raw" {
			anchor = b.VersionID
		}
	}
	resolution, e := creator.Handle(ctx, datasetapp.CreateDatasetCommand{WorkspaceID: ws, Code: uuid.NewString(), Name: "C1 resolution", DatasetType: datasetdomain.DatasetTypeStandardized})
	if e != nil {
		t.Fatal(e)
	}
	job, e := entityapp.NewMatchService(filepath.Join(root, "industry-packs"), tx, entityinfra.NewPostgresRepository(pool), ds, writer, store).Start(ctx, entityapp.StartJobCommand{WorkspaceID: ws, InputDatasetVersionID: anchor, OutputDatasetID: resolution.ID, SourceType: "CSV", SourceRef: "c1-test-anchor", SourceRole: entitydomain.SourceAnchor, PolicyRef: "park/matching/company-match-policy-v1.yaml", ActorID: &actor})
	if e != nil || job.Status != entitydomain.JobSucceeded || job.OutputDatasetVersionID == nil {
		t.Fatal("real entity resolution owner did not finalize", e, job.Status)
	}
	inputs = append(inputs, workflowdomain.InputBinding{Name: "enterprise_resolution", DatasetVersionID: *job.OutputDatasetVersionID})
	return raw, input, inputs
}
