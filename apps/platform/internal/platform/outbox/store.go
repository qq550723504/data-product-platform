package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
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

	// RoutingVersion and RequiredHandlers freeze the handler obligation of the
	// event at the moment it is recorded. Every persisted event must carry this
	// obligation. Retention-only is represented by a non-empty RoutingVersion
	// with a non-nil empty RequiredHandlers slice.
	RoutingVersion   string
	RequiredHandlers []string
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

// ObligationSource resolves the handler obligation for an event type. It is
// consulted by Append so the obligation is frozen when the business fact is
// recorded, not whenever some instance happens to dispatch it.
type ObligationSource interface {
	// Obligation returns the routing version and required handler names for the
	// event type. ok is false when the event type is not declared; the caller
	// must not invent an obligation in that case.
	Obligation(eventType string) (routingVersion string, requiredHandlers []string, ok bool)
}

var (
	appendObligationMu sync.RWMutex
	appendObligation   ObligationSource
)

// ConfigureAppendObligation installs the process-wide obligation source used by
// Append. Composition roots call it once at startup with the routing table for
// their deployment profile, before any event is appended. Passing nil clears
// the source (used by tests).
func ConfigureAppendObligation(source ObligationSource) {
	appendObligationMu.Lock()
	defer appendObligationMu.Unlock()
	appendObligation = source
}

func obligationSource() ObligationSource {
	appendObligationMu.RLock()
	defer appendObligationMu.RUnlock()
	return appendObligation
}

// Append writes the event inside the caller's transaction so the event commits
// atomically with the aggregate change it announces.
//
// When the event does not already carry a routing obligation, Append must
// resolve one from the configured ObligationSource and freeze it on the row.
// Missing routing configuration or an undeclared event type is an error.
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
	if event.RoutingVersion == "" {
		source := obligationSource()
		if source == nil {
			return fmt.Errorf("append outbox event: routing obligation source is not configured")
		}
		version, handlers, ok := source.Obligation(event.EventType)
		if !ok {
			return fmt.Errorf(
				"append outbox event: event type %q is not declared in the routing table",
				event.EventType,
			)
		}
		event.RoutingVersion = version
		if handlers == nil {
			handlers = []string{}
		}
		event.RequiredHandlers = handlers
	}
	if event.RequiredHandlers == nil {
		event.RequiredHandlers = []string{}
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO outbox_event (
			id, aggregate_type, aggregate_id, event_type, event_version, payload, available_at,
			routing_version, required_handlers
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, event.ID, event.AggregateType, event.AggregateID, event.EventType,
		int16(event.EventVersion), event.Payload, event.AvailableAt,
		event.RoutingVersion, event.RequiredHandlers)
	if err != nil {
		return fmt.Errorf("append outbox event: %w", err)
	}
	return nil
}
