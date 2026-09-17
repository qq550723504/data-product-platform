package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	metadataengine "github.com/qq550723504/data-product-platform/apps/platform/internal/engine/metadata"
	metadatadomain "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/domain"
	metadatainfra "github.com/qq550723504/data-product-platform/apps/platform/internal/metadata/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	productdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
	productinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/product/infrastructure"
)

type Service struct {
	provider  metadatadomain.Provider
	domainFQN string
	tx        *transaction.Manager
	repo      *metadatainfra.PostgresRepository
	products  *productinfra.PostgresRepository
	engine    metadataengine.Engine
}

func NewService(provider metadatadomain.Provider, domainFQN string, tx *transaction.Manager, repo *metadatainfra.PostgresRepository, products *productinfra.PostgresRepository, engine metadataengine.Engine) *Service {
	return &Service{
		provider:  provider,
		domainFQN: strings.TrimSpace(domainFQN),
		tx:        tx,
		repo:      repo,
		products:  products,
		engine:    engine,
	}
}

type BindResourceCommand struct {
	ResourceID         uuid.UUID
	EntityType         string
	FullyQualifiedName string
	Primary            bool
}

func (s *Service) BindResource(ctx context.Context, cmd BindResourceCommand) (metadatadomain.ResourceBinding, error) {
	if cmd.ResourceID == uuid.Nil || strings.TrimSpace(cmd.EntityType) == "" || strings.TrimSpace(cmd.FullyQualifiedName) == "" {
		return metadatadomain.ResourceBinding{}, fmt.Errorf("resource ID, entity type and fully qualified name are required")
	}
	asset, err := s.engine.GetAsset(ctx, cmd.EntityType, cmd.FullyQualifiedName)
	if err != nil {
		return metadatadomain.ResourceBinding{}, err
	}
	binding := metadatadomain.ResourceBinding{
		ID:              uuid.New(),
		ResourceID:      cmd.ResourceID,
		Provider:        s.provider,
		EntityType:      asset.EntityType,
		ExternalID:      asset.ID,
		ExternalFQN:     asset.FullyQualifiedName,
		BindingMetadata: asset.Metadata,
		IsPrimary:       cmd.Primary,
	}
	if err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return s.repo.UpsertBinding(ctx, tx, binding)
	}); err != nil {
		return metadatadomain.ResourceBinding{}, err
	}
	return binding, nil
}

func (s *Service) HandleOutboxEvent(ctx context.Context, event outbox.PublishedEvent) error {
	if event.EventType != "ProductReleased" {
		return nil
	}
	return s.ProjectProductRelease(ctx, event.ID, event.AggregateID)
}

func (s *Service) ProjectProductRelease(ctx context.Context, sourceEventID, releaseID uuid.UUID) error {
	if s.domainFQN == "" {
		return fmt.Errorf("metadata projection domain is required")
	}
	if stored, err := s.repo.GetProjection(ctx, s.provider, "PRODUCT_RELEASE", releaseID); err == nil {
		if stored.Status == metadatadomain.ProjectionSucceeded {
			return nil
		}
	} else if !errors.Is(err, metadatainfra.ErrNotFound) {
		return err
	}

	release, err := s.products.GetRelease(ctx, releaseID)
	if err != nil {
		return err
	}
	if release.Status != productdomain.ReleasePublished {
		return fmt.Errorf("ProductRelease %s must be PUBLISHED before metadata projection, got %s", release.ID, release.Status)
	}
	product, err := s.products.GetProduct(ctx, release.ProductID)
	if err != nil {
		return err
	}
	version, err := s.products.GetVersion(ctx, release.ProductVersionID)
	if err != nil {
		return err
	}

	projection := metadatadomain.GovernanceProjection{
		ID:            uuid.New(),
		WorkspaceID:   product.WorkspaceID,
		Provider:      s.provider,
		ObjectType:    "PRODUCT_RELEASE",
		ObjectID:      release.ID,
		SourceEventID: &sourceEventID,
		Metadata: map[string]any{
			"productId":        product.ID,
			"productCode":      product.Code,
			"releaseNo":        release.ReleaseNo,
			"productVersion":   version.Semver(),
			"evidenceSnapshot": release.EvidenceSnapshotID,
		},
	}
	if err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return s.repo.BeginProjection(ctx, tx, projection)
	}); err != nil {
		return err
	}

	external, projectErr := s.engine.UpsertDataProduct(ctx, metadataengine.GovernanceProduct{
		Name:        product.Code,
		DisplayName: product.Name,
		Description: product.Description,
		Domain:      s.domainFQN,
		Metadata: map[string]any{
			"releaseNo":      release.ReleaseNo,
			"productVersion": version.Semver(),
		},
	})
	if projectErr != nil {
		markErr := s.tx.Do(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			return s.repo.MarkProjectionFailed(ctx, tx, s.provider, "PRODUCT_RELEASE", release.ID, projectErr)
		})
		if markErr != nil {
			return fmt.Errorf("metadata projection failed: %v; persist projection failure: %w", projectErr, markErr)
		}
		return projectErr
	}

	return s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return s.repo.MarkProjectionSucceeded(ctx, tx, s.provider, "PRODUCT_RELEASE", release.ID,
			external.ID, external.FullyQualifiedName, map[string]any{
				"productId":        product.ID,
				"productCode":      product.Code,
				"releaseNo":        release.ReleaseNo,
				"productVersion":   version.Semver(),
				"externalMetadata": external.Metadata,
			})
	})
}
