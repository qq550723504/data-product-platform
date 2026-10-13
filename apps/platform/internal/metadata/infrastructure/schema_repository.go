package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
)

var ErrSchemaConflict = errors.New("schema request key conflicts with frozen request")

func (r *PostgresRepository) PrepareSchema(ctx context.Context, tx pgx.Tx, req d.SchemaRequest) (d.SchemaRequest, bool, error) {
	plan, _ := json.Marshal(req.Plan)
	_, err := tx.Exec(ctx, `INSERT INTO source_schema_request(id,workspace_id,resource_id,actor_id,authorization_id,consumer_ref,purpose,binding_id,external_id,external_fqn,rule_ref,plan_hash,plan,request_key,rule_version,rule_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT(workspace_id,request_key) DO NOTHING`, req.ID, req.Plan.WorkspaceID, req.Plan.ResourceID, req.Plan.ActorID, req.Plan.AuthorizationID, req.Plan.ConsumerRef, req.Plan.Purpose, req.BindingID, req.ExternalID, req.Plan.ExternalFQN, req.Plan.RuleRef, req.PlanHash, plan, req.Key, req.Plan.RuleVersion, req.Plan.RuleHash)
	if err != nil {
		return req, false, err
	}
	prior, err := readSchemaRequest(ctx, tx, req.Plan.WorkspaceID, req.Key)
	if err != nil {
		return req, false, err
	}
	if prior.PlanHash != req.PlanHash || prior.BindingID != req.BindingID || prior.ExternalID != req.ExternalID {
		return req, false, ErrSchemaConflict
	}
	return prior, prior.ID == req.ID, nil
}

type schemaQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readSchemaRequest(ctx context.Context, q schemaQuery, ws uuid.UUID, key string) (d.SchemaRequest, error) {
	var v d.SchemaRequest
	var plan []byte
	err := q.QueryRow(ctx, `SELECT id,plan,plan_hash,binding_id,external_id,request_key,requested_at FROM source_schema_request WHERE workspace_id=$1 AND request_key=$2`, ws, key).Scan(&v.ID, &plan, &v.PlanHash, &v.BindingID, &v.ExternalID, &v.Key, &v.RequestedAt)
	if err == nil {
		err = json.Unmarshal(plan, &v.Plan)
	}
	if err == nil && (v.Plan.Validate() != nil || d.Hash(v.Plan) != v.PlanHash) {
		err = ErrSchemaConflict
	}
	return v, err
}
func (r *PostgresRepository) SchemaRequest(ctx context.Context, id uuid.UUID) (d.SchemaRequest, error) {
	var v d.SchemaRequest
	var plan []byte
	err := r.pool.QueryRow(ctx, `SELECT id,plan,plan_hash,binding_id,external_id,request_key,requested_at FROM source_schema_request WHERE id=$1`, id).Scan(&v.ID, &plan, &v.PlanHash, &v.BindingID, &v.ExternalID, &v.Key, &v.RequestedAt)
	if err == nil {
		err = json.Unmarshal(plan, &v.Plan)
	}
	if err == nil && (v.Plan.Validate() != nil || d.Hash(v.Plan) != v.PlanHash) {
		err = ErrSchemaConflict
	}
	return v, err
}
func (r *PostgresRepository) SchemaTerminal(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bool, error) {
	var done bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_schema_observation WHERE request_id=$1 AND status IN ('MATCH','DRIFT','DENIED'))`, id).Scan(&done)
	return done, err
}
func (r *PostgresRepository) BeginSchemaAttempt(ctx context.Context, tx pgx.Tx, req d.SchemaRequest, id uuid.UUID, runtime json.RawMessage) error {
	hash, err := d.RuntimeHash(runtime)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO source_schema_attempt(id,request_id,workspace_id) VALUES($1,$2,$3)`, id, req.ID, req.Plan.WorkspaceID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO source_schema_attempt_runtime(attempt_id,contract,contract_hash) VALUES($1,$2,$3)`, id, runtime, hash)
	return err
}
func (r *PostgresRepository) PendingSchemaAttempts(ctx context.Context, resource uuid.UUID) ([]d.SchemaAttempt, error) {
	rows, err := r.pool.Query(ctx, `SELECT a.id,a.request_id,s.contract,s.contract_hash FROM source_schema_attempt a JOIN source_schema_request r ON r.id=a.request_id LEFT JOIN source_schema_attempt_runtime s ON s.attempt_id=a.id WHERE r.resource_id=$1 AND NOT EXISTS(SELECT 1 FROM source_schema_observation o WHERE o.attempt_id=a.id) ORDER BY a.started_at,a.id`, resource)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []d.SchemaAttempt{}
	for rows.Next() {
		var a d.SchemaAttempt
		var contract []byte
		var hash *string
		if err := rows.Scan(&a.ID, &a.RequestID, &contract, &hash); err != nil {
			return nil, err
		}
		actual, err := d.RuntimeHash(contract)
		if err != nil || hash == nil || actual != *hash {
			return nil, errors.New("schema attempt runtime contract unavailable")
		}
		a.Runtime = contract
		a.RuntimeHash = *hash
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r *PostgresRepository) AppendSchemaObservation(ctx context.Context, tx pgx.Tx, o d.SchemaObservation) error {
	columns, _ := json.Marshal(o.Columns)
	if o.Columns == nil {
		columns = []byte("[]")
	}
	_, err := tx.Exec(ctx, `INSERT INTO source_schema_observation(id,request_id,attempt_id,status,reason,columns,schema_hash,started_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, o.ID, o.RequestID, o.AttemptID, o.Status, o.Reason, columns, o.SchemaHash, o.StartedAt, o.CompletedAt)
	return err
}
func (r *PostgresRepository) SchemaHistory(ctx context.Context, resource, ws uuid.UUID) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT r.id,r.actor_id,r.rule_ref,r.plan_hash,r.external_fqn,r.plan,r.requested_at,COALESCE((SELECT jsonb_agg(to_jsonb(o) ORDER BY recorded_at,id) FROM source_schema_observation o WHERE o.request_id=r.id),'[]'),COALESCE((SELECT jsonb_agg(to_jsonb(h) ORDER BY h.created_at,h.id) FROM source_schema_handling h JOIN source_schema_observation o ON o.id=h.observation_id WHERE o.request_id=r.id),'[]') FROM source_schema_request r WHERE r.resource_id=$1 AND r.workspace_id=$2 ORDER BY r.requested_at DESC,r.id DESC LIMIT 50`, resource, ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var id, actor uuid.UUID
		var ref, hash, fqn string
		var plan, observations, handling []byte
		var at time.Time
		if err := rows.Scan(&id, &actor, &ref, &hash, &fqn, &plan, &at, &observations, &handling); err != nil {
			return nil, err
		}
		var p d.SchemaPlan
		var os, hs []map[string]any
		if err := json.Unmarshal(plan, &p); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(observations, &os); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(handling, &hs); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{"id": id, "responsibleActorId": actor, "ruleRef": ref, "ruleVersion": p.RuleVersion, "ruleHash": p.RuleHash, "connectorHash": p.ConnectorHash, "planHash": hash, "externalFqn": fqn, "baseline": p.Columns, "requestedAt": at, "observations": os, "handling": hs})
	}
	return result, rows.Err()
}
func (r *PostgresRepository) HandleSchema(ctx context.Context, tx pgx.Tx, resource, ws, actor, observation uuid.UUID, key, reason string) (bool, error) {
	var valid bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM source_schema_observation o JOIN source_schema_request r ON r.id=o.request_id WHERE o.id=$1 AND r.resource_id=$2 AND r.workspace_id=$3 AND r.actor_id=$4 AND o.status='DRIFT')`, observation, resource, ws, actor).Scan(&valid)
	if err != nil {
		return false, err
	}
	if !valid {
		return false, ErrSchemaConflict
	}
	tag, err := tx.Exec(ctx, `INSERT INTO source_schema_handling(id,observation_id,actor_id,reason,request_key) VALUES($1,$2,$3,$4,$5) ON CONFLICT(observation_id,request_key) DO NOTHING`, uuid.New(), observation, actor, reason, key)
	if err != nil {
		return false, err
	}
	var saved string
	err = tx.QueryRow(ctx, `SELECT reason FROM source_schema_handling WHERE observation_id=$1 AND request_key=$2`, observation, key).Scan(&saved)
	if saved != reason {
		return false, ErrSchemaConflict
	}
	return tag.RowsAffected() == 1, err
}
