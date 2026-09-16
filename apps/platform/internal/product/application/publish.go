package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/product/domain"
)

const publishCommandType = "PUBLISH_PRODUCT_RELEASE"

type PublishReleaseCommand struct {
	ReleaseID      uuid.UUID
	IdempotencyKey string
	ActorID        *uuid.UUID
	TraceID        string
}

func (s *Service) PublishRelease(ctx context.Context, cmd PublishReleaseCommand) (domain.ProductRelease, error) {
	key, err := domain.NormalizeIdempotencyKey(cmd.IdempotencyKey)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	release, err := s.repo.GetRelease(ctx, cmd.ReleaseID)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	product, err := s.repo.GetProduct(ctx, release.ProductID)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	version, err := s.repo.GetVersion(ctx, release.ProductVersionID)
	if err != nil {
		return domain.ProductRelease{}, err
	}

	if record, found, err := s.repo.FindIdempotency(ctx, product.WorkspaceID, publishCommandType, key); err != nil {
		return domain.ProductRelease{}, err
	} else if found {
		if record.ObjectID != release.ID {
			return domain.ProductRelease{}, domain.ErrIdempotencyConflict
		}
		stored, err := s.repo.GetRelease(ctx, release.ID)
		if err != nil {
			return domain.ProductRelease{}, err
		}
		if stored.Status != domain.ReleasePublished {
			return domain.ProductRelease{}, fmt.Errorf("idempotent publish record exists but release status is %s", stored.Status)
		}
		return stored, nil
	}

	if release.Status != domain.ReleaseReady {
		return domain.ProductRelease{}, domain.ErrReleaseNotReady
	}
	readiness, err := s.Readiness(ctx, release.ID)
	if err != nil {
		return domain.ProductRelease{}, err
	}
	if readiness.Overall != "READY" {
		return domain.ProductRelease{}, fmt.Errorf("%w: blockers=%v", domain.ErrReleaseNotReady, readiness.Blockers)
	}

	manifest, items, err := s.repo.BuildReleaseEvidenceManifest(ctx, release, product, version)
	if err != nil {
		return domain.ProductRelease{}, err
	}

	var snapshot evidence.Snapshot
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		created, err := evidence.CreateSnapshot(
			ctx,
			tx,
			product.WorkspaceID,
			"PRODUCT_RELEASE",
			release.ID,
			manifest,
			items,
			cmd.ActorID,
		)
		if err != nil {
			return err
		}
		snapshot = created
		if err := release.Publish(snapshot.ID, cmd.ActorID); err != nil {
			return err
		}
		if err := s.repo.PublishRelease(ctx, tx, release, snapshot.ID, cmd.ActorID); err != nil {
			return err
		}
		if err := s.repo.InsertIdempotency(ctx, tx, product.WorkspaceID, publishCommandType, key, release.ID, &snapshot.ID); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "PRODUCT_RELEASE", release.ID, "ProductReleased", map[string]any{
			"releaseId":          release.ID,
			"productId":          release.ProductID,
			"productVersionId":   release.ProductVersionID,
			"evidenceSnapshotId": snapshot.ID,
			"evidenceRootHash":   snapshot.RootHash,
			"status":             release.Status,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &product.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "PRODUCT_RELEASE_PUBLISHED",
			ObjectType:  "PRODUCT_RELEASE",
			ObjectID:    release.ID,
			BeforeState: map[string]any{"status": domain.ReleaseReady},
			AfterState: map[string]any{
				"status":             release.Status,
				"evidenceSnapshotId": snapshot.ID,
				"evidenceRootHash":   snapshot.RootHash,
			},
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.ProductRelease{}, err
	}
	return s.repo.GetRelease(ctx, release.ID)
}
