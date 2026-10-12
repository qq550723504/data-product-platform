package workflow

import (
	"context"

	lifecycle "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	workflow "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
)

// Decorates the existing synchronous engine. Queue claim, output production,
// immutable dependencies and execution transitions retain their original owner.
type PinnedEngine struct {
	Pins  *lifecycle.FixtureSubjectPins
	Inner workflow.ProcessingEngine
}

func (e PinnedEngine) Execute(ctx context.Context, r workflow.ProcessingRequest) (workflow.ProcessingResult, error) {
	var result workflow.ProcessingResult
	if e.Pins == nil || e.Inner == nil {
		return result, lifecycle.ErrStorageUnsupported
	}
	err := e.Pins.ExecuteProcessing(ctx, r.WorkspaceID, r.ExecutionID, func(ctx context.Context) error { var err error; result, err = e.Inner.Execute(ctx, r); return err })
	return result, err
}
