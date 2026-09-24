package application

import (
	"errors"
	"testing"

	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
)

func TestAnnotationEngineErrorDoesNotLeakProviderCause(t *testing.T) {
	err := NewAnnotationEngineError(
		ErrAnnotationEngineUnavailable,
		"submit tasks",
		true,
		503,
		errors.New("provider secret response"),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "submit tasks: annotation engine unavailable" {
		t.Fatalf("error = %q", got)
	}
	if !errors.Is(err, ErrAnnotationEngineUnavailable) {
		t.Fatal("expected error kind to unwrap")
	}
}

func TestResolutionStatusKeepsUncertainWritesUnknown(t *testing.T) {
	err := NewAnnotationEngineOutcomeError(
		ErrAnnotationEngineUnavailable,
		"submit tasks",
		true,
		true,
		503,
		errors.New("connection reset after write"),
	)
	status, outcome := resolutionStatus(EngineLookupUnknown, err, annotationdomain.EngineAttemptSubmit)
	if status != annotationdomain.EngineOperationUnknown ||
		outcome != annotationdomain.EngineAttemptUnknown {
		t.Fatalf("resolution = %s/%s, want UNKNOWN/UNKNOWN", status, outcome)
	}
}

func TestResolutionStatusMapsDefiniteRejectWithoutRetry(t *testing.T) {
	err := NewAnnotationEngineOutcomeError(
		ErrAnnotationEngineInvalidRequest,
		"submit tasks",
		false,
		false,
		400,
		errors.New("bad request"),
	)
	status, outcome := resolutionStatus(EngineLookupUnknown, err, annotationdomain.EngineAttemptSubmit)
	if status != annotationdomain.EngineOperationRejected ||
		outcome != annotationdomain.EngineAttemptRejected {
		t.Fatalf("resolution = %s/%s, want REJECTED/REJECTED", status, outcome)
	}
}

func TestResolutionStatusMapsProviderConflict(t *testing.T) {
	status, outcome := resolutionStatus(EngineLookupConflict, nil, annotationdomain.EngineAttemptLookup)
	if status != annotationdomain.EngineOperationConflict ||
		outcome != annotationdomain.EngineAttemptConflict {
		t.Fatalf("resolution = %s/%s, want CONFLICT/CONFLICT", status, outcome)
	}
}

func TestResolutionStatusRetriesDefinitePreSendFailure(t *testing.T) {
	err := NewAnnotationEngineOutcomeError(
		ErrAnnotationEngineUnavailable,
		"submit tasks",
		true,
		false,
		0,
		errors.New("credential refresh failed before send"),
	)
	status, outcome := resolutionStatus(
		EngineLookupUnknown,
		err,
		annotationdomain.EngineAttemptSubmit,
	)
	if status != annotationdomain.EngineOperationPending ||
		outcome != annotationdomain.EngineAttemptFailedPreSend {
		t.Fatalf("resolution = %s/%s, want PENDING/FAILED_PRE_SEND", status, outcome)
	}
}


func TestResolutionStatusRecordsAcceptedSubmitBeforeVerification(t *testing.T) {
	status, outcome := resolutionStatus(
		EngineLookupUnknown,
		nil,
		annotationdomain.EngineAttemptSubmit,
	)
	if status != annotationdomain.EngineOperationUnknown ||
		outcome != annotationdomain.EngineAttemptSucceeded {
		t.Fatalf("resolution = %s/%s, want UNKNOWN/SUCCEEDED", status, outcome)
	}
}
