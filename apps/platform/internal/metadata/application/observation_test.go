package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
)

type observationEngine struct {
	asset   metadataengine.Asset
	err     error
	bounded bool
}

func (e *observationEngine) GetAsset(ctx context.Context, _, _ string) (metadataengine.Asset, error) {
	deadline, ok := ctx.Deadline()
	e.bounded = ok && time.Until(deadline) <= 3*time.Second
	return e.asset, e.err
}
func (*observationEngine) UpsertDataProduct(context.Context, metadataengine.GovernanceProduct) (metadataengine.ExternalEntity, error) {
	panic("observation must never write to provider")
}

func TestObservationFailureAndIdentityMismatchPreserveConfirmedBinding(t *testing.T) {
	binding := metadatadomain.ResourceBinding{ExternalID: "confirmed-id", ExternalFQN: "db.schema.table", UpdatedAt: time.Now().Add(-time.Hour), BindingMetadata: map[string]any{"version": 1.0}}
	for _, engine := range []*observationEngine{
		{err: errors.New("provider unavailable")},
		{asset: metadataengine.Asset{ID: "replacement-id", FullyQualifiedName: binding.ExternalFQN}},
		{asset: metadataengine.Asset{ID: binding.ExternalID, FullyQualifiedName: "other.table"}},
	} {
		before := binding
		s := &Service{engine: engine}
		status, at := s.ObserveBinding(context.Background(), binding)
		if status != "STALE" || at != nil || !engine.bounded || !reflect.DeepEqual(before, binding) {
			t.Fatalf("failure rewrote confirmed binding or became current: %s %v", status, at)
		}
	}
}

func TestObservationRequiresExactProviderIdentity(t *testing.T) {
	engine := &observationEngine{asset: metadataengine.Asset{ID: "confirmed-id", FullyQualifiedName: "db.schema.table"}}
	status, at := (&Service{engine: engine}).ObserveBinding(context.Background(), metadatadomain.ResourceBinding{ExternalID: "confirmed-id", ExternalFQN: "db.schema.table"})
	if status != "OBSERVED" || at == nil || !engine.bounded {
		t.Fatal("exact provider identity was not observed")
	}
}
