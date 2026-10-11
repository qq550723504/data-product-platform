package infrastructure

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/remediation/domain"
)

type Repository struct{ Pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{Pool: pool} }

func (r *Repository) CaseTx(ctx context.Context, tx pgx.Tx, workspace, id uuid.UUID) (domain.Case, error) {
	var c domain.Case
	err := tx.QueryRow(ctx, `SELECT id,workspace_id,version_id,assessment_id,finding_id,plan_ref,plan_version,plan_hash,
 detection_rule_id,workflow_hash,input_name,quarantine_reason,recheck_rule_ref,recheck_rule_hash,created_by
 FROM quality_remediation_case WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, id, workspace).Scan(
		&c.ID, &c.WorkspaceID, &c.VersionID, &c.AssessmentID, &c.FindingID, &c.PlanRef, &c.PlanVersion, &c.PlanHash,
		&c.DetectionRuleID, &c.WorkflowHash, &c.InputName, &c.QuarantineReason, &c.RecheckRuleRef, &c.RecheckRuleHash, &c.CreatedBy)
	return c, err
}

// Detail is a read-only projection of typed, immutable rows and current Execution.
// It never persists a guessed case status or changes the underlying gate facts.
func (r *Repository) Detail(ctx context.Context, workspace, id uuid.UUID) (map[string]any, error) {
	var body []byte
	err := r.Pool.QueryRow(ctx, `SELECT jsonb_build_object('case',to_jsonb(c),'finding',to_jsonb(f),
 'assignments',COALESCE((SELECT jsonb_agg(a ORDER BY a.ordinal) FROM quality_remediation_assignment a WHERE a.case_id=c.id),'[]'::jsonb),
 'attempts',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',l.id,'ordinal',l.ordinal,'reason',l.reason,'executionId',e.id,
 'status',e.status,'errorCode',e.error_code,'errorMessage',e.error_message,'outputDatasetId',e.output_dataset_id,'outputVersionId',e.output_dataset_version_id,'metrics',e.metrics)
 ORDER BY l.ordinal) FROM quality_remediation_execution l JOIN execution e ON e.id=l.execution_id WHERE l.case_id=c.id),'[]'::jsonb),
 'rechecks',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',rc.id,'qualityAttemptId',rc.quality_attempt_id,'requestKey',rc.request_key,
 'executionLinkId',rc.execution_link_id,'outcome',COALESCE(o.outcome,'PENDING'),'errorCode',o.error_code,
 'assessmentId',o.assessment_id,'gateDecision',q.gate_decision) ORDER BY rc.created_at,rc.id)
 FROM quality_remediation_recheck rc LEFT JOIN quality_remediation_recheck_outcome o ON o.id=rc.id LEFT JOIN quality_result q ON q.id=o.assessment_id
 WHERE rc.case_id=c.id),'[]'::jsonb),'closure',(SELECT to_jsonb(cl) FROM quality_remediation_close cl WHERE cl.case_id=c.id))
 FROM quality_remediation_case c JOIN quality_finding f ON f.id=c.finding_id WHERE c.id=$1 AND c.workspace_id=$2`, id, workspace).Scan(&body)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(body, &result)
	return result, err
}
func (r *Repository) List(ctx context.Context, workspace, version uuid.UUID) ([]map[string]any, error) {
	rows, err := r.Pool.Query(ctx, `SELECT to_jsonb(c) FROM quality_remediation_case c WHERE workspace_id=$1
 AND ($2::uuid='00000000-0000-0000-0000-000000000000' OR version_id=$2) ORDER BY created_at DESC,id DESC LIMIT 100`, workspace, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var body []byte
		if err = rows.Scan(&body); err != nil {
			return nil, err
		}
		var c map[string]any
		if err = json.Unmarshal(body, &c); err != nil {
			return nil, err
		}
		items = append(items, c)
	}
	return items, rows.Err()
}
