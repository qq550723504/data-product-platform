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
// Its private server-owned config contains the command and credential mounts;
// only Plan and bounded structural observations cross the port.
type NativeSchemaObserver struct{ Path string }
type schemaConfig struct {
	Plan        d.SchemaPlan `json:"plan"`
	Command     []string     `json:"command"`
	StopCommand []string     `json:"stopCommand"`
}

func (a *NativeSchemaObserver) load() (schemaConfig, error) {
	var c schemaConfig
	b, err := os.ReadFile(a.Path)
	if err != nil {
		return c, errors.New("schema profile unavailable")
	}
	if json.Unmarshal(b, &c) != nil || len(c.Command) == 0 || len(c.StopCommand) == 0 {
		return c, errors.New("invalid schema profile")
	}
	return c, nil
}
func (a *NativeSchemaObserver) Plan(context.Context) (d.SchemaPlan, error) {
	c, err := a.load()
	return c.Plan, err
}
func argsFor(args []string, id uuid.UUID) []string {
	out := make([]string, len(args))
	for i, v := range args {
		out[i] = strings.ReplaceAll(v, "{attemptId}", id.String())
	}
	return out
}
func (a *NativeSchemaObserver) Stop(ctx context.Context, id uuid.UUID) error {
	// Only server-owned commands and a durable platform attempt identity. No
	// request-derived executable, path, image, shell command, or credentials.
	var c schemaConfig
	b, err := os.ReadFile(a.Path)
	if err != nil || json.Unmarshal(b, &c) != nil || len(c.StopCommand) == 0 {
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
func (a *NativeSchemaObserver) Scan(ctx context.Context, id uuid.UUID, onInvocation func() error) (metadataengine.SourceSchema, error) {
	var result metadataengine.SourceSchema
	c, err := a.load()
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
	defer a.Stop(context.WithoutCancel(ctx), id)
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
	if err := onInvocation(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return result, errors.New("NATIVE_SOURCE_SCAN_COST_UNCONFIRMED")
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
