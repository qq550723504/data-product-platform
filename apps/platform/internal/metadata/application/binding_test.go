package application

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
)

func TestBindingRejectsIncompleteProviderIdentityBeforePersistence(t *testing.T) {
	for _, asset := range []metadataengine.Asset{
		{EntityType: "TABLE", FullyQualifiedName: "db.schema.table"},
		{ID: "  ", EntityType: "TABLE", FullyQualifiedName: "db.schema.table"},
		{ID: "stable-id", FullyQualifiedName: "db.schema.table"},
		{ID: "stable-id", EntityType: "TABLE"},
	} {
		s := &Service{engine: &observationEngine{asset: asset}}
		_, err := s.BindResource(context.Background(), BindResourceCommand{ResourceID: uuid.New(), EntityType: "TABLE", FullyQualifiedName: "db.schema.table"})
		var external *metadataengine.ExternalError
		if !errors.As(err, &external) || external.Kind != metadataengine.ErrorInvalidResponse {
			t.Fatalf("incomplete asset %+v: %v, want invalid provider response", asset, err)
		}
	}
}
