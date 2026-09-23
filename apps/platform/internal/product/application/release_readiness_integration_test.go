package application_test

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
	rightsapp "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/application"
	rightsinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/infrastructure"
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
		          NULL,'{}'::jsonb,now(),now())
	`, datasetVersionID, datasetID); err != nil {
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
		) VALUES ($1,$2,$3,'PARK-OPERATOR','LICENSED_BANK','ENTERPRISE_CREDIT_RISK_SUPPORT','ACTIVE',
		          $4,$5,'{}'::jsonb,now(),now())
	`, authorizationID, workspaceID, "AUTH-"+uuid.NewString(), validFrom, validTo); err != nil {
		t.Fatalf("insert Authorization: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO rights_snapshot (
			id, workspace_id, purpose, consumer_ref, as_of, manifest, root_hash, created_at, status
		) VALUES ($1,$2,'ENTERPRISE_CREDIT_RISK_SUPPORT','LICENSED_BANK',now(),
		          '{"purpose":"ENTERPRISE_CREDIT_RISK_SUPPORT","consumerRef":"LICENSED_BANK","authorizations":[]}'::jsonb,
		          'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd',now(),'BUILDING')
	`, rightsSnapshotID, workspaceID); err != nil {
		t.Fatalf("insert RightsSnapshot: %v", err)
	}
	insertRightsProvenanceFixture(t, ctx, pool, workspaceID, authorizationID, rightsSnapshotID)
	if _, err := pool.Exec(ctx, `UPDATE rights_snapshot SET status='FINALIZED' WHERE id=$1`, rightsSnapshotID); err != nil {
		t.Fatalf("finalize RightsSnapshot: %v", err)
	}

	qualityResultID := uuid.New()
	complianceResultID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics, created_at
		) VALUES ($1,$2,$3,'park/quality/enterprise-activity-quality-v1.yaml','1.0.0',
			'c4b903018effbb6d36545f03ec3a6513aa3d5b35f12f42a50c150dc4f1ea35dc',
			'legacy-quality-fixture','native-quality','1','PASS','{"dimensions":{}}'::jsonb,now())
	`, qualityResultID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert QualityResult: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO compliance_result (
			id, workspace_id, dataset_version_id, policy_ref, policy_version,
			gate_decision, summary, created_at
		) VALUES ($1,$2,$3,'park/compliance/enterprise-activity-compliance-v1.yaml','1.0.0','PASS','{"dimensions":{}}'::jsonb,now())
	`, complianceResultID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert ComplianceResult: %v", err)
	}
	for _, evidenceType := range []string{"QUALITY_RESULT", "COMPLIANCE_RESULT"} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin Evidence fixture: %v", err)
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  workspaceID,
			EvidenceType: evidenceType,
			Title:        evidenceType,
		}, evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: datasetVersionID, RelationType: "GOVERNANCE_EVIDENCE"}); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("append Evidence fixture: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit Evidence fixture: %v", err)
		}
	}

	txManager := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	service := application.NewService(txManager, repo)
	rightsService := rightsapp.NewService(txManager, rightsinfra.NewPostgresRepository(pool))
	createReleaseSnapshot := func(releaseID uuid.UUID) uuid.UUID {
		t.Helper()
		snapshot, err := rightsService.CreateSnapshot(ctx, rightsapp.CreateSnapshotCommand{
			WorkspaceID:      workspaceID,
			ProductReleaseID: &releaseID,
			Purpose:          "ENTERPRISE_CREDIT_RISK_SUPPORT",
			ConsumerRef:      "LICENSED_BANK",
			AuthorizationIDs: []uuid.UUID{authorizationID},
			TraceID:          "release-readiness-rights",
		})
		if err != nil {
			t.Fatalf("create release-bound RightsSnapshot: %v", err)
		}
		return snapshot.ID
	}
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
	releaseRightsSnapshotID := createReleaseSnapshot(release.ID)

	result, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          release.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   releaseRightsSnapshotID,
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

	mismatchRelease, err := service.CreateRelease(ctx, application.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: version.ID,
		ReleaseNo:        "R-READINESS-RIGHTS-MISMATCH",
		Datasets:         []domain.ReleaseDataset{{DatasetVersionID: datasetVersionID, Role: domain.DatasetPrimary}},
		TraceID:          "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("create rights-mismatch ProductRelease: %v", err)
	}
	mismatchReadiness, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          mismatchRelease.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   releaseRightsSnapshotID,
		QualityResultID:    qualityResultID,
		ComplianceResultID: complianceResultID,
		TraceID:            "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("validate rights-mismatch release: %v", err)
	}
	if mismatchReadiness.Overall != "NOT_READY" || !slices.Contains(mismatchReadiness.Blockers, "RIGHTS_SNAPSHOT_RELEASE_MISMATCH") {
		t.Fatalf("rights-mismatch readiness = %s blockers=%v, want RIGHTS_SNAPSHOT_RELEASE_MISMATCH", mismatchReadiness.Overall, mismatchReadiness.Blockers)
	}

	contextMismatchRelease, err := service.CreateRelease(ctx, application.CreateReleaseCommand{
		ProductID:        product.ID,
		ProductVersionID: version.ID,
		ReleaseNo:        "R-READINESS-RIGHTS-CONTEXT-MISMATCH",
		Datasets:         []domain.ReleaseDataset{{DatasetVersionID: datasetVersionID, Role: domain.DatasetPrimary}},
		TraceID:          "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("create rights-context-mismatch ProductRelease: %v", err)
	}
	contextMismatchSnapshotID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO rights_snapshot (
			id, workspace_id, product_release_id, purpose, consumer_ref, as_of,
			manifest, root_hash, created_at, status
		) VALUES (
			$1,$2,$3,'ENTERPRISE_CREDIT_RISK_SUPPORT','LICENSED_BANK',now(),
			jsonb_build_object(
				'purpose','ENTERPRISE_CREDIT_RISK_SUPPORT',
				'consumerRef','LICENSED_BANK',
				'authorizations',jsonb_build_array(
					jsonb_build_object(
						'authorizationId',$4::text,
						'code','FROZEN-CONTEXT-MISMATCH',
						'grantorRef','PARK-OPERATOR',
						'granteeRef','WRONG_BANK',
						'purpose','ENTERPRISE_CREDIT_RISK_SUPPORT',
						'resources',jsonb_build_array()
					)
				)
			),
			$5,now(),'BUILDING'
		)
	`, contextMismatchSnapshotID, workspaceID, contextMismatchRelease.ID, authorizationID, repeatHex(12)); err != nil {
		t.Fatalf("insert context-mismatch RightsSnapshot: %v", err)
	}
	insertRightsProvenanceFixture(t, ctx, pool, workspaceID, authorizationID, contextMismatchSnapshotID)
	if _, err := pool.Exec(ctx, `UPDATE rights_snapshot SET status='FINALIZED' WHERE id=$1`, contextMismatchSnapshotID); err != nil {
		t.Fatalf("finalize context-mismatch RightsSnapshot: %v", err)
	}
	contextMismatchReadiness, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          contextMismatchRelease.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   contextMismatchSnapshotID,
		QualityResultID:    qualityResultID,
		ComplianceResultID: complianceResultID,
		TraceID:            "release-readiness-e2e",
	})
	if err != nil {
		t.Fatalf("validate rights-context-mismatch release: %v", err)
	}
	if contextMismatchReadiness.Overall != "NOT_READY" || !slices.Contains(contextMismatchReadiness.Blockers, "RIGHTS_CONTEXT_MISMATCH") {
		t.Fatalf("rights-context-mismatch readiness = %s blockers=%v, want RIGHTS_CONTEXT_MISMATCH", contextMismatchReadiness.Overall, contextMismatchReadiness.Blockers)
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
	blockingRightsSnapshotID := createReleaseSnapshot(blockingRelease.ID)
	badQualityID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			rule_set_content_sha256, rule_set_content, evaluator_name, evaluator_version,
			gate_decision, metrics, created_at
		) VALUES ($1,$2,$3,'park/quality/enterprise-activity-quality-v1.yaml','1.0.0',
			'c4b903018effbb6d36545f03ec3a6513aa3d5b35f12f42a50c150dc4f1ea35dc',
			'legacy-quality-fixture','native-quality','1','FAIL','{"dimensions":{}}'::jsonb,now())
	`, badQualityID, workspaceID, datasetVersionID); err != nil {
		t.Fatalf("insert blocking QualityResult: %v", err)
	}
	blocked, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          blockingRelease.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   blockingRightsSnapshotID,
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
	foreignRightsSnapshotID := createReleaseSnapshot(foreignRelease.ID)
	foreignReadiness, err := service.ValidateRelease(ctx, application.ValidateReleaseCommand{
		ReleaseID:          foreignRelease.ID,
		ContractVersionID:  contractVersionID,
		RightsSnapshotID:   foreignRightsSnapshotID,
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
