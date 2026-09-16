package cost

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
	WorkspaceID  uuid.UUID
	ExecutionID  *uuid.UUID
	CostType     string
	Quantity     float64
	Unit         string
	Amount       *float64
	Currency     string
	PricingMode  string
	Metadata     map[string]any
	OccurredAt   time.Time
}

func Append(ctx context.Context, tx pgx.Tx, event Event) error {
	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	if event.PricingMode == "" {
		event.PricingMode = "POC_ESTIMATE"
	}
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal cost metadata: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cost_event (
			id, workspace_id, execution_id, cost_type, quantity, unit,
			amount, currency, pricing_mode, metadata, occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, event.ID, event.WorkspaceID, event.ExecutionID, event.CostType, event.Quantity, event.Unit,
		event.Amount, nullable(event.Currency), event.PricingMode, metadata, event.OccurredAt)
	if err != nil {
		return fmt.Errorf("append cost event: %w", err)
	}
	return nil
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
