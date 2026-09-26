package workflowqueue

import (
	"errors"
	"testing"

	workflowapp "github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/application"
)

func TestManagedSubmissionOutcomeUnknown(t *testing.T) {
	unknown := workflowapp.NewManagedEngineError(
		workflowapp.ManagedEngineOutcomeUnknown,
		"submit",
		true,
		0,
		errors.New("response lost"),
	)
	if !managedSubmissionOutcomeUnknown(unknown) {
		t.Fatal("OUTCOME_UNKNOWN submit error must leave execution in SUBMITTING")
	}

	definite := workflowapp.NewManagedEngineError(
		workflowapp.ManagedEngineRejected,
		"submit",
		false,
		400,
		errors.New("provider rejected request"),
	)
	if managedSubmissionOutcomeUnknown(definite) {
		t.Fatal("definite provider rejection must not be treated as outcome unknown")
	}

	if managedSubmissionOutcomeUnknown(errors.New("plain error")) {
		t.Fatal("unclassified error must not silently become outcome unknown at queue boundary")
	}
}
