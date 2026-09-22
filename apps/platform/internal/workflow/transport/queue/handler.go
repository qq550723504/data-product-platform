package workflowqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type Handler struct {
	service      *workflowapp.ExecutionService
	repo         *workflowinfra.PostgresRepository
	native       workflowapp.ProcessingEngine
	nativeLocker workflowapp.NativeRecoveryLocker
	managed      map[string]workflowapp.ManagedExecutionBridge
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

func (h *Handler) WithNativeExecutionLocker(locker workflowapp.NativeRecoveryLocker) *Handler {
	h.nativeLocker = locker
	return h
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

	// Delivery retries must never create duplicate output or duplicate remote jobs
	// for the same Core Execution. Operational retries are explicit and create a
	// new Execution through ExecutionService.Retry.
	switch execution.Status {
	case domain.ExecutionSucceeded, domain.ExecutionFailed, domain.ExecutionCancelled:
		return nil
	case domain.ExecutionSubmitting, domain.ExecutionRunning:
		return nil
	case domain.ExecutionQueued:
	default:
		return fmt.Errorf("execution %s has unsupported status %s", execution.ID, execution.Status)
	}

	// Revalidate ownership and reference usability immediately before dispatch so a
	// worker never reads a foreign input or writes a foreign output. A reference that
	// no longer satisfies the current contract is quarantined rather than executed.
	if err := h.repo.ValidateExecutionOwnership(ctx, execution); err != nil {
		if isReferenceFailure(err) {
			return h.quarantine(ctx, execution, err)
		}
		return fmt.Errorf("validate execution %s ownership: %w", execution.ID, err)
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

func isReferenceFailure(err error) bool {
	return errors.Is(err, domain.ErrWorkspaceMismatch) || errors.Is(err, workflowinfra.ErrNotFound) || errors.Is(err, domain.ErrExecutionReferenceUnusable)
}

func (h *Handler) quarantine(ctx context.Context, execution domain.Execution, cause error) error {
	code, message := "EXECUTION_REFERENCE_MISSING", "an execution reference no longer exists"
	switch {
	case errors.Is(cause, domain.ErrWorkspaceMismatch):
		code, message = "EXECUTION_REFERENCE_WORKSPACE_MISMATCH", "execution references are not owned by one workspace"
	case errors.Is(cause, domain.ErrExecutionReferenceUnusable):
		code, message = "EXECUTION_REFERENCE_UNUSABLE", "an execution input is no longer immutable and usable"
	}
	if _, err := h.service.Fail(ctx, execution.ID, code, message, map[string]any{"quarantined": true}, execution.ID.String()); err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
		return fmt.Errorf("quarantine execution %s: %w", execution.ID, err)
	}
	return nil
}

func (h *Handler) submitManaged(ctx context.Context, execution domain.Execution, request workflowapp.ProcessingRequest, engineType string) error {
	bridge, ok := h.managed[engineType]
	if !ok {
		if _, err := h.service.Fail(ctx, execution.ID, "PROCESSING_ENGINE_UNAVAILABLE", "managed processing engine "+engineType+" is not configured", map[string]any{
			"engineType": engineType,
		}, execution.ID.String()); err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
			return fmt.Errorf("persist unavailable engine failure for execution %s: %w", execution.ID, err)
		}
		return nil
	}

	// Persist QUEUED -> SUBMITTING before any remote network call. If another
	// worker already claimed this Execution, it wins and this delivery is a no-op.
	claimed, err := h.service.BeginManagedSubmission(ctx, execution.ID, engineType, execution.ID.String())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil
		}
		if isReferenceFailure(err) {
			return h.quarantine(ctx, execution, err)
		}
		return fmt.Errorf("claim %s submission for execution %s: %w", engineType, execution.ID, err)
	}
	execution = claimed
	request = workflowapp.ProcessingRequestFromExecution(execution, request.WorkflowVersion)

	run, err := bridge.Submit(ctx, request)
	if err != nil {
		if _, failErr := h.service.Fail(
			ctx,
			execution.ID,
			"REMOTE_SUBMIT_FAILED",
			"remote processing engine submission failed",
			map[string]any{"engineType": engineType},
			execution.ID.String(),
		); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("persist remote submit failure for execution %s: %w", execution.ID, failErr)
		}
		return nil
	}
	if strings.TrimSpace(run.ID) == "" {
		if _, failErr := h.service.Fail(ctx, execution.ID, "REMOTE_SUBMIT_INVALID", "remote processing engine returned no durable execution id", map[string]any{
			"engineType": engineType,
		}, execution.ID.String()); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("persist invalid remote submit result: %w", failErr)
		}
		return nil
	}

	if _, err := h.service.Start(ctx, execution.ID, run.ID, execution.ID.String()); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil
		}
		return fmt.Errorf("mark managed execution %s running: %w", execution.ID, err)
	}
	// Even if the remote runtime reports a terminal state immediately, completion
	// is delegated to ManagedReconciler so output import follows one serialized path.
	return nil
}

func (h *Handler) executeNative(ctx context.Context, execution domain.Execution, request workflowapp.ProcessingRequest) error {
	if h.nativeLocker == nil {
		return h.executeNativeLocked(ctx, execution, request)
	}
	err := h.nativeLocker.WithAdvisoryLock(ctx, workflowapp.NativeExecutionLockKey(execution.ID), func(ctx context.Context) error {
		return h.executeNativeLocked(ctx, execution, request)
	})
	if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		// Another worker/reconciler owns this exact Core Execution. Duplicate
		// delivery is a no-op; the current owner will converge the state.
		return nil
	}
	return err
}

func (h *Handler) executeNativeLocked(ctx context.Context, execution domain.Execution, request workflowapp.ProcessingRequest) error {
	if h.native == nil {
		return fmt.Errorf("native processing engine is not configured")
	}
	engineExecutionID := "native:" + execution.ID.String()
	started, err := h.service.StartWithReferenceCheck(ctx, execution.ID, engineExecutionID, execution.ID.String())
	if err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil
		}
		if isReferenceFailure(err) {
			return h.quarantine(ctx, execution, err)
		}
		return fmt.Errorf("start execution %s: %w", execution.ID, err)
	}
	request = workflowapp.ProcessingRequestFromExecution(started, request.WorkflowVersion)

	result, err := h.native.Execute(ctx, request)
	if err != nil {
		if _, failErr := h.service.Fail(ctx, started.ID, "PROCESSING_FAILED", "processing engine execution failed", map[string]any{
			"engineType": started.EngineType,
		}, started.ID.String()); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("persist processing failure for execution %s: %w", started.ID, failErr)
		}
		return nil
	}
	if result.Metrics == nil {
		result.Metrics = map[string]any{}
	}
	if result.EngineExecutionID != "" && result.EngineExecutionID != engineExecutionID {
		result.Metrics["adapterExecutionId"] = result.EngineExecutionID
	}
	if _, err := h.service.Succeed(ctx, started.ID, result.OutputDatasetVersionID, result.Metrics, started.ID.String()); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil
		}
		return fmt.Errorf("complete execution %s: %w", started.ID, err)
	}
	return nil
}
