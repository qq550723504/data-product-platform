package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/resource/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/resource/infrastructure"
)

type CreateDataResourceCommand struct {
	WorkspaceID      uuid.UUID
	ProjectID        *uuid.UUID
	Code             string
	Name             string
	Description      string
	DomainCode       string
	ResourceType     domain.ResourceType
	OwnerID          *uuid.UUID
	SensitivityLevel string
	ActorID          *uuid.UUID
	TraceID          string
}

type CreateService struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewCreateService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *CreateService {
	return &CreateService{tx: tx, repo: repo}
}

func (s *CreateService) Handle(ctx context.Context, cmd CreateDataResourceCommand) (domain.DataResource, error) {
	resource, err := domain.NewDataResource(uuid.New(), cmd.WorkspaceID, cmd.Code, cmd.Name, cmd.ResourceType, cmd.ActorID)
	if err != nil {
		return domain.DataResource{}, err
	}
	resource.ProjectID = cmd.ProjectID
	resource.Description = cmd.Description
	resource.DomainCode = cmd.DomainCode
	resource.OwnerID = cmd.OwnerID
	resource.SensitivityLevel = cmd.SensitivityLevel

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgxTx) error {
		if err := s.repo.Insert(ctx, tx, resource); err != nil {
			return err
		}

		event, err := outbox.NewEvent("DATA_RESOURCE", resource.ID, "DataResourceCreated", map[string]any{
			"resourceId":  resource.ID,
			"workspaceId": resource.WorkspaceID,
			"code":        resource.Code,
			"type":        resource.ResourceType,
		})
		if err != nil {
			return fmt.Errorf("create data resource event: %w", err)
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}

		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &resource.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "DATA_RESOURCE_CREATED",
			ObjectType:  "DATA_RESOURCE",
			ObjectID:    resource.ID,
			AfterState: map[string]any{
				"code":            resource.Code,
				"name":            resource.Name,
				"resourceType":    resource.ResourceType,
				"lifecycleStatus": resource.LifecycleStatus,
			},
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.DataResource{}, err
	}

	return resource, nil
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
