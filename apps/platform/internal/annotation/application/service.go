package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	annotationdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/domain"
	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

var ErrIdempotencyConflict = errors.New("annotation idempotency key reused with different input")

type Service struct {
	tx   *transaction.Manager
	repo *annotationinfra.Repository
}

func NewService(tx *transaction.Manager, repo *annotationinfra.Repository) *Service {
	return &Service{tx: tx, repo: repo}
}

type CreateCampaignCommand struct {
	Spec    annotationdomain.CampaignSpec
	TraceID string
}

func (s *Service) CreateCampaign(ctx context.Context, cmd CreateCampaignCommand) (annotationdomain.Campaign, error) {
	campaign, err := annotationdomain.NewCampaign(cmd.Spec, time.Now().UTC())
	if err != nil {
		return annotationdomain.Campaign{}, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertCampaign(ctx, tx, campaign); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "ANNOTATION_CAMPAIGN", campaign.ID, "AnnotationCampaignCreated", map[string]any{
			"campaignId": campaign.ID, "workspaceId": campaign.WorkspaceID,
			"inputDatasetVersionId": campaign.InputDatasetVersionID,
			"inputCertificationId": campaign.InputCertificationID,
		}); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID: campaign.WorkspaceID, EvidenceType: "ANNOTATION_CAMPAIGN_CREATED",
			Title: "Annotation campaign created", SourceType: "CORE",
			Metadata: map[string]any{
				"inputDatasetVersionId": campaign.InputDatasetVersionID,
				"schemaHash": campaign.Schema.ContentSHA256,
				"taxonomyHash": campaign.Taxonomy.ContentSHA256,
			},
			CreatedBy: cmd.Spec.ActorID,
		}, evidence.Relation{ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: campaign.ID, RelationType: "CREATION_EVIDENCE"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &campaign.WorkspaceID, ActorType: actorType(cmd.Spec.ActorID),
			ActorID: cmd.Spec.ActorID, Action: "ANNOTATION_CAMPAIGN_CREATED",
			ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: campaign.ID,
			AfterState: map[string]any{
				"status": campaign.Status, "revision": campaign.Revision,
				"inputDatasetVersionId": campaign.InputDatasetVersionID,
			},
			TraceID: cmd.TraceID,
		})
	})
	return campaign, err
}

type TaskInput struct {
	SourceItemRef       string
	SourceContentSHA256 string
	TaskTextSHA256      string
	PrimaryAnnotatorRef string
}

type CreateTasksCommand struct {
	WorkspaceID uuid.UUID
	CampaignID  uuid.UUID
	Tasks       []TaskInput
	ActorID     *uuid.UUID
	TraceID     string
}

func (s *Service) CreateTasks(ctx context.Context, cmd CreateTasksCommand) ([]annotationdomain.Task, error) {
	if cmd.WorkspaceID == uuid.Nil || cmd.CampaignID == uuid.Nil || len(cmd.Tasks) == 0 {
		return nil, annotationdomain.ErrInvalidTask
	}
	tasks := make([]annotationdomain.Task, 0, len(cmd.Tasks))
	now := time.Now().UTC()
	for _, input := range cmd.Tasks {
		task, err := annotationdomain.NewTask(
			cmd.WorkspaceID, cmd.CampaignID, input.SourceItemRef,
			input.SourceContentSHA256, input.TaskTextSHA256, input.PrimaryAnnotatorRef, now,
		)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		campaign, err := s.repo.GetCampaignTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		if campaign.WorkspaceID != cmd.WorkspaceID || campaign.Status != annotationdomain.CampaignDraft {
			return annotationdomain.ErrInvalidCampaign
		}
		for _, task := range tasks {
			if err := s.repo.InsertTask(ctx, tx, task); err != nil {
				return err
			}
		}
		if err := appendEvent(ctx, tx, "ANNOTATION_CAMPAIGN", cmd.CampaignID, "AnnotationTasksCreated", map[string]any{
			"campaignId": cmd.CampaignID, "workspaceId": cmd.WorkspaceID, "count": len(tasks),
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_TASKS_CREATED", ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: cmd.CampaignID,
			AfterState: map[string]any{"count": len(tasks)}, TraceID: cmd.TraceID,
		})
	})
	return tasks, err
}

type ActivateCampaignCommand struct {
	WorkspaceID      uuid.UUID
	CampaignID       uuid.UUID
	ExpectedRevision int64
	ActorID          *uuid.UUID
	TraceID          string
}

func (s *Service) ActivateCampaign(ctx context.Context, cmd ActivateCampaignCommand) (annotationdomain.Campaign, error) {
	var activated annotationdomain.Campaign
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		campaign, err := s.repo.GetCampaignTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		if campaign.WorkspaceID != cmd.WorkspaceID || campaign.Status != annotationdomain.CampaignDraft {
			return annotationdomain.ErrInvalidCampaign
		}
		tasks, err := s.repo.ListTasksTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		if len(tasks) == 0 {
			return annotationdomain.ErrInvalidCampaign
		}
		manifestHash, err := taskManifestHash(tasks)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := s.repo.ActivateCampaign(
			ctx, tx, cmd.CampaignID, cmd.ExpectedRevision, len(tasks), manifestHash, now,
		); err != nil {
			return err
		}
		activated = campaign
		activated.Status = annotationdomain.CampaignActive
		activated.Revision = campaign.Revision + 1
		activated.ExpectedTaskCount = len(tasks)
		activated.TaskManifestHash = manifestHash
		activated.ActivatedAt = &now

		if err := appendEvent(ctx, tx, "ANNOTATION_CAMPAIGN", cmd.CampaignID, "AnnotationCampaignActivated", map[string]any{
			"campaignId": cmd.CampaignID, "taskCount": len(tasks), "taskManifestHash": manifestHash,
		}); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID: cmd.WorkspaceID, EvidenceType: "ANNOTATION_TASK_MANIFEST_FROZEN",
			Title: "Annotation task manifest frozen", SourceType: "CORE",
			Metadata: map[string]any{"taskCount": len(tasks), "taskManifestHash": manifestHash},
			CreatedBy: cmd.ActorID,
		}, evidence.Relation{ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: cmd.CampaignID, RelationType: "TASK_MANIFEST"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_CAMPAIGN_ACTIVATED", ObjectType: "ANNOTATION_CAMPAIGN", ObjectID: cmd.CampaignID,
			BeforeState: map[string]any{"status": campaign.Status, "revision": campaign.Revision},
			AfterState: map[string]any{
				"status": annotationdomain.CampaignActive, "revision": campaign.Revision + 1,
				"taskCount": len(tasks), "taskManifestHash": manifestHash,
			},
			TraceID: cmd.TraceID,
		})
	})
	return activated, err
}

type RecordResultCommand struct {
	WorkspaceID            uuid.UUID
	CampaignID             uuid.UUID
	TaskID                 uuid.UUID
	ExpectedTaskRevision   int64
	AuthorRef              string
	ProviderBindingRef     string
	ExternalTaskID         string
	ExternalAnnotationID   string
	ExternalRevision       string
	ObservationKey         string
	CanonicalPayload       []byte
	CanonicalPayloadSHA256 string
	NormalizerVersion      string
	ActorID                *uuid.UUID
	TraceID                string
}

func (s *Service) RecordAnnotationResult(ctx context.Context, cmd RecordResultCommand) (annotationdomain.Result, error) {
	if existing, err := s.repo.GetResultByObservation(ctx, cmd.WorkspaceID, cmd.CampaignID, cmd.ObservationKey); err == nil {
		if resultReplayMatches(existing, cmd) {
			return existing, nil
		}
		return annotationdomain.Result{}, ErrIdempotencyConflict
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.Result{}, err
	}

	if hashBytes(cmd.CanonicalPayload) != cmd.CanonicalPayloadSHA256 {
		return annotationdomain.Result{}, annotationdomain.ErrInvalidResult
	}
	result := annotationdomain.Result{
		ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, CampaignID: cmd.CampaignID, TaskID: cmd.TaskID,
		AuthorRef: cmd.AuthorRef, ProviderBindingRef: cmd.ProviderBindingRef,
		ExternalTaskID: cmd.ExternalTaskID, ExternalAnnotationID: cmd.ExternalAnnotationID,
		ExternalRevision: cmd.ExternalRevision, ObservationKey: cmd.ObservationKey,
		CanonicalPayload: append([]byte(nil), cmd.CanonicalPayload...),
		CanonicalPayloadSHA256: cmd.CanonicalPayloadSHA256, NormalizerVersion: cmd.NormalizerVersion,
		CreatedAt: time.Now().UTC(), CreatedBy: cmd.ActorID,
	}
	if err := result.Validate(); err != nil {
		return annotationdomain.Result{}, err
	}

	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		task, err := s.repo.GetTaskTx(ctx, tx, cmd.TaskID)
		if err != nil {
			return err
		}
		if task.WorkspaceID != cmd.WorkspaceID || task.CampaignID != cmd.CampaignID ||
			task.Revision != cmd.ExpectedTaskRevision {
			return annotationinfra.ErrStaleRevision
		}
		if err := s.repo.InsertResult(ctx, tx, result); err != nil {
			return err
		}
		if err := s.repo.AdvanceTaskForResult(ctx, tx, cmd.TaskID, cmd.ExpectedTaskRevision); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "ANNOTATION_TASK", cmd.TaskID, "AnnotationResultRecorded", map[string]any{
			"taskId": cmd.TaskID, "resultId": result.ID, "payloadSha256": result.CanonicalPayloadSHA256,
		}); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID: cmd.WorkspaceID, EvidenceType: "ANNOTATION_RESULT_RECORDED",
			Title: "Annotation result recorded", SourceType: "CORE",
			Metadata: map[string]any{
				"taskId": cmd.TaskID, "resultId": result.ID,
				"payloadSha256": result.CanonicalPayloadSHA256,
				"normalizerVersion": result.NormalizerVersion,
			},
			CreatedBy: cmd.ActorID,
		}, evidence.Relation{ObjectType: "ANNOTATION_RESULT", ObjectID: result.ID, RelationType: "RESULT_EVIDENCE"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_RESULT_RECORDED", ObjectType: "ANNOTATION_RESULT", ObjectID: result.ID,
			AfterState: map[string]any{
				"taskId": cmd.TaskID, "payloadSha256": result.CanonicalPayloadSHA256,
			},
			TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		if existing, readErr := s.repo.GetResultByObservation(ctx, cmd.WorkspaceID, cmd.CampaignID, cmd.ObservationKey); readErr == nil {
			if resultReplayMatches(existing, cmd) {
				return existing, nil
			}
			return annotationdomain.Result{}, ErrIdempotencyConflict
		}
		return annotationdomain.Result{}, err
	}
	return result, nil
}

type ReviewAnnotationCommand struct {
	WorkspaceID          uuid.UUID
	CampaignID           uuid.UUID
	TaskID               uuid.UUID
	ExpectedTaskRevision int64
	ReviewerRef          string
	Action               string
	Reason               string
	IdempotencyKey       string
	ReviewedResultID     *uuid.UUID
	CorrectedPayload     []byte
	CorrectedPayloadHash string
	ActorID              *uuid.UUID
	TraceID              string
}

type ReviewAnnotationResult struct {
	Attempt  annotationdomain.ReviewAttempt
	Outcome  annotationdomain.ReviewAttemptOutcome
	Decision *annotationdomain.ReviewDecision
}

func (s *Service) ReviewAnnotation(ctx context.Context, cmd ReviewAnnotationCommand) (ReviewAnnotationResult, error) {
	fingerprint, err := reviewFingerprint(cmd)
	if err != nil {
		return ReviewAnnotationResult{}, err
	}

	attempt, err := s.ensureReviewAttempt(ctx, cmd, fingerprint)
	if err != nil {
		return ReviewAnnotationResult{}, err
	}
	if outcome, err := s.repo.GetReviewAttemptOutcome(ctx, attempt.ID); err == nil {
		var decision *annotationdomain.ReviewDecision
		if existing, decisionErr := s.repo.GetDecisionByAttempt(ctx, attempt.ID); decisionErr == nil {
			decision = &existing
		} else if !errors.Is(decisionErr, pgx.ErrNoRows) {
			return ReviewAnnotationResult{}, decisionErr
		}
		return ReviewAnnotationResult{Attempt: attempt, Outcome: outcome, Decision: decision}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return ReviewAnnotationResult{}, err
	}

	decision, err := s.commitReviewDecision(ctx, cmd, attempt)
	if err == nil {
		outcome, outcomeErr := s.repo.GetReviewAttemptOutcome(ctx, attempt.ID)
		if outcomeErr != nil {
			return ReviewAnnotationResult{}, outcomeErr
		}
		return ReviewAnnotationResult{Attempt: attempt, Outcome: outcome, Decision: &decision}, nil
	}
	if !errors.Is(err, annotationinfra.ErrStaleRevision) {
		if outcomeErr := s.recordReviewFailure(ctx, cmd, attempt, annotationdomain.ReviewAttemptRejected, "REVIEW_FAILED"); outcomeErr != nil {
			return ReviewAnnotationResult{}, fmt.Errorf("review failed: %v; record outcome: %w", err, outcomeErr)
		}
		return ReviewAnnotationResult{}, err
	}
	outcome, outcomeErr := s.recordReviewFailure(ctx, cmd, attempt, annotationdomain.ReviewAttemptStaleConflict, "STALE_REVISION")
	if outcomeErr != nil {
		return ReviewAnnotationResult{}, fmt.Errorf("review stale: %v; record outcome: %w", err, outcomeErr)
	}
	return ReviewAnnotationResult{Attempt: attempt, Outcome: outcome}, annotationinfra.ErrStaleRevision
}

func (s *Service) ensureReviewAttempt(
	ctx context.Context,
	cmd ReviewAnnotationCommand,
	fingerprint string,
) (annotationdomain.ReviewAttempt, error) {
	if existing, err := s.repo.GetReviewAttemptByKey(ctx, cmd.WorkspaceID, cmd.IdempotencyKey); err == nil {
		if existing.RequestFingerprint != fingerprint {
			return annotationdomain.ReviewAttempt{}, ErrIdempotencyConflict
		}
		return existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return annotationdomain.ReviewAttempt{}, err
	}

	attempt := annotationdomain.ReviewAttempt{
		ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, CampaignID: cmd.CampaignID, TaskID: cmd.TaskID,
		ReviewerRef: strings.TrimSpace(cmd.ReviewerRef), ExpectedTaskRevision: cmd.ExpectedTaskRevision,
		Action: cmd.Action, Reason: strings.TrimSpace(cmd.Reason), IdempotencyKey: cmd.IdempotencyKey,
		RequestFingerprint: fingerprint, CreatedAt: time.Now().UTC(),
	}
	if err := attempt.Validate(); err != nil {
		return annotationdomain.ReviewAttempt{}, err
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertReviewAttempt(ctx, tx, attempt); err != nil {
			return err
		}
		if err := cost.AppendAnnotationReviewActivity(ctx, tx, cost.AnnotationReviewActivity{
			WorkspaceID: cmd.WorkspaceID, AttemptID: attempt.ID, Quantity: 1, Unit: "review",
			PricingMode: "ACTUAL", Metadata: map[string]any{
				"taskId": cmd.TaskID, "action": cmd.Action, "reviewerRef": cmd.ReviewerRef,
			}, OccurredAt: attempt.CreatedAt,
		}); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "ANNOTATION_REVIEW_ATTEMPT", attempt.ID, "AnnotationReviewAttemptStarted", map[string]any{
			"attemptId": attempt.ID, "taskId": cmd.TaskID, "action": cmd.Action,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_REVIEW_ATTEMPT_STARTED", ObjectType: "ANNOTATION_REVIEW_ATTEMPT",
			ObjectID: attempt.ID, AfterState: map[string]any{
				"taskId": cmd.TaskID, "expectedTaskRevision": cmd.ExpectedTaskRevision, "action": cmd.Action,
			}, Reason: cmd.Reason, TraceID: cmd.TraceID,
		})
	})
	if err != nil {
		if existing, readErr := s.repo.GetReviewAttemptByKey(ctx, cmd.WorkspaceID, cmd.IdempotencyKey); readErr == nil {
			if existing.RequestFingerprint == fingerprint {
				return existing, nil
			}
			return annotationdomain.ReviewAttempt{}, ErrIdempotencyConflict
		}
		return annotationdomain.ReviewAttempt{}, err
	}
	return attempt, nil
}

func (s *Service) commitReviewDecision(
	ctx context.Context,
	cmd ReviewAnnotationCommand,
	attempt annotationdomain.ReviewAttempt,
) (annotationdomain.ReviewDecision, error) {
	var decision annotationdomain.ReviewDecision
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		selectedResultID := cmd.ReviewedResultID
		if cmd.Action == annotationdomain.ReviewCorrect {
			if cmd.ReviewedResultID == nil || hashBytes(cmd.CorrectedPayload) != cmd.CorrectedPayloadHash {
				return annotationdomain.ErrInvalidReviewDecision
			}
			correctionID := uuid.New()
			correction := annotationdomain.Result{
				ID: correctionID, WorkspaceID: cmd.WorkspaceID, CampaignID: cmd.CampaignID, TaskID: cmd.TaskID,
				AuthorRef: cmd.ReviewerRef, ObservationKey: "review-correction:" + attempt.ID.String(),
				CanonicalPayload: append([]byte(nil), cmd.CorrectedPayload...),
				CanonicalPayloadSHA256: cmd.CorrectedPayloadHash, NormalizerVersion: "review-correction-v1",
				CorrectedFromResultID: cmd.ReviewedResultID, CreatedAt: time.Now().UTC(), CreatedBy: cmd.ActorID,
			}
			if err := s.repo.InsertResult(ctx, tx, correction); err != nil {
				return err
			}
			selectedResultID = &correctionID
		}

		decision = annotationdomain.ReviewDecision{
			ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, CampaignID: cmd.CampaignID, TaskID: cmd.TaskID,
			ReviewAttemptID: attempt.ID, ReviewedResultID: cmd.ReviewedResultID, SelectedResultID: selectedResultID,
			ReviewerRef: cmd.ReviewerRef, Outcome: cmd.Action, Reason: cmd.Reason,
			ExpectedTaskRevision: cmd.ExpectedTaskRevision, CreatedAt: time.Now().UTC(),
		}
		if err := s.repo.InsertReviewDecision(ctx, tx, decision); err != nil {
			return err
		}
		outcome := annotationdomain.ReviewAttemptOutcome{
			ID: uuid.New(), AttemptID: attempt.ID, Outcome: annotationdomain.ReviewAttemptSucceeded, OccurredAt: time.Now().UTC(),
		}
		if err := s.repo.InsertReviewAttemptOutcome(ctx, tx, outcome); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "ANNOTATION_TASK", cmd.TaskID, "AnnotationReviewed", map[string]any{
			"taskId": cmd.TaskID, "decisionId": decision.ID, "outcome": decision.Outcome,
			"selectedResultId": decision.SelectedResultID,
		}); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID: cmd.WorkspaceID, EvidenceType: "ANNOTATION_REVIEW_DECISION",
			Title: "Annotation review decision", SourceType: "CORE",
			Metadata: map[string]any{
				"taskId": cmd.TaskID, "decisionId": decision.ID, "outcome": decision.Outcome,
				"reviewedResultId": decision.ReviewedResultID, "selectedResultId": decision.SelectedResultID,
			}, CreatedBy: cmd.ActorID,
		}, evidence.Relation{ObjectType: "ANNOTATION_REVIEW_DECISION", ObjectID: decision.ID, RelationType: "DECISION_EVIDENCE"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_REVIEWED", ObjectType: "ANNOTATION_REVIEW_DECISION", ObjectID: decision.ID,
			AfterState: map[string]any{
				"taskId": cmd.TaskID, "outcome": decision.Outcome, "selectedResultId": decision.SelectedResultID,
			}, Reason: cmd.Reason, TraceID: cmd.TraceID,
		})
	})
	return decision, err
}

func (s *Service) recordReviewFailure(
	ctx context.Context,
	cmd ReviewAnnotationCommand,
	attempt annotationdomain.ReviewAttempt,
	outcomeValue string,
	errorCode string,
) (annotationdomain.ReviewAttemptOutcome, error) {
	outcome := annotationdomain.ReviewAttemptOutcome{
		ID: uuid.New(), AttemptID: attempt.ID, Outcome: outcomeValue, ErrorCode: errorCode, OccurredAt: time.Now().UTC(),
	}
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertReviewAttemptOutcome(ctx, tx, outcome); err != nil {
			return err
		}
		if err := appendEvent(ctx, tx, "ANNOTATION_REVIEW_ATTEMPT", attempt.ID, "AnnotationReviewAttemptFailed", map[string]any{
			"attemptId": attempt.ID, "taskId": cmd.TaskID, "outcome": outcomeValue, "errorCode": errorCode,
		}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_REVIEW_ATTEMPT_FAILED", ObjectType: "ANNOTATION_REVIEW_ATTEMPT",
			ObjectID: attempt.ID, AfterState: map[string]any{"outcome": outcomeValue, "errorCode": errorCode},
			Reason: cmd.Reason, TraceID: cmd.TraceID,
		})
	})
	return outcome, err
}


type FinalizeAnnotationSnapshotCommand struct {
	WorkspaceID uuid.UUID
	CampaignID  uuid.UUID
	ActorID     *uuid.UUID
	TraceID     string
}

func (s *Service) FinalizeAnnotationSnapshot(
	ctx context.Context,
	cmd FinalizeAnnotationSnapshotCommand,
) (annotationdomain.Snapshot, error) {
	var snapshot annotationdomain.Snapshot
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		campaign, err := s.repo.GetCampaignTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		if campaign.WorkspaceID != cmd.WorkspaceID || campaign.Status != annotationdomain.CampaignActive {
			return annotationdomain.ErrInvalidSnapshot
		}
		tasks, err := s.repo.ListTasksTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		results, err := s.repo.ListResultsTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		decisions, err := s.repo.ListDecisionsTx(ctx, tx, cmd.CampaignID)
		if err != nil {
			return err
		}
		if len(tasks) == 0 || len(tasks) != campaign.ExpectedTaskCount || len(decisions) != len(tasks) {
			return annotationdomain.ErrInvalidSnapshot
		}

		manifest, outputCount, err := snapshotManifest(campaign, tasks, results, decisions)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		snapshot = annotationdomain.Snapshot{
			ID: uuid.New(), WorkspaceID: cmd.WorkspaceID, CampaignID: cmd.CampaignID,
			Status: annotationdomain.SnapshotBuilding, Manifest: manifest,
			ManifestHashPayload: append([]byte(nil), manifest...), RootHash: hashBytes(manifest),
			ExpectedTaskCount: len(tasks), ExpectedResultCount: len(results),
			ExpectedDecisionCount: len(decisions), ExpectedOutputCount: outputCount,
			CreatedAt: now, CreatedBy: cmd.ActorID,
		}
		if err := snapshot.Validate(); err != nil {
			return err
		}
		if err := s.repo.InsertAndFinalizeSnapshot(ctx, tx, snapshot, tasks, results, decisions, now); err != nil {
			return err
		}
		snapshot.Status = annotationdomain.SnapshotFinalized
		snapshot.FinalizedAt = &now

		if err := appendEvent(ctx, tx, "ANNOTATION_SNAPSHOT", snapshot.ID, "AnnotationSnapshotFinalized", map[string]any{
			"snapshotId": snapshot.ID, "campaignId": cmd.CampaignID,
			"rootHash": snapshot.RootHash, "taskCount": len(tasks),
			"resultCount": len(results), "decisionCount": len(decisions), "outputCount": outputCount,
		}); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID: cmd.WorkspaceID, EvidenceType: "ANNOTATION_SNAPSHOT_FINALIZED",
			Title: "Annotation snapshot finalized", SourceType: "CORE",
			Metadata: map[string]any{
				"campaignId": cmd.CampaignID, "rootHash": snapshot.RootHash,
				"taskCount": len(tasks), "resultCount": len(results),
				"decisionCount": len(decisions), "outputCount": outputCount,
			}, CreatedBy: cmd.ActorID,
		}, evidence.Relation{ObjectType: "ANNOTATION_SNAPSHOT", ObjectID: snapshot.ID, RelationType: "FINALIZATION_EVIDENCE"}); err != nil {
			return err
		}
		return audit.Append(ctx, tx, audit.Event{
			WorkspaceID: &cmd.WorkspaceID, ActorType: actorType(cmd.ActorID), ActorID: cmd.ActorID,
			Action: "ANNOTATION_SNAPSHOT_FINALIZED", ObjectType: "ANNOTATION_SNAPSHOT", ObjectID: snapshot.ID,
			AfterState: map[string]any{
				"campaignId": cmd.CampaignID, "rootHash": snapshot.RootHash,
				"taskCount": len(tasks), "resultCount": len(results),
				"decisionCount": len(decisions), "outputCount": outputCount,
			}, TraceID: cmd.TraceID,
		})
	})
	return snapshot, err
}

func snapshotManifest(
	campaign annotationdomain.Campaign,
	tasks []annotationdomain.Task,
	results []annotationdomain.Result,
	decisions []annotationdomain.ReviewDecision,
) ([]byte, int, error) {
	type taskItem struct {
		ID                  string `json:"id"`
		SourceItemRef       string `json:"sourceItemRef"`
		SourceContentSHA256 string `json:"sourceContentSha256"`
		TaskTextSHA256      string `json:"taskTextSha256"`
	}
	type resultItem struct {
		ID                    string  `json:"id"`
		TaskID                string  `json:"taskId"`
		CanonicalPayloadSHA256 string  `json:"canonicalPayloadSha256"`
		AuthorRef             string  `json:"authorRef"`
		CorrectedFromResultID *string `json:"correctedFromResultId,omitempty"`
	}
	type decisionItem struct {
		ID               string  `json:"id"`
		TaskID           string  `json:"taskId"`
		Outcome          string  `json:"outcome"`
		ReviewedResultID *string `json:"reviewedResultId,omitempty"`
		SelectedResultID *string `json:"selectedResultId,omitempty"`
		ReviewerRef      string  `json:"reviewerRef"`
		Reason           string  `json:"reason"`
	}
	type outputItem struct {
		TaskID           string `json:"taskId"`
		SelectedResultID string `json:"selectedResultId"`
	}
	type manifest struct {
		FormatVersion                 string         `json:"formatVersion"`
		CampaignID                    string         `json:"campaignId"`
		InputDatasetVersionID         string         `json:"inputDatasetVersionId"`
		InputCertificationID          string         `json:"inputCertificationId"`
		AnnotationContributionID      string         `json:"annotationContributionResourceId"`
		TaskManifestHash              string         `json:"taskManifestHash"`
		SchemaHash                    string         `json:"schemaHash"`
		TaxonomyHash                  string         `json:"taxonomyHash"`
		RubricHash                    string         `json:"rubricHash"`
		RendererHash                  string         `json:"rendererHash"`
		ReviewPolicyHash              string         `json:"reviewPolicyHash"`
		Tasks                         []taskItem     `json:"tasks"`
		Results                       []resultItem   `json:"results"`
		Decisions                     []decisionItem `json:"decisions"`
		Outputs                       []outputItem   `json:"outputs"`
	}

	taskItems := make([]taskItem, 0, len(tasks))
	for _, task := range tasks {
		taskItems = append(taskItems, taskItem{
			ID: task.ID.String(), SourceItemRef: task.SourceItemRef,
			SourceContentSHA256: task.SourceContentSHA256, TaskTextSHA256: task.TaskTextSHA256,
		})
	}
	resultItems := make([]resultItem, 0, len(results))
	for _, result := range results {
		var corrected *string
		if result.CorrectedFromResultID != nil {
			value := result.CorrectedFromResultID.String()
			corrected = &value
		}
		resultItems = append(resultItems, resultItem{
			ID: result.ID.String(), TaskID: result.TaskID.String(),
			CanonicalPayloadSHA256: result.CanonicalPayloadSHA256,
			AuthorRef: result.AuthorRef, CorrectedFromResultID: corrected,
		})
	}
	decisionItems := make([]decisionItem, 0, len(decisions))
	outputs := make([]outputItem, 0)
	for _, decision := range decisions {
		var reviewed, selected *string
		if decision.ReviewedResultID != nil {
			value := decision.ReviewedResultID.String()
			reviewed = &value
		}
		if decision.SelectedResultID != nil {
			value := decision.SelectedResultID.String()
			selected = &value
		}
		decisionItems = append(decisionItems, decisionItem{
			ID: decision.ID.String(), TaskID: decision.TaskID.String(), Outcome: decision.Outcome,
			ReviewedResultID: reviewed, SelectedResultID: selected,
			ReviewerRef: decision.ReviewerRef, Reason: decision.Reason,
		})
		if selected != nil && (decision.Outcome == annotationdomain.ReviewAccept || decision.Outcome == annotationdomain.ReviewCorrect) {
			outputs = append(outputs, outputItem{TaskID: decision.TaskID.String(), SelectedResultID: *selected})
		}
	}
	sort.Slice(taskItems, func(i, j int) bool { return taskItems[i].ID < taskItems[j].ID })
	sort.Slice(resultItems, func(i, j int) bool { return resultItems[i].ID < resultItems[j].ID })
	sort.Slice(decisionItems, func(i, j int) bool { return decisionItems[i].ID < decisionItems[j].ID })
	sort.Slice(outputs, func(i, j int) bool { return outputs[i].TaskID < outputs[j].TaskID })

	encoded, err := json.Marshal(manifest{
		FormatVersion: "annotation-snapshot-v1", CampaignID: campaign.ID.String(),
		InputDatasetVersionID: campaign.InputDatasetVersionID.String(),
		InputCertificationID: campaign.InputCertificationID.String(),
		AnnotationContributionID: campaign.AnnotationContributionID.String(),
		TaskManifestHash: campaign.TaskManifestHash,
		SchemaHash: campaign.Schema.ContentSHA256, TaxonomyHash: campaign.Taxonomy.ContentSHA256,
		RubricHash: campaign.Rubric.ContentSHA256, RendererHash: campaign.Renderer.ContentSHA256,
		ReviewPolicyHash: campaign.ReviewPolicy.ContentSHA256,
		Tasks: taskItems, Results: resultItems, Decisions: decisionItems, Outputs: outputs,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("marshal annotation snapshot manifest: %w", err)
	}
	return encoded, len(outputs), nil
}

func appendEvent(
	ctx context.Context,
	tx pgx.Tx,
	aggregateType string,
	aggregateID uuid.UUID,
	eventType string,
	payload any,
) error {
	event, err := outbox.NewEvent(aggregateType, aggregateID, eventType, payload)
	if err != nil {
		return err
	}
	return outbox.Append(ctx, tx, event)
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}

func taskManifestHash(tasks []annotationdomain.Task) (string, error) {
	type item struct {
		ID                  string `json:"id"`
		SourceItemRef       string `json:"sourceItemRef"`
		SourceContentSHA256 string `json:"sourceContentSha256"`
		TaskTextSHA256      string `json:"taskTextSha256"`
		PrimaryAnnotatorRef string `json:"primaryAnnotatorRef"`
	}
	items := make([]item, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, item{
			ID: task.ID.String(), SourceItemRef: task.SourceItemRef,
			SourceContentSHA256: task.SourceContentSHA256, TaskTextSHA256: task.TaskTextSHA256,
			PrimaryAnnotatorRef: task.PrimaryAnnotatorRef,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	encoded, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return hashBytes(encoded), nil
}

func reviewFingerprint(cmd ReviewAnnotationCommand) (string, error) {
	payload := struct {
		WorkspaceID          uuid.UUID  `json:"workspaceId"`
		CampaignID           uuid.UUID  `json:"campaignId"`
		TaskID               uuid.UUID  `json:"taskId"`
		ExpectedTaskRevision int64      `json:"expectedTaskRevision"`
		ReviewerRef          string     `json:"reviewerRef"`
		Action               string     `json:"action"`
		Reason               string     `json:"reason"`
		ReviewedResultID     *uuid.UUID `json:"reviewedResultId"`
		CorrectedPayloadHash string     `json:"correctedPayloadHash"`
	}{
		WorkspaceID: cmd.WorkspaceID, CampaignID: cmd.CampaignID, TaskID: cmd.TaskID,
		ExpectedTaskRevision: cmd.ExpectedTaskRevision, ReviewerRef: strings.TrimSpace(cmd.ReviewerRef),
		Action: cmd.Action, Reason: strings.TrimSpace(cmd.Reason), ReviewedResultID: cmd.ReviewedResultID,
		CorrectedPayloadHash: cmd.CorrectedPayloadHash,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return hashBytes(encoded), nil
}

func resultReplayMatches(existing annotationdomain.Result, cmd RecordResultCommand) bool {
	return existing.TaskID == cmd.TaskID &&
		existing.AuthorRef == strings.TrimSpace(cmd.AuthorRef) &&
		existing.CanonicalPayloadSHA256 == cmd.CanonicalPayloadSHA256 &&
		existing.NormalizerVersion == strings.TrimSpace(cmd.NormalizerVersion)
}

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
