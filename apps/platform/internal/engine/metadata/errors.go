package metadata

import "fmt"

type ErrorKind string

const (
	ErrorInvalidRequest ErrorKind = "INVALID_REQUEST"
	ErrorNotFound       ErrorKind = "NOT_FOUND"
	ErrorUnauthorized   ErrorKind = "UNAUTHORIZED"
	ErrorUnavailable    ErrorKind = "UNAVAILABLE"
	ErrorInvalidResponse ErrorKind = "INVALID_RESPONSE"
	ErrorRejected       ErrorKind = "REJECTED"
)

// ExternalError is the provider-neutral failure contract exposed by metadata
// adapters. Cause is retained for internal inspection through errors.As/Unwrap,
// while Error deliberately avoids leaking provider-specific response bodies.
type ExternalError struct {
	Kind       ErrorKind
	Operation  string
	StatusCode int
	Cause      error
}

func (e *ExternalError) Error() string {
	if e == nil {
		return "metadata provider operation failed"
	}
	operation := e.Operation
	if operation == "" {
		operation = "request"
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("metadata provider %s failed: kind=%s status=%d", operation, e.Kind, e.StatusCode)
	}
	return fmt.Sprintf("metadata provider %s failed: kind=%s", operation, e.Kind)
}

func (e *ExternalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewExternalError(kind ErrorKind, operation string, statusCode int, cause error) *ExternalError {
	return &ExternalError{Kind: kind, Operation: operation, StatusCode: statusCode, Cause: cause}
}
