package readmodel

import (
	"github.com/qq550723504/data-product-platform/apps/platform/internal/contract/domain"
	"testing"
)

func TestRelatedBindingHistoryKeepsOriginalDependenciesAndDispositions(t *testing.T) {
	f := func(kind, id string) domain.AcceptanceBindingFact {
		return domain.AcceptanceBindingFact{Kind: kind, ID: id}
	}
	rep := f("REPRESENTATION", "rep")
	old := f("RESPONSIBILITY", "old")
	old.RepresentationID = rep.ID
	replacement := f("RESPONSIBILITY", "replacement")
	replacement.RepresentationID = rep.ID
	app := f("APPLICABILITY", "app")
	app.ConfirmerBindingID = old.ID
	disposition := f("DISPOSITION", "d")
	disposition.TargetKind = old.Kind
	disposition.TargetID = old.ID
	// Related lookup must not silently replace the explicitly referenced old role.
	related := relatedBindingFacts(&app, []domain.AcceptanceBindingFact{replacement, rep, old, disposition, app})
	if len(related) != 3 {
		t.Fatalf("closure %v", related)
	}
	seen := map[string]bool{}
	for _, x := range related {
		seen[x.ID] = true
	}
	if !seen[old.ID] || !seen[rep.ID] || !seen[disposition.ID] || seen[replacement.ID] {
		t.Fatalf("wrong closure %v", related)
	}
	fromDisposition := relatedBindingFacts(&disposition, []domain.AcceptanceBindingFact{replacement, rep, old, disposition, app})
	if len(fromDisposition) != 2 {
		t.Fatalf("cycle/target closure %v", fromDisposition)
	}
	if old.State != "" || app.State != "" {
		t.Fatal("history input mutated")
	}
}
