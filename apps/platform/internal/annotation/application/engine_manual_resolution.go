package application

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
)

type ResolveManualEngineOperationCommand struct {
	WorkspaceID      uuid.UUID
	OperationID      uuid.UUID
	ExpectedRevision int64
	TargetStatus     string
	CampaignBinding  *EngineCampaignBinding
	ExternalTaskIDs  map[uuid.UUID]string
	Reason           string
	ActorID          *uuid.UUID
	TraceID          string
}

func (s *EngineService) ResolveManualOperation(
	ctx context.Context,
	cmd ResolveManualEngineOperationCommand,
) (annotationdomain.EngineOperation, error) {
	if err := s.configured(); err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	cmd.TargetStatus = strings.TrimSpace(cmd.TargetStatus)
	if cmd.WorkspaceID == uuid.Nil || cmd.OperationID == uuid.Nil ||
		cmd.ExpectedRevision < 1 || cmd.ActorID == nil || *cmd.ActorID == uuid.Nil ||
		cmd.Reason == "" {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}
	switch cmd.TargetStatus {
	case annotationdomain.EngineOperationMatched,
		annotationdomain.EngineOperationRejected,
		annotationdomain.EngineOperationConflict:
	default:
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}

	operation, err := s.repo.GetEngineOperation(ctx, cmd.OperationID)
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	if operation.WorkspaceID != cmd.WorkspaceID ||
		operation.Status != annotationdomain.EngineOperationManualResolution ||
		operation.Revision != cmd.ExpectedRevision {
		return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
	}

	resolution := engineResolution{State: EngineLookupUnknown}
	if cmd.TargetStatus == annotationdomain.EngineOperationMatched {
		resolution.State = EngineLookupMatched
		switch operation.OperationKind {
		case annotationdomain.EngineOperationEnsureCampaign:
			if cmd.CampaignBinding == nil {
				return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineBinding
			}
			resolution.CampaignBinding = cmd.CampaignBinding
		case annotationdomain.EngineOperationSubmitTasks:
			if len(cmd.ExternalTaskIDs) == 0 {
				return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineBinding
			}
			resolution.ExternalTaskIDs = cmd.ExternalTaskIDs
		default:
			return annotationdomain.EngineOperation{}, annotationdomain.ErrInvalidEngineOperation
		}
	}

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		campaign, err := s.repo.LockCampaignTx(ctx, tx, operation.CampaignID)
		if err != nil {
			return err
		}
		if campaign.WorkspaceID != operation.WorkspaceID ||
			campaign.Status != annotationdomain.CampaignActive {
			return annotationdomain.ErrInvalidCampaign
		}

		if cmd.TargetStatus == annotationdomain.EngineOperationMatched {
			if err := s.persistEngineBindings(ctx, tx, operation, resolution); err != nil {
				return err
			}
		}

		operation, err = s.repo.ResolveManualEngineOperation(
			ctx,
			tx,
			operation.ID,
			cmd.ExpectedRevision,
			cmd.TargetStatus,
		)
		if err != nil {
			return err
		}

		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  operation.WorkspaceID,
			EvidenceType: "ANNOTATION_ENGINE_MANUAL_RESOLUTION",
			Title:        "Annotation engine operation manually resolved",
			SourceType:   "CORE",
			Metadata: map[string]any{
				"operationId":  operation.ID,
				"targetStatus": operation.Status,
				"reason":       cmd.Reason,
				"resolvedAt":   time.Now().UTC(),
			},
			CreatedBy: cmd.ActorID,
		}, evidence.Relation{
			ObjectType:   "ANNOTATION_ENGINE_OPERATION",
			ObjectID:     operation.ID,
			RelationType: "MANUAL_RESOLUTION_EVIDENCE",
		}); err != nil {
			return err
		}

		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &operation.WorkspaceID,
			ActorType:   actorType(cmd.ActorID),
			ActorID:     cmd.ActorID,
			Action:      "ANNOTATION_ENGINE_MANUALLY_RESOLVED",
			ObjectType:  "ANNOTATION_ENGINE_OPERATION",
			ObjectID:    operation.ID,
			AfterState: map[string]any{
				"status": operation.Status,
				"reason": cmd.Reason,
			},
			TraceID: cmd.TraceID,
		}); err != nil {
			return err
		}

		return appendEvent(
			ctx,
			tx,
			"ANNOTATION_ENGINE_OPERATION",
			operation.ID,
			"AnnotationEngineManualResolutionApplied",
			map[string]any{
				"operationId": operation.ID,
				"status":      operation.Status,
				"reason":      cmd.Reason,
			},
		)
	})
	if err != nil {
		return annotationdomain.EngineOperation{}, err
	}
	return operation, nil
}
