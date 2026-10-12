package main

import (
	"github.com/google/uuid"
	lc "github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/application"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
	"testing"
)

func TestSurvivorInventoryRejectsMarkerReplacementUnexpectedOrDuplicate(t *testing.T) {
	w, p, inc := uuid.New(), uuid.New(), uuid.New()
	a := domain.Copy{ID: uuid.New(), WorkspaceID: w, ProviderInstanceID: p, IncarnationID: inc, Namespace: "closed", Key: "a", ProviderVersion: "exact-a", ByteSize: 27}
	b := a
	b.ID = uuid.New()
	b.Key = "b"
	b.ProviderVersion = "exact-b"
	i := lc.NamespaceInventory{WorkspaceID: w, ProviderInstanceID: p, IncarnationID: inc, Namespace: "closed", Versions: []lc.ListedVersion{{Key: "b", Version: "exact-b", ByteSize: 27}}}
	if !inventoryMatches(i, []domain.Copy{a, b}, []domain.Copy{a}) {
		t.Fatal("exact survivor rejected")
	}
	i.Versions[0].DeleteMarker = true
	if inventoryMatches(i, []domain.Copy{a, b}, []domain.Copy{a}) {
		t.Fatal("marker accepted")
	}
	i.Versions[0].DeleteMarker = false
	i.Versions[0].Version = "replacement"
	if inventoryMatches(i, []domain.Copy{a, b}, []domain.Copy{a}) {
		t.Fatal("replacement accepted")
	}
	i.Versions[0].Version = "exact-b"
	i.Versions = append(i.Versions, i.Versions[0])
	if inventoryMatches(i, []domain.Copy{a, b}, []domain.Copy{a}) {
		t.Fatal("duplicate/extra accepted")
	}
}
