package acceptance_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/config"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/storage"
)

// TestBrowserCSVIngest creates no business fixture through SQL or application
// commands. All resources, versions and the entity job are created in the UI.
func TestBrowserCSVIngest(t *testing.T) {
	if os.Getenv("LIVE_BROWSER_ACCEPTANCE") != "1" {
		t.Skip("opt-in isolated browser CSV acceptance")
	}
	t.Setenv("INDUSTRY_PACK_ROOT", repoPath(t, "industry-packs"))
	cfg, err := config.Load()
	liveOK(t, err, "load CSV acceptance config")
	liveOK(t, validateLiveConfig(cfg), "refuse non-isolated CSV target")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	pool, err := database.Open(ctx, cfg.PostgresDSN)
	liveOK(t, err, "open CSV acceptance database")
	defer pool.Close()
	artifacts := repoPath(t, ".artifacts", "live-browser")
	liveJSONFile(t, filepath.Join(artifacts, "ingest-verification.json"), map[string]any{"verified": false})
	listener, err := net.Listen("tcp", "127.0.0.1:18080")
	liveOK(t, err, "CSV API port must be unused")
	liveOK(t, listener.Close(), "release CSV API port")
	api := startLiveProcess(t, ctx, filepath.Join(artifacts, "platform-api"), repoPath(t, "apps", "platform"), filepath.Join(artifacts, "ingest-api.log"))
	workspace, actor, reviewer, publisher := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	liveWaitAPI(t, ctx, api, workspace)
	nonce := uuid.NewString()[:8]
	name, reason := "UI-CSV-"+nonce, "INGEST_REVIEW_"+nonce
	// Quoted first header plus BOM catches the previous Go CSV reader mismatch.
	text := fmt.Sprintf("\ufeff\"source_company_id\",\"company_name\",unified_social_credit_code,legal_representative,registered_address,entry_date,company_status\r\nA-%[1]s,深圳澄明科技%[1]s有限公司,91440300CSV%[1]s,合成甲%[1]s,合成园区%[1]s号,2025-01-01,ACTIVE\r\nB-%[1]s,深圳市澄明科技%[1]s有限公司,,合成甲%[1]s,合成园区%[1]s号,2025-01-01,ACTIVE\r\n", nonce)
	assertLiveCount(t, ctx, pool, 0, `SELECT count(*) FROM dataset WHERE workspace_id=$1`, workspace)
	runLiveBrowser(t, ctx, "ingest", map[string]any{"workspaceId": workspace, "ingestActorId": actor, "reviewerId": reviewer, "publisherId": publisher, "name": name, "reason": reason, "csvText": text}, artifacts)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM data_resource WHERE workspace_id=$1`, workspace)
	assertLiveCount(t, ctx, pool, 2, `SELECT count(*) FROM dataset WHERE workspace_id=$1`, workspace)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM entity_match_job WHERE workspace_id=$1`, workspace)
	assertLiveCount(t, ctx, pool, 0, `SELECT count(*) FROM data_product WHERE workspace_id=$1`, workspace)
	var rawID, rawVersionID uuid.UUID
	var checksum, uri string
	var rowCount, byteSize int64
	liveOK(t, pool.QueryRow(ctx, `SELECT d.id,dv.id,dv.checksum_value,dv.storage_uri,dv.row_count,dv.byte_size FROM dataset d JOIN dataset_version dv ON dv.dataset_id=d.id WHERE d.workspace_id=$1 AND d.dataset_type='RAW' AND d.name=$2 AND dv.status='READY'`, workspace, name).Scan(&rawID, &rawVersionID, &checksum, &uri, &rowCount, &byteSize), "read browser-created RAW version")
	if rowCount != 2 || byteSize != int64(len([]byte(text))) || checksum != fmt.Sprintf("%x", sha256.Sum256([]byte(text))) {
		t.Fatal("RAW row count, size or checksum differs from the selected original file")
	}
	store, err := storage.New(cfg.Storage.Endpoint, cfg.Storage.AccessKey, cfg.Storage.SecretKey, cfg.Storage.Bucket, cfg.Storage.UseSSL)
	liveOK(t, err, "create CSV object reader")
	if !bytes.Equal(readLiveObject(t, ctx, store, uri), []byte(text)) {
		t.Fatal("MinIO did not retain the exact original UTF-8 BOM CSV bytes")
	}
	var jobID, outputVersionID uuid.UUID
	var jobStatus string
	liveOK(t, pool.QueryRow(ctx, `SELECT id,status,output_dataset_version_id FROM entity_match_job WHERE workspace_id=$1 AND input_dataset_version_id=$2`, workspace, rawVersionID).Scan(&jobID, &jobStatus, &outputVersionID), "read browser-created resolution")
	if jobStatus != "SUCCEEDED" || outputVersionID == uuid.Nil {
		t.Fatal("manual browser review did not produce a successful output version")
	}
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM dataset_version_lineage WHERE output_version_id=$1 AND input_version_id=$2`, outputVersionID, rawVersionID)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM entity_match_candidate WHERE job_id=$1 AND status='CONFIRMED' AND reviewed_by=$2 AND reviewer_reason=$3 AND evidence_id IS NOT NULL`, jobID, reviewer, reason)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM entity_mapping em JOIN entity e ON e.id=em.entity_id WHERE e.workspace_id=$1 AND em.status='CONFIRMED' AND em.reviewed_by=$2 AND em.reviewer_reason=$3 AND em.evidence_id IS NOT NULL`, workspace, reviewer, reason)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='ENTITY_MATCH_JOB_STARTED' AND actor_id=$2`, jobID, actor)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM audit_event WHERE object_id=$1 AND action='DATASET_VERSION_READY' AND actor_id=$2`, rawVersionID, actor)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM audit_event WHERE workspace_id=$1 AND action='ENTITY_MATCH_CONFIRMED' AND actor_id=$2 AND reason=$3`, workspace, reviewer, reason)
	assertLiveCount(t, ctx, pool, 1, `SELECT count(*) FROM evidence WHERE workspace_id=$1 AND evidence_type='ENTITY_MATCH_REVIEW' AND created_by=$2 AND metadata->>'reviewerReason'=$3`, workspace, reviewer, reason)
	liveJSONFile(t, filepath.Join(artifacts, "ingest-verification.json"), map[string]any{"verified": true, "workspaceId": workspace, "rawDatasetId": rawID, "rawVersionId": rawVersionID, "outputVersionId": outputVersionID, "jobId": jobID, "checksum": checksum, "rowCount": rowCount, "originalBOMBytesPreserved": true, "allBusinessCreationViaBrowser": true, "reviewAndAuditPersisted": true, "duplicateResolutionBlocked": true, "automaticProductPublication": false})
}
