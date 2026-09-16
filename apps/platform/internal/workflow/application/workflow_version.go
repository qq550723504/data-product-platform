package application

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
	"gopkg.in/yaml.v3"
)

type CreateWorkflowVersionCommand struct {
	WorkspaceID   uuid.UUID
	Code          string
	Name          string
	Description   string
	Version       string
	DefinitionRef string
	DefinitionYAML []byte
	ActorID       *uuid.UUID
	TraceID       string
}

type WorkflowVersionService struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewWorkflowVersionService(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *WorkflowVersionService {
	return &WorkflowVersionService{tx: tx, repo: repo}
}

func (s *WorkflowVersionService) Create(ctx context.Context, cmd CreateWorkflowVersionCommand) (domain.WorkflowVersion, error) {
	if len(cmd.DefinitionYAML) == 0 {
		return domain.WorkflowVersion{}, domain.ErrInvalidDefinition
	}
	var definition map[string]any
	if err := yaml.Unmarshal(cmd.DefinitionYAML, &definition); err != nil {
		return domain.WorkflowVersion{}, fmt.Errorf("decode workflow definition: %w", err)
	}
	workflow, err := domain.NewWorkflow(cmd.WorkspaceID, cmd.Code, cmd.Name, cmd.Description, cmd.ActorID)
	if err != nil {
		return domain.WorkflowVersion{}, err
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256(cmd.DefinitionYAML))
	var version domain.WorkflowVersion
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		storedWorkflow, err := s.repo.EnsureWorkflow(ctx, tx, workflow)
		if err != nil {
			return err
		}
		version, err = domain.NewWorkflowVersion(storedWorkflow.ID, cmd.Version, cmd.DefinitionRef, checksum, definition, cmd.ActorID)
		if err != nil {
			return err
		}
		if err := s.repo.InsertVersion(ctx, tx, version); err != nil {
			return err
		}
		event, err := outbox.NewEvent("WORKFLOW_VERSION", version.ID, "WorkflowVersionCreated", map[string]any{
			"workflowId": version.WorkflowID,
			"workflowVersionId": version.ID,
			"version": version.Version,
			"definitionRef": version.DefinitionRef,
			"definitionSha256": version.DefinitionSHA256,
		})
		if err != nil {
			return err
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID,
			ActorType: actorType(cmd.ActorID),
			ActorID: cmd.ActorID,
			Action: "WORKFLOW_VERSION_CREATED",
			ObjectType: "WORKFLOW_VERSION",
			ObjectID: version.ID,
			AfterState: map[string]any{
				"workflowId": version.WorkflowID,
				"version": version.Version,
				"definitionRef": version.DefinitionRef,
				"definitionSha256": version.DefinitionSHA256,
			},
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		return domain.WorkflowVersion{}, err
	}
	return version, nil
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
