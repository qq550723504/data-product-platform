package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
)

type fakeManagedRepo struct {
	execution domain.Execution
	version   domain.WorkflowVersion
}

func (r *fakeManagedRepo) ListManagedExecutionIDsByEngine(_ context.Context, _ string, after uuid.UUID, _ int) ([]uuid.UUID, error) {
	if after != uuid.Nil {
		return nil, nil
	}
	return []uuid.UUID{r.execution.ID}, nil
}

func (r *fakeManagedRepo) GetExecution(context.Context, uuid.UUID) (domain.Execution, error) {
	return r.execution, nil
}

func (r *fakeManagedRepo) GetVersion(context.Context, uuid.UUID) (domain.WorkflowVersion, error) {
	return r.version, nil
}

type fakeManagedStateService struct {
	started           int
	succeeded         int
	failed            int
	outputID          uuid.UUID
	engineExecutionID string
	failCode          string
	failMessage       string
	metrics           map[string]any
}

func (s *fakeManagedStateService) Start(_ context.Context, _ uuid.UUID, engineExecutionID, _ string) (domain.Execution, error) {
	s.started++
	s.engineExecutionID = engineExecutionID
	return domain.Execution{Status: domain.ExecutionRunning, EngineExecutionID: engineExecutionID}, nil
}

func (s *fakeManagedStateService) Succeed(_ context.Context, _ uuid.UUID, outputDatasetVersionID uuid.UUID, metrics map[string]any, _ string) (domain.Execution, error) {
	s.succeeded++
	s.outputID = outputDatasetVersionID
	s.metrics = metrics
	return domain.Execution{Status: domain.ExecutionSucceeded}, nil
}

func (s *fakeManagedStateService) Fail(_ context.Context, _ uuid.UUID, code, message string, metrics map[string]any, _ string) (domain.Execution, error) {
	s.failed++
	s.failCode = code
	s.failMessage = message
	s.metrics = metrics
	return domain.Execution{Status: domain.ExecutionFailed}, nil
}

type fakeManagedBridge struct {
	state       EngineRunState
	startErr    error
	startCalls  int
	startRunID  string
	statusErr   error
	statusCalls int
	finalizeErr error
	outputID    uuid.UUID
}

func (b *fakeManagedBridge) EngineType() string { return "HOP" }

func (b *fakeManagedBridge) PrepareSubmission(context.Context, ProcessingRequest) (EngineRun, error) {
	return EngineRun{}, errors.New("not used")
}

func (b *fakeManagedBridge) StartSubmission(_ context.Context, _ ProcessingRequest, runID string) (EngineRun, error) {
	b.startCalls++
	b.startRunID = runID
	if b.startErr != nil {
		return EngineRun{ID: runID, State: EngineRunQueued}, b.startErr
	}
	return EngineRun{ID: runID, State: b.state}, nil
}

func (b *fakeManagedBridge) RecoverSubmission(ctx context.Context, request ProcessingRequest, runID string) (EngineRun, error) {
	return b.StartSubmission(ctx, request, runID)
}

func (b *fakeManagedBridge) Status(context.Context, ProcessingRequest, string) (EngineRun, error) {
	b.statusCalls++
	if b.statusErr != nil {
		return EngineRun{}, b.statusErr
	}
	return EngineRun{
		ID:           "hop-run-1",
		Name:         "energy-monthly",
		State:        b.state,
		ErrorMessage: "provider-specific stack trace must not enter Core error_message",
		Metrics:      map[string]any{"nrErrors": 0},
	}, nil
}

func (b *fakeManagedBridge) Finalize(context.Context, ProcessingRequest, EngineRun) (ProcessingResult, error) {
	if b.finalizeErr != nil {
		return ProcessingResult{}, b.finalizeErr
	}
	return ProcessingResult{
		OutputDatasetVersionID: b.outputID,
		EngineExecutionID:      "hop-run-1",
		Metrics:                map[string]any{"outputReused": false},
	}, nil
}

func TestManagedReconcilerRetriesFinalizationAfterRemoteSuccess(t *testing.T) {
	execution := runningHopExecution()
	repo := &fakeManagedRepo{
		execution: execution,
		version: domain.WorkflowVersion{
			ID: execution.WorkflowVersionID,
		},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{
		state:       EngineRunSucceeded,
		finalizeErr: errors.New("staged output not visible yet"),
		outputID:    uuid.New(),
	}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err == nil {
		t.Fatal("expected first reconciliation to report finalization error")
	}
	if state.succeeded != 0 || state.failed != 0 {
		t.Fatalf("Core execution must remain RUNNING while finalization is retryable; succeeded=%d failed=%d", state.succeeded, state.failed)
	}

	bridge.finalizeErr = nil
	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("second reconciliation: %v", err)
	}
	if state.succeeded != 1 || state.failed != 0 {
		t.Fatalf("expected one success after retry; succeeded=%d failed=%d", state.succeeded, state.failed)
	}
	if state.outputID != bridge.outputID {
		t.Fatalf("expected output %s, got %s", bridge.outputID, state.outputID)
	}
	if state.metrics["externalExecutionId"] != "hop-run-1" {
		t.Fatalf("missing remote execution traceability: %#v", state.metrics)
	}
}

func TestManagedReconcilerTerminalizesPermanentlyInvalidManagedOutput(t *testing.T) {
	execution := runningHopExecution()
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{
		state: EngineRunSucceeded,
		finalizeErr: NewManagedEngineError(
			ManagedEngineOutputInvalid,
			"finalize output",
			false,
			0,
			errors.New("provider-specific invalid output detail"),
		),
	}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("terminalize invalid managed output: %v", err)
	}
	if state.succeeded != 0 || state.failed != 1 {
		t.Fatalf("expected one terminal failure; succeeded=%d failed=%d", state.succeeded, state.failed)
	}
	if state.failCode != "REMOTE_OUTPUT_INVALID" {
		t.Fatalf("failure code = %q, want REMOTE_OUTPUT_INVALID", state.failCode)
	}
	if state.failMessage != "managed processing output is invalid" {
		t.Fatalf("failure message leaked provider detail: %q", state.failMessage)
	}
	if state.metrics["finalizationErrorKind"] != string(ManagedEngineOutputInvalid) {
		t.Fatalf("missing finalization error kind: %#v", state.metrics)
	}
}

func TestManagedReconcilerMapsRemoteFailureToPlatformOwnedError(t *testing.T) {
	execution := runningHopExecution()
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{state: EngineRunFailed}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("reconcile remote failure: %v", err)
	}
	if state.failed != 1 || state.succeeded != 0 {
		t.Fatalf("expected one failure; succeeded=%d failed=%d", state.succeeded, state.failed)
	}
	if state.failCode != "REMOTE_EXECUTION_FAILED" {
		t.Fatalf("unexpected failure code %q", state.failCode)
	}
	if state.failMessage != "remote processing engine reported failure" {
		t.Fatalf("raw provider diagnostics leaked into Core error message: %q", state.failMessage)
	}
}

func TestManagedReconcilerLeavesRunningRemoteExecutionUntouched(t *testing.T) {
	execution := runningHopExecution()
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{state: EngineRunRunning}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("reconcile running remote execution: %v", err)
	}
	if state.succeeded != 0 || state.failed != 0 {
		t.Fatalf("non-terminal remote run must not transition Core execution; succeeded=%d failed=%d", state.succeeded, state.failed)
	}
}

func TestManagedReconcilerDoesNotRecoverFreshKnownSubmission(t *testing.T) {
	claimedAt := time.Now().UTC()
	execution := domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionSubmitting,
		EngineType:        "HOP",
		EngineExecutionID: "hop-run-fresh",
		StartedAt:         &claimedAt,
	}
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{state: EngineRunRunning}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("fresh submission reconciliation: %v", err)
	}
	if bridge.startCalls != 0 || state.started != 0 || state.failed != 0 {
		t.Fatalf("fresh submission must remain owned by initial submitter; startCalls=%d started=%d failed=%d",
			bridge.startCalls, state.started, state.failed)
	}
}

func TestManagedReconcilerPreservesAmbiguousSubmissionAfterRecoveryRejection(t *testing.T) {
	claimedAt := time.Now().UTC().Add(-10 * time.Minute)
	execution := domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionSubmitting,
		EngineType:        "HOP",
		EngineExecutionID: "hop-run-ambiguous",
		StartedAt:         &claimedAt,
		Metrics: map[string]any{
			managedSubmissionOutcomeUnknownMetric: true,
		},
	}
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{
		startErr: NewManagedEngineError(
			ManagedEngineRejected,
			"start submission",
			false,
			400,
			errors.New("recovery rejected"),
		),
	}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err == nil {
		t.Fatal("ambiguous prior outcome plus recovery rejection must remain unresolved")
	}
	if bridge.startCalls != 1 || state.started != 0 || state.failed != 0 {
		t.Fatalf("ambiguous submission was terminalized; startCalls=%d started=%d failed=%d",
			bridge.startCalls, state.started, state.failed)
	}
}

func TestManagedReconcilerFailsUnambiguousExpiredSubmissionAfterRecoveryRejection(t *testing.T) {
	claimedAt := time.Now().UTC().Add(-10 * time.Minute)
	execution := domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionSubmitting,
		EngineType:        "HOP",
		EngineExecutionID: "hop-run-rejected",
		StartedAt:         &claimedAt,
	}
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{
		startErr: NewManagedEngineError(
			ManagedEngineRejected,
			"start submission",
			false,
			400,
			errors.New("provider rejected recovery"),
		),
	}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("terminalize definite recovery rejection: %v", err)
	}
	if bridge.startCalls != 1 || state.failed != 1 || state.failCode != "REMOTE_SUBMIT_FAILED" {
		t.Fatalf("unambiguous rejection must fail; startCalls=%d failed=%d code=%q",
			bridge.startCalls, state.failed, state.failCode)
	}
}

func TestManagedReconcilerConfirmsKnownUncertainSubmission(t *testing.T) {
	claimedAt := time.Now().UTC().Add(-10 * time.Minute)
	execution := domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionSubmitting,
		EngineType:        "HOP",
		EngineExecutionID: "hop-run-1",
		StartedAt:         &claimedAt,
	}
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{state: EngineRunRunning}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("reconcile known uncertain submission: %v", err)
	}
	if bridge.startCalls != 1 || bridge.startRunID != "hop-run-1" || state.started != 1 || state.engineExecutionID != "hop-run-1" {
		t.Fatalf("startCalls=%d startRunID=%q started=%d engineExecutionID=%q", bridge.startCalls, bridge.startRunID, state.started, state.engineExecutionID)
	}
	if state.failed != 0 {
		t.Fatalf("known remote identity must not expire as unknown; failed=%d", state.failed)
	}
}

func TestManagedReconcilerKeepsKnownSubmissionWhenStartOutcomeUnknown(t *testing.T) {
	claimedAt := time.Now().UTC().Add(-10 * time.Minute)
	execution := domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionSubmitting,
		EngineType:        "HOP",
		EngineExecutionID: "hop-run-1",
		StartedAt:         &claimedAt,
	}
	repo := &fakeManagedRepo{
		execution: execution,
		version:   domain.WorkflowVersion{ID: execution.WorkflowVersionID},
	}
	state := &fakeManagedStateService{}
	bridge := &fakeManagedBridge{
		startErr: NewManagedEngineError(ManagedEngineOutcomeUnknown, "start", true, 503, errors.New("response lost")),
	}
	reconciler := NewManagedReconciler(state, repo, bridge)

	if err := reconciler.RunOnce(context.Background()); err == nil {
		t.Fatal("expected start outcome-unknown error while preserving SUBMITTING")
	}
	if bridge.startCalls != 1 || bridge.startRunID != "hop-run-1" || state.started != 0 || state.failed != 0 {
		t.Fatalf("startCalls=%d startRunID=%q started=%d failed=%d, want 1/hop-run-1/0/0", bridge.startCalls, bridge.startRunID, state.started, state.failed)
	}
}

func TestManagedReconcilerExpiresUncertainSubmittingExecution(t *testing.T) {
	claimedAt := time.Now().UTC().Add(-10 * time.Minute)
	execution := domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionSubmitting,
		EngineType:        "HOP",
		StartedAt:         &claimedAt,
	}
	repo := &fakeManagedRepo{execution: execution}
	state := &fakeManagedStateService{}
	reconciler := NewManagedReconciler(state, repo, &fakeManagedBridge{})

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("expire uncertain submission: %v", err)
	}
	if state.failed != 1 || state.failCode != "REMOTE_SUBMISSION_OUTCOME_UNKNOWN" {
		t.Fatalf("expected uncertain submission failure, got count=%d code=%q", state.failed, state.failCode)
	}
}

func runningHopExecution() domain.Execution {
	return domain.Execution{
		ID:                uuid.New(),
		WorkspaceID:       uuid.New(),
		WorkflowVersionID: uuid.New(),
		OutputDatasetID:   uuid.New(),
		TargetPeriod:      "2026-09",
		Status:            domain.ExecutionRunning,
		EngineType:        "HOP",
		EngineExecutionID: "hop-run-1",
		Inputs: []domain.InputBinding{
			{Name: "energy_standardized", DatasetVersionID: uuid.New()},
		},
	}
}
