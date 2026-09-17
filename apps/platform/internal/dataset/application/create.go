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
	resourceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

type CreateDatasetCommand struct {
	WorkspaceID      uuid.UUID
	ProjectID        *uuid.UUID
	Code             string
	Name             string
	Description      string
	DatasetType      domain.DatasetType
	SourceResourceID *uuid.UUID
	OwnerID          *uuid.UUID
	ActorID          *uuid.UUID
	TraceID          string
}

type CreateDatasetService struct {
	tx        *transaction.Manager
	repo      *infrastructure.PostgresRepository
	resources *resourceinfra.PostgresRepository
}

// NewCreateDatasetService accepts the explicit resource repository used by newer
// callers, while retaining the two-argument constructor used by older clients.
// Omission or nil NEVER disables ownership checks: both use the real repository.
func NewCreateDatasetService(tx *transaction.Manager, repo *infrastructure.PostgresRepository, resources ...*resourceinfra.PostgresRepository) *CreateDatasetService {
	if len(resources) > 1 {
		panic("NewCreateDatasetService accepts at most one resource repository")
	}
	resourceRepo := resourceinfra.NewPostgresRepository()
	if len(resources) == 1 && resources[0] != nil {
		resourceRepo = resources[0]
	}
	return &CreateDatasetService{tx: tx, repo: repo, resources: resourceRepo}
}

func (s *CreateDatasetService) Handle(ctx context.Context, cmd CreateDatasetCommand) (domain.Dataset, error) {
	dataset, err := domain.NewDataset(uuid.New(), cmd.WorkspaceID, cmd.Code, cmd.Name, cmd.DatasetType, cmd.SourceResourceID, cmd.ActorID)
	if err != nil {
		return domain.Dataset{}, err
	}
	dataset.ProjectID = cmd.ProjectID
	dataset.Description = cmd.Description
	dataset.OwnerID = cmd.OwnerID

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// A DataResource foreign key only proves the resource exists. Verify that
		// it belongs to this workspace so a dataset cannot claim another
		// tenant's resource as its source.
		if dataset.SourceResourceID != nil {
			sourceWorkspace, err := s.resources.WorkspaceOf(ctx, tx, *dataset.SourceResourceID)
			if err != nil {
				return fmt.Errorf("resolve source resource %s: %w", *dataset.SourceResourceID, err)
			}
			if sourceWorkspace != dataset.WorkspaceID {
				return fmt.Errorf("%w: source resource %s belongs to workspace %s", domain.ErrSourceResourceWorkspace, *dataset.SourceResourceID, sourceWorkspace)
			}
		}
		if err := s.repo.InsertDataset(ctx, tx, dataset); err != nil {
			return err
		}
		event, err := outbox.NewEvent("DATASET", dataset.ID, "DatasetCreated", map[string]any{
			"datasetId":   dataset.ID,
			"workspaceId": dataset.WorkspaceID,
			"code":        dataset.Code,
			"datasetType": dataset.DatasetType,
		})
		if err != nil {
			return fmt.Errorf("create dataset event: %w", err)
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &dataset.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "DATASET_CREATED",
			ObjectType:  "DATASET",
			ObjectID:    dataset.ID,
			AfterState: map[string]any{
				"code":        dataset.Code,
				"name":        dataset.Name,
				"datasetType": dataset.DatasetType,
			},
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.Dataset{}, err
	}
	return dataset, nil
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
