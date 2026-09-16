package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type InvalidateVersionCommand struct {
	VersionID uuid.UUID
	Reason    string
	ActorID   *uuid.UUID
	TraceID   string
}

type InvalidateVersionService struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewInvalidateVersionService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *InvalidateVersionService {
	return &InvalidateVersionService{tx: tx, repo: repo}
}

func (s *InvalidateVersionService) Handle(ctx context.Context, cmd InvalidateVersionCommand) (domain.DatasetVersion, error) {
	version, err := s.repo.GetVersion(ctx, cmd.VersionID)
	if err != nil {
		return domain.DatasetVersion{}, err
	}
	before := version.Status
	if err := version.Invalidate(cmd.Reason); err != nil {
		return domain.DatasetVersion{}, err
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.Invalidate(ctx, tx, version); err != nil {
			return err
		}
		event, err := outbox.NewEvent("DATASET_VERSION", version.ID, "DatasetVersionInvalidated", map[string]any{
			"datasetVersionId": version.ID,
			"datasetId":        version.DatasetID,
			"versionNo":        version.VersionNo,
			"reason":           version.InvalidationReason,
		})
		if err != nil {
			return fmt.Errorf("create invalidation event: %w", err)
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			ActorType:  actorType(cmd.ActorID),
			ActorID:    cmd.ActorID,
			Action:     "DATASET_VERSION_INVALIDATED",
			ObjectType: "DATASET_VERSION",
			ObjectID:   version.ID,
			BeforeState: map[string]any{
				"status": before,
			},
			AfterState: map[string]any{
				"status": version.Status,
				"reason": version.InvalidationReason,
			},
			Reason:  version.InvalidationReason,
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.DatasetVersion{}, err
	}
	return version, nil
}
