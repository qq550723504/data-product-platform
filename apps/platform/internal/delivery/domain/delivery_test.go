package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestOperationLifecycleRequiresContainmentBeforeBlocked(t *testing.T) {
	now := time.Now().UTC()
	op, err := NewOperation(uuid.New(), uuid.New(), "delivery-1", "test-provider", "principal-a", "consumer-a", "delegation-a", "RESEARCH", "READ", "dataset-version", "REDEMPTION", "CREDENTIAL", now.Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if op.DelegationRef != "delegation-a" {
		t.Fatalf("delegation ref = %q, want delegation-a", op.DelegationRef)
	}
	if err := op.Transition(StatusBlocked); err != nil {
		t.Fatalf("prepared -> blocked should be allowed by initial gate, got %v", err)
	}
	if err := op.Transition(StatusFailed); !errors.Is(err, ErrTerminalOperation) {
		t.Fatalf("terminal operation must not be rewritten, got %v", err)
	}

	op, err = NewOperation(uuid.New(), uuid.New(), "delivery-2", "test-provider", "principal-a", "consumer-a", "", "RESEARCH", "READ", "dataset-version", "REDEMPTION", "CREDENTIAL", now.Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(StatusIssuancePending); err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(StatusContainmentPending); err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(StatusBlocked); err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(StatusFailed); !errors.Is(err, ErrTerminalOperation) {
		t.Fatalf("terminal operation must not be rewritten, got %v", err)
	}
}

func TestCapabilityMustBeNarrowAndWithinFreshCap(t *testing.T) {
	now := time.Now().UTC()
	op, err := NewOperation(uuid.New(), uuid.New(), "delivery-1", "test-provider", "principal-a", "consumer-a", "", "RESEARCH", "READ", "dataset-version", "REDEMPTION", "CREDENTIAL", now.Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	capability := Capability{
		Credential:                  "secret",
		CapabilityRef:               "provider-ref",
		ProviderCredentialExpiresAt: now.Add(5 * time.Minute),
		DatasetVersionID:            op.DatasetVersionID,
		ConsumerRef:                 op.EffectiveConsumerRef,
		Action:                      op.Action,
		ScopeRef:                    op.ScopeRef,
		DeliveryChannel:             op.DeliveryChannel,
		DeliveryMode:                op.DeliveryMode,
		AuthoritativelyVerified:     true,
	}
	evaluation := GateEvaluation{Allowed: true, FreshCapExpiresAt: timePtr(now.Add(10 * time.Minute))}
	if err := capability.ValidateAgainst(op, evaluation); err != nil {
		t.Fatalf("narrow capability should pass: %v", err)
	}
	capability.DeliveryMode = "DIRECT_DATA"
	if !errors.Is(capability.ValidateAgainst(op, evaluation), ErrCapabilityTooBroad) {
		t.Fatal("a capability for another delivery mode must fail closed")
	}
	capability.DeliveryMode = op.DeliveryMode
	capability.ScopeRef = "workspace-wide"
	if !errors.Is(capability.ValidateAgainst(op, evaluation), ErrCapabilityTooBroad) {
		t.Fatal("broader scope must fail closed")
	}
}

func TestCapabilityCannotCrossFreshExpiryCap(t *testing.T) {
	now := time.Now().UTC()
	op, err := NewOperation(uuid.New(), uuid.New(), "delivery-1", "test-provider", "principal-a", "consumer-a", "", "RESEARCH", "READ", "dataset-version", "REDEMPTION", "CREDENTIAL", now.Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	capability := Capability{
		Credential:                  "secret",
		CapabilityRef:               "provider-ref",
		ProviderCredentialExpiresAt: now.Add(11 * time.Minute),
		DatasetVersionID:            op.DatasetVersionID,
		ConsumerRef:                 op.EffectiveConsumerRef,
		Action:                      op.Action,
		ScopeRef:                    op.ScopeRef,
		DeliveryChannel:             op.DeliveryChannel,
		DeliveryMode:                op.DeliveryMode,
		AuthoritativelyVerified:     true,
	}
	evaluation := GateEvaluation{Allowed: true, FreshCapExpiresAt: timePtr(now.Add(10 * time.Minute))}
	if !errors.Is(capability.ValidateAgainst(op, evaluation), ErrCredentialNotIssuable) {
		t.Fatal("credential beyond fresh cap must fail closed")
	}
}

func timePtr(value time.Time) *time.Time { return &value }
