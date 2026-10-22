package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

// SQL-only inspection: no SDK, mutation, provider request or credential renewal.
// The stopped-plan consumes this bound observation; activation rechecks it live.
func admit(ctx context.Context, pool *pgxpool.Pool, cfg config, in input) (any, error) {
	if (in.Schema != 68 && in.Schema != 71) || len(in.Versions) != 5 {
		return nil, fmt.Errorf("bounded ledger admission required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var applied []int
	if err = tx.QueryRow(ctx, `SELECT array_agg(version::int ORDER BY version) FROM schema_migration`).Scan(&applied); err != nil || len(applied) == 0 || applied[len(applied)-1] != in.Schema || !reflect.DeepEqual(applied, in.SchemaVersions) {
		return nil, fmt.Errorf("exact migration history required")
	}
	rows, err := tx.Query(ctx, `SELECT v.id,v.checksum_value,v.byte_size,v.storage_uri,d.dataset_type FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1 AND v.status='READY' AND v.checksum_algorithm='SHA256' ORDER BY v.id`, cfg.Workspace)
	if err != nil {
		return nil, err
	}
	actual := map[uuid.UUID]expectedVersion{}
	kinds := map[string]int{}
	for rows.Next() {
		var v expectedVersion
		var kind string
		if err = rows.Scan(&v.ID, &v.SHA256, &v.Bytes, &v.URI, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		actual[v.ID] = v
		kinds[kind]++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	var total int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dataset_version v JOIN dataset d ON d.id=v.dataset_id WHERE d.workspace_id=$1`, cfg.Workspace).Scan(&total); err != nil {
		return nil, err
	}
	if total != 5 || len(actual) != 5 || kinds["RAW"] != 3 || kinds["STANDARDIZED"] != 1 || kinds["CURATED"] != 1 {
		return nil, fmt.Errorf("exact original five versions required")
	}
	seen := map[uuid.UUID]bool{}
	for _, v := range in.Versions {
		if v.ID == uuid.Nil || seen[v.ID] || !reflect.DeepEqual(actual[v.ID], v) || !strings.HasPrefix(v.URI, "s3://raw/") {
			return nil, fmt.Errorf("original immutable version drift")
		}
		seen[v.ID] = true
	}
	result := map[string]any{"status": "PASS", "schema": in.Schema, "schemaVersions": applied, "versions": in.Versions, "workspaceId": cfg.Workspace}
	if in.CustodyID != uuid.Nil || in.CustodyPlan != nil || in.ManifestSHA256 != "" {
		if in.Schema != 71 || in.CustodyID == uuid.Nil || in.CustodyPlan == nil || in.CustodyPlan.Validate() != nil || in.CustodyPlan.WorkspaceID != cfg.Workspace || in.CustodyPlan.ActorID != cfg.Actor || in.CustodyPlan.PrincipalRef != cfg.Profile.Subject || in.CustodyPlan.Endpoint != cfg.Profile.StorageEndpoint {
			return nil, fmt.Errorf("same accepted custody scope required")
		}
		var p lc.CustodyPlan
		var manifest, requestHash string
		err = tx.QueryRow(ctx, `SELECT a.workspace_id,a.actor_id,a.principal_ref,a.run_id,a.provider_witness,a.endpoint,a.output_directory,a.plan_sha256,a.expires_at,
   a.first_version_id,a.first_uri,a.first_sha256,a.first_bytes,a.second_version_id,a.second_uri,a.second_sha256,a.second_bytes,t.manifest_sha256,a.request_hash
   FROM operator_custody_acquisition a JOIN operator_custody_termination t ON t.acquisition_id=a.id AND t.workspace_id=a.workspace_id
   WHERE a.id=$1 AND a.workspace_id=$2 AND t.outcome='ACCEPTED' AND t.owner_verified_quiescent`, in.CustodyID, cfg.Workspace).Scan(
			&p.WorkspaceID, &p.ActorID, &p.PrincipalRef, &p.RunID, &p.ProviderWitness, &p.Endpoint, &p.OutputDirectory, &p.PlanSHA256, &p.ExpiresAt,
			&p.First.VersionID, &p.First.URI, &p.First.SHA256, &p.First.Bytes, &p.Second.VersionID, &p.Second.URI, &p.Second.SHA256, &p.Second.Bytes, &manifest, &requestHash)
		original, _ := json.Marshal(in.CustodyPlan)
		originalHash := sha256.Sum256(original)
		if requestHash != hex.EncodeToString(originalHash[:]) {
			return nil, fmt.Errorf("original custody request identity drift")
		}
		// pgx timestamptz encodes microseconds. Retain the original plan/hash;
		// compare only its persisted projection at the database's precision.
		if err != nil || p.WorkspaceID != in.CustodyPlan.WorkspaceID || p.ActorID != in.CustodyPlan.ActorID || p.PrincipalRef != in.CustodyPlan.PrincipalRef || p.RunID != in.CustodyPlan.RunID || p.ProviderWitness != in.CustodyPlan.ProviderWitness || p.Endpoint != in.CustodyPlan.Endpoint || p.OutputDirectory != in.CustodyPlan.OutputDirectory || p.PlanSHA256 != in.CustodyPlan.PlanSHA256 || !p.ExpiresAt.Equal(in.CustodyPlan.ExpiresAt.Truncate(time.Microsecond)) || p.First != in.CustodyPlan.First || p.Second != in.CustodyPlan.Second || manifest != in.ManifestSHA256 {
			return nil, fmt.Errorf("Core accepted custody identity/manifest mismatch")
		}
		var n int
		err = tx.QueryRow(ctx, `SELECT count(*) FROM operator_custody_call c JOIN operator_custody_observation o ON o.call_id=c.id AND o.workspace_id=c.workspace_id
    WHERE c.acquisition_id=$1 AND c.workspace_id=$2 AND o.failure='' AND o.http_status=200 AND o.eof AND o.close_ok AND o.invocation_certainty='CONFIRMED'
    AND EXISTS(SELECT 1 FROM cost_allocation ca JOIN cost_event ce ON ce.id=ca.cost_event_id WHERE ca.operator_custody_call_id=c.id AND ce.workspace_id=c.workspace_id AND ce.activity_id=c.id AND ce.cost_type='OPERATOR_CUSTODY_INVOCATION')`, in.CustodyID, cfg.Workspace).Scan(&n)
		if err != nil || n != 2 {
			return nil, fmt.Errorf("two observed and costed calls required")
		}
		result["custodyId"] = in.CustodyID
		result["custodyPlan"] = in.CustodyPlan
		result["manifestSha256"] = manifest
	}
	return result, tx.Commit(ctx)
}
