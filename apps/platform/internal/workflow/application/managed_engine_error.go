package application

import "fmt"

type ManagedEngineErrorKind string

const (
	ManagedEngineInvalidRequest  ManagedEngineErrorKind = "INVALID_REQUEST"
	ManagedEngineNotFound        ManagedEngineErrorKind = "NOT_FOUND"
	ManagedEngineUnauthorized    ManagedEngineErrorKind = "UNAUTHORIZED"
	ManagedEngineUnavailable     ManagedEngineErrorKind = "UNAVAILABLE"
	ManagedEngineInvalidResponse ManagedEngineErrorKind = "INVALID_RESPONSE"
	ManagedEngineRejected        ManagedEngineErrorKind = "REJECTED"
	ManagedEngineOutputInvalid   ManagedEngineErrorKind = "OUTPUT_INVALID"
)

// ManagedEngineError is the provider-neutral failure contract for remote
// processing adapters and managed-output finalization.
type ManagedEngineError struct {
	Kind       ManagedEngineErrorKind
	Operation  string
	Retryable  bool
	StatusCode int
	Cause      error
}

func (e *ManagedEngineError) Error() string {
	if e == nil {
		return "managed processing engine operation failed"
	}
	operation := e.Operation
	if operation == "" {
		operation = "operation"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("managed processing engine %s failed: kind=%s status=%d", operation, e.Kind, e.StatusCode)
	}
	return fmt.Sprintf("managed processing engine %s failed: kind=%s", operation, e.Kind)
}

func (e *ManagedEngineError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewManagedEngineError(kind ManagedEngineErrorKind, operation string, retryable bool, statusCode int, cause error) *ManagedEngineError {
	return &ManagedEngineError{
		Kind:       kind,
		Operation:  operation,
		Retryable:  retryable,
		StatusCode: statusCode,
		Cause:      cause,
	}
}
