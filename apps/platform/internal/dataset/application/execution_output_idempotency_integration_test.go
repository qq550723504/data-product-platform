package application_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// failingStore refuses the object write, which is how a run interrupts between
// "row allocated" and "row published" (the C2 half-product window).
type failingStore struct{}

func (failingStore) Put(context.Context, string, io.Reader, int64, string) (string, error) {
	return "", errors.New("object store unavailable")
}

// recordingStore is an in-memory object store that records what was actually
// written at every key, so a test can prove the published object still holds the
// bytes its checksum claims.
type recordingStore struct {
	mu      sync.Mutex
	objects map[string][]byte
	puts    int
}

func newRecordingStore() *recordingStore {
	return &recordingStore{objects: map[string][]byte{}}
}

func (s *recordingStore) Put(_ context.Context, objectName string, reader io.Reader, size int64, _ string) (string, error) {
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if int64(len(content)) != size {
		return "", fmt.Errorf("size mismatch: got %d want %d", len(content), size)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[objectName] = content
	s.puts++
	return "s3://test-bucket/" + objectName, nil
}

// read returns the bytes currently stored at a storage URI.
func (s *recordingStore) read(uri string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, ok := s.objects[strings.TrimPrefix(uri, "s3://test-bucket/")]
	return content, ok
}

// gateStore blocks its Put until the test releases it, which makes the window
// between "this delivery allocated the shared output row" and "this delivery
// committed it" deterministic instead of timing-dependent.
type gateStore struct {
	inner   *recordingStore
	entered chan struct{}
	release chan struct{}
}

func newGateStore(inner *recordingStore) *gateStore {
	return &gateStore{inner: inner, entered: make(chan struct{}, 1), release: make(chan struct{})}
}

func (s *gateStore) Put(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	select {
	case s.entered <- struct{}{}:
	default:
	}
	<-s.release
	return s.inner.Put(ctx, objectName, reader, size, contentType)
}

// c2aFixture wires the real writer against a real PostgreSQL, matching how the
// native engine consumes it.
type c2aFixture struct {
	pool   *pgxpool.Pool
	upload *application.UploadVersionService
	repo   *infrastructure.PostgresRepository
	ctx    context.Context
}

// newC2AFixture provisions a dedicated CURATED dataset. Every assertion is scoped
// to this workspace because other packages share TEST_POSTGRES_DSN.
func newC2AFixture(t *testing.T, store application.ObjectStore) (c2aFixture, uuid.UUID, uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	txManager := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	createDataset := application.NewCreateDatasetService(txManager, repo, nil)

	workspaceID := uuid.New()
	dataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "C2A-CURATED-" + uuid.NewString(),
		Name:        "C2-a CURATED output",
		DatasetType: domain.DatasetTypeCurated,
		TraceID:     "c2a-test",
	})
	if err != nil {
		t.Fatalf("create CURATED dataset: %v", err)
	}
	return c2aFixture{
		pool:   pool,
		upload: application.NewUploadVersionService(txManager, repo, store),
		repo:   repo,
		ctx:    ctx,
	}, workspaceID, dataset.ID
}

func (f c2aFixture) outputCommand(datasetID, executionID uuid.UUID, body string) application.UploadVersionCommand {
	return application.UploadVersionCommand{
		DatasetID:              datasetID,
		Filename:               "enterprise-activity.csv",
		ContentType:            "text/csv",
		Content:                []byte(body),
		TraceID:                executionID.String(),
		GeneratedByExecutionID: &executionID,
		Metadata:               map[string]any{"workflowVersionId": uuid.NewString()},
	}
}

func (f c2aFixture) countVersions(t *testing.T, datasetID, executionID uuid.UUID) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(f.ctx, `
		SELECT count(*) FROM dataset_version
		WHERE dataset_id=$1 AND generated_by_execution_id=$2
	`, datasetID, executionID).Scan(&count); err != nil {
		t.Fatalf("count output versions: %v", err)
	}
	return count
}

// TestExecutionOutputReplayPublishesExactlyOneVersion is the core C2-a guarantee:
// a redelivered output write is not a second DatasetVersion, and it does not
// duplicate the business facts that announce the version.
func TestExecutionOutputReplayPublishesExactlyOneVersion(t *testing.T) {
	fixture, _, datasetID := newC2AFixture(t, fakeStore{})
	executionID := uuid.New()
	// Two different payloads prove the idempotency key - not the bytes - decides
	// identity: the first published content wins and is never rewritten.
	first, err := fixture.upload.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n1,alpha\n"))
	if err != nil {
		t.Fatalf("first output write: %v", err)
	}
	if first.Status != domain.VersionReady || first.VersionNo != 1 {
		t.Fatalf("first output = version %d status %s, want 1 READY", first.VersionNo, first.Status)
	}
	if first.GeneratedByExecutionID == nil || *first.GeneratedByExecutionID != executionID {
		t.Fatalf("first output is not bound to the execution: %#v", first.GeneratedByExecutionID)
	}

	second, err := fixture.upload.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n9,omega\n"))
	if err != nil {
		t.Fatalf("replayed output write: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay created a new version: %s vs %s", second.ID, first.ID)
	}
	if second.VersionNo != first.VersionNo {
		t.Fatalf("replay changed version_no: %d vs %d", second.VersionNo, first.VersionNo)
	}
	if second.ChecksumValue != first.ChecksumValue {
		t.Fatalf("replay rewrote published content: %s vs %s", second.ChecksumValue, first.ChecksumValue)
	}
	if count := fixture.countVersions(t, datasetID, executionID); count != 1 {
		t.Fatalf("output versions for one execution = %d, want 1", count)
	}

	var maxVersion int64
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT max(version_no) FROM dataset_version WHERE dataset_id=$1`, datasetID).Scan(&maxVersion); err != nil {
		t.Fatalf("read max version: %v", err)
	}
	if maxVersion != 1 {
		t.Fatalf("dataset version numbers = up to %d, want 1 (replay must not consume a number)", maxVersion)
	}

	// One published fact per business action, even though Handle ran twice.
	assertSingle(t, fixture, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='DatasetVersionCreated'`, first.ID, "DatasetVersionCreated outbox events")
	assertSingle(t, fixture, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='DATASET_VERSION_READY'`, first.ID, "DATASET_VERSION_READY audit events")
	assertSingle(t, fixture, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='DATASET_VERSION_CREATED'`, first.ID, "DATASET_VERSION_CREATED audit events")

	var current *uuid.UUID
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT current_version_id FROM dataset WHERE id=$1`, datasetID).Scan(&current); err != nil {
		t.Fatalf("read current version: %v", err)
	}
	if current == nil || *current != first.ID {
		t.Fatalf("dataset current_version_id = %v, want %s", current, first.ID)
	}
}

// TestInterruptedOutputIsRepairedWithoutConsumingANewVersionNumber covers the
// half-product window: the row exists and is FAILED, the object write never
// succeeded. Recovery must repair that row instead of allocating version 2.
func TestInterruptedOutputIsRepairedWithoutConsumingANewVersionNumber(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()
	txManager := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	createDataset := application.NewCreateDatasetService(txManager, repo, nil)

	workspaceID := uuid.New()
	dataset, err := createDataset.Handle(ctx, application.CreateDatasetCommand{
		WorkspaceID: workspaceID,
		Code:        "C2A-HALF-" + uuid.NewString(),
		Name:        "C2-a half product",
		DatasetType: domain.DatasetTypeCurated,
		TraceID:     "c2a-test",
	})
	if err != nil {
		t.Fatalf("create CURATED dataset: %v", err)
	}

	executionID := uuid.New()
	broken := application.NewUploadVersionService(txManager, repo, failingStore{})
	command := application.UploadVersionCommand{
		DatasetID:              dataset.ID,
		Filename:               "enterprise-activity.csv",
		ContentType:            "text/csv",
		Content:                []byte("id,name\n1,alpha\n"),
		TraceID:                executionID.String(),
		GeneratedByExecutionID: &executionID,
	}
	if _, err := broken.Handle(ctx, command); err == nil {
		t.Fatal("expected the interrupted object write to fail")
	}

	var halfProductID uuid.UUID
	var halfProductNo int64
	var halfProductStatus domain.VersionStatus
	if err := pool.QueryRow(ctx, `
		SELECT id, version_no, status FROM dataset_version
		WHERE dataset_id=$1 AND generated_by_execution_id=$2
	`, dataset.ID, executionID).Scan(&halfProductID, &halfProductNo, &halfProductStatus); err != nil {
		t.Fatalf("read half-product row: %v", err)
	}
	if halfProductStatus != domain.VersionFailed {
		t.Fatalf("half-product status = %s, want FAILED", halfProductStatus)
	}

	// Recovery reruns the same output write with a working store.
	recovered := application.NewUploadVersionService(txManager, repo, fakeStore{})
	version, err := recovered.Handle(ctx, command)
	if err != nil {
		t.Fatalf("repair half-product: %v", err)
	}
	if version.ID != halfProductID {
		t.Fatalf("repair allocated a new row: %s vs %s", version.ID, halfProductID)
	}
	if version.VersionNo != halfProductNo {
		t.Fatalf("repair changed version_no: %d vs %d", version.VersionNo, halfProductNo)
	}
	if version.Status != domain.VersionReady {
		t.Fatalf("repaired status = %s, want READY", version.Status)
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM dataset_version WHERE dataset_id=$1`, dataset.ID).Scan(&count); err != nil {
		t.Fatalf("count dataset versions: %v", err)
	}
	if count != 1 {
		t.Fatalf("dataset versions after repair = %d, want 1", count)
	}
}

// TestExecutionOutputRecoveryPrefersHalfProductOverTerminalHistory covers an
// upgraded installation that already has terminal history for the same output
// key and a newer recoverable half-product. Recovery must select the half-product
// instead of returning the older terminal row and permanently rejecting the
// managed finalize.
func TestExecutionOutputRecoveryPrefersHalfProductOverTerminalHistory(t *testing.T) {
	fixture, _, datasetID := newC2AFixture(t, fakeStore{})
	executionID := uuid.New()
	terminalID := uuid.New()
	halfProductID := uuid.New()
	if _, err := fixture.pool.Exec(fixture.ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, metadata, generated_by_execution_id,
			storage_type, storage_uri, checksum_algorithm, checksum_value
		) VALUES
			($1, $3, 1, 'SUPERSEDED', '{}'::jsonb, $2, 'OBJECT_STORAGE', 's3://history/old.csv', 'SHA256', repeat('a', 64)),
			($4, $3, 2, 'FAILED', '{}'::jsonb, $2, NULL, NULL, NULL, NULL),
			($5, $3, 3, 'CREATED', '{}'::jsonb, $2, NULL, NULL, NULL, NULL)
	`, terminalID, executionID, datasetID, uuid.New(), halfProductID); err != nil {
		t.Fatalf("insert terminal history and half-product: %v", err)
	}

	version, err := fixture.upload.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n1,recovered\n"))
	if err != nil {
		t.Fatalf("recover output: %v", err)
	}
	if version.ID != halfProductID {
		t.Fatalf("recovery selected version %s, want recoverable half-product %s", version.ID, halfProductID)
	}
	if version.Status != domain.VersionReady {
		t.Fatalf("recovered status = %s, want READY", version.Status)
	}
	selected, err := fixture.repo.FindVersionByExecution(fixture.ctx, executionID)
	if err != nil {
		t.Fatalf("find recovered output: %v", err)
	}
	if selected.ID != halfProductID {
		t.Fatalf("execution lookup selected version %s, want %s", selected.ID, halfProductID)
	}
}

// TestConcurrentDeliveriesNeverOverwritePublishedContent is the "one row is not
// enough" proof: the unique index guarantees one DatasetVersion row, but object
// storage has no conditional write, so the writer must also guarantee that a
// losing attempt cannot overwrite the winner's bytes. Each attempt stages its
// own object; the published storage URI must still hold content matching the
// published checksum after every attempt finished.
func TestConcurrentDeliveriesNeverOverwritePublishedContent(t *testing.T) {
	store := newRecordingStore()
	fixture, _, datasetID := newC2AFixture(t, store)
	executionID := uuid.New()

	const writers = 8
	var wg sync.WaitGroup
	versions := make([]domain.DatasetVersion, writers)
	errs := make([]error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			versions[index], errs[index] = fixture.upload.Handle(
				fixture.ctx,
				fixture.outputCommand(datasetID, executionID, fmt.Sprintf("id,name\n%d,company-%d\n", index, index)),
			)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent writer %d: %v", i, err)
		}
		if versions[i].ID != versions[0].ID {
			t.Fatalf("writer %d published a different version: %s vs %s", i, versions[i].ID, versions[0].ID)
		}
	}

	// The row the loser adopted must be the row that was actually published.
	published, err := fixture.repo.GetVersion(fixture.ctx, versions[0].ID)
	if err != nil {
		t.Fatalf("read published version: %v", err)
	}
	if published.Status != domain.VersionReady {
		t.Fatalf("published status = %s, want READY", published.Status)
	}
	content, ok := store.read(published.StorageURI)
	if !ok {
		t.Fatalf("published storage URI %q was never written", published.StorageURI)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != published.ChecksumValue {
		t.Fatalf("published object was overwritten: checksum %s but stored bytes hash to %s", published.ChecksumValue, got)
	}
	// Every attempt used a distinct staging key, so no attempt could collide with
	// the winner's object.
	if store.puts < writers {
		t.Fatalf("object writes = %d, want at least %d (one staged object per attempt)", store.puts, writers)
	}
	if count := fixture.countVersions(t, datasetID, executionID); count != 1 {
		t.Fatalf("concurrent deliveries produced %d output versions, want 1", count)
	}
	assertSingle(t, fixture, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='DatasetVersionCreated'`, published.ID, "DatasetVersionCreated outbox events")
	assertSingle(t, fixture, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='DATASET_VERSION_READY'`, published.ID, "DATASET_VERSION_READY audit events")
}

// TestConcurrentDeliveryFailingTheSharedRowStillPublishesTheWinner pins the
// commit-phase race the review found: deliveries of one Execution share the
// output row, so a loser whose object write failed can mark that row FAILED
// between the winner's allocation and the winner's commit. The winner must then
// republish the row in the same transaction it announces, or it would report a
// READY version the database does not hold.
func TestConcurrentDeliveryFailingTheSharedRowStillPublishesTheWinner(t *testing.T) {
	store := newRecordingStore()
	fixture, _, datasetID := newC2AFixture(t, store)
	txManager := transaction.NewManager(fixture.pool)
	gated := newGateStore(store)
	winner := application.NewUploadVersionService(txManager, fixture.repo, gated)
	loser := application.NewUploadVersionService(txManager, fixture.repo, failingStore{})
	executionID := uuid.New()
	body := "id,name\n1,winner\n"

	type outcome struct {
		version domain.DatasetVersion
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		version, err := winner.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, body))
		done <- outcome{version: version, err: err}
	}()

	// The winner allocated the shared row and is staging its object.
	<-gated.entered
	// A competing delivery of the same Execution cannot store its object and marks
	// the shared row FAILED before the winner commits.
	if _, err := loser.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n2,loser\n")); err == nil {
		t.Fatal("delivery with an unavailable object store must fail")
	}
	var failedStatus domain.VersionStatus
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT status FROM dataset_version WHERE dataset_id=$1 AND generated_by_execution_id=$2`, datasetID, executionID).Scan(&failedStatus); err != nil {
		t.Fatalf("read shared row: %v", err)
	}
	if failedStatus != domain.VersionFailed {
		t.Fatalf("shared row status before the winner commits = %s, want FAILED", failedStatus)
	}

	close(gated.release)
	result := <-done
	if result.err != nil {
		t.Fatalf("winner: %v", result.err)
	}
	if result.version.Status != domain.VersionReady {
		t.Fatalf("winner returned status %s, want READY", result.version.Status)
	}

	// The reported fact and the committed row must agree, and the published object
	// must be the winner's own staged content.
	published, err := fixture.repo.GetVersion(fixture.ctx, result.version.ID)
	if err != nil {
		t.Fatalf("read published version: %v", err)
	}
	if published.Status != domain.VersionReady {
		t.Fatalf("committed status = %s, want READY", published.Status)
	}
	if published.StorageURI != result.version.StorageURI {
		t.Fatalf("committed storage URI = %q, want the winner's %q", published.StorageURI, result.version.StorageURI)
	}
	content, ok := store.read(published.StorageURI)
	if !ok {
		t.Fatalf("published storage URI %q was never written", published.StorageURI)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(content)); got != published.ChecksumValue {
		t.Fatalf("published object checksum = %s, want %s", got, published.ChecksumValue)
	}
	if count := fixture.countVersions(t, datasetID, executionID); count != 1 {
		t.Fatalf("output versions = %d, want 1", count)
	}
	// The loser stored nothing and announced nothing: only the winner's publish is
	// recorded as a fact, and it records that the row was recovered from FAILED.
	assertSingle(t, fixture, `SELECT count(*) FROM outbox_event WHERE aggregate_id=$1 AND event_type='DatasetVersionCreated'`, published.ID, "DatasetVersionCreated outbox events")
	assertSingle(t, fixture, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='DATASET_VERSION_READY'`, published.ID, "DATASET_VERSION_READY audit events")
	var previousStatus string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT payload->>'previousStatus' FROM outbox_event
		WHERE aggregate_id=$1 AND event_type='DatasetVersionCreated'
	`, published.ID).Scan(&previousStatus); err != nil {
		t.Fatalf("read DatasetVersionCreated payload: %v", err)
	}
	if previousStatus != string(domain.VersionFailed) {
		t.Fatalf("recovery event previousStatus = %q, want FAILED", previousStatus)
	}
	var current *uuid.UUID
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT current_version_id FROM dataset WHERE id=$1`, datasetID).Scan(&current); err != nil {
		t.Fatalf("read dataset current version: %v", err)
	}
	if current == nil || *current != published.ID {
		t.Fatalf("dataset current_version_id = %v, want %s", current, published.ID)
	}
}

// TestInvalidatedOutputIsNotSilentlyReused makes the failure mode explicit: a
// withdrawn output must stop the replay, not be resurrected by it.
func TestInvalidatedOutputIsNotSilentlyReused(t *testing.T) {
	fixture, _, datasetID := newC2AFixture(t, fakeStore{})
	executionID := uuid.New()
	published, err := fixture.upload.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n1,alpha\n"))
	if err != nil {
		t.Fatalf("publish output: %v", err)
	}
	invalidate := application.NewInvalidateVersionService(transaction.NewManager(fixture.pool), fixture.repo)
	if _, err := invalidate.Handle(fixture.ctx, application.InvalidateVersionCommand{
		VersionID: published.ID,
		Reason:    "withdrawn by governance",
		TraceID:   "c2a-test",
	}); err != nil {
		t.Fatalf("invalidate output: %v", err)
	}

	_, err = fixture.upload.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n2,beta\n"))
	if err == nil {
		t.Fatal("replay of an INVALID output must fail, not resurrect it")
	}
	var status domain.VersionStatus
	if err := fixture.pool.QueryRow(fixture.ctx, `SELECT status FROM dataset_version WHERE id=$1`, published.ID).Scan(&status); err != nil {
		t.Fatalf("read invalidated status: %v", err)
	}
	if status != domain.VersionInvalid {
		t.Fatalf("status = %s, want INVALID preserved", status)
	}
	if count := fixture.countVersions(t, datasetID, executionID); count != 1 {
		t.Fatalf("output versions after rejected replay = %d, want 1", count)
	}
}

// TestLineageReplayKeepsOneEdgePerInput locks the C2-a regression from the design
// doc (section 5.1 item 3): a recovery re-run must not add a second lineage edge
// for the same (output, input, relation) triple, which would double-count inputs
// in release traceability.
func TestLineageReplayKeepsOneEdgePerInput(t *testing.T) {
	fixture, _, datasetID := newC2AFixture(t, fakeStore{})
	executionID := uuid.New()
	output, err := fixture.upload.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n1,alpha\n"))
	if err != nil {
		t.Fatalf("publish output: %v", err)
	}

	inputDataset := application.CreateDatasetCommand{
		WorkspaceID: uuid.New(),
		Code:        "C2A-INPUT-" + uuid.NewString(),
		Name:        "C2-a lineage input",
		DatasetType: domain.DatasetTypeRaw,
		TraceID:     "c2a-test",
	}
	createDataset := application.NewCreateDatasetService(transaction.NewManager(fixture.pool), fixture.repo, nil)
	input, err := createDataset.Handle(fixture.ctx, inputDataset)
	if err != nil {
		t.Fatalf("create input dataset: %v", err)
	}
	inputVersion, err := fixture.upload.Handle(fixture.ctx, application.UploadVersionCommand{
		DatasetID:   input.ID,
		Filename:    "source.csv",
		ContentType: "text/csv",
		Content:     []byte("id,name\n1,alpha\n"),
		TraceID:     "c2a-test",
	})
	if err != nil {
		t.Fatalf("upload input version: %v", err)
	}

	// The same replay the native engine performs after a recovery re-run.
	for attempt := 0; attempt < 3; attempt++ {
		if err := transaction.NewManager(fixture.pool).Do(fixture.ctx, func(ctx context.Context, tx pgx.Tx) error {
			return fixture.repo.AddLineage(ctx, tx, output.ID, inputVersion.ID, "DERIVED_FROM", &executionID)
		}); err != nil {
			t.Fatalf("AddLineage attempt %d: %v", attempt, err)
		}
	}

	var edges int
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT count(*) FROM dataset_version_lineage
		WHERE output_version_id=$1 AND input_version_id=$2 AND relation_type='DERIVED_FROM'
	`, output.ID, inputVersion.ID).Scan(&edges); err != nil {
		t.Fatalf("count lineage edges: %v", err)
	}
	if edges != 1 {
		t.Fatalf("lineage edges after replay = %d, want 1", edges)
	}
}

// TestDatabaseRefusesASecondOutputForOneExecution proves the guarantee is a
// constraint, not just application logic: any other writer inserting through
// SQL is rejected too.
func TestDatabaseRefusesASecondOutputForOneExecution(t *testing.T) {
	fixture, _, datasetID := newC2AFixture(t, fakeStore{})
	executionID := uuid.New()
	if _, err := fixture.upload.Handle(fixture.ctx, fixture.outputCommand(datasetID, executionID, "id,name\n1,alpha\n")); err != nil {
		t.Fatalf("publish output: %v", err)
	}

	_, err := fixture.pool.Exec(fixture.ctx, `
		INSERT INTO dataset_version (id, dataset_id, version_no, status, metadata, created_at, generated_by_execution_id)
		VALUES ($1,$2,$3,'CREATED','{}'::jsonb, now(), $4)
	`, uuid.New(), datasetID, 99, executionID)
	if err == nil {
		t.Fatal("database accepted a second output version for one execution")
	}
	if !containsUniqueViolation(err) {
		t.Fatalf("unexpected error for duplicate output: %v", err)
	}
}

func assertSingle(t *testing.T, fixture c2aFixture, query string, argument any, label string) {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(fixture.ctx, query, argument).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", label, err)
	}
	if count != 1 {
		t.Fatalf("%s = %d, want 1", label, count)
	}
}

func containsUniqueViolation(err error) bool {
	return bytes.Contains([]byte(err.Error()), []byte("uq_dataset_version_execution_output"))
}
