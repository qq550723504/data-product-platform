package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
)

// These are read classifications of recorded facts, never worker states.
type SchemaRequestItem struct {
	WorkspaceID        uuid.UUID `json:"workspaceId"`
	ResourceID         uuid.UUID `json:"resourceId"`
	ResourceCode       string    `json:"resourceCode"`
	ResourceName       string    `json:"resourceName"`
	RequestID          uuid.UUID `json:"requestId"`
	ResponsibleActorID uuid.UUID `json:"responsibleActorId"`
	ExternalFQN        string    `json:"externalFqn"`
	RuleRef            string    `json:"ruleRef"`
	RuleVersion        string    `json:"ruleVersion"`
	RuleHash           string    `json:"ruleHash"`
	PlanHash           string    `json:"planHash"`
	RequestedAt        time.Time `json:"requestedAt"`
	AttemptCount       int       `json:"attemptCount"`
}

type SchemaRequestPageInfo struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

type UnobservedSchemaRequests struct {
	Items   []SchemaRequestItem   `json:"items"`
	Page    SchemaRequestPageInfo `json:"page"`
	Summary struct {
		NoAttemptCount       int `json:"noAttemptCount"`
		AttemptRecordedCount int `json:"attemptRecordedCount"`
	} `json:"summary"`
	ReadAt time.Time `json:"readAt"`
}

type SchemaRequestAttempt struct {
	ID            uuid.UUID  `json:"id"`
	RequestID     uuid.UUID  `json:"requestId"`
	WorkspaceID   uuid.UUID  `json:"workspaceId"`
	RegisteredAt  time.Time  `json:"registeredAt"`
	ObservationID *uuid.UUID `json:"observationId"`
}

type SchemaRequestObservation struct {
	ID         uuid.UUID  `json:"id"`
	RequestID  uuid.UUID  `json:"requestId"`
	AttemptID  *uuid.UUID `json:"attemptId"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason"`
	RecordedAt time.Time  `json:"recordedAt"`
}

type SchemaRequestDetail struct {
	SchemaRequestItem
	BindingID       uuid.UUID             `json:"bindingId"`
	ExternalID      string                `json:"externalId"`
	ConnectorHash   string                `json:"connectorHash"`
	Baseline        []d.SchemaColumn      `json:"baseline"`
	SelectedAttempt *SchemaRequestAttempt `json:"selectedAttempt"`
	Attempts        struct {
		Items []SchemaRequestAttempt `json:"items"`
		Page  SchemaRequestPageInfo  `json:"page"`
	} `json:"attempts"`
	Observations struct {
		Items []SchemaRequestObservation `json:"items"`
		Page  SchemaRequestPageInfo      `json:"page"`
	} `json:"observations"`
	ReadAt time.Time `json:"readAt"`
}

func ValidSchemaRequestView(view string) bool {
	return view == "ALL" || view == "NO_ATTEMPT" || view == "ATTEMPT_RECORDED"
}

func validSchemaRequestPage(limit, offset int) bool {
	return limit >= 1 && limit <= 100 && offset >= 0 && offset <= 2147483647
}

const schemaRequestFields = `r.workspace_id AS "workspaceId",r.resource_id AS "resourceId",
 resource.code AS "resourceCode",resource.name AS "resourceName",r.id AS "requestId",
 r.actor_id AS "responsibleActorId",r.external_fqn AS "externalFqn",r.rule_ref AS "ruleRef",
 r.rule_version AS "ruleVersion",r.rule_hash AS "ruleHash",r.plan_hash AS "planHash",
 r.requested_at AS "requestedAt",
 (SELECT count(*) FROM source_schema_attempt a WHERE a.request_id=r.id AND a.workspace_id=r.workspace_id) AS "attemptCount"`

func (r *PostgresRepository) UnobservedSchemaRequests(ctx context.Context, ws uuid.UUID, view string, limit, offset int) (UnobservedSchemaRequests, error) {
	var result UnobservedSchemaRequests
	if ws == uuid.Nil || !ValidSchemaRequestView(view) || !validSchemaRequestPage(limit, offset) {
		return result, errors.New("invalid unobserved schema request query")
	}
	var body []byte
	// Every observation kind excludes the request. Counts and page share this
	// statement snapshot; attempts cannot multiply request rows.
	err := r.pool.QueryRow(ctx, `WITH requests AS MATERIALIZED (
 SELECT `+schemaRequestFields+` FROM source_schema_request r
 JOIN data_resource resource ON resource.id=r.resource_id AND resource.workspace_id=r.workspace_id
 WHERE r.workspace_id=$1 AND NOT EXISTS(SELECT 1 FROM source_schema_observation o WHERE o.request_id=r.id)
), filtered AS (
 SELECT * FROM requests WHERE $2='ALL' OR ($2='NO_ATTEMPT' AND "attemptCount"=0) OR ($2='ATTEMPT_RECORDED' AND "attemptCount">0)
), page AS (
 SELECT * FROM filtered ORDER BY "requestedAt" DESC,"requestId" DESC LIMIT $3 OFFSET $4
)
SELECT jsonb_build_object(
 'items',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p."requestedAt" DESC,p."requestId" DESC) FROM page p),'[]'::jsonb),
 'page',jsonb_build_object('limit',$3::int,'offset',$4::int,'total',(SELECT count(*) FROM filtered)),
 'summary',jsonb_build_object('noAttemptCount',(SELECT count(*) FROM requests WHERE "attemptCount"=0),
 'attemptRecordedCount',(SELECT count(*) FROM requests WHERE "attemptCount">0)),'readAt',statement_timestamp())`, ws, view, limit, offset).Scan(&body)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}

func (r *PostgresRepository) SchemaRequestDetail(ctx context.Context, ws, resource, request uuid.UUID, attempt *uuid.UUID, limit, attemptOffset, observationOffset int) (SchemaRequestDetail, error) {
	var result SchemaRequestDetail
	if ws == uuid.Nil || resource == uuid.Nil || request == uuid.Nil || (attempt != nil && *attempt == uuid.Nil) || !validSchemaRequestPage(limit, attemptOffset) || !validSchemaRequestPage(limit, observationOffset) {
		return result, errors.New("invalid schema request detail query")
	}
	var body []byte
	// Detail remains addressable after an observation arrives. Never pick latest,
	// and never expose the opaque provider runtime contract to this read surface.
	err := r.pool.QueryRow(ctx, `WITH target AS MATERIALIZED (
 SELECT `+schemaRequestFields+`,r.binding_id AS "bindingId",r.external_id AS "externalId",
 r.plan->>'connectorHash' AS "connectorHash",r.plan->'columns' AS baseline
 FROM source_schema_request r JOIN data_resource resource ON resource.id=r.resource_id AND resource.workspace_id=r.workspace_id
 WHERE r.workspace_id=$1 AND r.resource_id=$2 AND r.id=$3
 AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM source_schema_attempt a WHERE a.id=$4 AND a.request_id=r.id AND a.workspace_id=r.workspace_id))
), attempts AS MATERIALIZED (
 SELECT a.id,a.request_id AS "requestId",a.workspace_id AS "workspaceId",a.started_at AS "registeredAt",o.id AS "observationId"
 FROM target t JOIN source_schema_attempt a ON a.request_id=t."requestId" AND a.workspace_id=t."workspaceId"
 LEFT JOIN source_schema_observation o ON o.attempt_id=a.id AND o.request_id=a.request_id
), observations AS MATERIALIZED (
 SELECT o.id,o.request_id AS "requestId",o.attempt_id AS "attemptId",o.status,o.reason,o.recorded_at AS "recordedAt"
 FROM target t JOIN source_schema_observation o ON o.request_id=t."requestId"
), attempt_page AS (SELECT * FROM attempts ORDER BY "registeredAt" DESC,id DESC LIMIT $5 OFFSET $6),
 observation_page AS (SELECT * FROM observations ORDER BY "recordedAt" DESC,id DESC LIMIT $5 OFFSET $7)
SELECT to_jsonb(t)||jsonb_build_object(
 'selectedAttempt',(SELECT to_jsonb(a) FROM attempts a WHERE a.id=$4),
 'attempts',jsonb_build_object('items',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY a."registeredAt" DESC,a.id DESC) FROM attempt_page a),'[]'::jsonb),
 'page',jsonb_build_object('limit',$5::int,'offset',$6::int,'total',(SELECT count(*) FROM attempts))),
 'observations',jsonb_build_object('items',COALESCE((SELECT jsonb_agg(to_jsonb(o) ORDER BY o."recordedAt" DESC,o.id DESC) FROM observation_page o),'[]'::jsonb),
 'page',jsonb_build_object('limit',$5::int,'offset',$7::int,'total',(SELECT count(*) FROM observations))),
 'readAt',statement_timestamp()) FROM target t`, ws, resource, request, attempt, limit, attemptOffset, observationOffset).Scan(&body)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}
