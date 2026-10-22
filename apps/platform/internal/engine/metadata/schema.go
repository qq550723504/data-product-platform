package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"time"
)

var ErrSourceSchemaCostUnconfirmed = errors.New("source schema invocation cost unconfirmed")
var ErrSourceSchemaInvocationUnknown = errors.New("source schema invocation history unavailable")

// SourceSchemaRunGate executes only the bounded source-start handoff while the
// fresh authority fence and exact binding locks are held, not the full scan.
type SourceSchemaRunGate func(handoff func() error) error

// SourceSchemaObserver executes a native source discovery workflow. A plain
// catalog GET or catalog revision is deliberately insufficient for this port.
type SourceSchemaObserver interface {
	Plan(context.Context) (domain.SchemaPlan, error)
	// Freeze a secret-free engine runtime contract before committing the attempt.
	Prepare(context.Context, uuid.UUID, domain.SchemaPlan) (json.RawMessage, error)
	// Read the retained provider start receipt; false means a prepared runtime
	// never started. Unavailable/deleted receipts must return an error.
	Invocation(context.Context, uuid.UUID, json.RawMessage) (bool, error)
	// A nil error confirms completed discovery and a terminated native process.
	// Stop is required for failed/unknown attempts and crash recovery, not success.
	// First callback records the durable native start receipt before protocol output;
	// second reauthorizes and sends RUN under the same authority/binding locks.
	Scan(context.Context, uuid.UUID, json.RawMessage, func() error, SourceSchemaRunGate) (SourceSchema, error)
	Stop(context.Context, uuid.UUID, json.RawMessage) error
}
type SourceSchema struct {
	ExternalID    string                `json:"externalId"`
	ExternalFQN   string                `json:"externalFqn"`
	ConnectorHash string                `json:"connectorHash"`
	Columns       []domain.SchemaColumn `json:"columns"`
	StartedAt     time.Time             `json:"startedAt"`
	CompletedAt   time.Time             `json:"completedAt"`
}
