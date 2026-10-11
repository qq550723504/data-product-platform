package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/ingestion/domain"
)

type Repository struct{ Pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{Pool: pool} }

type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *Repository) Load(ctx context.Context, q Querier, id uuid.UUID, lock bool) (d.Operation, error) {
	var op d.Operation
	var manifest []byte
	sql := `SELECT id,workspace_id,actor_id,principal_ref,request_key,batch_key,connection_ref,instance_ref,config_hash,template_hash,schema_hash,state,COALESCE(current_attempt_id,'00000000-0000-0000-0000-000000000000'),remote_id,manifest FROM ingestion_operation WHERE id=$1`
	if lock {
		sql += " FOR UPDATE"
	}
	err := q.QueryRow(ctx, sql, id).Scan(&op.ID, &op.Request.WorkspaceID, &op.Request.ActorID, &op.Request.PrincipalRef, &op.Request.RequestKey, &op.Request.BatchKey, &op.Request.ConnectionRef, &op.Request.InstanceRef, &op.Request.ConfigHash, &op.Request.TemplateHash, &op.Request.SchemaHash, &op.State, &op.AttemptID, &op.RemoteID, &manifest)
	if err != nil {
		return op, err
	}
	if len(manifest) > 0 {
		op.Manifest = &d.Manifest{}
		if err := json.Unmarshal(manifest, op.Manifest); err != nil {
			return op, err
		}
	}
	rows, err := q.Query(ctx, `SELECT code,dataset_id,resource_id,columns,row_count FROM ingestion_request_slot WHERE operation_id=$1 ORDER BY slot_no`, id)
	if err != nil {
		return op, err
	}
	defer rows.Close()
	for rows.Next() {
		var slot d.Slot
		if err := rows.Scan(&slot.Code, &slot.DatasetID, &slot.ResourceID, &slot.Columns, &slot.Rows); err != nil {
			return op, err
		}
		op.Request.Slots = append(op.Request.Slots, slot)
	}
	return op, rows.Err()
}
func (r *Repository) Prepare(ctx context.Context, tx pgx.Tx, req d.Request) (d.Operation, bool, error) {
	fp, err := req.Fingerprint()
	if err != nil {
		return d.Operation{}, false, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "ingestion/"+req.WorkspaceID.String()+"/"+req.RequestKey); err != nil {
		return d.Operation{}, false, err
	}
	var id uuid.UUID
	var old string
	err = tx.QueryRow(ctx, `SELECT id,request_fingerprint FROM ingestion_operation WHERE workspace_id=$1 AND request_key=$2`, req.WorkspaceID, req.RequestKey).Scan(&id, &old)
	if err == nil {
		if old != fp {
			return d.Operation{}, false, d.ErrConflict
		}
		op, err := r.Load(ctx, tx, id, true)
		return op, false, err
	}
	if err != pgx.ErrNoRows {
		return d.Operation{}, false, err
	}
	op := d.Operation{ID: uuid.New(), Request: req, State: "PREPARED"}
	_, err = tx.Exec(ctx, `INSERT INTO ingestion_operation(id,workspace_id,actor_id,principal_ref,request_key,batch_key,connection_ref,instance_ref,config_hash,template_hash,schema_hash,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, op.ID, req.WorkspaceID, req.ActorID, req.PrincipalRef, req.RequestKey, req.BatchKey, req.ConnectionRef, req.InstanceRef, req.ConfigHash, req.TemplateHash, req.SchemaHash, fp)
	if err != nil {
		return op, false, err
	}
	for i, s := range req.Slots {
		if _, err = tx.Exec(ctx, `INSERT INTO ingestion_request_slot(operation_id,slot_no,code,dataset_id,resource_id,columns,row_count) VALUES($1,$2,$3,$4,$5,$6,$7)`, op.ID, i+1, s.Code, s.DatasetID, s.ResourceID, s.Columns, s.Rows); err != nil {
			return op, false, err
		}
	}
	if err = r.NewAttempt(ctx, tx, &op); err != nil {
		return op, false, err
	}
	return op, true, nil
}
func (r *Repository) NewAttempt(ctx context.Context, tx pgx.Tx, op *d.Operation) error {
	op.AttemptID = uuid.New()
	op.State = "PREPARED"
	op.RemoteID = ""
	op.Manifest = nil
	if _, err := tx.Exec(ctx, `INSERT INTO ingestion_attempt(id,operation_id) VALUES($1,$2)`, op.AttemptID, op.ID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE ingestion_operation SET current_attempt_id=$2,state='PREPARED',remote_id='' WHERE id=$1`, op.ID, op.AttemptID)
	return err
}
func (r *Repository) Claim(ctx context.Context, tx pgx.Tx, op *d.Operation, kind string) (uuid.UUID, error) {
	id := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO ingestion_invocation(id,operation_id,workspace_id,attempt_id,kind) VALUES($1,$2,$3,$4,$5)`, id, op.ID, op.Request.WorkspaceID, op.AttemptID, kind); err != nil {
		return uuid.Nil, err
	}
	costID := uuid.New()
	if err := cost.Append(ctx, tx, cost.Event{ID: costID, WorkspaceID: op.Request.WorkspaceID, ActivityID: id, CostType: "INGESTION_ENGINE_INVOCATION", Quantity: 1, Unit: "invocation", PricingMode: "ACTUAL", Metadata: map[string]any{"kind": kind}}); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO cost_allocation(id,cost_event_id,ingestion_invocation_id) VALUES($1,$2,$3)`, uuid.New(), costID, id); err != nil {
		return uuid.Nil, err
	}
	switch kind {
	case "REGISTER":
		op.State = "REGISTERING"
	case "START":
		op.State = "STARTING"
	}
	_, err := tx.Exec(ctx, `UPDATE ingestion_operation SET state=$2 WHERE id=$1`, op.ID, op.State)
	return id, err
}
func (r *Repository) Observe(ctx context.Context, tx pgx.Tx, op *d.Operation, invocationID uuid.UUID, state, remote string, m *d.Manifest) error {
	if remote != "" && op.RemoteID != "" && remote != op.RemoteID {
		return d.ErrConflict
	}
	if remote != "" {
		op.RemoteID = remote
	}
	if _, err := tx.Exec(ctx, `INSERT INTO ingestion_observation(id,invocation_id,state,remote_id) VALUES($1,$2,$3,$4)`, uuid.New(), invocationID, state, remote); err != nil {
		return err
	}
	var data []byte
	var hash *string
	if m != nil {
		var h string
		var err error
		data, h, err = m.Canonical()
		if err != nil {
			return err
		}
		hash = &h
		op.Manifest = m
	}
	op.State = state
	_, err := tx.Exec(ctx, `UPDATE ingestion_operation SET state=$2,remote_id=$3,manifest=COALESCE($4::jsonb,manifest),manifest_hash=COALESCE($5,manifest_hash) WHERE id=$1`, op.ID, state, op.RemoteID, data, hash)
	return err
}
func (r *Repository) Receipt(ctx context.Context, q Querier, id uuid.UUID) (d.Receipt, error) {
	var rec d.Receipt
	err := q.QueryRow(ctx, `SELECT id,operation_id,attempt_id,manifest_hash FROM ingestion_receipt WHERE operation_id=$1`, id).Scan(&rec.ID, &rec.OperationID, &rec.AttemptID, &rec.ManifestHash)
	if err != nil {
		return rec, err
	}
	rows, err := q.Query(ctx, `SELECT code,dataset_id,version_id,storage_uri,content_hash FROM ingestion_receipt_slot WHERE receipt_id=$1 ORDER BY code`, rec.ID)
	if err != nil {
		return rec, err
	}
	defer rows.Close()
	for rows.Next() {
		var b d.Binding
		if err := rows.Scan(&b.Code, &b.DatasetID, &b.VersionID, &b.URI, &b.SHA256); err != nil {
			return rec, err
		}
		rec.Bindings = append(rec.Bindings, b)
	}
	if len(rec.Bindings) != 3 {
		return rec, fmt.Errorf("partial receipt")
	}
	return rec, rows.Err()
}
