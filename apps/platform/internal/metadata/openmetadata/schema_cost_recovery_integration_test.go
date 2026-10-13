package openmetadata_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/application"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	infra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	om "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/openmetadata"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/database"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/routing"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	rd "github.com/qq550723504/data-product-platform/apps/platform/internal/rights/domain"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func failSchemaCost(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ws uuid.UUID) func() {
	t.Helper()
	name := pgx.Identifier{"schema_cost_failure_" + strings.ReplaceAll(ws.String(), "-", "")}.Sanitize()
	_, err := pool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id='%s' AND NEW.cost_type='SOURCE_SCHEMA_SCAN_INVOCATION' THEN RAISE EXCEPTION 'injected cost database outage'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON cost_event FOR EACH ROW EXECUTE FUNCTION %s();`, name, ws, name, name))
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	drop := func() {
		if removed {
			return
		}
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON cost_event; DROP FUNCTION %s();`, name, name)); err != nil {
			t.Fatal(err)
		}
		removed = true
	}
	t.Cleanup(drop)
	return drop
}

func schemaCostCounts(t *testing.T, ctx context.Context, pool *pgxpool.Pool, request uuid.UUID) (int, int) {
	t.Helper()
	var costs, observations int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM cost_allocation c JOIN source_schema_attempt a ON a.id=c.source_schema_attempt_id WHERE a.request_id=$1`, request).Scan(&costs); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM source_schema_observation WHERE request_id=$1`, request).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	return costs, observations
}

type schemaTestEntitlements struct {
	expected d.SchemaPlan
	checks   int
}

func (c *schemaTestEntitlements) CheckCurrentEntitlementTx(_ context.Context, _ pgx.Tx, r rd.EntitlementRequest) (rd.EntitlementDecision, error) {
	c.checks++
	if r.WorkspaceID != c.expected.WorkspaceID || r.AuthorizationID != c.expected.AuthorizationID || r.DataResourceID != c.expected.ResourceID || r.ConsumerRef != c.expected.ConsumerRef || r.Purpose != c.expected.Purpose || r.Action != "READ" || r.Path != rd.EntitlementDownstream || r.Scope.Type != "ALL_RESOURCE" || r.Scope.Ref != c.expected.ResourceID.String() {
		return rd.EntitlementDecision{}, errors.New("entitlement context changed")
	}
	return rd.EntitlementDecision{Decision: "ALLOWED"}, nil
}

func nativeRecoveryFixture(t *testing.T) (context.Context, *pgxpool.Pool, d.SchemaRequest, any, *schemaTestEntitlements, *app.SchemaService) {
	t.Helper()
	router, err := routing.NewRouter(true)
	if err != nil {
		t.Fatal(err)
	}
	outbox.ConfigureAppendObligation(router)
	t.Cleanup(func() { outbox.ConfigureAppendObligation(nil) })
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	pool, err := database.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	p := d.SchemaPlan{Enabled: true, WorkspaceID: uuid.New(), ResourceID: uuid.New(), ActorID: uuid.New(), AuthorizationID: uuid.New(), ConsumerRef: "TEST", Purpose: "TEST", ExternalFQN: "test." + uuid.NewString(), RuleRef: "test/schema", RuleVersion: "1", RuleHash: strings.Repeat("a", 64), ConnectorHash: strings.Repeat("b", 64), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO data_resource(id,workspace_id,code,name,resource_type) VALUES($1,$2,$3,'profile recovery test','TABLE_LIKE')`, p.ResourceID, p.WorkspaceID, p.ResourceID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO data_authorization(id,workspace_id,code,grantor_ref,grantee_ref,purpose) VALUES($1,$2,$3,'TEST','TEST','TEST')`, p.AuthorizationID, p.WorkspaceID, p.AuthorizationID.String()); err != nil {
		t.Fatal(err)
	}
	repo := infra.NewPostgresRepository(pool)
	b, err := repo.UpsertBinding(ctx, tx, d.ResourceBinding{ResourceID: p.ResourceID, Provider: "TEST_METADATA", EntityType: "TABLE", ExternalID: uuid.NewString(), ExternalFQN: p.ExternalFQN, IsPrimary: true})
	if err != nil {
		t.Fatal(err)
	}
	req, _, err := repo.PrepareSchema(ctx, tx, d.SchemaRequest{ID: uuid.New(), Plan: p, PlanHash: d.Hash(p), BindingID: b.ID, ExternalID: b.ExternalID, Key: uuid.NewString()})
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var observer any
	gate := &schemaTestEntitlements{expected: p}
	s := &app.SchemaService{Service: app.NewService("TEST_METADATA", "", transaction.NewManager(pool), repo, nil, nil), Rights: gate}
	return ctx, pool, req, observer, gate, s
}

// Opt-in real Docker receipts + real PostgreSQL business recovery. The scanner
// speaks a controlled protocol on network=none, not native metadata discovery.
func TestSourceSchemaNativeReceiptCostRecovery(t *testing.T) {
	if os.Getenv("C2_SCHEMA_NATIVE_DOCKER") != "1" {
		t.Skip("C2_SCHEMA_NATIVE_DOCKER is not set")
	}
	for _, stage := range []string{"prepared before launch", "started before cost commit", "database charge outage"} {
		t.Run(stage, func(t *testing.T) {
			ctx, pool, req, _, _, s := nativeRecoveryFixture(t)
			marker, _ := json.Marshal(map[string]any{"invocationStarted": true, "connectorHash": req.Plan.ConnectorHash, "externalFqn": req.Plan.ExternalFQN})
			result, _ := json.Marshal(map[string]any{"externalId": req.ExternalID, "externalFqn": req.Plan.ExternalFQN, "connectorHash": req.Plan.ConnectorHash, "columns": req.Plan.Columns, "startedAt": "2026-10-12T00:00:00Z", "completedAt": "2026-10-12T00:00:01Z"})
			script := fmt.Sprintf("printf '%%s\\n' '%s'; read command; [ \"$command\" = RUN ] || exit 1; printf '%%s\\n' '%s'", marker, result)
			prefix := "dpp-c2-cost-recovery-" + uuid.NewString() + "-"
			cfg := map[string]any{"plan": req.Plan, "prepareCommand": []string{"docker", "create", "--name", prefix + "{attemptId}", "--label", "dpp.c2.cost-recovery.fixture=true", "--network", "none", "-i", "--entrypoint", "sh", "sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea", "-c", script}, "command": []string{"docker", "start", "--attach", "--interactive", "{runtimeId}"}, "stopCommand": []string{"docker", "stop", "--timeout", "1", "{runtimeId}"}, "invocationCommand": []string{"docker", "inspect", "--format", "{{json .State}}", "{runtimeId}"}}
			path := filepath.Join(t.TempDir(), "native.json")
			raw, _ := json.Marshal(cfg)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			native := &om.NativeSchemaObserver{Path: path}
			s.Observer = native
			if stage == "database charge outage" {
				restore := failSchemaCost(t, ctx, pool, req.Plan.WorkspaceID)
				if err := s.Process(ctx, req.ID); err == nil {
					t.Fatal("native cost failure closed")
				}
				if costs, observations := schemaCostCounts(t, ctx, pool, req.ID); costs != 0 || observations != 0 {
					t.Fatal("native cost failure lost pending state")
				}
				restore()
			} else {
				attempt := uuid.New()
				runtime, err := native.Prepare(ctx, attempt, req.Plan)
				if err != nil {
					t.Fatal(err)
				}
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				if err := infra.NewPostgresRepository(pool).BeginSchemaAttempt(ctx, tx, req, attempt, runtime); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				if started, err := native.Invocation(ctx, attempt, runtime); err != nil || started {
					t.Fatal("prepared runtime was already invoked", err)
				}
				if stage == "started before cost commit" {
					var c struct{ RuntimeID string }
					if err := json.Unmarshal(runtime, &c); err != nil {
						t.Fatal(err)
					}
					if output, err := exec.CommandContext(ctx, "docker", "start", c.RuntimeID).CombinedOutput(); err != nil {
						t.Fatalf("owned fixture start failed: %s %v", output, err)
					}
					if started, err := native.Invocation(ctx, attempt, runtime); err != nil || !started {
						t.Fatal("native start receipt absent", err)
					}
				}
			}
			if err := s.Process(ctx, req.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.Process(ctx, req.ID); err != nil {
				t.Fatal(err)
			}
			expected := 2
			if stage == "prepared before launch" {
				expected = 1
			}
			if costs, observations := schemaCostCounts(t, ctx, pool, req.ID); costs != expected || observations != 2 {
				t.Fatalf("native receipt cost mismatch: costs=%d observations=%d expected=%d", costs, observations, expected)
			}
			rows, err := pool.Query(ctx, `SELECT s.contract->>'runtimeId' FROM source_schema_attempt_runtime s JOIN source_schema_attempt a ON a.id=s.attempt_id WHERE a.request_id=$1`, req.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			for rows.Next() {
				var cid string
				if err := rows.Scan(&cid); err != nil {
					t.Fatal(err)
				}
				if output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", cid).CombinedOutput(); err != nil || strings.TrimSpace(string(output)) == "running" {
					t.Fatal("owned fixture not contained/retained", err)
				}
				t.Logf("retained native receipt runtime %s", cid)
			}
		})
	}
}
