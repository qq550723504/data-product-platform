package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidEngineType        = errors.New("execution engine type is required")
	ErrInvalidEngineExecutionID = errors.New("execution engine execution id is required")
)

// SelectEngine changes the runtime selected for a queued Execution.
// Engine selection is mutable only before execution starts; retries preserve
// the selected engine through Execution.Retry.
func (e *Execution) SelectEngine(engineType string) error {
	if e.Status != ExecutionQueued {
		return ErrInvalidTransition
	}
	engineType = strings.ToUpper(strings.TrimSpace(engineType))
	if engineType == "" {
		return ErrInvalidEngineType
	}
	e.EngineType = engineType
	return nil
}

// AttachManagedSubmissionReference records a durable remote identity while the
// outcome of the start request is still uncertain. It deliberately keeps the
// Execution in SUBMITTING until reconciliation confirms the remote run is
// queryable.
func (e *Execution) AttachManagedSubmissionReference(engineExecutionID string) error {
	if e.Status != ExecutionSubmitting {
		return ErrInvalidTransition
	}
	engineExecutionID = strings.TrimSpace(engineExecutionID)
	if engineExecutionID == "" {
		return ErrInvalidEngineExecutionID
	}
	if e.EngineExecutionID != "" && e.EngineExecutionID != engineExecutionID {
		return ErrInvalidTransition
	}
	e.EngineExecutionID = engineExecutionID
	return nil
}

// BeginManagedSubmission durably claims a queued Execution before the caller
// contacts a remote runtime. SUBMITTING is intentionally a Core state: a
// duplicate queue delivery must never dispatch the same Core Execution twice.
func (e *Execution) BeginManagedSubmission(engineType string) error {
	if e.Status != ExecutionQueued {
		return ErrInvalidTransition
	}
	engineType = strings.ToUpper(strings.TrimSpace(engineType))
	if engineType == "" {
		return ErrInvalidEngineType
	}
	now := time.Now().UTC()
	e.EngineType = engineType
	e.Status = ExecutionSubmitting
	e.StartedAt = &now
	return nil
}
