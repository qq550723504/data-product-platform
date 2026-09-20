package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

// FailVersionCommand is the explicit operator/application command used to
// withdraw an unproduced DatasetVersion half-product. It is intentionally
// separate from generic PATCH so migration remediation and normal lifecycle
// changes use the same domain-controlled transition.
type FailVersionCommand struct {
	VersionID uuid.UUID
	Reason    string
	ActorID   *uuid.UUID
	TraceID   string
}

type FailVersionService struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewFailVersionService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *FailVersionService {
	return &FailVersionService{tx: tx, repo: repo}
}

func (s *FailVersionService) Handle(ctx context.Context, cmd FailVersionCommand) (domain.DatasetVersion, error) {
	version, err := s.repo.GetVersion(ctx, cmd.VersionID)
	if err != nil {
		return domain.DatasetVersion{}, err
	}
	before := version.Status
	if err := version.MarkFailed(); err != nil {
		return domain.DatasetVersion{}, err
	}
	workspaceID, _, err := s.repo.GetWorkspaceAndType(ctx, version.DatasetID)
	if err != nil {
		return domain.DatasetVersion{}, fmt.Errorf("resolve DatasetVersion workspace: %w", err)
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := deliveryfence.Advance(ctx, tx, workspaceID); err != nil {
			return err
		}
		if err := s.repo.Fail(ctx, tx, version); err != nil {
			return err
		}
		event, err := outbox.NewEvent("DATASET_VERSION", version.ID, "DatasetVersionFailed", map[string]any{
			"datasetVersionId": version.ID,
			"datasetId":        version.DatasetID,
			"versionNo":        version.VersionNo,
			"previousStatus":   before,
			"reason":           cmd.Reason,
		})
		if err != nil {
			return fmt.Errorf("create dataset version failure event: %w", err)
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			ActorType:  actorType(cmd.ActorID),
			ActorID:    cmd.ActorID,
			Action:     "DATASET_VERSION_FAILED",
			ObjectType: "DATASET_VERSION",
			ObjectID:   version.ID,
			BeforeState: map[string]any{
				"status": before,
			},
			AfterState: map[string]any{
				"status": version.Status,
				"reason": cmd.Reason,
			},
			Reason:  cmd.Reason,
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.DatasetVersion{}, err
	}
	return version, nil
}
