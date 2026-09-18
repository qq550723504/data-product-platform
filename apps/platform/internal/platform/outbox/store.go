package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Event is the write-side representation of an outbox event. Payload structure
// changes bump EventVersion; historical rows are never rewritten in place.
type Event struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	EventType     string
	EventVersion  int
	Payload       json.RawMessage
	AvailableAt   time.Time
}

// NewEvent builds a version-1 event with a flat payload.
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
		EventVersion:  MaxSupportedEventVersion,
		Payload:       encoded,
		AvailableAt:   time.Now().UTC(),
	}, nil
}

// Append writes the event inside the caller's transaction so the event commits
// atomically with the aggregate change it announces.
func Append(ctx context.Context, tx pgx.Tx, event Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.AvailableAt.IsZero() {
		event.AvailableAt = time.Now().UTC()
	}
	if event.EventVersion == 0 {
		event.EventVersion = MaxSupportedEventVersion
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO outbox_event (
			id, aggregate_type, aggregate_id, event_type, event_version, payload, available_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, event.ID, event.AggregateType, event.AggregateID, event.EventType,
		int16(event.EventVersion), event.Payload, event.AvailableAt)
	if err != nil {
		return fmt.Errorf("append outbox event: %w", err)
	}
	return nil
}
