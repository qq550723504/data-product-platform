package application

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/deliveryfence"
	"time"
)

type EngineInvocationObserver struct {
	Start         func(context.Context) (annotationdomain.EngineAttempt, error)
	Finish        func(context.Context, annotationdomain.EngineAttempt, error) error
	LastAttemptID uuid.UUID
}
type engineInvocationContextKey struct{}

func WithEngineInvocationObserver(ctx context.Context, o *EngineInvocationObserver) context.Context {
	return context.WithValue(ctx, engineInvocationContextKey{}, o)
}
func EngineInvocationFromContext(ctx context.Context) *EngineInvocationObserver {
	o, _ := ctx.Value(engineInvocationContextKey{}).(*EngineInvocationObserver)
	return o
}
func (s *EngineService) controlledInvocationContext(ctx context.Context, operation annotationdomain.EngineOperation, first annotationdomain.EngineAttempt) context.Context {
	used := false
	observer := &EngineInvocationObserver{}
	observer.Start = func(ctx context.Context) (annotationdomain.EngineAttempt, error) {
		attempt := first
		err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := deliveryfence.Lock(ctx, tx, operation.WorkspaceID); err != nil {
				return err
			}
			campaign, err := s.repo.LockCampaignTx(ctx, tx, operation.CampaignID)
			if err != nil {
				return err
			}
			if campaign.Status != annotationdomain.CampaignActive || campaign.WorkspaceID != operation.WorkspaceID {
				return annotationdomain.ErrInvalidCampaign
			}
			if s.sendGuard == nil {
				return ErrActivationGuardRequired
			}
			if err := s.sendGuard.ValidateEngineSendTx(ctx, tx, campaign); err != nil {
				return err
			}
			if used {
				attempt, err = s.repo.StartEngineAttempt(ctx, tx, operation.WorkspaceID, operation.ID, annotationdomain.EngineAttemptFetch, time.Now().UTC())
				if err != nil {
					return err
				}
				if err := appendEvent(ctx, tx, "ANNOTATION_ENGINE_OPERATION", operation.ID, "AnnotationEngineAttemptStarted", map[string]any{"attemptId": attempt.ID, "attemptKind": attempt.AttemptKind, "physicalHTTP": true}); err != nil {
					return err
				}
			}
			return cost.AppendAnnotationEngineActivity(ctx, tx, cost.AnnotationEngineActivity{WorkspaceID: operation.WorkspaceID, AttemptID: attempt.ID, Quantity: 1, Unit: "invocation", PricingMode: "ACTUAL", OccurredAt: attempt.StartedAt, Metadata: map[string]any{"provider": operation.Provider, "physicalHTTP": true}})
		})
		if err == nil {
			used = true
		}
		return attempt, err
	}
	reconciler := &EngineResultReconciler{tx: s.tx, repo: s.repo}
	observer.Finish = func(ctx context.Context, attempt annotationdomain.EngineAttempt, remoteErr error) error {
		observationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return reconciler.finishFetchAttempt(observationCtx, operation, attempt, remoteErr)
	}
	return WithEngineInvocationObserver(ctx, observer)
}
func controlledEngine(engine AnnotationEnginePort) bool {
	c, ok := engine.(interface {
		SourceContract() annotationdomain.SourceContract
	})
	return ok && c.SourceContract().Protocol() == annotationdomain.ControlledSubmissionProtocol
}
