// Preparation only: synthetic historical records in an explicitly separate DB.
// No domain commands, engines, dispatch, model, provider or delivery are invoked.
package main

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/migration"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(os.Getenv("READONLY_PREPARE_DSN"))
	must(err)
	if !strings.HasPrefix(cfg.ConnConfig.Database, "agent_q2_acceptance_") {
		log.Fatal("independent database required")
	}
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	must(err)
	defer db.Close()
	root, err := os.Getwd()
	must(err)
	must(migration.NewRunner(db, filepath.Join(root, "migrations")).Up(ctx))
	data, err := os.ReadFile(filepath.Join(root, "tests/agent-quality-live/seed.sql"))
	must(err)
	fixtures, _, ok := strings.Cut(string(data), "-- Core runtime has no DML")
	if !ok {
		log.Fatal("fixture boundary missing")
	}
	_, err = db.Exec(ctx, fixtures)
	must(err)
	tx, err := db.Begin(ctx)
	must(err)
	defer tx.Rollback(ctx)
	ws := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	product := uuid.MustParse("10101010-1010-4010-8010-101010101010")
	version := uuid.MustParse("20202020-2020-4020-8020-202020202020")
	release := uuid.MustParse("30303030-3030-4030-8030-303030303030")
	_, err = tx.Exec(ctx, `INSERT INTO data_product(id,workspace_id,code,name,lifecycle_status,health_status,metadata) VALUES($1,$2,'READONLY-SYNTHETIC','只读验收 · 合成发布','PUBLISHED','HEALTHY','{"syntheticAcceptance":true}');`, product, ws)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO product_version(id,product_id,major_version,minor_version,patch_version,definition_snapshot,build_status,expected_asset_count) VALUES($1,$2,1,0,0,'{"syntheticAcceptance":true}','BUILDING',0)`, version, product)
	must(err)
	_, err = tx.Exec(ctx, `UPDATE product_version SET build_status='FINALIZED' WHERE id=$1`, version)
	must(err)
	snap, err := evidence.CreateSnapshot(ctx, tx, ws, "PRODUCT_RELEASE", release, map[string]any{"syntheticAcceptance": true, "releaseId": release}, []evidence.SnapshotItem{{EvidenceID: uuid.MustParse("a2100007-a101-4101-8101-000000000001"), Category: "SYNTHETIC_QUALITY"}}, nil)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO product_release(id,product_id,product_version_id,release_no,status,evidence_snapshot_id,release_notes,metadata) VALUES($1,$2,$3,'R-READONLY-SYNTHETIC','READY',$4,'合成夹具，不代表真实发布或交付','{"syntheticAcceptance":true}')`, release, product, version, snap.ID)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO product_release_dataset(release_id,dataset_version_id,role) VALUES($1,'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb','PRIMARY')`, release)
	must(err)
	_, err = tx.Exec(ctx, `UPDATE product_release SET status='PUBLISHED',released_at=now() WHERE id=$1`, release)
	must(err)
	_, err = tx.Exec(ctx, `INSERT INTO audit_event(workspace_id,actor_type,action,object_type,object_id,metadata) VALUES($1,'SYSTEM','SYNTHETIC_ACCEPTANCE_FIXTURE','PRODUCT_RELEASE',$2,'{"syntheticAcceptance":true}')`, ws, release)
	must(err)
	must(tx.Commit(ctx))
	log.Print("Independent synthetic read fixture prepared; no engine or execution invoked")
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
