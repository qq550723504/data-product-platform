package application

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

// ProcessingRequest contains only Core platform identifiers and frozen business inputs.
// Engine-specific configuration belongs to the adapter boundary, not to Core domain state.
type ProcessingRequest struct {
	ExecutionID     uuid.UUID
	WorkspaceID     uuid.UUID
	WorkflowVersion domain.WorkflowVersion
	Inputs          []domain.InputBinding
	OutputDatasetID uuid.UUID
	TargetPeriod    string
}

// ProcessingResult maps an adapter result back into Core identifiers/metrics.
// EngineExecutionID is an external reference and is never the Core execution identity.
type ProcessingResult struct {
	OutputDatasetVersionID uuid.UUID
	EngineExecutionID      string
	Metrics                map[string]any
}

// ProcessingEngine is the synchronous execution SPI used by the current native engine.
// Remote engines can be bridged to this contract by an orchestrator/reconciler without
// changing Workflow, Dataset, ProductVersion or ProductRelease domain models.
type ProcessingEngine interface {
	Execute(ctx context.Context, request ProcessingRequest) (ProcessingResult, error)
}

type EngineRunState string

const (
	EngineRunQueued    EngineRunState = "QUEUED"
	EngineRunRunning   EngineRunState = "RUNNING"
	EngineRunSucceeded EngineRunState = "SUCCEEDED"
	EngineRunFailed    EngineRunState = "FAILED"
	EngineRunCancelled EngineRunState = "CANCELLED"
	EngineRunUnknown   EngineRunState = "UNKNOWN"
)

// ManagedSubmitRequest is an opaque provider-neutral remote execution request.
// Definition contains the engine-specific immutable execution package; Core never stores
// or interprets its format. DefinitionRef is a stable human-readable reference for audit.
type ManagedSubmitRequest struct {
	Name          string
	DefinitionRef string
	Definition    []byte
	ContentType   string
	Parameters    map[string]string
}

type EngineRun struct {
	ID           string
	Name         string
	State        EngineRunState
	StartedAt    *time.Time
	FinishedAt   *time.Time
	ErrorMessage string
	Metrics      map[string]any
}

type EngineLogPage struct {
	Text       string
	From       int
	NextOffset int
}

// ManagedProcessingEngine models the lifecycle exposed by remote runtimes such as
// Apache Hop Server. The external run ID remains separate from Core Execution.ID.
type ManagedProcessingEngine interface {
	// PrepareSubmission allocates a durable remote run identity without starting work.
	// Core must persist the returned ID before StartSubmission is invoked.
	PrepareSubmission(ctx context.Context, request ManagedSubmitRequest) (EngineRun, error)
	StartSubmission(ctx context.Context, request ManagedSubmitRequest, runID string) (EngineRun, error)

	// Submit remains a convenience composition for direct adapter callers. Core's
	// queue handler uses the two-phase methods above to close the crash window.
	Submit(ctx context.Context, request ManagedSubmitRequest) (EngineRun, error)
	Status(ctx context.Context, name, runID string) (EngineRun, error)
	Cancel(ctx context.Context, name, runID string) error
	Logs(ctx context.Context, name, runID string, from int) (EngineLogPage, error)
	Metrics(ctx context.Context, name, runID string) (map[string]any, error)
}
