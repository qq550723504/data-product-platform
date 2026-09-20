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
	} else {
		existing, found, err := s.loadExistingAttempt(ctx, cmd, attemptID)
		if err != nil {
			return domain.Assessment{}, err
		}
		if found {
			return existing, nil
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
	findings, metrics, err := native.Evaluate(policy, native.DatasetContext{
		Table:    table,
		Metadata: version.Metadata,
		ReadyAt:  version.ReadyAt,
		Now:      cmd.Now,
	})
	if err != nil {
		return domain.Assessment{}, err
	}
	result := domain.NewAssessment(cmd.WorkspaceID, version.ID, cmd.RuleSetRef, policy.Metadata.Version,
		policy.SourceContentSHA256, policy.SourceContent, native.EvaluatorName, native.EvaluatorVersion,
		metrics, findings, cmd.ActorID)

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertResult(ctx, tx, result); err != nil {
			return err
		}
		if err := cost.AppendQualityAssessmentActivity(ctx, tx, cost.QualityAssessmentActivity{
			WorkspaceID:  cmd.WorkspaceID,
			AssessmentID: result.ID,
			AttemptID:    attemptID,
			CostType:     cost.QualityEngineInvocation,
			Quantity:     1,
			Unit:         "assessment",
			PricingMode:  "ACTUAL",
			Metadata: map[string]any{
				"ruleSetRef":       result.RuleSetRef,
				"ruleSetVersion":   result.RuleSetVersion,
				"evaluatorName":    result.EvaluatorName,
				"evaluatorVersion": result.EvaluatorVersion,
			},
			OccurredAt: result.CreatedAt,
		}); err != nil {
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
		return audit.Append(ctx, tx, audit.Event{
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
		})
	})
	if err != nil && cmd.AssessmentAttemptID != uuid.Nil {
		// A concurrent caller may have committed the same physical attempt
		// between the initial lookup and this transaction. Recover its existing
		// assessment instead of surfacing a duplicate-allocation conflict.
		if existing, found, lookupErr := s.loadExistingAttempt(ctx, cmd, cmd.AssessmentAttemptID); lookupErr == nil && found {
			return existing, nil
		}
	}
	return result, err
}

func (s *Service) loadExistingAttempt(ctx context.Context, cmd RunCommand, attemptID uuid.UUID) (domain.Assessment, bool, error) {
	existingID, found, err := s.repo.FindAssessmentIDByAttempt(ctx, cmd.WorkspaceID, attemptID)
	if err != nil {
		return domain.Assessment{}, false, err
	}
	if !found {
		return domain.Assessment{}, false, nil
	}
	existing, err := s.repo.GetAssessment(ctx, existingID)
	if err != nil {
		return domain.Assessment{}, false, fmt.Errorf("load idempotent quality assessment: %w", err)
	}
	if existing.DatasetVersionID != cmd.DatasetVersionID || existing.RuleSetRef != cmd.RuleSetRef {
		return domain.Assessment{}, false, ErrAssessmentAttemptConflict
	}
	return existing, true, nil
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
