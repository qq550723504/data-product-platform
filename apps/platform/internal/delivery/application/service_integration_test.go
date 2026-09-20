package application

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type integrationGate struct {
	mu      sync.Mutex
	allowed bool
	checks  int
}

func (g *integrationGate) Evaluate(_ context.Context, _ pgx.Tx, req domain.GateRequest) (domain.GateEvaluation, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.checks++
	if !g.allowed {
		return domain.GateEvaluation{Allowed: false, Blockers: []string{"TEST_GATE_BLOCKED"}}, nil
	}
	cap := time.Now().UTC().Add(10 * time.Minute)
	return domain.GateEvaluation{
		Allowed:              true,
		PrincipalRef:         req.PrincipalRef,
		EffectiveConsumerRef: req.EffectiveConsumerRef,
		FreshCapExpiresAt:    &cap,
	}, nil
}

type integrationProvider struct {
	mu           sync.Mutex
	issueCalls   int
	recoverCalls int
	requestKeys  []string
	capability   domain.Capability
}

func (p *integrationProvider) Issue(_ context.Context, req ProviderRequest) (domain.Capability, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.issueCalls++
	p.requestKeys = append(p.requestKeys, req.ProviderRequestKey)
	return p.capability, nil
}

func (p *integrationProvider) Recover(_ context.Context, key string) (domain.Capability, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recoverCalls++
	p.requestKeys = append(p.requestKeys, key)
	return p.capability, nil
}

func (p *integrationProvider) Revoke(context.Context, string) error { return nil }

func TestCredentialIssueRecoversSameProviderKeyAfterCrashWindow(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source")
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

	workspaceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO dataset(id, workspace_id, code, name, dataset_type) VALUES($1,$2,$3,'Delivery fixture','CURATED')`, datasetID, workspaceID, "DELIVERY-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id, dataset_id, version_no, status, storage_type, storage_uri, content_type, checksum_algorithm, checksum_value, metadata, ready_at) VALUES($1,$2,1,'READY','OBJECT_STORAGE','s3://delivery-fixture','application/octet-stream','SHA256',repeat('a',64),'{}'::jsonb,now())`, versionID, datasetID); err != nil {
		t.Fatal(err)
	}

	gate := &integrationGate{allowed: true}
	provider := &integrationProvider{}
	expires := time.Now().UTC().Add(5 * time.Minute)
	provider.capability = domain.Capability{
		Credential:                  "secret-never-persisted",
		CapabilityRef:               "provider-capability-ref",
		ProviderCredentialExpiresAt: expires,
		DatasetVersionID:            versionID,
		ConsumerRef:                 "consumer-a",
		Action:                      "READ",
		ScopeRef:                    "dataset-version",
		DeliveryChannel:             "REDEMPTION",
		AuthoritativelyVerified:     true,
	}
	txManager := transaction.NewManager(pool)
	service := NewService(txManager, infrastructure.NewPostgresRepository(pool), gate, provider)
	service.AfterProviderCall = func() { panic("simulated process crash after provider success") }
	cmd := IssueCredentialCommand{
		WorkspaceID: workspaceID, DatasetVersionID: versionID, ProviderName: "test-provider",
		PrincipalRef: "principal-a", EffectiveConsumerRef: "consumer-a", Purpose: "RESEARCH",
		Action: "READ", ScopeRef: "dataset-version", DeliveryChannel: "REDEMPTION",
		RequestedExpiresAt: time.Now().UTC().Add(time.Hour), IdempotencyKey: "delivery-crash-window-1",
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected simulated crash")
			}
		}()
		_, _ = service.IssueCredential(ctx, cmd)
	}()
	service.AfterProviderCall = nil
	result, err := service.IssueCredential(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if result.Operation.Status != domain.StatusIssued || result.Capability == nil || result.Capability.Credential != "secret-never-persisted" {
		t.Fatalf("recovered result = %#v", result)
	}
	provider.mu.Lock()
	if provider.issueCalls != 1 || provider.recoverCalls != 1 || len(provider.requestKeys) != 2 || provider.requestKeys[0] != provider.requestKeys[1] {
		t.Fatalf("provider calls = issue:%d recover:%d keys:%v", provider.issueCalls, provider.recoverCalls, provider.requestKeys)
	}
	provider.mu.Unlock()

	var operationCount, terminalEvents, secretCount, attemptCount, costCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_operation WHERE workspace_id=$1`, workspaceID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_type='DELIVERY_OPERATION' AND event_type='DatasetDeliveryIssued' AND aggregate_id=(SELECT id FROM delivery_operation WHERE workspace_id=$1)`, workspaceID).Scan(&terminalEvents); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_operation WHERE workspace_id=$1 AND credential_hash IS NULL`, workspaceID).Scan(&secretCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_provider_attempt a JOIN delivery_operation o ON o.id=a.delivery_operation_id WHERE o.workspace_id=$1`, workspaceID).Scan(&attemptCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN delivery_operation o ON o.id=c.delivery_operation_id WHERE o.workspace_id=$1`, workspaceID).Scan(&costCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 || terminalEvents != 1 || secretCount != 0 || attemptCount != 2 || costCount != 2 {
		t.Fatalf("facts operation=%d terminal_events=%d secret_rows=%d attempts=%d costs=%d", operationCount, terminalEvents, secretCount, attemptCount, costCount)
	}
}
