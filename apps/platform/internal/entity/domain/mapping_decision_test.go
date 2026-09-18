package domain

import (
	"strings"
	"testing"
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
	if err != nil || key != "" {
		t.Fatalf("empty key = %q, err = %v; want empty key and nil error", key, err)
	}

	if _, err := NormalizeMappingDecisionKey(strings.Repeat("x", 256)); err == nil {
		t.Fatal("overlong idempotency key was accepted")
	}
}

func TestSourceOriginValid(t *testing.T) {
	for _, origin := range []SourceOrigin{OriginUnknown, OriginMatchCandidate, OriginWorkflowAlias} {
		if !origin.Valid() {
			t.Fatalf("origin %q reported invalid", origin)
		}
	}
	if SourceOrigin("SOMETHING_ELSE").Valid() {
		t.Fatal("unknown origin reported valid")
	}
}
