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

func (r *PostgresRepository) InsertQuarantine(ctx context.Context, tx pgx.Tx, record QuarantineRecord) error {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	payload, err := json.Marshal(record.Payload)
	if err != nil {
		return fmt.Errorf("marshal quarantine payload: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO execution_quarantine_record (
			id, execution_id, input_name, source_key, reason_code, reason_message, payload
		) VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),$7)
	`, record.ID, record.ExecutionID, record.InputName, record.SourceKey, record.ReasonCode, record.ReasonMessage, payload)
	if err != nil {
		return fmt.Errorf("insert execution quarantine record: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CountQuarantine(ctx context.Context, executionID uuid.UUID) (int64, error) {
	var count int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM execution_quarantine_record WHERE execution_id=$1`, executionID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count execution quarantine records: %w", err)
	}
	return count, nil
}
