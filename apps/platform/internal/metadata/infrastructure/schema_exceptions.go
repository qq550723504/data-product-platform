package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SchemaExceptionItem is an immutable historical observation, never a current
// unresolved task. DENIED can precede an attempt; scan times can be unknown.
type SchemaExceptionItem struct {
	WorkspaceID        uuid.UUID  `json:"workspaceId"`
	ResourceID         uuid.UUID  `json:"resourceId"`
	ResourceCode       string     `json:"resourceCode"`
	ResourceName       string     `json:"resourceName"`
	RequestID          uuid.UUID  `json:"requestId"`
	ObservationID      uuid.UUID  `json:"observationId"`
	AttemptID          *uuid.UUID `json:"attemptId"`
	ResponsibleActorID uuid.UUID  `json:"responsibleActorId"`
	ExternalFQN        string     `json:"externalFqn"`
	RuleRef            string     `json:"ruleRef"`
	RuleVersion        string     `json:"ruleVersion"`
	RuleHash           string     `json:"ruleHash"`
	PlanHash           string     `json:"planHash"`
	Status             string     `json:"status"`
	Reason             string     `json:"reason"`
	SchemaHash         string     `json:"schemaHash"`
	RequestedAt        time.Time  `json:"requestedAt"`
	StartedAt          *time.Time `json:"startedAt"`
	CompletedAt        *time.Time `json:"completedAt"`
	RecordedAt         time.Time  `json:"recordedAt"`
}

type SchemaExceptionPage struct {
	Items []SchemaExceptionItem `json:"items"`
	Page  struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"page"`
	// Counts always cover all three historical exception kinds in this workspace,
	// while Page.Total follows the selected status filter.
	Summary struct {
		UnavailableCount int `json:"unavailableCount"`
		DeniedCount      int `json:"deniedCount"`
		InterruptedCount int `json:"interruptedCount"`
	} `json:"summary"`
	ReadAt time.Time `json:"readAt"`
}

func ValidSchemaExceptionStatus(status string) bool {
	return status == "ALL" || status == "UNAVAILABLE" || status == "DENIED" || status == "INTERRUPTED"
}

func (r *PostgresRepository) SchemaExceptions(ctx context.Context, workspace uuid.UUID, status string, limit, offset int) (SchemaExceptionPage, error) {
	var result SchemaExceptionPage
	if workspace == uuid.Nil || !ValidSchemaExceptionStatus(status) || limit < 1 || limit > 100 || offset < 0 || offset > 2147483647 {
		return result, errors.New("invalid schema exception history query")
	}
	// One statement snapshot for counts and page. No latest/terminal/handling
	// filter: retry success must never hide an earlier exception observation.
	var body []byte
	err := r.pool.QueryRow(ctx, `WITH observations AS MATERIALIZED (
 SELECT `+schemaObservationBaseFields+`, o.started_at AS "startedAt"`+schemaObservationFrom+`
 WHERE r.workspace_id=$1 AND o.status IN ('UNAVAILABLE','DENIED','INTERRUPTED')
), filtered AS (
 SELECT * FROM observations WHERE $2='ALL' OR status=$2
), page AS (
 SELECT * FROM filtered ORDER BY "recordedAt" DESC,"observationId" DESC LIMIT $3 OFFSET $4
)
SELECT jsonb_build_object(
 'items',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p."recordedAt" DESC,p."observationId" DESC) FROM page p),'[]'::jsonb),
 'page',jsonb_build_object('limit',$3::int,'offset',$4::int,'total',(SELECT count(*) FROM filtered)),
 'summary',jsonb_build_object('unavailableCount',(SELECT count(*) FROM observations WHERE status='UNAVAILABLE'),
 'deniedCount',(SELECT count(*) FROM observations WHERE status='DENIED'),
 'interruptedCount',(SELECT count(*) FROM observations WHERE status='INTERRUPTED')),
 'readAt',statement_timestamp())`, workspace, status, limit, offset).Scan(&body)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}
