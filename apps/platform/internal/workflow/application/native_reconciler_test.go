package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

type fakeNativeLocker struct {
	busy bool
}

func (l *fakeNativeLocker) WithAdvisoryLock(ctx context.Context, _ string, fn func(context.Context) error) error {
	if l.busy {
		return errors.New("lock busy")
	}
	return fn(ctx)
}

type fakeNativeRepo struct {
	execution  domain.Execution
	version    domain.WorkflowVersion
	validateErr error
}

func (r *fakeNativeRepo) ListStaleNativeExecutionIDs(_ context.Context, _ time.Time, after uuid.UUID, _ int) ([]uuid.UUID, error) {
	if after != uuid.Nil {
		return nil, nil
	}
	return []uuid.UUID{r.execution.ID}, nil
}

func (r *fakeNativeRepo) GetExecution(context.Context, uuid.UUID) (domain.Execution, error) {
	return r.execution, nil
}

func (r *fakeNativeRepo) GetVersion(context.Context, uuid.UUID) (domain.WorkflowVersion, error) {
	return r.version, nil
}

func (r *fakeNativeRepo) ValidateExecutionOwnership(context.Context, domain.Execution) error {
	return r.validateErr
}

type fakeNativeOutputRepo struct {
	version datasetdomain.DatasetVersion
	err     error
}

func (r *fakeNativeOutputRepo) FindVersionByExecution(context.Context, uuid.UUID) (datasetdomain.DatasetVersion, error) {
	return r.version, r.err
}

type fakeNativeState struct {
	recoveryActions []string
	succeeded       int
	failed          int
	outputID        uuid.UUID
	failCode        string
	metrics         map[string]any
}

func (s *fakeNativeState) RecordNativeRecovery(_ context.Context, _ uuid.UUID, action, _ string) error {
	s.recoveryActions = append(s.recoveryActions, action)
	return nil
}

func (s *fakeNativeState) Succeed(_ context.Context, _ uuid.UUID, outputID uuid.UUID, metrics map[string]any, _ string) (domain.Execution, error) {
	s.succeeded++
	s.outputID = outputID
	s.metrics = metrics
	return domain.Execution{Status: domain.ExecutionSucceeded}, nil
}

func (s *fakeNativeState) Fail(_ context.Context, _ uuid.UUID, code, _ string, metrics map[string]any, _ string) (domain.Execution, error) {
	s.failed++
	s.failCode = code
	s.metrics = metrics
	return domain.Execution{Status: domain.ExecutionFailed}, nil
}

type fakeNativeEngine struct {
	calls  int
	result ProcessingResult
	err    error
}

func (e *fakeNativeEngine) Execute(context.Context, ProcessingRequest) (ProcessingResult, error) {
	e.calls++
	return e.result, e.err
}

func staleNativeExecution() domain.Execution {
	started := time.Now().UTC().Add(-time.Hour)
	return domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionRunning,
		EngineType:        "NATIVE",
		EngineExecutionID: "native:recovery-test",
		Metrics:           map[string]any{"before": "kept"},
		StartedAt:         &started,
		CreatedAt:         started.Add(-time.Minute),
	}
}

func TestNativeReconcilerAdoptsExistingReadyOutput(t *testing.T) {
	execution := staleNativeExecution()
	output := datasetdomain.DatasetVersion{ID: uuid.New(), DatasetID: execution.OutputDatasetID, Status: datasetdomain.VersionReady}
	repo := &fakeNativeRepo{execution: execution}
	state := &fakeNativeState{}
	engine := &fakeNativeEngine{}
	reconciler := NewNativeReconciler(&fakeNativeLocker{}, state, repo, &fakeNativeOutputRepo{version: output}, engine)
	reconciler.leaseTTL = time.Second

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("reconcile ready output: %v", err)
	}
	if engine.calls != 0 {
		t.Fatalf("ready output must not re-execute engine, calls=%d", engine.calls)
	}
	if state.succeeded != 1 || state.outputID != output.ID {
		t.Fatalf("expected recovered success for output %s, got count=%d output=%s", output.ID, state.succeeded, state.outputID)
	}
	if len(state.recoveryActions) != 1 || state.recoveryActions[0] != "ADOPT_EXISTING_OUTPUT" {
		t.Fatalf("unexpected recovery actions: %#v", state.recoveryActions)
	}
	if state.metrics["nativeRecovery"] != true || state.metrics["outputReused"] != true {
		t.Fatalf("missing recovery metrics: %#v", state.metrics)
	}
}

func TestNativeReconcilerReexecutesWhenNoOutputExists(t *testing.T) {
	execution := staleNativeExecution()
	outputID := uuid.New()
	repo := &fakeNativeRepo{execution: execution, version: domain.WorkflowVersion{ID: execution.WorkflowVersionID}}
	state := &fakeNativeState{}
	engine := &fakeNativeEngine{result: ProcessingResult{OutputDatasetVersionID: outputID, EngineExecutionID: "native:" + execution.ID.String(), Metrics: map[string]any{"rows": 5}}}
	reconciler := NewNativeReconciler(&fakeNativeLocker{}, state, repo, &fakeNativeOutputRepo{err: datasetinfra.ErrNotFound}, engine)
	reconciler.leaseTTL = time.Second

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("reexecute stale native execution: %v", err)
	}
	if engine.calls != 1 || state.succeeded != 1 || state.outputID != outputID {
		t.Fatalf("unexpected recovery result: engine=%d succeeded=%d output=%s", engine.calls, state.succeeded, state.outputID)
	}
	if len(state.recoveryActions) != 1 || state.recoveryActions[0] != "REEXECUTE" {
		t.Fatalf("unexpected recovery actions: %#v", state.recoveryActions)
	}
}

func TestNativeReconcilerTerminalizesFailedRecovery(t *testing.T) {
	execution := staleNativeExecution()
	repo := &fakeNativeRepo{execution: execution, version: domain.WorkflowVersion{ID: execution.WorkflowVersionID}}
	state := &fakeNativeState{}
	engine := &fakeNativeEngine{err: errors.New("synthetic native failure")}
	reconciler := NewNativeReconciler(&fakeNativeLocker{}, state, repo, &fakeNativeOutputRepo{err: datasetinfra.ErrNotFound}, engine)
	reconciler.leaseTTL = time.Second

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("terminalize failed recovery: %v", err)
	}
	if state.failed != 1 || state.failCode != "NATIVE_RECOVERY_FAILED" {
		t.Fatalf("expected NATIVE_RECOVERY_FAILED, count=%d code=%q", state.failed, state.failCode)
	}
}

func TestNativeReconcilerFailsClosedWhenReferencesBecomeUnusable(t *testing.T) {
	execution := staleNativeExecution()
	repo := &fakeNativeRepo{execution: execution, validateErr: domain.ErrExecutionReferenceUnusable}
	state := &fakeNativeState{}
	engine := &fakeNativeEngine{}
	reconciler := NewNativeReconciler(&fakeNativeLocker{}, state, repo, &fakeNativeOutputRepo{err: datasetinfra.ErrNotFound}, engine)
	reconciler.leaseTTL = time.Second

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("fail closed on unusable references: %v", err)
	}
	if engine.calls != 0 || state.failed != 1 || state.failCode != "NATIVE_RECOVERY_REFERENCE_UNUSABLE" {
		t.Fatalf("unexpected reference recovery result: engine=%d failed=%d code=%q", engine.calls, state.failed, state.failCode)
	}
}
