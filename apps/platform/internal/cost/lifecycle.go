package cost

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const LifecycleProviderInvocation = "LIFECYCLE_PROVIDER_INVOCATION"

// Only a confirmed physical invocation is allocated here, including a failed
// or timed-out invocation. Start-only uncertainty has no definite quantity.
// The existing CostEvent / CostAllocation stores remain the system of record.
func AppendLifecycleProviderActivity(ctx context.Context, tx pgx.Tx, workspace, attemptID uuid.UUID, invokedAt time.Time) error {
	return appendLifecycleInvocation(ctx, tx, workspace, attemptID, invokedAt, "lifecycle_provider_attempt_id")
}

// AppendLifecycleStorageActivity reuses the same ledger and per-physical-call
// cost type, with an inspection-call subject rather than a delete reservation.
func AppendLifecycleStorageActivity(ctx context.Context, tx pgx.Tx, workspace, callID uuid.UUID, invokedAt time.Time) error {
	return appendLifecycleInvocation(ctx, tx, workspace, callID, invokedAt, "lifecycle_storage_call_id")
}

func appendLifecycleInvocation(ctx context.Context, tx pgx.Tx, workspace, attemptID uuid.UUID, invokedAt time.Time, subjectColumn string) error {
	if workspace == uuid.Nil || attemptID == uuid.Nil || invokedAt.IsZero() {
		return errors.New("lifecycle cost requires exact workspace/attempt/invocation time")
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO cost_event(id,workspace_id,execution_id,activity_id,cost_type,quantity,unit,amount,currency,pricing_mode,metadata,occurred_at)
		VALUES($1,$2,NULL,$3,$4,1,'invocation',NULL,NULL,'ACTUAL','{}'::jsonb,$5)
		ON CONFLICT(workspace_id,activity_id,cost_type) WHERE activity_id IS NOT NULL DO NOTHING RETURNING id`,
		uuid.New(), workspace, attemptID, LifecycleProviderInvocation, invokedAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM cost_event WHERE workspace_id=$1 AND activity_id=$2 AND cost_type=$3
			AND quantity=1 AND unit='invocation' AND amount IS NULL AND currency IS NULL AND pricing_mode='ACTUAL' AND occurred_at=$4`,
			workspace, attemptID, LifecycleProviderInvocation, invokedAt).Scan(&id)
	}
	if err != nil {
		return fmt.Errorf("append lifecycle provider cost: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,`+subjectColumn+`) VALUES($1,$2,$3)
		ON CONFLICT(cost_event_id) DO NOTHING`, uuid.New(), id, attemptID); err != nil {
		return err
	}
	var allocated *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT `+subjectColumn+` FROM cost_allocation WHERE cost_event_id=$1`, id).Scan(&allocated); err != nil {
		return err
	}
	if allocated == nil || *allocated != attemptID {
		return errors.New("lifecycle provider cost allocated to another subject")
	}
	return nil
}
