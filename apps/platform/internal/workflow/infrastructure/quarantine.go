package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type QuarantineRecord struct {
	ID            uuid.UUID
	ExecutionID   uuid.UUID
	InputName     string
	SourceKey     string
	ReasonCode    string
	ReasonMessage string
	Payload       map[string]any
}

// ListQuarantine returns a bounded page from the existing execution evidence.
// The workspace predicate is enforced in SQL as well as by the HTTP boundary.
func (r *PostgresRepository) ListQuarantine(ctx context.Context, workspaceID, executionID uuid.UUID, limit, offset int) ([]QuarantineRecord, int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT q.id,q.execution_id,q.input_name,COALESCE(q.source_key,''),q.reason_code,COALESCE(q.reason_message,''),q.payload
		FROM execution_quarantine_record q JOIN execution e ON e.id=q.execution_id
		WHERE e.workspace_id=$1 AND e.id=$2 ORDER BY q.input_name,q.id LIMIT $3 OFFSET $4`, workspaceID, executionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]QuarantineRecord, 0)
	for rows.Next() {
		var v QuarantineRecord
		if err := rows.Scan(&v.ID, &v.ExecutionID, &v.InputName, &v.SourceKey, &v.ReasonCode, &v.ReasonMessage, &v.Payload); err != nil {
			return nil, 0, err
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var total int64
	err = r.pool.QueryRow(ctx, `SELECT count(*) FROM execution_quarantine_record q JOIN execution e ON e.id=q.execution_id WHERE e.workspace_id=$1 AND e.id=$2`, workspaceID, executionID).Scan(&total)
	return items, total, err
}

func (r *PostgresRepository) InsertQuarantine(ctx context.Context, tx pgx.Tx, record QuarantineRecord) error {
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return fmt.Errorf("marshal quarantine payload: %w", err)
	}
	if record.ID == uuid.Nil {
		record.ID = quarantineRecordID(record, payload)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO execution_quarantine_record (
			id, execution_id, input_name, source_key, reason_code, reason_message, payload
		) VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),$7)
		ON CONFLICT (id) DO NOTHING
	`, record.ID, record.ExecutionID, record.InputName, record.SourceKey, record.ReasonCode, record.ReasonMessage, payload)
	if err != nil {
		return fmt.Errorf("insert execution quarantine record: %w", err)
	}
	return nil
}

func quarantineRecordID(record QuarantineRecord, canonicalPayload []byte) uuid.UUID {
	identity := []byte(
		"execution-quarantine:" + record.ExecutionID.String() + "\x00" +
			record.InputName + "\x00" + record.SourceKey + "\x00" + record.ReasonCode + "\x00",
	)
	identity = append(identity, canonicalPayload...)
	return uuid.NewSHA1(uuid.NameSpaceURL, identity)
}

func (r *PostgresRepository) CountQuarantine(ctx context.Context, executionID uuid.UUID) (int64, error) {
	var count int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM execution_quarantine_record WHERE execution_id=$1`, executionID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count execution quarantine records: %w", err)
	}
	return count, nil
}
