package application

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
)

func (s *EngineService) persistEngineBindings(
	ctx context.Context,
	tx pgx.Tx,
	operation annotationdomain.EngineOperation,
	resolution engineResolution,
) error {
	now := time.Now().UTC()

	switch operation.OperationKind {
	case annotationdomain.EngineOperationEnsureCampaign:
		if resolution.CampaignBinding == nil {
			return annotationdomain.ErrInvalidEngineBinding
		}
		binding := resolution.CampaignBinding
		_, err := s.repo.InsertEngineCampaignBinding(
			ctx,
			tx,
			annotationdomain.EngineCampaignBinding{
				ID:                uuid.New(),
				WorkspaceID:       operation.WorkspaceID,
				CampaignID:        operation.CampaignID,
				Provider:          binding.Provider,
				ProviderInstance:  binding.ProviderInstance,
				ExternalProjectID: binding.ExternalProjectID,
				RequestID:         binding.RequestID,
				ConfigSHA256:      binding.ConfigSHA256,
				CreatedAt:         now,
			},
		)
		return err

	case annotationdomain.EngineOperationSubmitTasks:
		binding, err := s.repo.GetEngineCampaignBindingTx(ctx, tx, operation.CampaignID)
		if err != nil {
			return err
		}
		if len(resolution.ExternalTaskIDs) == 0 {
			return annotationdomain.ErrInvalidEngineBinding
		}

		coreTasks, err := s.repo.ListTasksTx(ctx, tx, operation.CampaignID)
		if err != nil {
			return err
		}
		if len(resolution.ExternalTaskIDs) != len(coreTasks) {
			return annotationdomain.ErrInvalidEngineBinding
		}
		coreTaskIDs := make(map[uuid.UUID]struct{}, len(coreTasks))
		for _, task := range coreTasks {
			coreTaskIDs[task.ID] = struct{}{}
		}

		for taskID, externalTaskID := range resolution.ExternalTaskIDs {
			if _, exists := coreTaskIDs[taskID]; !exists {
				return annotationdomain.ErrInvalidEngineBinding
			}
			if _, err := s.repo.InsertEngineTaskBinding(
				ctx,
				tx,
				annotationdomain.EngineTaskBinding{
					ID:                uuid.New(),
					WorkspaceID:       operation.WorkspaceID,
					CampaignBindingID: binding.ID,
					CampaignID:        operation.CampaignID,
					TaskID:            taskID,
					ExternalTaskID:    externalTaskID,
					CreatedAt:         now,
				},
			); err != nil {
				return err
			}
		}
		return nil

	default:
		return annotationdomain.ErrInvalidEngineOperation
	}
}

func resolutionStatus(state EngineLookupState, err error, attemptKind string) (string, string) {
	if err != nil {
		var engineErr *AnnotationEngineError
		if errors.As(err, &engineErr) {
			if attemptKind == annotationdomain.EngineAttemptSubmit &&
				engineErr.Retryable &&
				!engineErr.OutcomeUncertain {
				return annotationdomain.EngineOperationPending, annotationdomain.EngineAttemptFailedPreSend
			}
			if engineErr.OutcomeUncertain || engineErr.Retryable {
				return annotationdomain.EngineOperationUnknown, annotationdomain.EngineAttemptUnknown
			}
			if errors.Is(err, ErrAnnotationEngineInvalidRequest) ||
				errors.Is(err, ErrAnnotationEngineUnauthorized) ||
				errors.Is(err, ErrAnnotationEngineRejected) {
				return annotationdomain.EngineOperationRejected, annotationdomain.EngineAttemptRejected
			}
		}
		return annotationdomain.EngineOperationUnknown, annotationdomain.EngineAttemptUnknown
	}

	switch state {
	case EngineLookupMatched:
		return annotationdomain.EngineOperationMatched, annotationdomain.EngineAttemptSucceeded
	case EngineLookupConflict:
		return annotationdomain.EngineOperationConflict, annotationdomain.EngineAttemptConflict
	default:
		return annotationdomain.EngineOperationUnknown, annotationdomain.EngineAttemptUnknown
	}
}
