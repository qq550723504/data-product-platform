package cost

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQueryCostAmountsRemainExact(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse postgres DSN: %v", err)
	}
	// Temporary tables are connection-local: force the query repository and
	// fixture DDL to use the same connection without touching shared CI data.
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	defer pool.Close()

	for _, ddl := range []string{
		`CREATE TEMP TABLE cost_event (
			id uuid, workspace_id uuid, execution_id uuid, activity_id uuid,
			cost_type text, quantity numeric(20,6), unit text, amount numeric(20,6),
			currency text, pricing_mode text, metadata jsonb, occurred_at timestamptz
		)`,
		`CREATE TEMP TABLE cost_allocation (cost_event_id uuid, quality_assessment_id uuid, quality_assessment_attempt_id uuid)`,
		`CREATE TEMP TABLE quality_assessment_attempt_outcome (attempt_id uuid, assessment_id uuid, outcome text)`,
	} {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			t.Fatalf("create temporary cost tables: %v", err)
		}
	}

	workspaceID, executionID, assessmentID := uuid.New(), uuid.New(), uuid.New()
	values := []struct {
		amount string
	}{
		{"12345678901234.123456"},
		{"0.000001"},
		{""},
	}
	for _, v := range values {
		id := uuid.New()
		var amount any
		if v.amount != "" {
			amount = v.amount
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO cost_event (id,workspace_id,execution_id,activity_id,cost_type,quantity,unit,amount,currency,pricing_mode,metadata,occurred_at)
			VALUES ($1,$2,$3,$4,'TEST',1,'unit',$5,'USD','ACTUAL','{}'::jsonb,now())
		`, id, workspaceID, executionID, uuid.New(), amount); err != nil {
			t.Fatalf("insert cost fixture: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO cost_allocation(cost_event_id,quality_assessment_id) VALUES ($1,$2)`, id, assessmentID); err != nil {
			t.Fatalf("insert allocation fixture: %v", err)
		}
	}

	repo := NewQueryRepository(pool)
	assertItems := func(label string, items []Item) {
		t.Helper()
		if len(items) != len(values) {
			t.Fatalf("%s items = %d, want %d", label, len(items), len(values))
		}
		found := map[string]bool{}
		for _, item := range items {
			encoded, err := json.Marshal(item)
			if err != nil {
				t.Fatalf("%s marshal amount: %v", label, err)
			}
			if item.Amount == nil {
				if strings.Contains(string(encoded), "\"amount\"") {
					t.Fatalf("%s null amount should be omitted: %s", label, encoded)
				}
				found[""] = true
				continue
			}
			found[*item.Amount] = true
			if !strings.Contains(string(encoded), "\"amount\":\""+*item.Amount+"\"") {
				t.Fatalf("%s amount must serialize as exact string: %s", label, encoded)
			}
		}
		for _, v := range values {
			if !found[v.amount] {
				t.Fatalf("%s missing exact amount %q", label, v.amount)
			}
		}
	}
	byExecution, err := repo.ListByExecution(ctx, executionID)
	if err != nil {
		t.Fatalf("query by execution: %v", err)
	}
	assertItems("execution", byExecution)
	byAssessment, err := repo.ListByQualityAssessment(ctx, assessmentID)
	if err != nil {
		t.Fatalf("query by quality assessment: %v", err)
	}
	assertItems("quality assessment", byAssessment)
}
