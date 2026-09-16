package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	contractapp "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/application"
	contractdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	contractinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/contract/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
)

func TestAuthorizationSnapshotAndContractLifecycle(t *testing.T) {
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

	workspaceID := uuid.New()
	resourceID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type)
		VALUES ($1,$2,$3,$4,'TABLE_LIKE')
	`, resourceID, workspaceID, "ENERGY-"+uuid.NewString(), "Enterprise energy"); err != nil {
		t.Fatalf("insert data resource: %v", err)
	}

	txManager := transaction.NewManager(pool)
	rightsRepo := rightsinfra.NewPostgresRepository(pool)
	rightsService := rightsapp.NewService(txManager, rightsRepo)

	validFrom := time.Now().UTC().Add(-time.Hour)
	validTo := time.Now().UTC().Add(24 * time.Hour)
	authorization, err := rightsService.Create(ctx, rightsapp.CreateAuthorizationCommand{
		WorkspaceID: workspaceID,
		Code:        "AUTH-" + uuid.NewString(),
		GrantorRef:  "PARK-OPERATOR",
		GranteeRef:  "DATA-PRODUCT-PLATFORM",
		Purpose:     "ENTERPRISE_CREDIT_RISK_SUPPORT",
		ValidFrom:   &validFrom,
		ValidTo:     &validTo,
		Resources: []rightsdomain.ResourceGrantSpec{
			{
				DataResourceID: resourceID,
				Actions:        []string{"READ", "AGGREGATE", "DERIVE", "PRODUCTIZE"},
				Scope:          map[string]any{"region": "POC"},
			},
		},
		TraceID: "rights-contract-e2e",
	})
	if err != nil {
		t.Fatalf("create authorization: %v", err)
	}
	authorization, err = rightsService.Submit(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, TraceID: "rights-contract-e2e"})
	if err != nil {
		t.Fatalf("submit authorization: %v", err)
	}
	authorization, err = rightsService.Approve(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, TraceID: "rights-contract-e2e"})
	if err != nil {
		t.Fatalf("approve authorization: %v", err)
	}
	authorization, err = rightsService.Activate(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, At: time.Now().UTC(), TraceID: "rights-contract-e2e"})
	if err != nil {
		t.Fatalf("activate authorization: %v", err)
	}
	if authorization.Status != rightsdomain.StatusActive {
		t.Fatalf("authorization status = %s, want ACTIVE", authorization.Status)
	}

	snapshot, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID:      workspaceID,
		Purpose:          "ENTERPRISE_CREDIT_RISK_SUPPORT",
		ConsumerRef:      "LICENSED_BANK",
		AsOf:             time.Now().UTC(),
		AuthorizationIDs: []uuid.UUID{authorization.ID},
		TraceID:          "rights-contract-e2e",
	})
	if err != nil {
		t.Fatalf("create rights snapshot: %v", err)
	}
	if snapshot.RootHash == "" || len(snapshot.Manifest.Authorizations) != 1 {
		t.Fatalf("rights snapshot missing hash or authorization manifest")
	}
	storedSnapshot, err := rightsRepo.GetSnapshot(ctx, snapshot.ID)
	if err != nil {
		t.Fatalf("read rights snapshot: %v", err)
	}
	if storedSnapshot.RootHash != snapshot.RootHash {
		t.Fatalf("stored rights snapshot hash changed")
	}

	if _, err := rightsService.Suspend(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, TraceID: "rights-contract-e2e"}); err != nil {
		t.Fatalf("suspend authorization: %v", err)
	}
	_, err = rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID:      workspaceID,
		Purpose:          "ENTERPRISE_CREDIT_RISK_SUPPORT",
		AsOf:             time.Now().UTC(),
		AuthorizationIDs: []uuid.UUID{authorization.ID},
		TraceID:          "rights-contract-e2e",
	})
	if !errors.Is(err, rightsdomain.ErrAuthorizationInvalid) {
		t.Fatalf("snapshot from suspended authorization error = %v, want ErrAuthorizationInvalid", err)
	}

	contractRepo := contractinfra.NewPostgresRepository(pool)
	contractService := contractapp.NewService(txManager, contractRepo)
	contractYAML, err := os.ReadFile(repoPath(t, "examples", "enterprise-activity", "contract", "data-contract-v1.yaml"))
	if err != nil {
		t.Fatalf("read Data Contract fixture: %v", err)
	}
	version, err := contractService.CreateVersionFromYAML(ctx, contractapp.CreateVersionFromYAMLCommand{
		WorkspaceID:  workspaceID,
		SourceRef:    "examples/enterprise-activity/contract/data-contract-v1.yaml",
		DocumentYAML: contractYAML,
		TraceID:      "rights-contract-e2e",
	})
	if err != nil {
		t.Fatalf("create ContractVersion: %v", err)
	}
	if version.Semver() != "1.0.0" || version.Status != contractdomain.VersionDraft {
		t.Fatalf("contract version = %s/%s, want 1.0.0/DRAFT", version.Semver(), version.Status)
	}
	version, err = contractService.PublishVersion(ctx, contractapp.PublishVersionCommand{VersionID: version.ID, TraceID: "rights-contract-e2e"})
	if err != nil {
		t.Fatalf("publish ContractVersion: %v", err)
	}
	if version.Status != contractdomain.VersionPublished || version.PublishedAt == nil {
		t.Fatalf("published contract version = status %s publishedAt %v", version.Status, version.PublishedAt)
	}

	if _, err := pool.Exec(ctx, `UPDATE contract_version SET document='{}'::jsonb WHERE id=$1`, version.ID); err == nil {
		t.Fatal("published ContractVersion document mutation unexpectedly succeeded")
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_event
		WHERE trace_id='rights-contract-e2e'
		  AND object_type IN ('AUTHORIZATION','RIGHTS_SNAPSHOT','CONTRACT_VERSION')
	`).Scan(&auditCount); err != nil {
		t.Fatalf("count governance audit events: %v", err)
	}
	if auditCount < 7 {
		t.Fatalf("governance audit events = %d, want at least 7", auditCount)
	}
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../"))
	return filepath.Join(append([]string{root}, parts...)...)
}
