package domain

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEnsureMappingDecisionAllowed(t *testing.T) {
	confirmed := MappingConfirmed
	auto := MappingAutoMatched
	conflict := MappingConflict

	t.Run("automatic matching cannot replace a human confirmation", func(t *testing.T) {
		if err := EnsureMappingDecisionAllowed(&confirmed, auto); err != ErrMappingConfirmedImmutable {
			t.Fatalf("err = %v, want ErrMappingConfirmedImmutable", err)
		}
	})

	t.Run("human confirmation may replace automatic matching", func(t *testing.T) {
		if err := EnsureMappingDecisionAllowed(&auto, confirmed); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("automatic rematch without a prior confirmation is allowed", func(t *testing.T) {
		if err := EnsureMappingDecisionAllowed(&auto, auto); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if err := EnsureMappingDecisionAllowed(&conflict, auto); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("first decision is allowed", func(t *testing.T) {
		if err := EnsureMappingDecisionAllowed(nil, auto); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})
}

func TestNormalizeMappingDecisionKey(t *testing.T) {
	key, err := NormalizeMappingDecisionKey("  confirm:candidate  ")
	if err != nil {
		t.Fatalf("normalize key: %v", err)
	}
	if key != "confirm:candidate" {
		t.Fatalf("key = %q, want trimmed value", key)
	}

	key, err = NormalizeMappingDecisionKey("   ")
	if err != ErrMappingDecisionKeyRequired || key != "" {
		t.Fatalf("empty key = %q, err = %v; want ErrMappingDecisionKeyRequired", key, err)
	}

	if _, err := NormalizeMappingDecisionKey(strings.Repeat("x", 256)); err == nil {
		t.Fatal("overlong idempotency key was accepted")
	}
}

func TestSourceOriginValid(t *testing.T) {
	for _, origin := range []SourceOrigin{OriginMatchCandidate, OriginManualReview, OriginWorkflowAlias} {
		if !origin.Valid() {
			t.Fatalf("origin %q reported invalid", origin)
		}
	}
	if SourceOrigin("SOMETHING_ELSE").Valid() {
		t.Fatal("unknown origin reported valid")
	}
}

func TestEnsureMappingDecisionExpectation(t *testing.T) {
	current := uuid.New()
	other := uuid.New()

	t.Run("first confirmation without a token is safe", func(t *testing.T) {
		if err := EnsureMappingDecisionExpectation(nil, true, nil, MappingConfirmed); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if err := EnsureMappingDecisionExpectation(nil, false, nil, MappingConfirmed); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("replacing a current decision without the token is rejected explicitly", func(t *testing.T) {
		if err := EnsureMappingDecisionExpectation(&current, false, nil, MappingConfirmed); err != ErrMappingDecisionExpectationRequired {
			t.Fatalf("err = %v, want ErrMappingDecisionExpectationRequired", err)
		}
	})

	t.Run("a stale observed decision is a concurrency conflict", func(t *testing.T) {
		if err := EnsureMappingDecisionExpectation(&current, true, &other, MappingConfirmed); err != ErrMappingDecisionConflict {
			t.Fatalf("err = %v, want ErrMappingDecisionConflict", err)
		}
		if err := EnsureMappingDecisionExpectation(&current, true, nil, MappingConfirmed); err != ErrMappingDecisionConflict {
			t.Fatalf("err = %v, want ErrMappingDecisionConflict", err)
		}
	})

	t.Run("an explicit matching expectation is accepted", func(t *testing.T) {
		if err := EnsureMappingDecisionExpectation(&current, true, &current, MappingConfirmed); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("automatic decisions keep the generic optimistic check only", func(t *testing.T) {
		if err := EnsureMappingDecisionExpectation(&current, false, nil, MappingAutoMatched); err != nil {
			t.Fatalf("err = %v, want nil (automatic path is governed by EnsureMappingDecisionAllowed)", err)
		}
		if err := EnsureMappingDecisionExpectation(&current, true, &other, MappingAutoMatched); err != ErrMappingDecisionConflict {
			t.Fatalf("err = %v, want ErrMappingDecisionConflict", err)
		}
	})
}

func TestMappingDecisionMatchesRequest(t *testing.T) {
	reviewer := uuid.New()
	jobID := uuid.New()
	candidateID := uuid.New()
	request := func() (MappingDecision, MappingDecisionCommand) {
		recordedAt := time.Now().UTC()
		cmd := MappingDecisionCommand{
			Mapping: EntityMapping{
				ID: uuid.New(), WorkspaceID: uuid.New(), EntityID: uuid.New(),
				SourceType: "CSV", SourceRef: "enterprise.csv", SourceKey: "ENT-1", SourceName: "Acme",
				MatchMethod: "ALIAS", MatchRuleID: "R-1", MatchPolicyVersion: "v1",
				MatchEngineName: "RULES", MatchEngineVersion: "1", MatchModelVersion: "m1",
				Confidence: 0.9, Status: MappingConfirmed, ReviewedBy: &reviewer, ReviewerReason: "reviewed",
			},
			SourceOrigin:      OriginMatchCandidate,
			SourceJobID:       &jobID,
			SourceCandidateID: &candidateID,
		}
		existing := MappingDecision{
			ID: uuid.New(), MappingID: uuid.New(),
			EntityID:           cmd.Mapping.EntityID,
			SourceType:         cmd.Mapping.SourceType,
			SourceRef:          cmd.Mapping.SourceRef,
			SourceKey:          cmd.Mapping.SourceKey,
			SourceName:         cmd.Mapping.SourceName,
			MatchMethod:        cmd.Mapping.MatchMethod,
			MatchRuleID:        cmd.Mapping.MatchRuleID,
			MatchPolicyVersion: cmd.Mapping.MatchPolicyVersion,
			MatchEngineName:    cmd.Mapping.MatchEngineName,
			MatchEngineVersion: cmd.Mapping.MatchEngineVersion,
			MatchModelVersion:  cmd.Mapping.MatchModelVersion,
			Confidence:         cmd.Mapping.Confidence,
			Status:             cmd.Mapping.Status,
			ReviewedBy:         cmd.Mapping.ReviewedBy,
			ReviewerReason:     cmd.Mapping.ReviewerReason,
			EvidenceID:         ptrUUID(uuid.New()),
			DecidedAt:          recordedAt,
			IdempotencyKey:     "confirm:1",
			SourceOrigin:       OriginMatchCandidate,
			SourceJobID:        &jobID,
			SourceCandidateID:  &candidateID,
		}
		return existing, cmd
	}

	existing, cmd := request()
	if !MappingDecisionMatchesRequest(existing, cmd) {
		t.Fatal("identical request semantics were reported as different")
	}

	// Random identifiers and timestamps must not break idempotent replay.
	different := cmd
	different.Mapping.ID = uuid.New()
	different.Mapping.CreatedAt = time.Now().Add(time.Minute)
	different.IdempotencyKey = "confirm:1"
	if !MappingDecisionMatchesRequest(existing, different) {
		t.Fatal("random ids/timestamps changed the request semantics")
	}

	differentEntity := cmd
	differentEntity.Mapping.EntityID = uuid.New()
	if MappingDecisionMatchesRequest(existing, differentEntity) {
		t.Fatal("a different target entity was treated as a replay")
	}

	differentReason := cmd
	differentReason.Mapping.ReviewerReason = "another reason"
	if MappingDecisionMatchesRequest(existing, differentReason) {
		t.Fatal("a different reviewer reason was treated as a replay")
	}

	differentReviewer := cmd
	differentReviewer.Mapping.ReviewedBy = ptrUUID(uuid.New())
	if MappingDecisionMatchesRequest(existing, differentReviewer) {
		t.Fatal("a different reviewer identity was treated as a replay")
	}

	differentSource := cmd
	differentSource.Mapping.SourceKey = "ENT-2"
	if MappingDecisionMatchesRequest(existing, differentSource) {
		t.Fatal("a different source key was treated as a replay")
	}

	differentAssociation := cmd
	differentAssociation.SourceCandidateID = ptrUUID(uuid.New())
	if MappingDecisionMatchesRequest(existing, differentAssociation) {
		t.Fatal("a different source candidate association was treated as a replay")
	}

}

func ptrUUID(id uuid.UUID) *uuid.UUID { return &id }
