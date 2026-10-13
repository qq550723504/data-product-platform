package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetapp "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	entityapp "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/application"
	entitydomain "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/domain"
	entityinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/entity/infrastructure"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type planeState struct{ StandardizedDatasetID, OutputDatasetID, StandardizedVersionID, WorkflowVersionID, ProfileID uuid.UUID }
type planeCache map[string][]byte

func (s planeCache) Get(_ context.Context, uri string) (io.ReadCloser, error) {
	b, ok := s[uri]
	if !ok {
		return nil, lc.ErrStorageScope
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

type planeOutput struct {
	slots  map[uuid.UUID]lc.ProcessingOutputSlot
	writer *storage.LifecycleOutputWriter
	pins   *lc.FixtureSubjectPins
	source domain.Copy
}

func (s *planeOutput) Put(context.Context, string, io.Reader, int64, string) (string, error) {
	return "", lc.ErrStorageUnsupported
}
func (s *planeOutput) PutVersion(ctx context.Context, v datasetdomain.DatasetVersion, _ string, r io.Reader, n int64, _ string) (string, error) {
	slot, ok := s.slots[v.DatasetID]
	if !ok || n <= 0 || n > 128<<10 {
		return "", lc.ErrStorageScope
	}
	b, e := io.ReadAll(io.LimitReader(r, n+1))
	if e != nil || int64(len(b)) != n {
		return "", lc.ErrStorageContract
	}
	sum := sha256.Sum256(b)
	intent := lc.ProcessingOutputIntent{ID: uuid.New(), WorkspaceID: s.source.WorkspaceID, DatasetVersionID: v.ID, DatasetID: v.DatasetID, NamespaceID: slot.NamespaceID, Namespace: slot.Namespace, Key: slot.Key, ContentSHA256: hex.EncodeToString(sum[:]), ByteSize: n, Source: s.source}
	version, call, e := s.writer.PutPrepared(ctx, intent, b)
	if e != nil {
		if errors.Is(e, lc.ErrStorageUnknown) {
			s.pins.MarkProcessingOutputUnknown(ctx)
		}
		return "", e
	}
	if version == "" || call == uuid.Nil {
		return "", lc.ErrStorageUnknown
	}
	return "s3://" + slot.Namespace + "/" + slot.Key, nil
}

func planeServices(ctx context.Context, c config, pool *pgxpool.Pool, cs []domain.Copy) (*lc.OccupancyService, *lc.CopyService, *lc.FixtureReaderService, *lc.FixtureSubjectPins, error) {
	tx := transaction.NewManager(pool)
	g := &gate{}
	o := lc.NewOccupancyService(tx, &infrastructure.PostgresRepository{}, authority{}, g, nil)
	var ns uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND namespace=$2`, c.WorkspaceID, namespaces[1]).Scan(&ns); e != nil {
		return nil, nil, nil, nil, e
	}
	g.s = lc.NewCopyService(o, []lc.CopyTargetSlot{{Slot: "archive-a", Action: domain.ArchiveCopy, NamespaceID: ns, Namespace: namespaces[1], Key: "archive/a.csv"}}, noCopyProtection{}, nil, c.Epoch)
	p, e := storage.NewLifecycleReader(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: c.Access, SecretKey: c.Secret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: lc.PinnedReaderCallJournal{Service: o, Observations: lc.NewPostgresStorageCallJournal(o), SubjectGate: planeCurrentDeliveryGate(pool)}}, cs)
	if e != nil {
		return nil, nil, nil, nil, e
	}
	reader := lc.NewFixtureReaderService(o, p)
	selection := map[uuid.UUID]uuid.UUID{}
	for _, cp := range cs {
		if (cp.Namespace == namespaces[0] && !(c.ReferenceDemand && cp.Key == "fixture/b.csv")) || (c.ReferenceDemand && cp.Namespace == namespaces[1] && cp.Key == "fixture/b.csv") {
			selection[cp.DatasetVersionID] = cp.ID
		}
	}
	pins := lc.NewFixtureSubjectPins(reader, selection)
	if reader == nil || pins == nil {
		return nil, nil, nil, nil, lc.ErrStorageScope
	}
	return o, g.s, reader, pins, nil
}
func planeWriter(ctx context.Context, c config, pool *pgxpool.Pool, o *lc.OccupancyService, pins *lc.FixtureSubjectPins, cs []domain.Copy, state planeState) (*datasetapp.UploadVersionService, error) {
	var ns uuid.UUID
	if e := pool.QueryRow(ctx, `SELECT id FROM lifecycle_fixture_namespace WHERE workspace_id=$1 AND namespace=$2`, c.WorkspaceID, namespaces[0]).Scan(&ns); e != nil {
		return nil, e
	}
	slots := []lc.ProcessingOutputSlot{{DatasetID: state.StandardizedDatasetID, NamespaceID: ns, Namespace: namespaces[0], Key: "fixture/standardized.csv"}, {DatasetID: state.OutputDatasetID, NamespaceID: ns, Namespace: namespaces[0], Key: "fixture/processed.csv"}}
	journal := lc.FixtureOutputJournal{Service: pins, Slots: slots, Observations: lc.NewPostgresStorageCallJournal(o)}
	writer, e := storage.NewLifecycleOutputWriter(storage.LifecycleCopyConfig{Endpoint: c.Endpoint, AccessKey: c.ProducerAccess, SecretKey: c.ProducerSecret, WorkspaceID: c.WorkspaceID, ProviderInstanceID: c.ProviderInstanceID, IncarnationID: c.IncarnationID, Namespaces: namespaces, Journal: journal})
	if e != nil {
		return nil, e
	}
	store := &planeOutput{slots: map[uuid.UUID]lc.ProcessingOutputSlot{slots[0].DatasetID: slots[0], slots[1].DatasetID: slots[1]}, writer: writer, pins: pins, source: cs[0]}
	return datasetapp.NewUploadVersionService(transaction.NewManager(pool), datasetinfra.NewPostgresRepository(pool), store).WithPublicationLease(journal), nil
}

func planeSetup(ctx context.Context, c config, pool *pgxpool.Pool, cs []domain.Copy) (planeState, error) {
	var state planeState
	tx := transaction.NewManager(pool)
	datasets := datasetinfra.NewPostgresRepository(pool)
	create := datasetapp.NewCreateDatasetService(tx, datasets, resourceinfra.NewPostgresRepository())
	for i, kind := range []datasetdomain.DatasetType{datasetdomain.DatasetTypeStandardized, datasetdomain.DatasetTypeCurated} {
		d, e := create.Handle(ctx, datasetapp.CreateDatasetCommand{WorkspaceID: c.WorkspaceID, Code: fmt.Sprintf("LC307-PLANE-%d", i), Name: "Controlled synthetic processing output", DatasetType: kind, ActorID: &c.ActorID})
		if e != nil {
			return state, e
		}
		if i == 0 {
			state.StandardizedDatasetID = d.ID
		} else {
			state.OutputDatasetID = d.ID
		}
	}
	o, policy, _, pins, e := planeServices(ctx, c, pool, cs)
	if e != nil {
		return state, e
	}
	if _, e = policy.BindFixtureCopyPolicy(ctx, lc.BindCopyPolicyCommand{CommandHeader: lc.CommandHeader{WorkspaceID: c.WorkspaceID, IdempotencyKey: "plane-alpha-policy"}, DatasetVersionID: cs[0].DatasetVersionID, UseUntil: c.ExpiresAt, RestoreUntil: c.ExpiresAt}); e != nil {
		return state, e
	}
	writer, e := planeWriter(ctx, c, pool, o, pins, cs, state)
	if e != nil {
		return state, e
	}
	payloads := pilot.PlaneSyntheticPayloads()
	cache := planeCache{}
	for _, cp := range cs {
		if cp.Namespace != namespaces[0] {
			continue
		}
		idx := 0
		if cp.Key == "fixture/b.csv" {
			idx = 1
		}
		cache["s3://"+cp.Namespace+"/"+cp.Key] = payloads[idx]
	}
	match := entityapp.NewMatchService("/run/repo/industry-packs", tx, entityinfra.NewPostgresRepository(pool), datasets, writer, cache)
	job, e := match.Start(ctx, entityapp.StartJobCommand{WorkspaceID: c.WorkspaceID, InputDatasetVersionID: cs[0].DatasetVersionID, OutputDatasetID: state.StandardizedDatasetID, SourceType: "CSV", SourceRef: "a.csv", SourceRole: entitydomain.SourceAnchor, PolicyRef: "park/matching/company-match-policy-v1.yaml", ActorID: &c.ActorID})
	if e != nil {
		return state, e
	}
	if job.Status != entitydomain.JobSucceeded || job.OutputDatasetVersionID == nil {
		return state, errors.New("synthetic anchor resolution did not finish")
	}
	state.StandardizedVersionID = *job.OutputDatasetVersionID
	definition := []byte("apiVersion: dataprod.platform/v1alpha1\nkind: Workflow\nmetadata:\n  name: lifecycle-synthetic-native\n  version: 1.0.0\nspec:\n  processor: enterprise-activity-native-v1\n")
	version, e := workflowapp.NewWorkflowVersionService(tx, workflowinfra.NewPostgresRepository(pool)).Create(ctx, workflowapp.CreateWorkflowVersionCommand{WorkspaceID: c.WorkspaceID, Code: "LC307-NATIVE", Name: "Synthetic native processing", Version: "1", DefinitionRef: "controlled-synthetic-native", DefinitionYAML: definition, ActorID: &c.ActorID})
	if e != nil {
		return state, e
	}
	state.WorkflowVersionID = version.ID
	state.ProfileID, e = planeCertification(ctx, c, pool, cs, cache)
	if e != nil {
		return state, e
	}
	if e = write(c, "plane-state.json", state); e != nil {
		return state, e
	}
	return state, nil
}

func loadPlaneState(c config) (planeState, error) {
	var s planeState
	b, e := os.ReadFile(filepath.Join(c.OutputDirectory, "plane-state.json"))
	if e != nil || len(b) > 256<<10 {
		return s, errors.New("plane state unavailable")
	}
	e = json.Unmarshal(b, &s)
	return s, e
}
