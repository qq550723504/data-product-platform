package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

var ErrScopeUnavailable = errors.New("lifecycle fixture scope is unavailable")

type PostgresRepository struct{}

func (r *PostgresRepository) LoadFixtureCopies(ctx context.Context, tx pgx.Tx, workspace uuid.UUID, ids []uuid.UUID) ([]domain.Copy, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id,c.workspace_id,c.dataset_version_id,n.provider_instance_id,n.incarnation_id,
		       n.namespace,c.object_key,c.provider_version,c.content_sha256,c.byte_size
		FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id
		WHERE c.workspace_id=$1 AND c.id=ANY($2) ORDER BY c.id
	`, workspace, ids)
	if err != nil {
		return nil, fmt.Errorf("load lifecycle fixture scope: %w", err)
	}
	defer rows.Close()
	var copies []domain.Copy
	for rows.Next() {
		var c domain.Copy
		if err := rows.Scan(&c.ID, &c.WorkspaceID, &c.DatasetVersionID, &c.ProviderInstanceID, &c.IncarnationID,
			&c.Namespace, &c.Key, &c.ProviderVersion, &c.ContentSHA256, &c.ByteSize); err != nil {
			return nil, err
		}
		copies = append(copies, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(copies) != len(ids) {
		return nil, ErrScopeUnavailable
	}
	return copies, nil
}

func (r *PostgresRepository) FindReplay(ctx context.Context, tx pgx.Tx, workspace uuid.UUID, key string) (uuid.UUID, string, bool, error) {
	var id uuid.UUID
	var fingerprint string
	err := tx.QueryRow(ctx, `SELECT object_id,request_fingerprint FROM command_idempotency
		WHERE workspace_id=$1 AND command_type='LIFECYCLE.PREPARE_DELETE_PLAN' AND idempotency_key=$2`, workspace, key).Scan(&id, &fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", false, nil
	}
	return id, fingerprint, err == nil, err
}

func (r *PostgresRepository) RecordIdempotency(ctx context.Context, tx pgx.Tx, p domain.Plan, key, fingerprint string) error {
	_, err := tx.Exec(ctx, `INSERT INTO command_idempotency(workspace_id,command_type,idempotency_key,object_id,result_ref,request_fingerprint)
		VALUES ($1,'LIFECYCLE.PREPARE_DELETE_PLAN',$2,$3,$3,$4)`, p.WorkspaceID, key, p.ID, fingerprint)
	return err
}

func (r *PostgresRepository) InsertDraft(ctx context.Context, tx pgx.Tx, p domain.Plan) error {
	_, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_plan
		(id,workspace_id,action,prepared_by,principal_ref,dependency_revision,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, p.ID, p.WorkspaceID, p.Action, p.PreparedBy, p.PrincipalRef, p.DependencyRevision, p.CreatedAt)
	return err
}

// AddTarget always acquires the parent lock before writing a child. The DB
// trigger repeats that guard for independent writers, including late writers.
func (r *PostgresRepository) AddTarget(ctx context.Context, tx pgx.Tx, workspace, planID, copyID uuid.UUID) error {
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM lifecycle_disposition_plan WHERE workspace_id=$1 AND id=$2 FOR UPDATE`, workspace, planID).Scan(&status); err != nil {
		return err
	}
	if status != domain.Draft {
		return domain.ErrInvalidPlan
	}
	_, err := tx.Exec(ctx, `INSERT INTO lifecycle_disposition_plan_target(plan_id,workspace_id,copy_id) VALUES($1,$2,$3)`, planID, workspace, copyID)
	return err
}

func (r *PostgresRepository) GetPlan(ctx context.Context, tx pgx.Tx, workspace, id uuid.UUID, lock bool) (domain.Plan, error) {
	var p domain.Plan
	query := `SELECT id,workspace_id,action,status,prepared_by,principal_ref,dependency_revision,created_at,finalized_at,COALESCE(content_hash,'')
		FROM lifecycle_disposition_plan WHERE workspace_id=$1 AND id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	if err := tx.QueryRow(ctx, query, workspace, id).Scan(&p.ID, &p.WorkspaceID, &p.Action, &p.Status, &p.PreparedBy, &p.PrincipalRef,
		&p.DependencyRevision, &p.CreatedAt, &p.FinalizedAt, &p.ContentHash); err != nil {
		return p, err
	}
	rows, err := tx.Query(ctx, `SELECT copy_id FROM lifecycle_disposition_plan_target WHERE workspace_id=$1 AND plan_id=$2 ORDER BY copy_id`, workspace, id)
	if err != nil {
		return p, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var copyID uuid.UUID
		if err := rows.Scan(&copyID); err != nil {
			rows.Close()
			return p, err
		}
		ids = append(ids, copyID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	p.Targets, err = r.LoadFixtureCopies(ctx, tx, workspace, ids)
	return p, err
}

func (r *PostgresRepository) Finalize(ctx context.Context, tx pgx.Tx, workspace, id uuid.UUID, at time.Time) (domain.Plan, error) {
	p, err := r.GetPlan(ctx, tx, workspace, id, true)
	if err != nil {
		return p, err
	}
	if p.Status != domain.Draft {
		return p, domain.ErrInvalidPlan
	}
	hash, err := domain.ScopeHash(workspace, p.Action, p.Targets)
	if err != nil {
		return p, err
	}
	if _, err = tx.Exec(ctx, `UPDATE lifecycle_disposition_plan SET status='FINALIZED',content_hash=$3,finalized_at=$4 WHERE workspace_id=$1 AND id=$2`, workspace, id, hash, at); err != nil {
		return p, err
	}
	p.Status, p.ContentHash, p.FinalizedAt = domain.Finalized, hash, &at
	return p, nil
}
