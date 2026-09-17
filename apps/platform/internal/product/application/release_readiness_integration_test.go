package application_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestReleaseValidationUsesRealGovernanceResults(t *testing.T) {
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
	datasetID := uuid.New()
	datasetVersionID := uuid.New()
	executionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (
			id, workspace_id, code, name, dataset_type, lifecycle_status, metadata,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,'CURATED','ACTIVE','{}'::jsonb,now(),now())
	`, datasetID, workspaceID, "READINESS-DATASET-"+uuid.NewString(), "Readiness curated dataset"); err != nil {
		t.Fatalf("insert dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, generated_by_execution_id,
			metadata, created_at, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://test-bucket/readiness.csv',
		          'text/csv','SHA256','bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
		          $3,'{}'::jsonb,now(),now())
	`, datasetVersionID, datasetID, executionID); err != nil {
		t.Fatalf("insert DatasetVersion: %v", err)
	}

	contractID := uuid.New()
	contractVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_contract (id, workspace_id, code, name, product_code)
		VALUES ($1,$2,$3,'Enterprise Activity Contract','DP-ENTERPRISE-ACTIVITY')
	`, contractID, workspaceID, "CONTRACT-"+uuid.NewString()); err != nil {
		t.Fatalf("insert DataContract: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO contract_version (
			id, contract_id, major_version, minor_version, patch_version, status,
			document, source_sha256, created_at, published_at
		) VALUES ($1,$2,1,0,0,'PUBLISHED','{"spec":{"product":{"code":"DP-ENTERPRISE-ACTIVITY"}}}'::jsonb,
		          'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc',now(),now())
	`, contractVersionID, contractID); err != nil {
		t.Fatalf("insert ContractVersion: %v", err)
	}

	authorizationID := uuid.New()
	rightsSnapshotID := uuid.New()
	validFrom := time.Now().UTC().Add(-time.Hour)
	validTo := time.Now().UTC().Add(time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_authorization (
			id, workspace_id, code, grantor_ref, grantee_ref, purpose, status,
			valid_from, valid_to, metadata, created_at, updated_at
		) VALUES ($1,$2,$3,'PARK-OPERATOR','DATA-PRODUCT-PLATFORM','ENTERPRISE_CREDIT_RISK_SUPPORT','ACTIVE',
		          $4,$5,'{}'::jsonb,now(),now())
	`, authorizationID, workspaceID, "AUTH-"+uuid.NewString(), validFrom, validTo); err != nil {
		t.Fatalf("insert Authorization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO rights_snapshot (
			id, workspace_id, purpose, consumer_ref, as_of, manifest, root_hash, created_at
		) VALUES ($1,$2,'ENTERPRISE_CREDIT_RISK_SUPPORT','LICENSED_BANK',now(),
		          '{"purpose":"ENTERPRISE_CREDIT_RISK_SUPPORT","authorizations":[]}'::jsonb,
		          'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd',now())
	`, rightsSnapshotID, workspaceID); err != nil {
		t.Fatalf("insert RightsSnapshot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO rights_snapshot_authorization (rights_snapshot_id, authorization_id)
		VALUES ($1,$2)
	`, rightsSnapshotID, authorizationID); err != nil {
		t.Fatalf("bind snapshot authorization: %v", err)
	}

	qualityResultID := uuid.New()
	complianceResultID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, created_at
		) VALUES ($1,$2,$3,'park/quality/enterprise-activity-quality-v1.yaml','1.0.0','PASS','{}'::jsonb,now())
	`, qualityResultID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert QualityResult: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO compliance_result (
			id, workspace_id, dataset_version_id, policy_ref, policy_version,
			gate_decision, summary, created_at
		) VALUES ($1,$2,$3,'park/compliance/enterprise-activity-compliance-v1.yaml','1.0.0','PASS','{}'::jsonb,now())
	`, complianceResultID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert ComplianceResult: %v", err)
	}
	for index, evidenceType := range []string{"QUALITY_RESULT", "COMPLIANCE_RESULT"} {
		evidenceID := uuid.New()
		if _, err := pool.Exec(ctx, `
			INSERT INTO evidence (
				id, workspace_id, evidence_type, title, hash_algorithm, hash_value,
				metadata, created_at
			) VALUES ($1,$2,$3,$3,'SHA256',$4,'{}'::jsonb,now())
		`, evidenceID, workspaceID, evidenceType, repeatHex(index+1)); err != nil {
			t.Fatalf("insert Evidence: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO evidence_relation (evidence_id, object_type, object_id, relation_type)
			VALUES ($1,'DATASET_VERSION',$2,'GOVERNANCE_EVIDENCE')
		`, evidenceID, datasetVersionID); err != nil {
			t.Fatalf("insert Evidence relation: %v", err)
		}
	}

	txManager := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	service := application.NewService(txManager, repo)
	product, err := service.CreateProduct(ctx, application.CreateProductCommand{
		WorkspaceID: workspaceID,
		Code:        "DP-READINESS-" + uuid.NewString(),
		Name:        "企业经营活跃度",
		TraceID:     "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	version, err := service.CreateVersion(ctx, application.CreateVersionCommand{
		ProductID:         product.ID,
		MajorVersion:      1,
		MinorVersion:      0,
		PatchVersion:      0,
		ContractVersionID: &contractVersionID,
		Assets: []domain.AssetSpec{{
			AssetType:      domain.AssetDataset,
			Name:           "enterprise_activity_curated",
			DatasetID:      &datasetID,
			DeliveryConfig: map[string]any{"mode": "DATASET"},
		}},
		TraceID: "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("create ProductVersion: %v", err)
	}
	release, err := service.CreateRelease(ctx, application.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: version.ID,
		ReleaseNo:        "R-READINESS-001",
		Datasets:         []domain.ReleaseDataset{{DatasetVersionID: datasetVersionID, Role: domain.DatasetPrimary}},
		TraceID:          "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("create ProductRelease: %v", err)
	}

	result, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          release.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   rightsSnapshotID,
		QualityResultID:    qualityResultID,
		ComplianceResultID: complianceResultID,
		TraceID:            "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("validate ProductRelease: %v", err)
	}
	if result.Overall != "READY" || len(result.Blockers) != 0 {
		t.Fatalf("readiness = %s blockers=%v, want READY without blockers", result.Overall, result.Blockers)
	}
	storedRelease, err := repo.GetRelease(ctx, release.ID)
	if err != nil {
		t.Fatalf("get validated release: %v", err)
	}
	if storedRelease.Status != domain.ReleaseReady {
		t.Fatalf("release status = %s, want READY", storedRelease.Status)
	}

	blockingRelease, err := service.CreateRelease(ctx, application.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: version.ID,
		ReleaseNo:        "R-READINESS-002",
		Datasets:         []domain.ReleaseDataset{{DatasetVersionID: datasetVersionID, Role: domain.DatasetPrimary}},
		TraceID:          "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("create blocking ProductRelease: %v", err)
	}
	badQualityID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, created_at
		) VALUES ($1,$2,$3,'park/quality/enterprise-activity-quality-v1.yaml','1.0.0','FAIL','{}'::jsonb,now())
	`, badQualityID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert blocking QualityResult: %v", err)
	}
	blocked, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          blockingRelease.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   rightsSnapshotID,
		QualityResultID:    badQualityID,
		ComplianceResultID: complianceResultID,
		TraceID:            "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("validate blocking release: %v", err)
	}
	if blocked.Overall != "NOT_READY" || !slices.Contains(blocked.Blockers, "QUALITY_GATE_BLOCKING") {
		t.Fatalf("blocking readiness = %s blockers=%v, want QUALITY_GATE_BLOCKING", blocked.Overall, blocked.Blockers)
	}
	storedBlockingRelease, err := repo.GetRelease(ctx, blockingRelease.ID)
	if err != nil {
		t.Fatalf("get blocking release: %v", err)
	}
	if storedBlockingRelease.Status != domain.ReleaseValidating {
		t.Fatalf("blocking release status = %s, want VALIDATING", storedBlockingRelease.Status)
	}

	// A release may not bind a DatasetVersion owned by another workspace, even when
	// every other governance fact is valid for the product's workspace.
	foreignWorkspaceID := uuid.New()
	foreignDatasetID := uuid.New()
	foreignDatasetVersionID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (
			id, workspace_id, code, name, dataset_type, lifecycle_status, metadata,
			created_at, updated_at
		) VALUES ($1,$2,$3,'Foreign curated dataset','CURATED','ACTIVE','{}'::jsonb,now(),now())
	`, foreignDatasetID, foreignWorkspaceID, "READINESS-FOREIGN-"+uuid.NewString()); err != nil {
		t.Fatalf("insert foreign dataset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, checksum_algorithm, checksum_value, generated_by_execution_id,
			metadata, created_at, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT_STORAGE','s3://test-bucket/foreign.csv',
		          'text/csv','SHA256',$3,$4,'{}'::jsonb,now(),now())
	`, foreignDatasetVersionID, foreignDatasetID, repeatHex(7), uuid.New()); err != nil {
		t.Fatalf("insert foreign DatasetVersion: %v", err)
	}
	foreignRelease, err := service.CreateRelease(ctx, application.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: version.ID,
		ReleaseNo:        "R-READINESS-FOREIGN",
		Datasets:         []domain.ReleaseDataset{{DatasetVersionID: foreignDatasetVersionID, Role: domain.DatasetPrimary}},
		TraceID:          "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("create foreign ProductRelease: %v", err)
	}
	foreignReadiness, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          foreignRelease.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   rightsSnapshotID,
		QualityResultID:    qualityResultID,
		ComplianceResultID: complianceResultID,
		TraceID:            "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("validate foreign release: %v", err)
	}
	if foreignReadiness.Overall != "NOT_READY" || !slices.Contains(foreignReadiness.Blockers, "DATASET_NOT_USABLE") {
		t.Fatalf("foreign readiness = %s blockers=%v, want DATASET_NOT_USABLE", foreignReadiness.Overall, foreignReadiness.Blockers)
	}
	storedForeignRelease, err := repo.GetRelease(ctx, foreignRelease.ID)
	if err != nil {
		t.Fatalf("get foreign release: %v", err)
	}
	if storedForeignRelease.Status != domain.ReleaseValidating {
		t.Fatalf("foreign release status = %s, want VALIDATING", storedForeignRelease.Status)
	}
}

func repeatHex(seed int) string {
	alphabet := "0123456789abcdef"
	value := string(alphabet[seed%len(alphabet)])
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
