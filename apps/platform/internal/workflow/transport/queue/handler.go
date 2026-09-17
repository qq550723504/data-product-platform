package workflowqueue

import (
	"context"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type Handler struct {
	service *workflowapp.ExecutionService
	repo    *workflowinfra.PostgresRepository
	native  workflowapp.ProcessingEngine
	managed map[string]workflowapp.ManagedExecutionBridge
}

func NewHandler(service *workflowapp.ExecutionService, repo *workflowinfra.PostgresRepository, native workflowapp.ProcessingEngine, managed ...workflowapp.ManagedExecutionBridge) *Handler {
	registry := make(map[string]workflowapp.ManagedExecutionBridge, len(managed))
	for _, bridge := range managed {
		if bridge == nil {
			continue
		}
		engineType := strings.ToUpper(strings.TrimSpace(bridge.EngineType()))
		if engineType != "" {
			registry[engineType] = bridge
		}
	}
	return &Handler{service: service, repo: repo, native: native, managed: registry}
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
		// Remote executions are completed by ManagedReconciler. A duplicate queue
		// delivery must never submit the same external job twice.
		return nil
	case domain.ExecutionQueued:
	default:
		return fmt.Errorf("execution %s has unsupported status %s", execution.ID, execution.Status)
	}

	workflowVersion, err := h.repo.GetVersion(ctx, execution.WorkflowVersionID)
	if err != nil {
		return fmt.Errorf("load workflow version %s: %w", execution.WorkflowVersionID, err)
	}
	request := workflowapp.ProcessingRequestFromExecution(execution, workflowVersion)

	if engineType := workflowapp.ManagedEngineType(workflowVersion); engineType != "" {
		return h.submitManaged(ctx, execution, request, engineType)
	}
	return h.executeNative(ctx, execution, request)
}

func (h *Handler) submitManaged(ctx context.Context, execution domain.Execution, request workflowapp.ProcessingRequest, engineType string) error {
	bridge, ok := h.managed[engineType]
	if !ok {
		if _, err := h.service.Fail(ctx, execution.ID, "PROCESSING_ENGINE_UNAVAILABLE", "managed processing engine "+engineType+" is not configured", map[string]any{
			"engineType": engineType,
		}, execution.ID.String()); err != nil {
			return fmt.Errorf("persist unavailable engine failure for execution %s: %w", execution.ID, err)
		}
		return nil
	}

	if execution.EngineType != engineType {
		selected, err := h.service.SelectEngine(ctx, execution.ID, engineType, execution.ID.String())
		if err != nil {
			return fmt.Errorf("select %s engine for execution %s: %w", engineType, execution.ID, err)
		}
		execution = selected
	}

	run, err := bridge.Submit(ctx, request)
	if err != nil {
		if _, failErr := h.service.Fail(ctx, execution.ID, "REMOTE_SUBMIT_FAILED", err.Error(), map[string]any{
			"engineType": engineType,
		}, execution.ID.String()); failErr != nil {
			return fmt.Errorf("remote submit failed: %v; persist failure: %w", err, failErr)
		}
		return nil
	}
	if strings.TrimSpace(run.ID) == "" {
		if _, failErr := h.service.Fail(ctx, execution.ID, "REMOTE_SUBMIT_INVALID", "managed engine returned an empty external execution id", map[string]any{
			"engineType": engineType,
		}, execution.ID.String()); failErr != nil {
			return fmt.Errorf("persist invalid remote submit result: %w", failErr)
		}
		return nil
	}

	if _, err := h.service.Start(ctx, execution.ID, run.ID, execution.ID.String()); err != nil {
		return fmt.Errorf("mark managed execution %s running: %w", execution.ID, err)
	}
	// Even if the remote runtime reports a terminal state immediately, completion
	// is delegated to ManagedReconciler so output import follows one idempotent path.
	return nil
}

func (h *Handler) executeNative(ctx context.Context, execution domain.Execution, request workflowapp.ProcessingRequest) error {
	if h.native == nil {
		return fmt.Errorf("native processing engine is not configured")
	}
	engineExecutionID := "native:" + execution.ID.String()
	started, err := h.service.Start(ctx, execution.ID, engineExecutionID, execution.ID.String())
	if err != nil {
		return fmt.Errorf("start execution %s: %w", execution.ID, err)
	}
	request = workflowapp.ProcessingRequestFromExecution(started, request.WorkflowVersion)

	result, err := h.native.Execute(ctx, request)
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
	if result.Metrics == nil {
		result.Metrics = map[string]any{}
	}
	if result.EngineExecutionID != "" && result.EngineExecutionID != engineExecutionID {
		result.Metrics["adapterExecutionId"] = result.EngineExecutionID
	}
	if _, err := h.service.Succeed(ctx, started.ID, result.OutputDatasetVersionID, result.Metrics, started.ID.String()); err != nil {
		return fmt.Errorf("complete execution %s: %w", started.ID, err)
	}
	return nil
}
