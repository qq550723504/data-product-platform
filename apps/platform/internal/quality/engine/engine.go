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

type ExecutionError struct {
	Code      string
	Retryable bool
}

func (e ExecutionError) Error() string {
	if e.Code == "" {
		return "quality engine execution failed"
	}
	return "quality engine execution failed: " + e.Code
}

func SanitizeError(err error) error {
	if err == nil {
		return nil
	}
	var classified ExecutionError
	if errors.As(err, &classified) {
		return classified
	}
	return ExecutionError{Code: "PROVIDER_EXECUTION_FAILED"}
}

type Engine interface {
	Descriptor() Descriptor
	Evaluate(context.Context, Request) (Result, error)
}
