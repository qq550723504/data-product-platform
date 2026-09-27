package application

import (
	"testing"

	"github.com/google/uuid"
)

func TestGoldAttemptRunCommandFreezesEvaluatorIdentity(t *testing.T) {
	cmd := GoldRunCommand{
		WorkspaceID:      uuid.New(),
		DatasetVersionID: uuid.New(),
		TraceID:          "gold-attempt",
	}
	attemptID := uuid.New()
	base := goldAttemptRunCommand(cmd, attemptID)
	if base.EngineName != GoldEvaluatorName || base.engineVersion != GoldEvaluatorVersion {
		t.Fatalf("gold engine identity = %q/%q, want %q/%q",
			base.EngineName, base.engineVersion, GoldEvaluatorName, GoldEvaluatorVersion)
	}
	if base.AssessmentAttemptID != attemptID || base.RuleSetRef != GoldRuleSetRef {
		t.Fatalf("gold attempt binding = attempt %s ruleset %q", base.AssessmentAttemptID, base.RuleSetRef)
	}
}
