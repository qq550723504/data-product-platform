package domain

import (
	"github.com/google/uuid"
	"testing"
)

func TestVerifiedPublicationProgressAndReceiptContext(t *testing.T) {
	q := Request{Instance: "test", RequestKey: "stable-key", ExternalSubjectID: "subject", ExternalIdentifier: "release", PublicationID: uuid.New()}
	r := ProviderResult{Instance: q.Instance, RequestKey: q.RequestKey, RequestHash: q.Hash(), State: "PUBLISHED", ExternalID: q.ExternalIdentifier, RemoteID: "remote"}
	if !r.Valid(q, "SUBMIT") {
		t.Fatal("valid receipt rejected")
	}
	for _, change := range []func(*ProviderResult){
		func(r *ProviderResult) { r.Instance = "another" }, func(r *ProviderResult) { r.RequestKey = "another" }, func(r *ProviderResult) { r.RequestHash = "another" }, func(r *ProviderResult) { r.ExternalID = "another" }, func(r *ProviderResult) { r.RemoteID = "" }, func(r *ProviderResult) { r.State = "ABSENT"; r.ExternalID = ""; r.RemoteID = "" },
	} {
		copy := r
		change(&copy)
		if copy.Valid(q, "SUBMIT") {
			t.Fatalf("mismatched submit receipt accepted: %+v", copy)
		}
	}
	states, err := AdvancePublication(Submitting, "PUBLISHED")
	if err != nil || len(states) != 2 || states[0] != Registered || states[1] != Published {
		t.Fatal("provider success skipped registered history")
	}
	for _, from := range []PublicationStatus{Draft, Suspended, Withdrawn} {
		if _, err := AdvancePublication(from, "PUBLISHED"); err == nil {
			t.Fatalf("illegal progression from %s", from)
		}
	}
}
