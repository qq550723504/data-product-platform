package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

func TestGoldQualityPreflightReadsFinalizedSnapshotWithoutCreatingAssessment(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	router, err := routing.NewRouter(false)
	if err != nil {
		t.Fatalf("routing: %v", err)
	}
	outbox.ConfigureAppendObligation(router)

	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	fx := seedGoldPreflightIntegrationFixture(t, ctx, pool)
	service := NewService(transaction.NewManager(pool), annotationinfra.NewRepository(pool), nil)
	actor := uuid.New()

	reviewed, err := service.ReviewAnnotation(ctx, ReviewAnnotationCommand{
		WorkspaceID:          fx.workspaceID,
		CampaignID:           fx.campaignID,
		TaskID:               fx.taskID,
		ExpectedTaskRevision: 2,
		ReviewerRef:          actor.String(),
		Action:               annotationdomain.ReviewAccept,
		Reason:               "integration verified",
		IdempotencyKey:       "gold-preflight-review-" + uuid.NewString(),
		ReviewedResultID:     &fx.resultID,
		ActorID:              &actor,
		TraceID:              "gold-preflight-integration",
	})
	if err != nil {
		t.Fatalf("review annotation: %v", err)
	}
	if reviewed.Decision == nil || reviewed.Decision.Outcome != annotationdomain.ReviewAccept {
		t.Fatalf("review result = %+v", reviewed)
	}

	snapshot, err := service.FinalizeAnnotationSnapshot(ctx, FinalizeAnnotationSnapshotCommand{
		WorkspaceID: fx.workspaceID,
		CampaignID:  fx.campaignID,
		ActorID:     &actor,
		TraceID:     "gold-preflight-integration",
	})
	if err != nil {
		t.Fatalf("finalize snapshot: %v", err)
	}
	if snapshot.Status != annotationdomain.SnapshotFinalized {
		t.Fatalf("snapshot status=%s", snapshot.Status)
	}

	var qualityBefore int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM quality_result WHERE workspace_id=$1", fx.workspaceID).Scan(&qualityBefore); err != nil {
		t.Fatalf("count quality before: %v", err)
	}

	preflight, err := service.GoldQualityPreflight(ctx, fx.workspaceID, fx.campaignID)
	if err != nil {
		t.Fatalf("GoldQualityPreflight: %v", err)
	}
	if preflight.Blocking {
		t.Fatalf("preflight unexpectedly blocking: %+v", preflight.Findings)
	}
	if preflight.SnapshotID != snapshot.ID || preflight.SnapshotRoot != snapshot.RootHash {
		t.Fatalf("preflight snapshot=%s/%s want=%s/%s", preflight.SnapshotID, preflight.SnapshotRoot, snapshot.ID, snapshot.RootHash)
	}
	if preflight.Metrics["taskCount"] != 1 || preflight.Metrics["usableSelectedCount"] != 1 ||
		preflight.Metrics["rejectedCount"] != 0 || preflight.Metrics["invalidCount"] != 0 {
		t.Fatalf("preflight metrics=%+v", preflight.Metrics)
	}

	var qualityAfter int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM quality_result WHERE workspace_id=$1", fx.workspaceID).Scan(&qualityAfter); err != nil {
		t.Fatalf("count quality after: %v", err)
	}
	if qualityAfter != qualityBefore {
		t.Fatalf("preflight created formal quality assessment: before=%d after=%d", qualityBefore, qualityAfter)
	}
}

type goldPreflightIntegrationFixture struct {
	workspaceID uuid.UUID
	campaignID  uuid.UUID
	taskID      uuid.UUID
	resultID    uuid.UUID
}

func seedGoldPreflightIntegrationFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) goldPreflightIntegrationFixture {
	t.Helper()
	workspaceID := uuid.New()
	resourceID := uuid.New()
	datasetID := uuid.New()
	versionID := uuid.New()
	qualityID := uuid.New()
	profileID := uuid.New()
	certificationID := uuid.New()
	campaignID := uuid.New()
	taskID := uuid.New()
	resultID := uuid.New()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	inputChecksum := strings.Repeat("1", 64)
	specContent := `{"kind":"single-label-v1","labels":["A","B"]}`
	specHash := goldPreflightSHA256([]byte(specContent))

	goldPreflightExec(t, ctx, pool, `
		INSERT INTO data_resource(id, workspace_id, code, name, resource_type, lifecycle_status)
		VALUES ($1,$2,$3,'gold preflight contribution','OTHER','READY')
	`, resourceID, workspaceID, "GP-"+suffix)
	goldPreflightExec(t, ctx, pool, `
		INSERT INTO dataset(id, workspace_id, code, name, dataset_type)
		VALUES ($1,$2,$3,'gold preflight input','CURATED')
	`, datasetID, workspaceID, "GPDS-"+suffix)
	goldPreflightExec(t, ctx, pool, `
		INSERT INTO dataset_version(
			id, dataset_id, version_no, status, storage_type, storage_uri,
			content_type, row_count, checksum_algorithm, checksum_value, ready_at
		) VALUES ($1,$2,1,'READY','OBJECT','test://gold-preflight','application/json',1,'SHA256',$3,now())
	`, versionID, datasetID, inputChecksum)

	ruleContent := "gold-preflight-input-quality"
	goldPreflightExec(t, ctx, pool, `
		INSERT INTO quality_result(
			id, workspace_id, dataset_version_id, rule_set_ref, rule_set_version,
			gate_decision, metrics, rule_set_content_sha256, rule_set_content,
			evaluator_name, evaluator_version
		) VALUES ($1,$2,$3,'gold-preflight-input','1','PASS',$4,$5,$6,'fixture','1')
	`, qualityID, workspaceID, versionID, []byte(`{"dimensions":{}}`), goldPreflightSHA256([]byte(ruleContent)), ruleContent)

	profileContent := "gold-preflight-input-profile"
	profileHash := goldPreflightSHA256([]byte(profileContent))
	goldPreflightExec(t, ctx, pool, `
		INSERT INTO certification_profile(
			id, workspace_id, profile_ref, code, name, version, content_sha256, content_snapshot,
			purpose_mode, action_mode, consumer_mode, delivery_mode,
			quality_gate_required, rights_required, compliance_required, contract_required,
			traceability_required, evidence_required, membership_state
		) VALUES ($1,$2,'gold-preflight-input',$3,'gold preflight input','1',$4,$5,
		          'ANY','ANY','ANY','ANY',true,false,false,false,false,false,'DRAFT')
	`, profileID, workspaceID, "GPP-"+suffix, profileHash, profileContent)
	goldPreflightExec(t, ctx, pool, "UPDATE certification_profile SET membership_state='FINALIZED' WHERE id=$1", profileID)
	goldPreflightExec(t, ctx, pool, `
		INSERT INTO dataset_certification(
			id, workspace_id, dataset_version_id, quality_assessment_id,
			certification_profile_id, profile_ref, profile_version,
			profile_content_sha256, profile_content_snapshot,
			decision, blockers, reason, issued_at
		) VALUES ($1,$2,$3,$4,$5,'gold-preflight-input','1',$6,$7,'CERTIFIED','[]'::jsonb,'fixture',now())
	`, certificationID, workspaceID, versionID, qualityID, profileID, profileHash, profileContent)

	goldPreflightExec(t, ctx, pool, `
		INSERT INTO annotation_campaign(
			id, workspace_id, input_dataset_version_id, input_certification_id,
			annotation_contribution_resource_id, purpose, action,
			schema_ref, schema_version, schema_content_sha256, schema_content_snapshot,
			taxonomy_ref, taxonomy_version, taxonomy_content_sha256, taxonomy_content_snapshot,
			rubric_ref, rubric_version, rubric_content_sha256, rubric_content_snapshot,
			renderer_ref, renderer_version, renderer_content_sha256, renderer_content_snapshot,
			review_policy_ref, review_policy_version, review_policy_content_sha256, review_policy_content_snapshot
		) VALUES (
			$1,$2,$3,$4,$5,'gold-pilot','PROCESS',
			'schema','1',$6,$7,'taxonomy','1',$6,$7,'rubric','1',$6,$7,
			'renderer','1',$6,$7,'review','1',$6,$7
		)
	`, campaignID, workspaceID, versionID, certificationID, resourceID, specHash, specContent)
	goldPreflightExec(t, ctx, pool, `
		INSERT INTO annotation_task(
			id, workspace_id, campaign_id, source_item_ref, source_content_sha256,
			task_text_sha256, primary_annotator_ref
		) VALUES ($1,$2,$3,'row:1',$4,$5,'annotator')
	`, taskID, workspaceID, campaignID, strings.Repeat("a", 64), strings.Repeat("b", 64))
	goldPreflightExec(t, ctx, pool, `
		UPDATE annotation_campaign
		   SET status='ACTIVE', revision=2, expected_task_count=1,
		       task_manifest_hash=$2, input_checksum_sha256=$3, activated_at=now()
		 WHERE id=$1
	`, campaignID, strings.Repeat("c", 64), inputChecksum)

	payload := []byte(`{"label":"A"}`)
	goldPreflightExec(t, ctx, pool, `
		INSERT INTO annotation_result(
			id, workspace_id, campaign_id, task_id, author_ref,
			provider_binding_ref, external_task_id, external_annotation_id, external_revision,
			observation_key, canonical_payload, canonical_payload_sha256, normalizer_version
		) VALUES ($1,$2,$3,$4,'annotator','fixture-provider','gold-preflight-task','gold-preflight-annotation','1',$5,$6,$7,'fixture-v1')
	`, resultID, workspaceID, campaignID, taskID, "gold-preflight:"+uuid.NewString(), payload, goldPreflightSHA256(payload))
	goldPreflightExec(t, ctx, pool, "UPDATE annotation_task SET status='REVIEWABLE', revision=2 WHERE id=$1", taskID)

	return goldPreflightIntegrationFixture{
		workspaceID: workspaceID,
		campaignID:  campaignID,
		taskID:      taskID,
		resultID:    resultID,
	}
}

func goldPreflightExec(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, query, args...); err != nil {
		t.Fatalf("gold preflight fixture SQL: %v", err)
	}
}

func goldPreflightSHA256(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
