package domain

import (
	"errors"
	"github.com/google/uuid"
	"testing"
)

func TestPublicationOnlyRequestsCoreIntent(t *testing.T) {
	for _, state := range []PublicationStatus{Submitting, Registered, Published, Suspended, Withdrawn, "FAILED", ""} {
		if _, err := RequestPublication(state); !errors.Is(err, ErrState) {
			t.Fatalf("%s became requestable: %v", state, err)
		}
	}
	if next, err := RequestPublication(Draft); err != nil || next != Submitting {
		t.Fatalf("draft request: %s %v", next, err)
	}
}
func TestIdentifierRejectsUnsupportedOrAmbiguousInternalObject(t *testing.T) {
	spec := IdentifierSpec{Provider: "exchange-a", ObjectType: "PRODUCT_RELEASE", ObjectID: uuid.New(), ExternalID: "release:123", BasisEvidenceID: uuid.New()}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"CONNECTOR", "ORGANIZATION", "unknown"} {
		s := spec
		s.ObjectType = kind
		if s.Validate() == nil {
			t.Fatalf("accepted %s without owner", kind)
		}
	}
	s := spec
	id := uuid.New()
	s.SubjectBindingID = &id
	if s.Validate() == nil {
		t.Fatal("accepted conflicting object identities")
	}
	for _, provider := range []string{"NDI", "https://provider", "a/b", "", " x", "a\n"} {
		s := spec
		s.Provider = provider
		if s.Validate() == nil {
			t.Fatalf("accepted provider %q", provider)
		}
	}
	s = spec
	s.ExternalID = "id\ncredential"
	if s.Validate() == nil {
		t.Fatal("control characters accepted")
	}
}
func TestFingerprintBindsCompleteBusinessRequest(t *testing.T) {
	a := PublicationSpec{Provider: "a", IdentityBindingID: uuid.New(), ProductReleaseID: uuid.New()}
	x, _ := Fingerprint(a)
	y, _ := Fingerprint(a)
	if x != y {
		t.Fatal("unstable fingerprint")
	}
	a.Provider = "b"
	z, _ := Fingerprint(a)
	if x == z {
		t.Fatal("provider missing from fingerprint")
	}
}
