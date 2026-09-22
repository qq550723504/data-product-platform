package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

const (
	defaultNativeRecoveryLeaseTTL = 10 * time.Minute
	defaultNativeRecoveryLimit    = 100
)

type NativeRecoveryLocker interface {
	WithAdvisoryLock(context.Context, string, func(context.Context) error) error
}

type NativeExecutionRepository interface {
	ListStaleNativeExecutionIDs(context.Context, time.Time, uuid.UUID, int) ([]uuid.UUID, error)
	GetExecution(context.Context, uuid.UUID) (domain.Execution, error)
	GetVersion(context.Context, uuid.UUID) (domain.WorkflowVersion, error)
	ValidateExecutionOwnership(context.Context, domain.Execution) error
}

type NativeOutputRepository interface {
	FindVersionByExecution(context.Context, uuid.UUID) (datasetdomain.DatasetVersion, error)
}

type NativeExecutionStateService interface {
	RecordNativeRecovery(context.Context, uuid.UUID, string, string) error
	Succeed(context.Context, uuid.UUID, uuid.UUID, map[string]any, string) (domain.Execution, error)
	Fail(context.Context, uuid.UUID, string, string, map[string]any, string) (domain.Execution, error)
}

type NativeReconciler struct {
	locker   NativeRecoveryLocker
	service  NativeExecutionStateService
	repo     NativeExecutionRepository
	outputs  NativeOutputRepository
	engine   ProcessingEngine
	leaseTTL time.Duration
	limit    int
}

func NewNativeReconciler(locker NativeRecoveryLocker, service NativeExecutionStateService, repo NativeExecutionRepository, outputs NativeOutputRepository, engine ProcessingEngine) *NativeReconciler {
	return &NativeReconciler{
		locker:   locker,
		service:  service,
		repo:     repo,
		outputs:  outputs,
		engine:   engine,
		leaseTTL: defaultNativeRecoveryLeaseTTL,
		limit:    defaultNativeRecoveryLimit,
	}
}

func (r *NativeReconciler) RunOnce(ctx context.Context) error {
	if r == nil || r.locker == nil || r.service == nil || r.repo == nil || r.outputs == nil || r.engine == nil {
		return errors.New("native reconciler is not fully configured")
	}
	leaseTTL := r.leaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultNativeRecoveryLeaseTTL
	}
	limit := r.limit
	if limit <= 0 {
		limit = defaultNativeRecoveryLimit
	}
	cutoff := time.Now().UTC().Add(-leaseTTL)

	var failures []error
	cursor := uuid.Nil
	for {
		ids, err := r.repo.ListStaleNativeExecutionIDs(ctx, cutoff, cursor, limit)
		if err != nil {
			failures = append(failures, err)
			break
		}
		for _, executionID := range ids {
			lockKey := "native-execution-recovery:" + executionID.String()
			err := r.locker.WithAdvisoryLock(ctx, lockKey, func(ctx context.Context) error {
				return r.reconcileOne(ctx, executionID, cutoff)
			})
			if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
				continue
			}
			if err != nil {
				failures = append(failures, err)
			}
		}
		if len(ids) < limit {
			break
		}
		cursor = ids[len(ids)-1]
	}
	return errors.Join(failures...)
}

func (r *NativeReconciler) reconcileOne(ctx context.Context, executionID uuid.UUID, cutoff time.Time) error {
	execution, err := r.repo.GetExecution(ctx, executionID)
	if err != nil {
		return fmt.Errorf("load native execution %s: %w", executionID, err)
	}
	if execution.Status != domain.ExecutionRunning || execution.EngineType != "NATIVE" {
		return nil
	}
	leaseStarted := execution.CreatedAt
	if execution.StartedAt != nil {
		leaseStarted = *execution.StartedAt
	}
	if leaseStarted.After(cutoff) {
		return nil
	}

	output, outputErr := r.outputs.FindVersionByExecution(ctx, execution.ID)
	if outputErr == nil {
		switch output.Status {
		case datasetdomain.VersionReady, datasetdomain.VersionSuperseded:
			if err := r.recordRecovery(ctx, execution.ID, "ADOPT_EXISTING_OUTPUT"); err != nil {
				return err
			}
			metrics := mergeNativeRecoveryMetrics(execution.Metrics, map[string]any{
				"outputReused": true,
				"recoveryAction": "ADOPT_EXISTING_OUTPUT",
			})
			if _, err := r.service.Succeed(ctx, execution.ID, output.ID, metrics, execution.ID.String()); err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
				return fmt.Errorf("finalize recovered native execution %s: %w", execution.ID, err)
			}
			return nil
		case datasetdomain.VersionInvalid:
			if err := r.recordRecovery(ctx, execution.ID, "TERMINAL_INVALID_OUTPUT"); err != nil {
				return err
			}
			if _, err := r.service.Fail(ctx, execution.ID, "NATIVE_OUTPUT_INVALID", "native execution output is invalid", mergeNativeRecoveryMetrics(execution.Metrics, nil), execution.ID.String()); err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
				return fmt.Errorf("terminalize invalid native output for execution %s: %w", execution.ID, err)
			}
			return nil
		}
	} else if !errors.Is(outputErr, datasetinfra.ErrNotFound) {
		return fmt.Errorf("find native execution output %s: %w", execution.ID, outputErr)
	}

	if err := r.repo.ValidateExecutionOwnership(ctx, execution); err != nil {
		if errors.Is(err, domain.ErrWorkspaceMismatch) || errors.Is(err, domain.ErrExecutionReferenceUnusable) {
			if recordErr := r.recordRecovery(ctx, execution.ID, "REFERENCE_UNUSABLE"); recordErr != nil {
				return recordErr
			}
			if _, failErr := r.service.Fail(ctx, execution.ID, "NATIVE_RECOVERY_REFERENCE_UNUSABLE", "native execution recovery references are no longer usable", mergeNativeRecoveryMetrics(execution.Metrics, nil), execution.ID.String()); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
				return fmt.Errorf("terminalize unusable native recovery references for execution %s: %w", execution.ID, failErr)
			}
			return nil
		}
		return fmt.Errorf("validate native recovery references for execution %s: %w", execution.ID, err)
	}

	version, err := r.repo.GetVersion(ctx, execution.WorkflowVersionID)
	if err != nil {
		return fmt.Errorf("load workflow version for native recovery %s: %w", execution.ID, err)
	}
	if err := r.recordRecovery(ctx, execution.ID, "REEXECUTE"); err != nil {
		return err
	}

	result, err := r.engine.Execute(ctx, ProcessingRequestFromExecution(execution, version))
	if err != nil {
		metrics := mergeNativeRecoveryMetrics(execution.Metrics, map[string]any{"recoveryAction": "REEXECUTE"})
		if _, failErr := r.service.Fail(ctx, execution.ID, "NATIVE_RECOVERY_FAILED", "native execution recovery failed", metrics, execution.ID.String()); failErr != nil && !errors.Is(failErr, domain.ErrInvalidTransition) {
			return fmt.Errorf("terminalize failed native recovery for execution %s: %w", execution.ID, failErr)
		}
		return nil
	}
	metrics := mergeNativeRecoveryMetrics(result.Metrics, map[string]any{
		"recoveryAction": "REEXECUTE",
		"outputReused": outputErr == nil,
	})
	if result.EngineExecutionID != "" && result.EngineExecutionID != execution.EngineExecutionID {
		metrics["adapterExecutionId"] = result.EngineExecutionID
	}
	if _, err := r.service.Succeed(ctx, execution.ID, result.OutputDatasetVersionID, metrics, execution.ID.String()); err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
		return fmt.Errorf("complete recovered native execution %s: %w", execution.ID, err)
	}
	return nil
}

func (r *NativeReconciler) recordRecovery(ctx context.Context, executionID uuid.UUID, action string) error {
	if err := r.service.RecordNativeRecovery(ctx, executionID, action, executionID.String()); err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
		return fmt.Errorf("record native recovery %s for execution %s: %w", action, executionID, err)
	}
	return nil
}

func mergeNativeRecoveryMetrics(base map[string]any, extra map[string]any) map[string]any {
	metrics := make(map[string]any, len(base)+len(extra)+1)
	for key, value := range base {
		metrics[key] = value
	}
	for key, value := range extra {
		metrics[key] = value
	}
	metrics["nativeRecovery"] = true
	return metrics
}
