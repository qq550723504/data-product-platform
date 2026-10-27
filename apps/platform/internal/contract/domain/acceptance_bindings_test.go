package domain

import (
	"testing"
	"time"
)

func bindingFixture() (AcceptanceBindingContext, []AcceptanceBindingFact) {
	at := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	c := AcceptanceBindingContext{WorkspaceID: "w", DeliveryID: "d", DatasetVersionID: "v", ConsumerRef: "receiver", ActorID: "actor1", Subject: "subject1", ContractVersionID: "s", SourceSHA256: "source", CriteriaSetHash: "criteria", StandardSupported: true, At: at}
	facts := []AcceptanceBindingFact{}
	for i, role := range AcceptanceRoles {
		id := string(rune('1' + i))
		party := "receiver"
		if i == 0 {
			party = "provider"
		}
		facts = append(facts, AcceptanceBindingFact{ID: "p" + id, WorkspaceID: "w", Kind: "REPRESENTATION", ActorID: "actor" + id, Subject: "subject" + id, PartyRef: party, ValidFrom: at.Add(-time.Hour)}, AcceptanceBindingFact{ID: "r" + id, WorkspaceID: "w", Kind: "RESPONSIBILITY", Role: role, DeliveryID: "d", DatasetVersionID: "v", RepresentationID: "p" + id, ValidFrom: at.Add(-time.Hour)})
	}
	facts = append(facts, AcceptanceBindingFact{ID: "a", WorkspaceID: "w", Kind: "APPLICABILITY", DeliveryID: "d", DatasetVersionID: "v", ContractVersionID: "s", SourceSHA256: "source", CriteriaSetHash: "criteria", ConfirmerBindingID: "r3", ValidFrom: at.Add(-time.Hour)})
	return c, facts
}
func TestAcceptanceBindingsCurrentAndTimeBoundary(t *testing.T) {
	c, fs := bindingFixture()
	roles, app, _ := EvaluateAcceptanceBindings(c, fs)
	for _, r := range roles {
		if r.Status != "ASSIGNED" {
			t.Fatalf("role %+v", r)
		}
	}
	if !roles[0].IsCurrentCaller || roles[1].IsCurrentCaller || app.Status != "MATCHED" {
		t.Fatal("caller/current mismatch")
	}
	end := c.At
	fs[2].ValidTo = &end
	roles, _, _ = EvaluateAcceptanceBindings(c, fs)
	if roles[1].Status != "INACTIVE" {
		t.Fatal("exclusive valid_to must expire representative")
	}
	fs[2].ValidTo = nil
	fs[2].ValidFrom = c.At.Add(time.Second)
	roles, _, _ = EvaluateAcceptanceBindings(c, fs)
	if roles[1].Status != "INACTIVE" {
		t.Fatal("future representative accepted")
	}
}
func TestAcceptanceBindingsDispositionAndHistory(t *testing.T) {
	c, fs := bindingFixture()
	future := c.At.Add(time.Hour)
	fs = append(fs, AcceptanceBindingFact{ID: "disposition", WorkspaceID: "w", Kind: "DISPOSITION", TargetKind: "REPRESENTATION", TargetID: "p3", Disposition: "REVOKED", EffectiveAt: &future})
	_, app, h := EvaluateAcceptanceBindings(c, fs)
	if app.Status != "MATCHED" || h[len(h)-1].State != "SCHEDULED" {
		t.Fatal("future disposition must not rewrite current facts")
	}
	c.At = future
	rs, app, h := EvaluateAcceptanceBindings(c, fs)
	if rs[2].Status != "INACTIVE" || app.Status != "MISMATCH" || h[len(h)-1].State != "EFFECTIVE" || fs[4].State != "" {
		t.Fatal("revocation or immutable input failed")
	}
}
func TestAcceptanceBindingsAmbiguityAndSeparation(t *testing.T) {
	c, fs := bindingFixture()
	copy := fs[3]
	copy.ID = "duplicate"
	fs = append(fs, copy)
	rs, _, _ := EvaluateAcceptanceBindings(c, fs)
	if rs[1].Status != "AMBIGUOUS" || rs[1].Binding != nil {
		t.Fatal("ambiguous receiver inferred latest")
	}
	for _, field := range []string{"actor", "subject"} {
		c, fs = bindingFixture()
		if field == "actor" {
			fs[6].ActorID = fs[0].ActorID
			fs[4].ActorID = fs[0].ActorID
		} else {
			fs[6].Subject = fs[0].Subject
			fs[4].Subject = fs[0].Subject
		}
		rs, a, _ := EvaluateAcceptanceBindings(c, fs)
		if rs[3].Status != "CONFLICT" || rs[2].Status != "CONFLICT" || a.Status != "MISMATCH" {
			t.Fatalf("separation %s not enforced", field)
		}
	}
}
func TestAcceptanceApplicabilityExactSourceAndConfirmer(t *testing.T) {
	for _, change := range []string{"source", "criteria", "format", "party", "confirmer"} {
		c, fs := bindingFixture()
		switch change {
		case "source":
			c.SourceSHA256 = "other"
		case "criteria":
			c.CriteriaSetHash = "other"
		case "format":
			c.StandardSupported = false
		case "party":
			fs[4].PartyRef = "foreign"
		case "confirmer":
			fs[5].ID = "replacement"
		}
		_, a, _ := EvaluateAcceptanceBindings(c, fs)
		if a.Status != "MISMATCH" {
			t.Fatalf("%s accepted %+v", change, a)
		}
	}
	c, fs := bindingFixture()
	copy := fs[len(fs)-1]
	copy.ID = "duplicate"
	fs = append(fs, copy)
	_, a, _ := EvaluateAcceptanceBindings(c, fs)
	if a.Status != "AMBIGUOUS" || a.Binding != nil {
		t.Fatal("ambiguous applicability inferred")
	}
	c.ContractVersionID = "other"
	_, a, _ = EvaluateAcceptanceBindings(c, fs)
	if a.Status != "MISSING" {
		t.Fatal("different standard inherited applicability")
	}
	c.ContractVersionID = ""
	_, a, _ = EvaluateAcceptanceBindings(c, fs)
	if a.Status != "UNSELECTED" {
		t.Fatal("standard selected implicitly")
	}
}
func TestAcceptanceBindingIdentityIncludesKind(t *testing.T) {
	c, fs := bindingFixture()
	fs[1].ID = fs[0].ID
	rs, _, _ := EvaluateAcceptanceBindings(c, fs)
	if rs[0].Status != "ASSIGNED" {
		t.Fatal("same UUID in distinct tables confused fact types")
	}
}
