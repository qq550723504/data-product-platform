package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

// ManagedExecutionBridge connects one provider-neutral Core Execution to a
// remotely managed runtime. Provider-specific request packaging and output
// import stay behind this boundary.
type ManagedExecutionBridge interface {
	EngineType() string

	// Local preparation must finish before a physical provider attempt is
	// recorded. Invoke* methods are the exact external side-effect boundary.
	PrepareRegisterRequest(ctx context.Context, request ProcessingRequest) (ManagedSubmitRequest, error)
	InvokeRegisterSubmission(ctx context.Context, request ManagedSubmitRequest) (EngineRun, error)
	PrepareStartRequest(ctx context.Context, request ProcessingRequest) (ManagedSubmitRequest, error)
	InvokeStartSubmission(ctx context.Context, request ManagedSubmitRequest, runID string) (EngineRun, error)
	InvokeRecoverSubmission(ctx context.Context, request ManagedSubmitRequest, runID string) (EngineRun, error)

	Status(ctx context.Context, request ProcessingRequest, runID string) (EngineRun, error)
	Finalize(ctx context.Context, request ProcessingRequest, run EngineRun) (ProcessingResult, error)
}

type ManagedExecutionRepository interface {
	ListManagedExecutionIDsByEngine(ctx context.Context, engineType string, after uuid.UUID, limit int) ([]uuid.UUID, error)
	GetExecution(ctx context.Context, executionID uuid.UUID) (domain.Execution, error)
	GetVersion(ctx context.Context, versionID uuid.UUID) (domain.WorkflowVersion, error)
}

type ManagedExecutionStateService interface {
	Start(ctx context.Context, executionID uuid.UUID, engineExecutionID, traceID string) (domain.Execution, error)
	RecordManagedSubmissionRecoveryAttempt(ctx context.Context, executionID uuid.UUID, traceID string) (uuid.UUID, error)
	RecordManagedSubmissionRecoveryOutcome(ctx context.Context, executionID, attemptID uuid.UUID, outcome, traceID string) error
	Succeed(ctx context.Context, executionID, outputDatasetVersionID uuid.UUID, metrics map[string]any, traceID string) (domain.Execution, error)
	Fail(ctx context.Context, executionID uuid.UUID, code, message string, metrics map[string]any, traceID string) (domain.Execution, error)
}

func ProcessingRequestFromExecution(execution domain.Execution, version domain.WorkflowVersion) ProcessingRequest {
	return ProcessingRequest{
		ExecutionID:     execution.ID,
		WorkspaceID:     execution.WorkspaceID,
		WorkflowVersion: version,
		Inputs:          execution.Inputs,
		OutputDatasetID: execution.OutputDatasetID,
		TargetPeriod:    execution.TargetPeriod,
	}
}

func ManagedEngineType(version domain.WorkflowVersion) string {
	spec, ok := stringMap(version.Definition["spec"])
	if !ok {
		return ""
	}
	managed, ok := stringMap(spec["managedExecution"])
	if !ok {
		return ""
	}
	engine, _ := managed["engine"].(string)
	return strings.ToUpper(strings.TrimSpace(engine))
}

func stringMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			name, ok := key.(string)
			if !ok {
				return nil, false
			}
			converted[name] = item
		}
		return converted, true
	default:
		return nil, false
	}
}

const DefaultManagedSubmissionTimeout = 5 * time.Minute

type ManagedReconciler struct {
	service           ManagedExecutionStateService
	repo              ManagedExecutionRepository
	bridges           map[string]ManagedExecutionBridge
	recoveryLocker    NativeRecoveryLocker
	limit             int
	submissionTimeout time.Duration
}

func NewManagedReconciler(service ManagedExecutionStateService, repo ManagedExecutionRepository, bridges ...ManagedExecutionBridge) *ManagedReconciler {
	registry := make(map[string]ManagedExecutionBridge, len(bridges))
	for _, bridge := range bridges {
		if bridge == nil {
			continue
		}
		engineType := strings.ToUpper(strings.TrimSpace(bridge.EngineType()))
		if engineType != "" {
			registry[engineType] = bridge
		}
	}
	return &ManagedReconciler{
		service:           service,
		repo:              repo,
		bridges:           registry,
		limit:             100,
		submissionTimeout: DefaultManagedSubmissionTimeout,
	}
}

func ManagedSubmissionRecoveryLockKey(executionID uuid.UUID) string {
	return "managed-submission-recovery:" + executionID.String()
}

func (r *ManagedReconciler) WithRecoveryLocker(locker NativeRecoveryLocker) *ManagedReconciler {
	r.recoveryLocker = locker
	return r
}

func (r *ManagedReconciler) RunOnce(ctx context.Context) error {
	var failures []error
	for engineType, bridge := range r.bridges {
		cursor := uuid.Nil
		for {
			ids, err := r.repo.ListManagedExecutionIDsByEngine(ctx, engineType, cursor, r.limit)
			if err != nil {
				failures = append(failures, err)
				break
			}
			for _, executionID := range ids {
				if err := r.reconcileOne(ctx, bridge, executionID); err != nil {
					failures = append(failures, err)
				}
			}
			if len(ids) < r.limit {
				break
			}
			cursor = ids[len(ids)-1]
		}
	}
	return errors.Join(failures...)
}

func (r *ManagedReconciler) reconcileOne(ctx context.Context, bridge ManagedExecutionBridge, executionID uuid.UUID) error {
	execution, err := r.repo.GetExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("load managed execution %s: %w", executionID, err)
	}

	if execution.Status == domain.ExecutionSubmitting {
		claimedAt := execution.StartedAt
		if claimedAt == nil {
			claimedAt = &execution.CreatedAt
		}
		if time.Since(*claimedAt) < r.submissionTimeout {
			// The original submitter still owns the submission lease. Recovery
			// must not issue a concurrent start for the same durable remote id.
			return nil
		}

		if strings.TrimSpace(execution.EngineExecutionID) != "" {
			if r.recoveryLocker == nil {
				return fmt.Errorf("managed submission recovery locker is not configured")
			}
			err := r.recoveryLocker.WithAdvisoryLock(ctx, ManagedSubmissionRecoveryLockKey(executionID), func(ctx context.Context) error {
				return r.recoverExpiredSubmission(ctx, bridge, executionID)
			})
			if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
				return nil
			}
			return err
		}

		_, err := r.service.Fail(
			ctx,
			execution.ID,
			"REMOTE_SUBMISSION_OUTCOME_UNKNOWN",
			"remote submission did not yield a durable execution id before the submission lease expired",
			map[string]any{"engineType": execution.EngineType},
			execution.ID.String(),
		)
		if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
			return fmt.Errorf("expire uncertain managed submission %s: %w", executionID, err)
		}
		return nil
	}

	if execution.Status != domain.ExecutionRunning {
		return nil
	}
	version, err := r.repo.GetVersion(ctx, execution.WorkflowVersionID)
	if err != nil {
		return fmt.Errorf("load workflow version for execution %s: %w", executionID, err)
	}
	request := ProcessingRequestFromExecution(execution, version)
	run, err := bridge.Status(ctx, request, execution.EngineExecutionID)
	if err != nil {
		return fmt.Errorf("reconcile %s status for execution %s: %w", execution.EngineType, executionID, err)
	}

	metrics := cloneMetrics(run.Metrics)
	metrics["engineType"] = execution.EngineType
	metrics["externalExecutionId"] = execution.EngineExecutionID

	switch run.State {
	case EngineRunQueued, EngineRunRunning, EngineRunUnknown:
		return nil
	case EngineRunSucceeded:
		result, err := bridge.Finalize(ctx, request, run)
		if err != nil {
			var managedErr *ManagedEngineError
			if errors.As(err, &managedErr) && managedErr.Kind == ManagedEngineOutputInvalid && !managedErr.Retryable {
				metrics["finalizationErrorKind"] = string(managedErr.Kind)
				_, failErr := r.service.Fail(
					ctx,
					execution.ID,
					"REMOTE_OUTPUT_INVALID",
					"managed processing output is invalid",
					metrics,
					execution.ID.String(),
				)
				if failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
					return fmt.Errorf("terminalize invalid managed output for execution %s: %w", executionID, failErr)
				}
				return nil
			}
			// The remote run is terminal but the Core output has not been imported.
			// Retry transient/unknown finalization failures; do not strand a known
			// permanently invalid output in RUNNING forever.
			return fmt.Errorf("finalize managed execution %s: %w", executionID, err)
		}
		for key, value := range result.Metrics {
			metrics[key] = value
		}
		if result.EngineExecutionID != "" {
			metrics["adapterExecutionId"] = result.EngineExecutionID
		}
		_, err = r.service.Succeed(ctx, execution.ID, result.OutputDatasetVersionID, metrics, execution.ID.String())
		if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
			return fmt.Errorf("complete managed execution %s: %w", executionID, err)
		}
		return nil
	case EngineRunFailed:
		_, err := r.service.Fail(
			ctx,
			execution.ID,
			"REMOTE_EXECUTION_FAILED",
			"remote processing engine reported failure",
			metrics,
			execution.ID.String(),
		)
		if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
			return fmt.Errorf("fail managed execution %s: %w", executionID, err)
		}
		return nil
	case EngineRunCancelled:
		_, err := r.service.Fail(
			ctx,
			execution.ID,
			"REMOTE_EXECUTION_CANCELLED",
			"remote processing engine cancelled execution",
			metrics,
			execution.ID.String(),
		)
		if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
			return fmt.Errorf("cancel managed execution %s: %w", executionID, err)
		}
		return nil
	default:
		return nil
	}
}

func (r *ManagedReconciler) recoverExpiredSubmission(ctx context.Context, bridge ManagedExecutionBridge, executionID uuid.UUID) error {
	execution, err := r.repo.GetExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("reload managed execution %s for submission recovery: %w", executionID, err)
	}
	if execution.Status != domain.ExecutionSubmitting || strings.TrimSpace(execution.EngineExecutionID) == "" {
		return nil
	}
	claimedAt := execution.StartedAt
	if claimedAt == nil {
		claimedAt = &execution.CreatedAt
	}
	if time.Since(*claimedAt) < r.submissionTimeout {
		return nil
	}

	version, err := r.repo.GetVersion(ctx, execution.WorkflowVersionID)
	if err != nil {
		return fmt.Errorf("load workflow version for uncertain submission %s: %w", executionID, err)
	}
	request := ProcessingRequestFromExecution(execution, version)
	preparedRequest, err := bridge.PrepareStartRequest(ctx, request)
	if err != nil {
		return fmt.Errorf("prepare managed submission recovery %s before provider call: %w", executionID, err)
	}
	attemptID, err := r.service.RecordManagedSubmissionRecoveryAttempt(ctx, execution.ID, execution.ID.String())
	if err != nil {
		return fmt.Errorf("record managed submission recovery attempt %s: %w", executionID, err)
	}

	if _, err := bridge.InvokeRecoverSubmission(ctx, preparedRequest, execution.EngineExecutionID); err != nil {
		outcome := "RETRYABLE_ERROR"
		if IsManagedEngineOutcomeUnknown(err) {
			outcome = "OUTCOME_UNKNOWN"
		} else if IsManagedEngineDefiniteRejection(err) {
			outcome = "DEFINITE_REJECTION"
		} else if managedErr := new(ManagedEngineError); errors.As(err, &managedErr) {
			outcome = string(managedErr.Kind)
		} else {
			outcome = "LOCAL_ERROR"
		}
		if recordErr := r.service.RecordManagedSubmissionRecoveryOutcome(ctx, execution.ID, attemptID, outcome, execution.ID.String()); recordErr != nil {
			return fmt.Errorf("record managed submission recovery outcome %s: %w", executionID, recordErr)
		}
		if !IsManagedEngineDefiniteRejection(err) {
			// Local/config/storage errors and remote outcome-unknown/unavailable
			// failures are not authoritative proof that the remote start failed.
			return fmt.Errorf("reconcile uncertain %s start for execution %s: %w", execution.EngineType, executionID, err)
		}
		_, failErr := r.service.Fail(
			ctx,
			execution.ID,
			"REMOTE_SUBMIT_FAILED",
			"remote processing engine start was rejected",
			map[string]any{"engineType": execution.EngineType, "externalExecutionId": execution.EngineExecutionID, "recoveryAttemptId": attemptID},
			execution.ID.String(),
		)
		if failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("terminalize rejected managed start %s: %w", executionID, failErr)
		}
		return nil
	}
	if err := r.service.RecordManagedSubmissionRecoveryOutcome(ctx, execution.ID, attemptID, "ACCEPTED", execution.ID.String()); err != nil {
		return fmt.Errorf("record managed submission recovery success %s: %w", executionID, err)
	}
	if _, err := r.service.Start(ctx, execution.ID, execution.EngineExecutionID, execution.ID.String()); err != nil &&
		!errors.Is(err, domain.ErrInvalidTransition) {
		return fmt.Errorf("confirm managed submission %s: %w", executionID, err)
	}
	return nil
}

func cloneMetrics(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}
