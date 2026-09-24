package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type AnnotationResultRecorder interface {
	RecordAnnotationResult(context.Context, RecordResultCommand) (annotationdomain.Result, error)
}

type EngineResultReconciler struct {
	tx       *transaction.Manager
	repo     *annotationinfra.Repository
	engine   AnnotationEnginePort
	recorder AnnotationResultRecorder
}

func NewEngineResultReconciler(
	tx *transaction.Manager,
	repo *annotationinfra.Repository,
	engine AnnotationEnginePort,
	recorder AnnotationResultRecorder,
) *EngineResultReconciler {
	return &EngineResultReconciler{tx: tx, repo: repo, engine: engine, recorder: recorder}
}

func (r *EngineResultReconciler) ReconcileCampaign(ctx context.Context, campaignID uuid.UUID) error {
	if r == nil || r.tx == nil || r.repo == nil || r.engine == nil || r.recorder == nil {
		return errors.New("annotation result reconciler is not configured")
	}
	if campaignID == uuid.Nil {
		return annotationdomain.ErrInvalidCampaign
	}

	campaign, err := r.repo.GetCampaign(ctx, campaignID)
	if err != nil {
		return err
	}
	if campaign.Status == annotationdomain.CampaignSealed || campaign.Status == annotationdomain.CampaignCancelled {
		return nil
	}
	if campaign.Status != annotationdomain.CampaignActive {
		return annotationdomain.ErrInvalidCampaign
	}

	binding, err := r.repo.GetEngineCampaignBinding(ctx, campaignID)
	if err != nil {
		return err
	}
	if binding.Provider != r.engine.Provider() || binding.ProviderInstance != r.engine.InstanceRef() {
		return annotationinfra.ErrEngineBindingConflict
	}
	taskBindings, err := r.repo.ListEngineTaskBindings(ctx, campaignID)
	if err != nil {
		return err
	}
	coreTasks, err := r.repo.ListTasks(ctx, campaignID)
	if err != nil {
		return err
	}
	if len(taskBindings) != len(coreTasks) || len(taskBindings) == 0 {
		return annotationinfra.ErrEngineBindingConflict
	}

	byTask := make(map[uuid.UUID]annotationdomain.EngineTaskBinding, len(taskBindings))
	for _, taskBinding := range taskBindings {
		if taskBinding.CampaignBindingID != binding.ID || taskBinding.WorkspaceID != campaign.WorkspaceID {
			return annotationinfra.ErrEngineBindingConflict
		}
		byTask[taskBinding.TaskID] = taskBinding
	}

	operation, err := r.repo.GetMatchedTaskSubmissionOperation(
		ctx, campaignID, binding.Provider, binding.ProviderInstance,
	)
	if err != nil {
		return err
	}

	cursor := EngineResultCursor{}
	for {
		attempt, err := r.startFetchAttempt(ctx, operation)
		if err != nil {
			return err
		}
		page, fetchErr := r.engine.FetchResults(ctx, EngineCampaignBinding{
			Provider:          binding.Provider,
			ProviderInstance:  binding.ProviderInstance,
			ExternalProjectID: binding.ExternalProjectID,
			RequestID:         binding.RequestID,
			ConfigSHA256:      binding.ConfigSHA256,
		}, cursor)
		if err := r.finishFetchAttempt(ctx, operation, attempt, fetchErr); err != nil {
			return err
		}
		if fetchErr != nil {
			return fetchErr
		}

		for _, observation := range page.Results {
			taskBinding, ok := byTask[observation.TaskID]
			if !ok || strings.TrimSpace(taskBinding.ExternalTaskID) != strings.TrimSpace(observation.ExternalTaskID) {
				return annotationinfra.ErrEngineBindingConflict
			}
			if !observation.ProviderSubmitted || observation.ProviderCancelled {
				continue
			}
			task, err := r.repo.GetTask(ctx, observation.TaskID)
			if err != nil {
				return err
			}
			if task.WorkspaceID != campaign.WorkspaceID || task.CampaignID != campaignID {
				return annotationinfra.ErrEngineBindingConflict
			}
			actorBinding, err := r.repo.ResolveEngineActorBinding(
				ctx,
				campaign.WorkspaceID,
				binding.Provider,
				binding.ProviderInstance,
				strings.TrimSpace(observation.ExternalAuthorRef),
			)
			if err != nil {
				return err
			}
			if strings.TrimSpace(actorBinding.CoreActorRef) != strings.TrimSpace(task.PrimaryAnnotatorRef) {
				return fmt.Errorf("annotation provider actor binding does not match primary annotator")
			}
			observationKey := providerObservationKey(r.engine.InstanceRef(), binding.ExternalProjectID, observation)
			if _, err := r.recorder.RecordAnnotationResult(ctx, RecordResultCommand{
				WorkspaceID:            campaign.WorkspaceID,
				CampaignID:             campaignID,
				TaskID:                 observation.TaskID,
				ExpectedTaskRevision:   task.Revision,
				AuthorRef:              strings.TrimSpace(actorBinding.CoreActorRef),
				ProviderBindingRef:     binding.ID.String(),
				ExternalTaskID:         strings.TrimSpace(observation.ExternalTaskID),
				ExternalAnnotationID:   strings.TrimSpace(observation.ExternalAnnotationID),
				ExternalRevision:       strings.TrimSpace(observation.ExternalRevision),
				ObservationKey:         observationKey,
				CanonicalPayload:       append([]byte(nil), observation.CanonicalPayload...),
				CanonicalPayloadSHA256: strings.TrimSpace(observation.CanonicalPayloadSHA256),
				NormalizerVersion:      strings.TrimSpace(observation.NormalizerVersion),
				TraceID:                attempt.ID.String(),
			}); err != nil {
				return err
			}
		}

		if page.NextCursor == nil {
			return nil
		}
		cursor = *page.NextCursor
	}
}

func (r *EngineResultReconciler) startFetchAttempt(
	ctx context.Context,
	operation annotationdomain.EngineOperation,
) (annotationdomain.EngineAttempt, error) {
	var attempt annotationdomain.EngineAttempt
	err := r.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		attempt, err = r.repo.StartEngineAttempt(
			ctx, tx, operation.WorkspaceID, operation.ID, annotationdomain.EngineAttemptFetch, time.Now().UTC(),
		)
		if err != nil {
			return err
		}
		if err := cost.AppendAnnotationEngineActivity(ctx, tx, cost.AnnotationEngineActivity{
			WorkspaceID: operation.WorkspaceID,
			AttemptID:   attempt.ID,
			Quantity:    1,
			Unit:        "invocation",
			PricingMode: "ACTUAL",
			Metadata: map[string]any{
				"provider":      operation.Provider,
				"operationKind": operation.OperationKind,
				"attemptKind":   attempt.AttemptKind,
				"attemptNo":     attempt.AttemptNo,
			},
			OccurredAt: attempt.StartedAt,
		}); err != nil {
			return err
		}
		return appendEvent(ctx, tx, "ANNOTATION_ENGINE_OPERATION", operation.ID, "AnnotationEngineAttemptStarted", map[string]any{
			"operationId": operation.ID,
			"attemptId":   attempt.ID,
			"attemptNo":   attempt.AttemptNo,
			"attemptKind": attempt.AttemptKind,
		})
	})
	return attempt, err
}

func (r *EngineResultReconciler) finishFetchAttempt(
	ctx context.Context,
	operation annotationdomain.EngineOperation,
	attempt annotationdomain.EngineAttempt,
	remoteErr error,
) error {
	outcome := annotationdomain.EngineAttemptSucceeded
	var statusCode *int
	diagnostic := ""
	if remoteErr != nil {
		outcome = annotationdomain.EngineAttemptUnknown
		var engineErr *AnnotationEngineError
		if errors.As(remoteErr, &engineErr) {
			if engineErr.StatusCode != 0 {
				code := engineErr.StatusCode
				statusCode = &code
			}
			if engineErr.Kind != nil {
				diagnostic = engineErr.Kind.Error()
			}
			if !engineErr.Retryable && !engineErr.OutcomeUncertain {
				outcome = annotationdomain.EngineAttemptRejected
			}
		}
	}
	return r.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := r.repo.AppendEngineAttemptOutcome(ctx, tx, annotationdomain.EngineAttemptOutcome{
			ID:                 uuid.New(),
			AttemptID:          attempt.ID,
			Outcome:            outcome,
			ProviderStatusCode: statusCode,
			DiagnosticRef:      diagnostic,
			OccurredAt:         time.Now().UTC(),
		}); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  operation.WorkspaceID,
			EvidenceType: "ANNOTATION_ENGINE_RESULT_FETCH",
			Title:        "Annotation engine result fetch observed",
			SourceType:   "ENGINE_ADAPTER",
			Metadata: map[string]any{
				"attemptId": attempt.ID,
				"outcome":   outcome,
				"provider":  operation.Provider,
			},
		}, evidence.Relation{
			ObjectType:   "ANNOTATION_ENGINE_OPERATION",
			ObjectID:     operation.ID,
			RelationType: "RESULT_FETCH_EVIDENCE",
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &operation.WorkspaceID,
			ActorType:   "SYSTEM",
			Action:      "ANNOTATION_ENGINE_RESULT_FETCHED",
			ObjectType:  "ANNOTATION_ENGINE_OPERATION",
			ObjectID:    operation.ID,
			AfterState:  map[string]any{"attemptId": attempt.ID, "outcome": outcome},
		})
	})
}

func providerObservationKey(
	providerInstance, externalProjectID string,
	observation EngineResultObservation,
) string {
	identity := strings.Join([]string{
		strings.TrimSpace(providerInstance),
		strings.TrimSpace(externalProjectID),
		strings.TrimSpace(observation.ExternalTaskID),
		strings.TrimSpace(observation.ExternalAnnotationID),
		strings.TrimSpace(observation.ExternalRevision),
		strings.TrimSpace(observation.CanonicalPayloadSHA256),
	}, "\x00")
	return "annotation-provider:" + hashBytes([]byte(identity))
}
