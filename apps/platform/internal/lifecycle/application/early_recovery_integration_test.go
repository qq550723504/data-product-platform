package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type recoveryQueueProbe struct{ work []uuid.UUID }

func (q *recoveryQueueProbe) EnqueueRecovery(_ context.Context, _, work uuid.UUID) error {
	q.work = append(q.work, work)
	return nil
}

// SQL-only observations simulate the metadata view before and after an original
// side effect. These fixtures cannot send DELETE/COPY or prove native acceptance.
func TestEarlyDeleteRecoveryRemainsPendingThenResolvesWithoutResend(t *testing.T) {
	pool := planDatabase(t)
	f, service, inspector := recoveryFixture(t, pool)
	ctx := context.Background()
	dispatch := requireDispatch(t, f, "early-delete", 0)
	work := recoveryWork(t, pool, dispatch.AttemptID)
	queue := &recoveryQueueProbe{}
	service.enqueuer = queue
	if err := service.RunRecovery(ctx, f.w, work); !errors.Is(err, ErrStorageUnknown) {
		t.Fatal("early presence must return a queue-retryable uncertainty", err)
	}
	if err := service.RunOnce(ctx); err != nil || len(queue.work) != 1 || queue.work[0] != work {
		t.Fatal("early presence disappeared from durable pending scan", queue.work, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_reconciliation_result(id,workspace_id,work_id,run_id,outcome,terminal,inventory_call_id,stat_call_id,observed_at,dependency_revision)
	 SELECT $1,workspace_id,work_id,run_id,outcome,true,inventory_call_id,stat_call_id,observed_at,dependency_revision FROM lifecycle_reconciliation_result WHERE work_id=$2`, uuid.New(), work); err == nil {
		t.Fatal("database accepted inconclusive presence as terminal")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatal("expected the terminal/outcome check", err)
		}
	}
	inspector.present = false
	if err := service.RunRecovery(ctx, f.w, work); err != nil {
		t.Fatal("later exact absence could not resolve the same work", err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome='UNKNOWN_PRESENT' AND NOT terminal`, work) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome='ABSENT_PARTIAL' AND terminal`, work) != 1 {
		t.Fatal("resolution overwrote the first observation or failed to append")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_provider_attempt`) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_provider_send_claim`) != 0 || count(t, pool, `SELECT count(*) FROM lifecycle_dispatch_release`) != 0 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != 3 {
		t.Fatal("metadata retry changed original send/reservation or lost lookup costs")
	}
	calls := inspector.calls.Load()
	if err := service.RunRecovery(ctx, f.w, work); err != nil || inspector.calls.Load() != calls {
		t.Fatal("terminal duplicate performed I/O", err)
	}
	if err := service.RunOnce(ctx); err != nil || len(queue.work) != 1 {
		t.Fatal("terminal work was enqueued again", queue.work, err)
	}
}

func TestEarlyCopyRecoveryRemainsPendingThenFindsOriginalTarget(t *testing.T) {
	pool, w, copies, service, _, authority := newCopyFixture(t)
	authority.caller.Capabilities[CapabilityReconcileCopy] = struct{}{}
	op, original, work := unknownCopy(t, pool, service, w, copies[0].ID, "archive")
	_, recovery, inspector := copyRecoveryTestService(service, pool, w)
	queue := &recoveryQueueProbe{}
	recovery.enqueuer = queue
	ctx := context.Background()
	inspector.missing = true
	if err := recovery.RunRecovery(ctx, w, work); !errors.Is(err, ErrCopyTargetUnknown) {
		t.Fatal("early missing target must return a queue-retryable uncertainty", err)
	}
	if err := recovery.RunOnce(ctx); err != nil || len(queue.work) != 1 || queue.work[0] != work {
		t.Fatal("early missing target disappeared from durable pending scan", queue.work, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifecycle_reconciliation_result(id,workspace_id,work_id,run_id,outcome,terminal,inventory_call_id,observed_at,dependency_revision)
	 SELECT $1,workspace_id,work_id,run_id,outcome,true,inventory_call_id,observed_at,dependency_revision FROM lifecycle_reconciliation_result WHERE work_id=$2`, uuid.New(), work); err == nil {
		t.Fatal("database accepted an inconclusive missing target as terminal")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Fatal("expected the terminal/outcome check", err)
		}
	}
	inspector.missing = false
	if err := recovery.RunRecovery(ctx, w, work); err != nil {
		t.Fatal("later original target could not resolve the same work", err)
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome='UNKNOWN_COPY_TARGET' AND NOT terminal`, work) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_reconciliation_result WHERE work_id=$1 AND outcome='COPY_ARCHIVED' AND terminal`, work) != 1 {
		t.Fatal("copy resolution overwrote uncertainty or failed to append")
	}
	if count(t, pool, `SELECT count(*) FROM lifecycle_storage_call WHERE copy_operation_id=$1 AND operation='ARCHIVE_COPY'`, op.ID) != 1 || count(t, pool, `SELECT count(*) FROM lifecycle_storage_observation WHERE call_id=$1 AND failure='TRANSPORT'`, original) != 1 || count(t, pool, `SELECT count(*) FROM cost_allocation WHERE lifecycle_storage_call_id IS NOT NULL`) != 9 {
		t.Fatal("metadata recovery resent COPY, overwrote original uncertainty or lost costs")
	}
	calls := inspector.calls
	if err := recovery.RunRecovery(ctx, w, work); err != nil || inspector.calls != calls {
		t.Fatal("terminal duplicate performed I/O", err)
	}
	if err := recovery.RunOnce(ctx); err != nil || len(queue.work) != 1 {
		t.Fatal("resolved COPY was enqueued again", queue.work, err)
	}
}
