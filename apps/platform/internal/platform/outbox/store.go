package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Event struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	Payload       json.RawMessage
	AvailableAt   time.Time
}

func NewEvent(aggregateType string, aggregateID uuid.UUID, eventType string, payload any) (Event, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("marshal outbox payload: %w", err)
	}
	return Event{
		ID:            uuid.New(),
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		EventType:     eventType,
		Payload:       encoded,
		AvailableAt:   time.Now().UTC(),
	}, nil
}

func Append(ctx context.Context, tx pgx.Tx, event Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.AvailableAt.IsZero() {
		event.AvailableAt = time.Now().UTC()
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO outbox_event (
			id, aggregate_type, aggregate_id, event_type, payload, available_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, event.ID, event.AggregateType, event.AggregateID, event.EventType, event.Payload, event.AvailableAt)
	if err != nil {
		return fmt.Errorf("append outbox event: %w", err)
	}
	return nil
}
