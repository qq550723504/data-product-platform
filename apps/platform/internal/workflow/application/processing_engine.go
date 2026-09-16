package application

import (
	"context"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

// ProcessingRequest contains only Core platform identifiers and frozen business inputs.
// Engine-specific configuration belongs to the adapter boundary, not to Core domain state.
type ProcessingRequest struct {
	ExecutionID       uuid.UUID
	WorkflowVersion   domain.WorkflowVersion
	Inputs            []domain.InputBinding
	OutputDatasetID   uuid.UUID
	TargetPeriod      string
}

// ProcessingResult maps an adapter result back into Core identifiers/metrics.
// EngineExecutionID is an external reference and is never the Core execution identity.
type ProcessingResult struct {
	OutputDatasetVersionID uuid.UUID
	EngineExecutionID      string
	Metrics                map[string]any
}

// ProcessingEngine is the replaceable execution SPI. The initial adapter is native;
// Apache Hop and other engines can implement this interface in later sprints.
type ProcessingEngine interface {
	Execute(ctx context.Context, request ProcessingRequest) (ProcessingResult, error)
}
