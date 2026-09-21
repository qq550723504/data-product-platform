package application

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/cost"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/industrypack"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/domain"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/quality/native"
)

type ObjectStore interface {
	Get(ctx context.Context, storageURI string) (io.ReadCloser, error)
}

var ErrAssessmentAttemptConflict = errors.New("quality assessment attempt conflicts with an existing assessment")
var ErrAssessmentAttemptInProgress = errors.New("quality assessment attempt is already in progress")
var ErrAssessmentAttemptFailed = errors.New("quality assessment attempt already failed")

const attemptOutcomeRecoveryTimeout = 5 * time.Second
const assessmentAttemptLeaseDuration = time.Hour
const assessmentAttemptLockPrefix = "quality-assessment-attempt:"

type Service struct {
	industryPackRoot string
	tx               *transaction.Manager
	datasetRepo      *datasetinfra.PostgresRepository
	repo             *infrastructure.PostgresRepository
	store            ObjectStore
}

func NewService(industryPackRoot string, tx *transaction.Manager, datasetRepo *datasetinfra.PostgresRepository, repo *infrastructure.PostgresRepository, store ObjectStore) *Service {
	return &Service{
		industryPackRoot: industryPackRoot,
		tx:               tx,
		datasetRepo:      datasetRepo,
		repo:             repo,
		store:            store,
	}
}

type RunCommand struct {
	WorkspaceID         uuid.UUID
	DatasetVersionID    uuid.UUID
	RuleSetRef          string
	AssessmentAttemptID uuid.UUID
	ActorID             *uuid.UUID
	TraceID             string
	Now                 time.Time
}

func (s *Service) Run(ctx context.Context, cmd RunCommand) (domain.Assessment, error) {
	attemptID := cmd.AssessmentAttemptID
	if attemptID == uuid.Nil {
		attemptID = uuid.New()
	}
	if cmd.AssessmentAttemptID != uuid.Nil {
		attempt, found, err := s.reconcileAttempt(ctx, cmd, attemptID)
		if err != nil {
			return domain.Assessment{}, err
		}
		if found {
			return attempt, nil
		}
	}
	version, err := s.datasetRepo.GetVersion(ctx, cmd.DatasetVersionID)
	if err != nil {
		return domain.Assessment{}, err
	}
	// The result workspace is derived from the Dataset, not trusted from the caller.
	// A DatasetVersion foreign key proves the row exists, not which workspace owns it.
	// Ownership must be resolved before the status check so a foreign version's status
	// cannot leak through the quality error path.
	datasetWorkspace, _, err := s.datasetRepo.GetWorkspaceAndType(ctx, version.DatasetID)
	if err != nil {
		return domain.Assessment{}, fmt.Errorf("resolve DatasetVersion dataset: %w", err)
	}
	if datasetWorkspace != cmd.WorkspaceID {
		return domain.Assessment{}, fmt.Errorf("%w: DatasetVersion %s belongs to workspace %s", datasetdomain.ErrDatasetWorkspace, version.ID, datasetWorkspace)
	}
	if version.Status != datasetdomain.VersionReady && version.Status != datasetdomain.VersionSuperseded {
		return domain.Assessment{}, fmt.Errorf("quality checks require READY or SUPERSEDED DatasetVersion, got %s", version.Status)
	}
	policyPath, err := industrypack.ResolvePath(s.industryPackRoot, cmd.RuleSetRef)
	if err != nil {
		return domain.Assessment{}, err
	}
	policy, err := native.LoadPolicy(policyPath)
	if err != nil {
		return domain.Assessment{}, err
	}
	reader, err := s.store.Get(ctx, version.StorageURI)
	if err != nil {
		return domain.Assessment{}, fmt.Errorf("open DatasetVersion object: %w", err)
	}
	defer reader.Close()
	table, err := tabular.ReadCSV(reader)
	if err != nil {
		return domain.Assessment{}, err
	}
	var result domain.Assessment
	err = s.tx.WithAdvisoryLock(ctx, assessmentAttemptLockPrefix+attemptID.String(), func(ctx context.Context) error {
		startedAt := time.Now().UTC()
		claimed, _, err := s.claimAttempt(ctx, cmd, attemptID, startedAt)
		if err != nil {
			return err
		}
		if !claimed {
			state, found, err := s.reconcileAttemptState(ctx, cmd, attemptID)
			if err != nil {
				return err
			}
			if !found {
				return ErrAssessmentAttemptInProgress
			}
			result, err = s.replayAttempt(ctx, state)
			return err
		}
		findings, metrics, err := native.Evaluate(policy, native.DatasetContext{
			Table:    table,
			Metadata: version.Metadata,
			ReadyAt:  version.ReadyAt,
			Now:      cmd.Now,
		})
		if err != nil {
			if outcomeErr := s.recordAttemptFailureAfterEvaluation(ctx, cmd, attemptID, err.Error(), time.Now().UTC()); outcomeErr != nil {
				return fmt.Errorf("quality evaluation failed: %v; record attempt outcome: %w", err, outcomeErr)
			}
			return err
		}
		result = domain.NewAssessment(cmd.WorkspaceID, version.ID, cmd.RuleSetRef, policy.Metadata.Version,
			policy.SourceContentSHA256, policy.SourceContent, native.EvaluatorName, native.EvaluatorVersion,
			metrics, findings, cmd.ActorID)

		err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if err := s.repo.InsertResult(ctx, tx, result); err != nil {
				return err
			}
			if err := s.datasetRepo.SetQualityStatus(ctx, tx, version.ID, string(result.GateDecision)); err != nil {
				return err
			}
			if _, err := evidence.Append(ctx, tx, evidence.Record{
				WorkspaceID:  cmd.WorkspaceID,
				EvidenceType: "QUALITY_RESULT",
				Title:        "Quality gate result",
				SourceType:   "QUALITY_RESULT",
				SourceID:     &result.ID,
				Metadata: map[string]any{
					"datasetVersionId":     version.ID,
					"ruleSetRef":           cmd.RuleSetRef,
					"ruleSetVersion":       result.RuleSetVersion,
					"ruleSetContentSha256": result.RuleSetContentSHA256,
					"gateDecision":         result.GateDecision,
					"metrics":              result.Metrics,
				},
				CreatedBy: cmd.ActorID,
			}, evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: version.ID, RelationType: "QUALITY_EVIDENCE"},
				evidence.Relation{ObjectType: "QUALITY_RESULT", ObjectID: result.ID, RelationType: "EVIDENCE_FOR"}); err != nil {
				return err
			}
			eventType := "QualityPassed"
			if result.GateDecision == domain.GateFail {
				eventType = "QualityFailed"
			} else if result.GateDecision == domain.GateReview {
				eventType = "QualityReviewRequired"
			}
			event, err := outbox.NewEvent("QUALITY_RESULT", result.ID, eventType, map[string]any{
				"qualityResultId":      result.ID,
				"datasetVersionId":     version.ID,
				"gateDecision":         result.GateDecision,
				"ruleSetVersion":       result.RuleSetVersion,
				"ruleSetContentSha256": result.RuleSetContentSHA256,
				"evaluatorName":        result.EvaluatorName,
				"evaluatorVersion":     result.EvaluatorVersion,
			})
			if err != nil {
				return err
			}
			if err := outbox.Append(ctx, tx, event); err != nil {
				return err
			}
			if err := audit.Append(ctx, tx, audit.Event{
				WorkspaceID: &cmd.WorkspaceID,
				ActorType:   actorType(cmd.ActorID),
				ActorID:     cmd.ActorID,
				Action:      "QUALITY_CHECK_COMPLETED",
				ObjectType:  "QUALITY_RESULT",
				ObjectID:    result.ID,
				AfterState: map[string]any{
					"datasetVersionId":     version.ID,
					"gateDecision":         result.GateDecision,
					"ruleSetVersion":       result.RuleSetVersion,
					"ruleSetContentSha256": result.RuleSetContentSHA256,
					"evaluatorName":        result.EvaluatorName,
					"evaluatorVersion":     result.EvaluatorVersion,
				},
				TraceID: cmd.TraceID,
			}); err != nil {
				return err
			}
			_, err = s.repo.AppendAssessmentAttemptOutcome(ctx, tx, attemptID, "SUCCEEDED", &result.ID, "", result.CreatedAt)
			return err
		})
		if err != nil {
			if outcomeErr := s.recordAttemptFailureAfterEvaluation(ctx, cmd, attemptID, err.Error(), time.Now().UTC()); outcomeErr != nil {
				return fmt.Errorf("persist quality assessment failed: %v; record attempt outcome: %w", err, outcomeErr)
			}
		}
		return err
	})
	if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		return domain.Assessment{}, ErrAssessmentAttemptInProgress
	}
	return result, err
}

func (s *Service) replayAttempt(ctx context.Context, state infrastructure.AssessmentAttemptState) (domain.Assessment, error) {
	if state.AssessmentID != nil {
		existing, err := s.repo.GetAssessment(ctx, *state.AssessmentID)
		if err != nil {
			return domain.Assessment{}, fmt.Errorf("load idempotent quality assessment: %w", err)
		}
		return existing, nil
	}
	if state.Outcome == "FAILED" {
		return domain.Assessment{}, fmt.Errorf("%w: %s", ErrAssessmentAttemptFailed, state.ErrorMessage)
	}
	return domain.Assessment{}, ErrAssessmentAttemptInProgress
}

func (s *Service) reconcileAttempt(ctx context.Context, cmd RunCommand, attemptID uuid.UUID) (domain.Assessment, bool, error) {
	var state infrastructure.AssessmentAttemptState
	var found bool
	err := s.tx.WithAdvisoryLock(ctx, assessmentAttemptLockPrefix+attemptID.String(), func(ctx context.Context) error {
		var err error
		state, found, err = s.reconcileAttemptState(ctx, cmd, attemptID)
		return err
	})
	if errors.Is(err, transaction.ErrAdvisoryLockBusy) {
		return domain.Assessment{}, true, ErrAssessmentAttemptInProgress
	}
	if err != nil || !found {
		return domain.Assessment{}, found, err
	}
	assessment, err := s.replayAttempt(ctx, state)
	return assessment, true, err
}

func (s *Service) reconcileAttemptState(ctx context.Context, cmd RunCommand, attemptID uuid.UUID) (infrastructure.AssessmentAttemptState, bool, error) {
	var state infrastructure.AssessmentAttemptState
	var found bool
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		attempt, attemptFound, err := s.repo.ReconcileAssessmentAttempt(ctx, tx, attemptID, cmd.WorkspaceID, cmd.DatasetVersionID, cmd.RuleSetRef, time.Now().UTC())
		if errors.Is(err, infrastructure.ErrAssessmentAttemptConflict) {
			return fmt.Errorf("%w: %v", ErrAssessmentAttemptConflict, err)
		}
		if err != nil {
			return err
		}
		found = attemptFound
		if attemptFound {
			state = attempt.State
			if attempt.LeaseExpired {
				if err := s.appendAttemptFailureFacts(ctx, tx, cmd, attemptID, attempt.State.ErrorMessage, time.Now().UTC(), nil, ""); err != nil {
					return fmt.Errorf("record reconciled quality assessment failure: %w", err)
				}
			}
		}
		return nil
	})
	return state, found, err
}

func (s *Service) claimAttempt(ctx context.Context, cmd RunCommand, attemptID uuid.UUID, startedAt time.Time) (bool, infrastructure.AssessmentAttemptState, error) {
	var claimed bool
	var state infrastructure.AssessmentAttemptState
	err := s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		claimed, state, err = s.repo.ClaimAssessmentAttempt(ctx, tx, attemptID, cmd.WorkspaceID,
			cmd.DatasetVersionID, cmd.RuleSetRef, startedAt, time.Now().UTC().Add(assessmentAttemptLeaseDuration), cmd.ActorID)
		if errors.Is(err, infrastructure.ErrAssessmentAttemptConflict) {
			return fmt.Errorf("%w: %v", ErrAssessmentAttemptConflict, err)
		}
		if err != nil || !claimed {
			return err
		}
		return cost.AppendQualityAssessmentAttemptActivity(ctx, tx, cost.QualityAssessmentAttemptActivity{
			WorkspaceID: cmd.WorkspaceID,
			AttemptID:   attemptID,
			CostType:    cost.QualityEngineInvocation,
			Quantity:    1,
			Unit:        "assessment",
			PricingMode: "ACTUAL",
			Metadata: map[string]any{
				"ruleSetRef": cmd.RuleSetRef,
				"stage":      "evaluation_started",
			},
			OccurredAt: startedAt,
		})
	})
	return claimed, state, err
}

func (s *Service) recordAttemptFailureAfterEvaluation(ctx context.Context, cmd RunCommand, attemptID uuid.UUID, errorMessage string, occurredAt time.Time) error {
	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), attemptOutcomeRecoveryTimeout)
	defer cancel()
	return s.tx.Do(recoveryCtx, func(ctx context.Context, tx pgx.Tx) error {
		return s.appendAttemptFailureFacts(ctx, tx, cmd, attemptID, errorMessage, occurredAt, cmd.ActorID, cmd.TraceID)
	})
}

func (s *Service) appendAttemptFailureFacts(ctx context.Context, tx pgx.Tx, cmd RunCommand, attemptID uuid.UUID, errorMessage string, occurredAt time.Time, actorID *uuid.UUID, traceID string) error {
	inserted, err := s.repo.AppendAssessmentAttemptOutcome(ctx, tx, attemptID, "FAILED", nil, errorMessage, occurredAt)
	if err != nil || !inserted {
		return err
	}
	payload := map[string]any{
		"qualityAssessmentAttemptId": attemptID,
		"workspaceId":                cmd.WorkspaceID,
		"datasetVersionId":           cmd.DatasetVersionID,
		"ruleSetRef":                 cmd.RuleSetRef,
		"outcome":                    "FAILED",
		"errorMessage":               errorMessage,
	}
	event, err := outbox.NewEvent("QUALITY_ASSESSMENT_ATTEMPT", attemptID, "QualityAssessmentAttemptFailed", payload)
	if err != nil {
		return err
	}
	if err := outbox.Append(ctx, tx, event); err != nil {
		return err
	}
	if _, err := evidence.Append(ctx, tx, evidence.Record{
		WorkspaceID:  cmd.WorkspaceID,
		EvidenceType: "QUALITY_ASSESSMENT_ATTEMPT_FAILED",
		Title:        "Quality assessment attempt failed",
		SourceType:   "QUALITY_ASSESSMENT_ATTEMPT",
		SourceID:     &attemptID,
		Metadata:     payload,
		CreatedAt:    occurredAt,
		CreatedBy:    actorID,
	}, evidence.Relation{ObjectType: "QUALITY_ASSESSMENT_ATTEMPT", ObjectID: attemptID, RelationType: "FAILURE_EVIDENCE"},
		evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: cmd.DatasetVersionID, RelationType: "QUALITY_ATTEMPT_FAILURE"}); err != nil {
		return err
	}
	return audit.Append(ctx, tx, audit.Event{
		WorkspaceID: &cmd.WorkspaceID,
		ActorType:   actorType(actorID),
		ActorID:     actorID,
		Action:      "QUALITY_ASSESSMENT_ATTEMPT_FAILED",
		ObjectType:  "QUALITY_ASSESSMENT_ATTEMPT",
		ObjectID:    attemptID,
		AfterState:  payload,
		TraceID:     traceID,
		OccurredAt:  occurredAt,
	})
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
