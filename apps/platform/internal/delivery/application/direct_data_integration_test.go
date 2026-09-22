package application

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type directIntegrationGate struct {
	mu               sync.Mutex
	allowed          bool
	checks           int
	certificationRef uuid.UUID
	blocker          string
}

func (g *directIntegrationGate) EvaluateDirectData(_ context.Context, tx pgx.Tx, request DirectDataGateRequest) (DirectDataGateResult, error) {
	if tx == nil {
		return DirectDataGateResult{}, errors.New("delivery fence transaction is required")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checks++
	evaluation := domain.GateEvaluation{
		Allowed:              g.allowed,
		PrincipalRef:         request.PrincipalRef,
		EffectiveConsumerRef: request.EffectiveConsumerRef,
	}
	if !g.allowed {
		blocker := g.blocker
		if blocker == "" {
			blocker = "TEST_GATE_BLOCKED"
		}
		evaluation.Blockers = []string{blocker}
		return DirectDataGateResult{Evaluation: evaluation}, nil
	}
	cap := request.RequestedExpiresAt
	evaluation.FreshCapExpiresAt = &cap
	id := g.certificationRef
	return DirectDataGateResult{Evaluation: evaluation, CertificationRef: &id}, nil
}

func TestDirectDataDeliveryLinearizesIssuedAndNeverReplaysPayload(t *testing.T) {
	pool, ctx := directDataTestDatabase(t)
	workspaceID, versionID := insertDirectDataFixture(t, ctx, pool)
	profileID := insertDirectDataProfile(t, ctx, pool, workspaceID)
	gate := &directIntegrationGate{allowed: true, certificationRef: uuid.New()}
	service := NewDirectDataService(
		transaction.NewManager(pool),
		infrastructure.NewPostgresRepository(pool),
		gate,
		datasetinfra.NewPostgresRepository(pool),
	)

	cmd := DirectDataCommand{
		WorkspaceID: workspaceID, DatasetVersionID: versionID, ProfileID: profileID,
		PrincipalRef: "principal-a", EffectiveConsumerRef: "consumer-a",
		Purpose: "RESEARCH", Action: "READ", ScopeType: "ALL_RESOURCE",
		IdempotencyKey: "direct-issued-" + uuid.NewString(), TraceID: "direct-issued",
	}
	first, err := service.Deliver(ctx, cmd)
	if err != nil {
		t.Fatalf("first direct delivery: %v", err)
	}
	if !first.PayloadReady || first.ReplayRequired || first.Operation.Status != domain.StatusIssued || first.DatasetVersion.ID != versionID {
		t.Fatalf("first result = %#v", first)
	}
	gate.mu.Lock()
	checksAfterFirst := gate.checks
	gate.mu.Unlock()
	if checksAfterFirst != 1 {
		t.Fatalf("fresh gate checks = %d, want 1", checksAfterFirst)
	}

	replay, err := service.Deliver(ctx, cmd)
	if !errors.Is(err, ErrDirectDataReplayRequiresNewAttempt) {
		t.Fatalf("same-key replay error = %v, want ErrDirectDataReplayRequiresNewAttempt", err)
	}
	if replay.PayloadReady || !replay.ReplayRequired || replay.Operation.ID != first.Operation.ID {
		t.Fatalf("replay result = %#v", replay)
	}
	gate.mu.Lock()
	checksAfterReplay := gate.checks
	gate.mu.Unlock()
	if checksAfterReplay != 1 {
		t.Fatalf("same-key replay re-evaluated gate %d times", checksAfterReplay)
	}

	var operations, issuedEvents, attempts, costs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_operation WHERE workspace_id=$1`, workspaceID).Scan(&operations); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_type='DELIVERY_OPERATION' AND aggregate_id=$1 AND event_type='DatasetDeliveryIssued'`, first.Operation.ID).Scan(&issuedEvents); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_provider_attempt WHERE delivery_operation_id=$1`, first.Operation.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation WHERE delivery_operation_id=$1`, first.Operation.ID).Scan(&costs); err != nil {
		t.Fatal(err)
	}
	if operations != 1 || issuedEvents != 1 || attempts != 0 || costs != 0 {
		t.Fatalf("direct facts operations=%d issued_events=%d provider_attempts=%d costs=%d", operations, issuedEvents, attempts, costs)
	}
}

func TestDirectDataReplacementAttemptReevaluatesFreshGateAndPersistsBlocked(t *testing.T) {
	pool, ctx := directDataTestDatabase(t)
	workspaceID, versionID := insertDirectDataFixture(t, ctx, pool)
	profileID := insertDirectDataProfile(t, ctx, pool, workspaceID)
	gate := &directIntegrationGate{allowed: true, certificationRef: uuid.New()}
	service := NewDirectDataService(
		transaction.NewManager(pool),
		infrastructure.NewPostgresRepository(pool),
		gate,
		datasetinfra.NewPostgresRepository(pool),
	)

	base := DirectDataCommand{
		WorkspaceID: workspaceID, DatasetVersionID: versionID, ProfileID: profileID,
		PrincipalRef: "principal-a", EffectiveConsumerRef: "consumer-a",
		Purpose: " research ", Action: " read ", ScopeType: " all_resource ",
		IdempotencyKey: " direct-first-" + uuid.NewString() + " ",
	}
	first, err := service.Deliver(ctx, base)
	if err != nil {
		t.Fatalf("initial allowed delivery: %v", err)
	}

	gate.mu.Lock()
	gate.allowed = false
	gate.blocker = "DATASET_VERSION_INVALID"
	gate.mu.Unlock()
	if first.Operation.Purpose != "RESEARCH" || first.Operation.Action != "READ" || first.Operation.IdempotencyKey == "" || strings.TrimSpace(first.Operation.IdempotencyKey) != first.Operation.IdempotencyKey {
		t.Fatalf("immutable operation context was not canonicalized: purpose=%q action=%q key=%q", first.Operation.Purpose, first.Operation.Action, first.Operation.IdempotencyKey)
	}
	replacement := base
	replacement.Purpose = "RESEARCH"
	replacement.Action = "READ"
	replacement.ScopeType = "ALL_RESOURCE"
	replacement.IdempotencyKey = "direct-replacement-" + uuid.NewString()
	replacement.RetryOfDeliveryOperationID = &first.Operation.ID
	blocked, err := service.Deliver(ctx, replacement)
	if err != nil {
		t.Fatalf("replacement blocked delivery: %v", err)
	}
	if blocked.PayloadReady || blocked.Operation.Status != domain.StatusBlocked || len(blocked.Blockers) != 1 || blocked.Blockers[0] != "DATASET_VERSION_INVALID" {
		t.Fatalf("blocked result = %#v", blocked)
	}

	var blockedEvents, gateFacts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_type='DELIVERY_OPERATION' AND aggregate_id=$1 AND event_type='DatasetDeliveryBlocked'`, blocked.Operation.ID).Scan(&blockedEvents); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_gate_evaluation WHERE delivery_operation_id=$1 AND decision='BLOCKED'`, blocked.Operation.ID).Scan(&gateFacts); err != nil {
		t.Fatal(err)
	}
	if blockedEvents != 1 || gateFacts != 1 {
		t.Fatalf("blocked facts events=%d gate_evaluations=%d", blockedEvents, gateFacts)
	}
	var persistedProfileID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT certification_profile_id FROM delivery_gate_evaluation WHERE delivery_operation_id=$1 AND decision='BLOCKED'`, blocked.Operation.ID).Scan(&persistedProfileID); err != nil {
		t.Fatal(err)
	}
	if persistedProfileID != profileID {
		t.Fatalf("blocked gate profile = %s, want %s", persistedProfileID, profileID)
	}
	var retryOf uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT retry_of_delivery_operation_id FROM delivery_operation WHERE id=$1`, blocked.Operation.ID).Scan(&retryOf); err != nil {
		t.Fatal(err)
	}
	if retryOf != first.Operation.ID {
		t.Fatalf("replacement retry_of = %s, want %s", retryOf, first.Operation.ID)
	}
}

func TestDirectDataReplacementRejectsUnrelatedOperation(t *testing.T) {
	pool, ctx := directDataTestDatabase(t)
	workspaceID, versionID := insertDirectDataFixture(t, ctx, pool)
	otherWorkspaceID, otherVersionID := insertDirectDataFixture(t, ctx, pool)
	profileID := insertDirectDataProfile(t, ctx, pool, workspaceID)
	otherProfileID := insertDirectDataProfile(t, ctx, pool, otherWorkspaceID)
	gate := &directIntegrationGate{allowed: true, certificationRef: uuid.New()}
	service := NewDirectDataService(
		transaction.NewManager(pool),
		infrastructure.NewPostgresRepository(pool),
		gate,
		datasetinfra.NewPostgresRepository(pool),
	)
	other, err := service.Deliver(ctx, DirectDataCommand{
		WorkspaceID: otherWorkspaceID, DatasetVersionID: otherVersionID, ProfileID: otherProfileID,
		PrincipalRef: "principal-a", EffectiveConsumerRef: "consumer-a",
		Purpose: "RESEARCH", Action: "READ", ScopeType: "ALL_RESOURCE",
		IdempotencyKey: "direct-other-" + uuid.NewString(),
	})
	if err != nil {
		t.Fatalf("create unrelated issued operation: %v", err)
	}
	_, err = service.Deliver(ctx, DirectDataCommand{
		WorkspaceID: workspaceID, DatasetVersionID: versionID, ProfileID: profileID,
		PrincipalRef: "principal-a", EffectiveConsumerRef: "consumer-a",
		Purpose: "RESEARCH", Action: "READ", ScopeType: "ALL_RESOURCE",
		RetryOfDeliveryOperationID: &other.Operation.ID,
		IdempotencyKey:             "direct-invalid-retry-" + uuid.NewString(),
	})
	if !errors.Is(err, domain.ErrInvalidOperation) {
		t.Fatalf("unrelated retry_of error = %v, want ErrInvalidOperation", err)
	}
}

func directDataTestDatabase(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve direct-data test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../"))
	if err := migration.NewRunner(pool, filepath.Join(root, "migrations")).Up(ctx); err != nil {
		t.Fatal(err)
	}
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	t.Cleanup(func() { outbox.ConfigureAppendObligation(nil) })
	return pool, ctx
}

func insertDirectDataProfile(t *testing.T, ctx context.Context, pool *pgxpool.Pool, workspaceID uuid.UUID) uuid.UUID {
	t.Helper()
	profileID := uuid.New()
	snapshot := "direct-data-integration-profile"
	if _, err := pool.Exec(ctx, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version,
			content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required,
			contract_required, contract_code, traceability_required, evidence_required
		) VALUES(
			$1,$2,$3,$4,'Direct data integration profile','1.0.0',
			encode(digest(convert_to($5,'UTF8'),'sha256'),'hex'),$5,
			'ANY','ANY','ANY','ANY',
			false,false,false,false,NULL,false,false
		)
	`, profileID, workspaceID, "direct/profile/"+profileID.String(), "DIRECT-"+profileID.String(), snapshot); err != nil {
		t.Fatalf("insert certification profile: %v", err)
	}
	return profileID
}

func insertDirectDataFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID) {
	t.Helper()
	workspaceID, datasetID, versionID := uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES($1,$2,$3,'Direct data fixture','CURATED')
	`, datasetID, workspaceID, "DIRECT-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri, content_type,
			byte_size, checksum_algorithm, checksum_value, metadata, ready_at
		) VALUES($1,$2,1,'READY','OBJECT_STORAGE',$3,'text/csv',4,'SHA256',repeat('a',64),'{}'::jsonb,now())
	`, versionID, datasetID, "s3://data-product-platform/direct/"+versionID.String()+".csv"); err != nil {
		t.Fatal(err)
	}
	return workspaceID, versionID
}
