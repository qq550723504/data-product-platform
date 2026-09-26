package workflowqueue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	workflowinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

type Handler struct {
	service       *workflowapp.ExecutionService
	repo          *workflowinfra.PostgresRepository
	native        workflowapp.ProcessingEngine
	nativeLocker  workflowapp.NativeRecoveryLocker
	managedLocker workflowapp.NativeRecoveryLocker
	managed       map[string]workflowapp.ManagedExecutionBridge
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

func (h *Handler) WithManagedSubmissionLocker(locker workflowapp.NativeRecoveryLocker) *Handler {
	h.managedLocker = locker
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

	registerRequest, err := bridge.PrepareRegisterRequest(ctx, request)
	if err != nil {
		return h.failManagedSubmissionPreparation(ctx, execution, engineType, "REGISTER", err)
	}
	registerAttemptID, err := h.service.RecordManagedSubmissionAttempt(ctx, execution.ID, "REGISTER", execution.ID.String())
	if err != nil {
		return fmt.Errorf("record managed register attempt for execution %s: %w", execution.ID, err)
	}
	prepared, err := bridge.InvokeRegisterSubmission(ctx, registerRequest)
	registerOutcome := managedSubmissionAttemptOutcome(err)
	if err == nil && strings.TrimSpace(prepared.ID) == "" {
		registerOutcome = "INVALID_RESPONSE"
	}
	if recordErr := h.service.RecordManagedSubmissionAttemptOutcome(
		ctx, execution.ID, registerAttemptID, "REGISTER", registerOutcome, execution.ID.String(),
	); recordErr != nil {
		return fmt.Errorf("record managed register outcome for execution %s: %w", execution.ID, recordErr)
	}
	if err != nil {
		if managedSubmissionOutcomeUnknown(err) {
			// Registration may have reached the provider but no durable identity
			// was returned. Keep SUBMITTING; the lease will expire to an explicit
			// unknown-outcome failure rather than allowing an automatic resubmit.
			return nil
		}
		if _, failErr := h.service.Fail(
			ctx,
			execution.ID,
			"REMOTE_SUBMIT_FAILED",
			"remote processing engine submission was rejected before start",
			map[string]any{"engineType": engineType},
			execution.ID.String(),
		); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("persist remote prepare failure for execution %s: %w", execution.ID, failErr)
		}
		return nil
	}
	if strings.TrimSpace(prepared.ID) == "" {
		if _, failErr := h.service.Fail(ctx, execution.ID, "REMOTE_SUBMIT_INVALID", "remote processing engine returned no durable execution id", map[string]any{
			"engineType": engineType,
		}, execution.ID.String()); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("persist invalid remote prepare result: %w", failErr)
		}
		return nil
	}

	// The durable remote identity must be committed before remote work starts.
	// If this write fails, no StartSubmission call has happened, so an Asynq
	// retry cannot create duplicate remote work.
	if _, err := h.service.AttachManagedSubmissionReference(
		ctx, execution.ID, prepared.ID, execution.ID.String(),
	); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil
		}
		return fmt.Errorf("persist prepared remote submission reference for execution %s: %w", execution.ID, err)
	}

	startRequest, err := bridge.PrepareStartRequest(ctx, request)
	if err != nil {
		return h.failManagedSubmissionPreparation(ctx, execution, engineType, "START", err)
	}

	// Persist an "armed" phase before the remote side effect. This closes the
	// crash/DB-commit window without claiming that an ambiguous response has
	// actually occurred yet.
	if _, err := h.service.MarkManagedSubmissionStartArmed(ctx, execution.ID, execution.ID.String()); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil
		}
		return fmt.Errorf("persist remote start armed phase for execution %s: %w", execution.ID, err)
	}

	if h.managedLocker == nil {
		return fmt.Errorf("managed submission locker is not configured")
	}
	err = h.managedLocker.WithAdvisoryLock(ctx, workflowapp.ManagedSubmissionRecoveryLockKey(execution.ID), func(ctx context.Context) error {
		current, err := h.repo.GetExecution(ctx, execution.ID)
		if err != nil {
			return fmt.Errorf("reload managed submission %s before start: %w", execution.ID, err)
		}
		if current.Status != domain.ExecutionSubmitting || current.EngineExecutionID != prepared.ID {
			return nil
		}
		claimedAt := current.StartedAt
		if claimedAt == nil {
			claimedAt = &current.CreatedAt
		}
		if time.Since(*claimedAt) >= workflowapp.DefaultManagedSubmissionTimeout {
			// The original submitter's lease expired. Recovery owns any further
			// provider interaction for this durable remote identity.
			return nil
		}
		return h.startManagedSubmissionLocked(ctx, current, startRequest, engineType, prepared.ID, bridge)
	})
	if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		return nil
	}
	return err
}

func (h *Handler) startManagedSubmissionLocked(
	ctx context.Context,
	execution domain.Execution,
	request workflowapp.ManagedSubmitRequest,
	engineType, runID string,
	bridge workflowapp.ManagedExecutionBridge,
) error {
	attemptID, err := h.service.RecordManagedSubmissionAttempt(ctx, execution.ID, "START", execution.ID.String())
	if err != nil {
		return fmt.Errorf("record managed start attempt for execution %s: %w", execution.ID, err)
	}
	run, err := bridge.InvokeStartSubmission(ctx, request, runID)
	if recordErr := h.service.RecordManagedSubmissionAttemptOutcome(
		ctx, execution.ID, attemptID, "START", managedSubmissionAttemptOutcome(err), execution.ID.String(),
	); recordErr != nil {
		return fmt.Errorf("record managed start outcome for execution %s: %w", execution.ID, recordErr)
	}
	if err != nil {
		if managedSubmissionOutcomeUnknown(err) {
			if _, markErr := h.service.MarkManagedSubmissionOutcomeUnknown(ctx, execution.ID, execution.ID.String()); markErr != nil &&
				!errors.Is(markErr, domain.ErrInvalidTransition) {
				return fmt.Errorf("persist unknown remote start outcome for execution %s: %w", execution.ID, markErr)
			}
			return nil
		}
		if !workflowapp.IsManagedEngineDefiniteRejection(err) {
			// Local/config/storage errors are not authoritative proof of a
			// remote rejection. Keep SUBMITTING for fenced reconciliation.
			return fmt.Errorf("managed submission start remains unresolved for execution %s: %w", execution.ID, err)
		}
		if _, failErr := h.service.Fail(
			ctx,
			execution.ID,
			"REMOTE_SUBMIT_FAILED",
			"remote processing engine start was rejected",
			map[string]any{"engineType": engineType, "externalExecutionId": runID},
			execution.ID.String(),
		); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("persist remote start failure for execution %s: %w", execution.ID, failErr)
		}
		return nil
	}
	if strings.TrimSpace(run.ID) == "" {
		run.ID = runID
	}

	if _, err := h.service.Start(ctx, execution.ID, runID, execution.ID.String()); err != nil {
		if errors.Is(err, domain.ErrInvalidTransition) {
			return nil
		}
		return fmt.Errorf("mark managed execution %s running: %w", execution.ID, err)
	}
	return nil
}

func managedSubmissionOutcomeUnknown(err error) bool {
	return workflowapp.IsManagedEngineOutcomeUnknown(err)
}

func managedSubmissionAttemptOutcome(err error) string {
	if err == nil {
		return "SUCCEEDED"
	}
	if workflowapp.IsManagedEngineOutcomeUnknown(err) {
		return "OUTCOME_UNKNOWN"
	}
	if workflowapp.IsManagedEngineDefiniteRejection(err) {
		return "DEFINITE_REJECTION"
	}
	var managedErr *workflowapp.ManagedEngineError
	if errors.As(err, &managedErr) {
		return string(managedErr.Kind)
	}
	return "LOCAL_ERROR"
}

func (h *Handler) failManagedSubmissionPreparation(ctx context.Context, execution domain.Execution, engineType, phase string, cause error) error {
	if isReferenceFailure(cause) {
		return h.quarantine(ctx, execution, cause)
	}
	if _, failErr := h.service.Fail(
		ctx,
		execution.ID,
		"MANAGED_SUBMIT_PREPARATION_FAILED",
		"managed processing submission preparation failed before provider call",
		map[string]any{"engineType": engineType, "phase": phase},
		execution.ID.String(),
	); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
		return fmt.Errorf("persist managed submission preparation failure for execution %s: %w", execution.ID, failErr)
	}
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
