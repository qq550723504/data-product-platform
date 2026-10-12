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
	if os.Args[len(os.Args)-1] == "stop" {
		os.Exit(0)
	}
	fmt.Println(os.Getenv("C2_SCHEMA_MARKER"))
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() || scanner.Text() != "RUN" {
		os.Exit(1)
	}
	if os.Getenv("C2_SCHEMA_ERROR") == "1" {
		fmt.Fprintln(os.Stderr, "private-source-password")
		os.Exit(1)
	}
	fmt.Println(os.Getenv("C2_SCHEMA_RESULT"))
	os.Exit(0)
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
	config := schemaConfig{Plan: plan, Command: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "scan"}, StopCommand: []string{os.Args[0], "-test.run=TestNativeSchemaProcessHelper", "--", "stop"}}
	body, _ = json.Marshal(config)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	observer := &NativeSchemaObserver{Path: path}
	calls := 0
	actual, err := observer.Scan(context.Background(), uuid.New(), func() error { calls++; return nil })
	if err != nil || actual.ExternalID != result.ExternalID || calls != 1 {
		t.Fatalf("native protocol failed: calls=%d err=%v", calls, err)
	}
	if _, err := observer.Scan(context.Background(), uuid.New(), func() error { return errors.New("cost commit unavailable") }); err == nil {
		t.Fatal("unconfirmed invocation ran")
	}
	t.Setenv("C2_SCHEMA_MARKER", `{"invocationStarted":true,"connectorHash":"different-source"}`)
	if _, err := observer.Scan(context.Background(), uuid.New(), func() error { t.Fatal("different source passed pre-IO handshake"); return nil }); err == nil {
		t.Fatal("unbound source accepted")
	}
	t.Setenv("C2_SCHEMA_MARKER", string(marker))
	t.Setenv("C2_SCHEMA_ERROR", "1")
	if _, err := observer.Scan(context.Background(), uuid.New(), func() error { return nil }); err == nil || strings.Contains(err.Error(), "private-source-password") {
		t.Fatal("provider failure/secret boundary lost")
	}
	config.Plan.Enabled = false
	body, _ = json.Marshal(config)
	_ = os.WriteFile(path, body, 0600)
	if current, err := observer.Plan(context.Background()); err != nil || current.Enabled {
		t.Fatal("disabled plan must be distinguishable from an unreadable profile")
	}
	if _, err := observer.Scan(context.Background(), uuid.New(), func() error { t.Fatal("disabled profile launched native work"); return nil }); err == nil {
		t.Fatal("disabled plan launched a scan")
	}
	if err := observer.Stop(context.Background(), uuid.New()); err != nil {
		t.Fatal("disabled authority blocked process containment")
	}
}
