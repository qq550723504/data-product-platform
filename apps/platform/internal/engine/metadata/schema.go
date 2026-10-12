package metadata

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"time"
)

// SourceSchemaObserver executes a native source discovery workflow. A plain
// catalog GET or catalog revision is deliberately insufficient for this port.
type SourceSchemaObserver interface {
	Plan(context.Context) (domain.SchemaPlan, error)
	// Freeze a secret-free engine runtime contract before committing the attempt.
	Prepare(context.Context, uuid.UUID, domain.SchemaPlan) (json.RawMessage, error)
	// A nil error confirms completed discovery and a terminated native process.
	// Stop is required for failed/unknown attempts and crash recovery, not success.
	Scan(context.Context, uuid.UUID, json.RawMessage, func() error) (SourceSchema, error)
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
