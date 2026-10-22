package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Attention is a read projection, never a new remediation lifecycle/status.
type AttentionItem struct {
	ID                  uuid.UUID  `json:"id"`
	WorkspaceID         uuid.UUID  `json:"workspaceId"`
	DatasetID           uuid.UUID  `json:"datasetId"`
	DatasetCode         string     `json:"datasetCode"`
	DatasetName         string     `json:"datasetName"`
	VersionID           uuid.UUID  `json:"versionId"`
	VersionNo           int        `json:"versionNo"`
	ResourceID          *uuid.UUID `json:"resourceId"`
	AssessmentID        uuid.UUID  `json:"assessmentId"`
	FindingID           uuid.UUID  `json:"findingId"`
	RuleID              string     `json:"ruleId"`
	Message             string     `json:"message"`
	ActorID             *uuid.UUID `json:"actorId"`
	ExecutionID         *uuid.UUID `json:"executionId"`
	ExecutionStatus     *string    `json:"executionStatus"`
	OutputDatasetID     *uuid.UUID `json:"outputDatasetId"`
	OutputVersionID     *uuid.UUID `json:"outputVersionId"`
	RecheckID           *uuid.UUID `json:"recheckId"`
	RecheckOutcome      *string    `json:"recheckOutcome"`
	RecheckAssessmentID *uuid.UUID `json:"recheckAssessmentId"`
	RecheckGateDecision *string    `json:"recheckGateDecision"`
	RecheckDatasetID    *uuid.UUID `json:"recheckDatasetId"`
	RecheckVersionID    *uuid.UUID `json:"recheckVersionId"`
	PendingRecheck      bool       `json:"pendingRecheck"`
	ActiveExecution     bool       `json:"activeExecution"`
	ManuallyClosed      bool       `json:"manuallyClosed"`
	NeedsFollowUp       bool       `json:"needsFollowUp"`
	CreatedAt           time.Time  `json:"createdAt"`
}

type AttentionPage struct {
	Items []AttentionItem `json:"items"`
	Page  struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"page"`
	Summary struct {
		FollowUpCount int `json:"followUpCount"`
		ClosedCount   int `json:"closedCount"`
	} `json:"summary"`
	ReadAt time.Time `json:"readAt"`
}

func ValidAttentionView(view string) bool {
	return view == "FOLLOW_UP" || view == "CLOSED" || view == "ALL"
}

func (r *Repository) Attention(ctx context.Context, workspace uuid.UUID, view string, limit, offset int) (AttentionPage, error) {
	var result AttentionPage
	if workspace == uuid.Nil || !ValidAttentionView(view) || limit < 1 || limit > 100 || offset < 0 {
		return result, errors.New("invalid quality attention query")
	}
	// One statement gives counts and rows the same PostgreSQL snapshot. Lateral
	// selections and EXISTS avoid multiplying cases by their append-only children.
	var body []byte
	err := r.Pool.QueryRow(ctx, `WITH cases AS MATERIALIZED (
 SELECT c.id, c.workspace_id AS "workspaceId", d.id AS "datasetId",
 d.code AS "datasetCode", d.name AS "datasetName", v.id AS "versionId",
 v.version_no AS "versionNo", d.source_resource_id AS "resourceId",
 c.assessment_id AS "assessmentId", c.finding_id AS "findingId",
 c.detection_rule_id AS "ruleId", f.message, a.actor_id AS "actorId",
 ex.execution_id AS "executionId", ex.status AS "executionStatus",
 ex.output_dataset_id AS "outputDatasetId", ex.output_dataset_version_id AS "outputVersionId",
 rc.id AS "recheckId", rc.outcome AS "recheckOutcome", rc.assessment_id AS "recheckAssessmentId",
 rc.gate_decision AS "recheckGateDecision", rc.output_dataset_id AS "recheckDatasetId",
 rc.output_dataset_version_id AS "recheckVersionId",
 pending.recheck AS "pendingRecheck", pending.execution AS "activeExecution",
 cl.id IS NOT NULL AS "manuallyClosed",
 (cl.id IS NULL OR pending.recheck OR pending.execution) AS "needsFollowUp", c.created_at AS "createdAt"
 FROM quality_remediation_case c
 JOIN dataset_version v ON v.id=c.version_id
 JOIN dataset d ON d.id=v.dataset_id AND d.workspace_id=c.workspace_id
 JOIN quality_result q ON q.id=c.assessment_id AND q.workspace_id=c.workspace_id AND q.dataset_version_id=v.id
 JOIN quality_finding f ON f.id=c.finding_id AND f.result_id=q.id
 LEFT JOIN quality_remediation_close cl ON cl.case_id=c.id
 LEFT JOIN LATERAL (SELECT actor_id FROM quality_remediation_assignment WHERE case_id=c.id ORDER BY ordinal DESC LIMIT 1) a ON true
 LEFT JOIN LATERAL (
  SELECT l.execution_id, e.status, e.output_dataset_id, e.output_dataset_version_id
  FROM quality_remediation_execution l JOIN execution e ON e.id=l.execution_id AND e.workspace_id=c.workspace_id
  WHERE l.case_id=c.id ORDER BY l.ordinal DESC LIMIT 1
 ) ex ON true
 LEFT JOIN LATERAL (
  SELECT x.id, COALESCE(o.outcome,'PENDING') AS outcome, o.assessment_id, qr.gate_decision,
  e.output_dataset_id, e.output_dataset_version_id
  FROM quality_remediation_recheck x
  JOIN quality_remediation_execution l ON l.id=x.execution_link_id AND l.case_id=c.id
  JOIN execution e ON e.id=l.execution_id AND e.workspace_id=c.workspace_id
  LEFT JOIN quality_remediation_recheck_outcome o ON o.id=x.id
  LEFT JOIN quality_result qr ON qr.id=o.assessment_id AND qr.workspace_id=c.workspace_id AND qr.dataset_version_id=e.output_dataset_version_id
  WHERE x.case_id=c.id ORDER BY x.created_at DESC,x.id DESC LIMIT 1
 ) rc ON true
 CROSS JOIN LATERAL (
  SELECT EXISTS(SELECT 1 FROM quality_remediation_recheck x WHERE x.case_id=c.id AND NOT EXISTS(SELECT 1 FROM quality_remediation_recheck_outcome o WHERE o.id=x.id)) AS recheck,
  EXISTS(SELECT 1 FROM quality_remediation_execution l JOIN execution e ON e.id=l.execution_id WHERE l.case_id=c.id AND e.status NOT IN ('SUCCEEDED','FAILED','CANCELLED')) AS execution
 ) pending
 WHERE c.workspace_id=$1
), filtered AS (
 SELECT * FROM cases WHERE $2='ALL' OR ($2='FOLLOW_UP' AND "needsFollowUp") OR ($2='CLOSED' AND "manuallyClosed")
), page AS (
 SELECT * FROM filtered ORDER BY "createdAt" DESC,id DESC LIMIT $3 OFFSET $4
)
SELECT jsonb_build_object(
 'items',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p."createdAt" DESC,p.id DESC) FROM page p),'[]'::jsonb),
 'page',jsonb_build_object('limit',$3::int,'offset',$4::int,'total',(SELECT count(*) FROM filtered)),
 'summary',jsonb_build_object('followUpCount',(SELECT count(*) FROM cases WHERE "needsFollowUp"),'closedCount',(SELECT count(*) FROM cases WHERE "manuallyClosed")),
 'readAt',statement_timestamp())`, workspace, view, limit, offset).Scan(&body)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}
