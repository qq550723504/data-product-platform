package cost

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
	ID          uuid.UUID      `json:"id"`
	WorkspaceID uuid.UUID      `json:"workspaceId"`
	ExecutionID *uuid.UUID     `json:"executionId,omitempty"`
	ActivityID  *uuid.UUID     `json:"activityId,omitempty"`
	CostType    string         `json:"costType"`
	Quantity    float64        `json:"quantity"`
	Unit        string         `json:"unit"`
	Amount      *float64       `json:"amount,omitempty"`
	Currency    string         `json:"currency,omitempty"`
	PricingMode string         `json:"pricingMode"`
	Metadata    map[string]any `json:"metadata"`
	OccurredAt  time.Time      `json:"occurredAt"`
}

type QueryRepository struct {
	pool *pgxpool.Pool
}

func NewQueryRepository(pool *pgxpool.Pool) *QueryRepository {
	return &QueryRepository{pool: pool}
}

func (r *QueryRepository) ListByExecution(ctx context.Context, executionID uuid.UUID) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, workspace_id, execution_id, cost_type, quantity, unit,
		       activity_id, amount, COALESCE(currency,''), pricing_mode, metadata, occurred_at
		FROM cost_event
		WHERE execution_id = $1
		ORDER BY occurred_at, id
	`, executionID)
	if err != nil {
		return nil, fmt.Errorf("query cost events for execution %s: %w", executionID, err)
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		var item Item
		var metadata []byte
		if err := rows.Scan(
			&item.ID,
			&item.WorkspaceID,
			&item.ExecutionID,
			&item.CostType,
			&item.Quantity,
			&item.Unit,
			&item.ActivityID,
			&item.Amount,
			&item.Currency,
			&item.PricingMode,
			&metadata,
			&item.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan cost event query result: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &item.Metadata); err != nil {
				return nil, fmt.Errorf("decode cost event metadata: %w", err)
			}
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost event query results: %w", err)
	}
	return items, nil
}

func (r *QueryRepository) ListByQualityAssessment(ctx context.Context, assessmentID uuid.UUID) ([]Item, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.workspace_id, e.execution_id, e.activity_id, e.cost_type,
		       e.quantity, e.unit, e.amount, COALESCE(e.currency,''), e.pricing_mode,
		       e.metadata, e.occurred_at
		FROM cost_event e
		JOIN cost_allocation a ON a.cost_event_id=e.id
		LEFT JOIN quality_assessment_attempt_outcome o
		  ON o.attempt_id=a.quality_assessment_attempt_id
		WHERE a.quality_assessment_id=$1
		   OR (a.quality_assessment_attempt_id IS NOT NULL
		       AND o.assessment_id=$1 AND o.outcome='SUCCEEDED')
		ORDER BY e.occurred_at, e.id
	`, assessmentID)
	if err != nil {
		return nil, fmt.Errorf("query cost events for quality assessment %s: %w", assessmentID, err)
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		var item Item
		var metadata []byte
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.ExecutionID, &item.ActivityID,
			&item.CostType, &item.Quantity, &item.Unit, &item.Amount,
			&item.Currency, &item.PricingMode, &metadata, &item.OccurredAt,
		); err != nil {
			return nil, fmt.Errorf("scan quality assessment cost event: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &item.Metadata); err != nil {
				return nil, fmt.Errorf("decode quality assessment cost metadata: %w", err)
			}
		}
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate quality assessment cost events: %w", err)
	}
	return items, nil
}
