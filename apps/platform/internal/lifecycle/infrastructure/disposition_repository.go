package infrastructure

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func (r *PostgresRepository) GetApproval(ctx context.Context, tx pgx.Tx, w, id uuid.UUID, lock bool) (domain.DispositionApproval, error) {
	var a domain.DispositionApproval
	query := `SELECT id,workspace_id,plan_id,plan_hash,action,basis_ref,actor_id,principal_ref,valid_from,expires_at,dependency_revision
		FROM lifecycle_disposition_approval WHERE workspace_id=$1 AND id=$2`
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, w, id).Scan(&a.ID, &a.WorkspaceID, &a.PlanID, &a.PlanHash, &a.Action, &a.BasisRef, &a.ActorID, &a.PrincipalRef, &a.ValidFrom, &a.ExpiresAt, &a.DependencyRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrScopeUnavailable
	}
	return a, err
}

func (r *PostgresRepository) ApprovalRevoked(ctx context.Context, tx pgx.Tx, w, id uuid.UUID) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_approval_revocation WHERE workspace_id=$1 AND approval_id=$2)`, w, id).Scan(&found)
	return found, err
}

func (r *PostgresRepository) HasUnresolvedDispatch(ctx context.Context, tx pgx.Tx, w, copyID uuid.UUID) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_disposition_dispatch d WHERE d.workspace_id=$1 AND d.copy_id=$2
		AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id))`, w, copyID).Scan(&found)
	return found, err
}

func (r *PostgresRepository) GetAttempt(ctx context.Context, tx pgx.Tx, w, id uuid.UUID, lock bool) (domain.ProviderAttempt, error) {
	var a domain.ProviderAttempt
	query := `SELECT id,workspace_id,dispatch_id,plan_id,copy_id,action,provider_request_key,started_at,dependency_revision
		FROM lifecycle_provider_attempt WHERE workspace_id=$1 AND id=$2`
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, w, id).Scan(&a.ID, &a.WorkspaceID, &a.DispatchID, &a.PlanID, &a.CopyID, &a.Action, &a.ProviderRequestKey, &a.StartedAt, &a.DependencyRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrScopeUnavailable
	}
	return a, err
}

type DispatchEvaluation struct {
	ID         uuid.UUID
	Eligible   bool
	Blockers   []string
	DispatchID *uuid.UUID
	AttemptID  *uuid.UUID
}

func (r *PostgresRepository) FinalizeDispatchEvaluation(ctx context.Context, tx pgx.Tx, w, id uuid.UUID) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM lifecycle_disposition_gate_evaluation WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, w, id).Scan(&status); err != nil {
		return err
	}
	if status != domain.Draft {
		return domain.ErrInvalidDispositionFact
	}
	_, err := tx.Exec(ctx, `UPDATE lifecycle_disposition_gate_evaluation SET status='FINALIZED',content_hash=lifecycle_gate_inputs_hash(id) WHERE workspace_id=$1 AND id=$2`, w, id)
	return err
}

func (r *PostgresRepository) GetDispatchEvaluation(ctx context.Context, tx pgx.Tx, w, id uuid.UUID) (DispatchEvaluation, error) {
	var e DispatchEvaluation
	err := tx.QueryRow(ctx, `SELECT g.id,g.eligible,g.blockers,d.id,a.id FROM lifecycle_disposition_gate_evaluation g
		LEFT JOIN lifecycle_disposition_dispatch d ON d.evaluation_id=g.id LEFT JOIN lifecycle_provider_attempt a ON a.dispatch_id=d.id
		WHERE g.workspace_id=$1 AND g.id=$2 AND g.status='FINALIZED'`, w, id).Scan(&e.ID, &e.Eligible, &e.Blockers, &e.DispatchID, &e.AttemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrScopeUnavailable
	}
	return e, err
}

func (r *PostgresRepository) RecordHoldDispatchConflicts(ctx context.Context, tx pgx.Tx, w, copyID, holdID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx, `INSERT INTO lifecycle_hold_dispatch_conflict(workspace_id,hold_id,dispatch_id)
		SELECT $1,$3,d.id FROM lifecycle_disposition_dispatch d WHERE d.workspace_id=$1 AND d.copy_id=$2
		AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=d.id) RETURNING dispatch_id`, w, copyID, holdID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PostgresRepository) HoldPreservationUnconfirmed(ctx context.Context, tx pgx.Tx, w, holdID uuid.UUID) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_hold_dispatch_conflict c WHERE c.workspace_id=$1 AND c.hold_id=$2
		AND NOT EXISTS(SELECT 1 FROM lifecycle_dispatch_release r WHERE r.dispatch_id=c.dispatch_id))`, w, holdID).Scan(&found)
	return found, err
}
