package openmetadata

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	d "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
)

// NativeSchemaObserver is a thin launcher for the unmodified ingestion package.
// Private configuration uses credential file references. Frozen runtime data is
// opaque to Core; retained native start receipts make invocation costs recoverable.
type NativeSchemaObserver struct{ Path string }
type schemaConfig struct {
	Plan              d.SchemaPlan `json:"plan"`
	Command           []string     `json:"command"`
	StopCommand       []string     `json:"stopCommand"`
	PrepareCommand    []string     `json:"prepareCommand"`
	InvocationCommand []string     `json:"invocationCommand"`
	RuntimeID         string       `json:"runtimeId"`
}

func (a *NativeSchemaObserver) load() (schemaConfig, error) {
	var c schemaConfig
	b, err := os.ReadFile(a.Path)
	if err != nil {
		return c, errors.New("schema profile unavailable")
	}
	if json.Unmarshal(b, &c) != nil || !c.validCommands() {
		return c, errors.New("invalid schema profile")
	}
	return c, nil
}
func (c schemaConfig) validCommands() bool {
	return len(c.Command) > 0 && len(c.StopCommand) > 0 && len(c.PrepareCommand) > 0 && len(c.InvocationCommand) > 0
}
func (a *NativeSchemaObserver) Plan(context.Context) (d.SchemaPlan, error) {
	c, err := a.load()
	return c.Plan, err
}
func (a *NativeSchemaObserver) Prepare(ctx context.Context, id uuid.UUID, expected d.SchemaPlan) (json.RawMessage, error) {
	c, err := a.load()
	if err != nil {
		return nil, err
	}
	if c.Plan.Validate() != nil || d.Hash(c.Plan) != d.Hash(expected) {
		return nil, errors.New("schema runtime contract does not match frozen plan")
	}
	// Launcher argv uses private credential file references, never credential
	// values. Core treats this bounded Adapter extension as opaque immutable data.
	b, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	if _, err = d.RuntimeHash(b); err != nil {
		return nil, err
	}
	// Allocate a retained, unstarted native runtime. Creation performs no source
	// I/O; its durable CREATED vs STARTED receipt resolves a crash before launch.
	args := argsFor(c.PrepareCommand, id)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	if err != nil {
		return nil, errors.New("schema runtime preparation unavailable")
	}
	c.RuntimeID = strings.TrimSpace(string(output))
	if len(c.RuntimeID) != 64 || strings.IndexFunc(c.RuntimeID, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }) >= 0 {
		return nil, errors.New("schema runtime identity unavailable")
	}
	for _, args := range [][]string{c.Command, c.StopCommand, c.InvocationCommand} {
		for i, v := range args {
			args[i] = strings.ReplaceAll(v, "{runtimeId}", c.RuntimeID)
		}
	}
	b, err = json.Marshal(c)
	if err != nil {
		return nil, err
	}
	if _, err = d.RuntimeHash(b); err != nil {
		return nil, err
	}
	return b, nil
}
func frozenSchemaConfig(runtime json.RawMessage) (schemaConfig, error) {
	var c schemaConfig
	if _, err := d.RuntimeHash(runtime); err != nil {
		return c, err
	}
	if json.Unmarshal(runtime, &c) != nil || !c.validCommands() {
		return c, errors.New("schema runtime contract unavailable")
	}
	return c, nil
}
func (a *NativeSchemaObserver) Invocation(ctx context.Context, id uuid.UUID, runtime json.RawMessage) (bool, error) {
	c, err := frozenSchemaConfig(runtime)
	if err != nil {
		return false, metadataengine.ErrSourceSchemaInvocationUnknown
	}
	args := argsFor(c.InvocationCommand, id)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, args[0], args[1:]...).Output()
	if err != nil || len(output) > 8192 {
		return false, metadataengine.ErrSourceSchemaInvocationUnknown
	}
	var state struct {
		Status    string
		StartedAt time.Time
	}
	if json.Unmarshal(output, &state) != nil || state.Status == "" {
		return false, metadataengine.ErrSourceSchemaInvocationUnknown
	}
	if !state.StartedAt.IsZero() {
		return true, nil
	}
	if state.Status == "created" {
		return false, nil
	}
	return false, metadataengine.ErrSourceSchemaInvocationUnknown
}
func argsFor(args []string, id uuid.UUID) []string {
	out := make([]string, len(args))
	for i, v := range args {
		out[i] = strings.ReplaceAll(v, "{attemptId}", id.String())
	}
	return out
}
func (a *NativeSchemaObserver) Stop(ctx context.Context, id uuid.UUID, runtime json.RawMessage) error {
	// Only server-owned commands and a durable platform attempt identity. No
	// request-derived executable, path, image, shell command, or credentials.
	c, err := frozenSchemaConfig(runtime)
	if err != nil {
		return errors.New("schema process containment unavailable")
	}
	args := argsFor(c.StopCommand, id)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, args[0], args[1:]...).CombinedOutput()
	if err != nil && !strings.Contains(string(output), "No such container") {
		return errors.New("schema process containment unconfirmed")
	}
	return nil
}
func (a *NativeSchemaObserver) Scan(ctx context.Context, id uuid.UUID, runtime json.RawMessage, onInvocation, authorizeSourceIO func() error) (metadataengine.SourceSchema, error) {
	var result metadataengine.SourceSchema
	c, err := frozenSchemaConfig(runtime)
	if err != nil {
		return result, err
	}
	if err := c.Plan.Validate(); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	args := argsFor(c.Command, id)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	defer a.Stop(context.WithoutCancel(ctx), id, runtime)
	// Never publish native driver diagnostics, command arguments, or secret paths.
	output, err := cmd.StdoutPipe()
	if err != nil {
		return result, errors.New("NATIVE_SOURCE_SCAN_UNAVAILABLE")
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		return result, errors.New("NATIVE_SOURCE_SCAN_UNAVAILABLE")
	}
	if err := cmd.Start(); err != nil {
		return result, errors.New("NATIVE_SOURCE_SCAN_UNAVAILABLE")
	}
	// Confirm native STARTED independently from protocol output. The runtime was
	// allocated before the attempt committed, and is retained after exit/crash.
	for {
		started, err := a.Invocation(ctx, id, runtime)
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return result, metadataengine.ErrSourceSchemaInvocationUnknown
		}
		if started {
			break
		}
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return result, metadataengine.ErrSourceSchemaInvocationUnknown
		case <-time.After(25 * time.Millisecond):
		}
	}
	// Record native startup before parsing the marker; RUN remains gated.
	if err := onInvocation(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return result, metadataengine.ErrSourceSchemaCostUnconfirmed
	}
	decoder := json.NewDecoder(io.LimitReader(output, 128*1024))
	var marker struct {
		Started       bool   `json:"invocationStarted"`
		ConnectorHash string `json:"connectorHash"`
		ExternalFQN   string `json:"externalFqn"`
	}
	if decoder.Decode(&marker) != nil || !marker.Started || marker.ConnectorHash != c.Plan.ConnectorHash || marker.ExternalFQN != c.Plan.ExternalFQN {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return result, errors.New("NATIVE_SOURCE_SCAN_UNAVAILABLE")
	}
	if err := authorizeSourceIO(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return result, errors.New("NATIVE_SOURCE_SCAN_AUTHORITY_UNCONFIRMED")
	}
	if _, err := io.WriteString(input, "RUN\n"); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return result, errors.New("NATIVE_SOURCE_SCAN_UNAVAILABLE")
	}
	_ = input.Close()
	decodeErr := decoder.Decode(&result)
	if err := cmd.Wait(); err != nil {
		return result, errors.New("NATIVE_SOURCE_SCAN_UNAVAILABLE")
	}
	if decodeErr != nil || result.ExternalID == "" || result.ExternalFQN != c.Plan.ExternalFQN || result.ConnectorHash != c.Plan.ConnectorHash || result.StartedAt.IsZero() || result.CompletedAt.Before(result.StartedAt) || d.ValidateColumns(result.Columns) != nil {
		return metadataengine.SourceSchema{}, errors.New("NATIVE_SOURCE_SCAN_INVALID")
	}
	return result, nil
}
