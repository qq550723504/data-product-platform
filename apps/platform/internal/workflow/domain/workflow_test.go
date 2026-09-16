package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestExecutionLifecycleAndRetry(t *testing.T) {
	workspaceID := uuid.New()
	workflowVersionID := uuid.New()
	outputDatasetID := uuid.New()
	inputVersionID := uuid.New()

	execution, err := NewExecution(workspaceID, workflowVersionID, outputDatasetID, "2025-03", []InputBinding{{
		Name: "enterprise_standardized",
		DatasetVersionID: inputVersionID,
	}}, nil)
	if err != nil {
		t.Fatalf("new execution: %v", err)
	}
	if execution.Status != ExecutionQueued || execution.Attempt != 1 {
		t.Fatalf("unexpected initial state: %#v", execution)
	}
	if err := execution.Start("native-1"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if execution.Status != ExecutionRunning || execution.StartedAt == nil {
		t.Fatalf("expected running state: %#v", execution)
	}
	if err := execution.Fail("CALCULATION_FAILED", "boom", map[string]any{"rows": 3}); err != nil {
		t.Fatalf("fail: %v", err)
	}
	if execution.Status != ExecutionFailed || execution.FinishedAt == nil {
		t.Fatalf("expected failed state: %#v", execution)
	}
	retry, err := execution.Retry(nil)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retry.Status != ExecutionQueued || retry.Attempt != 2 || retry.RetryOfExecutionID == nil || *retry.RetryOfExecutionID != execution.ID {
		t.Fatalf("unexpected retry: %#v", retry)
	}
	if len(retry.Inputs) != 1 || retry.Inputs[0].DatasetVersionID != inputVersionID {
		t.Fatalf("retry did not preserve frozen inputs: %#v", retry.Inputs)
	}
}

func TestExecutionRejectsInvalidTransitions(t *testing.T) {
	execution, err := NewExecution(uuid.New(), uuid.New(), uuid.New(), "2025-03", []InputBinding{{Name: "input", DatasetVersionID: uuid.New()}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := execution.Succeed(uuid.New(), nil); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("succeed before start = %v, want invalid transition", err)
	}
	if _, err := execution.Retry(nil); !errors.Is(err, ErrRetryRequiresFailure) {
		t.Fatalf("retry queued = %v, want retry failure", err)
	}
}

func TestExecutionRequiresUniqueFrozenInputs(t *testing.T) {
	_, err := NewExecution(uuid.New(), uuid.New(), uuid.New(), "2025-03", []InputBinding{
		{Name: "enterprise", DatasetVersionID: uuid.New()},
		{Name: "enterprise", DatasetVersionID: uuid.New()},
	}, nil)
	if !errors.Is(err, ErrInvalidExecutionInput) {
		t.Fatalf("duplicate input error = %v", err)
	}
}
