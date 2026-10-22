package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
)

// SchemaAttentionItem projects one historical observation, not a source health
// status or a new task lifecycle. Resource names are current display labels;
// responsibility, rule and source identity belong to the frozen request.
type SchemaAttentionItem struct {
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
	CompletedAt        *time.Time `json:"completedAt"`
	RecordedAt         time.Time  `json:"recordedAt"`
	HandlingCount      int        `json:"handlingCount"`
}

type SchemaAttentionPage struct {
	Items []SchemaAttentionItem `json:"items"`
	Page  struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"page"`
	Summary struct {
		UnhandledCount int `json:"unhandledCount"`
		HandledCount   int `json:"handledCount"`
	} `json:"summary"`
	ReadAt time.Time `json:"readAt"`
}

type SchemaHandling struct {
	ID            uuid.UUID `json:"id"`
	ObservationID uuid.UUID `json:"observationId"`
	ActorID       uuid.UUID `json:"actorId"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"createdAt"`
}

type SchemaObservationDetail struct {
	SchemaAttentionItem
	BindingID     uuid.UUID        `json:"bindingId"`
	ExternalID    string           `json:"externalId"`
	ConnectorHash string           `json:"connectorHash"`
	StartedAt     *time.Time       `json:"startedAt"`
	Baseline      []d.SchemaColumn `json:"baseline"`
	Columns       []d.SchemaColumn `json:"columns"`
	Handling      []SchemaHandling `json:"handling"`
}

func ValidSchemaAttentionView(view string) bool { return view == "UNHANDLED" || view == "ALL" }

const schemaObservationFields = `r.workspace_id AS "workspaceId", r.resource_id AS "resourceId",
 resource.code AS "resourceCode", resource.name AS "resourceName",
 r.id AS "requestId", o.id AS "observationId", o.attempt_id AS "attemptId",
 r.actor_id AS "responsibleActorId", r.external_fqn AS "externalFqn",
 r.rule_ref AS "ruleRef", r.rule_version AS "ruleVersion", r.rule_hash AS "ruleHash",
 r.plan_hash AS "planHash", o.status, o.reason, o.schema_hash AS "schemaHash",
 r.requested_at AS "requestedAt", o.completed_at AS "completedAt", o.recorded_at AS "recordedAt",
 (SELECT count(*) FROM source_schema_handling h WHERE h.observation_id=o.id) AS "handlingCount"`

const schemaObservationFrom = ` FROM source_schema_observation o
 JOIN source_schema_request r ON r.id=o.request_id
 JOIN data_resource resource ON resource.id=r.resource_id AND resource.workspace_id=r.workspace_id `

func (r *PostgresRepository) SchemaAttention(ctx context.Context, workspace uuid.UUID, view string, limit, offset int) (SchemaAttentionPage, error) {
	var result SchemaAttentionPage
	if workspace == uuid.Nil || !ValidSchemaAttentionView(view) || limit < 1 || limit > 100 || offset < 0 || offset > 2147483647 {
		return result, errors.New("invalid schema attention query")
	}
	// Counts and rows share one statement snapshot. Count observations, not joined
	// handling rows. A later MATCH request never changes an earlier DRIFT fact.
	var body []byte
	err := r.pool.QueryRow(ctx, `WITH observations AS MATERIALIZED (
 SELECT `+schemaObservationFields+schemaObservationFrom+`
 WHERE r.workspace_id=$1 AND o.status='DRIFT'
), filtered AS (
 SELECT * FROM observations WHERE $2='ALL' OR "handlingCount"=0
), page AS (
 SELECT * FROM filtered ORDER BY "recordedAt" DESC,"observationId" DESC LIMIT $3 OFFSET $4
)
SELECT jsonb_build_object(
 'items',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p."recordedAt" DESC,p."observationId" DESC) FROM page p),'[]'::jsonb),
 'page',jsonb_build_object('limit',$3::int,'offset',$4::int,'total',(SELECT count(*) FROM filtered)),
 'summary',jsonb_build_object('unhandledCount',(SELECT count(*) FROM observations WHERE "handlingCount"=0),'handledCount',(SELECT count(*) FROM observations WHERE "handlingCount">0)),
 'readAt',statement_timestamp())`, workspace, view, limit, offset).Scan(&body)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}

func (r *PostgresRepository) SchemaObservationDetail(ctx context.Context, workspace, resource, observation uuid.UUID) (SchemaObservationDetail, error) {
	var result SchemaObservationDetail
	if workspace == uuid.Nil || resource == uuid.Nil || observation == uuid.Nil {
		return result, errors.New("invalid schema observation identity")
	}
	// Deliberately independent of mutable bindings, scanner composition and the
	// capped history list. All identities and content come from this exact fact.
	var body []byte
	err := r.pool.QueryRow(ctx, `SELECT to_jsonb(detail) FROM (SELECT `+schemaObservationFields+`,
 r.binding_id AS "bindingId", r.external_id AS "externalId", r.plan->>'connectorHash' AS "connectorHash",
 o.started_at AS "startedAt", r.plan->'columns' AS baseline, o.columns,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',h.id,'observationId',h.observation_id,
 'actorId',h.actor_id,'reason',h.reason,'createdAt',h.created_at) ORDER BY h.created_at,h.id)
 FROM source_schema_handling h WHERE h.observation_id=o.id),'[]'::jsonb) AS handling
 `+schemaObservationFrom+`WHERE r.workspace_id=$1 AND r.resource_id=$2 AND o.id=$3) detail`, workspace, resource, observation).Scan(&body)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}
