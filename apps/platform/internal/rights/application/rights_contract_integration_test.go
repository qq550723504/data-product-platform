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
		GranteeRef:  "LICENSED_BANK",
		Purpose:     "ENTERPRISE_CREDIT_RISK_SUPPORT",
		ValidFrom:   &validFrom,
		ValidTo:     &validTo,
		Resources: []rightsdomain.ResourceGrantSpec{
			{
				DataResourceID: resourceID,
				Actions:        []string{"USE", "DERIVE"},
				ScopeType:      "ALL_RESOURCE",
				ScopeRef:       resourceID.String(),
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

	provenanceService := rightsapp.NewService(transaction.NewManager(pool), rightsinfra.NewPostgresRepository(pool))
	declaration, err := provenanceService.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{
		Spec: rightsdomain.RightsDeclarationSpec{
			WorkspaceID:    workspaceID,
			DataResourceID: resourceID,
			ClaimantRef:    "PARK-OPERATOR",
			BasisType:      "LICENSE",
			BasisRef:       "rights-contract-fixture",
			Parties:        []rightsdomain.RightsParty{{PartyRef: "PARK-OPERATOR", Role: "RIGHTS_HOLDER"}},
			Permissions: []rightsdomain.RightsPermission{
				{Kind: rightsdomain.PermissionGrant, Action: "USE", Purpose: "ENTERPRISE_CREDIT_RISK_SUPPORT", Scope: rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resourceID.String()}},
				{Kind: rightsdomain.PermissionGrant, Action: "DERIVE", Purpose: "ENTERPRISE_CREDIT_RISK_SUPPORT", Scope: rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resourceID.String()}},
			},
		},
		TraceID: "rights-contract-e2e",
	})
	if err != nil {
		t.Fatalf("create rights declaration: %v", err)
	}
	if _, err := provenanceService.VerifyRightsDeclaration(ctx, rightsapp.VerifyRightsDeclarationCommand{DeclarationID: declaration.ID, Outcome: rightsdomain.DeclarationVerified, TraceID: "rights-contract-e2e"}); err != nil {
		t.Fatalf("verify rights declaration: %v", err)
	}
	if _, err := provenanceService.BindAuthorizationProvenance(ctx, rightsapp.BindAuthorizationProvenanceCommand{
		WorkspaceID: workspaceID, AuthorizationID: authorization.ID, DataResourceID: resourceID, DeclarationID: declaration.ID,
		GrantorRef: "PARK-OPERATOR", AuthorityMode: rightsdomain.AuthorityDirect, AsOf: time.Now().UTC(), TraceID: "rights-contract-e2e",
	}); err != nil {
		t.Fatalf("bind authorization provenance: %v", err)
	}
	selected, err := rightsRepo.CheckCurrentEntitlement(ctx, rightsdomain.EntitlementRequest{
		WorkspaceID:    workspaceID,
		DataResourceID: resourceID,
		ConsumerRef:    "LICENSED_BANK",
		Purpose:        "ENTERPRISE_CREDIT_RISK_SUPPORT",
		Action:         "USE",
		Scope:          rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resourceID.String()},
		AsOf:           time.Now().UTC(),
		Path:           rightsdomain.EntitlementDownstream,
	})
	if err != nil {
		t.Fatalf("check selected downstream entitlement: %v", err)
	}
	if selected.Decision != rightsdomain.DecisionAllowed || selected.AuthorizationID != authorization.ID {
		t.Fatalf("selected downstream authorization = decision %s authorization %s, want ALLOWED/%s", selected.Decision, selected.AuthorizationID, authorization.ID)
	}


	// A grant declared in one workspace must not name another workspace's resource.
	foreignWorkspace := uuid.New()
	foreignResourceID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_resource (id, workspace_id, code, name, resource_type)
		VALUES ($1,$2,$3,$4,'TABLE_LIKE')
	`, foreignResourceID, foreignWorkspace, "FOREIGN-"+uuid.NewString(), "Foreign resource"); err != nil {
		t.Fatalf("insert foreign data resource: %v", err)
	}
	foreignCode := "AUTH-FOREIGN-" + uuid.NewString()
	if _, err := rightsService.Create(ctx, rightsapp.CreateAuthorizationCommand{
		WorkspaceID: workspaceID,
		Code:        foreignCode,
		GrantorRef:  "PARK-OPERATOR",
		GranteeRef:  "LICENSED_BANK",
		Purpose:     "ENTERPRISE_CREDIT_RISK_SUPPORT",
		Resources:   []rightsdomain.ResourceGrantSpec{{DataResourceID: foreignResourceID, Actions: []string{"READ"}, ScopeType: "ALL_RESOURCE", ScopeRef: foreignResourceID.String()}},
		TraceID:     "rights-contract-e2e",
	}); !errors.Is(err, rightsdomain.ErrResourceWorkspace) {
		t.Fatalf("cross workspace grant error = %v, want ErrResourceWorkspace", err)
	}
	var foreignAuthorizationCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM data_authorization WHERE code=$1`, foreignCode).Scan(&foreignAuthorizationCount); err != nil {
		t.Fatalf("count rejected foreign authorization: %v", err)
	}
	if foreignAuthorizationCount != 0 {
		t.Fatal("rejected cross workspace authorization was persisted")
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

func TestHistoricalEntitlementAndSnapshotRejectFutureVerification(t *testing.T) {
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
	`, resourceID, workspaceID, "HISTORICAL-"+uuid.NewString(), "Historical rights fixture"); err != nil {
		t.Fatalf("insert data resource: %v", err)
	}

	txManager := transaction.NewManager(pool)
	rightsRepo := rightsinfra.NewPostgresRepository(pool)
	rightsService := rightsapp.NewService(txManager, rightsRepo)
	provenanceService := rightsapp.NewService(txManager, rightsRepo)

	purpose := "HISTORICAL_RIGHTS_TEST"
	consumer := "LICENSED_BANK"
	validFrom := time.Now().UTC().Add(-time.Hour)
	validTo := time.Now().UTC().Add(24 * time.Hour)
	authorization, err := rightsService.Create(ctx, rightsapp.CreateAuthorizationCommand{
		WorkspaceID: workspaceID,
		Code:        "AUTH-" + uuid.NewString(),
		GrantorRef:  "PARK-OPERATOR",
		GranteeRef:  consumer,
		Purpose:     purpose,
		ValidFrom:   &validFrom,
		ValidTo:     &validTo,
		Resources: []rightsdomain.ResourceGrantSpec{{
			DataResourceID: resourceID,
			Actions:        []string{"USE"},
			ScopeType:      "ALL_RESOURCE",
			ScopeRef:       resourceID.String(),
		}},
		TraceID: "historical-rights-test",
	})
	if err != nil {
		t.Fatalf("create authorization: %v", err)
	}
	for _, transition := range []func() error{
		func() error {
			_, err := rightsService.Submit(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, TraceID: "historical-rights-test"})
			return err
		},
		func() error {
			_, err := rightsService.Approve(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, TraceID: "historical-rights-test"})
			return err
		},
		func() error {
			_, err := rightsService.Activate(ctx, rightsapp.TransitionCommand{AuthorizationID: authorization.ID, At: time.Now().UTC(), TraceID: "historical-rights-test"})
			return err
		},
	} {
		if err := transition(); err != nil {
			t.Fatalf("transition authorization: %v", err)
		}
	}

	declaration, err := provenanceService.CreateRightsDeclaration(ctx, rightsapp.CreateRightsDeclarationCommand{
		Spec: rightsdomain.RightsDeclarationSpec{
			WorkspaceID:    workspaceID,
			DataResourceID: resourceID,
			ClaimantRef:    "PARK-OPERATOR",
			BasisType:      "LICENSE",
			BasisRef:       "historical-rights-test",
			Parties:        []rightsdomain.RightsParty{{PartyRef: "PARK-OPERATOR", Role: "RIGHTS_HOLDER"}},
			Permissions: []rightsdomain.RightsPermission{{
				Kind:    rightsdomain.PermissionGrant,
				Action:  "USE",
				Purpose: purpose,
				Scope:   rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resourceID.String()},
			}},
		},
		TraceID: "historical-rights-test",
	})
	if err != nil {
		t.Fatalf("create rights declaration: %v", err)
	}

	// Model a legacy binding that existed before the historical cutoff, while
	// its verification fact was recorded afterwards. Historical queries must
	// not treat that future verification as valid evidence.
	asOf := time.Now().UTC()
	bindingID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO authorization_provenance_binding (
			id, workspace_id, authorization_id, data_resource_id, rights_declaration_id,
			grantor_ref, grantor_authority_mode, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,'DIRECT_DECLARATION_PARTY',$7)
	`, bindingID, workspaceID, authorization.ID, resourceID, declaration.ID, "PARK-OPERATOR", asOf.Add(-time.Minute)); err != nil {
		t.Fatalf("insert historical binding: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO rights_declaration_verification (id, declaration_id, outcome, occurred_at)
		VALUES ($1,$2,'VERIFIED',$3)
	`, uuid.New(), declaration.ID, asOf.Add(time.Hour)); err != nil {
		t.Fatalf("insert future verification: %v", err)
	}

	decision, err := provenanceService.CheckCurrentEntitlement(ctx, rightsapp.CheckCurrentEntitlementCommand{
		EntitlementRequest: rightsdomain.EntitlementRequest{
			WorkspaceID:     workspaceID,
			AuthorizationID: authorization.ID,
			DataResourceID:  resourceID,
			ConsumerRef:     consumer,
			Purpose:         purpose,
			Action:          "USE",
			Scope:           rightsdomain.NormalizedScope{Type: "ALL_RESOURCE", Ref: resourceID.String()},
			AsOf:            asOf,
			Path:            rightsdomain.EntitlementDownstream,
		},
		TraceID: "historical-rights-test",
	})
	if err != nil {
		t.Fatalf("check historical entitlement: %v", err)
	}
	if decision.Decision != rightsdomain.DecisionNotAllowed {
		t.Fatalf("historical entitlement decision = %s, want %s", decision.Decision, rightsdomain.DecisionNotAllowed)
	}

	_, err = rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
		WorkspaceID:      workspaceID,
		Purpose:          purpose,
		ConsumerRef:      consumer,
		AsOf:             asOf,
		AuthorizationIDs: []uuid.UUID{authorization.ID},
		TraceID:          "historical-rights-test",
	})
	if !errors.Is(err, rightsdomain.ErrAuthorizationInvalid) {
		t.Fatalf("historical rights snapshot error = %v, want ErrAuthorizationInvalid", err)
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
