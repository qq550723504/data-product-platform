package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBootstrapScopeRejectsDestructiveForeignAndImplicitRetry(t *testing.T) {
	var sent int
	tx := &bootstrapTransport{host: "127.0.0.1:9000", receipts: filepath.Join(t.TempDir(), "receipts.jsonl"), base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sent++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})}
	ctx := context.WithValue(context.Background(), stepKey{}, &step{id: uuid.New(), label: "test-metadata"})
	for _, shape := range []struct{ method, url string }{{"DELETE", "http://127.0.0.1:9000/fixture"}, {"GET", "http://foreign.invalid/fixture"}, {"GET", "https://127.0.0.1:9000/fixture"}} {
		r, _ := http.NewRequestWithContext(ctx, shape.method, shape.url, nil)
		if _, err := tx.RoundTrip(r); err == nil {
			t.Fatal("unsafe shape admitted")
		}
	}
	if sent != 0 {
		t.Fatal("rejected request sent")
	}
	r, _ := http.NewRequestWithContext(ctx, "GET", "http://127.0.0.1:9000/", nil)
	response, err := tx.RoundTrip(r)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, err = tx.RoundTrip(r); err == nil {
		t.Fatal("implicit retry admitted")
	}
	if sent != 1 {
		t.Fatalf("physical calls=%d", sent)
	}
	b, err := os.ReadFile(tx.receipts)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSpace(string(b)), "\n")) != 2 {
		t.Fatal("start/observation not retained exactly once")
	}
}

func writeConfig(t *testing.T, path string, c config) {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
}
func fixtureConfig(t *testing.T) config {
	t.Helper()
	return config{RunID: uuid.New(), WorkspaceID: uuid.New(), ProviderInstanceID: uuid.New(), IncarnationID: uuid.New(), ActorID: uuid.New(), Enabled: true,
		Endpoint: "127.0.0.1:9000", PostgresDSN: "not-connected-in-this-test", RootAccess: "synthetic", RootSecret: "synthetic", InspectAccess: "synthetic-inspect", InspectSecret: "synthetic", Token: "private-test-token",
		OutputDirectory: filepath.Join(t.TempDir(), "output"), Environment: json.RawMessage(`{}`)}
}

func TestCurrentAuthorityRevocationAndNoDestructiveCapabilities(t *testing.T) {
	c := fixtureConfig(t)
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, c)
	a := authority{path: path, token: c.Token, run: c.RunID}
	if _, err := a.resolve(c.WorkspaceID, lc.CapabilityInspectStorage); err != nil {
		t.Fatal(err)
	}
	for _, cap := range []string{"LIFECYCLE_APPROVE_PLAN", "LIFECYCLE_DISPATCH", "LIFECYCLE_ADMIT_READER"} {
		if _, err := a.resolve(c.WorkspaceID, cap); err == nil {
			t.Fatal("unauthorized capability", cap)
		}
	}
	if _, err := a.resolve(uuid.New(), lc.CapabilityInspectStorage); err == nil {
		t.Fatal("foreign workspace admitted")
	}
	c.Enabled = false
	writeConfig(t, path, c)
	if _, err := a.resolve(c.WorkspaceID, lc.CapabilityInspectStorage); err == nil {
		t.Fatal("revoked authority admitted")
	}
}

func TestPreparationRefusesConfiguredOutputWithoutIO(t *testing.T) {
	c := fixtureConfig(t)
	if err := os.Mkdir(c.OutputDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, c)
	if err := prepare(context.Background(), path); err == nil || !strings.Contains(err.Error(), "one-shot") {
		t.Fatal("preparation replay not refused before database/provider I/O", err)
	}
	t.Setenv("LIFECYCLE_FIXTURE_RESUME_EMPTY", "1")
	if err := os.WriteFile(filepath.Join(c.OutputDirectory, "manifest.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepare(context.Background(), path); err == nil || !strings.Contains(err.Error(), "nonempty") {
		t.Fatal("recovery would re-create a populated fixture", err)
	}
}

func TestDeleteCandidateRejectsProtectedTargetsAndReplayBeforeIO(t *testing.T) {
	c := fixtureConfig(t)
	if err := os.Mkdir(c.OutputDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	objects := []object{
		{CopyID: uuid.New(), Namespace: namespaces[0], Key: "fixture/a.csv", Version: "synthetic-native-id", Retention: "NONE", LegalHold: true},
		{CopyID: uuid.New(), Namespace: namespaces[1], Key: "fixture/b.csv", Version: "synthetic-native-id", Retention: "NONE"},
		{CopyID: uuid.New(), Namespace: namespaces[0], Key: "fixture/b.csv", Version: "synthetic-native-id", Retention: "NONE", LegalHold: true},
		{CopyID: uuid.New(), Namespace: namespaces[1], Key: "fixture/a.csv", Version: "synthetic-native-id", Retention: "GOVERNANCE"},
	}
	manifest := map[string]any{"schema": "lifecycle307-predelete-manifest-v1", "run_id": c.RunID, "workspace_id": c.WorkspaceID, "provider_instance_id": c.ProviderInstanceID, "incarnation_id": c.IncarnationID, "objects": objects}
	b, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(c.OutputDirectory, "manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if err = prepareUnprotectedPlan(context.Background(), "not-read-before-rejection", c); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatal("protected target admitted", err)
	}
	if err = os.WriteFile(filepath.Join(c.OutputDirectory, "deletion-candidate.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = prepareUnprotectedPlan(context.Background(), "not-read-before-rejection", c); err == nil || !strings.Contains(err.Error(), "already") {
		t.Fatal("candidate replay admitted", err)
	}
}
