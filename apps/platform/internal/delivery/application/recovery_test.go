package application

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/delivery/domain"
)

func TestMatchesIssuedCapabilityRequiresTheOriginalCapability(t *testing.T) {
	now := time.Now().UTC()
	op := domain.Operation{
		ID:                          uuid.New(),
		DatasetVersionID:            uuid.New(),
		Status:                      domain.StatusIssued,
		EffectiveConsumerRef:        "consumer-a",
		CredentialRef:               "provider-capability-ref",
		CredentialHash:              hashSecret("secret-a"),
		ProviderCredentialExpiresAt: &now,
	}
	capability := domain.Capability{Credential: "secret-a", CapabilityRef: "provider-capability-ref", ProviderCredentialExpiresAt: now}
	if !matchesIssuedCapability(op, capability) {
		t.Fatal("the originally issued capability should match")
	}

	capability.Credential = "secret-b"
	if matchesIssuedCapability(op, capability) {
		t.Fatal("a different credential must not match the issued fact")
	}

	capability.Credential = "secret-a"
	capability.CapabilityHash = hashSecret("secret-b")
	if matchesIssuedCapability(op, capability) {
		t.Fatal("a provider-supplied hash that disagrees with the recovered secret must not match")
	}

	capability.CapabilityHash = ""
	capability.Credential = "secret-a"
	capability.CapabilityRef = "different-ref"
	if matchesIssuedCapability(op, capability) {
		t.Fatal("a different capability ref must not match the issued fact")
	}

	capability.CapabilityRef = "provider-capability-ref"
	capability.ProviderCredentialExpiresAt = now.Add(time.Minute)
	if matchesIssuedCapability(op, capability) {
		t.Fatal("a recovered capability with extended expiry must not match the issued fact")
	}
}
