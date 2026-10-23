package migration_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/infrastructure"
)

// Uses a disposable database, never the retained Q1 acceptance database.
func TestAgentPlanningAtomicFreezeAndHistoricalDownGuard(t *testing.T) {
	pool := scratchDatabase(t, 73)
	ctx := context.Background()
	applyMigrationFile(t, pool, 73, "down")
	if _, err := pool.Exec(ctx, `DELETE FROM schema_migration WHERE version=73`); err != nil {
		t.Fatal(err)
	}
	applyMigrationFile(t, pool, 73, "up")
	ws, ds, version, task, planID, actor := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO dataset(id,workspace_id,code,name,dataset_type) VALUES($1,$2,'AGENT-TEST','Synthetic Agent source','RAW')`, ds, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO dataset_version(id,dataset_id,version_no,status,storage_uri,checksum_algorithm,checksum_value,ready_at) VALUES($1,$2,1,'READY','s3://agent-test/source.csv','SHA256',$3,now())`, version, ds, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	p := domain.Plan{ID: planID, TaskID: task, WorkspaceID: ws, Ordinal: 1, Phase: domain.Diagnosis, SourceVersionID: version, TemplateRef: "test/plan", TemplateVersion: "1", TemplateHash: strings.Repeat("a", 64), DetectionRuleRef: "test/detect", DetectionRuleHash: strings.Repeat("b", 64), WorkflowHash: strings.Repeat("c", 64), RecheckRuleRef: "test/recheck", RecheckRuleHash: strings.Repeat("d", 64), Goal: "Check source", AcceptanceText: "Findings with evidence", Inputs: []domain.Input{{Name: "source", VersionID: version, Required: true}}, Actions: []domain.Action{{Ordinal: 1, Command: "DETECT", Phase: domain.Diagnosis}}}
	if err := p.Freeze(); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO agent_task(id,workspace_id,kind,created_by,subject,current_plan_revision_id,revision,control_state) VALUES($1,$2,'QUALITY_DIAGNOSIS',$3,'test/operator',$4,1,'ACTIVE')`, task, ws, actor, planID); err != nil {
		t.Fatal(err)
	}
	repo := infrastructure.Repository{Pool: pool}
	// A separate transaction starts before atomic finalization. It cannot read
	// an externally visible DRAFT and cannot append a late member after commit.
	late, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Rollback(ctx)
	var visible int
	if err = late.QueryRow(ctx, `SELECT count(*) FROM agent_plan_revision WHERE id=$1`, planID).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("uncommitted draft became visible", visible, err)
	}
	if err = repo.InsertPlan(ctx, tx, p); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = late.Exec(ctx, `INSERT INTO agent_plan_input(plan_revision_id,input_name,dataset_version_id,required) VALUES($1,'late-transaction',$2,true)`, planID, version); err == nil {
		t.Fatal("pre-freeze transaction crossed finalization")
	}
	late.Rollback(ctx)
	read, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.PlanTx(ctx, read, task, planID)
	if err != nil {
		t.Fatal(err)
	}
	read.Rollback(ctx)
	if got.ContentHash != p.ContentHash || got.TaskID != task || got.Inputs[0].VersionID != version {
		t.Fatal("persisted plan differs")
	}
	for name, sql := range map[string]string{
		"header":        `UPDATE agent_plan_revision SET goal='changed' WHERE id=$1`,
		"input update":  `UPDATE agent_plan_input SET input_name='changed' WHERE plan_revision_id=$1`,
		"input delete":  `DELETE FROM agent_plan_input WHERE plan_revision_id=$1`,
		"late input":    `INSERT INTO agent_plan_input(plan_revision_id,input_name,dataset_version_id,required) SELECT $1,'late',source_version_id,true FROM agent_plan_revision WHERE id=$1`,
		"action delete": `DELETE FROM agent_plan_action WHERE plan_revision_id=$1`,
		"plan delete":   `DELETE FROM agent_plan_revision WHERE id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, sql, planID); err == nil {
				t.Fatal("frozen mutation accepted")
			}
		})
	}
	if err := tryApplyMigrationFile(t, pool, 73, "down"); err == nil {
		t.Fatal("down erased plan history")
	}
	draft, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = draft.Exec(ctx, `INSERT INTO agent_plan_revision SELECT (jsonb_populate_record(NULL::agent_plan_revision,to_jsonb(p)||jsonb_build_object('id',$2::uuid,'ordinal',2,'frozen',false))).* FROM agent_plan_revision p WHERE id=$1`, planID, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err = draft.Commit(ctx); err == nil {
		t.Fatal("DRAFT committed outside atomic creation")
	}
}
