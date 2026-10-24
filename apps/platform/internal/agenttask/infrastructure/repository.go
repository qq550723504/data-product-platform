package infrastructure

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/agenttask/domain"
)

type Repository struct{ Pool *pgxpool.Pool }

const taskColumns = `id,workspace_id,kind,created_by,subject,parent_task_id,current_plan_revision_id,revision,control_state,created_at`

type scanner interface{ Scan(...any) error }

func scanTask(row scanner) (domain.Task, error) {
	var t domain.Task
	err := row.Scan(&t.ID, &t.WorkspaceID, &t.Kind, &t.CreatedBy, &t.Subject, &t.ParentTaskID, &t.CurrentPlanRevisionID, &t.Revision, &t.ControlState, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}
	return t, err
}
func (r *Repository) TaskTx(ctx context.Context, tx pgx.Tx, ws, id uuid.UUID, lock bool) (domain.Task, error) {
	sql := `SELECT ` + taskColumns + ` FROM agent_task WHERE workspace_id=$1 AND id=$2`
	if lock {
		sql += ` FOR UPDATE`
	}
	return scanTask(tx.QueryRow(ctx, sql, ws, id))
}
func (r *Repository) PlanTx(ctx context.Context, tx pgx.Tx, task, id uuid.UUID) (domain.Plan, error) {
	var p domain.Plan
	var canonical string
	err := tx.QueryRow(ctx, `SELECT canonical_content,id,content_hash,created_at FROM agent_plan_revision WHERE task_id=$1 AND id=$2 AND frozen`, task, id).Scan(&canonical, &p.ID, &p.ContentHash, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal([]byte(canonical), &p); err != nil {
		return p, err
	}
	p.Canonical = canonical
	return p, nil
}
func (r *Repository) InsertPlan(ctx context.Context, tx pgx.Tx, p domain.Plan) error {
	_, err := tx.Exec(ctx, `INSERT INTO agent_plan_revision(id,task_id,workspace_id,ordinal,phase,source_version_id,source_assessment_id,finding_id,case_id,
 template_ref,template_version,template_hash,detection_rule_ref,detection_rule_hash,workflow_version_id,workflow_hash,output_dataset_id,target_period,
 recheck_rule_ref,recheck_rule_hash,goal,purpose_note,acceptance_text,content_hash,canonical_content)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)`,
		p.ID, p.TaskID, p.WorkspaceID, p.Ordinal, p.Phase, p.SourceVersionID, p.AssessmentID, p.FindingID, p.CaseID, p.TemplateRef, p.TemplateVersion, p.TemplateHash,
		p.DetectionRuleRef, p.DetectionRuleHash, p.WorkflowVersionID, p.WorkflowHash, p.OutputDatasetID, p.TargetPeriod, p.RecheckRuleRef, p.RecheckRuleHash,
		p.Goal, p.PurposeNote, p.AcceptanceText, p.ContentHash, p.Canonical)
	if err != nil {
		return err
	}
	for _, i := range p.Inputs {
		if _, err = tx.Exec(ctx, `INSERT INTO agent_plan_input(plan_revision_id,input_name,dataset_version_id,required,receipt_id,resolution_job_id) VALUES($1,$2,$3,$4,$5,$6)`, p.ID, i.Name, i.VersionID, i.Required, i.ReceiptID, i.ResolutionJobID); err != nil {
			return err
		}
	}
	for _, a := range p.Actions {
		if _, err = tx.Exec(ctx, `INSERT INTO agent_plan_action(plan_revision_id,ordinal,command_kind,approval_phase) VALUES($1,$2,$3,$4)`, p.ID, a.Ordinal, a.Command, a.Phase); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE agent_plan_revision SET frozen=true WHERE id=$1`, p.ID)
	return err
}
func (r *Repository) PendingTx(ctx context.Context, tx pgx.Tx, task uuid.UUID) (bool, error) {
	var pending bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_action_request a WHERE a.task_id=$1 AND NOT EXISTS(SELECT 1 FROM agent_action_disposition d WHERE d.action_id=a.id))`, task).Scan(&pending)
	return pending, err
}
func (r *Repository) ReceiptTx(ctx context.Context, tx pgx.Tx, ws, target uuid.UUID, kind, key, fingerprint string) (domain.Receipt, bool, error) {
	var v domain.Receipt
	var hash string
	err := tx.QueryRow(ctx, `SELECT id,task_id,plan_revision_id,approval_id,action_id,task_revision,fingerprint FROM agent_command_receipt WHERE workspace_id=$1 AND target_id=$2 AND command_kind=$3 AND request_key=$4`, ws, target, kind, key).Scan(&v.ID, &v.TaskID, &v.PlanID, &v.ApprovalID, &v.ActionID, &v.TaskRevision, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, false, nil
	}
	if err != nil {
		return v, false, err
	}
	if hash != fingerprint {
		return v, true, domain.ErrConflict
	}
	return v, true, nil
}
func (r *Repository) List(ctx context.Context, ws uuid.UUID, limit, offset int) ([]domain.Task, int, error) {
	var total int
	if err := r.Pool.QueryRow(ctx, `SELECT count(*) FROM agent_task WHERE workspace_id=$1`, ws).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.Pool.Query(ctx, `SELECT `+taskColumns+` FROM agent_task WHERE workspace_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`, ws, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []domain.Task{}
	for rows.Next() {
		t, e := scanTask(rows)
		if e != nil {
			return nil, 0, e
		}
		items = append(items, t)
	}
	return items, total, rows.Err()
}
func (r *Repository) DetailTx(ctx context.Context, tx pgx.Tx, ws, id, planID uuid.UUID, offset int) (domain.Detail, error) {
	var d domain.Detail
	var err error
	d.Task, err = r.TaskTx(ctx, tx, ws, id, false)
	if err != nil {
		return d, err
	}
	if planID == uuid.Nil {
		planID = d.Task.CurrentPlanRevisionID
	}
	d.Plan, err = r.PlanTx(ctx, tx, id, planID)
	if err != nil {
		return d, err
	}
	d.Plans = []domain.PlanSummary{}
	d.Approvals = []domain.Approval{}
	d.Actions = []domain.Intent{}
	d.PlanPage = domain.Page{Limit: 25, Offset: offset}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM agent_plan_revision WHERE task_id=$1`, id).Scan(&d.PlanPage.Total); err != nil {
		return d, err
	}
	rows, err := tx.Query(ctx, `SELECT id,ordinal,phase,content_hash FROM agent_plan_revision WHERE task_id=$1 ORDER BY ordinal DESC LIMIT 25 OFFSET $2`, id, offset)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var p domain.PlanSummary
		if err = rows.Scan(&p.ID, &p.Ordinal, &p.Phase, &p.Hash); err != nil {
			rows.Close()
			return d, err
		}
		d.Plans = append(d.Plans, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	rows, err = tx.Query(ctx, `SELECT id,task_id,plan_revision_id,content_hash,phase,actor_id,subject,scope_hash,reason,created_at FROM agent_approval WHERE task_id=$1 AND plan_revision_id=$2 ORDER BY created_at,id`, id, planID)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var a domain.Approval
		if err = rows.Scan(&a.ID, &a.TaskID, &a.PlanID, &a.ContentHash, &a.Phase, &a.ActorID, &a.Subject, &a.ScopeHash, &a.Reason, &a.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Approvals = append(d.Approvals, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	rows, err = tx.Query(ctx, `SELECT a.id,a.task_id,a.plan_revision_id,a.approval_id,a.ordinal,a.command_kind,a.business_request_key,
 CASE WHEN d.id IS NOT NULL THEN 'CANCELLED' ELSE a.dispatch_state END,a.reason,a.created_at FROM agent_action_request a LEFT JOIN agent_action_disposition d ON d.action_id=a.id WHERE a.task_id=$1 ORDER BY a.created_at,a.id`, id)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var a domain.Intent
		if err = rows.Scan(&a.ID, &a.TaskID, &a.PlanID, &a.ApprovalID, &a.Ordinal, &a.Command, &a.BusinessKey, &a.State, &a.Reason, &a.CreatedAt); err != nil {
			rows.Close()
			return d, err
		}
		d.Actions = append(d.Actions, a)
	}
	err = rows.Err()
	rows.Close()
	return d, err
}
