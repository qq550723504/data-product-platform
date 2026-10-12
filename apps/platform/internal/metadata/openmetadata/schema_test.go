package openmetadata

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	me "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeSchemaProcessHelper(t *testing.T) {
	if os.Getenv("C2_SCHEMA_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "prepare" {
			fmt.Println(strings.Repeat("a", 64))
			os.Exit(0)
		}
		if arg == "invocation" {
			fmt.Println(`{"Status":"exited","StartedAt":"2026-10-12T00:00:00Z"}`)
			os.Exit(0)
		}
		if arg == "stop" {
			if i+1 < len(os.Args) {
				if err := os.WriteFile(os.Args[i+1], []byte("contained"), 0600); err != nil {
					os.Exit(1)
				}
			}
			os.Exit(0)
		}
	}
	switch os.Getenv("C2_SCHEMA_PREMARKER") {
	case "exit":
		os.Exit(1)
	case "hang":
		time.Sleep(10 * time.Second)
	case "malformed":
		fmt.Println("not a protocol marker")
		os.Exit(1)
	}
	fmt.Println(os.Getenv("C2_SCHEMA_MARKER"))
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() || scanner.Text() != "RUN" {
		os.Exit(1)
	}
	if path := os.Getenv("C2_SCHEMA_RUN_RECEIPT"); path != "" {
		if os.WriteFile(path, []byte("source-started"), 0600) != nil {
			os.Exit(1)
		}
	}
	if os.Getenv("C2_SCHEMA_ERROR") == "1" {
		fmt.Fprintln(os.Stderr, "private-source-password")
		os.Exit(1)
	}
	fmt.Println(os.Getenv("C2_SCHEMA_RESULT"))
	os.Exit(0)
}

func TestNativeSchemaCostsStartupBeforeMarker(t *testing.T) {
	t.Setenv("C2_SCHEMA_HELPER", "1")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	plan := d.SchemaPlan{Enabled: true, WorkspaceID: uuid.New(), ResourceID: uuid.New(), ActorID: uuid.New(), AuthorizationID: uuid.New(), ConsumerRef: "consumer", Purpose: "purpose", ExternalFQN: "source.db.schema.energy", RuleRef: "rule", RuleVersion: "1", RuleHash: strings.Repeat("a", 64), ConnectorHash: strings.Repeat("b", 64), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	cfg := schemaConfig{Plan: plan, Command: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "scan"}, StopCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "stop"}, PrepareCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "prepare"}, InvocationCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "invocation"}}
	runtime, _ := json.Marshal(cfg)
	observer := &NativeSchemaObserver{}
	for _, failure := range []string{"exit", "hang", "malformed"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("C2_SCHEMA_PREMARKER", failure)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			costs, gates := 0, 0
			_, err := observer.Scan(ctx, uuid.New(), runtime, func() error { costs++; return nil }, func(run func() error) error { gates++; return run() })
			if err == nil || costs != 1 || gates != 0 {
				t.Fatalf("startup cost/source gate wrong: %s costs=%d gates=%d err=%v", failure, costs, gates, err)
			}
		})
	}
	cfg.Command = []string{filepath.Join(t.TempDir(), "missing-executable")}
	runtime, _ = json.Marshal(cfg)
	if _, err := observer.Scan(context.Background(), uuid.New(), runtime, func() error { t.Fatal("unstarted process charged"); return nil }, func(func() error) error { t.Fatal("unstarted process reached source gate"); return nil }); err == nil {
		t.Fatal("unstarted process accepted")
	}
}

func TestNativeSchemaFrozenRuntimeSurvivesProfileRotation(t *testing.T) {
	t.Setenv("C2_SCHEMA_HELPER", "1")
	plan := d.SchemaPlan{Enabled: true, WorkspaceID: uuid.New(), ResourceID: uuid.New(), ActorID: uuid.New(), AuthorizationID: uuid.New(), ConsumerRef: "consumer", Purpose: "purpose", ExternalFQN: "source.db.schema.energy", RuleRef: "rule", RuleVersion: "1", RuleHash: strings.Repeat("a", 64), ConnectorHash: strings.Repeat("b", 64), Columns: []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}}
	root := t.TempDir()
	path := filepath.Join(root, "profile.json")
	original := filepath.Join(root, "original-runtime")
	replacement := filepath.Join(root, "replacement-runtime")
	cfg := schemaConfig{Plan: plan, Command: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "scan"}, StopCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "stop", original}, PrepareCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "prepare"}, InvocationCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "invocation"}}
	write := func(c schemaConfig) {
		b, _ := json.Marshal(c)
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(cfg)
	a := &NativeSchemaObserver{Path: path}
	id := uuid.New()
	runtime, err := a.Prepare(context.Background(), id, plan)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := d.RuntimeHash(runtime)
	if err != nil {
		t.Fatal(err)
	}
	cfg.StopCommand[len(cfg.StopCommand)-1] = replacement
	cfg.Command = []string{"invalid-replacement-launcher"}
	write(cfg)
	// The DB JSONB round trip can reorder object keys; the canonical identity holds.
	var obj map[string]any
	if err := json.Unmarshal(runtime, &obj); err != nil {
		t.Fatal(err)
	}
	saved, _ := json.Marshal(obj)
	again, _ := d.RuntimeHash(saved)
	if hash != again {
		t.Fatal("runtime identity changed through JSONB ordering")
	}
	if err := a.Stop(context.Background(), id, saved); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatal("original containment command not used")
	}
	if _, err := os.Stat(replacement); !os.IsNotExist(err) {
		t.Fatal("rotated containment command used")
	}
	now := time.Now().UTC()
	result := me.SourceSchema{ExternalID: "native-table", ExternalFQN: plan.ExternalFQN, ConnectorHash: plan.ConnectorHash, Columns: plan.Columns, StartedAt: now, CompletedAt: now.Add(time.Second)}
	marker, _ := json.Marshal(map[string]any{"invocationStarted": true, "connectorHash": plan.ConnectorHash, "externalFqn": plan.ExternalFQN})
	body, _ := json.Marshal(result)
	t.Setenv("C2_SCHEMA_MARKER", string(marker))
	t.Setenv("C2_SCHEMA_RESULT", string(body))
	if _, err := a.Scan(context.Background(), id, saved, func() error { return nil }, func(run func() error) error { return run() }); err != nil {
		t.Fatal("rotated launcher replaced frozen attempt:", err)
	}
	if err := os.WriteFile(path, []byte("unreadable-current-contract"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.Stop(context.Background(), id, saved); err != nil {
		t.Fatal("current profile failure hid frozen containment:", err)
	}
}
func TestNativeSchemaRequiresDurableInvocationAcknowledgement(t *testing.T) {
	t.Setenv("C2_SCHEMA_HELPER", "1")
	columns := []d.SchemaColumn{{Name: "energy_kwh", Type: "TEXT", Constraint: "NOT_NULL"}}
	plan := d.SchemaPlan{Enabled: true, WorkspaceID: uuid.New(), ResourceID: uuid.New(), ActorID: uuid.New(), AuthorizationID: uuid.New(), ConsumerRef: "consumer", Purpose: "purpose", ExternalFQN: "source.db.schema.energy", RuleRef: "schema-v1", RuleVersion: "1", RuleHash: strings.Repeat("a", 64), ConnectorHash: strings.Repeat("b", 64), Columns: columns}
	now := time.Now().UTC()
	result := me.SourceSchema{ExternalID: "native-table", ConnectorHash: plan.ConnectorHash, ExternalFQN: plan.ExternalFQN, Columns: columns, StartedAt: now, CompletedAt: now.Add(time.Second)}
	marker, _ := json.Marshal(map[string]any{"invocationStarted": true, "connectorHash": plan.ConnectorHash, "externalFqn": plan.ExternalFQN})
	t.Setenv("C2_SCHEMA_MARKER", string(marker))
	body, _ := json.Marshal(result)
	t.Setenv("C2_SCHEMA_RESULT", string(body))
	path := filepath.Join(t.TempDir(), "private-config.json")
	config := schemaConfig{Plan: plan, Command: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "scan"}, StopCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "stop"}, PrepareCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "prepare"}, InvocationCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "invocation"}}
	body, _ = json.Marshal(config)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	observer := &NativeSchemaObserver{Path: path}
	scan := func(started func() error, gate me.SourceSchemaRunGate) (me.SourceSchema, error) {
		id := uuid.New()
		runtime, err := observer.Prepare(context.Background(), id, plan)
		if err != nil {
			return me.SourceSchema{}, err
		}
		return observer.Scan(context.Background(), id, runtime, started, gate)
	}
	frozen, err := observer.Prepare(context.Background(), uuid.New(), plan)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	receipt := filepath.Join(t.TempDir(), "native-run-receipt")
	t.Setenv("C2_SCHEMA_RUN_RECEIPT", receipt)
	actual, err := scan(func() error { calls++; return nil }, func(run func() error) error {
		if err := run(); err != nil {
			return err
		}
		// The real child receives RUN before this gate returns. Waiting for this
		// bounded test receipt does not move full discovery into the production gate.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(receipt); err == nil {
				return nil
			}
			time.Sleep(10 * time.Millisecond)
		}
		return errors.New("RUN was not handed off inside the gate")
	})
	if err != nil || actual.ExternalID != result.ExternalID || calls != 1 {
		t.Fatalf("native protocol failed: calls=%d err=%v", calls, err)
	}
	if _, err := scan(func() error { return errors.New("cost commit unavailable") }, func(func() error) error { t.Fatal("unconfirmed cost reached source gate"); return nil }); err == nil {
		t.Fatal("unconfirmed invocation ran")
	}
	if _, err := scan(func() error { return nil }, func(func() error) error { return errors.New("source authority revoked during startup") }); err == nil || err.Error() != "NATIVE_SOURCE_SCAN_AUTHORITY_UNCONFIRMED" {
		t.Fatal("startup cost replaced fresh source gate", err)
	}
	t.Setenv("C2_SCHEMA_MARKER", `{"invocationStarted":true,"connectorHash":"different-source"}`)
	if _, err := scan(func() error { return nil }, func(func() error) error { t.Fatal("different source passed pre-IO handshake"); return nil }); err == nil {
		t.Fatal("unbound source accepted")
	}
	t.Setenv("C2_SCHEMA_MARKER", string(marker))
	t.Setenv("C2_SCHEMA_ERROR", "1")
	if _, err := scan(func() error { return nil }, func(run func() error) error { return run() }); err == nil || strings.Contains(err.Error(), "private-source-password") {
		t.Fatal("provider failure/secret boundary lost")
	}
	config.Plan.Enabled = false
	body, _ = json.Marshal(config)
	_ = os.WriteFile(path, body, 0600)
	if current, err := observer.Plan(context.Background()); err != nil || current.Enabled {
		t.Fatal("disabled plan must be distinguishable from an unreadable profile")
	}
	if _, err := scan(func() error { t.Fatal("disabled profile launched native work"); return nil }, func(run func() error) error { return run() }); err == nil {
		t.Fatal("disabled plan launched a scan")
	}
	if err := observer.Stop(context.Background(), uuid.New(), frozen); err != nil {
		t.Fatal("disabled authority blocked process containment")
	}
	if err := observer.Stop(context.Background(), uuid.New(), nil); err == nil {
		t.Fatal("missing frozen containment contract allowed")
	}
}
