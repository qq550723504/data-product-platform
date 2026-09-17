package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

// ManagedExecutionBridge connects one provider-neutral Core Execution to a
// remotely managed runtime. Provider-specific request packaging and output
// import stay behind this boundary.
type ManagedExecutionBridge interface {
	EngineType() string
	Submit(ctx context.Context, request ProcessingRequest) (EngineRun, error)
	Status(ctx context.Context, request ProcessingRequest, runID string) (EngineRun, error)
	Finalize(ctx context.Context, request ProcessingRequest, run EngineRun) (ProcessingResult, error)
}

func ProcessingRequestFromExecution(execution domain.Execution, version domain.WorkflowVersion) ProcessingRequest {
	return ProcessingRequest{
		ExecutionID:     execution.ID,
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

type ManagedReconciler struct {
	service *ExecutionService
	repo    *infrastructure.PostgresRepository
	bridges map[string]ManagedExecutionBridge
	limit   int
}

func NewManagedReconciler(service *ExecutionService, repo *infrastructure.PostgresRepository, bridges ...ManagedExecutionBridge) *ManagedReconciler {
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
	return &ManagedReconciler{service: service, repo: repo, bridges: registry, limit: 100}
}

func (r *ManagedReconciler) RunOnce(ctx context.Context) error {
	var failures []error
	for engineType, bridge := range r.bridges {
		ids, err := r.repo.ListRunningExecutionIDsByEngine(ctx, engineType, r.limit)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, executionID := range ids {
			if err := r.reconcileOne(ctx, bridge, executionID.String()); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func (r *ManagedReconciler) reconcileOne(ctx context.Context, bridge ManagedExecutionBridge, executionID string) error {
	parsedID, err := parseExecutionID(executionID)
	if err != nil {
		return err
	}
	execution, err := r.repo.GetExecution(ctx, parsedID)
	if err != nil {
		return fmt.Errorf("load managed execution %s: %w", executionID, err)
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
			// The remote run is terminal but the Core output has not been imported.
			// Keep the Execution RUNNING so a later reconciliation can retry finalization.
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
		message := strings.TrimSpace(run.ErrorMessage)
		if message == "" {
			message = "remote processing engine reported failure"
		}
		_, err := r.service.Fail(ctx, execution.ID, "REMOTE_EXECUTION_FAILED", message, metrics, execution.ID.String())
		if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
			return fmt.Errorf("fail managed execution %s: %w", executionID, err)
		}
		return nil
	case EngineRunCancelled:
		_, err := r.service.Fail(ctx, execution.ID, "REMOTE_EXECUTION_CANCELLED", "remote processing engine cancelled execution", metrics, execution.ID.String())
		if err != nil && !errors.Is(err, domain.ErrInvalidTransition) {
			return fmt.Errorf("cancel managed execution %s: %w", executionID, err)
		}
		return nil
	default:
		return nil
	}
}

func cloneMetrics(source map[string]any) map[string]any {
	result := make(map[string]any, len(source)+2)
	for key, value := range source {
		result[key] = value
	}
	return result
}
