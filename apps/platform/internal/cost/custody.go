package cost

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Custody quantity counts physical transport attempts, including uncertain
// delivery to the provider. It does not assert a known monetary charge.
const OperatorCustodyInvocation = "OPERATOR_CUSTODY_INVOCATION"

func AppendOperatorCustodyActivity(ctx context.Context, tx pgx.Tx, workspace, call uuid.UUID, at time.Time) error {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `INSERT INTO cost_event(id,workspace_id,activity_id,cost_type,quantity,unit,pricing_mode,metadata,occurred_at)
	 VALUES($1,$2,$3,$4,1,'invocation','ACTUAL','{}'::jsonb,$5)
	 ON CONFLICT(workspace_id,activity_id,cost_type) WHERE activity_id IS NOT NULL DO NOTHING RETURNING id`, uuid.New(), workspace, call, OperatorCustodyInvocation, at).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT id FROM cost_event WHERE workspace_id=$1 AND activity_id=$2 AND cost_type=$3 AND quantity=1 AND unit='invocation' AND pricing_mode='ACTUAL' AND amount IS NULL AND currency IS NULL AND occurred_at=$4`, workspace, call, OperatorCustodyInvocation, at).Scan(&id)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,operator_custody_call_id) VALUES($1,$2,$3) ON CONFLICT(cost_event_id) DO NOTHING`, uuid.New(), id, call)
	if err != nil {
		return err
	}
	var ok bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cost_allocation WHERE cost_event_id=$1 AND operator_custody_call_id=$2)`, id, call).Scan(&ok)
	if err == nil && !ok {
		return errors.New("custody cost allocated to another subject")
	}
	return err
}
