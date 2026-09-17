package application

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	WorkspaceID      uuid.UUID
	DatasetVersionID uuid.UUID
	RuleSetRef       string
	ActorID          *uuid.UUID
	TraceID          string
	Now              time.Time
}

func (s *Service) Run(ctx context.Context, cmd RunCommand) (domain.Result, error) {
	version, err := s.datasetRepo.GetVersion(ctx, cmd.DatasetVersionID)
	if err != nil {
		return domain.Result{}, err
	}
	// The result workspace is derived from the Dataset, not trusted from the caller.
	// A DatasetVersion foreign key proves the row exists, not which workspace owns it.
	// Ownership must be resolved before the status check so a foreign version's status
	// cannot leak through the quality error path.
	datasetWorkspace, _, err := s.datasetRepo.GetWorkspaceAndType(ctx, version.DatasetID)
	if err != nil {
		return domain.Result{}, fmt.Errorf("resolve DatasetVersion dataset: %w", err)
	}
	if datasetWorkspace != cmd.WorkspaceID {
		return domain.Result{}, fmt.Errorf("%w: DatasetVersion %s belongs to workspace %s", datasetdomain.ErrDatasetWorkspace, version.ID, datasetWorkspace)
	}
	if version.Status != datasetdomain.VersionReady && version.Status != datasetdomain.VersionSuperseded {
		return domain.Result{}, fmt.Errorf("quality checks require READY or SUPERSEDED DatasetVersion, got %s", version.Status)
	}
	policyPath, err := industrypack.ResolvePath(s.industryPackRoot, cmd.RuleSetRef)
	if err != nil {
		return domain.Result{}, err
	}
	policy, err := native.LoadPolicy(policyPath)
	if err != nil {
		return domain.Result{}, err
	}
	reader, err := s.store.Get(ctx, version.StorageURI)
	if err != nil {
		return domain.Result{}, fmt.Errorf("open DatasetVersion object: %w", err)
	}
	defer reader.Close()
	table, err := tabular.ReadCSV(reader)
	if err != nil {
		return domain.Result{}, err
	}
	findings, metrics, err := native.Evaluate(policy, native.DatasetContext{
		Table:    table,
		Metadata: version.Metadata,
		ReadyAt:  version.ReadyAt,
		Now:      cmd.Now,
	})
	if err != nil {
		return domain.Result{}, err
	}
	result := domain.NewResult(cmd.WorkspaceID, version.ID, cmd.RuleSetRef, policy.Metadata.Version, metrics, findings, cmd.ActorID)

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
				"datasetVersionId": version.ID,
				"ruleSetRef":       cmd.RuleSetRef,
				"ruleSetVersion":   result.RuleSetVersion,
				"gateDecision":     result.GateDecision,
				"metrics":          result.Metrics,
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
			"qualityResultId":  result.ID,
			"datasetVersionId": version.ID,
			"gateDecision":     result.GateDecision,
			"ruleSetVersion":   result.RuleSetVersion,
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
				"datasetVersionId": version.ID,
				"gateDecision":     result.GateDecision,
				"ruleSetVersion":   result.RuleSetVersion,
			},
			TraceID: cmd.TraceID,
		})
	})
	return result, err
}

func actorType(actorID *uuid.UUID) string {
	if actorID == nil {
		return "SYSTEM"
	}
	return "USER"
}
