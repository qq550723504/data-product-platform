package application

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func TestFixtureBasisCompleteNineCopiesSingleApprovedSource(t *testing.T) {
	pool := planDatabase(t)
	f := newDisposalFixture(t, pool)
	ctx := context.Background()
	ns := uuid.New()
	c := f.copies[0]
	if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_namespace(id,workspace_id,provider_instance_id,incarnation_id,namespace) VALUES($1,$2,$3,$4,'synthetic-quarantine')`, ns, f.w, c.ProviderInstanceID, c.IncarnationID); err != nil {
		t.Fatal(err)
	}
	ids := []uuid.UUID{f.copies[0].ID, f.copies[1].ID}
	for i := 0; i < 7; i++ {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_fixture_copy(id,workspace_id,namespace_id,dataset_version_id,object_key,provider_version,content_sha256,byte_size) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, f.w, ns, c.DatasetVersionID, fmt.Sprintf("sql-only/restore-%d", i), fmt.Sprintf("sql-only-version-%d", i), c.ContentSHA256, c.ByteSize); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	f.authority.caller.Capabilities[CapabilityPreparePlan] = struct{}{}
	plans := NewPlanService(f.o.tx, f.o.repo, &testAuthority{caller: f.authority.caller})
	prepare := func(key string, members []uuid.UUID) domain.Plan {
		p, err := plans.PrepareDeletePlan(ctx, PrepareDeletePlanCommand{WorkspaceID: f.w, CopyIDs: members, IdempotencyKey: key})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	full := prepare("all-nine", ids)
	approved := prepare("single-source", ids[:1])
	partial := prepare("partial-inventory", ids[:8])
	execution := NewExactExecution(f.s)
	cmd := FixtureDisposalBasisCommand{CommandHeader: f.header("basis"), RunID: uuid.New(), InventoryPlanID: full.ID, ApprovedPlanID: approved.ID, InventoryHash: full.ContentHash, ApprovedHash: approved.ContentHash, BasisRef: "human-confirmed-synthetic-single-source"}
	bad := cmd
	bad.IdempotencyKey = "partial-basis"
	bad.InventoryPlanID = partial.ID
	bad.InventoryHash = partial.ContentHash
	if _, err := execution.RegisterFixtureDisposalBasis(ctx, bad); err == nil {
		t.Fatal("partial inventory was accepted")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_fixture_disposal_basis`) != 0 {
		t.Fatal("failed basis committed")
	}
	result, err := execution.RegisterFixtureDisposalBasis(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := execution.RegisterFixtureDisposalBasis(ctx, cmd)
	if err != nil || !replay.Replayed || replay.ID != result.ID {
		t.Fatal("basis replay failed", err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_fixture_disposal_basis`) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 0 {
		t.Fatal("basis duplicated or invented provider work")
	}
	if _, err := pool.Exec(ctx, `UPDATE lifecycle_fixture_disposal_basis SET approved_plan_id=inventory_plan_id`); err == nil {
		t.Fatal("basis history changed")
	}
}
