package engine

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
)

// Descriptor identifies one quality execution implementation without leaking
// provider-specific configuration into application/domain code.
type Descriptor struct {
	Name         string
	Version      string
	Capabilities []string
}

// RuleSet is the exact frozen Core rule-set input supplied to an engine.
type RuleSet struct {
	Ref     string
	Content []byte
}

// DatasetContext is the provider-neutral frozen input an engine may evaluate.
type DatasetContext struct {
	Table           tabular.Table
	Metadata        map[string]any
	ReadyAt         *time.Time
	Now             time.Time
	LineagePresent  *bool
	EvidencePresent *bool
}

type Request struct {
	AttemptID        uuid.UUID
	DatasetVersionID uuid.UUID
	RuleSet          RuleSet
	Dataset          DatasetContext
}

// Result contains only normalized Core quality semantics plus optional safe
// provider diagnostics metadata. It is not itself a QualityAssessment.
type ExecutionMetadata struct {
	ExecutionRef   string
	DurationMillis int64
}

type Result struct {
	Findings       []domain.Finding
	Metrics        map[string]any
	DiagnosticsRef string
	Execution      ExecutionMetadata
}

type ExecutionErrorCode string

const (
	ErrorProviderExecutionFailed ExecutionErrorCode = "PROVIDER_EXECUTION_FAILED"
	ErrorProviderTimeout         ExecutionErrorCode = "PROVIDER_TIMEOUT"
	ErrorProviderUnavailable     ExecutionErrorCode = "PROVIDER_UNAVAILABLE"
	ErrorProviderInvalidResponse ExecutionErrorCode = "PROVIDER_INVALID_RESPONSE"
)

type ExecutionError struct {
	code      ExecutionErrorCode
	retryable bool
}

func NewExecutionError(code ExecutionErrorCode, retryable bool) error {
	if !validExecutionErrorCode(code) {
		code = ErrorProviderExecutionFailed
		retryable = false
	}
	return ExecutionError{code: code, retryable: retryable}
}

func (e ExecutionError) Code() ExecutionErrorCode { return e.code }
func (e ExecutionError) Retryable() bool          { return e.retryable }

func (e ExecutionError) Error() string {
	code := e.code
	if !validExecutionErrorCode(code) {
		code = ErrorProviderExecutionFailed
	}
	return "quality engine execution failed: " + string(code)
}

func validExecutionErrorCode(code ExecutionErrorCode) bool {
	switch code {
	case ErrorProviderExecutionFailed, ErrorProviderTimeout, ErrorProviderUnavailable, ErrorProviderInvalidResponse:
		return true
	default:
		return false
	}
}

func SanitizeError(err error) error {
	if err == nil {
		return nil
	}
	var classified ExecutionError
	if errors.As(err, &classified) && validExecutionErrorCode(classified.code) {
		return classified
	}
	return NewExecutionError(ErrorProviderExecutionFailed, false)
}

type Engine interface {
	Descriptor() Descriptor
	Evaluate(context.Context, Request) (Result, error)
}
