package application_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

func TestPublishReleaseCreatesOneImmutableEvidenceSnapshotAndIsIdempotent(t *testing.T) {
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
	productID := uuid.New()
	productVersionID := uuid.New()
	datasetID := uuid.New()
	datasetVersionID := uuid.New()
	contractID := uuid.New()
	contractVersionID := uuid.New()
	authorizationID := uuid.New()
	rightsSnapshotID := uuid.New()
	qualityResultID := uuid.New()
	complianceResultID := uuid.New()
	releaseID := uuid.New()
	executionID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO dataset (id, workspace_id, code, name, dataset_type, lifecycle_status, metadata, created_at, updated_at)
		VALUES ($1,$2,$3,'Enterprise Activity','CURATED','ACTIVE','{}'::jsonb,now(),now());
		INSERT INTO dataset_version (
			id, dataset_id, version_no, status, storage_type, storage_uri, content_type,
			checksum_algorithm, checksum_value, generated_by_execution_id, metadata, created_at, ready_at
		) VALUES ($4,$1,1,'READY','OBJECT_STORAGE','s3://test-bucket/enterprise-activity.csv','text/csv',
		          'SHA256','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',$5,
		          '{"workflowVersion":"1.0.0","indicatorSet":"park-enterprise-activity@1.0.0","entityPolicyVersion":"1.0.0"}'::jsonb,now(),now())
	`, datasetID, workspaceID, "PUBLISH-DATASET-"+uuid.NewString(), datasetVersionID, executionID); err != nil {
		t.Fatalf("insert dataset fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO data_contract (id, workspace_id, code, name, product_code)
		VALUES ($1,$2,$3,'Enterprise Activity Contract','DP-ENTERPRISE-ACTIVITY');
		INSERT INTO contract_version (
			id, contract_id, major_version, minor_version, patch_version, status, document,
			source_ref, source_sha256, created_at, published_at
		) VALUES ($4,$1,1,0,0,'PUBLISHED','{"spec":{"product":{"code":"DP-ENTERPRISE-ACTIVITY"}}}'::jsonb,
		          'examples/enterprise-activity/contract/data-contract-v1.yaml',
		          'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',now(),now())
	`, contractID, workspaceID, "PUBLISH-CONTRACT-"+uuid.NewString(), contractVersionID); err != nil {
		t.Fatalf("insert contract fixture: %v", err)
	}

	validFrom := time.Now().UTC().Add(-time.Hour)
	validTo := time.Now().UTC().Add(time.Hour)
	if _, err := pool.Exec(ctx, `
		INSERT INTO data_authorization (
			id, workspace_id, code, grantor_ref, grantee_ref, purpose, status,
			valid_from, valid_to, metadata, created_at, updated_at
		) VALUES ($1,$2,$3,'PARK-OPERATOR','DATA-PRODUCT-PLATFORM','ENTERPRISE_CREDIT_RISK_SUPPORT','ACTIVE',
		          $4,$5,'{}'::jsonb,now(),now());
		INSERT INTO rights_snapshot (
			id, workspace_id, purpose, consumer_ref, as_of, manifest, root_hash, created_at
		) VALUES ($6,$2,'ENTERPRISE_CREDIT_RISK_SUPPORT','LICENSED_BANK',now(),
		          '{"purpose":"ENTERPRISE_CREDIT_RISK_SUPPORT","authorizations":[]}'::jsonb,
		          'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc',now());
		INSERT INTO rights_snapshot_authorization (rights_snapshot_id, authorization_id) VALUES ($6,$1)
	`, authorizationID, workspaceID, "PUBLISH-AUTH-"+uuid.NewString(), validFrom, validTo, rightsSnapshotID); err != nil {
		t.Fatalf("insert rights fixture: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO quality_result (
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version, gate_decision, metrics, created_at
		) VALUES ($1,$2,$3,'park/quality/enterprise-activity-quality-v1.yaml','1.0.0','PASS','{}'::jsonb,now());
		INSERT INTO compliance_result (
			id, workspace_id, dataset_version_id, policy_ref, policy_version, gate_decision, summary, created_at
		) VALUES ($4,$2,$3,'park/compliance/enterprise-activity-compliance-v1.yaml','1.0.0','PASS','{}'::jsonb,now())
	`, qualityResultID, workspaceID, datasetVersionID, complianceResultID); err != nil {
		t.Fatalf("insert governance results: %v", err)
	}

	for i, evidenceType := range []string{"QUALITY_RESULT", "COMPLIANCE_RESULT"} {
		evidenceID := uuid.New()
		if _, err := pool.Exec(ctx, `
			INSERT INTO evidence (
				id, workspace_id, evidence_type, title, source_type, source_id,
				hash_algorithm, hash_value, metadata, created_at
			) VALUES ($1,$2,$3,$3,$3,$4,'SHA256',$5,'{}'::jsonb,now());
			INSERT INTO evidence_relation (evidence_id, object_type, object_id, relation_type)
			VALUES ($1,'DATASET_VERSION',$6,$7)
		`, evidenceID, workspaceID, evidenceType,
			[]uuid.UUID{qualityResultID, complianceResultID}[i],
			repeatHex(i+3), datasetVersionID, evidenceType+"_EVIDENCE"); err != nil {
			t.Fatalf("insert evidence fixture: %v", err)
		}
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO data_product (
			id, workspace_id, code, name, lifecycle_status, health_status, metadata,
			created_at, updated_at
		) VALUES ($1,$2,$3,'企业经营活跃度','READY','HEALTHY','{}'::jsonb,now(),now());
		INSERT INTO product_version (
			id, product_id, major_version, minor_version, patch_version, status,
			contract_version_id, definition_snapshot, created_at
		) VALUES ($4,$1,1,0,0,'DRAFT',$5,'{"reference":"enterprise-activity"}'::jsonb,now());
		INSERT INTO product_asset (
			id, product_version_id, asset_type, name, dataset_id, delivery_config, created_at
		) VALUES ($6,$4,'DATASET','enterprise_activity_curated',$7,'{"mode":"DATASET"}'::jsonb,now());
		INSERT INTO product_release (
			id, product_id, product_version_id, release_no, status,
			contract_version_id, rights_snapshot_id, quality_result_id, compliance_result_id,
			metadata, created_at
		) VALUES ($8,$1,$4,'R-PUBLISH-001','READY',$5,$9,$10,$11,'{}'::jsonb,now());
		INSERT INTO product_release_dataset (release_id, dataset_version_id, role)
		VALUES ($8,$12,'PRIMARY')
	`, productID, workspaceID, "DP-PUBLISH-"+uuid.NewString(), productVersionID, contractVersionID,
		uuid.New(), datasetID, releaseID, rightsSnapshotID, qualityResultID, complianceResultID, datasetVersionID); err != nil {
		t.Fatalf("insert product/release fixture: %v", err)
	}

	txManager := transaction.NewManager(pool)
	repo := infrastructure.NewPostgresRepository(pool)
	service := application.NewService(txManager, repo)
	key := "publish-" + uuid.NewString()
	published, err := service.PublishRelease(ctx, application.PublishReleaseCommand{
		ReleaseID:      releaseID,
		IdempotencyKey: key,
		TraceID:        "publish-e2e",
	})
	if err != nil {
		t.Fatalf("publish release: %v", err)
	}
	if published.Status != domain.ReleasePublished || published.EvidenceSnapshotID == nil {
		t.Fatalf("published release = status %s evidenceSnapshot %v", published.Status, published.EvidenceSnapshotID)
	}

	second, err := service.PublishRelease(ctx, application.PublishReleaseCommand{
		ReleaseID:      releaseID,
		IdempotencyKey: key,
		TraceID:        "publish-e2e-retry",
	})
	if err != nil {
		t.Fatalf("repeat idempotent publish: %v", err)
	}
	if second.EvidenceSnapshotID == nil || *second.EvidenceSnapshotID != *published.EvidenceSnapshotID {
		t.Fatalf("idempotent publish changed EvidenceSnapshot: first=%v second=%v", published.EvidenceSnapshotID, second.EvidenceSnapshotID)
	}

	var snapshotCount, idempotencyCount, releasedEventCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence_snapshot WHERE object_type='PRODUCT_RELEASE' AND object_id=$1`, releaseID).Scan(&snapshotCount); err != nil {
		t.Fatalf("count EvidenceSnapshots: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM command_idempotency WHERE object_id=$1 AND command_type='PUBLISH_PRODUCT_RELEASE'`, releaseID).Scan(&idempotencyCount); err != nil {
		t.Fatalf("count idempotency records: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE aggregate_type='PRODUCT_RELEASE' AND aggregate_id=$1 AND event_type='ProductReleased'`, releaseID).Scan(&releasedEventCount); err != nil {
		t.Fatalf("count ProductReleased events: %v", err)
	}
	if snapshotCount != 1 || idempotencyCount != 1 || releasedEventCount != 1 {
		t.Fatalf("publish artifacts = snapshots %d idempotency %d events %d, want 1/1/1", snapshotCount, idempotencyCount, releasedEventCount)
	}

	if _, err := pool.Exec(ctx, `UPDATE evidence_snapshot SET root_hash='tampered' WHERE id=$1`, *published.EvidenceSnapshotID); err == nil {
		t.Fatal("EvidenceSnapshot mutation unexpectedly succeeded")
	}
}
