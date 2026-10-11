package application

import (
	"context"
	"testing"
)

func TestPreparationNeedsTrustedActorAndStableKeyBeforeAnyFactMutation(t *testing.T) {
	s := &CertificationService{}
	if _, err := s.PrepareInputs(context.Background(), nil, PrepareCertificationInputsCommand{}); err == nil {
		t.Fatal("untrusted preparation accepted")
	}
}
