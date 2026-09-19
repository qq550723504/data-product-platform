package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/workflow/infrastructure"
)

const reconcileQueuedExecutionCommandType = "WORKFLOW.RECONCILE_QUEUED_EXECUTION"

type QueuedExecutionReconciler struct {
	tx   *transaction.Manager
	repo *infrastructure.PostgresRepository
}

func NewQueuedExecutionReconciler(tx *transaction.Manager, repo *infrastructure.PostgresRepository) *QueuedExecutionReconciler {
	return &QueuedExecutionReconciler{tx: tx, repo: repo}
}

type QueuedExecutionReport struct {
	ExecutionID       uuid.UUID  `json:"executionId"`
	WorkspaceID       uuid.UUID  `json:"workspaceId"`
	Status            string     `json:"status"`
	Action            string     `json:"action"`
	Reason            string     `json:"reason,omitempty"`
	SourceEventID     *uuid.UUID `json:"sourceEventId,omitempty"`
	SourceEventType   string     `json:"sourceEventType,omitempty"`
	SourceEventStatus string     `json:"sourceEventStatus,omitempty"`
	SourceRouting     string     `json:"sourceRoutingVersion,omitempty"`
	QueueConfirmed    bool       `json:"queueConfirmed"`
	DispatchEventID   *uuid.UUID `json:"dispatchEventId,omitempty"`
}

func (r *QueuedExecutionReconciler) Run(ctx context.Context, apply bool, limit int) ([]QueuedExecutionReport, error) {
	ids, err := r.repo.ListQueuedExecutionIDs(ctx, limit)
	if err != nil {
		return nil, err
	}
	reports := make([]QueuedExecutionReport, 0, len(ids))
	for _, executionID := range ids {
		report, err := r.reconcileOne(ctx, executionID, apply)
		if err != nil {
			return reports, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func (r *QueuedExecutionReconciler) reconcileOne(ctx context.Context, executionID uuid.UUID, apply bool) (QueuedExecutionReport, error) {
	source, found, err := r.repo.LatestExecutionQueueSource(ctx, executionID)
	if err != nil {
		return QueuedExecutionReport{}, err
	}
	var result QueuedExecutionReport
	err = r.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		execution, err := r.repo.GetExecutionTx(ctx, tx, executionID, true)
		if err != nil {
			return err
		}
		result = QueuedExecutionReport{
			ExecutionID:    execution.ID,
			WorkspaceID:    execution.WorkspaceID,
			Status:         string(execution.Status),
			Action:         "REPORT_ONLY",
			QueueConfirmed: source.QueueConfirmed,
		}
		if found {
			result.SourceEventID = &source.ID
			result.SourceEventType = source.EventType
			result.SourceEventStatus = source.Status
			result.SourceRouting = source.RoutingVersion
		}
		if execution.Status != domain.ExecutionQueued {
			result.Action = "SKIPPED"
			result.Reason = "execution is no longer QUEUED"
			return nil
		}
		if !found {
			result.Action = "SKIPPED"
			result.Reason = "no historical queue event found"
			return nil
		}
		if len(source.RequiredHandlers) != 0 || source.QueueConfirmed || source.Status != "PUBLISHED" {
			result.Action = "SKIPPED"
			result.Reason = "latest queue event is not an unconfirmed retention-only event"
			return nil
		}
		if err := r.repo.ValidateQueuedExecution(ctx, tx, execution); err != nil {
			result.Action = "SKIPPED"
			result.Reason = "execution references or workspace are not currently valid"
			return nil
		}
		if !apply {
			result.Reason = "legacy retention-only PUBLISHED event is not queue-delivery evidence"
			return nil
		}
		active, err := r.repo.HasActiveExecutionQueueDispatch(ctx, tx, execution.ID)
		if err != nil {
			return err
		}
		if active {
			result.Action = "SKIPPED"
			result.Reason = "a T2 execution-queue dispatch record already exists"
			return nil
		}

		reconcileKey := "execution-reconcile/" + execution.ID.String()
		fingerprint, err := hashExecutionRequest(map[string]any{
			"executionId":   execution.ID,
			"workspaceId":   execution.WorkspaceID,
			"sourceEventId": source.ID,
			"sourceType":    source.EventType,
			"sourceStatus":  source.Status,
		})
		if err != nil {
			return err
		}
		if existing, found, err := r.repo.FindIdempotencyTx(ctx, tx, execution.WorkspaceID, reconcileQueuedExecutionCommandType, reconcileKey); err != nil {
			return err
		} else if found {
			if existing.RequestFingerprint != fingerprint {
				return domain.ErrIdempotencyConflict
			}
			result.Action = "ALREADY_REQUESTED"
			result.DispatchEventID = existing.ResultRef
			result.Reason = "same reconciliation operation already has a dispatch record"
			return nil
		}

		event, err := outbox.NewEvent("EXECUTION", execution.ID, "ExecutionReconciliationQueued", map[string]any{
			"executionId":   execution.ID,
			"workspaceId":   execution.WorkspaceID,
			"sourceEventId": source.ID,
			"sourceType":    source.EventType,
			"sourceStatus":  source.Status,
			"reconcileKey":  reconcileKey,
		})
		if err != nil {
			return err
		}
		if inserted, err := r.repo.TryInsertIdempotency(ctx, tx, execution.WorkspaceID, reconcileQueuedExecutionCommandType, reconcileKey, execution.ID, &event.ID, fingerprint); err != nil {
			return err
		} else if !inserted {
			return fmt.Errorf("reconciliation idempotency record became occupied unexpectedly")
		}
		if err := outbox.Append(ctx, tx, event); err != nil {
			return err
		}
		if err := audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &execution.WorkspaceID,
			ActorType:   "SERVICE",
			Action:      "EXECUTION_RECONCILIATION_DISPATCH_REQUESTED",
			ObjectType:  "EXECUTION",
			ObjectID:    execution.ID,
			AfterState: map[string]any{
				"status":          execution.Status,
				"sourceEventId":   source.ID,
				"dispatchEventId": event.ID,
			},
			Reason:  "controlled reconciliation of legacy retention-only queue event",
			TraceID: reconcileKey,
		}); err != nil {
			return err
		}
		result.Action = "DISPATCH_REQUESTED"
		result.Reason = "created one idempotent T2 execution-queue dispatch record"
		result.DispatchEventID = &event.ID
		return nil
	})
	if err != nil {
		return QueuedExecutionReport{}, err
	}
	return result, nil
}
