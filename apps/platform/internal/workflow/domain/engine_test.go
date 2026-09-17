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
