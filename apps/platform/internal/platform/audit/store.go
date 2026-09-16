package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Event struct {
	ID           uuid.UUID
	WorkspaceID  *uuid.UUID
	ActorType    string
	ActorID      *uuid.UUID
	Action       string
	ObjectType   string
	ObjectID     uuid.UUID
	BeforeState  any
	AfterState   any
	Reason       string
	TraceID      string
	Metadata     any
	OccurredAt   time.Time
}

func Append(ctx context.Context, tx pgx.Tx, event Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.ActorType == "" {
		event.ActorType = "SYSTEM"
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	before, err := marshalNullable(event.BeforeState)
	if err != nil {
		return fmt.Errorf("marshal audit before state: %w", err)
	}
	after, err := marshalNullable(event.AfterState)
	if err != nil {
		return fmt.Errorf("marshal audit after state: %w", err)
	}
	metadata, err := marshalDefaultObject(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO audit_event (
			id, workspace_id, actor_type, actor_id, action,
			object_type, object_id, before_state, after_state,
			reason, trace_id, metadata, occurred_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`,
		event.ID, event.WorkspaceID, event.ActorType, event.ActorID, event.Action,
		event.ObjectType, event.ObjectID, before, after,
		event.Reason, event.TraceID, metadata, event.OccurredAt,
	)
	if err != nil {
		return fmt.Errorf("append audit event: %w", err)
	}
	return nil
}

func marshalNullable(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return json.Marshal(value)
}

func marshalDefaultObject(value any) ([]byte, error) {
	if value == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(value)
}
