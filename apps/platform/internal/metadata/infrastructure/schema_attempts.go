package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Unobserved attempts in already observed requests are recorded facts, not runtime states.
type SchemaUnobservedAttempt struct {
	SchemaRequestItem
	AttemptID               uuid.UUID `json:"attemptId"`
	RegisteredAt            time.Time `json:"registeredAt"`
	RequestObservationCount int       `json:"requestObservationCount"`
}

type SchemaUnobservedAttempts struct {
	Items  []SchemaUnobservedAttempt `json:"items"`
	Page   SchemaRequestPageInfo     `json:"page"`
	ReadAt time.Time                 `json:"readAt"`
}

func (r *PostgresRepository) UnobservedAttemptsInObservedRequests(ctx context.Context, ws uuid.UUID, limit, offset int) (SchemaUnobservedAttempts, error) {
	var result SchemaUnobservedAttempts
	if ws == uuid.Nil || !validSchemaRequestPage(limit, offset) {
		return result, errors.New("invalid unobserved schema attempt query")
	}
	var body []byte
	// One attempt per row. EXISTS avoids multiplying rows by request history;
	// only an observation bound to this exact attempt removes it from the set.
	// No runtime contract, current provider or worker recovery lookup is needed.
	err := r.pool.QueryRow(ctx, `WITH attempts AS MATERIALIZED (
 SELECT `+schemaRequestFields+`,a.id AS "attemptId",a.started_at AS "registeredAt",
 (SELECT count(*) FROM source_schema_observation o WHERE o.request_id=r.id) AS "requestObservationCount"
 FROM source_schema_attempt a JOIN source_schema_request r ON r.id=a.request_id AND r.workspace_id=a.workspace_id
 JOIN data_resource resource ON resource.id=r.resource_id AND resource.workspace_id=r.workspace_id
 WHERE r.workspace_id=$1
 AND EXISTS(SELECT 1 FROM source_schema_observation o WHERE o.request_id=r.id)
 AND NOT EXISTS(SELECT 1 FROM source_schema_observation o WHERE o.attempt_id=a.id)
), page AS (
 SELECT * FROM attempts ORDER BY "registeredAt" DESC,"attemptId" DESC LIMIT $2 OFFSET $3
)
SELECT jsonb_build_object(
 'items',COALESCE((SELECT jsonb_agg(to_jsonb(p) ORDER BY p."registeredAt" DESC,p."attemptId" DESC) FROM page p),'[]'::jsonb),
 'page',jsonb_build_object('limit',$2::int,'offset',$3::int,'total',(SELECT count(*) FROM attempts)),
 'readAt',statement_timestamp())`, ws, limit, offset).Scan(&body)
	if err != nil {
		return result, err
	}
	err = json.Unmarshal(body, &result)
	return result, err
}
