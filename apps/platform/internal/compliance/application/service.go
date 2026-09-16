package application

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	compliancedomain "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/domain"
	complianceinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/compliance/native"
	datasetdomain "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/domain"
	datasetinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/dataset/infrastructure"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/evidence"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/audit"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/industrypack"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/outbox"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/tabular"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/platform/transaction"
)

type ObjectStore interface {
	Get(ctx context.Context, storageURI string) (io.ReadCloser, error)
}

type Service struct {
	industryPackRoot string
	tx               *transaction.Manager
	datasetRepo      *datasetinfra.PostgresRepository
	repo             *complianceinfra.PostgresRepository
	store            ObjectStore
}

func NewService(industryPackRoot string, tx *transaction.Manager, datasetRepo *datasetinfra.PostgresRepository, repo *complianceinfra.PostgresRepository, store ObjectStore) *Service {
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
	PolicyRef        string
	ActorID          *uuid.UUID
	TraceID          string
}

func (s *Service) Run(ctx context.Context, cmd RunCommand) (compliancedomain.Result, error) {
	version, err := s.datasetRepo.GetVersion(ctx, cmd.DatasetVersionID)
	if err != nil {
		return compliancedomain.Result{}, err
	}
	if version.Status != datasetdomain.VersionReady && version.Status != datasetdomain.VersionSuperseded {
		return compliancedomain.Result{}, fmt.Errorf("compliance checks require READY or SUPERSEDED DatasetVersion, got %s", version.Status)
	}
	policyPath, err := industrypack.ResolvePath(s.industryPackRoot, cmd.PolicyRef)
	if err != nil {
		return compliancedomain.Result{}, err
	}
	policy, err := native.LoadPolicy(policyPath)
	if err != nil {
		return compliancedomain.Result{}, err
	}
	reader, err := s.store.Get(ctx, version.StorageURI)
	if err != nil {
		return compliancedomain.Result{}, fmt.Errorf("open DatasetVersion object: %w", err)
	}
	defer reader.Close()
	table, err := tabular.ReadCSV(reader)
	if err != nil {
		return compliancedomain.Result{}, err
	}
	findings, summary := native.Evaluate(policy, table)
	result := compliancedomain.NewResult(cmd.WorkspaceID, version.ID, cmd.PolicyRef, policy.Metadata.Version, summary, findings, cmd.ActorID)

	err = s.tx.Do(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.InsertResult(ctx, tx, result); err != nil {
			return err
		}
		if err := s.datasetRepo.SetComplianceStatus(ctx, tx, version.ID, string(result.GateDecision)); err != nil {
			return err
		}
		if _, err := evidence.Append(ctx, tx, evidence.Record{
			WorkspaceID:  cmd.WorkspaceID,
			EvidenceType: "COMPLIANCE_RESULT",
			Title:        "Compliance gate result",
			SourceType:   "COMPLIANCE_RESULT",
			SourceID:     &result.ID,
			Metadata: map[string]any{
				"datasetVersionId": version.ID,
				"policyRef":        cmd.PolicyRef,
				"policyVersion":    result.PolicyVersion,
				"gateDecision":     result.GateDecision,
				"summary":          result.Summary,
			},
			CreatedBy: cmd.ActorID,
		}, evidence.Relation{ObjectType: "DATASET_VERSION", ObjectID: version.ID, RelationType: "COMPLIANCE_EVIDENCE"},
			evidence.Relation{ObjectType: "COMPLIANCE_RESULT", ObjectID: result.ID, RelationType: "EVIDENCE_FOR"}); err != nil {
			return err
		}
		eventType := "CompliancePassed"
		if result.GateDecision == compliancedomain.GateFail {
			eventType = "ComplianceFailed"
		} else if result.GateDecision == compliancedomain.GateReview {
			eventType = "ComplianceReviewRequired"
		}
		event, err := outbox.NewEvent("COMPLIANCE_RESULT", result.ID, eventType, map[string]any{
			"complianceResultId": result.ID,
			"datasetVersionId":   version.ID,
			"gateDecision":       result.GateDecision,
			"policyVersion":      result.PolicyVersion,
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
			Action:      "COMPLIANCE_CHECK_COMPLETED",
			ObjectType:  "COMPLIANCE_RESULT",
			ObjectID:    result.ID,
			AfterState: map[string]any{
				"datasetVersionId": version.ID,
				"gateDecision":     result.GateDecision,
				"policyVersion":    result.PolicyVersion,
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
