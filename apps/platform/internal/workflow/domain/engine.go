package domain

import (
	"errors"
	"strings"
)

var ErrInvalidEngineType = errors.New("execution engine type is required")

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
