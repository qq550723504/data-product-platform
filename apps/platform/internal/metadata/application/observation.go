package application

import (
	"context"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	"time"
)

// ObserveBinding distinguishes an immutable Core fact from a current external
// observation. Failure never rewrites the cached binding or Core gates.
func (s *Service) ObserveBinding(ctx context.Context, b metadatadomain.ResourceBinding) (string, *time.Time) {
	if s == nil || s.engine == nil {
		return "STALE", nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	asset, err := s.engine.GetAsset(ctx, b.EntityType, b.ExternalFQN)
	if err != nil || asset.ID != b.ExternalID || asset.FullyQualifiedName != b.ExternalFQN {
		return "STALE", nil
	}
	now := time.Now().UTC()
	return "OBSERVED", &now
}
