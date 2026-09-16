package workflowqueue

import (
	"context"
	"fmt"

	"github.com/hibiken/asynq"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type Handler struct {
	service *workflowapp.ExecutionService
	repo    *workflowinfra.PostgresRepository
	engine  workflowapp.ProcessingEngine
}

func NewHandler(service *workflowapp.ExecutionService, repo *workflowinfra.PostgresRepository, engine workflowapp.ProcessingEngine) *Handler {
	return &Handler{service: service, repo: repo, engine: engine}
}

func (h *Handler) Handle(ctx context.Context, task *asynq.Task) error {
	executionID, err := ExecutionID(task)
	if err != nil {
		return fmt.Errorf("parse workflow execution task: %w", err)
	}
	execution, err := h.repo.GetExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("load execution %s: %w", executionID, err)
	}

	// Delivery retries must never create duplicate output for the same Core Execution.
	// Operational retries are explicit and create a new Execution through ExecutionService.Retry.
	switch execution.Status {
	case domain.ExecutionSucceeded, domain.ExecutionFailed, domain.ExecutionCancelled:
		return nil
	case domain.ExecutionRunning:
		return nil
	case domain.ExecutionQueued:
	default:
		return fmt.Errorf("execution %s has unsupported status %s", execution.ID, execution.Status)
	}

	workflowVersion, err := h.repo.GetVersion(ctx, execution.WorkflowVersionID)
	if err != nil {
		return fmt.Errorf("load workflow version %s: %w", execution.WorkflowVersionID, err)
	}
	engineExecutionID := "native:" + execution.ID.String()
	started, err := h.service.Start(ctx, execution.ID, engineExecutionID, execution.ID.String())
	if err != nil {
		return fmt.Errorf("start execution %s: %w", execution.ID, err)
	}

	result, err := h.engine.Execute(ctx, workflowapp.ProcessingRequest{
		ExecutionID:     started.ID,
		WorkflowVersion: workflowVersion,
		Inputs:          started.Inputs,
		OutputDatasetID: started.OutputDatasetID,
		TargetPeriod:    started.TargetPeriod,
	})
	if err != nil {
		if _, failErr := h.service.Fail(ctx, started.ID, "PROCESSING_FAILED", err.Error(), map[string]any{
			"engineType": started.EngineType,
		}, started.ID.String()); failErr != nil {
			return fmt.Errorf("processing failed: %v; persist failure: %w", err, failErr)
		}
		// The Core execution is now terminal FAILED. Returning nil prevents the queue
		// transport from replaying the same execution and accidentally duplicating output.
		return nil
	}
	if result.EngineExecutionID != "" && result.EngineExecutionID != engineExecutionID {
		result.Metrics["adapterExecutionId"] = result.EngineExecutionID
	}
	if _, err := h.service.Succeed(ctx, started.ID, result.OutputDatasetVersionID, result.Metrics, started.ID.String()); err != nil {
		return fmt.Errorf("complete execution %s: %w", started.ID, err)
	}
	return nil
}
