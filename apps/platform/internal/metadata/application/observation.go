package application

import (
	"context"
	"time"

	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
)

type BindingObservation struct {
	Status     string
	ObservedAt *time.Time
	Version    any
	Governance *metadataengine.GovernanceFields
}

// ObserveBinding reads the current provider revision without rewriting the
// cached adapter binding or any Core business fact.
func (s *Service) ObserveBinding(ctx context.Context, b metadatadomain.ResourceBinding) BindingObservation {
	if s == nil || s.engine == nil {
		return BindingObservation{Status: "STALE"}
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	asset, err := s.engine.GetAsset(ctx, b.EntityType, b.ExternalFQN)
	if err != nil || asset.ID != b.ExternalID || asset.FullyQualifiedName != b.ExternalFQN {
		return BindingObservation{Status: "STALE"}
	}
	now := time.Now().UTC()
	return BindingObservation{Status: "OBSERVED", ObservedAt: &now, Version: asset.Metadata["version"], Governance: asset.Governance}
}
