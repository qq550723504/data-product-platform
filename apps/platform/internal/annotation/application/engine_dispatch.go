package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
)

func (s *EngineService) Dispatch(
	ctx context.Context,
	operationID uuid.UUID,
	workerRef string,
	lease time.Duration,
) (EngineDispatchResult, error) {
	if err := s.configured(); err != nil {
		return EngineDispatchResult{}, err
	}
	workerRef = strings.TrimSpace(workerRef)
	if operationID == uuid.Nil || workerRef == "" || !validLease(lease) {
		return EngineDispatchResult{}, annotationdomain.ErrInvalidEngineOperation
	}

	operation, err := s.repo.GetEngineOperation(ctx, operationID)
	if err != nil {
		return EngineDispatchResult{}, err
	}
	if terminalEngineOperation(operation.Status) {
		return EngineDispatchResult{Operation: operation}, nil
	}
	if operation.Status == annotationdomain.EngineOperationSending {
		if operation.ClaimExpiresAt == nil || operation.ClaimExpiresAt.After(time.Now().UTC()) {
			return EngineDispatchResult{}, annotationinfra.ErrEngineClaimBusy
		}
		err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			var recoverErr error
			operation, recoverErr = s.repo.RecoverExpiredEngineSending(
				ctx,
				tx,
				operation.ID,
				time.Now().UTC(),
			)
			return recoverErr
		})
		if err != nil {
			return EngineDispatchResult{}, err
		}
	}

	attempt, claimed, err := s.claimEngineAttempt(ctx, operation, workerRef, lease)
	if err != nil {
		return EngineDispatchResult{}, err
	}

	resolution, remoteErr := s.invokeEngine(ctx, claimed, attempt)
	final, finalizeErr := s.finalizeEngineAttempt(
		ctx,
		claimed,
		attempt,
		workerRef,
		resolution,
		remoteErr,
	)
	if finalizeErr != nil {
		return EngineDispatchResult{}, finalizeErr
	}
	if remoteErr != nil {
		return EngineDispatchResult{Operation: final, AttemptID: attempt.ID}, remoteErr
	}
	return EngineDispatchResult{Operation: final, AttemptID: attempt.ID}, nil
}

func (s *EngineService) claimEngineAttempt(
	ctx context.Context,
	operation annotationdomain.EngineOperation,
	workerRef string,
	lease time.Duration,
) (annotationdomain.EngineAttempt, annotationdomain.EngineOperation, error) {
	var attempt annotationdomain.EngineAttempt
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		claimed, err := s.repo.ClaimEngineOperation(
			ctx,
			tx,
			operation.ID,
			operation.Revision,
			workerRef,
			time.Now().UTC().Add(lease),
		)
		if err != nil {
			return err
		}
		operation = claimed

		attemptKind := annotationdomain.EngineAttemptLookup
		if operation.Status == annotationdomain.EngineOperationPending {
			attemptKind = annotationdomain.EngineAttemptSubmit
		}
		if attemptKind == annotationdomain.EngineAttemptSubmit &&
			operation.OperationKind == annotationdomain.EngineOperationSubmitTasks {
			if s.sendGuard == nil {
				return ErrActivationGuardRequired
			}
			campaign, guardErr := s.repo.LockCampaignTx(ctx, tx, operation.CampaignID)
			if guardErr != nil {
				return guardErr
			}
			if campaign.WorkspaceID != operation.WorkspaceID ||
				campaign.Status != annotationdomain.CampaignActive {
				return annotationdomain.ErrInvalidCampaign
			}
			if guardErr := s.sendGuard.ValidateEngineSendTx(ctx, tx, campaign); guardErr != nil {
				return guardErr
			}
			if _, guardErr := evidence.Append(
				ctx,
				tx,
				evidence.Record{
					WorkspaceID:  operation.WorkspaceID,
					EvidenceType: "ANNOTATION_ENGINE_SEND_AUTHORIZED",
					Title:        "Annotation engine data send authorized",
					SourceType:   "CORE",
					Metadata: map[string]any{
						"campaignId":            operation.CampaignID,
						"operationId":           operation.ID,
						"payloadManifestSha256": operation.PayloadManifestSHA256,
						"consumerRef":           campaign.ConsumerRef,
						"purpose":               campaign.Purpose,
						"action":                "PROCESS",
						"provider":              operation.Provider,
						"providerInstance":      operation.ProviderInstanceRef,
					},
				},
				evidence.Relation{
					ObjectType:   "ANNOTATION_ENGINE_OPERATION",
					ObjectID:     operation.ID,
					RelationType: "SEND_AUTHORIZATION_EVIDENCE",
				},
			); guardErr != nil {
				return guardErr
			}
		}
		attempt, err = s.repo.StartEngineAttempt(
			ctx,
			tx,
			operation.WorkspaceID,
			operation.ID,
			attemptKind,
			time.Now().UTC(),
		)
		if err != nil {
			return err
		}

		if err := cost.AppendAnnotationEngineActivity(
			ctx,
			tx,
			cost.AnnotationEngineActivity{
				WorkspaceID: operation.WorkspaceID,
				AttemptID:   attempt.ID,
				Quantity:    1,
				Unit:        "invocation",
				PricingMode: "ACTUAL",
				Metadata: map[string]any{
					"provider":         operation.Provider,
					"providerInstance": operation.ProviderInstanceRef,
					"operationKind":    operation.OperationKind,
					"attemptKind":      attempt.AttemptKind,
					"attemptNo":        attempt.AttemptNo,
				},
				OccurredAt: attempt.StartedAt,
			},
		); err != nil {
			return err
		}

		if attemptKind == annotationdomain.EngineAttemptSubmit {
			operation, err = s.repo.TransitionEngineOperation(
				ctx,
				tx,
				operation.ID,
				operation.Revision,
				workerRef,
				annotationdomain.EngineOperationPending,
				annotationdomain.EngineOperationSending,
				false,
			)
			if err != nil {
				return err
			}
		}

		return appendEvent(
			ctx,
			tx,
			"ANNOTATION_ENGINE_OPERATION",
			operation.ID,
			"AnnotationEngineAttemptStarted",
			map[string]any{
				"operationId": operation.ID,
				"attemptId":   attempt.ID,
				"attemptNo":   attempt.AttemptNo,
				"attemptKind": attempt.AttemptKind,
				"provider":    operation.Provider,
			},
		)
	})
	return attempt, operation, err
}

func (s *EngineService) invokeEngine(
	ctx context.Context,
	operation annotationdomain.EngineOperation,
	attempt annotationdomain.EngineAttempt,
) (engineResolution, error) {
	switch operation.OperationKind {
	case annotationdomain.EngineOperationEnsureCampaign:
		manifest, err := decodeCampaignManifest(operation)
		if err != nil {
			return engineResolution{}, err
		}
		request := engineCampaignRequest(operation, manifest)
		if attempt.AttemptKind == annotationdomain.EngineAttemptSubmit {
			if _, err := s.engine.EnsureCampaignBinding(ctx, request); err != nil {
				return engineResolution{}, err
			}
			return engineResolution{
				State:         EngineLookupUnknown,
				DiagnosticRef: "project creation accepted; persisted binding verification required",
			}, nil
		}

		lookup, err := s.engine.LookupCampaignBinding(ctx, request)
		if err != nil {
			return engineResolution{}, err
		}
		return engineResolution{
			State:           lookup.State,
			CampaignBinding: lookup.Binding,
			DiagnosticRef:   lookup.DiagnosticRef,
		}, nil

	case annotationdomain.EngineOperationSubmitTasks:
		manifest, err := decodeTasksManifest(operation)
		if err != nil {
			return engineResolution{}, err
		}
		request := engineSubmitRequest(operation, manifest)
		if attempt.AttemptKind == annotationdomain.EngineAttemptSubmit {
			submission, err := s.engine.SubmitTasks(ctx, request)
			if err != nil {
				return engineResolution{}, err
			}
			return engineResolution{
				State:           submission.State,
				ExternalTaskIDs: submission.ExternalTaskIDs,
				DiagnosticRef:   submission.DiagnosticRef,
			}, nil
		}

		lookup, err := s.engine.LookupSubmission(ctx, engineLookupRequest(request))
		if err != nil {
			return engineResolution{}, err
		}
		return engineResolution{
			State:           lookup.State,
			ExternalTaskIDs: lookup.ExternalTaskIDs,
			DiagnosticRef:   lookup.DiagnosticRef,
		}, nil

	default:
		return engineResolution{}, annotationdomain.ErrInvalidEngineOperation
	}
}

func (s *EngineService) finalizeEngineAttempt(
	ctx context.Context,
	operation annotationdomain.EngineOperation,
	attempt annotationdomain.EngineAttempt,
	workerRef string,
	resolution engineResolution,
	remoteErr error,
) (annotationdomain.EngineOperation, error) {
	targetStatus, attemptOutcome := resolutionStatus(resolution.State, remoteErr, attempt.AttemptKind)
	var statusCode *int
	diagnostic := strings.TrimSpace(resolution.DiagnosticRef)
	var engineErr *AnnotationEngineError
	if errors.As(remoteErr, &engineErr) {
		if engineErr.StatusCode != 0 {
			code := engineErr.StatusCode
			statusCode = &code
		}
		if diagnostic == "" && engineErr.Kind != nil {
			diagnostic = engineErr.Kind.Error()
		}
	}

	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.AppendEngineAttemptOutcome(
			ctx,
			tx,
			annotationdomain.EngineAttemptOutcome{
				ID:                 uuid.New(),
				AttemptID:          attempt.ID,
				Outcome:            attemptOutcome,
				ProviderStatusCode: statusCode,
				DiagnosticRef:      diagnostic,
				OccurredAt:         time.Now().UTC(),
			},
		); err != nil {
			return err
		}

		if targetStatus == annotationdomain.EngineOperationMatched {
			if err := s.persistEngineBindings(ctx, tx, operation, resolution); err != nil {
				return err
			}
		}

		currentFrom := operation.Status
		if attempt.AttemptKind == annotationdomain.EngineAttemptSubmit {
			currentFrom = annotationdomain.EngineOperationSending
		}
		var err error
		operation, err = s.repo.TransitionEngineOperation(
			ctx,
			tx,
			operation.ID,
			operation.Revision,
			workerRef,
			currentFrom,
			targetStatus,
			true,
		)
		if err != nil {
			return err
		}

		if _, err := evidence.Append(
			ctx,
			tx,
			evidence.Record{
				WorkspaceID:  operation.WorkspaceID,
				EvidenceType: "ANNOTATION_ENGINE_ATTEMPT",
				Title:        "Annotation engine attempt observed",
				SourceType:   "ENGINE_ADAPTER",
				Metadata: map[string]any{
					"attemptId":       attempt.ID,
					"attemptNo":       attempt.AttemptNo,
					"attemptKind":     attempt.AttemptKind,
					"outcome":         attemptOutcome,
					"operationStatus": targetStatus,
					"provider":        operation.Provider,
					"diagnosticRef":   diagnostic,
				},
			},
			evidence.Relation{
				ObjectType:   "ANNOTATION_ENGINE_OPERATION",
				ObjectID:     operation.ID,
				RelationType: "ENGINE_ATTEMPT_EVIDENCE",
			},
		); err != nil {
			return err
		}

		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &operation.WorkspaceID,
			ActorType:   "SYSTEM",
			Action:      "ANNOTATION_ENGINE_ATTEMPT_RECORDED",
			ObjectType:  "ANNOTATION_ENGINE_OPERATION",
			ObjectID:    operation.ID,
			AfterState: map[string]any{
				"status":         targetStatus,
				"attemptId":      attempt.ID,
				"attemptOutcome": attemptOutcome,
			},
		}); err != nil {
			return err
		}

		return appendEvent(
			ctx,
			tx,
			"ANNOTATION_ENGINE_OPERATION",
			operation.ID,
			"AnnotationEngineOperationResolved",
			map[string]any{
				"operationId":    operation.ID,
				"attemptId":      attempt.ID,
				"status":         targetStatus,
				"attemptOutcome": attemptOutcome,
			},
		)
	})
	return operation, err
}
