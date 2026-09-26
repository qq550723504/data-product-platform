package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestExecutionSelectEngine(t *testing.T) {
	execution, err := NewExecution(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"2026-09",
		[]InputBinding{{Name: "energy_standardized", DatasetVersionID: uuid.New()}},
		nil,
	)
	if err != nil {
		t.Fatalf("new execution: %v", err)
	}

	if err := execution.SelectEngine(" hop "); err != nil {
		t.Fatalf("select engine: %v", err)
	}
	if execution.EngineType != "HOP" {
		t.Fatalf("expected HOP, got %q", execution.EngineType)
	}

	if err := execution.Start("remote-1"); err != nil {
		t.Fatalf("start execution: %v", err)
	}
	if err := execution.SelectEngine("NATIVE"); err != ErrInvalidTransition {
		t.Fatalf("expected ErrInvalidTransition after start, got %v", err)
	}
}

func TestExecutionBeginManagedSubmissionClaimsBeforeRemoteStart(t *testing.T) {
	execution, err := NewExecution(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"2026-09",
		[]InputBinding{{Name: "energy_standardized", DatasetVersionID: uuid.New()}},
		nil,
	)
	if err != nil {
		t.Fatalf("new execution: %v", err)
	}

	if err := execution.BeginManagedSubmission(" hop "); err != nil {
		t.Fatalf("begin managed submission: %v", err)
	}
	if execution.Status != ExecutionSubmitting || execution.EngineType != "HOP" || execution.StartedAt == nil {
		t.Fatalf("unexpected submitting state: %#v", execution)
	}
	if err := execution.BeginManagedSubmission("HOP"); err != ErrInvalidTransition {
		t.Fatalf("duplicate submission claim should fail, got %v", err)
	}
	if err := execution.Start("hop-run-1"); err != nil {
		t.Fatalf("start remote execution: %v", err)
	}
	if execution.Status != ExecutionRunning || execution.EngineExecutionID != "hop-run-1" {
		t.Fatalf("unexpected running state: %#v", execution)
	}
}

func TestExecutionSelectEngineRejectsEmpty(t *testing.T) {
	execution, err := NewExecution(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"2026-09",
		[]InputBinding{{Name: "energy_standardized", DatasetVersionID: uuid.New()}},
		nil,
	)
	if err != nil {
		t.Fatalf("new execution: %v", err)
	}
	if err := execution.SelectEngine("  "); err != ErrInvalidEngineType {
		t.Fatalf("expected ErrInvalidEngineType, got %v", err)
	}
}


func TestExecutionAttachManagedSubmissionReferenceKeepsSubmitting(t *testing.T) {
	execution, err := NewExecution(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		"2026-09",
		[]InputBinding{{Name: "energy_standardized", DatasetVersionID: uuid.New()}},
		nil,
	)
	if err != nil {
		t.Fatalf("new execution: %v", err)
	}
	if err := execution.BeginManagedSubmission("HOP"); err != nil {
		t.Fatalf("begin managed submission: %v", err)
	}
	if err := execution.AttachManagedSubmissionReference(" hop-run-1 "); err != nil {
		t.Fatalf("attach managed submission reference: %v", err)
	}
	if execution.Status != ExecutionSubmitting || execution.EngineExecutionID != "hop-run-1" {
		t.Fatalf("unexpected uncertain submission state: %#v", execution)
	}
	if err := execution.AttachManagedSubmissionReference("hop-run-1"); err != nil {
		t.Fatalf("same remote identity should replay idempotently: %v", err)
	}
	if err := execution.AttachManagedSubmissionReference("hop-run-2"); err != ErrInvalidTransition {
		t.Fatalf("different remote identity must conflict, got %v", err)
	}
}
